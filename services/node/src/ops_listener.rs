use anyhow::{Context, Result};
use bytes::Bytes;
use http_body_util::{BodyExt, Full};
use hyper::body::Incoming;
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{Request, StatusCode};
use hyper_util::rt::TokioIo;
use std::sync::Arc;
use tokio::net::TcpListener;
use tokio::sync::broadcast;
use tokio::time::{timeout, Duration};
use tracing::{debug, error, info, warn};
use prometheus::Encoder;

use crate::config::ConfigHolder;
use crate::http_types::NodeResponse;
use crate::metrics::Metrics;

pub struct OpsListener {
    addr: String,
    token: Option<String>,
    metrics: Arc<Metrics>,
    config_holder: Arc<ConfigHolder>,
    _cache_dir: Option<String>,
}

impl OpsListener {
    pub fn new(
        addr: String,
        token: Option<String>,
        metrics: Arc<Metrics>,
        config_holder: Arc<ConfigHolder>,
        cache_dir: Option<String>,
    ) -> Self {
        Self {
            addr,
            token,
            metrics,
            config_holder,
            _cache_dir: cache_dir,
        }
    }

    pub async fn start(self, mut shutdown: broadcast::Receiver<()>) -> Result<()> {
        info!("Starting ops listener on {}", self.addr);

        let listener = TcpListener::bind(&self.addr)
            .await
            .context("Failed to bind ops listener")?;

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    info!("Ops listener shutdown requested; stopping accept loop");
                    break;
                }
                accept_result = listener.accept() => {
                    let (stream, peer) = match accept_result {
                        Ok(v) => v,
                        Err(e) => {
                            error!("Ops accept error: {}", e);
                            continue;
                        }
                    };
                    debug!("Accepted ops connection from {}", peer);

                    let io = TokioIo::new(stream);
                    let token = self.token.clone();
                    let metrics = self.metrics.clone();
                    let config_holder = self.config_holder.clone();

                    tokio::spawn(async move {
                        let service = service_fn(move |req: Request<Incoming>| {
                            let token = token.clone();
                            let metrics = metrics.clone();
                            let config_holder = config_holder.clone();
                            async move { Self::handle_request(req, token.as_deref(), metrics, config_holder).await }
                        });

                        let fut = http1::Builder::new().serve_connection(io, service);
                        if let Err(e) = timeout(Duration::from_secs(30), fut).await {
                            warn!("Ops connection timeout: {}", e);
                        }
                    });
                }
            }
        }

        Ok(())
    }

    async fn handle_request(
        req: Request<Incoming>,
        token: Option<&str>,
        metrics: Arc<Metrics>,
        config_holder: Arc<ConfigHolder>,
    ) -> Result<NodeResponse> {
        if let Some(expected) = token {
            let provided = req
                .headers()
                .get("x-ops-token")
                .and_then(|v| v.to_str().ok())
                .unwrap_or("");
            if provided != expected {
                return Ok(Self::plain(StatusCode::UNAUTHORIZED, "unauthorized"));
            }
        }

        let path = req.uri().path();
        match path {
            "/healthz" => Ok(Self::plain(StatusCode::OK, "ok")),
            "/readyz" => {
                let ready = config_holder.get_state().is_some();
                Ok(Self::plain(
                    if ready { StatusCode::OK } else { StatusCode::SERVICE_UNAVAILABLE },
                    if ready { "ready" } else { "not ready" },
                ))
            }
            "/metrics" => Ok(Self::metrics(metrics)),
            "/debug/config" => Ok(Self::debug_config(config_holder)),
            _ => Ok(Self::plain(StatusCode::NOT_FOUND, "not found")),
        }
    }

    fn plain(status: StatusCode, body: &str) -> NodeResponse {
        hyper::Response::builder()
            .status(status)
            .header("Content-Type", "text/plain; charset=utf-8")
            .body(Full::new(Bytes::from(body.to_string())).map_err(|e| match e {}).boxed())
            .unwrap()
    }

    fn metrics(metrics: Arc<Metrics>) -> NodeResponse {
        let encoder = prometheus::TextEncoder::new();
        let mf = metrics.registry().gather();
        let mut buf = Vec::new();
        if let Err(e) = encoder.encode(&mf, &mut buf) {
            return Self::plain(StatusCode::INTERNAL_SERVER_ERROR, &format!("encode error: {}", e));
        }

        hyper::Response::builder()
            .status(StatusCode::OK)
            .header("Content-Type", encoder.format_type())
            .body(Full::new(Bytes::from(buf)).map_err(|e| match e {}).boxed())
            .unwrap()
    }

    fn debug_config(config_holder: Arc<ConfigHolder>) -> NodeResponse {
        let state = match config_holder.get_state() {
            Some(s) => s,
            None => {
                return hyper::Response::builder()
                    .status(StatusCode::SERVICE_UNAVAILABLE)
                    .header("Content-Type", "application/json")
                    .body(Full::new(Bytes::from("{\"ok\":false,\"error\":\"no config\"}")).map_err(|e| match e {}).boxed())
                    .unwrap();
            }
        };

        let cfg = state.config.as_ref();
        let body = serde_json::json!({
            "ok": true,
            "version": cfg.version,
            "checksum": cfg.checksum,
            "domains": cfg.domains.len(),
            "origins": cfg.origins.len(),
            "certificates": cfg.certificates.len(),
            "cache_rules": cfg.cache_rules.len(),
            "waf_policies": cfg.waf_policies.len(),
            "waf_bans": cfg.waf_bans.len(),
            "waf_whitelist": cfg.waf_whitelist.len(),
        });

        hyper::Response::builder()
            .status(StatusCode::OK)
            .header("Content-Type", "application/json")
            .body(Full::new(Bytes::from(body.to_string())).map_err(|e| match e {}).boxed())
            .unwrap()
    }
}
