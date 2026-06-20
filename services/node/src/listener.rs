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
use rustls::crypto::ring::sign::any_supported_type;
use rustls::pki_types::{CertificateDer, PrivateKeyDer};
use rustls::server::{ClientHello, ResolvesServerCert};
use rustls::sign::CertifiedKey;
use rustls::ServerConfig;
use std::collections::{HashMap, HashSet};
use std::io::BufReader;
use std::net::SocketAddr;
use std::sync::Arc;
use tokio::net::TcpListener;
use tokio::sync::broadcast;
use tokio::sync::RwLock;
use tokio::sync::Semaphore;
use tokio::task::JoinSet;
use tokio::time::{timeout, Duration};
use tokio_rustls::TlsAcceptor;
use tracing::{debug, error, info, warn};

use crate::cert_store::CertStore;
use crate::config::{CertificateConfig, ConfigHolder};
use crate::http_types::{ClientScheme, LocalAddr, NodeResponse};
use crate::metrics::Metrics;
use crate::proxy::ProxyService;

#[derive(Debug)]
struct MapResolver {
    exact: HashMap<String, Arc<CertifiedKey>>,
    wildcards: HashMap<String, Arc<CertifiedKey>>,
    // Optional: when no https_enabled domain contributes a cert we
    // leave this None so no-SNI clients get a clean handshake failure
    // instead of being handed a stranger's certificate.
    fallback: Option<Arc<CertifiedKey>>,
}

impl ResolvesServerCert for MapResolver {
    fn resolve(&self, client_hello: ClientHello) -> Option<Arc<CertifiedKey>> {
        let sni = client_hello.server_name();
        let Some(host) = sni else {
            return self.fallback.clone();
        };

        if let Some(key) = self.exact.get(host) {
            return Some(key.clone());
        }

        // Wildcard match: RFC 6125 §6.4.3 — a `*.example.com` certificate
        // covers exactly one DNS label, not arbitrary depth. The previous
        // suffix walk also returned `*.example.com` for `a.b.example.com`,
        // which is broader than what mainstream TLS clients accept and
        // would produce confusing "ERR_CERT_COMMON_NAME_INVALID" results
        // at the browser instead of a clean SNI-miss handshake failure.
        if let Some(dot_pos) = host.find('.') {
            let suffix = &host[dot_pos + 1..];
            if let Some(key) = self.wildcards.get(suffix) {
                return Some(key.clone());
            }
        }

        None
    }
}

struct CertIdResolver {
    exact: HashMap<String, String>,
    wildcards: HashMap<String, String>,
    // Same rationale as MapResolver.fallback: absent when no https-
    // enabled domain has a usable cert, which yields a clean handshake
    // failure for no-SNI clients instead of exposing an unrelated cert.
    fallback: Option<Arc<CertifiedKey>>,
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
            None => {
                // no-SNI clients only see a cert if one was explicitly
                // elected as the fallback — i.e. belongs to a domain
                // with effective_https_enabled(). Domains with HTTPS
                // turned off never contribute a fallback, so they
                // cannot be exposed via no-SNI by accident.
                return self.fallback.clone();
            }
        };

        let mut cert_id = self.exact.get(host).map(|s| s.as_str());
        if cert_id.is_none() {
            // Wildcard match is restricted to exactly one sub-label
            // (RFC 6125 §6.4.3). See MapResolver::resolve for the
            // rationale. Going deeper would let a `*.example.com`
            // certificate be presented for `a.b.example.com`, which
            // every mainstream TLS client rejects anyway.
            if let Some(dot_pos) = host.find('.') {
                let suffix = &host[dot_pos + 1..];
                if let Some(id) = self.wildcards.get(suffix) {
                    cert_id = Some(id.as_str());
                }
            }
        }

        if let Some(id) = cert_id {
            match self.cert_store.get_certified_key(id) {
                Ok(Some(key)) => return Some(key),
                Ok(None) => warn!(cert_id = id, sni = host, "Certificate not found in store"),
                Err(e) => {
                    warn!(cert_id = id, sni = host, error = %e, "Failed to load certificate from store");
                }
            }
        }

        None
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

    pub async fn start(self, shutdown: broadcast::Receiver<()>) -> Result<()> {
        // Wrap in Arc so each spawned accept loop (primary + one per
        // extra domain.listen_port) can hold its own reference without
        // moving fields out of self.
        Arc::new(self).run(shutdown).await
    }

    async fn run(self: Arc<Self>, mut shutdown: broadcast::Receiver<()>) -> Result<()> {
        info!("Starting listener (primary={})", self.addr);

        // Bind primary listener up-front so a misconfigured address fails
        // the node boot rather than silently running without a public
        // endpoint. Extra per-domain ports are best-effort and retry.
        let primary_listener = TcpListener::bind(&self.addr)
            .await
            .with_context(|| format!("Failed to bind primary listener on {}", &self.addr))?;
        info!("Listening on {} (primary)", self.addr);

        let primary_port = parse_port_from_addr(&self.addr);

        let mut active_ports: HashSet<u16> = HashSet::new();
        if let Some(p) = primary_port {
            active_ports.insert(p);
        }

        // Pool of running accept-loop tasks. Loops exit via the shared
        // shutdown broadcast — we never stop a loop for a removed port
        // (that would reset live connections), so active_ports grows
        // monotonically across the process lifetime.
        let mut accept_tasks: JoinSet<()> = JoinSet::new();
        {
            let inner = self.clone();
            let sd = shutdown.resubscribe();
            let label = format!("{} (primary)", self.addr);
            accept_tasks.spawn(async move {
                if let Err(e) = inner.run_accept_loop(primary_listener, sd).await {
                    error!(label = %label, error = %e, "accept loop exited with error");
                }
            });
        }

        // Apply initial config if the control plane already delivered
        // one before the listener started (startup ordering race).
        self.sync_extra_ports(&mut active_ports, &mut accept_tasks, &shutdown)
            .await;

        let mut change_rx = self.config_holder.subscribe();
        // 30s retry cadence picks up ports whose bind failed earlier
        // (e.g. the port was briefly held by a previous process during
        // a rolling restart). Idempotent against already-bound ports.
        let mut retry_ticker = tokio::time::interval(Duration::from_secs(30));
        retry_ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
        // First tick fires immediately; consume it so we only retry
        // after a real interval or an explicit config change.
        retry_ticker.tick().await;

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    info!("Listener shutdown requested; stopping coordinator");
                    break;
                }
                r = change_rx.changed() => {
                    if r.is_err() {
                        // Holder dropped — nothing more to observe.
                        debug!("config holder watch channel closed");
                    } else {
                        self.sync_extra_ports(&mut active_ports, &mut accept_tasks, &shutdown).await;
                    }
                }
                _ = retry_ticker.tick() => {
                    self.sync_extra_ports(&mut active_ports, &mut accept_tasks, &shutdown).await;
                }
            }
        }

        info!("Waiting for {} accept loops to drain", accept_tasks.len());
        let drain = async {
            while let Some(join_result) = accept_tasks.join_next().await {
                if let Err(e) = join_result {
                    error!("Accept loop join error: {}", e);
                }
            }
        };
        if timeout(Duration::from_secs(15), drain).await.is_err() {
            warn!("Timed out waiting for accept loops to stop");
        }

        Ok(())
    }

    /// Drive a single bound TcpListener's accept loop. Runs until the
    /// shutdown broadcast fires, then drains in-flight connection tasks.
    async fn run_accept_loop(
        self: Arc<Self>,
        listener: TcpListener,
        mut shutdown: broadcast::Receiver<()>,
    ) -> Result<()> {
        let mut connections = JoinSet::new();

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    info!("Accept loop shutdown requested");
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
                    let local_addr = stream.local_addr().ok();

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
                                local_addr,
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
                            Self::handle_plain_connection(stream, peer_addr, local_addr, proxy_service, config_holder, metrics.clone(), cache_dir.clone(), max_connections).await
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

        info!(
            "Waiting for {} in-flight connections to drain",
            connections.len()
        );
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

    /// Bind any port referenced by a domain.listen_port that is not yet
    /// tracked in `active_ports`. Never closes stale listeners (see run()
    /// comment). Bind failures log-and-retry on the next tick.
    async fn sync_extra_ports(
        self: &Arc<Self>,
        active_ports: &mut HashSet<u16>,
        accept_tasks: &mut JoinSet<()>,
        shutdown: &broadcast::Receiver<()>,
    ) {
        let Some(cfg) = self.config_holder.get() else {
            return;
        };

        let mut needed: HashSet<u16> = HashSet::new();
        for d in &cfg.domains {
            if let Some(p) = d.effective_listen_port() {
                needed.insert(p);
            }
        }

        // Derive missing ports without mutating active_ports up front —
        // we only insert after a successful bind.
        let missing: Vec<u16> = needed
            .into_iter()
            .filter(|p| !active_ports.contains(p))
            .collect();

        for port in missing {
            // Bind 0.0.0.0:port (v4) — mirrors how default listen_addr is
            // configured. If the operator needs IPv6 or a specific
            // interface they must set listen_addr manually; per-domain
            // listen_port only controls the port number.
            let addr = format!("0.0.0.0:{}", port);
            match TcpListener::bind(&addr).await {
                Ok(listener) => {
                    info!(port, %addr, "bound extra listener for domain.listen_port");
                    active_ports.insert(port);
                    let me = self.clone();
                    let sd = shutdown.resubscribe();
                    accept_tasks.spawn(async move {
                        if let Err(e) = me.run_accept_loop(listener, sd).await {
                            error!(port, error = %e, "extra accept loop failed");
                        }
                    });
                }
                Err(e) => {
                    warn!(port, %addr, error = %e, "failed to bind extra listener (will retry)");
                }
            }
        }
    }

    async fn handle_plain_connection(
        stream: tokio::net::TcpStream,
        peer_addr: std::net::SocketAddr,
        local_addr: Option<SocketAddr>,
        proxy_service: Arc<ProxyService>,
        config_holder: Arc<ConfigHolder>,
        metrics: Arc<Metrics>,
        cache_dir: Option<String>,
        max_connections: usize,
    ) -> Result<()> {
        let io = TokioIo::new(stream);

        let service = service_fn(move |mut req: Request<Incoming>| {
            req.extensions_mut().insert(peer_addr);
            if let Some(addr) = local_addr {
                req.extensions_mut().insert(LocalAddr(addr));
            }
            req.extensions_mut().insert(ClientScheme("http"));
            let proxy_service = proxy_service.clone();
            let metrics = metrics.clone();
            let config_holder = config_holder.clone();
            let cache_dir = cache_dir.clone();
            async move {
                Self::handle_request(
                    req,
                    proxy_service,
                    config_holder,
                    metrics,
                    cache_dir,
                    max_connections,
                )
                .await
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
        local_addr: Option<SocketAddr>,
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
        let tls_acceptor = Self::get_or_build_tls_acceptor(
            &config_holder,
            &tls_acceptor_cache,
            cert_store.as_ref(),
        )
        .await?;

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
            if let Some(addr) = local_addr {
                req.extensions_mut().insert(LocalAddr(addr));
            }
            req.extensions_mut().insert(ClientScheme("https"));
            let proxy_service = proxy_service.clone();
            let metrics = metrics.clone();
            let config_holder = config_holder.clone();
            let cache_dir = cache_dir.clone();
            async move {
                Self::handle_request(
                    req,
                    proxy_service,
                    config_holder,
                    metrics,
                    cache_dir,
                    max_connections,
                )
                .await
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

        // Build outside any lock. PEM parsing + rustls ServerConfig
        // assembly can take tens of milliseconds for fleets with many
        // certs; doing it while holding the write lock stalled every
        // concurrent handshake on that config generation. Two tasks
        // racing on the same new `key` will both build and the loser's
        // acceptor will simply be dropped — wasted CPU but not a
        // correctness issue.
        let acceptor = Self::build_tls_acceptor_from_config(&config, cert_store)?;

        let mut guard = cache.write().await;
        // Re-check after acquiring the write lock (another task could have refreshed it).
        if let Some(ref cached) = *guard {
            if cached.key == key {
                return Ok(cached.acceptor.clone());
            }
        }
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
            return Ok(Self::plain_response(
                StatusCode::NOT_FOUND,
                "not found",
                "text/plain; charset=utf-8",
            ));
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
                if !domain.effective_https_enabled() {
                    continue;
                }
                let cert_id = domain.cert_id.as_deref().unwrap_or(domain.name.as_str());

                if fallback_cert_id.is_none() && !cert_id.is_empty() {
                    fallback_cert_id = Some(cert_id.to_string());
                }

                if domain.name.starts_with("*.") {
                    wildcards
                        .entry(domain.name[2..].to_string())
                        .or_insert(cert_id.to_string());
                } else {
                    exact
                        .entry(domain.name.clone())
                        .or_insert(cert_id.to_string());
                }
            }

            // Deliberately NOT falling back to config.certificates.keys()
            // here. The previous code would grab "any certificate" —
            // including certs owned by domains with https_enabled=false —
            // and hand it to no-SNI clients, undermining the
            // effective_https_enabled() gate the loop above enforces.
            // When no https-enabled domain contributes a fallback, we
            // leave fallback=None and no-SNI handshakes fail cleanly.
            let fallback = match fallback_cert_id {
                Some(id) => store.get_certified_key(&id)?,
                None => None,
            };

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
            if !domain.effective_https_enabled() {
                continue;
            }
            let cert_id = domain.cert_id.as_deref().unwrap_or(domain.name.as_str());
            let Some(cert_cfg) = config.certificates.get(cert_id) else {
                continue;
            };

            let key_arc = if let Some(k) = cert_cache.get(cert_id) {
                k.clone()
            } else {
                let (certs, key) = Self::load_certificate(cert_cfg)?;
                let signing_key =
                    any_supported_type(&key).context("Unsupported certificate key type")?;
                let certified = Arc::new(CertifiedKey::new(certs, signing_key));
                cert_cache.insert(cert_id.to_string(), certified.clone());
                certified
            };

            if fallback.is_none() {
                fallback = Some(key_arc.clone());
            }

            if domain.name.starts_with("*.") {
                wildcards
                    .entry(domain.name[2..].to_string())
                    .or_insert(key_arc);
            } else {
                exact.entry(domain.name.clone()).or_insert(key_arc);
            }
        }

        // Mirror the disk-backed branch: fallback is derived ONLY from
        // https-enabled domains. Removed the old catch-all over
        // config.certificates, which could expose certs whose owning
        // domain has HTTPS turned off.
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

/// Extract the port portion of a socket address string like
/// "0.0.0.0:80", "[::]:443", or "127.0.0.1:8080". Returns None for
/// malformed inputs — callers treat that as "unknown primary port", which
/// simply means sync_extra_ports will try to bind the configured
/// listen_port even if it equals the primary. The bind fails with
/// EADDRINUSE, which is logged and harmless.
fn parse_port_from_addr(addr: &str) -> Option<u16> {
    addr.rsplit_once(':')
        .and_then(|(_, port)| port.trim().parse::<u16>().ok())
}

#[cfg(test)]
mod tests {
    use super::parse_port_from_addr;

    #[test]
    fn parses_ipv4_port() {
        assert_eq!(parse_port_from_addr("0.0.0.0:80"), Some(80));
        assert_eq!(parse_port_from_addr("127.0.0.1:8080"), Some(8080));
    }

    #[test]
    fn parses_ipv6_port() {
        assert_eq!(parse_port_from_addr("[::]:443"), Some(443));
    }

    #[test]
    fn rejects_missing_port() {
        assert_eq!(parse_port_from_addr("0.0.0.0"), None);
        assert_eq!(parse_port_from_addr(""), None);
        assert_eq!(parse_port_from_addr("0.0.0.0:abc"), None);
    }
}
