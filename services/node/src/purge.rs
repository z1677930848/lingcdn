use anyhow::Result;
use futures::StreamExt;
use std::sync::Arc;
use tokio::sync::broadcast;
use tracing::{error, info, warn};

use crate::cache::Cache;
use crate::grpc_client::GrpcClient;
use crate::proto::node::{PurgeCommand, PurgeResult};

pub struct PurgeAgent {
    cache: Arc<Cache>,
}

impl PurgeAgent {
    pub fn new(cache: Arc<Cache>) -> Self {
        Self { cache }
    }

    pub async fn start(
        self,
        mut grpc_client: GrpcClient,
        node_id: String,
        token: String,
        mut shutdown: broadcast::Receiver<()>,
    ) -> Result<()> {
        let (tx, mut stream) = grpc_client
            .stream_purge(&node_id, &token)
            .await?;

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    break;
                }
                maybe_cmd = stream.next() => {
                    match maybe_cmd {
                        Some(Ok(cmd)) => {
                            let res = self.execute_purge(&cmd).await;
                            if let Err(e) = tx.send(res).await {
                                return Err(anyhow::anyhow!("Failed to send purge result: {}", e));
                            }
                        }
                        Some(Err(e)) => {
                            return Err(anyhow::anyhow!("Purge stream error: {}", e));
                        }
                        None => {
                            return Err(anyhow::anyhow!("Purge stream ended"));
                        }
                    }
                }
            }
        }

        Ok(())
    }

    async fn execute_purge(&self, command: &PurgeCommand) -> PurgeResult {
        let purge_type = if command.purge_type.is_empty() {
            "url"
        } else {
            command.purge_type.as_str()
        };

        info!(
            "Executing purge: request_id={}, type={}, urls={}, prefixes={}, tags={}",
            command.request_id,
            purge_type,
            command.urls.len(),
            command.prefixes.len(),
            command.tags.len()
        );

        let mut success_count = 0u32;
        let mut error_count = 0u32;
        let mut errors = Vec::new();

        match purge_type {
            "prefix" => {
                for prefix in &command.prefixes {
                    match self.cache.purge_by_prefix(prefix) {
                        Ok(n) => success_count += n,
                        Err(e) => {
                            error_count += 1;
                            errors.push(format!("prefix {}: {}", prefix, e));
                        }
                    }
                }
            }
            "tag" => {
                for tag in &command.tags {
                    match self.cache.purge_by_tag(tag) {
                        Ok(n) => success_count += n,
                        Err(e) => {
                            error_count += 1;
                            errors.push(format!("tag {}: {}", tag, e));
                        }
                    }
                }
            }
            _ => {
                for url in &command.urls {
                    match self.cache.purge_by_url(url) {
                        Ok(removed) => {
                            if removed {
                                success_count += 1;
                                info!("Purged: {}", url);
                            } else {
                                warn!("URL not in cache: {}", url);
                            }
                        }
                        Err(e) => {
                            error_count += 1;
                            let err_msg = format!("Failed to purge {}: {}", url, e);
                            error!("{}", err_msg);
                            errors.push(err_msg);
                        }
                    }
                }
            }
        }

        let ok = error_count == 0;
        let reason = if ok {
            format!("Purged {} entries successfully", success_count)
        } else {
            format!(
                "Purged {} entries, {} errors: {}",
                success_count,
                error_count,
                errors.join("; ")
            )
        };

        PurgeResult {
            request_id: command.request_id.clone(),
            ok,
            reason,
        }
    }
}
