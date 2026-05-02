use anyhow::{Context, Result};
use bytes::Bytes;
use http_body_util::{BodyExt, Full};
use hyper::body::Incoming;
use hyper::server::conn::http1;
use hyper::server::conn::http2;
use hyper::service::service_fn;
use hyper::{Request, StatusCode};
use hyper_util::rt::TokioExecutor;
use hyper_util::rt::TokioIo;
use rustls::ServerConfig;
use rustls::server::{ClientHello, ResolvesServerCert};
use rustls::crypto::ring::sign::any_supported_type;
use rustls::pki_types::{CertificateDer, PrivateKeyDer};
use rustls::sign::CertifiedKey;
use std::collections::HashMap;
use std::io::BufReader;
use std::sync::Arc;
use tokio::net::TcpListener;
use tokio::sync::broadcast;
use tokio::sync::RwLock;
use tokio::sync::Semaphore;
use tokio::task::JoinSet;
use tokio::time::{Duration, timeout};
use tokio_rustls::TlsAcceptor;
use tracing::{debug, error, info, warn};

use crate::cert_store::CertStore;
use crate::config::{CertificateConfig, ConfigHolder};
use crate::http_types::NodeResponse;
use crate::metrics::Metrics;
use crate::proxy::ProxyService;

#[derive(Debug)]
struct MapResolver {
    exact: HashMap<String, Arc<CertifiedKey>>,
    wildcards: HashMap<String, Arc<CertifiedKey>>,
    fallback: Arc<CertifiedKey>,
}

impl ResolvesServerCert for MapResolver {
    fn resolve(&self, client_hello: ClientHello) -> Option<Arc<CertifiedKey>> {
        let sni = client_hello.server_name();
        let Some(host) = sni else {
            return Some(self.fallback.clone());
        };

        if let Some(key) = self.exact.get(host) {
            return Some(key.clone());
        }

        // Try each possible suffix from most-specific to least-specific.
        let mut rest = host;
        while let Some(dot_pos) = rest.find('.') {
            let suffix = &rest[dot_pos + 1..];
            if let Some(key) = self.wildcards.get(suffix) {
                return Some(key.clone());
            }
            rest = suffix;
        }

        Some(self.fallback.clone())
    }
}

struct CertIdResolver {
    exact: HashMap<String, String>,
    wildcards: HashMap<String, String>,
    fallback: Arc<CertifiedKey>,
    cert_store: Arc<CertStore>,
}

impl std::fmt::Debug for CertIdResolver {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("CertIdResolver")
            .field("exact", &self.exact)
            .field("wildcards", &self.wildcards)
            .finish()
    }
}

impl ResolvesServerCert for CertIdResolver {
    fn resolve(&self, client_hello: ClientHello) -> Option<Arc<CertifiedKey>> {
        let host = match client_hello.server_name() {
            Some(h) => h,
            None => return Some(self.fallback.clone()),
        };

        let mut cert_id = self.exact.get(host).map(|s| s.as_str());
        if cert_id.is_none() {
            // Try each possible suffix from most-specific to least-specific.
            let mut rest = host;
            while let Some(dot_pos) = rest.find('.') {
                let suffix = &rest[dot_pos + 1..];
                if let Some(id) = self.wildcards.get(suffix) {
                    cert_id = Some(id.as_str());
                    break;
                }
                rest = suffix;
            }
        }

        if let Some(id) = cert_id {
            match self.cert_store.get_certified_key(id) {
                Ok(Some(key)) => return Some(key),
                Ok(None) => {
                    warn!(cert_id = id, sni = host, "Certificate not found in store, falling back to default");
                }
                Err(e) => {
                    warn!(cert_id = id, sni = host, error = %e, "Failed to load certificate from store, falling back to default");
                }
            }
        }

        Some(self.fallback.clone())
    }
}

pub struct Listener {
    addr: String,
    tls_enabled: bool,
    config_holder: Arc<ConfigHolder>,
    proxy_service: Arc<ProxyService>,
    metrics: Arc<Metrics>,
    connection_limit: Arc<Semaphore>,
    max_connections: usize,
    cache_dir: Option<String>,
    cert_store: Option<Arc<CertStore>>,
    // Cache the built TLS acceptor to avoid rebuilding/parsing all certs per connection.
    tls_acceptor_cache: Arc<RwLock<Option<TlsAcceptorCache>>>,
}

#[derive(Clone)]
struct TlsAcceptorCache {
    key: String,
    acceptor: TlsAcceptor,
}

impl Listener {
    pub fn new(
        addr: String,
        tls_enabled: bool,
        config_holder: Arc<ConfigHolder>,
        proxy_service: Arc<ProxyService>,
        metrics: Arc<Metrics>,
        max_connections: usize,
        cache_dir: Option<String>,
        cert_store: Option<Arc<CertStore>>,
    ) -> Self {
        Self {
            addr,
            tls_enabled,
            config_holder,
            proxy_service,
            metrics,
            connection_limit: Arc::new(Semaphore::new(max_connections)),
            max_connections,
            cache_dir,
            cert_store,
            tls_acceptor_cache: Arc::new(RwLock::new(None)),
        }
    }

    pub async fn start(self, mut shutdown: broadcast::Receiver<()>) -> Result<()> {
        info!("Starting listener on {}", self.addr);

        let listener = TcpListener::bind(&self.addr)
            .await
            .context("Failed to bind listener")?;

        info!("Listening on {}", self.addr);

        let mut connections = JoinSet::new();

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    info!("Listener shutdown requested; stopping accept loop");
                    break;
                }
                accept_result = listener.accept() => {
                    let (stream, peer_addr) = match accept_result {
                        Ok(conn) => conn,
                        Err(e) => {
                            error!("Failed to accept connection: {}", e);
                            continue;
                        }
                    };

                    debug!("Accepted connection from {}", peer_addr);
                    let permit = match self.connection_limit.clone().try_acquire_owned() {
                        Ok(p) => p,
                        Err(_) => {
                            warn!("Too many concurrent connections (>{}); dropping {}", self.max_connections, peer_addr);
                            continue;
                        }
                    };
                    self.metrics.active_connections.inc();

                    let proxy_service = self.proxy_service.clone();
                    let metrics = self.metrics.clone();
                    let config_holder = self.config_holder.clone();
                    let tls_enabled = self.tls_enabled;
                    let cache_dir = self.cache_dir.clone();
                    let max_connections = self.max_connections;
                    let tls_acceptor_cache = self.tls_acceptor_cache.clone();
                    let cert_store = self.cert_store.clone();

                    connections.spawn(async move {
                        let result = if tls_enabled {
                            Self::handle_tls_connection(
                                stream,
                                peer_addr,
                                proxy_service,
                                config_holder,
                                metrics.clone(),
                                cache_dir.clone(),
                                max_connections,
                                tls_acceptor_cache,
                                cert_store,
                            )
                            .await
                        } else {
                            Self::handle_plain_connection(stream, peer_addr, proxy_service, config_holder, metrics.clone(), cache_dir.clone(), max_connections).await
                        };

                        if let Err(e) = result {
                            error!("Connection error: {}", e);
                        }

                        metrics.active_connections.dec();
                        drop(permit);
                    });
                }
            }
        }

        info!("Waiting for {} in-flight connections to drain", connections.len());
        let drain = async {
            while let Some(join_result) = connections.join_next().await {
                if let Err(e) = join_result {
                    error!("Connection task join error: {}", e);
                }
            }
        };

        if timeout(Duration::from_secs(10), drain).await.is_err() {
            warn!("Timed out waiting for existing connections to finish");
        }

        Ok(())
    }

    async fn handle_plain_connection(
        stream: tokio::net::TcpStream,
        peer_addr: std::net::SocketAddr,
        proxy_service: Arc<ProxyService>,
        config_holder: Arc<ConfigHolder>,
        metrics: Arc<Metrics>,
        cache_dir: Option<String>,
        max_connections: usize,
    ) -> Result<()> {
        let io = TokioIo::new(stream);

        let service = service_fn(move |mut req: Request<Incoming>| {
            req.extensions_mut().insert(peer_addr);
            let proxy_service = proxy_service.clone();
            let metrics = metrics.clone();
            let config_holder = config_holder.clone();
            let cache_dir = cache_dir.clone();
            async move {
                Self::handle_request(req, proxy_service, config_holder, metrics, cache_dir, max_connections).await
            }
        });

        http1::Builder::new()
            .preserve_header_case(true)
            .title_case_headers(true)
            .serve_connection(io, service)
            .await
            .context("Failed to serve connection")?;

        Ok(())
    }

    async fn handle_tls_connection(
        stream: tokio::net::TcpStream,
        peer_addr: std::net::SocketAddr,
        proxy_service: Arc<ProxyService>,
        config_holder: Arc<ConfigHolder>,
        metrics: Arc<Metrics>,
        cache_dir: Option<String>,
        max_connections: usize,
        tls_acceptor_cache: Arc<RwLock<Option<TlsAcceptorCache>>>,
        cert_store: Option<Arc<CertStore>>,
    ) -> Result<()> {
        // Build (or reuse) TLS acceptor with SNI support.
        // IMPORTANT: never rebuild per connection; that becomes O(cert_count) per handshake.
        let tls_acceptor = Self::get_or_build_tls_acceptor(&config_holder, &tls_acceptor_cache, cert_store.as_ref()).await?;

        let tls_stream = tls_acceptor
            .accept(stream)
            .await
            .context("TLS handshake failed")?;

        let use_h2 = tls_stream
            .get_ref()
            .1
            .alpn_protocol()
            .map(|p| p == b"h2")
            .unwrap_or(false);

        let io = TokioIo::new(tls_stream);

        let service = service_fn(move |mut req: Request<Incoming>| {
            req.extensions_mut().insert(peer_addr);
            let proxy_service = proxy_service.clone();
            let metrics = metrics.clone();
            let config_holder = config_holder.clone();
            let cache_dir = cache_dir.clone();
            async move {
                Self::handle_request(req, proxy_service, config_holder, metrics, cache_dir, max_connections).await
            }
        });

        if use_h2 {
            http2::Builder::new(TokioExecutor::new())
                .serve_connection(io, service)
                .await
                .context("Failed to serve h2 connection")?;
        } else {
            http1::Builder::new()
                .preserve_header_case(true)
                .title_case_headers(true)
                .serve_connection(io, service)
                .await
                .context("Failed to serve connection")?;
        }

        Ok(())
    }

    async fn get_or_build_tls_acceptor(
        config_holder: &Arc<ConfigHolder>,
        cache: &Arc<RwLock<Option<TlsAcceptorCache>>>,
        cert_store: Option<&Arc<CertStore>>,
    ) -> Result<TlsAcceptor> {
        let config = config_holder.get().context("No runtime config available")?;
        let key = format!("{}:{}", config.version, config.checksum);

        {
            let guard = cache.read().await;
            if let Some(ref cached) = *guard {
                if cached.key == key {
                    return Ok(cached.acceptor.clone());
                }
            }
        }

        let mut guard = cache.write().await;
        // Re-check after acquiring the write lock (another task could have refreshed it).
        if let Some(ref cached) = *guard {
            if cached.key == key {
                return Ok(cached.acceptor.clone());
            }
        }

        let acceptor = Self::build_tls_acceptor_from_config(&config, cert_store)?;
        *guard = Some(TlsAcceptorCache {
            key,
            acceptor: acceptor.clone(),
        });
        Ok(acceptor)
    }

    async fn handle_request(
        req: Request<Incoming>,
        proxy_service: Arc<ProxyService>,
        _config_holder: Arc<ConfigHolder>,
        metrics: Arc<Metrics>,
        _cache_dir: Option<String>,
        _max_connections: usize,
    ) -> Result<NodeResponse> {
        metrics.requests_total.inc();

        let path = req.uri().path();

        if path == "/metrics" || path == "/healthz" || path == "/readyz" {
            return Ok(Self::plain_response(StatusCode::NOT_FOUND, "not found", "text/plain; charset=utf-8"));
        }

        // Proxy the request
        let response = proxy_service.handle_request(req).await?;

        // Update metrics
        metrics.requests_by_status.inc();

        Ok(response)
    }

    fn plain_response(status: StatusCode, body: &str, content_type: &str) -> NodeResponse {
        let b = Full::new(Bytes::from(body.to_string()))
            .map_err(|e| match e {})
            .boxed();
        hyper::Response::builder()
            .status(status)
            .header("Content-Type", content_type)
            .body(b)
            .unwrap()
    }

    fn build_tls_acceptor_from_config(
        config: &crate::config::RuntimeConfig,
        cert_store: Option<&Arc<CertStore>>,
    ) -> Result<TlsAcceptor> {
        if let Some(store) = cert_store {
            // Build a resolver keyed by SNI -> cert_id. The actual keys are loaded from disk on-demand.
            let mut exact: HashMap<String, String> = HashMap::new();
            let mut wildcards: HashMap<String, String> = HashMap::new();
            let mut fallback_cert_id: Option<String> = None;

            for domain in &config.domains {
                let cert_id = domain
                    .cert_id
                    .as_deref()
                    .unwrap_or(domain.name.as_str());

                if fallback_cert_id.is_none() && !cert_id.is_empty() {
                    fallback_cert_id = Some(cert_id.to_string());
                }

                if domain.name.starts_with("*.") {
                    wildcards.entry(domain.name[2..].to_string()).or_insert(cert_id.to_string());
                } else {
                    exact.entry(domain.name.clone()).or_insert(cert_id.to_string());
                }
            }

            if fallback_cert_id.is_none() && !config.certificates.is_empty() {
                // If there are no domains, fall back to any certificate entry.
                fallback_cert_id = config.certificates.keys().next().cloned();
            }

            let fallback_cert_id = fallback_cert_id.context("No certificates available")?;
            let fallback = store
                .get_certified_key(&fallback_cert_id)?
                .context("fallback cert not present on disk")?;

            let resolver = CertIdResolver {
                exact,
                wildcards,
                fallback,
                cert_store: store.clone(),
            };

            let tls_config = ServerConfig::builder()
                .with_no_client_auth()
                .with_cert_resolver(Arc::new(resolver));
            let mut tls_config = tls_config;
            tls_config.alpn_protocols = vec![b"h2".to_vec(), b"http/1.1".to_vec()];
            return Ok(TlsAcceptor::from(Arc::new(tls_config)));
        }

        // Fallback: keep old behavior (all certs in-memory) when disk store is unavailable.
        let mut cert_cache: HashMap<String, Arc<CertifiedKey>> = HashMap::new();
        let mut exact: HashMap<String, Arc<CertifiedKey>> = HashMap::new();
        let mut wildcards: HashMap<String, Arc<CertifiedKey>> = HashMap::new();
        let mut fallback: Option<Arc<CertifiedKey>> = None;

        for domain in &config.domains {
            let cert_id = domain
                .cert_id
                .as_deref()
                .unwrap_or(domain.name.as_str());
            let Some(cert_cfg) = config.certificates.get(cert_id) else {
                continue;
            };

            let key_arc = if let Some(k) = cert_cache.get(cert_id) {
                k.clone()
            } else {
                let (certs, key) = Self::load_certificate(cert_cfg)?;
                let signing_key = any_supported_type(&key).context("Unsupported certificate key type")?;
                let certified = Arc::new(CertifiedKey::new(certs, signing_key));
                cert_cache.insert(cert_id.to_string(), certified.clone());
                certified
            };

            if fallback.is_none() {
                fallback = Some(key_arc.clone());
            }

            if domain.name.starts_with("*.") {
                wildcards.entry(domain.name[2..].to_string()).or_insert(key_arc);
            } else {
                exact.entry(domain.name.clone()).or_insert(key_arc);
            }
        }

        if fallback.is_none() {
            for (sni, cert_cfg) in &config.certificates {
                let cert_id = sni.as_str();
                let key_arc = if let Some(k) = cert_cache.get(cert_id) {
                    k.clone()
                } else {
                    let (certs, key) = Self::load_certificate(cert_cfg)?;
                    let signing_key = any_supported_type(&key).context("Unsupported certificate key type")?;
                    let certified = Arc::new(CertifiedKey::new(certs, signing_key));
                    cert_cache.insert(cert_id.to_string(), certified.clone());
                    certified
                };

                if fallback.is_none() {
                    fallback = Some(key_arc.clone());
                }

                if sni.starts_with("*.") {
                    wildcards.entry(sni[2..].to_string()).or_insert(key_arc);
                } else {
                    exact.entry(sni.clone()).or_insert(key_arc);
                }
            }
        }

        let fallback = fallback.context("No certificates available")?;

        let resolver = MapResolver {
            exact,
            wildcards,
            fallback,
        };

        let mut tls_config = ServerConfig::builder()
            .with_no_client_auth()
            .with_cert_resolver(Arc::new(resolver));
        tls_config.alpn_protocols = vec![b"h2".to_vec(), b"http/1.1".to_vec()];

        Ok(TlsAcceptor::from(Arc::new(tls_config)))
    }

    fn load_certificate(
        cert_config: &CertificateConfig,
    ) -> Result<(Vec<CertificateDer<'static>>, PrivateKeyDer<'static>)> {
        let cert_pem = cert_config
            .cert_pem
            .as_deref()
            .context("certificate missing cert_pem")?;
        let key_pem = cert_config
            .key_pem
            .as_deref()
            .context("certificate missing key_pem")?;

        // Parse certificate chain
        let mut cert_reader = BufReader::new(cert_pem);
        let certs: Vec<CertificateDer<'static>> = rustls_pemfile::certs(&mut cert_reader)
            .collect::<Result<Vec<_>, _>>()
            .context("Failed to parse certificate")?;

        // Parse private key
        let mut key_reader = BufReader::new(key_pem);
        let key = rustls_pemfile::private_key(&mut key_reader)
            .context("Failed to parse private key")?
            .context("No private key found")?;

        Ok((certs, key))
    }
}
