use anyhow::{Context, Result, anyhow};
use futures::StreamExt;
use std::collections::HashSet;
use std::sync::Arc;
use tokio::sync::broadcast;
use tokio::sync::Semaphore;
use tokio::task::JoinSet;
use tracing::{error, info, warn};

use crate::cert_store::CertStore;
use crate::config::{ConfigHolder, RuntimeConfig};
use crate::grpc_client::GrpcClient;
use crate::proto::node::{ConfigAck, ConfigEnvelope};

pub struct ConfigAgent {
    config_holder: Arc<ConfigHolder>,
    tls_enabled: bool,
    cert_store: Option<Arc<CertStore>>,
}

impl ConfigAgent {
    pub fn new(config_holder: Arc<ConfigHolder>, tls_enabled: bool, cert_store: Option<Arc<CertStore>>) -> Self {
        Self { config_holder, tls_enabled, cert_store }
    }

    pub async fn start(
        self,
        mut grpc_client: GrpcClient,
        node_id: String,
        token: String,
        mut shutdown: broadcast::Receiver<()>,
    ) -> Result<()> {
        info!("Starting config agent");

        let (ack_tx, mut config_stream) = grpc_client
            .stream_config(&node_id, &token)
            .await
            .context("Failed to start config stream")?;

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    info!("Config agent shutdown requested");
                    break;
                }
                maybe_msg = config_stream.next() => {
                    match maybe_msg {
                        Some(Ok(envelope)) => {
                            info!("Received config: version={}, checksum={}", envelope.version, envelope.checksum);

                            let ack = self.process_config(&grpc_client, &node_id, &token, envelope).await;

                            if let Err(e) = ack_tx.send(ack).await {
                                return Err(anyhow!("Failed to send config ack: {}", e));
                            }
                        }
                        Some(Err(e)) => {
                            return Err(anyhow!("Config stream error: {}", e));
                        }
                        None => {
                            warn!("Config stream ended unexpectedly by server (connection may have been closed or server restarted)");
                            return Err(anyhow!("Config stream ended unexpectedly (server closed the connection)"));
                        }
                    }
                }
            }
        }

        Ok(())
    }

    async fn process_config(&self, grpc_client: &GrpcClient, node_id: &str, token: &str, envelope: ConfigEnvelope) -> ConfigAck {
        // Validate checksum
        if !self.config_holder.validate_checksum(&envelope.payload, &envelope.checksum) {
            error!("Config checksum mismatch: expected={}", envelope.checksum);
            return ConfigAck {
                version: envelope.version,
                ok: false,
                reason: "Checksum validation failed".to_string(),
            };
        }

        // Deserialize config
        let mut config: RuntimeConfig = match serde_json::from_slice(&envelope.payload) {
            Ok(c) => c,
            Err(e) => {
                error!("Failed to deserialize config: {}", e);
                return ConfigAck {
                    version: envelope.version,
                    ok: false,
                    reason: format!("Deserialization failed: {}", e),
                };
            }
        };

        // Validate config
        if let Err(e) = self.validate_config(&config) {
            error!("Config validation failed: {}", e);
            return ConfigAck {
                version: envelope.version,
                ok: false,
                reason: format!("Validation failed: {}", e),
            };
        }

        if self.tls_enabled {
            if let Err(e) = self.ensure_certificates(grpc_client, node_id, token, &mut config).await {
                error!("Certificate sync failed: {}", e);
                return ConfigAck {
                    version: envelope.version,
                    ok: false,
                    reason: format!("Certificate sync failed: {}", e),
                };
            }
        }

        // Apply config atomically
        self.config_holder.update(config);
        info!("Config applied successfully: version={}", envelope.version);

        ConfigAck {
            version: envelope.version,
            ok: true,
            reason: "Config applied successfully".to_string(),
        }
    }

    async fn ensure_certificates(
        &self,
        grpc_client: &GrpcClient,
        node_id: &str,
        token: &str,
        config: &mut RuntimeConfig,
    ) -> Result<()> {
        let Some(cert_store) = self.cert_store.as_ref() else {
            return Ok(());
        };

        // 1) Backward compatibility: if the control plane embeds PEM blobs, persist them to disk
        // and drop from memory to keep runtime RSS stable.
        for (id, cert_cfg) in config.certificates.iter_mut() {
            let (Some(cert_pem), Some(key_pem)) = (cert_cfg.cert_pem.take(), cert_cfg.key_pem.take()) else {
                continue;
            };
            if cert_pem.is_empty() || key_pem.is_empty() {
                continue;
            }

            let mut merged = Vec::with_capacity(cert_pem.len() + 1 + key_pem.len() + 1);
            merged.extend_from_slice(&cert_pem);
            merged.push(b'\n');
            merged.extend_from_slice(&key_pem);
            merged.push(b'\n');

            cert_store
                .put_pem_if_absent(id, &merged)
                .with_context(|| format!("store inline cert_id={}", id))?;
        }

        // 2) Collect required cert IDs from domains.
        let mut needed: HashSet<String> = HashSet::new();
        for d in &config.domains {
            let cert_id = d
                .cert_id
                .as_deref()
                .unwrap_or(d.name.as_str())
                .trim();
            if !cert_id.is_empty() {
                needed.insert(cert_id.to_string());
            }
        }

        // 3) Fetch missing certs (bounded concurrency).
        let mut missing: Vec<String> = needed
            .into_iter()
            .filter(|id| !cert_store.has_pem(id))
            .collect();
        missing.sort();

        if missing.is_empty() {
            return Ok(());
        }

        let concurrency = std::env::var("CERT_PREFETCH_CONCURRENCY")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or_else(|| crate::autotune::default_cert_prefetch_concurrency());
        let sem = Arc::new(Semaphore::new(concurrency.max(1)));
        let mut join = JoinSet::new();

        info!("Prefetching {} certificates (concurrency={})", missing.len(), concurrency);

        for cert_id in missing {
            let permit = sem.clone().acquire_owned().await?;
            let mut client = grpc_client.clone();
            let cert_store = cert_store.clone();
            let node_id = node_id.to_string();
            let token = token.to_string();

            join.spawn(async move {
                let _permit = permit;
                let resp = client
                    .get_certificate(&node_id, &token, &cert_id)
                    .await
                    .with_context(|| format!("GetCertificate cert_id={}", cert_id))?;

                if !resp.ok {
                    return Err(anyhow!("GetCertificate {} failed: {}", cert_id, resp.reason));
                }
                if resp.cert_pem.trim().is_empty() || resp.key_pem.trim().is_empty() {
                    return Err(anyhow!("GetCertificate {} returned empty cert/key", cert_id));
                }

                let merged = format!("{}\n{}\n", resp.cert_pem.trim_end(), resp.key_pem.trim_end());
                cert_store
                    .put_pem_if_absent(&cert_id, merged.as_bytes())
                    .with_context(|| format!("persist fetched cert_id={}", cert_id))?;
                Ok::<(), anyhow::Error>(())
            });
        }

        // Partial application: collect all cert fetch failures but do not abort
        // the whole config. Previously one bad cert would reject the entire
        // config update, taking the node out of service. Now we apply what we
        // have and let TLS handshake fail only for the specific affected domain.
        let mut failures: Vec<String> = Vec::new();
        while let Some(res) = join.join_next().await {
            match res {
                Ok(Ok(())) => {}
                Ok(Err(e)) => failures.push(e.to_string()),
                Err(e) => failures.push(format!("cert prefetch task failed: {}", e)),
            }
        }
        if !failures.is_empty() {
            tracing::warn!(
                count = failures.len(),
                "cert prefetch partial failure; config applied with missing certs: {}",
                failures.join("; ")
            );
        }

        Ok(())
    }

    fn validate_config(&self, config: &RuntimeConfig) -> Result<()> {
        // Validate that every domain has at least one usable upstream.
        // A domain is valid when EITHER:
        //   (a) it carries per-domain inline origins (new model, see
        //       proxy.rs origin resolution), OR
        //   (b) its legacy origin_id resolves in the global pool.
        // Mirroring the runtime resolution rule keeps the validator from
        // rejecting the whole config whenever origin_id points to a row
        // that has been removed/migrated away from the global table.
        for domain in &config.domains {
            let has_domain_origins = domain
                .origins
                .iter()
                .any(|e| e.enabled && !e.address.trim().is_empty());
            let has_legacy_origin = config.origins.contains_key(&domain.origin_id);
            if !has_domain_origins && !has_legacy_origin {
                return Err(anyhow::anyhow!(
                    "Domain {} has no usable upstream (origin_id={} not in global pool and no per-domain origins)",
                    domain.name,
                    domain.origin_id
                ));
            }
        }

        // Validate legacy global-pool origins have at least one address.
        // Per-domain inline origins are validated implicitly by the
        // has_domain_origins check above (enabled + non-empty address).
        for (id, origin) in &config.origins {
            if origin.addresses.is_empty() {
                return Err(anyhow::anyhow!("Origin {} has no addresses", id));
            }
        }

        Ok(())
    }
}
