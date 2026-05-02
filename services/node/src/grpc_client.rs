use anyhow::{Context, Result};
use std::fs;
use tonic::transport::{Certificate, Channel, ClientTlsConfig, Endpoint, Identity};
use tonic::{metadata::MetadataValue, Request, Streaming};
use tracing::info;
use std::time::Duration;

use crate::proto::node::{
    node_control_client::NodeControlClient,
    RegisterNodeRequest, RegisterNodeResponse,
    HeartbeatRequest, HeartbeatResponse,
    ConfigAck, ConfigEnvelope,
    MetricsBatch,
    PurgeCommand, PurgeResult,
    CertificateRequest, CertificateResponse,
    GetCertificateRequest, GetCertificateResponse,
    WafBanReport, ReportWafBanRequest, ReportWafBanResponse,
};
use crate::config::NodeConfig;

#[derive(Clone)]
pub struct GrpcClient {
    client: NodeControlClient<Channel>,
    node_config: NodeConfig,
}

impl GrpcClient {
    pub async fn connect(node_config: NodeConfig) -> Result<Self> {
        info!("Connecting to control plane at {}", node_config.control_endpoint);

        let mut endpoint = Endpoint::from_shared(node_config.control_endpoint.clone())
            .context("Invalid control endpoint")?
            .timeout(Duration::from_secs(10))
            .connect_timeout(Duration::from_secs(5))
            .initial_connection_window_size(1024 * 1024)
            .tcp_keepalive(Some(Duration::from_secs(60)))
            .http2_keep_alive_interval(Duration::from_secs(30))
            .keep_alive_timeout(Duration::from_secs(10))
            .keep_alive_while_idle(true);

        if node_config.control_tls_enabled {
            let mut tls = ClientTlsConfig::new();
            if let Some(ref ca_path) = node_config.control_tls_ca_file {
                let ca_pem = fs::read(ca_path).with_context(|| format!("Failed to read CONTROL_CA_FILE {}", ca_path))?;
                tls = tls.ca_certificate(Certificate::from_pem(ca_pem));
            }
            if let (Some(ref cert_path), Some(ref key_path)) = (
                node_config.control_tls_client_cert_file.as_ref(),
                node_config.control_tls_client_key_file.as_ref(),
            ) {
                let cert_pem = fs::read(cert_path).with_context(|| format!("Failed to read CONTROL_CLIENT_CERT_FILE {}", cert_path))?;
                let key_pem = fs::read(key_path).with_context(|| format!("Failed to read CONTROL_CLIENT_KEY_FILE {}", key_path))?;
                tls = tls.identity(Identity::from_pem(cert_pem, key_pem));
            }
            if let Some(ref domain) = node_config.control_tls_domain_name {
                tls = tls.domain_name(domain);
            }
            endpoint = endpoint.tls_config(tls).context("Failed to apply control TLS config")?;
        }

        let channel = endpoint.connect_lazy();

        let client = NodeControlClient::new(channel);

        Ok(Self {
            client,
            node_config,
        })
    }

    pub async fn register_node(&mut self) -> Result<RegisterNodeResponse> {
        info!("Registering node with control plane");

        let request = Request::new(RegisterNodeRequest {
            bootstrap_token: self.node_config.bootstrap_token.clone(),
            hostname: self.node_config.hostname.clone(),
            version: self.node_config.version.clone(),
            capabilities: self.node_config.capabilities.clone(),
            region: self.node_config.region.clone().unwrap_or_default(),
        });

        let response = self.client
            .register_node(request)
            .await
            .context("Failed to register node")?
            .into_inner();

        info!("Node registered successfully: node_id={}", response.node_id);
        Ok(response)
    }

    /// Override the credential submitted in the next register_node() call.
    /// Used at startup to prefer a persisted per-node token over the
    /// configured bootstrap token — see NodeState in node_state.rs and
    /// authorizeNodeRegistration on the control plane.
    pub fn set_bootstrap_credential(&mut self, credential: String) {
        self.node_config.bootstrap_token = credential;
    }

    pub async fn heartbeat(
        &mut self,
        node_id: &str,
        token: &str,
        status: &str,
        metrics: std::collections::HashMap<String, String>,
        waf_bans: Vec<WafBanReport>,
    ) -> Result<HeartbeatResponse> {
        let request = Request::new(HeartbeatRequest {
            node_id: node_id.to_string(),
            token: token.to_string(),
            version: self.node_config.version.clone(),
            status: status.to_string(),
            metrics,
            region: self.node_config.region.clone().unwrap_or_default(),
            waf_bans,
        });

        let response = self.client
            .heartbeat(request)
            .await
            .context("Failed to send heartbeat")?
            .into_inner();

        Ok(response)
    }

    pub async fn stream_config(
        &mut self,
        node_id: &str,
        token: &str,
    ) -> Result<(
        tokio::sync::mpsc::Sender<ConfigAck>,
        Streaming<ConfigEnvelope>,
    )> {
        info!("Starting config stream");

        let (tx, mut rx) = tokio::sync::mpsc::channel::<ConfigAck>(32);

        let outbound = async_stream::stream! {
            while let Some(ack) = rx.recv().await {
                yield ack;
            }
        };

        let mut request = Request::new(outbound);
        request.metadata_mut().insert("node-id", MetadataValue::try_from(node_id).context("invalid node-id metadata")?);
        request.metadata_mut().insert("node-token", MetadataValue::try_from(token).context("invalid node-token metadata")?);

        let response = self.client
            .stream_config(request)
            .await
            .context("Failed to start config stream")?
            .into_inner();

        Ok((tx, response))
    }

    pub async fn stream_purge(
        &mut self,
        node_id: &str,
        token: &str,
    ) -> Result<(
        tokio::sync::mpsc::Sender<PurgeResult>,
        Streaming<PurgeCommand>,
    )> {
        let (tx, mut rx) = tokio::sync::mpsc::channel::<PurgeResult>(32);

        let outbound = async_stream::stream! {
            while let Some(res) = rx.recv().await {
                yield res;
            }
        };

        let mut request = Request::new(outbound);
        request.metadata_mut().insert("node-id", MetadataValue::try_from(node_id).context("invalid node-id metadata")?);
        request.metadata_mut().insert("node-token", MetadataValue::try_from(token).context("invalid node-token metadata")?);

        let response = match self.client.stream_purge(request).await {
            Ok(resp) => resp.into_inner(),
            Err(status) => {
                // Log the raw tonic Status so failures are diagnosable upstream.
                tracing::error!(
                    code = ?status.code(),
                    message = %status.message(),
                    "stream_purge RPC failed"
                );
                return Err(anyhow::anyhow!(
                    "stream_purge failed: code={:?} message={}",
                    status.code(),
                    status.message()
                ));
            }
        };

        Ok((tx, response))
    }

    pub async fn report_metrics(&mut self, node_id: &str, token: &str, batch: MetricsBatch) -> Result<HeartbeatResponse> {
        let mut request = Request::new(batch);
        request.metadata_mut().insert("node-id", MetadataValue::try_from(node_id).context("invalid node-id metadata")?);
        request.metadata_mut().insert("node-token", MetadataValue::try_from(token).context("invalid node-token metadata")?);

        let response = self.client
            .report_metrics(request)
            .await
            .context("Failed to report metrics")?
            .into_inner();

        Ok(response)
    }

    #[allow(dead_code)]
    pub async fn purge(&mut self, command: PurgeCommand) -> Result<PurgeResult> {
        info!("Executing purge command: request_id={}", command.request_id);

        let response = self.client
            .purge(Request::new(command))
            .await
            .context("Failed to execute purge")?
            .into_inner();

        Ok(response)
    }

    pub async fn request_certificate(
        &mut self,
        node_id: &str,
        domain: &str,
        csr_pem: String,
    ) -> Result<CertificateResponse> {
        let req = CertificateRequest {
            node_id: node_id.to_string(),
            domain: domain.to_string(),
            csr_pem,
            token: String::new(),
        };
        let response = self.client
            .request_certificate(Request::new(req))
            .await
            .context("Failed to request certificate")?
            .into_inner();
        Ok(response)
    }

    pub async fn get_certificate(
        &mut self,
        node_id: &str,
        token: &str,
        cert_id: &str,
    ) -> Result<GetCertificateResponse> {
        let req = GetCertificateRequest {
            node_id: node_id.to_string(),
            token: token.to_string(),
            cert_id: cert_id.to_string(),
        };
        let response = self
            .client
            .get_certificate(Request::new(req))
            .await
            .context("Failed to get certificate")?
            .into_inner();
        Ok(response)
    }

    pub async fn report_waf_ban(
        &mut self,
        node_id: &str,
        token: &str,
        ban: WafBanReport,
    ) -> Result<ReportWafBanResponse> {
        let req = ReportWafBanRequest {
            node_id: node_id.to_string(),
            token: token.to_string(),
            ban: Some(ban),
        };
        let response = self.client
            .report_waf_ban(Request::new(req))
            .await
            .context("Failed to report WAF ban")?
            .into_inner();
        Ok(response)
    }
}
