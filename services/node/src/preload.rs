use anyhow::Result;
use futures::StreamExt;
use std::sync::Arc;
use tokio::sync::broadcast;
use tracing::{error, info, warn};

use crate::grpc_client::GrpcClient;
use crate::proto::node::{PreloadCommand, PreloadResult};
use crate::proxy::ProxyService;

pub struct PreloadAgent {
    proxy: Arc<ProxyService>,
}

impl PreloadAgent {
    pub fn new(proxy: Arc<ProxyService>) -> Self {
        Self { proxy }
    }

    pub async fn start(
        self,
        mut grpc_client: GrpcClient,
        node_id: String,
        token: String,
        mut shutdown: broadcast::Receiver<()>,
    ) -> Result<()> {
        let (tx, mut stream) = grpc_client
            .stream_preload(&node_id, &token)
            .await?;

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    break;
                }
                maybe_cmd = stream.next() => {
                    match maybe_cmd {
                        Some(Ok(cmd)) => {
                            let res = self.execute_preload(&cmd).await;
                            if let Err(e) = tx.send(res).await {
                                return Err(anyhow::anyhow!("Failed to send preload result: {}", e));
                            }
                        }
                        Some(Err(e)) => {
                            return Err(anyhow::anyhow!("Preload stream error: {}", e));
                        }
                        None => {
                            return Err(anyhow::anyhow!("Preload stream ended"));
                        }
                    }
                }
            }
        }

        Ok(())
    }

    async fn execute_preload(&self, command: &PreloadCommand) -> PreloadResult {
        info!(
            "Executing preload: request_id={}, urls={}",
            command.request_id,
            command.urls.len()
        );

        let mut loaded = 0i32;
        let mut errors = Vec::new();

        for url in &command.urls {
            match self.proxy.preload_url(url).await {
                Ok(true) => {
                    loaded += 1;
                    info!("Preloaded: {}", url);
                }
                Ok(false) => {
                    warn!("Preload skipped (no cache rule): {}", url);
                }
                Err(e) => {
                    let err_msg = format!("Failed to preload {}: {}", url, e);
                    error!("{}", err_msg);
                    errors.push(err_msg);
                }
            }
        }

        let ok = errors.is_empty();
        let reason = if ok {
            format!("Preloaded {} URLs successfully", loaded)
        } else {
            format!(
                "Preloaded {} URLs, {} errors: {}",
                loaded,
                errors.len(),
                errors.join("; ")
            )
        };

        PreloadResult {
            request_id: command.request_id.clone(),
            ok,
            reason,
            loaded,
        }
    }
}
