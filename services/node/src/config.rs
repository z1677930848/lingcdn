use anyhow::{Context, Result};
use parking_lot::RwLock;
use semver::Version;
use serde::{Deserialize, Serialize};
use std::collections::{HashMap, HashSet};
use std::env;
use std::fs;
use std::net::IpAddr;
use std::path::PathBuf;
use std::sync::Arc;
use tracing::warn;

use ipnet::IpNet;
use regex::Regex;

use crate::router::Router;

const HARDCODED_UPGRADE_ENDPOINT: &str = "https://auth.lingcdn.cloud/api/upgrade/latest";
const DEFAULT_UPGRADE_ENDPOINT: &str = HARDCODED_UPGRADE_ENDPOINT;

/// Node configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(default)]
pub struct NodeConfig {
    pub node_id: Option<String>,
    pub control_endpoint: String,
    pub control_tls_enabled: bool,
    pub control_tls_ca_file: Option<String>,
    pub control_tls_client_cert_file: Option<String>,
    pub control_tls_client_key_file: Option<String>,
    pub control_tls_domain_name: Option<String>,
    pub bootstrap_token: String,
    pub hostname: String,
    pub version: String,
    pub capabilities: Vec<String>,
    pub listen_addr: String,
    pub ops_listen_addr: String,
    pub ops_token: Option<String>,
    pub tls_enabled: bool,
    pub cache_dir: Option<String>,
    pub memory_cache_capacity: usize,
    pub max_request_body_bytes: u64,
    pub max_response_body_bytes: u64,
    pub max_cache_object_bytes: u64,
    pub disk_cache_max_bytes: u64,
    pub disk_cache_gc_interval_seconds: u64,
    pub disk_cache_recreate_on_exceed: bool,
    pub access_log_path: String,
    pub error_log_path: String,
    pub region: Option<String>,
    pub geoip_db_path: Option<String>,
    pub geoip_update_endpoint: Option<String>,
    pub geoip_update_seconds: u64,
    pub geoip_update_max_download_bytes: u64,
    pub geoip_update_token: Option<String>,
    pub max_connections: usize,
    pub upgrade_endpoint: Option<String>,
    pub upgrade_pubkey: Option<String>,
    pub upgrade_channel: String,
    pub upgrade_check_seconds: u64,
    pub upgrade_max_download_bytes: u64,
    // XDP 配置
    pub xdp_enabled: bool,
    pub xdp_interface: Option<String>,
    pub xdp_rate_limit_enabled: bool,
    pub xdp_syn_flood_enabled: bool,
    pub xdp_rate_limit_pps: u64,
    pub xdp_syn_limit_pps: u64,
}

impl Default for NodeConfig {
    fn default() -> Self {
        Self {
            node_id: None,
            control_endpoint: "http://127.0.0.1:9443".to_string(),
            control_tls_enabled: false,
            control_tls_ca_file: None,
            control_tls_client_cert_file: None,
            control_tls_client_key_file: None,
            control_tls_domain_name: None,
            bootstrap_token: String::new(),
            hostname: hostname::get()
                .ok()
                .and_then(|h| h.into_string().ok())
                .unwrap_or_else(|| "unknown".to_string()),
            version: env!("CARGO_PKG_VERSION").to_string(),
            capabilities: vec!["http".to_string(), "https".to_string(), "cache".to_string()],
            listen_addr: "0.0.0.0:80".to_string(),
            ops_listen_addr: "127.0.0.1:9101".to_string(),
            ops_token: None,
            tls_enabled: false,
            cache_dir: Some("/var/lib/lingcdn/cache".to_string()),
            memory_cache_capacity: 10000,
            max_request_body_bytes: 32 * 1024 * 1024,
            max_response_body_bytes: 512 * 1024 * 1024,
            max_cache_object_bytes: 32 * 1024 * 1024,
            disk_cache_max_bytes: 20 * 1024 * 1024 * 1024,
            disk_cache_gc_interval_seconds: 60,
            disk_cache_recreate_on_exceed: true,
            access_log_path: "/var/log/lingcdn/access.log".to_string(),
            error_log_path: "/var/log/lingcdn/error.log".to_string(),
            region: None,
            geoip_db_path: None,
            geoip_update_endpoint: None,
            geoip_update_seconds: 604800,
            geoip_update_max_download_bytes: 256 * 1024 * 1024,
            geoip_update_token: None,
            max_connections: 1024,
            upgrade_endpoint: None,
            upgrade_pubkey: None,
            upgrade_channel: "stable".to_string(),
            upgrade_check_seconds: 900,
            upgrade_max_download_bytes: 1 * 1024 * 1024 * 1024,
            xdp_enabled: false,
            xdp_interface: None,
            xdp_rate_limit_enabled: true,
            xdp_syn_flood_enabled: true,
            xdp_rate_limit_pps: 10000,
            xdp_syn_limit_pps: 100,
        }
    }
}

/// Runtime configuration from control plane
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RuntimeConfig {
    pub version: String,
    pub checksum: String,
    pub domains: Vec<DomainConfig>,
    pub origins: HashMap<String, OriginConfig>,
    #[serde(default)]
    pub certificates: HashMap<String, CertificateConfig>,
    pub cache_rules: Vec<CacheRule>,
    #[serde(default)]
    pub waf_policies: Vec<WAFPolicy>,
    #[serde(default)]
    pub waf_bans: Vec<WAFBan>,
    #[serde(default)]
    pub waf_whitelist: Vec<WAFWhitelist>,
    #[serde(default)]
    pub license: Option<LicenseConfig>,
    #[serde(default)]
    pub templates: GlobalTemplates,
    #[serde(default)]
    pub stream_forwards: Vec<StreamForwardConfig>,
    #[serde(default)]
    pub l2_peers: Vec<L2PeerConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct L2PeerConfig {
    pub node_id: String,
    pub address: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamForwardConfig {
    pub id: String,
    #[serde(default)]
    pub user_id: String,
    #[serde(default = "default_stream_protocol")]
    pub protocol: String,
    pub listen_port: i32,
    pub origin_host: String,
    pub origin_port: i32,
    #[serde(default = "default_origin_enabled")]
    pub enabled: bool,
    #[serde(default = "default_health_check_enabled")]
    pub health_check_enabled: bool,
}

impl StreamForwardConfig {
    pub fn effective_listen_port(&self) -> Option<u16> {
        match self.listen_port {
            p if p > 0 && p <= u16::MAX as i32 => Some(p as u16),
            _ => None,
        }
    }
}

fn default_stream_protocol() -> String {
    "tcp".to_string()
}

fn default_health_check_enabled() -> bool {
    true
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LicenseConfig {
    pub status: String,
    #[serde(default)]
    pub expires_at_unix: i64,
    #[serde(default)]
    pub reason: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DomainConfig {
    pub id: String,
    pub name: String,
    pub origin_id: String,
    pub cert_id: Option<String>,
    #[serde(default)]
    pub listen_port: Option<i32>,
    #[serde(default)]
    pub https_enabled: Option<bool>,
    #[serde(default)]
    pub websocket_enabled: Option<bool>,
    pub origin_scheme: Option<String>,
    pub origin_port: Option<i32>,
    pub origin_host_mode: Option<String>,
    pub origin_host: Option<String>,
    pub origin_timeout_ms: Option<i64>,
    pub origin_connect_timeout_ms: Option<i64>,
    #[serde(default)]
    pub cache_enabled: Option<bool>,
    #[serde(default)]
    pub http2_enabled: Option<bool>,
    #[serde(default)]
    pub http3_enabled: Option<bool>,
    #[serde(default)]
    pub l2_origin_enabled: Option<bool>,
    #[serde(default)]
    pub error_pages: Vec<ErrorPage>,
    /// Per-domain upstreams. When non-empty, these replace the legacy
    /// `origin_id -> origins[origin_id].addresses` lookup entirely.
    /// The proxy performs weighted-random selection across `enabled`
    /// entries and falls back to the remaining enabled entries on
    /// transport-layer failure. Empty vec means "use legacy origin_id"
    /// (kept for domains migrated before the refactor).
    #[serde(default)]
    pub origin_auth: Option<OriginAuthConfig>,
    #[serde(default)]
    pub origins: Vec<DomainOriginConfig>,
    /// Per-domain origin distribution policy. "round_robin" (default)
    /// keeps the legacy weighted-random selection; "ip_hash" pins the
    /// pick to the client IP for sticky sessions. Unknown values are
    /// treated as "round_robin" at the call site.
    #[serde(default = "default_load_balance_method")]
    pub load_balance_method: String,
    /// Optional active health check for origin addresses. When `enabled`,
    /// a background task probes each origin and removes failing ones
    /// from the candidate pool until they recover. nil/None means no
    /// active probing — selection still falls back gracefully on
    /// transport-layer errors as before.
    #[serde(default)]
    pub origin_health_check: Option<OriginHealthCheckConfig>,
    #[serde(default)]
    pub signed_url_secret: Option<String>,
    #[serde(default)]
    pub bot_score_enabled: Option<bool>,
    #[serde(default)]
    pub response_compress_enabled: Option<bool>,
    #[serde(default)]
    pub edge_script_enabled: Option<bool>,
    #[serde(default)]
    pub edge_script_rules: Option<String>,
    #[serde(default)]
    pub image_transform_enabled: Option<bool>,
    #[serde(default)]
    pub video_segment_cache_enabled: Option<bool>,
}

impl DomainConfig {
    pub fn effective_https_enabled(&self) -> bool {
        self.https_enabled.unwrap_or_else(|| {
            self.cert_id
                .as_deref()
                .map(|s| !s.trim().is_empty())
                .unwrap_or(false)
        })
    }

    pub fn effective_websocket_enabled(&self) -> bool {
        self.websocket_enabled.unwrap_or(false)
    }

    pub fn effective_http3_enabled(&self) -> bool {
        self.http3_enabled.unwrap_or(false)
    }

    pub fn effective_l2_origin_enabled(&self) -> bool {
        self.l2_origin_enabled.unwrap_or(false)
    }

    pub fn effective_bot_score_enabled(&self) -> bool {
        self.bot_score_enabled.unwrap_or(false)
    }

    pub fn effective_response_compress_enabled(&self) -> bool {
        self.response_compress_enabled.unwrap_or(false)
    }

    pub fn effective_edge_script_enabled(&self) -> bool {
        self.edge_script_enabled.unwrap_or(false)
    }

    pub fn effective_image_transform_enabled(&self) -> bool {
        self.image_transform_enabled.unwrap_or(false)
    }

    pub fn effective_video_segment_cache_enabled(&self) -> bool {
        self.video_segment_cache_enabled.unwrap_or(false)
    }

    pub fn effective_listen_port(&self) -> Option<u16> {
        match self.listen_port.unwrap_or(0) {
            p if p > 0 && p <= u16::MAX as i32 => Some(p as u16),
            _ => None,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OriginHealthCheckConfig {
    #[serde(default)]
    pub enabled: bool,
    #[serde(default)]
    pub interval_sec: i32,
    #[serde(default)]
    pub timeout_ms: i64,
    #[serde(default)]
    pub path: String,
    #[serde(default)]
    pub expected_status: i32,
    #[serde(default)]
    pub fail_threshold: i32,
    #[serde(default)]
    pub pass_threshold: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OriginAuthConfig {
    #[serde(default)]
    pub enabled: bool,
    #[serde(default)]
    pub mode: Option<String>,
    #[serde(default)]
    pub headers: Vec<OriginAuthHeader>,
    #[serde(default)]
    pub basic_user: Option<String>,
    #[serde(default)]
    pub basic_pass: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OriginAuthHeader {
    pub name: String,
    #[serde(default)]
    pub value: String,
}

/// A single upstream address bound to a specific domain. `weight` is
/// clamped to 1..=100 by the control plane, `enabled=false` means
/// "skip this entry entirely" (not even eligible for failover).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DomainOriginConfig {
    pub address: String,
    #[serde(default = "default_origin_weight")]
    pub weight: i32,
    #[serde(default = "default_origin_enabled")]
    pub enabled: bool,
    #[serde(default = "default_health_check_enabled")]
    pub health_check_enabled: bool,
}

fn default_origin_weight() -> i32 {
    1
}

fn default_origin_enabled() -> bool {
    true
}

fn default_load_balance_method() -> String {
    "round_robin".to_string()
}

fn default_waf_rule_enabled() -> bool {
    true
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OriginConfig {
    pub id: String,
    pub name: String,
    pub addresses: Vec<String>,
    pub timeout_ms: u64,
    pub max_retries: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CertificateConfig {
    pub id: String,
    pub domain: String,
    #[serde(default)]
    pub cert_pem: Option<Vec<u8>>,
    #[serde(default)]
    pub key_pem: Option<Vec<u8>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorPage {
    pub status: i32,
    pub mode: String,    // html | json | redirect
    pub content: String, // html/json template or redirect URL
}

/// Admin-editable templates pushed from the control plane. Any empty
/// string means "no override" and the node keeps its hardcoded fallback.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct GlobalTemplates {
    #[serde(default)]
    pub error_pages: HashMap<i32, String>,
    #[serde(default)]
    pub error_default: String,
    #[serde(default)]
    pub waf_shield_page: String,
    #[serde(default)]
    pub waf_ban_default: String,
    #[serde(default)]
    pub waf_challenge_default_json: String,
    #[serde(default)]
    pub cname_not_found: String,
    #[serde(default)]
    pub direct_ip: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CacheRule {
    pub host_pattern: Option<String>,
    pub path_pattern: Option<String>,
    pub methods: Vec<String>,
    pub ttl_seconds: u64,
    pub cache_query_params: bool,
    #[serde(default)]
    pub stale_while_revalidate_seconds: u64,
    #[serde(default)]
    pub stale_if_error_seconds: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WAFPolicy {
    pub id: String,
    pub name: String,
    pub scope: String, // global | domain | line_group
    pub scope_id: Option<String>,
    pub description: Option<String>,
    pub enabled: bool,
    pub rules: Vec<WAFRule>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WAFRule {
    pub id: String,
    pub r#type: String, // ip_cidr | challenge_captcha | geo_block
    pub action: String, // allow | deny
    pub value: String,  // CIDR or path prefix
    pub threshold: Option<i64>,
    pub window_seconds: Option<i64>,
    pub note: Option<String>,
    pub priority: Option<i32>,
    pub shield_seconds: Option<i64>,
    pub auto_challenge_qps: Option<i64>,
    pub expires_at: Option<i64>,
    #[serde(default = "default_waf_rule_enabled")]
    pub enabled: bool,
    pub ban_seconds: Option<i64>,
    pub template_html: Option<String>,
    pub ban_template_html: Option<String>,
    pub redirect_url: Option<String>,
    pub ban_mode: Option<String>,
    pub captcha_type: Option<String>,
    pub path_prefix: Option<String>,
    pub methods: Option<Vec<String>>,
    pub ua_contains: Option<String>,
    pub log_only: Option<bool>,
    #[serde(default)]
    pub geo_countries: Vec<String>, // ISO 3166-1 alpha-2 codes for geo_block
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WAFBan {
    pub ip: String,
    pub reason: Option<String>,
    pub strikes: Option<i32>,
    pub expires_at: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WAFWhitelist {
    pub ip: String,
    pub note: Option<String>,
}

/// Thread-safe runtime configuration holder
#[derive(Clone)]
pub struct RuntimeState {
    pub config: Arc<RuntimeConfig>,
    pub router: Arc<Router>,
    pub waf: Arc<CompiledWaf>,
}

#[derive(Clone, Default)]
pub struct CompiledWaf {
    pub global_rules: Vec<CompiledWafRule>,
    pub domain_rules: HashMap<String, Vec<CompiledWafRule>>,
    pub whitelist_exact: HashSet<String>,
    pub whitelist_cidrs: Vec<IpNet>,
    pub bans_exact: HashMap<String, i64>,
    pub bans_cidrs: Vec<(IpNet, i64)>,
}

#[derive(Clone)]
pub struct CompiledWafRule {
    pub rule: WAFRule,
    pub priority: i32,
    pub seq: u64,
    pub cidr: Option<IpNet>,
    pub ua_contains_lower: Option<String>,
    pub regex: Option<Regex>,
}

impl CompiledWaf {
    fn compile_waf_regex(rule: &WAFRule) -> Option<Regex> {
        match rule.r#type.as_str() {
            "sql_injection" | "xss" | "path_traversal" | "ua_block" => {
                if rule.value.is_empty() {
                    None
                } else {
                    Regex::new(&rule.value).ok()
                }
            }
            _ => None,
        }
    }

    pub fn from_config(config: &RuntimeConfig) -> Self {
        let mut seq: u64 = 0;
        let mut global_rules: Vec<CompiledWafRule> = Vec::new();
        let mut domain_rules: HashMap<String, Vec<CompiledWafRule>> = HashMap::new();

        for policy in &config.waf_policies {
            if !policy.enabled {
                continue;
            }

            match policy.scope.as_str() {
                "global" => {
                    for rule in &policy.rules {
                        if !rule.enabled {
                            continue;
                        }
                        seq = seq.saturating_add(1);
                        let priority = rule.priority.unwrap_or(0);
                        let cidr = if rule.r#type == "ip_cidr" {
                            rule.value.parse::<IpNet>().ok()
                        } else {
                            None
                        };
                        let ua_contains_lower = rule.ua_contains.as_ref().and_then(|s| {
                            if s.is_empty() {
                                None
                            } else {
                                Some(s.to_ascii_lowercase())
                            }
                        });
                        global_rules.push(CompiledWafRule {
                            rule: rule.clone(),
                            priority,
                            seq,
                            cidr,
                            ua_contains_lower,
                            regex: Self::compile_waf_regex(rule),
                        });
                    }
                }
                "domain" => {
                    let Some(ref id) = policy.scope_id else {
                        continue;
                    };
                    let bucket = domain_rules.entry(id.clone()).or_default();
                    for rule in &policy.rules {
                        if !rule.enabled {
                            continue;
                        }
                        seq = seq.saturating_add(1);
                        let priority = rule.priority.unwrap_or(0);
                        let cidr = if rule.r#type == "ip_cidr" {
                            rule.value.parse::<IpNet>().ok()
                        } else {
                            None
                        };
                        let ua_contains_lower = rule.ua_contains.as_ref().and_then(|s| {
                            if s.is_empty() {
                                None
                            } else {
                                Some(s.to_ascii_lowercase())
                            }
                        });
                        bucket.push(CompiledWafRule {
                            rule: rule.clone(),
                            priority,
                            seq,
                            cidr,
                            ua_contains_lower,
                            regex: Self::compile_waf_regex(rule),
                        });
                    }
                }
                _ => {}
            }
        }

        // Sort rules by (priority, original sequence) to preserve stable ordering.
        let sort_rules = |rules: &mut Vec<CompiledWafRule>| {
            rules.sort_unstable_by(|a, b| (a.priority, a.seq).cmp(&(b.priority, b.seq)));
        };
        sort_rules(&mut global_rules);
        for rules in domain_rules.values_mut() {
            sort_rules(rules);
        }

        // Compile whitelist.
        let mut whitelist_exact: HashSet<String> = HashSet::new();
        let mut whitelist_cidrs: Vec<IpNet> = Vec::new();
        for w in &config.waf_whitelist {
            if w.ip.contains('/') {
                if let Ok(net) = w.ip.parse::<IpNet>() {
                    whitelist_cidrs.push(net);
                }
            } else if !w.ip.is_empty() {
                whitelist_exact.insert(w.ip.clone());
            }
        }

        // Compile global bans (best-effort); keep expiry and re-check at runtime.
        let now = chrono::Utc::now().timestamp();
        let mut bans_exact: HashMap<String, i64> = HashMap::new();
        let mut bans_cidrs: Vec<(IpNet, i64)> = Vec::new();
        for b in &config.waf_bans {
            if b.expires_at > 0 && now > b.expires_at {
                continue;
            }
            if b.ip.contains('/') {
                if let Ok(net) = b.ip.parse::<IpNet>() {
                    bans_cidrs.push((net, b.expires_at));
                }
            } else if !b.ip.is_empty() {
                // Preserve behavior: exact string match.
                let entry = bans_exact.entry(b.ip.clone()).or_insert(b.expires_at);
                // expires_at == 0 means "no expiry"; keep it if present.
                if *entry != 0 && b.expires_at == 0 {
                    *entry = 0;
                } else if *entry != 0 && b.expires_at > *entry {
                    *entry = b.expires_at;
                }
            }
        }

        Self {
            global_rules,
            domain_rules,
            whitelist_exact,
            whitelist_cidrs,
            bans_exact,
            bans_cidrs,
        }
    }

    pub fn is_ip_whitelisted(&self, client_ip: Option<&str>, client_addr: Option<IpAddr>) -> bool {
        let Some(ip) = client_ip else { return false };
        if self.whitelist_exact.contains(ip) {
            return true;
        }
        let Some(addr) = client_addr else {
            return false;
        };
        self.whitelist_cidrs.iter().any(|net| net.contains(&addr))
    }

    pub fn is_globally_banned(&self, client_ip: Option<&str>, client_addr: Option<IpAddr>) -> bool {
        let Some(ip) = client_ip else { return false };
        let now = chrono::Utc::now().timestamp();

        if let Some(expires_at) = self.bans_exact.get(ip) {
            if *expires_at <= 0 || now <= *expires_at {
                return true;
            }
        }

        let Some(addr) = client_addr else {
            return false;
        };
        for (net, expires_at) in &self.bans_cidrs {
            if *expires_at > 0 && now > *expires_at {
                continue;
            }
            if net.contains(&addr) {
                return true;
            }
        }
        false
    }
}

pub struct ConfigHolder {
    state: Arc<RwLock<Option<Arc<RuntimeState>>>>,
    // change_tx bumps a monotonically increasing counter on every update so
    // subscribers (e.g. the listener coordinator that maintains per-port
    // accept loops for domain.listen_port) can be woken up without
    // polling. tokio::sync::watch coalesces missed updates, which is what
    // we want — receivers only care that "something changed" and can then
    // re-read state via get().
    change_tx: tokio::sync::watch::Sender<u64>,
}

impl ConfigHolder {
    pub fn new() -> Self {
        let (change_tx, _) = tokio::sync::watch::channel(0u64);
        Self {
            state: Arc::new(RwLock::new(None)),
            change_tx,
        }
    }

    pub fn get(&self) -> Option<Arc<RuntimeConfig>> {
        self.state.read().as_ref().map(|s| s.config.clone())
    }

    pub fn get_state(&self) -> Option<Arc<RuntimeState>> {
        self.state.read().clone()
    }

    /// Subscribe to config updates. The receiver is notified on every
    /// successful `update()` call; the carried value is an opaque tick
    /// count that wraps on overflow. Consumers should re-read state via
    /// `get()` after `changed().await` returns.
    pub fn subscribe(&self) -> tokio::sync::watch::Receiver<u64> {
        self.change_tx.subscribe()
    }

    pub fn update(&self, config: RuntimeConfig) {
        let config = Arc::new(config);
        let router = Arc::new(Router::new(config.clone()));
        let waf = Arc::new(CompiledWaf::from_config(&config));
        *self.state.write() = Some(Arc::new(RuntimeState {
            config,
            router,
            waf,
        }));
        // Wake subscribers. send_modify is infallible even with zero
        // active receivers — an empty listener coordinator just means
        // nobody cares yet, which is fine during early startup.
        self.change_tx.send_modify(|n| *n = n.wrapping_add(1));
    }

    pub fn validate_checksum(&self, payload: &[u8], expected: &str) -> bool {
        use sha2::{Digest, Sha256};
        let mut hasher = Sha256::new();
        hasher.update(payload);
        let result = hasher.finalize();
        let actual = hex::encode(result);
        actual == expected
    }
}

impl Default for ConfigHolder {
    fn default() -> Self {
        Self::new()
    }
}

/// Load node configuration from config file (node.toml or NODE_CONFIG_PATH) with environment overrides.
pub fn load_node_config() -> Result<NodeConfig> {
    let mut config = if let Some(path) = resolve_config_path() {
        let content = fs::read_to_string(&path)
            .with_context(|| format!("Failed to read config file {}", path.display()))?;
        toml::from_str::<NodeConfig>(&content)
            .with_context(|| format!("Failed to parse {}", path.display()))?
    } else {
        NodeConfig::default()
    };

    apply_env_overrides(&mut config);
    if env::var("LISTEN_ADDR").is_err()
        && config.tls_enabled
        && config.listen_addr.trim() == "0.0.0.0:80"
    {
        config.listen_addr = "0.0.0.0:443".to_string();
    }
    // Use the hardcoded upgrade endpoint as a fallback only if no custom endpoint was
    // provided via config file or UPGRADE_ENDPOINT env var.
    if config.upgrade_endpoint.is_none() {
        config.upgrade_endpoint = Some(DEFAULT_UPGRADE_ENDPOINT.to_string());
    }
    Ok(config)
}

fn resolve_config_path() -> Option<PathBuf> {
    if let Ok(path) = env::var("NODE_CONFIG_PATH") {
        let pb = PathBuf::from(path);
        if pb.exists() {
            return Some(pb);
        }
    }

    for candidate in ["node.toml", "/etc/lingcdn/node.toml"] {
        let pb = PathBuf::from(candidate);
        if pb.exists() {
            return Some(pb);
        }
    }
    None
}

fn apply_env_overrides(config: &mut NodeConfig) {
    if let Ok(v) = env::var("CONTROL_ENDPOINT") {
        config.control_endpoint = v;
    }
    if let Ok(v) = env::var("CONTROL_TLS_ENABLED") {
        config.control_tls_enabled =
            matches!(v.to_ascii_lowercase().as_str(), "1" | "true" | "yes" | "on");
    }
    if let Ok(v) = env::var("CONTROL_CA_FILE") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.control_tls_ca_file = None;
        } else {
            config.control_tls_ca_file = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("CONTROL_CLIENT_CERT_FILE") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.control_tls_client_cert_file = None;
        } else {
            config.control_tls_client_cert_file = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("CONTROL_CLIENT_KEY_FILE") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.control_tls_client_key_file = None;
        } else {
            config.control_tls_client_key_file = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("CONTROL_TLS_DOMAIN_NAME") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.control_tls_domain_name = None;
        } else {
            config.control_tls_domain_name = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("BOOTSTRAP_TOKEN") {
        config.bootstrap_token = v;
    }
    if let Ok(v) = env::var("NODE_ID") {
        config.node_id = Some(v);
    }
    if let Ok(v) = env::var("NODE_HOSTNAME") {
        config.hostname = v;
    }
    if let Ok(v) = env::var("NODE_VERSION") {
        if let Some(override_version) = sanitize_node_version_override(&v, &config.version) {
            config.version = override_version;
        }
    }
    if let Ok(v) = env::var("NODE_CAPABILITIES") {
        let caps = v
            .split(',')
            .map(|s| s.trim())
            .filter(|s| !s.is_empty())
            .map(|s| s.to_string())
            .collect::<Vec<_>>();
        if !caps.is_empty() {
            config.capabilities = caps;
        }
    }
    if let Ok(v) = env::var("LISTEN_ADDR") {
        config.listen_addr = v;
    }
    if let Ok(v) = env::var("OPS_LISTEN_ADDR") {
        config.ops_listen_addr = v;
    }
    if let Ok(v) = env::var("OPS_TOKEN") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.ops_token = None;
        } else {
            config.ops_token = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("TLS_ENABLED") {
        config.tls_enabled = matches!(v.to_ascii_lowercase().as_str(), "1" | "true" | "yes" | "on");
    }
    if let Ok(v) = env::var("CACHE_DIR") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            config.cache_dir = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("MEMORY_CACHE_CAPACITY") {
        if let Ok(parsed) = v.parse::<usize>() {
            if parsed > 0 {
                config.memory_cache_capacity = parsed;
            }
        }
    }
    if let Ok(v) = env::var("MAX_REQUEST_BODY_BYTES") {
        if let Ok(parsed) = v.parse::<u64>() {
            config.max_request_body_bytes = parsed;
        }
    }
    if let Ok(v) = env::var("MAX_RESPONSE_BODY_BYTES") {
        if let Ok(parsed) = v.parse::<u64>() {
            config.max_response_body_bytes = parsed;
        }
    }
    if let Ok(v) = env::var("MAX_CACHE_OBJECT_BYTES") {
        if let Ok(parsed) = v.parse::<u64>() {
            config.max_cache_object_bytes = parsed;
        }
    }
    if let Ok(v) = env::var("DISK_CACHE_MAX_BYTES") {
        if let Ok(parsed) = v.parse::<u64>() {
            config.disk_cache_max_bytes = parsed;
        }
    }
    if let Ok(v) = env::var("DISK_CACHE_GC_INTERVAL_SECONDS") {
        if let Ok(parsed) = v.parse::<u64>() {
            if parsed > 0 {
                config.disk_cache_gc_interval_seconds = parsed;
            }
        }
    }
    if let Ok(v) = env::var("DISK_CACHE_RECREATE_ON_EXCEED") {
        config.disk_cache_recreate_on_exceed =
            matches!(v.to_ascii_lowercase().as_str(), "1" | "true" | "yes" | "on");
    }
    if let Ok(v) = env::var("ACCESS_LOG_PATH") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            config.access_log_path = trimmed.to_string();
        }
    }
    if let Ok(v) = env::var("ERROR_LOG_PATH") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            config.error_log_path = trimmed.to_string();
        }
    }
    if let Ok(v) = env::var("REGION") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            config.region = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("GEOIP_DB_PATH") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.geoip_db_path = None;
        } else {
            config.geoip_db_path = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("GEOIP_UPDATE_ENDPOINT") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.geoip_update_endpoint = None;
        } else {
            config.geoip_update_endpoint = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("GEOIP_UPDATE_SECONDS") {
        if let Ok(parsed) = v.trim().parse::<u64>() {
            if parsed >= 3600 {
                config.geoip_update_seconds = parsed;
            }
        }
    }
    if let Ok(v) = env::var("GEOIP_UPDATE_MAX_DOWNLOAD_BYTES") {
        if let Ok(parsed) = v.trim().parse::<u64>() {
            if parsed > 0 {
                config.geoip_update_max_download_bytes = parsed;
            }
        }
    }
    if let Ok(v) = env::var("GEOIP_UPDATE_TOKEN") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.geoip_update_token = None;
        } else {
            config.geoip_update_token = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("MAX_CONNECTIONS") {
        if let Ok(parsed) = v.parse::<usize>() {
            if parsed > 0 {
                config.max_connections = parsed;
            }
        }
    }
    if let Ok(v) = env::var("UPGRADE_ENDPOINT") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            config.upgrade_endpoint = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("UPGRADE_PUBKEY") {
        let trimmed = v.trim();
        if trimmed.is_empty() {
            config.upgrade_pubkey = None;
        } else {
            config.upgrade_pubkey = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("UPGRADE_MAX_DOWNLOAD_BYTES") {
        if let Ok(parsed) = v.parse::<u64>() {
            config.upgrade_max_download_bytes = parsed;
        }
    }
    if let Ok(v) = env::var("UPGRADE_CHANNEL") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            config.upgrade_channel = trimmed.to_string();
        }
    }
    if let Ok(v) = env::var("UPGRADE_CHECK_SECONDS") {
        if let Ok(parsed) = v.parse::<u64>() {
            if parsed >= 60 {
                config.upgrade_check_seconds = parsed;
            }
        }
    }
    // XDP 配置
    if let Ok(v) = env::var("XDP_ENABLED") {
        config.xdp_enabled = matches!(v.to_ascii_lowercase().as_str(), "1" | "true" | "yes" | "on");
    }
    if let Ok(v) = env::var("XDP_INTERFACE") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            config.xdp_interface = Some(trimmed.to_string());
        }
    }
    if let Ok(v) = env::var("XDP_RATE_LIMIT_ENABLED") {
        config.xdp_rate_limit_enabled =
            matches!(v.to_ascii_lowercase().as_str(), "1" | "true" | "yes" | "on");
    }
    if let Ok(v) = env::var("XDP_SYN_FLOOD_ENABLED") {
        config.xdp_syn_flood_enabled =
            matches!(v.to_ascii_lowercase().as_str(), "1" | "true" | "yes" | "on");
    }
    if let Ok(v) = env::var("XDP_RATE_LIMIT_PPS") {
        if let Ok(pps) = v.parse::<u64>() {
            config.xdp_rate_limit_pps = pps;
        }
    }
    if let Ok(v) = env::var("XDP_SYN_LIMIT_PPS") {
        if let Ok(pps) = v.parse::<u64>() {
            config.xdp_syn_limit_pps = pps;
        }
    }
}

/// Decide whether the NODE_VERSION env value should override the compile-time
/// CARGO_PKG_VERSION default. Returns Some(new_version) when the value is a
/// real semver, None when it is empty / "latest" / garbled.
///
/// NODE_VERSION env is sourced from /etc/lingcdn/node.env, written by
/// node_install.sh based on whatever value the portal returned at install
/// time. When the portal could not produce a concrete version (network blip,
/// build channel empty, etc.) the install script falls back to the literal
/// string "latest", and that string ends up here unchanged.
///
/// If we let "latest" overwrite the compile-time CARGO_PKG_VERSION the node
/// would heartbeat `version=latest` forever, which means:
///   * the upgrade task on the control plane never sees the node "reach the
///     target version" and stays in `running` until the 30-minute timeout,
///     surfacing as "节点无法升级" in the UI even though the binary itself
///     was replaced successfully;
///   * the upgrade worker's `is_newer("1.0.8", "latest")` falls back to a
///     string-equality check, which is non-deterministic.
///
/// Compile-time `env!("CARGO_PKG_VERSION")` is the source of truth: it
/// always matches the actual ELF that's running.
fn sanitize_node_version_override(raw: &str, current: &str) -> Option<String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        // Empty override is a no-op; quietly fall through to the default.
        return None;
    }
    if Version::parse(trimmed.trim_start_matches('v')).is_ok() {
        return Some(trimmed.to_string());
    }
    warn!(
        node_version_env = %trimmed,
        cargo_pkg_version = current,
        "NODE_VERSION env is not a valid semver (e.g. 'latest', empty, garbled) — ignoring and using compile-time version. Fix /etc/lingcdn/node.env to silence this warning."
    );
    None
}
