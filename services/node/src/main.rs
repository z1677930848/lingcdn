mod proto;
mod config;
mod grpc_client;
mod access_log;
mod cache;
mod autotune;
mod geoip;
mod geoip_holder;
mod geoip_updater;
mod router;
mod proxy;
mod metrics;
mod telemetry;
mod listener;
mod ops_listener;
mod http_types;
mod limited_body;
mod purge;
mod preload;
mod config_agent;
mod certificate_manager;
mod cert_store;
mod upgrade;
mod captcha;
mod xdp;
mod log_reporter;
mod node_state;
mod origin_health;
mod stream_listener;
mod stream_proxy;
mod edge_enhance;
mod edge_script;
mod l2_fetch;
mod media;
mod stream_health;

use anyhow::{Context, Result};
use std::path::{Path, PathBuf};
use tokio::net::lookup_host;
use url::Url;
use std::sync::Arc;
use tokio::signal;
use tokio::fs;
use tokio::sync::broadcast;
use tokio::time::{sleep, timeout, Duration};
use tracing::{error, info, warn};
use serde::Deserialize;
use upgrade::{UpgradeRunner, UpgradeTrigger};

use crate::cache::Cache;
use crate::config::{load_node_config, ConfigHolder, NodeConfig};
use crate::config_agent::ConfigAgent;
use crate::grpc_client::GrpcClient;
use crate::listener::Listener;
use crate::ops_listener::OpsListener;
use crate::stream_listener::StreamListener;
use crate::stream_health::StreamHealthChecker;
use crate::metrics::Metrics;
use crate::purge::PurgeAgent;
use crate::preload::PreloadAgent;
use crate::proxy::{ProxyService, BanEvent};
use crate::telemetry::TelemetryReporter;
use crate::certificate_manager::CertificateManager;
use crate::cert_store::CertStore;
use crate::access_log::AccessLogger;
use crate::geoip::GeoIpResolver;
use crate::geoip_holder::GeoIpHolder;
use crate::geoip_updater::GeoIpUpdater;
use crate::log_reporter::LogReporterLayer;

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize tracing with the local error-log appender layer.
    // The Layer goes live immediately; the writer task that drains its
    // mpsc into /var/log/lingcdn/error.log is spawned later, once node
    // config and metrics are available. WARN/ERROR events emitted in
    // between sit in the bounded buffer.
    let (log_reporter_layer, log_reporter_handle) = LogReporterLayer::new();

    use tracing_subscriber::layer::SubscriberExt;
    use tracing_subscriber::util::SubscriberInitExt;

    let env_filter = tracing_subscriber::EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info"));

    tracing_subscriber::registry()
        .with(env_filter)
        .with(tracing_subscriber::fmt::layer())
        .with(log_reporter_layer)
        .init();

    info!("Starting LingCDN Node v{}", env!("CARGO_PKG_VERSION"));

    // Load node configuration
    let node_config = load_node_config()?;
    info!("Node configuration loaded: hostname={}", node_config.hostname);
    log_reporter::set_hostname(node_config.hostname.clone());

    if crate::autotune::enabled() {
        let p = crate::autotune::profile();
        if p.total_mem_bytes > 0 {
            info!(
                "Autotune enabled: total_mem_mb={} cpu={}",
                p.total_mem_bytes / (1024 * 1024),
                p.cpu_count
            );
        } else {
            info!("Autotune enabled: total_mem unknown");
        }
    }

    // Pre-flight checks
    run_startup_checks(&node_config).await?;

    // Shared shutdown signal for all background tasks
    let (shutdown_tx, _) = broadcast::channel(8);

    // Initialize metrics
    let metrics = Arc::new(Metrics::new()?);
    info!("Metrics initialized");

    // Initialize cache
    let cache_path: Option<PathBuf> = node_config
        .cache_dir
        .as_ref()
        .map(|p| PathBuf::from(p));
    let cache = Arc::new(Cache::new(
        node_config.memory_cache_capacity,
        cache_path.as_deref(),
    )?);
    info!("Cache initialized");

    // Certificate storage (disk) + small in-memory LRU for SNI.
    // This keeps memory stable even with thousands of certs/domains.
    let cert_cache_capacity = std::env::var("TLS_CERT_CACHE_CAPACITY")
        .ok()
        .and_then(|v| v.trim().parse::<usize>().ok())
        .unwrap_or_else(|| crate::autotune::default_tls_cert_cache_capacity());
    let cert_store: Option<Arc<CertStore>> = cache_path.as_deref().and_then(|root| {
        match CertStore::new(root, cert_cache_capacity) {
            Ok(s) => {
                info!("Cert store initialized at {}", root.join("certs").display());
                Some(Arc::new(s))
            }
            Err(err) => {
                error!("Cert store disabled (init failed at {}): {}", root.join("certs").display(), err);
                None
            }
        }
    });

    // Initialize config holder
    let config_holder = Arc::new(ConfigHolder::new());

    let ops_listener = OpsListener::new(
        node_config.ops_listen_addr.clone(),
        node_config.ops_token.clone(),
        metrics.clone(),
        config_holder.clone(),
        node_config.cache_dir.clone(),
    );
    let ops_listener_handle = tokio::spawn({
        let shutdown = shutdown_tx.subscribe();
        async move {
            if let Err(e) = ops_listener.start(shutdown).await {
                error!("Ops listener error: {}", e);
            }
        }
    });

    if let Some(cache_path) = cache_path.clone() {
        let max_bytes = node_config.disk_cache_max_bytes;
        let interval_secs = node_config.disk_cache_gc_interval_seconds;
        let recreate = node_config.disk_cache_recreate_on_exceed;
        if max_bytes > 0 && interval_secs > 0 {
            let cache_for_gc = cache.clone();
            let mut shutdown = shutdown_tx.subscribe();
            tokio::spawn(async move {
                let mut ticker = tokio::time::interval(Duration::from_secs(interval_secs));
                loop {
                    tokio::select! {
                        _ = shutdown.recv() => break,
                        _ = ticker.tick() => {
                            let path = cache_path.clone();
                            let size_res = tokio::task::spawn_blocking(move || dir_size(&path)).await;
                            let size = match size_res {
                                Ok(Ok(v)) => v,
                                Ok(Err(e)) => {
                                    warn!("Disk cache size check failed for path {}: {}", cache_path.display(), e);
                                    continue;
                                }
                                Err(e) => {
                                    warn!("Disk cache size task panicked for path {}: {}", cache_path.display(), e);
                                    continue;
                                }
                            };

                            if size > max_bytes {
                                warn!("Disk cache over limit: size={} max={}", size, max_bytes);
                                cache_for_gc.set_disk_over_limit(true);
                                if recreate {
                                    let cache2 = cache_for_gc.clone();
                                    let path2 = cache_path.clone();
                                    let res = tokio::task::spawn_blocking(move || cache2.recreate_disk_cache(&path2)).await;
                                    match res {
                                        Ok(Ok(())) => {
                                            info!("Disk cache recreated");
                                            cache_for_gc.set_disk_over_limit(false);
                                        }
                                        Ok(Err(e)) => warn!("Failed to recreate disk cache at {}: {}", cache_path.display(), e),
                                        Err(e) => warn!("Failed to recreate disk cache task at {}: {}", cache_path.display(), e),
                                    }
                                } else {
                                    let cache2 = cache_for_gc.clone();
                                    let res = tokio::task::spawn_blocking(move || cache2.clear()).await;
                                    match res {
                                        Ok(Ok(())) => info!("Disk cache cleared"),
                                        Ok(Err(e)) => warn!("Failed to clear disk cache at {}: {}", cache_path.display(), e),
                                        Err(e) => warn!("Failed to clear disk cache task at {}: {}", cache_path.display(), e),
                                    }
                                }
                            } else {
                                cache_for_gc.set_disk_over_limit(false);
                            }
                        }
                    }
                }
            });
        }
    }

    // Access log (JSON lines). Always on — a node-side Filebeat tails
    // the file and ships entries to Elasticsearch.
    let access_logger = AccessLogger::new(
        PathBuf::from(&node_config.access_log_path),
        Some(metrics.access_log_dropped_total.clone()),
    );
    info!("访问日志已启用: {}", node_config.access_log_path);

    // Now that NodeConfig + Metrics are available, drain the
    // log_reporter_handle into the on-disk error.log. Same reasoning as
    // access log: Filebeat reads the file and ships to ES under the
    // cdn-error-* index.
    let error_log_writer_handle = log_reporter::spawn_writer(
        log_reporter_handle,
        PathBuf::from(&node_config.error_log_path),
        Some(metrics.error_log_dropped_total.clone()),
        shutdown_tx.subscribe(),
    );
    info!("错误日志已启用: {}", node_config.error_log_path);

    let geoip_resolver = if let Some(path) = node_config.geoip_db_path.as_deref() {
        let db_path = Path::new(path);
        if !db_path.exists() {
            warn!("GeoIP 数据库不存在，已禁用: {}", db_path.display());
            None
        } else {
            match GeoIpResolver::from_path(db_path) {
                Ok(resolver) => {
                    info!("GeoIP 已启用: {}", db_path.display());
                    Some(Arc::new(resolver))
                }
                Err(err) => {
                    warn!("GeoIP 加载失败，已禁用 {}: {}", db_path.display(), err);
                    None
                }
            }
        }
    } else {
        info!("GeoIP 未启用（GEOIP_DB_PATH 未配置）");
        None
    };
    let geoip_holder = Arc::new(GeoIpHolder::new(geoip_resolver));

    // Connect to control plane
    let base_grpc_client = GrpcClient::connect(node_config.clone())
        .await
        .context("Failed to connect to control plane")?;
    let mut grpc_client = base_grpc_client.clone();

    // Prefer the persisted per-node token over the bootstrap token. The
    // control plane's authorizeNodeRegistration first tries to match the
    // credential against any existing node record (by hostname) using
    // sha256(credential) == stored_hash — so re-submitting the last
    // returned nodeToken lets a restarted node re-register without
    // consuming another bootstrap token (and keeps working even after
    // the original bootstrap token expires or is revoked).
    let persisted_state = match node_state::load() {
        Ok(s) => s,
        Err(e) => {
            warn!("Failed to read persisted node state, will fall back to BOOTSTRAP_TOKEN: {}", e);
            None
        }
    };
    if let Some(ref st) = persisted_state {
        info!(
            "Using persisted node identity for re-registration: node_id={}",
            st.node_id
        );
        grpc_client.set_bootstrap_credential(st.node_token.clone());
    }

    // Register node
    let register_response = grpc_client
        .register_node()
        .await
        .context("Failed to register node")?;

    info!("Node registered: node_id={}", register_response.node_id);
    log_reporter::set_node_id(register_response.node_id.clone());

    // Persist the freshly issued (node_id, node_token) so the next restart
    // can re-register via the per-node credential path. Non-fatal on error:
    // a failure here only degrades future restart behaviour, not the
    // currently running session.
    if let Err(e) = node_state::save(&node_state::NodeState {
        node_id: register_response.node_id.clone(),
        node_token: register_response.token.clone(),
    }) {
        warn!(
            "Failed to persist node state to {}: {}",
            node_state::state_path().display(),
            e
        );
    }

    // Update node config with assigned ID
    let mut node_config = node_config;
    node_config.node_id = Some(register_response.node_id.clone());

    // Start config agent
    let config_holder_for_agent = config_holder.clone();
    let grpc_client_for_config = base_grpc_client.clone();
    let node_id_for_config = register_response.node_id.clone();
    let token_for_config = register_response.token.clone();
    let tls_enabled_for_config = node_config.tls_enabled;
    let cert_store_for_config = cert_store.clone();
    let mut config_shutdown = shutdown_tx.subscribe();
    let config_handle = tokio::spawn(async move {
        let mut backoff_secs = 5u64;
        loop {
            let config_agent = ConfigAgent::new(
                config_holder_for_agent.clone(),
                tls_enabled_for_config,
                cert_store_for_config.clone(),
            );

            match config_agent
                .start(
                    grpc_client_for_config.clone(),
                    node_id_for_config.clone(),
                    token_for_config.clone(),
                    config_shutdown.resubscribe(),
                )
                .await
            {
                Ok(_) => {
                    info!("Config agent exited gracefully");
                    break;
                }
                Err(e) => {
                    error!("Config agent error: {}, retrying in {}s", e, backoff_secs);
                    tokio::select! {
                        _ = sleep(Duration::from_secs(backoff_secs)) => {}
                        _ = config_shutdown.recv() => {
                            info!("Config agent shutdown requested during backoff");
                            break;
                        }
                    }
                    backoff_secs = (backoff_secs.saturating_mul(2)).min(60);
                }
            }
        }
    });

    let cache_for_purge = cache.clone();
    let grpc_client_for_purge = base_grpc_client.clone();
    let node_id_for_purge = register_response.node_id.clone();
    let token_for_purge = register_response.token.clone();
    let mut purge_shutdown = shutdown_tx.subscribe();
    let _purge_handle = tokio::spawn(async move {
        let mut backoff_secs = 5u64;
        loop {
            let agent = PurgeAgent::new(cache_for_purge.clone());
            match agent
                .start(
                    grpc_client_for_purge.clone(),
                    node_id_for_purge.clone(),
                    token_for_purge.clone(),
                    purge_shutdown.resubscribe(),
                )
                .await
            {
                Ok(_) => {
                    break;
                }
                Err(e) => {
                    error!("Purge agent error: {:#}, retrying in {}s", e, backoff_secs);
                    tokio::select! {
                        _ = sleep(Duration::from_secs(backoff_secs)) => {}
                        _ = purge_shutdown.recv() => {
                            break;
                        }
                    }
                    backoff_secs = (backoff_secs.saturating_mul(2)).min(60);
                }
            }
        }
    });

    // Start upgrade watcher (pull from portal/upgrade API if配置了 upgrade_endpoint)
    let mut upgrade_handle_opt = None;
    let mut upgrade_cmd_tx_opt: Option<tokio::sync::mpsc::Sender<UpgradeTrigger>> = None;
    if let Some(endpoint) = node_config.upgrade_endpoint.clone() {
        let (cmd_tx, cmd_rx) = tokio::sync::mpsc::channel::<UpgradeTrigger>(8);
        upgrade_cmd_tx_opt = Some(cmd_tx.clone());
        let runner = UpgradeRunner::new(endpoint, node_config.clone());
        let shutdown = shutdown_tx.subscribe();
        upgrade_handle_opt = Some(tokio::spawn(async move {
            runner.run(shutdown, cmd_rx).await;
        }));
    } else {
        info!("Upgrade watcher disabled (no upgrade_endpoint configured)");
    }

    let geoip_updater_handle_opt = if let Some(updater) = GeoIpUpdater::new(&node_config, geoip_holder.clone()) {
        let shutdown = shutdown_tx.subscribe();
        Some(tokio::spawn(async move {
            updater.run(shutdown).await;
        }))
    } else {
        info!("GeoIP auto update disabled (missing GEOIP_DB_PATH or GEOIP_UPDATE_ENDPOINT)");
        None
    };

    // Wait for initial config
    info!("Waiting for initial configuration...");
    for _ in 0..30 {
        if config_holder.get().is_some() {
            break;
        }
        sleep(Duration::from_secs(1)).await;
    }

    let runtime_config = config_holder
        .get()
        .context("No configuration received from control plane")?;

    info!("Initial configuration received: version={}", runtime_config.version);

    // 如果启用 TLS 且未配置证书，尝试向控制面申请证书
    if node_config.tls_enabled && cert_store.is_none() && runtime_config.certificates.is_empty() {
        if let Some(ref node_id) = node_config.node_id {
            let mut cert_manager = CertificateManager::new(base_grpc_client.clone());
            // `token` is required by control plane's RequestCertificate
            // (node_control.go::validateNodeToken). Previously this call
            // path was dead because the token was hard-coded to "".
            match cert_manager
                .request_certificates(node_id, &register_response.token, &runtime_config.domains)
                .await
            {
                Ok(new_certs) => {
                    if new_certs.is_empty() {
                        warn!("未获取到新的证书，TLS 可能不可用");
                    } else {
                        let mut merged = runtime_config.as_ref().clone();
                        merged.certificates.extend(new_certs);
                        config_holder.update(merged);
                        info!("已申请并加载证书（本地配置已更新）");
                    }
                }
                Err(e) => warn!("证书申请失败: {}", e),
            }
        } else {
            warn!("未获得 node_id，无法申请证书");
        }
    }

    // 创建拉黑事件通道
    let (ban_event_tx, ban_event_rx) = tokio::sync::mpsc::unbounded_channel();

    // Initialize XDP controller (Linux only)
    #[cfg(target_os = "linux")]
    let xdp_controller = if node_config.xdp_enabled {
        if let Some(ref interface) = node_config.xdp_interface {
            let xdp_config = xdp::XdpConfig {
                rate_limit_enabled: if node_config.xdp_rate_limit_enabled { 1 } else { 0 },
                syn_flood_enabled: if node_config.xdp_syn_flood_enabled { 1 } else { 0 },
                rate_limit_pps: node_config.xdp_rate_limit_pps,
                syn_limit_pps: node_config.xdp_syn_limit_pps,
                window_ns: 1_000_000_000, // 1 second window
            };
            let mut controller = xdp::XdpController::with_config(interface, xdp_config);
            match controller.load_and_attach() {
                Ok(()) => {
                    info!("XDP program loaded on interface: {}", interface);
                    // Sync initial blacklist/whitelist from config
                    if let Some(ref config) = config_holder.get() {
                        if let Err(e) = controller.sync_blacklist(&config.waf_bans).await {
                            warn!("Failed to sync XDP blacklist: {}", e);
                        }
                        if let Err(e) = controller.sync_whitelist(&config.waf_whitelist).await {
                            warn!("Failed to sync XDP whitelist: {}", e);
                        }
                    }
                    Some(Arc::new(tokio::sync::RwLock::new(controller)))
                }
                Err(e) => {
                    error!("Failed to load XDP program: {}", e);
                    None
                }
            }
        } else {
            warn!("XDP enabled but no interface specified (XDP_INTERFACE)");
            None
        }
    } else {
        info!("XDP disabled");
        None
    };

    #[cfg(not(target_os = "linux"))]
    let xdp_controller: Option<Arc<tokio::sync::RwLock<xdp::XdpController>>> = {
        if node_config.xdp_enabled {
            warn!("XDP is only supported on Linux");
        }
        None
    };

    // Origin health checker. Always constructed so the proxy has a
    // single object to ask "is this origin healthy?", but the
    // background probe task only does work for domains that opt in
    // via origin_health_check.enabled.
    let origin_health = Arc::new(crate::origin_health::OriginHealthChecker::new(config_holder.clone()));
    let origin_health_handle = tokio::spawn({
        let checker = origin_health.clone();
        let shutdown = shutdown_tx.subscribe();
        async move {
            checker.run(shutdown).await;
        }
    });

    // Initialize proxy service
    let proxy_service = Arc::new(ProxyService::new(
        cache.clone(),
        config_holder.clone(),
        metrics.clone(),
        Some(access_logger),
        geoip_holder.clone(),
        node_config.node_id.clone(),
        node_config.hostname.clone(),
        node_config.max_request_body_bytes,
        node_config.max_response_body_bytes,
        node_config.max_cache_object_bytes,
        Some(ban_event_tx),
        Some(origin_health.clone()),
    ));

    // Start ban event reporter task (实时上报拉黑IP到主控)
    let ban_reporter_handle = tokio::spawn({
        let grpc_client_for_ban = base_grpc_client.clone();
        let node_id_for_ban = register_response.node_id.clone();
        let token_for_ban = register_response.token.clone();
        let shutdown = shutdown_tx.subscribe();
        let xdp_for_ban = xdp_controller.clone();
        async move {
            start_ban_reporter(
                grpc_client_for_ban,
                node_id_for_ban,
                token_for_ban,
                ban_event_rx,
                shutdown,
                xdp_for_ban,
            ).await;
        }
    });

    // Start telemetry reporter
    // 10s cadence gives the control plane near-real-time CPU/memory
    // readings (previously hardcoded to 60s, so the admin dashboard saw
    // a stale snapshot at best and zeros if the CPU sample interval bug
    // in telemetry.rs::system_snapshot was active).
    let telemetry_reporter = TelemetryReporter::new(metrics.clone(), 10);
    let telemetry_handle = tokio::spawn({
        let grpc_client_for_telemetry = base_grpc_client.clone();
        let node_id_for_telemetry = register_response.node_id.clone();
        let token_for_telemetry = register_response.token.clone();
        let xdp_for_telemetry = xdp_controller.clone();
        let shutdown = shutdown_tx.subscribe();
        async move {
            telemetry_reporter.start(grpc_client_for_telemetry, node_id_for_telemetry, token_for_telemetry, xdp_for_telemetry, shutdown).await;
        }
    });

    // Start heartbeat task
    let node_id = register_response.node_id.clone();
    let token = register_response.token.clone();
    let heartbeat_handle = tokio::spawn({
        let grpc_client_for_heartbeat = base_grpc_client.clone();
        let metrics_for_heartbeat = metrics.clone();
        let shutdown_tx_for_heartbeat = shutdown_tx.clone();
        let shutdown = shutdown_tx.subscribe();
        let upgrade_cmd_tx = upgrade_cmd_tx_opt.clone();
        let proxy_for_heartbeat = proxy_service.clone();
        async move {
            start_heartbeat(
                grpc_client_for_heartbeat,
                node_id,
                token,
                metrics_for_heartbeat,
                shutdown_tx_for_heartbeat,
                shutdown,
                upgrade_cmd_tx,
                proxy_for_heartbeat,
            ).await;
        }
    });

    let proxy_for_preload = proxy_service.clone();
    let grpc_client_for_preload = base_grpc_client.clone();
    let node_id_for_preload = register_response.node_id.clone();
    let token_for_preload = register_response.token.clone();
    let mut preload_shutdown = shutdown_tx.subscribe();
    let _preload_handle = tokio::spawn(async move {
        let mut backoff_secs = 5u64;
        loop {
            let agent = PreloadAgent::new(proxy_for_preload.clone());
            match agent
                .start(
                    grpc_client_for_preload.clone(),
                    node_id_for_preload.clone(),
                    token_for_preload.clone(),
                    preload_shutdown.resubscribe(),
                )
                .await
            {
                Ok(_) => break,
                Err(e) => {
                    error!("Preload agent error: {:#}, retrying in {}s", e, backoff_secs);
                    tokio::select! {
                        _ = sleep(Duration::from_secs(backoff_secs)) => {}
                        _ = preload_shutdown.recv() => break,
                    }
                    backoff_secs = (backoff_secs.saturating_mul(2)).min(60);
                }
            }
        }
    });

    // Start listener
    let listener = Listener::new(
        node_config.listen_addr.clone(),
        node_config.tls_enabled,
        config_holder.clone(),
        proxy_service,
        metrics.clone(),
        node_config.max_connections,
        node_config.cache_dir.clone(),
        cert_store.clone(),
    );

    info!("Starting HTTP listener on {}", node_config.listen_addr);

    // Run listener in background
    let listener_handle = tokio::spawn({
        let shutdown = shutdown_tx.subscribe();
        async move {
            if let Err(e) = listener.start(shutdown).await {
                error!("Listener error: {}", e);
            }
        }
    });

    // L4 stream health checker + forwarding listener
    let stream_health = Arc::new(StreamHealthChecker::new(config_holder.clone()));
    let _stream_health_handle = tokio::spawn({
        let checker = stream_health.clone();
        let shutdown = shutdown_tx.subscribe();
        async move {
            checker.run(shutdown).await;
        }
    });

    let stream_listener = Arc::new(StreamListener::new(config_holder.clone(), stream_health.clone()));
    let stream_listener_handle = tokio::spawn({
        let shutdown = shutdown_tx.subscribe();
        async move {
            if let Err(e) = stream_listener.start(shutdown).await {
                error!("Stream listener error: {}", e);
            }
        }
    });

    // Wait for shutdown signal
    signal::ctrl_c().await?;
    info!("Shutdown signal received");

    // Notify all tasks to shut down
    let _ = shutdown_tx.send(());

    let shutdown_timeout = Duration::from_secs(20);

    match timeout(shutdown_timeout, listener_handle).await {
        Ok(join_result) => match join_result {
            Ok(()) => {}
            Err(e) => warn!("Listener task join error: {}", e),
        },
        Err(_) => warn!("Timeout waiting for listener to shut down"),
    }

    match timeout(shutdown_timeout, stream_listener_handle).await {
        Ok(join_result) => match join_result {
            Ok(()) => {}
            Err(e) => warn!("Stream listener task join error: {}", e),
        },
        Err(_) => warn!("Timeout waiting for stream listener to shut down"),
    }

    match timeout(shutdown_timeout, ops_listener_handle).await {
        Ok(join_result) => match join_result {
            Ok(()) => {}
            Err(e) => warn!("Ops listener task join error: {}", e),
        },
        Err(_) => warn!("Timeout waiting for ops listener to shut down"),
    }

    match timeout(shutdown_timeout, telemetry_handle).await {
        Ok(join_result) => {
            if let Err(e) = join_result {
                warn!("Telemetry reporter join error: {}", e);
            }
        }
        Err(_) => warn!("Timeout waiting for telemetry reporter to shut down"),
    }

    match timeout(shutdown_timeout, heartbeat_handle).await {
        Ok(join_result) => {
            if let Err(e) = join_result {
                warn!("Heartbeat task join error: {}", e);
            }
        }
        Err(_) => warn!("Timeout waiting for heartbeat task to shut down"),
    }

    match timeout(shutdown_timeout, ban_reporter_handle).await {
        Ok(join_result) => {
            if let Err(e) = join_result {
                warn!("Ban reporter task join error: {}", e);
            }
        }
        Err(_) => warn!("Timeout waiting for ban reporter task to shut down"),
    }

    match timeout(shutdown_timeout, error_log_writer_handle).await {
        Ok(join_result) => {
            if let Err(e) = join_result {
                warn!("Error log writer join error: {}", e);
            }
        }
        Err(_) => warn!("Timeout waiting for error log writer to shut down"),
    }

    match timeout(shutdown_timeout, origin_health_handle).await {
        Ok(join_result) => {
            if let Err(e) = join_result {
                warn!("Origin health checker join error: {}", e);
            }
        }
        Err(_) => warn!("Timeout waiting for origin health checker to shut down"),
    }

    match timeout(shutdown_timeout, config_handle).await {
        Ok(join_result) => {
            if let Err(e) = join_result {
                warn!("Config agent join error: {}", e);
            }
        }
        Err(_) => warn!("Timeout waiting for config agent to shut down"),
    }

    if let Some(handle) = upgrade_handle_opt {
        match timeout(shutdown_timeout, handle).await {
            Ok(join_result) => {
                if let Err(e) = join_result {
                    warn!("Upgrade watcher join error: {}", e);
                }
            }
            Err(_) => warn!("Timeout waiting for upgrade watcher to shut down"),
        }
    }

    if let Some(handle) = geoip_updater_handle_opt {
        match timeout(shutdown_timeout, handle).await {
            Ok(join_result) => {
                if let Err(e) = join_result {
                    warn!("GeoIP updater join error: {}", e);
                }
            }
            Err(_) => warn!("Timeout waiting for GeoIP updater to shut down"),
        }
    }

    // Flush cache to disk before exit
    match cache.flush() {
        Ok(_) => info!("Cache flushed to disk"),
        Err(e) => warn!("Failed to flush cache: {}", e),
    }

    info!("LingCDN Node stopped");
    Ok(())
}

async fn run_startup_checks(config: &NodeConfig) -> Result<()> {
    // Cache目录可写检查
    if let Some(ref cache_dir) = config.cache_dir {
        let path = Path::new(cache_dir);
        fs::create_dir_all(path)
            .await
            .with_context(|| format!("无法创建缓存目录 {:?}", path))?;

        let probe = path.join(".lingcdn_write_probe");
        if let Err(e) = fs::write(&probe, b"ok").await {
            return Err(anyhow::anyhow!("缓存目录不可写 {:?}: {}", path, e));
        }
        let _ = fs::remove_file(&probe).await;
        info!("缓存目录可用: {:?}", path);
    }

    // 访问日志 / 错误日志目录可写检查（用于 Filebeat 读取并推送到 ES）
    for (label, raw_path) in [
        ("访问日志", config.access_log_path.as_str()),
        ("错误日志", config.error_log_path.as_str()),
    ] {
        let path = Path::new(raw_path);
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)
                .await
                .with_context(|| format!("无法创建{}目录 {:?}", label, parent))?;

            let probe = parent.join(".lingcdn_log_probe");
            if let Err(e) = fs::write(&probe, b"ok").await {
                return Err(anyhow::anyhow!("{}目录不可写 {:?}: {}", label, parent, e));
            }
            let _ = fs::remove_file(&probe).await;
            info!("{}目录可用: {:?}", label, parent);
        }
    }

    // 控制端点 DNS 解析检查
    let url = Url::parse(&config.control_endpoint)
        .with_context(|| format!("控制端点URL不合法: {}", config.control_endpoint))?;
    if let Some(host) = url.host_str() {
        let port = url.port_or_known_default().unwrap_or(80);
        let resolved: Vec<_> = lookup_host((host, port))
            .await
            .with_context(|| format!("控制端点解析失败: {}:{}", host, port))?
            .collect();
        if resolved.is_empty() {
            return Err(anyhow::anyhow!("控制端点解析为空: {}:{}", host, port));
        }
        info!("控制端点可解析: {}:{}", host, port);
    }

    if config.tls_enabled {
        info!("TLS 已开启，将在收到证书配置后完成加载验证");
    }

    Ok(())
}

fn dir_size(path: &Path) -> Result<u64> {
    let meta = std::fs::symlink_metadata(path)
        .with_context(|| format!("Failed to stat {}", path.display()))?;
    if meta.is_file() {
        return Ok(meta.len());
    }
    if !meta.is_dir() {
        return Ok(0);
    }
    let mut total = 0u64;
    for entry in std::fs::read_dir(path).with_context(|| format!("Failed to read dir {}", path.display()))? {
        let entry = entry?;
        total = total.saturating_add(dir_size(&entry.path())?);
    }
    Ok(total)
}

async fn start_heartbeat(
    mut grpc_client: GrpcClient,
    node_id: String,
    token: String,
    metrics: Arc<Metrics>,
    shutdown_tx: broadcast::Sender<()>,
    mut shutdown: broadcast::Receiver<()>,
    upgrade_cmd_tx: Option<tokio::sync::mpsc::Sender<UpgradeTrigger>>,
    proxy_service: Arc<ProxyService>,
) {
    // Default `MissedTickBehavior` is Burst: if the previous iteration
    // took longer than 30s (slow gRPC call, suspended VM, tokio-runtime
    // starvation under load), tick() returns immediately for every
    // missed period, triggering a burst of back-to-back heartbeats that
    // the control plane then has to de-duplicate. Delay "resets the
    // clock" after each tick, which is what a heartbeat loop actually
    // wants — one beat per interval, no catch-up storms.
    let mut interval = tokio::time::interval(tokio::time::Duration::from_secs(30));
    interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
    let mut consecutive_failures: u32 = 0;

    loop {
        tokio::select! {
            _ = shutdown.recv() => {
                info!("Heartbeat shutdown requested; sending offline notification");
                let metrics = metrics.snapshot();
                if let Err(e) = grpc_client.heartbeat(&node_id, &token, "stopped", metrics, vec![]).await {
                    error!("Failed to send shutdown heartbeat: {}", e);
                }
                break;
            }
            _ = interval.tick() => {}
        }

        let metrics = metrics.snapshot();

        // 收集本地拉黑的IP用于上报
        let waf_bans: Vec<_> = proxy_service.collect_waf_bans()
            .into_iter()
            .map(|(ip, mode, strikes, remaining_secs)| {
                use crate::proto::node::WafBanReport;
                let expires_at_unix = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .map(|d| d.as_secs() as i64 + remaining_secs as i64)
                    .unwrap_or(0);
                WafBanReport {
                    ip,
                    reason: mode,
                    strikes: strikes as i32,
                    expires_at_unix,
                }
            })
            .collect();

        match grpc_client.heartbeat(&node_id, &token, "running", metrics, waf_bans).await {
            Ok(response) => {
                if !response.ok {
                    error!("Heartbeat failed: {}", response.message);
                    if response.message.to_ascii_lowercase().contains("disabled") {
                        warn!("Node disabled by control plane, shutting down gracefully");
                        // 发出关闭信号：停止监听、缓存刷新等
                        let _ = shutdown_tx.send(());
                        break;
                    }
                    consecutive_failures = consecutive_failures.saturating_add(1);
                    let backoff = Duration::from_secs(2u64.saturating_pow(consecutive_failures.min(5)) * 5);
                    tokio::select! {
                        _ = shutdown.recv() => {
                            info!("Heartbeat shutdown during backoff");
                            break;
                        }
                        _ = sleep(backoff) => {}
                    }
                } else {
                    consecutive_failures = 0;
                    if let Some(trigger) = parse_upgrade_trigger(&response.message) {
                        if let Some(tx) = &upgrade_cmd_tx {
                            if let Err(e) = tx.try_send(trigger) {
                                warn!("Failed to enqueue upgrade trigger: {}", e);
                            }
                        } else {
                            warn!("Upgrade command received but upgrade watcher is disabled");
                        }
                    }
                }
            }
            Err(e) => {
                error!("Failed to send heartbeat: {}", e);
                consecutive_failures = consecutive_failures.saturating_add(1);
                let backoff = Duration::from_secs(2u64.saturating_pow(consecutive_failures.min(5)) * 5);
                tokio::select! {
                    _ = shutdown.recv() => {
                        info!("Heartbeat shutdown during backoff");
                        break;
                    }
                    _ = sleep(backoff) => {}
                }
            }
        }
    }
}

#[derive(Debug, Deserialize)]
struct ControlCommand {
    #[serde(rename = "type")]
    typ: String,
    task_id: Option<String>,
    target_version: Option<String>,
    channel: Option<String>,
    force: Option<bool>,
}

fn parse_upgrade_trigger(message: &str) -> Option<UpgradeTrigger> {
    const PREFIX: &str = "lingcdn:cmd:";
    let msg = message.trim();
    if !msg.starts_with(PREFIX) {
        return None;
    }
    let payload = msg.trim_start_matches(PREFIX).trim();
    if payload.is_empty() {
        return None;
    }
    let cmd: ControlCommand = match serde_json::from_str(payload) {
        Ok(v) => v,
        Err(e) => {
            warn!("Invalid control command payload: {}", e);
            return None;
        }
    };
    if cmd.typ != "upgrade" {
        return None;
    }
    Some(UpgradeTrigger {
        task_id: cmd.task_id.unwrap_or_default(),
        target_version: cmd.target_version.unwrap_or_else(|| "latest".to_string()),
        channel: cmd.channel.unwrap_or_default(),
        force: cmd.force.unwrap_or(false),
    })
}

/// 实时上报拉黑IP到主控
async fn start_ban_reporter(
    mut grpc_client: GrpcClient,
    node_id: String,
    token: String,
    mut ban_event_rx: tokio::sync::mpsc::UnboundedReceiver<BanEvent>,
    mut shutdown: broadcast::Receiver<()>,
    xdp_controller: Option<Arc<tokio::sync::RwLock<xdp::XdpController>>>,
) {
    use crate::proto::node::WafBanReport;
    use std::net::IpAddr;

    loop {
        tokio::select! {
            _ = shutdown.recv() => {
                info!("Ban reporter shutdown requested");
                break;
            }
            event = ban_event_rx.recv() => {
                let Some(event) = event else {
                    info!("Ban event channel closed");
                    break;
                };

                // Sync to XDP blacklist if available (XDP currently supports IPv4 only)
                if let Some(ref xdp) = xdp_controller {
                    match event.ip.parse::<IpAddr>() {
                        Ok(IpAddr::V4(ipv4)) => {
                            let expires = if event.expires_at_unix <= 0 { 0 } else { event.expires_at_unix as u64 };
                            let controller = xdp.read().await;
                            if let Err(e) = controller.add_blacklist_ip(ipv4, expires).await {
                                warn!("Failed to add {} to XDP blacklist: {}", event.ip, e);
                            }
                        }
                        Ok(IpAddr::V6(_)) => {
                            warn!("XDP blacklist does not support IPv6 yet, skipping: {}", event.ip);
                        }
                        Err(_) => {
                            warn!("Failed to parse ban IP for XDP blacklist: {}", event.ip);
                        }
                    }
                }

                let ban = WafBanReport {
                    ip: event.ip.clone(),
                    reason: event.reason,
                    strikes: event.strikes as i32,
                    expires_at_unix: event.expires_at_unix,
                };

                match grpc_client.report_waf_ban(&node_id, &token, ban).await {
                    Ok(response) => {
                        if response.ok {
                            info!("Reported WAF ban to control: ip={}", event.ip);
                        } else {
                            warn!("Failed to report WAF ban: {}", response.message);
                        }
                    }
                    Err(e) => {
                        error!("Failed to report WAF ban: {}", e);
                    }
                }
            }
        }
    }
}
