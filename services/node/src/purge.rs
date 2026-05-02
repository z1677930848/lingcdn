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
        info!("Executing purge: request_id={}, urls={}", command.request_id, command.urls.len());

        let mut success_count = 0;
        let mut error_count = 0;
        let mut errors = Vec::new();

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

        let ok = error_count == 0;
        let reason = if ok {
            format!("Purged {} URLs successfully", success_count)
        } else {
            format!("Purged {} URLs, {} errors: {}", success_count, error_count, errors.join("; "))
        };

        PurgeResult {
            request_id: command.request_id.clone(),
            ok,
            reason,
        }
    }
}
