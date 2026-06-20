use anyhow::Result;
use rcgen::{Certificate, CertificateParams, DnType, PKCS_ECDSA_P256_SHA256, SanType};
use std::collections::HashMap;
use tracing::{info, warn};

use crate::config::{CertificateConfig, DomainConfig};
use crate::grpc_client::GrpcClient;

pub struct CertificateManager {
    client: GrpcClient,
}

impl CertificateManager {
    pub fn new(client: GrpcClient) -> Self {
        Self { client }
    }

    /// Request certificates for domains missing TLS material; returns configs keyed by domain.
    ///
    /// `token` is the per-node token handed out by RegisterNode — the control
    /// plane's RequestCertificate rejects empty tokens (previously this
    /// function passed "" and the entire self-heal path was dead code).
    pub async fn request_certificates(
        &mut self,
        node_id: &str,
        token: &str,
        domains: &[DomainConfig],
    ) -> Result<HashMap<String, CertificateConfig>> {
        let mut generated = HashMap::new();

        for domain in domains {
            let csr = Self::build_csr(&domain.name)?;

            match self
                .client
                .request_certificate(node_id, token, &domain.name, csr.csr_pem.clone())
                .await
            {
                Ok(resp) => {
                    if !resp.ok {
                        warn!("Certificate request failed {}: {}", domain.name, resp.reason);
                        continue;
                    }

                    let cert_cfg = CertificateConfig {
                        id: domain.name.clone(),
                        domain: domain.name.clone(),
                        cert_pem: Some(resp.cert_pem.into_bytes()),
                        key_pem: Some(csr.key_pem.clone().into_bytes()),
                    };
                    generated.insert(domain.name.clone(), cert_cfg);
                    info!("Certificate issued: {}", domain.name);
                }
                Err(e) => {
                    warn!("Certificate request error {}: {}", domain.name, e);
                }
            }
        }

        Ok(generated)
    }

    fn build_csr(domain: &str) -> Result<CsrBundle> {
        let mut params = CertificateParams::new(vec![domain.to_string()]);
        params
            .distinguished_name
            .push(DnType::CommonName, domain.to_string());
        params.alg = &PKCS_ECDSA_P256_SHA256;
        params.subject_alt_names = vec![SanType::DnsName(domain.to_string())];

        let cert = Certificate::from_params(params)?;
        let csr_pem = cert.serialize_request_pem()?;
        let key_pem = cert.serialize_private_key_pem();
        Ok(CsrBundle { key_pem, csr_pem })
    }
}

struct CsrBundle {
    key_pem: String,
    csr_pem: String,
}
