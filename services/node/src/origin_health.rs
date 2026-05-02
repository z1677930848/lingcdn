use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use parking_lot::RwLock;
use reqwest::Client;
use tokio::sync::broadcast;
use tracing::{debug, info, warn};

use crate::config::{ConfigHolder, OriginHealthCheckConfig};

/// Per-(domain_id, address) health record. Pure scalars so the whole
/// table can sit behind a single RwLock without contention concerns —
/// probes happen on a background timer, not in the request hot path,
/// and read-side `is_healthy` only takes a shared lock.
#[derive(Debug, Clone)]
struct HealthState {
    healthy: bool,
    consecutive_fail: u32,
    consecutive_pass: u32,
}

impl Default for HealthState {
    fn default() -> Self {
        // Start optimistic. A fresh origin gets a chance to serve the
        // first request before the probe has had time to run, which is
        // the right call when probes are misconfigured (better to try
        // the origin than to refuse all traffic).
        Self {
            healthy: true,
            consecutive_fail: 0,
            consecutive_pass: 0,
        }
    }
}

/// `OriginHealthChecker` owns the read/write state and the probe HTTP
/// client. Public methods are split into:
///   - `is_healthy` — synchronous, called from the request hot path.
///   - `run` — background loop launched once at boot; drives probes.
///
/// Cloning a checker shares the underlying state (Arc<RwLock<...>>),
/// so the background task and the proxy can both hold one without
/// duplicating the table.
pub struct OriginHealthChecker {
    states: Arc<RwLock<HashMap<(String, String), HealthState>>>,
    config_holder: Arc<ConfigHolder>,
    client: Client,
}

impl OriginHealthChecker {
    pub fn new(config_holder: Arc<ConfigHolder>) -> Self {
        // A short connect timeout keeps probes from piling up when an
        // origin is hard-down. The per-probe timeout below is bounded
        // by the per-domain `timeout_ms`; this is just the network
        // dial budget.
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(3))
            .pool_idle_timeout(Some(Duration::from_secs(30)))
            .build()
            .unwrap_or_else(|_| Client::new());
        Self {
            states: Arc::new(RwLock::new(HashMap::new())),
            config_holder,
            client,
        }
    }

    /// Look up the current health verdict. Unknown (domain, address)
    /// pairs are treated as healthy so traffic reaches a freshly
    /// added origin immediately rather than waiting one probe tick.
    pub fn is_healthy(&self, domain_id: &str, address: &str) -> bool {
        let key = (domain_id.to_string(), address.to_string());
        let states = self.states.read();
        states.get(&key).map(|s| s.healthy).unwrap_or(true)
    }

    /// Background driver. Runs on a 5-second heartbeat: cheap enough
    /// to be effectively idle when no domain has health-check on, and
    /// short enough that a `interval_sec=5` (the minimum) probe still
    /// gets exercised every cycle.
    pub async fn run(self: Arc<Self>, mut shutdown: broadcast::Receiver<()>) {
        info!("Origin health checker started");
        let mut last_probe: HashMap<(String, String), std::time::Instant> = HashMap::new();
        let tick = Duration::from_secs(5);
        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    info!("Origin health checker shutting down");
                    return;
                }
                _ = tokio::time::sleep(tick) => {}
            }

            let now = std::time::Instant::now();
            let snapshot = self.config_holder.get();
            let Some(config) = snapshot else { continue };

            // Build the desired (domain_id, address, hc_config) work
            // list from the current config. Any keys in `last_probe`
            // and `states` that don't appear here are stale — clean
            // them out at the bottom of the loop so memory doesn't
            // grow with every config rotation.
            let mut desired: HashMap<(String, String), OriginHealthCheckConfig> = HashMap::new();
            for domain in config.domains.iter() {
                let Some(hc) = domain.origin_health_check.as_ref() else { continue };
                if !hc.enabled {
                    continue;
                }
                for o in domain.origins.iter() {
                    if !o.enabled || o.address.trim().is_empty() {
                        continue;
                    }
                    desired.insert(
                        (domain.id.clone(), o.address.clone()),
                        hc.clone(),
                    );
                }
            }

            // Probe each desired pair whose interval has elapsed. We
            // launch probes in parallel via spawned tasks bounded by
            // the desired set size so a slow origin doesn't stall
            // probes for healthy ones.
            let mut handles = Vec::new();
            for (key, hc) in desired.iter() {
                let interval = Duration::from_secs(hc.interval_sec.max(5) as u64);
                if let Some(prev) = last_probe.get(key) {
                    if now.duration_since(*prev) < interval {
                        continue;
                    }
                }
                last_probe.insert(key.clone(), now);
                let checker = self.clone();
                let key = key.clone();
                let hc = hc.clone();
                handles.push(tokio::spawn(async move {
                    checker.probe_once(&key, &hc).await;
                }));
            }
            // Wait for all probes spawned this tick to finish so we
            // don't pile up unbounded work across ticks. Errors from
            // the JoinHandle are not actionable — the probe itself
            // already records pass/fail via update_state.
            for h in handles {
                let _ = h.await;
            }

            // Drop stale state for origins/domains that no longer
            // have health-check enabled, so memory tracks the live
            // config rather than every origin we have ever probed.
            let alive: std::collections::HashSet<(String, String)> = desired.keys().cloned().collect();
            last_probe.retain(|k, _| alive.contains(k));
            {
                let mut states = self.states.write();
                states.retain(|k, _| alive.contains(k));
            }
        }
    }

    async fn probe_once(&self, key: &(String, String), hc: &OriginHealthCheckConfig) {
        let path = if hc.path.is_empty() { "/".to_string() } else { hc.path.clone() };
        let timeout_ms = if hc.timeout_ms <= 0 { 5000u64 } else { hc.timeout_ms as u64 };

        // Build the probe URL. We always use http:// for the probe
        // and rely on the origin to accept loopback traffic on its
        // configured port — replicating the upstream handshake the
        // proxy itself does. Origin addresses are "host" or
        // "host:port"; if no port we default to 80 to keep the probe
        // simple.
        let addr = key.1.trim();
        let url = if addr.contains("://") {
            format!("{}{}", addr.trim_end_matches('/'), path)
        } else if addr.contains(':') {
            format!("http://{}{}", addr, path)
        } else {
            format!("http://{}:80{}", addr, path)
        };

        let result = tokio::time::timeout(
            Duration::from_millis(timeout_ms),
            self.client
                .get(&url)
                .header("User-Agent", "lingcdn-health/1.0")
                .header("Connection", "close")
                .send(),
        )
        .await;

        let success = match result {
            Ok(Ok(resp)) => {
                let code = resp.status().as_u16();
                if hc.expected_status > 0 {
                    code as i32 == hc.expected_status
                } else {
                    // Default acceptance: any 2xx/3xx is healthy. 4xx
                    // and 5xx are treated as failure because they
                    // typically indicate a misconfigured origin or
                    // outage that should drain traffic away.
                    (200..400).contains(&code)
                }
            }
            Ok(Err(err)) => {
                debug!(
                    "health probe error: domain={} addr={} url={} err={}",
                    key.0, key.1, url, err
                );
                false
            }
            Err(_) => {
                debug!(
                    "health probe timeout: domain={} addr={} url={} timeout_ms={}",
                    key.0, key.1, url, timeout_ms
                );
                false
            }
        };

        self.update_state(key, success, hc);
    }

    fn update_state(
        &self,
        key: &(String, String),
        success: bool,
        hc: &OriginHealthCheckConfig,
    ) {
        let fail_threshold = hc.fail_threshold.max(1) as u32;
        let pass_threshold = hc.pass_threshold.max(1) as u32;
        let mut states = self.states.write();
        let entry = states.entry(key.clone()).or_default();
        if success {
            entry.consecutive_fail = 0;
            entry.consecutive_pass = entry.consecutive_pass.saturating_add(1);
            if !entry.healthy && entry.consecutive_pass >= pass_threshold {
                entry.healthy = true;
                info!(
                    "origin recovered: domain={} addr={} consecutive_pass={}",
                    key.0, key.1, entry.consecutive_pass
                );
            }
        } else {
            entry.consecutive_pass = 0;
            entry.consecutive_fail = entry.consecutive_fail.saturating_add(1);
            if entry.healthy && entry.consecutive_fail >= fail_threshold {
                entry.healthy = false;
                warn!(
                    "origin marked unhealthy: domain={} addr={} consecutive_fail={}",
                    key.0, key.1, entry.consecutive_fail
                );
            }
        }
    }
}
