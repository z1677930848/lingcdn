use anyhow::{Context, Result};
use bytes::Bytes;
use http::{Request, StatusCode, Uri};
use http_body_util::{BodyExt, Empty, Full};
use hyper::body::Incoming;
use hyper_util::client::legacy::{Client, connect::HttpConnector};
use hyper_util::rt::TokioExecutor;
use hyper_rustls::HttpsConnector;
use http::Method;
use tracing::{debug, error, warn};
use rand::{thread_rng, Rng};
use hmac::{Hmac, Mac};
use sha2::Sha256;
use base64::{engine::general_purpose, Engine};
use hex;
use std::collections::{HashMap, HashSet};
use std::net::IpAddr;
use std::pin::Pin;
use std::task::{Context as TaskContext, Poll};
use std::time::{Duration, Instant};
use tokio::sync::mpsc;
use tokio::sync::{OwnedSemaphorePermit, Semaphore, Notify};
use futures::StreamExt;
use parking_lot::Mutex;

use crate::cache::{Cache, CacheEntry, CacheKey, CachedBody};
use crate::access_log::AccessLogger;
use crate::config::{CompiledWaf, CompiledWafRule, ConfigHolder, DomainConfig, OriginAuthConfig, OriginConfig};
use crate::http_types::{NodeBody, NodeResponse};
use crate::limited_body::LimitedBody;
use crate::geoip_holder::GeoIpHolder;
use crate::metrics::Metrics;
use chrono::{SecondsFormat, Utc};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};
use crate::captcha::{self, CaptchaAnswer, CaptchaType, ClickData, RotateData, SlideData};
use http::header::HeaderMap;
use http::header::ACCEPT;
use http::header::CONTENT_TYPE;
use http::header::CACHE_CONTROL;
use url::form_urlencoded;

const STATE_SHARDS: usize = 64;
const DEFAULT_IGNORE_QUERY_PARAMS: &[&str] = &[
    "utm_source",
    "utm_medium",
    "utm_campaign",
    "utm_term",
    "utm_content",
    "gclid",
    "fbclid",
    "msclkid",
    "igshid",
    "ttclid",
    "yclid",
    "_ga",
    "_gid",
];

fn shard_idx(key: &str) -> usize {
    // FNV-1a 64-bit, then take low bits. STATE_SHARDS must be power-of-two.
    let mut hash: u64 = 14695981039346656037;
    for b in key.as_bytes() {
        hash ^= *b as u64;
        hash = hash.wrapping_mul(1099511628211);
    }
    (hash as usize) & (STATE_SHARDS - 1)
}

fn unix_now_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_else(|_| Duration::from_secs(0))
        .as_secs()
}

fn plain_text_response(status: StatusCode, body: &str) -> NodeResponse {
    let mut resp = hyper::Response::new(
        Full::new(Bytes::from(body.to_string()))
            .map_err(|e| match e {})
            .boxed(),
    );
    *resp.status_mut() = status;
    resp.headers_mut().insert(
        CONTENT_TYPE,
        http::header::HeaderValue::from_static("text/plain; charset=utf-8"),
    );
    resp
}

fn build_response_or_fallback(
    builder: http::response::Builder,
    body: NodeBody,
    fallback_status: StatusCode,
    fallback_body: &str,
) -> NodeResponse {
    match builder.body(body) {
        Ok(resp) => resp,
        Err(e) => {
            warn!(error = %e, "failed to build http response");
            plain_text_response(fallback_status, fallback_body)
        }
    }
}

struct ShardedStringMap<V> {
    shards: [Mutex<HashMap<String, V>>; STATE_SHARDS],
}

impl<V> ShardedStringMap<V> {
    fn new() -> Self {
        Self {
            shards: std::array::from_fn(|_| Mutex::new(HashMap::new())),
        }
    }

    fn shard(&self, key: &str) -> &Mutex<HashMap<String, V>> {
        &self.shards[shard_idx(key)]
    }

    fn iter_shards(&self) -> impl Iterator<Item = &Mutex<HashMap<String, V>>> {
        self.shards.iter()
    }

    fn clear_all(&self) {
        for shard in self.shards.iter() {
            shard.lock().clear();
        }
    }
}

struct PermitBody {
    inner: NodeBody,
    _permit: OwnedSemaphorePermit,
}

impl http_body::Body for PermitBody {
    type Data = Bytes;
    type Error = anyhow::Error;

    fn poll_frame(
        mut self: Pin<&mut Self>,
        cx: &mut TaskContext<'_>,
    ) -> Poll<Option<Result<http_body::Frame<Self::Data>, Self::Error>>> {
        Pin::new(&mut self.inner).poll_frame(cx)
    }

    fn is_end_stream(&self) -> bool {
        self.inner.is_end_stream()
    }

    fn size_hint(&self) -> http_body::SizeHint {
        self.inner.size_hint()
    }
}

struct CacheQueryIgnore {
    exact: HashSet<String>,
    prefix: Vec<String>,
}

impl CacheQueryIgnore {
    fn from_env() -> Self {
        let raw = std::env::var("CACHE_IGNORE_QUERY_PARAMS")
            .unwrap_or_default()
            .trim()
            .to_string();
        if raw.is_empty() {
            return Self::from_list(DEFAULT_IGNORE_QUERY_PARAMS.iter().copied());
        }
        let lowered = raw.to_ascii_lowercase();
        if matches!(lowered.as_str(), "off" | "none" | "false" | "0") {
            return Self {
                exact: HashSet::new(),
                prefix: Vec::new(),
            };
        }
        let items = raw
            .split(',')
            .map(|s| s.trim())
            .filter(|s| !s.is_empty())
            .collect::<Vec<_>>();
        Self::from_list(items.into_iter())
    }

    fn from_list<I>(items: I) -> Self
    where
        I: IntoIterator,
        I::Item: AsRef<str>,
    {
        let mut exact = HashSet::new();
        let mut prefix = Vec::new();
        for it in items {
            let s = it.as_ref().trim();
            if s.is_empty() {
                continue;
            }
            if let Some(p) = s.strip_suffix('*') {
                let p = p.trim();
                if !p.is_empty() {
                    prefix.push(p.to_string());
                }
            } else {
                exact.insert(s.to_string());
            }
        }
        Self { exact, prefix }
    }

    fn should_ignore(&self, key: &str) -> bool {
        if self.exact.is_empty() && self.prefix.is_empty() {
            return false;
        }
        if self.exact.contains(key) {
            return true;
        }
        for p in &self.prefix {
            if key.starts_with(p) {
                return true;
            }
        }
        false
    }

    fn normalize(&self, raw: Option<&str>) -> Option<String> {
        let raw = raw?.trim();
        if raw.is_empty() {
            return None;
        }
        let mut pairs: Vec<(String, String)> = Vec::new();
        for (k, v) in form_urlencoded::parse(raw.as_bytes()) {
            let key = k.into_owned();
            if self.should_ignore(&key) {
                continue;
            }
            pairs.push((key, v.into_owned()));
        }
        if pairs.is_empty() {
            return None;
        }
        pairs.sort_unstable_by(|a, b| a.0.cmp(&b.0).then(a.1.cmp(&b.1)));
        let mut ser = form_urlencoded::Serializer::new(String::new());
        for (k, v) in pairs {
            ser.append_pair(&k, &v);
        }
        Some(ser.finish())
    }
}

struct CacheSingleFlight {
    inner: Arc<tokio::sync::Mutex<HashMap<String, Arc<Notify>>>>,
}

struct CacheSingleFlightGuard {
    key: String,
    inner: Arc<tokio::sync::Mutex<HashMap<String, Arc<Notify>>>>,
    notify: Arc<Notify>,
}

impl Drop for CacheSingleFlightGuard {
    fn drop(&mut self) {
        let key = self.key.clone();
        let inner = self.inner.clone();
        let notify = self.notify.clone();
        tokio::spawn(async move {
            let mut map = inner.lock().await;
            map.remove(&key);
            notify.notify_waiters();
        });
    }
}

enum CacheSingleFlightPermit {
    Leader(CacheSingleFlightGuard),
    Follower(Arc<Notify>),
}

impl CacheSingleFlight {
    fn new() -> Self {
        Self {
            inner: Arc::new(tokio::sync::Mutex::new(HashMap::new())),
        }
    }

    async fn enter(&self, key: String) -> CacheSingleFlightPermit {
        let mut map = self.inner.lock().await;
        if let Some(notify) = map.get(&key) {
            return CacheSingleFlightPermit::Follower(notify.clone());
        }
        let notify = Arc::new(Notify::new());
        map.insert(key.clone(), notify.clone());
        CacheSingleFlightPermit::Leader(CacheSingleFlightGuard {
            key,
            inner: Arc::clone(&self.inner),
            notify,
        })
    }
}

struct CaptchaPool {
    enabled: bool,
    max_per_type: usize,
    workers: usize,
    slide: Mutex<Vec<(SlideData, CaptchaAnswer)>>, // Slide + SlideRegion
    click: Mutex<Vec<(ClickData, CaptchaAnswer)>>,
    rotate: Mutex<Vec<(RotateData, CaptchaAnswer)>>,
}

enum CaptchaGenerated {
    Slide(SlideData, CaptchaAnswer),
    Click(ClickData, CaptchaAnswer),
    Rotate(RotateData, CaptchaAnswer),
}

impl CaptchaPool {
    fn from_env() -> Self {
        let enabled = match std::env::var("CAPTCHA_POOL_ENABLED") {
            Ok(v) => !matches!(v.trim().to_ascii_lowercase().as_str(), "0" | "false" | "no" | "off"),
            Err(_) => true,
        };

        let max_per_type = std::env::var("CAPTCHA_POOL_PER_TYPE")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or(128)
            .max(1);

        let default_workers = std::thread::available_parallelism()
            .map(|n| (n.get() / 2).max(1))
            .unwrap_or(1);
        let workers = std::env::var("CAPTCHA_POOL_WORKERS")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or(default_workers)
            .max(1);

        Self {
            enabled,
            max_per_type,
            workers,
            slide: Mutex::new(Vec::new()),
            click: Mutex::new(Vec::new()),
            rotate: Mutex::new(Vec::new()),
        }
    }

    fn len(&self, ty: CaptchaType) -> usize {
        match ty {
            CaptchaType::Slide | CaptchaType::SlideRegion => self.slide.lock().len(),
            CaptchaType::Click => self.click.lock().len(),
            CaptchaType::Rotate => self.rotate.lock().len(),
            CaptchaType::JsChallenge => 0,
        }
    }

    fn try_take_slide(&self) -> Option<(SlideData, CaptchaAnswer)> {
        self.slide.lock().pop()
    }

    fn try_take_click(&self) -> Option<(ClickData, CaptchaAnswer)> {
        self.click.lock().pop()
    }

    fn try_take_rotate(&self) -> Option<(RotateData, CaptchaAnswer)> {
        self.rotate.lock().pop()
    }

    fn push_slide(&self, item: (SlideData, CaptchaAnswer)) {
        self.slide.lock().push(item);
    }

    fn push_click(&self, item: (ClickData, CaptchaAnswer)) {
        self.click.lock().push(item);
    }

    fn push_rotate(&self, item: (RotateData, CaptchaAnswer)) {
        self.rotate.lock().push(item);
    }

    fn spawn_fillers(self: Arc<Self>) {
        if !self.enabled {
            return;
        }

        for _ in 0..self.workers {
            let pool = self.clone();
            tokio::spawn(async move {
                loop {
                    // Keep each pool topped up; do not hog the runtime when full.
                    let mut filled_any = false;
                    for ty in [CaptchaType::Slide, CaptchaType::Click, CaptchaType::Rotate] {
                        if pool.len(ty) >= pool.max_per_type {
                            continue;
                        }
                        filled_any = true;
                        let res = tokio::task::spawn_blocking(move || {
                            match ty {
                                CaptchaType::Slide => {
                                    let (data, ans) = captcha::generate_slide_captcha();
                                    CaptchaGenerated::Slide(data, ans)
                                }
                                CaptchaType::Click => {
                                    let (data, ans) = captcha::generate_click_captcha();
                                    CaptchaGenerated::Click(data, ans)
                                }
                                CaptchaType::Rotate => {
                                    let (data, ans) = captcha::generate_rotate_captcha();
                                    CaptchaGenerated::Rotate(data, ans)
                                }
                                _ => unreachable!(),
                            }
                        })
                        .await;

                        if let Ok(generated) = res {
                            match generated {
                                CaptchaGenerated::Slide(data, ans) => pool.push_slide((data, ans)),
                                CaptchaGenerated::Click(data, ans) => pool.push_click((data, ans)),
                                CaptchaGenerated::Rotate(data, ans) => pool.push_rotate((data, ans)),
                            }
                        }
                    }

                    if !filled_any {
                        tokio::time::sleep(Duration::from_millis(50)).await;
                    } else {
                        tokio::task::yield_now().await;
                    }
                }
            });
        }
    }
}

fn strip_host_port(host: &str) -> &str {
	let host = host.trim();
	if host.is_empty() {
		return host;
	}

	// IPv6: [::1]:443 or [::1]
	if let Some(rest) = host.strip_prefix('[') {
		if let Some(end) = rest.find(']') {
			return &rest[..end];
		}
		return host;
	}

	// host:port
	if let Some((h, port)) = host.rsplit_once(':') {
		if !h.is_empty() && !port.is_empty() && port.chars().all(|c| c.is_ascii_digit()) {
			return h;
		}
	}

	host
}

fn extract_port(host: &str) -> Option<u16> {
	let host = host.trim();
	if host.is_empty() {
		return None;
	}
	if let Some(rest) = host.strip_prefix('[') {
		if let Some(end) = rest.find(']') {
			if let Some(port_part) = rest[end + 1..].strip_prefix(':') {
				return port_part.parse::<u16>().ok();
			}
		}
		return None;
	}
	if let Some((_, port)) = host.rsplit_once(':') {
		if !port.is_empty() && port.chars().all(|c| c.is_ascii_digit()) {
			return port.parse::<u16>().ok();
		}
	}
	None
}

fn detect_client_scheme(req: &Request<Incoming>, client_addr: Option<IpAddr>) -> String {
	// X-Forwarded-Proto is only honored when the immediate peer is a trusted
	// proxy (loopback by default; private ranges via env NODE_TRUST_PRIVATE=1).
	// This prevents any Internet client from spoofing the origin scheme via
	// origin_scheme=follow_protocol/follow_both.
	if client_is_trusted(client_addr) {
		if let Some(hv) = req.headers().get("x-forwarded-proto").and_then(|v| v.to_str().ok()) {
			let val = hv.trim().to_lowercase();
			if val == "https" || val == "http" {
				return val;
			}
		}
	}
	if let Some(s) = req.uri().scheme_str() {
		return s.to_string();
	}
	"http".to_string()
}

fn client_is_trusted(client_addr: Option<IpAddr>) -> bool {
	let Some(ip) = client_addr else { return false };
	if ip.is_loopback() {
		return true;
	}
	// Opt-in extension: allow private/link-local ranges when deployed behind
	// an in-cluster LB or reverse proxy. Default is off.
	if std::env::var("NODE_TRUST_PRIVATE").ok().as_deref() == Some("1") {
		match ip {
			IpAddr::V4(v4) => {
				return v4.is_private() || v4.is_link_local();
			}
			IpAddr::V6(v6) => {
				// unique-local fc00::/7 or link-local fe80::/10
				let seg = v6.segments()[0];
				return (seg & 0xfe00) == 0xfc00 || (seg & 0xffc0) == 0xfe80;
			}
		}
	}
	false
}

fn default_port_for_scheme(scheme: &str) -> u16 {
	if scheme.eq_ignore_ascii_case("https") {
		443
	} else {
		80
	}
}

fn resolve_origin_scheme(domain: &DomainConfig, incoming_scheme: &str) -> String {
	match domain
		.origin_scheme
		.as_ref()
		.map(|s| s.as_str())
		.unwrap_or("http")
	{
		"https" => "https".to_string(),
		"follow_protocol" | "follow_both" => incoming_scheme.to_string(),
		_ => "http".to_string(),
	}
}

/// Build the effective `OriginConfig` for a domain, resolving the
/// legacy vs. per-domain-origins source of truth.
///
/// When `domain.origins` carries any enabled entries, they replace the
/// legacy `origins[origin_id].addresses` pool entirely. Selection rule:
///   1. Filter to `enabled=true` only.
///   2. Apply the active health filter (when health-check is enabled)
///      to drop origins that the health checker has marked as down.
///      If the filter would empty the pool we ignore it — this avoids
///      a "no origin available" outage when probes are misconfigured
///      or all backends transiently flap.
///   3. For `load_balance_method = "round_robin"` (default) perform a
///      weighted-random pick across the surviving entries. For
///      `"ip_hash"` we hash the client IP onto a stable index so the
///      same client always lands on the same origin (sticky session).
///   4. The remaining entries are appended in a deterministic order
///      (descending weight, then by original index) so failover walks
///      the same path every request — predictable for debugging.
///
/// When all per-domain entries are disabled (or the list is empty),
/// falls back to `legacy_origin` so domains that predate the refactor
/// continue to work. Timeout/retry settings follow the legacy origin
/// in all cases (those knobs live on the domain, not per-address).
fn resolve_effective_origin(
    domain: &DomainConfig,
    legacy_origin: &OriginConfig,
    client_ip: Option<&str>,
    health: Option<&crate::origin_health::OriginHealthChecker>,
) -> OriginConfig {
    // Only enabled entries participate. Index is preserved so we can
    // use it as a stable tiebreaker below.
    let enabled: Vec<(usize, &crate::config::DomainOriginConfig)> = domain
        .origins
        .iter()
        .enumerate()
        .filter(|(_, e)| e.enabled && !e.address.trim().is_empty())
        .collect();

    if enabled.is_empty() {
        return legacy_origin.clone();
    }

    // Apply health filter. When the filter would empty the pool we keep
    // the unfiltered list so the request still has a chance to succeed
    // against whatever upstream is least-bad — better than returning
    // 502 outright when the probe is misconfigured.
    let filtered: Vec<(usize, &crate::config::DomainOriginConfig)> = if let Some(hc) = health {
        let kept: Vec<(usize, &crate::config::DomainOriginConfig)> = enabled
            .iter()
            .copied()
            .filter(|(_, e)| hc.is_healthy(domain.id.as_str(), e.address.as_str()))
            .collect();
        if kept.is_empty() {
            enabled
        } else {
            kept
        }
    } else {
        enabled
    };

    let method = domain.load_balance_method.as_str();
    let picked_idx = if method.eq_ignore_ascii_case("ip_hash") {
        // Stable hash on client IP. None / empty falls back to 0 so a
        // request without a peer addr (e.g. internal probes) still
        // resolves deterministically rather than panicking.
        use std::hash::{Hash, Hasher};
        let mut hasher = std::collections::hash_map::DefaultHasher::new();
        client_ip.unwrap_or("").hash(&mut hasher);
        let h = hasher.finish() as usize;
        h % filtered.len()
    } else {
        // Weighted-random pick (default round_robin). Weights are
        // clamped by the control plane to 1..=100; defend against
        // zero/negative drift at the node edge.
        let weights: Vec<u32> = filtered
            .iter()
            .map(|(_, e)| e.weight.max(1) as u32)
            .collect();
        let total: u32 = weights.iter().sum();
        let mut picked = 0usize;
        if total > 0 {
            let mut rng = thread_rng();
            let roll: u32 = rng.gen_range(0..total);
            let mut acc = 0u32;
            for (i, w) in weights.iter().enumerate() {
                acc += *w;
                if roll < acc {
                    picked = i;
                    break;
                }
            }
        }
        picked
    };

    // Primary address first, then the rest ordered by (weight desc,
    // original index asc). Callers iterate addresses top-down for
    // failover, so this places heavier backups ahead of lighter ones
    // after the primary pick.
    let mut addresses: Vec<String> = Vec::with_capacity(filtered.len());
    addresses.push(filtered[picked_idx].1.address.clone());

    let mut rest: Vec<&(usize, &crate::config::DomainOriginConfig)> = filtered
        .iter()
        .enumerate()
        .filter(|(i, _)| *i != picked_idx)
        .map(|(_, v)| v)
        .collect();
    rest.sort_by(|a, b| {
        b.1.weight.cmp(&a.1.weight).then(a.0.cmp(&b.0))
    });
    for (_, e) in rest {
        addresses.push(e.address.clone());
    }

    OriginConfig {
        id: legacy_origin.id.clone(),
        name: legacy_origin.name.clone(),
        addresses,
        timeout_ms: legacy_origin.timeout_ms,
        max_retries: legacy_origin.max_retries,
    }
}

/// Heuristic reconciliation for scheme/port mismatch (inspired by EdgeNode
/// http_request_reverse_proxy.go:293-297). When the configured scheme doesn't
/// match the well-known port, the origin is almost certainly on the other
/// protocol and the mismatch would cause a 400 (e.g. "plain HTTP request was
/// sent to HTTPS port"). We auto-correct to avoid the most common misconfig.
///
/// Only promote to https when the configured port is 443, and demote to http
/// when the configured port is 80. Leave everything else alone so operators
/// with genuinely custom deployments aren't surprised.
fn reconcile_scheme_port(scheme: &mut String, port: u16) {
	match (scheme.as_str(), port) {
		("http", 443) => {
			*scheme = "https".to_string();
		}
		("https", 80) => {
			*scheme = "http".to_string();
		}
		_ => {}
	}
}

/// Normalize a Host header value for outbound (origin-facing) use.
///
/// Defends against three common sources of origin 400s:
///   1. Trailing dot: "example.com." -> "example.com"
///   2. Mixed case: "Example.COM" -> "example.com" (nginx server_name is
///      compared case-insensitively but some strict vhost configs are not)
///   3. Default port suffix: "example.com:80" with scheme=http -> "example.com"
///      (many nginx/Apache vhosts match `server_name example.com;` exactly and
///      return 400 when Host includes the port)
///
/// Non-default ports are preserved because some origins (object storage,
/// inner services) actually need the port in Host.
fn normalize_origin_host_header(host: &str, scheme: &str) -> String {
	let trimmed = host.trim().trim_end_matches('.');
	if trimmed.is_empty() {
		return String::new();
	}
	let lowered = trimmed.to_ascii_lowercase();
	// Strip IPv6 brackets + port via existing helper; otherwise split host:port.
	if let Some(rest) = lowered.strip_prefix('[') {
		// IPv6: "[::1]:80" -> keep brackets but strip standard port
		if let Some(end) = rest.find(']') {
			let host_part = &lowered[..=end + 0]; // include ']'
			let after = &lowered[end + 1..];
			if let Some(port_str) = after.strip_prefix(":") {
				if (scheme == "http" && port_str == "80")
					|| (scheme == "https" && port_str == "443")
				{
					// strip default port: keep "[::1]"
					let mut s = String::new();
					s.push('[');
					s.push_str(&rest[..end]);
					s.push(']');
					return s;
				}
			}
			return format!("{}{}", host_part, after);
		}
		return lowered;
	}
	if let Some((h, port)) = lowered.rsplit_once(':') {
		if !h.is_empty() && !port.is_empty() && port.chars().all(|c| c.is_ascii_digit()) {
			if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
				return h.to_string();
			}
		}
	}
	lowered
}

/// Ensure the outbound path begins with `/`. nginx rejects request lines that
/// don't start with `/` or an absolute URI with a 400, so a path produced by
/// overzealous stripPrefix/rewrite logic must be repaired before send.
fn ensure_leading_slash(path: &str) -> String {
	if path.is_empty() {
		return "/".to_string();
	}
	if path.starts_with('/') || path.starts_with("http://") || path.starts_with("https://") {
		return path.to_string();
	}
	format!("/{}", path)
}

/// Inject the usual forwarding / proxy headers into an outbound origin
/// request. Idempotent in spirit: we only set the Host header here (always
/// overwrite) and append X-Forwarded-For; existing X-Real-IP / X-Forwarded-*
/// headers from the client are preserved to support multi-tier proxy chains.
///
/// `client_ip` may be `None` when the peer socket address is unavailable.
/// `incoming_scheme` should be the scheme the client used to reach the edge
/// ("http" or "https"), not the origin scheme.
fn apply_forward_headers(
	headers: &mut http::HeaderMap,
	host_header: &str,
	client_ip: Option<IpAddr>,
	client_host: &str,
	incoming_scheme: &str,
) {
	// Host — always enforce the resolved origin Host. Without this, hyper
	// derives Host from the outbound URL (= origin IP:port), which fails
	// nginx vhost matching and typically returns 400/404.
	if !host_header.is_empty() {
		if let Ok(hv) = http::HeaderValue::from_str(host_header) {
			headers.insert(http::header::HOST, hv);
		}
	}

	// X-Real-IP — preserve existing value (multi-tier chain); only set when
	// absent.
	if let Some(ip) = client_ip {
		let has_real_ip = headers
			.keys()
			.any(|k| k.as_str().eq_ignore_ascii_case("x-real-ip"));
		if !has_real_ip {
			if let Ok(hv) = http::HeaderValue::from_str(&ip.to_string()) {
				headers.insert("x-real-ip", hv);
			}
		}
	}

	// X-Forwarded-For — append client IP to existing chain, capped to 16
	// entries to prevent header bloat from malicious clients (a common cause
	// of origin 400s on strict servers that limit header size).
	if let Some(ip) = client_ip {
		let client_ip_str = ip.to_string();
		let cap: usize = std::env::var("FORWARDED_FOR_MAX_ENTRIES")
			.ok()
			.and_then(|v| v.trim().parse::<usize>().ok())
			.unwrap_or(16)
			.max(1);
		let existing = headers
			.get(http::header::FORWARDED)
			.and_then(|v| v.to_str().ok())
			.map(|s| s.to_string());
		// Note: hyper normalizes header names to lowercase; use the typed
		// constant for X-Forwarded-For.
		let existing_xff = headers
			.get("x-forwarded-for")
			.and_then(|v| v.to_str().ok())
			.map(|s| s.to_string());
		let new_xff = match existing_xff {
			Some(chain) => {
				let mut parts: Vec<String> = chain
					.split(',')
					.map(|s| s.trim().to_string())
					.filter(|s| !s.is_empty())
					.collect();
				parts.push(client_ip_str);
				if parts.len() > cap {
					let drop = parts.len() - cap;
					parts.drain(0..drop);
				}
				parts.join(", ")
			}
			None => client_ip_str,
		};
		if let Ok(hv) = http::HeaderValue::from_str(&new_xff) {
			headers.insert("x-forwarded-for", hv);
		}
		let _ = existing; // Forwarded header passthrough unchanged
	}

	// X-Forwarded-Host — original Host the client used to reach the edge.
	if !client_host.is_empty() {
		let has = headers
			.keys()
			.any(|k| k.as_str().eq_ignore_ascii_case("x-forwarded-host"));
		if !has {
			if let Ok(hv) = http::HeaderValue::from_str(client_host) {
				headers.insert("x-forwarded-host", hv);
			}
		}
	}

	// X-Forwarded-Proto — scheme the client used to reach the edge.
	if !incoming_scheme.is_empty() {
		let has = headers
			.keys()
			.any(|k| k.as_str().eq_ignore_ascii_case("x-forwarded-proto"));
		if !has {
			if let Ok(hv) = http::HeaderValue::from_str(incoming_scheme) {
				headers.insert("x-forwarded-proto", hv);
			}
		}
	}

	// Normalize Connection: "close" -> "keep-alive" so upstream can pool
	// connections (EdgeNode http_request.go:1667-1669). Edge cases where
	// the client legitimately wants close are rare and do not justify
	// constantly spinning up new origin connections.
	if let Some(v) = headers.get(http::header::CONNECTION) {
		if v.as_bytes().eq_ignore_ascii_case(b"close") {
			headers.insert(
				http::header::CONNECTION,
				http::HeaderValue::from_static("keep-alive"),
			);
		}
	}
}

fn apply_origin_auth(headers: &mut http::HeaderMap, auth: &OriginAuthConfig) {
	if !auth.enabled {
		return;
	}
	match auth.mode.as_deref().unwrap_or("header") {
		"basic" => {
			let user = auth.basic_user.as_deref().unwrap_or("");
			let pass = auth.basic_pass.as_deref().unwrap_or("");
			let encoded = general_purpose::STANDARD.encode(format!("{}:{}", user, pass));
			if let Ok(hv) = http::HeaderValue::from_str(&format!("Basic {}", encoded)) {
				headers.insert(http::header::AUTHORIZATION, hv);
			}
		}
		_ => {
			for h in &auth.headers {
				let name = h.name.trim();
				if name.is_empty() {
					continue;
				}
				if let (Ok(hn), Ok(hv)) = (
					http::header::HeaderName::from_bytes(name.as_bytes()),
					http::HeaderValue::from_str(&h.value),
				) {
					headers.insert(hn, hv);
				}
			}
		}
	}
}

fn resolve_origin_port(domain: &DomainConfig, incoming_port: Option<u16>, scheme: &str) -> u16 {
	let mut port = domain.origin_port.unwrap_or(0);
	let mode = domain
		.origin_scheme
		.as_ref()
		.map(|s| s.as_str())
		.unwrap_or("http");
	if matches!(mode, "follow_port" | "follow_both") {
		if let Some(p) = incoming_port {
			port = p as i32;
		}
	}
	if port <= 0 {
		return default_port_for_scheme(scheme);
	}
	port as u16
}

fn format_host_with_port(host: &str, port: u16) -> String {
	if host.contains(':') && !host.starts_with('[') {
		format!("[{}]:{}", host, port)
	} else if host.starts_with('[') {
		format!("{}:{}", host, port)
	} else {
		format!("{}:{}", host, port)
	}
}

fn resolve_origin_host_header(
	domain: &DomainConfig,
	original_host_header: &str,
	origin_scheme: &str,
) -> String {
	let mode = domain
		.origin_host_mode
		.as_ref()
		.map(|s| s.as_str())
		.unwrap_or("request_host");
	// Callers that pass an empty Host header (HTTP/1.0 without Host, or a
	// stripped-port edge case) would otherwise end up with hyper auto-deriving
	// the Host from the outbound URL (= origin IP:port), which lands on the
	// wrong vhost for almost every origin. Fall back to the configured domain
	// name in all modes so the origin sees a sane Host.
	let fallback = domain.name.as_str();
	let raw = match mode {
		"custom" => {
			let custom = domain
				.origin_host
				.as_ref()
				.map(|s| s.as_str())
				.unwrap_or("");
			if !custom.is_empty() {
				custom.to_string()
			} else if !original_host_header.is_empty() {
				original_host_header.to_string()
			} else {
				fallback.to_string()
			}
		}
		"request_host_port" => {
			// Keep port even if it's the default for parity with the original
			// behavior — some origins rely on this. We still lowercase / trim.
			if !original_host_header.is_empty() {
				let t = original_host_header.trim().trim_end_matches('.');
				return t.to_ascii_lowercase();
			}
			fallback.to_string()
		}
		_ => {
			let stripped = strip_host_port(original_host_header);
			if !stripped.is_empty() {
				stripped.to_string()
			} else {
				fallback.to_string()
			}
		}
	};
	// Final normalization: lowercase, trim trailing dot, strip default port
	// for the actual origin scheme (not the inbound scheme).
	normalize_origin_host_header(&raw, origin_scheme)
}

/// 拉黑事件，用于实时上报到主控
#[derive(Debug, Clone)]
pub struct BanEvent {
    pub ip: String,
    pub reason: String,
    pub strikes: u32,
    pub expires_at_unix: i64,
}

pub struct ProxyService {
    client: Client<HttpsConnector<HttpConnector>, NodeBody>,
    cache: Arc<Cache>,
    config_holder: Arc<ConfigHolder>,
    metrics: Arc<Metrics>,
    access_logger: Option<AccessLogger>,
    geoip_holder: Arc<GeoIpHolder>,
    node_id: Option<String>,
    node_hostname: String,
    max_request_body_bytes: u64,
    max_response_body_bytes: u64,
    max_cache_object_bytes: u64,
    challenge_secret: [u8; 32],
    waf_sweep_last: parking_lot::Mutex<Instant>,
    cache_query_ignorer: CacheQueryIgnore,
    cache_singleflight: Arc<CacheSingleFlight>,
    cache_singleflight_enabled: bool,
    cache_singleflight_wait: Duration,
    cache_stale_if_error_secs: u64,
    cache_negative_ttl_404_secs: u64,
    cache_negative_ttl_410_secs: u64,
    waf_counters: Arc<ShardedStringMap<WafDomainStat>>,
    waf_fail: Arc<ShardedStringMap<FailState>>,
    waf_ban: Arc<ShardedStringMap<BanState>>,
    waf_rate_limits: Arc<ShardedStringMap<RateLimitEntry>>,
    // 行为验证码会话存储
    captcha_sessions: Arc<ShardedStringMap<CaptchaSession>>,
    captcha_pool: Arc<CaptchaPool>,
    origin_inflight: Option<Arc<Semaphore>>,
    origin_inflight_acquire_timeout: Option<Duration>,
    // 拉黑事件发送通道
    ban_event_tx: Option<mpsc::UnboundedSender<BanEvent>>,
    // 源站主动健康检查器（可选）。当 None 时回退为不做主动探测、
    // 仅依赖请求阶段的 transport-error failover；当 Some 时
    // resolve_effective_origin 会过滤掉被标记不健康的 origin。
    origin_health: Option<Arc<crate::origin_health::OriginHealthChecker>>,
}

impl ProxyService {
    pub fn new(
        cache: Arc<Cache>,
        config_holder: Arc<ConfigHolder>,
        metrics: Arc<Metrics>,
        access_logger: Option<AccessLogger>,
        geoip_holder: Arc<GeoIpHolder>,
        node_id: Option<String>,
        node_hostname: String,
        max_request_body_bytes: u64,
        max_response_body_bytes: u64,
        max_cache_object_bytes: u64,
        ban_event_tx: Option<mpsc::UnboundedSender<BanEvent>>,
        origin_health: Option<Arc<crate::origin_health::OriginHealthChecker>>,
    ) -> Self {
        let pool_idle_timeout_secs = std::env::var("ORIGIN_POOL_IDLE_TIMEOUT_SECS")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .unwrap_or(90)
            .max(1);
        let default_max_idle = std::thread::available_parallelism()
            .map(|n| (n.get() * 32).clamp(32, 256))
            .unwrap_or(64);
        let pool_max_idle_per_host = std::env::var("ORIGIN_POOL_MAX_IDLE_PER_HOST")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or(default_max_idle)
            .max(1);

        let mut http_connector = HttpConnector::new();
        http_connector.enforce_http(false);
        let https_connector = hyper_rustls::HttpsConnectorBuilder::new()
            .with_webpki_roots()
            .https_or_http()
            .enable_http1()
            .enable_http2()
            .wrap_connector(http_connector);

        let client: Client<HttpsConnector<HttpConnector>, NodeBody> = Client::builder(TokioExecutor::new())
            .pool_idle_timeout(Duration::from_secs(pool_idle_timeout_secs))
            .pool_max_idle_per_host(pool_max_idle_per_host)
            .build(https_connector);

        let captcha_pool = Arc::new(CaptchaPool::from_env());
        captcha_pool.clone().spawn_fillers();

        let default_origin_max_inflight = std::thread::available_parallelism()
            .map(|n| (n.get() * 512).clamp(512, 8192))
            .unwrap_or(1024);
        let origin_max_inflight = std::env::var("ORIGIN_MAX_INFLIGHT")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or(default_origin_max_inflight);
        let origin_inflight = if origin_max_inflight > 0 {
            Some(Arc::new(Semaphore::new(origin_max_inflight)))
        } else {
            None
        };
        let origin_inflight_acquire_timeout = std::env::var("ORIGIN_MAX_INFLIGHT_WAIT_MS")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .and_then(|ms| if ms == 0 { None } else { Some(Duration::from_millis(ms)) });

        let cache_singleflight_enabled = match std::env::var("CACHE_SINGLEFLIGHT_ENABLED") {
            Ok(v) => !matches!(v.trim().to_ascii_lowercase().as_str(), "0" | "false" | "no" | "off"),
            Err(_) => true,
        };
        let cache_singleflight_wait = std::env::var("CACHE_SINGLEFLIGHT_WAIT_MS")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .map(Duration::from_millis)
            .unwrap_or(Duration::from_millis(2000));
        let cache_stale_if_error_secs = std::env::var("CACHE_STALE_IF_ERROR_SECS")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .unwrap_or(60);
        let cache_negative_ttl_404_secs = std::env::var("CACHE_NEGATIVE_TTL_404_SECS")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .unwrap_or(30);
        let cache_negative_ttl_410_secs = std::env::var("CACHE_NEGATIVE_TTL_410_SECS")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .unwrap_or(30);

        Self {
            client,
            cache,
            config_holder,
            metrics,
            access_logger,
            geoip_holder,
            node_id,
            node_hostname,
            max_request_body_bytes,
            max_response_body_bytes,
            max_cache_object_bytes,
            challenge_secret: rand::random(),
            waf_sweep_last: parking_lot::Mutex::new(Instant::now()),
            cache_query_ignorer: CacheQueryIgnore::from_env(),
            cache_singleflight: Arc::new(CacheSingleFlight::new()),
            cache_singleflight_enabled,
            cache_singleflight_wait,
            cache_stale_if_error_secs,
            cache_negative_ttl_404_secs,
            cache_negative_ttl_410_secs,
            waf_counters: Arc::new(ShardedStringMap::new()),
            waf_fail: Arc::new(ShardedStringMap::new()),
            waf_ban: Arc::new(ShardedStringMap::new()),
            waf_rate_limits: Arc::new(ShardedStringMap::new()),
            captcha_sessions: Arc::new(ShardedStringMap::new()),
            captcha_pool,
            origin_inflight,
            origin_inflight_acquire_timeout,
            ban_event_tx,
            origin_health,
        }
    }

    fn maybe_sweep_waf_state(&self) {
        const SWEEP_INTERVAL: Duration = Duration::from_secs(1);
        let now = Instant::now();
        let should_sweep = {
            let mut last = self.waf_sweep_last.lock();
            if now.duration_since(*last) < SWEEP_INTERVAL {
                false
            } else {
                *last = now;
                true
            }
        };
        if should_sweep {
            self.sweep_waf_state();
            self.sweep_captcha_sessions();
        }
    }

    fn full_response(status: StatusCode, body: Bytes, content_type: &str) -> NodeResponse {
        let b = Full::new(body).map_err(|e| match e {}).boxed();
        let builder = hyper::Response::builder()
            .status(status)
            .header(CONTENT_TYPE, content_type);
        build_response_or_fallback(builder, b, status, "internal response build error")
    }

    pub async fn handle_request(
        &self,
        req: Request<Incoming>,
    ) -> Result<NodeResponse> {
        let started = std::time::Instant::now();
        let client_addr: Option<IpAddr> = req.extensions().get::<std::net::SocketAddr>().map(|addr| addr.ip());
        let uri = req.uri().clone();
        let host_header = req
            .headers()
            .get("host")
            .and_then(|h| h.to_str().ok())
            .unwrap_or("")
            .to_string();
        let host_header_str = host_header.as_str();
        let host = strip_host_port(host_header_str).to_string();
        let host_cache = host.to_ascii_lowercase();
        let host_str = host.as_str();
        let incoming_port = extract_port(host_header_str);
        let incoming_scheme = detect_client_scheme(&req, client_addr);
        let path = uri.path();
        let method = req.method().clone();
        let method_str = method.as_str();

        debug!("Handling request: {} {} {}", method_str, host_str, path);

        // 处理验证码验证请求
        if let Some(query) = uri.query() {
            if query.contains("_waf_verify=1") && method == Method::POST {
                let client_ip = client_addr.map(|ip| ip.to_string());
                return self
                    .handle_captcha_verify(req, client_ip.as_deref(), &started, host_str, &uri)
                    .await;
            }
        }

        let Some(state) = self.config_holder.get_state() else {
            let resp = Self::full_response(
                StatusCode::SERVICE_UNAVAILABLE,
                Bytes::from("No Config"),
                "text/plain; charset=utf-8",
            );
            self.log_access(&started, client_addr, host_str, &uri, method_str, resp.status().as_u16(), "BYPASS", 0, "no_config");
            return Ok(resp);
        };
        let config = state.config.clone();
        let router = state.router.clone();

        // Route the request. Compute once; avoid cloning Accept per request.
        let accept = req
            .headers()
            .get(ACCEPT)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("");
        let accept_json = accept.contains("application/json") || accept.contains("json");

        // 授权过期：所有请求直接返回提示页面（样式同 502，但文案不同）
        if let Some(license) = &config.license {
            let status = license.status.trim().to_ascii_lowercase();
            let now = chrono::Utc::now().timestamp();
            let expired = status == "expired"
                || (status == "active" && license.expires_at_unix > 0 && now > license.expires_at_unix);
            if expired {
                let resp = build_friendly_error(
                    StatusCode::SERVICE_UNAVAILABLE,
                    "系统授权已到期",
                    "系统授权已到期，节点已暂停对外服务，请联系管理员续费或更新授权。",
                    host_str,
                    path,
                    accept_json,
                    None,
                    Some(&config.templates),
                );
                self.log_access(&started, client_addr, host_str, &uri, method_str, resp.status().as_u16(), "BYPASS", 0, "license_expired");
                return Ok(resp);
            }
        }

        let route_match = match router.route(host_cache.as_str(), path, method_str) {
            Some(m) => m,
            None => {
                warn!("No route found for: {} {}", host_str, path);
                let resp = build_unmatched_host_page(
                    host_str,
                    path,
                    accept_json,
                    &config.templates,
                );
                let log_tag = if host_str.parse::<std::net::IpAddr>().is_ok() {
                    "direct_ip_access"
                } else {
                    "route_not_found"
                };
                self.log_access(&started, client_addr, host_str, &uri, method_str, resp.status().as_u16(), "BYPASS", 0, log_tag);
                return Ok(resp);
            }
        };

        // Avoid cloning DomainConfig per request; keep a reference into the runtime config.
        let domain = match config.domains.get(route_match.domain_idx) {
            Some(d) => d,
            None => {
                error!("Domain index out of range: idx={}", route_match.domain_idx);
                let resp = build_friendly_error(
                    StatusCode::NOT_FOUND,
                    "站点不可用",
                    "请求的域名配置异常，请联系管理员",
                    host_str,
                    path,
                    accept_json,
                    None,
                    Some(&config.templates),
                );
                self.log_access(&started, client_addr, host_str, &uri, method_str, resp.status().as_u16(), "BYPASS", 0, "domain_idx_oob");
                return Ok(resp);
            }
        };

        let http2_enabled = domain.http2_enabled.unwrap_or(true);
        if req.version() == http::Version::HTTP_2 && !http2_enabled {
            let resp = build_friendly_error(
                StatusCode::UPGRADE_REQUIRED,
                "HTTP/2 未开启",
                "该域名未开启 HTTP/2，请使用 HTTP/1.1 访问或在控制台开启 HTTP/2。",
                host_str,
                path,
                accept_json,
                Some(&domain.error_pages),
                Some(&config.templates),
            );
            self.log_access(&started, client_addr, host_str, &uri, method_str, resp.status().as_u16(), "BYPASS", 0, "http2_disabled");
            return Ok(resp);
        }

        let cache_enabled = domain.cache_enabled.unwrap_or(true);
        // Only evaluate cache rules for methods that can be cached.
        let cache_decision = if cache_enabled && method == Method::GET {
            router.cache_decision(host_cache.as_str(), path, method_str)
        } else {
            None
        };
        let cache_ttl = cache_decision.map(|d| d.ttl_seconds);
        let cache_key: Option<CacheKey> = if method == Method::GET && cache_ttl.is_some() {
            let include_query = cache_decision
                .as_ref()
                .map(|d| d.cache_query_params)
                .unwrap_or(true);
            Some(CacheKey::new(
                host_cache.clone(),
                path.to_string(),
                if include_query {
                    self.cache_query_ignorer.normalize(req.uri().query())
                } else {
                    None
                },
            ))
        } else {
            None
        };

        let mut cache_status = "BYPASS";

        // Try cache first (only for GET requests + cache rule enabled)
        if let Some(cache_key) = cache_key.as_ref() {
            if let Some(entry) = self.cache.get_async(cache_key).await {
                debug!("Serving from cache: {}", cache_key.to_string());
                self.metrics.cache_hits.inc();
                cache_status = "HIT";
                let bytes = entry.body_len();
                self.metrics.bytes_sent.inc_by(bytes);
                let resp = self.build_response_from_cache(entry).await?;
                self.log_access(
                    &started,
                    client_addr,
                    host_str,
                    &uri,
                    method_str,
                    resp.status().as_u16(),
                    cache_status,
                    bytes,
                    "",
                );
                return Ok(resp);
            }
        }
        if method == Method::GET && cache_ttl.is_some() {
            self.metrics.cache_misses.inc();
            cache_status = "MISS";
        }

        // Cache singleflight: wait for in-flight fill of the same key before going to origin.
        let mut cache_flight_guard: Option<CacheSingleFlightGuard> = None;
        if self.cache_singleflight_enabled {
            if let Some(cache_key) = cache_key.as_ref() {
                let key_str = cache_key.to_string();
                match self.cache_singleflight.enter(key_str).await {
                    CacheSingleFlightPermit::Leader(guard) => {
                        cache_flight_guard = Some(guard);
                    }
                    CacheSingleFlightPermit::Follower(notify) => {
                        let _ = tokio::time::timeout(self.cache_singleflight_wait, notify.notified()).await;
                        if let Some(entry) = self.cache.get_async(cache_key).await {
                            debug!("Serving from cache after wait: {}", cache_key.to_string());
                            self.metrics.cache_hits.inc();
                            cache_status = "HIT";
                            let bytes = entry.body_len();
                            self.metrics.bytes_sent.inc_by(bytes);
                            let resp = self.build_response_from_cache(entry).await?;
                            self.log_access(
                                &started,
                                client_addr,
                                host_str,
                                &uri,
                                method_str,
                                resp.status().as_u16(),
                                cache_status,
                                bytes,
                                "",
                            );
                            return Ok(resp);
                        }
                    }
                }
            }
        }

        // Prepare stale entry for stale-if-error fallback.
        let mut stale_entry: Option<CacheEntry> = None;
        if self.cache_stale_if_error_secs > 0 {
            if let Some(cache_key) = cache_key.as_ref() {
                if let Some((entry, is_stale)) = self
                    .cache
                    .get_async_with_stale(cache_key, self.cache_stale_if_error_secs)
                    .await
                {
                    if !is_stale {
                        debug!("Serving from cache (late hit): {}", cache_key.to_string());
                        self.metrics.cache_hits.inc();
                        cache_status = "HIT";
                        let bytes = entry.body_len();
                        self.metrics.bytes_sent.inc_by(bytes);
                        let resp = self.build_response_from_cache(entry).await?;
                        self.log_access(
                            &started,
                            client_addr,
                            host_str,
                            &uri,
                            method_str,
                            resp.status().as_u16(),
                            cache_status,
                            bytes,
                            "",
                        );
                        return Ok(resp);
                    }
                    stale_entry = Some(entry);
                }
            }
        }

        // Get origin config
        // Resolve the effective origin. Preference order:
        //   1. Per-domain origins (domain.origins) — new model, used
        //      when any enabled entry exists.
        //   2. Legacy global origin pool via domain.origin_id — kept
        //      for domains migrated before the refactor.
        // When both are absent we return 502 with a friendly page.
        let legacy_origin = config.origins.get(domain.origin_id.as_str());
        let has_domain_origins = domain.origins.iter().any(|e| e.enabled && !e.address.trim().is_empty());
        let origin_owned: OriginConfig = if has_domain_origins {
            // Use per-domain origins. Fabricate a minimal OriginConfig
            // envelope with sensible defaults when no legacy row
            // exists to copy timeout/retry from.
            let fallback = OriginConfig {
                id: domain.origin_id.clone(),
                name: format!("{}-origin", domain.name),
                addresses: Vec::new(),
                timeout_ms: domain.origin_timeout_ms.unwrap_or(60_000) as u64,
                max_retries: 3,
            };
            let base = legacy_origin.cloned().unwrap_or(fallback);
            // client_addr was stashed earlier by the listener; format
            // once and reuse for ip_hash so we don't pay the
            // allocation when the domain uses round_robin.
            let client_ip_owned = client_addr.map(|ip| ip.to_string());
            resolve_effective_origin(
                domain,
                &base,
                client_ip_owned.as_deref(),
                self.origin_health.as_deref(),
            )
        } else if let Some(o) = legacy_origin {
            o.clone()
        } else {
            error!("Origin not found: domain={} origin_id={}", domain.name, domain.origin_id);
            let resp = build_friendly_error(
                StatusCode::BAD_GATEWAY,
                "源站不可用",
                "未找到对应的源站配置，请在域名详情页配置回源地址。",
                host_str,
                path,
                accept_json,
                Some(&domain.error_pages),
                Some(&config.templates),
            );
            self.log_access(&started, client_addr, host_str, &uri, method_str, resp.status().as_u16(), cache_status, 0, "origin_not_found");
            return Ok(resp);
        };
        let origin = &origin_owned;

        // WAF check (avoid formatting client IP unless WAF is enabled).
        let client_ip_str: Option<String> = if !config.waf_policies.is_empty() {
            client_addr.map(|ip| ip.to_string())
        } else {
            None
        };
        if let Some(resp) = self.enforce_waf_compiled(
            state.waf.as_ref(),
            !config.waf_policies.is_empty(),
            domain.id.as_str(),
            host_str,
            path,
            method_str,
            client_ip_str.as_deref(),
            client_addr,
            req.headers(),
        ) {
            let status = resp.status().as_u16();
            self.log_access(
                &started,
                client_addr,
                host_str,
                &uri,
                method_str,
                status,
                "BYPASS",
                0,
                "waf_block",
            );
            return Ok(resp);
        }

        // Proxy to origin
        let _cache_flight_guard = cache_flight_guard;
        match self
            .proxy_to_origin(
                req,
                domain,
                origin,
                &incoming_scheme,
                incoming_port,
                host_header_str,
                accept_json,
                cache_key.as_ref(),
                cache_ttl,
            )
            .await
        {
            Ok(response) => {
                let status = response.status().as_u16();
                if status >= 500 {
                    if let Some(entry) = stale_entry.take() {
                        let bytes = entry.body_len();
                        let resp = self.build_response_from_cache(entry).await?;
                        self.log_access(
                            &started,
                            client_addr,
                            host_str,
                            &uri,
                            method_str,
                            resp.status().as_u16(),
                            "STALE",
                            bytes,
                            "",
                        );
                        return Ok(resp);
                    }
                }
                // Replace bare origin error pages (400/403/404/5xx) with the
                // domain's configured friendly error page when one exists.
                // This avoids exposing raw nginx/Apache error pages like the
                // 150-byte "400 Bad Request - nginx" page, which is very
                // confusing for end users and leaks origin implementation
                // details. Opt-out via ORIGIN_FRIENDLY_ERROR_PAGES=0.
                let replace_friendly = std::env::var("ORIGIN_FRIENDLY_ERROR_PAGES")
                    .map(|v| !matches!(v.trim(), "0" | "false" | "no" | "off"))
                    .unwrap_or(true);
                if replace_friendly && status >= 400 {
                    let has_custom = domain
                        .error_pages
                        .iter()
                        .any(|p| p.status as u16 == status);
                    if has_custom {
                        let resp = build_friendly_error(
                            response.status(),
                            "上游响应异常",
                            "源站返回了异常状态，已为您展示预设的错误页面。",
                            host_str,
                            path,
                            accept_json,
                            Some(&domain.error_pages),
                            Some(&config.templates),
                        );
                        self.log_access(
                            &started,
                            client_addr,
                            host_str,
                            &uri,
                            method_str,
                            resp.status().as_u16(),
                            cache_status,
                            0,
                            "origin_error_page_replaced",
                        );
                        return Ok(resp);
                    }
                }
                let bytes = response
                    .headers()
                    .get(http::header::CONTENT_LENGTH)
                    .and_then(|v| v.to_str().ok())
                    .and_then(|s| s.parse::<u64>().ok())
                    .unwrap_or(0);
                self.log_access(
                    &started,
                    client_addr,
                    host_str,
                    &uri,
                    method_str,
                    status,
                    cache_status,
                    bytes,
                    "",
                );
                Ok(response)
            }
            Err(e) => {
                if let Some(entry) = stale_entry.take() {
                    let bytes = entry.body_len();
                    let resp = self.build_response_from_cache(entry).await?;
                    self.log_access(
                        &started,
                        client_addr,
                        host_str,
                        &uri,
                        method_str,
                        resp.status().as_u16(),
                        "STALE",
                        bytes,
                        "origin_error",
                    );
                    return Ok(resp);
                }
                self.metrics.origin_errors.inc();
                error!("Proxy error: {}", e);
                let resp = build_friendly_error(
                    StatusCode::BAD_GATEWAY,
                    "回源失败",
                    "连接源站出错或超时，请稍后重试。",
                    host_str,
                    path,
                    accept_json,
                    Some(&domain.error_pages),
                    Some(&config.templates),
                );
                self.log_access(&started, client_addr, host_str, &uri, method_str, resp.status().as_u16(), cache_status, 0, "proxy_error");
                Ok(resp)
            }
        }
    }

    fn log_access(
        &self,
        started: &std::time::Instant,
        client_ip: Option<IpAddr>,
        domain: &str,
        uri: &Uri,
        method: &str,
        status: u16,
        cache_status: &str,
        bytes: u64,
        err: &str,
    ) {
        let Some(logger) = self.access_logger.as_ref() else { return };

        #[derive(Serialize)]
        struct AccessLog<'a> {
            #[serde(rename = "@timestamp")]
            ts: String,
            domain: &'a str,
            path: &'a str,
            method: &'a str,
            status: u16,
            bytes: u64,
            cache_status: &'a str,
            duration_ms: u64,
            node_id: &'a str,
            node: &'a str,
            #[serde(skip_serializing_if = "Option::is_none")]
            client_ip: Option<String>,
            #[serde(skip_serializing_if = "Option::is_none")]
            location: Option<String>,
            #[serde(skip_serializing_if = "Option::is_none")]
            error: Option<&'a str>,
        }

        let duration_ms = started.elapsed().as_millis() as u64;
        if !logger.should_log(status, duration_ms, !err.is_empty()) {
            return;
        }

        let client_ip = client_ip.map(|ip| ip.to_string());
        let ts = Utc::now().to_rfc3339_opts(SecondsFormat::Millis, true);
        let path = uri
            .path_and_query()
            .map(|p| p.as_str())
            .unwrap_or(uri.path());
        let node_id = self.node_id.as_deref().unwrap_or("");
        let error = if err.is_empty() { None } else { Some(err) };
        let location = client_ip
            .as_deref()
            .and_then(|ip| self.geoip_holder.get().and_then(|r| r.lookup(ip)));

        let entry = AccessLog {
            ts,
            domain,
            path,
            method,
            status,
            bytes,
            cache_status,
            duration_ms,
            node_id,
            node: &self.node_hostname,
            client_ip,
            location,
            error,
        };
        logger.log(&entry);
    }

    async fn proxy_to_origin(
        &self,
        req: Request<Incoming>,
        domain: &DomainConfig,
        origin: &OriginConfig,
        incoming_scheme: &str,
        incoming_port: Option<u16>,
        original_host_header: &str,
        accept_json: bool,
        cache_key: Option<&CacheKey>,
        cache_ttl: Option<u64>,
    ) -> Result<NodeResponse> {
        let start = std::time::Instant::now();
        if origin.addresses.is_empty() {
            return Err(anyhow::anyhow!("No origin addresses available"));
        }
        // Client address was stashed onto the request by the listener
        // (listener.rs inserts SocketAddr into extensions). Pull it out early
        // so we can feed it into X-Forwarded-For on the outbound request.
        let client_ip: Option<IpAddr> = req
            .extensions()
            .get::<std::net::SocketAddr>()
            .map(|a| a.ip());
        // The unstripped client-provided Host header is used for
        // X-Forwarded-Host so the origin can see what the end user asked for,
        // even when we rewrite Host for vhost matching.
        let client_host = original_host_header.to_string();

        let host_header = resolve_origin_host_header(
            domain,
            original_host_header,
            // Preliminary scheme for header normalization; the authoritative
            // scheme is computed below and used for the outbound URL. This
            // only affects whether the default port suffix is stripped from
            // the Host header we send, which rarely differs between the two.
            &resolve_origin_scheme(domain, incoming_scheme),
        );
        let method = req.method().clone();
        let retryable_method = method == Method::GET || method == Method::HEAD;

        let content_length = req
            .headers()
            .get(http::header::CONTENT_LENGTH)
            .and_then(|v| v.to_str().ok())
            .and_then(|s| s.parse::<u64>().ok())
            .unwrap_or(0);
        let has_body = content_length > 0 || req.headers().contains_key(http::header::TRANSFER_ENCODING);

        if self.max_request_body_bytes > 0 && content_length > self.max_request_body_bytes {
            let templates_state = self.config_holder.get_state();
            let templates_ref = templates_state.as_ref().map(|s| &s.config.templates);
            let resp = build_friendly_error(
                StatusCode::PAYLOAD_TOO_LARGE,
                "请求体过大",
                "上传内容超过节点允许的最大请求体大小。",
                strip_host_port(original_host_header),
                req.uri().path(),
                accept_json,
                Some(&domain.error_pages),
                templates_ref,
            );
            return Ok(resp);
        }

        let scheme = resolve_origin_scheme(domain, incoming_scheme);
        let port = resolve_origin_port(domain, incoming_port, &scheme);
        // Auto-correct scheme/port misconfigurations (http on :443 or https on
        // :80) — this prevents the most common cause of origin 400 responses
        // ("The plain HTTP request was sent to HTTPS port" and its inverse).
        let mut scheme = scheme;
        reconcile_scheme_port(&mut scheme, port);
        debug!(
            "origin resolve: domain={} configured_port={:?} resolved_port={} scheme={}",
            domain.name, domain.origin_port, port, scheme
        );

        let cache_ttl = if retryable_method && !has_body { cache_ttl } else { None };
        let cache_key = if cache_ttl.is_some() { cache_key } else { None };
        let max_retries = if retryable_method && !has_body { origin.max_retries } else { 0 };
        let timeout_ms = domain
            .origin_timeout_ms
            .map(|v| v.max(1) as u64)
            .unwrap_or(origin.timeout_ms.max(1) as u64);
        let timeout = Duration::from_millis(timeout_ms);

        if has_body || !retryable_method {
            // Try to buffer small request bodies so non-GET/HEAD requests can
            // also fail over across origin.addresses. Streaming/large bodies
            // fall back to first-address-only (original behavior).
            let max_retry_body = std::env::var("ORIGIN_BODY_RETRY_MAX_BYTES")
                .ok()
                .and_then(|v| v.trim().parse::<u64>().ok())
                .unwrap_or(1024 * 1024);
            let can_buffer = origin.addresses.len() > 1
                && content_length > 0
                && content_length <= max_retry_body
                && !req.headers().contains_key(http::header::TRANSFER_ENCODING);

            if !can_buffer {
                let origin_addr = origin.addresses.first().context("missing origin")?;
                let upstream_host = strip_host_port(origin_addr);
                let host_for_connect = format_host_with_port(upstream_host, port);
                let raw_path = req
                    .uri()
                    .path_and_query()
                    .map(|p| p.as_str())
                    .unwrap_or("/");
                let path = ensure_leading_slash(raw_path);
                let origin_uri = format!(
                    "{}://{}{}",
                    scheme,
                    host_for_connect,
                    path
                );
                let uri: Uri = origin_uri.parse().context("Invalid origin URI")?;
                if content_length > 0 {
                    self.metrics.bytes_received.inc_by(content_length);
                }
                let (parts, body) = req.into_parts();
                let body = body.map_err(|e| anyhow::anyhow!(e)).boxed();
                let body = if self.max_request_body_bytes > 0 {
                    LimitedBody::new(body, self.max_request_body_bytes, "request").boxed()
                } else {
                    body
                };
                let mut out_req = Request::from_parts(parts, body);
                *out_req.uri_mut() = uri;
                apply_forward_headers(
                    out_req.headers_mut(),
                    &host_header,
                    client_ip,
                    &client_host,
                    incoming_scheme,
                );
                if let Some(ref auth) = domain.origin_auth {
                    apply_origin_auth(out_req.headers_mut(), auth);
                }
                return self.send_to_origin(out_req, timeout, cache_key, cache_ttl, false, &start).await;
            }

            // Buffered-body failover path.
            let (parts, body) = req.into_parts();
            let collected = body
                .collect()
                .await
                .map_err(|e| anyhow::anyhow!("buffer request body: {}", e))?;
            let body_bytes = collected.to_bytes();
            if !body_bytes.is_empty() {
                self.metrics.bytes_received.inc_by(body_bytes.len() as u64);
            }

            let mut last_err: Option<anyhow::Error> = None;
            for origin_addr in origin.addresses.iter() {
                let upstream_host = strip_host_port(origin_addr);
                let host_for_connect = format_host_with_port(upstream_host, port);
                let raw_path = parts
                    .uri
                    .path_and_query()
                    .map(|p| p.as_str())
                    .unwrap_or("/");
                let path = ensure_leading_slash(raw_path);
                let origin_uri = format!(
                    "{}://{}{}",
                    scheme,
                    host_for_connect,
                    path
                );
                let uri: Uri = match origin_uri.parse() {
                    Ok(u) => u,
                    Err(e) => {
                        last_err = Some(anyhow::anyhow!("Invalid origin URI {}: {}", origin_uri, e));
                        continue;
                    }
                };
                let body = Full::new(body_bytes.clone())
                    .map_err(|e| match e {})
                    .boxed();
                let mut out_req = Request::from_parts(parts.clone(), body);
                *out_req.uri_mut() = uri;
                apply_forward_headers(
                    out_req.headers_mut(),
                    &host_header,
                    client_ip,
                    &client_host,
                    incoming_scheme,
                );
                if let Some(ref auth) = domain.origin_auth {
                    apply_origin_auth(out_req.headers_mut(), auth);
                }
                match self.send_to_origin(out_req, timeout, cache_key, cache_ttl, false, &start).await {
                    Ok(resp) => return Ok(resp),
                    Err(e) => {
                        last_err = Some(e);
                        continue;
                    }
                }
            }
            return Err(last_err.unwrap_or_else(|| anyhow::anyhow!("All origins failed")));
        }

        let mut last_err: Option<anyhow::Error> = None;
        for origin_addr in origin.addresses.iter() {
            let upstream_host = strip_host_port(origin_addr);
            let host_for_connect = format_host_with_port(upstream_host, port);
            let raw_path = req
                .uri()
                .path_and_query()
                .map(|p| p.as_str())
                .unwrap_or("/");
            let path = ensure_leading_slash(raw_path);
            let origin_uri = format!(
                "{}://{}{}",
                scheme,
                host_for_connect,
                path
            );
            let uri: Uri = match origin_uri.parse() {
                Ok(u) => u,
                Err(e) => {
                    last_err = Some(anyhow::anyhow!("Invalid origin URI {}: {}", origin_uri, e));
                    continue;
                }
            };

            let mut attempt = 0u32;
            loop {
                attempt += 1;
                let mut builder = Request::builder()
                    .method(method.clone())
                    .uri(uri.clone())
                    .version(req.version());
                for (k, v) in req.headers().iter() {
                    builder = builder.header(k, v);
                }
                let mut out_req = builder
                    .body(Empty::new().map_err(|e| match e {}).boxed())
                    .context("Failed to build outbound request")?;
                apply_forward_headers(
                    out_req.headers_mut(),
                    &host_header,
                    client_ip,
                    &client_host,
                    incoming_scheme,
                );
                if let Some(ref auth) = domain.origin_auth {
                    apply_origin_auth(out_req.headers_mut(), auth);
                }

                match self.send_to_origin(out_req, timeout, cache_key, cache_ttl, true, &start).await {
                    Ok(resp) => return Ok(resp),
                    Err(e) => {
                        last_err = Some(e);
                        if attempt as u32 <= max_retries {
                            let shift = (attempt - 1).min(10) as u32;
                            let exp = 1u64 << shift;
                            let base = 100u64.saturating_mul(exp).min(2000);
                            let jitter = thread_rng().gen_range(0..50u64);
                            tokio::time::sleep(Duration::from_millis(base + jitter)).await;
                            continue;
                        }
                        break;
                    }
                }
            }
        }

        Err(last_err.unwrap_or_else(|| anyhow::anyhow!("All origins failed")))
    }

    async fn send_to_origin(
        &self,
        out_req: Request<NodeBody>,
        timeout: Duration,
        cache_key: Option<&CacheKey>,
        cache_ttl: Option<u64>,
        allow_retry_status: bool,
        started: &std::time::Instant,
    ) -> Result<NodeResponse> {
        // Bound total concurrent origin streams (including long-running downloads). This prevents
        // overload collapse and keeps latency stable at high QPS. Set ORIGIN_MAX_INFLIGHT=0 to disable.
        let mut origin_permit: Option<OwnedSemaphorePermit> = if let Some(sem) = &self.origin_inflight {
            if let Some(wait) = self.origin_inflight_acquire_timeout {
                match tokio::time::timeout(wait, sem.clone().acquire_owned()).await {
                    Ok(Ok(p)) => Some(p),
                    Ok(Err(_)) => None, // semaphore closed; treat as no permit
                    Err(_) => {
                        return Ok(Self::full_response(
                            StatusCode::SERVICE_UNAVAILABLE,
                            Bytes::from("origin busy"),
                            "text/plain; charset=utf-8",
                        ));
                    }
                }
            } else {
                // Wait indefinitely (backpressure) if no timeout configured.
                Some(sem.clone().acquire_owned().await?)
            }
        } else {
            None
        };

        let response = tokio::time::timeout(timeout, self.client.request(out_req))
            .await
            .context("origin timeout")?
            .context("origin request failed")?;

        let (parts, body) = response.into_parts();
        if allow_retry_status {
            let code = parts.status.as_u16();
            if code == 502 || code == 503 || code == 504 {
                return Err(anyhow::anyhow!("retryable status {}", code));
            }
            // Optional failover for origin 4xx: some deployments prefer
            // trying the next origin on 400/403/404 (matches EdgeNode's
            // Retry40X). Enabled via env ORIGIN_RETRY_4XX=1 (list of codes
            // can be customized via ORIGIN_RETRY_4XX_CODES="400,403,404").
            if std::env::var("ORIGIN_RETRY_4XX")
                .map(|v| matches!(v.trim(), "1" | "true" | "yes" | "on"))
                .unwrap_or(false)
            {
                let codes: Vec<u16> = std::env::var("ORIGIN_RETRY_4XX_CODES")
                    .ok()
                    .map(|s| {
                        s.split(',')
                            .filter_map(|x| x.trim().parse::<u16>().ok())
                            .collect()
                    })
                    .unwrap_or_else(|| vec![400, 403, 404]);
                if codes.contains(&code) {
                    return Err(anyhow::anyhow!("retryable 4xx status {}", code));
                }
            }
        }

        let content_length = parts
            .headers
            .get(http::header::CONTENT_LENGTH)
            .and_then(|v| v.to_str().ok())
            .and_then(|s| s.parse::<u64>().ok())
            .unwrap_or(0);

        if self.max_response_body_bytes > 0 && content_length > self.max_response_body_bytes {
            return Ok(Self::full_response(
                StatusCode::BAD_GATEWAY,
                Bytes::from("origin response too large"),
                "text/plain; charset=utf-8",
            ));
        }

        self.metrics.origin_requests_total.inc();
        self.metrics.origin_request_duration.observe(started.elapsed().as_secs_f64());
        if content_length > 0 {
            self.metrics.bytes_sent.inc_by(content_length);
        }

        let max_cache_object = self.max_cache_object_bytes;
        let mem_max_object = {
            let v = self.cache.memory_max_object_bytes();
            if v == 0 { max_cache_object } else { v }
        };
        let disk_max_object = {
            let v = self.cache.disk_max_object_bytes();
            if v == 0 { max_cache_object } else { v }
        };
        let ttl = cache_ttl.unwrap_or(0);
        let negative_ttl = match parts.status.as_u16() {
            404 => self.cache_negative_ttl_404_secs,
            410 => self.cache_negative_ttl_410_secs,
            _ => 0,
        };
        // Negative cache eligibility: we keep the strict size checks only when the
        // origin declared a Content-Length. If CL is absent (e.g. chunked 404/410
        // pages, which are common), we still try to cache, but buffer the body
        // first and enforce the size cap after measuring.
        let has_cl = content_length > 0;
        let negative_cache_allowed = negative_ttl > 0
            && ttl > 0
            && cache_key.is_some()
            && max_cache_object > 0
            && (!has_cl
                || (content_length <= max_cache_object
                    && content_length <= mem_max_object
                    && (self.max_response_body_bytes == 0 || content_length <= self.max_response_body_bytes)));
        if negative_cache_allowed {
            if let Some(cache_key) = cache_key {
                let body_bytes = body
                    .collect()
                    .await
                    .context("Failed to read response body")?
                    .to_bytes();
                // Post-read enforcement for unknown-length bodies.
                let body_len = body_bytes.len() as u64;
                let within_caps = body_len <= max_cache_object
                    && body_len <= mem_max_object
                    && (self.max_response_body_bytes == 0 || body_len <= self.max_response_body_bytes);
                let headers: Vec<(Box<str>, Box<str>)> = parts
                    .headers
                    .iter()
                    .map(|(k, v)| (k.as_str().into(), v.to_str().unwrap_or("").into()))
                    .collect();
                if within_caps {
                    let _ = self.cache_response_bytes(
                        cache_key,
                        parts.status.as_u16(),
                        headers,
                        body_bytes.clone(),
                        negative_ttl,
                    );
                }
                let resp = hyper::Response::from_parts(
                    parts,
                    Full::new(body_bytes).map_err(|e| match e {}).boxed(),
                );
                return Ok(resp);
            }
            warn!("skip negative cache because cache_key is missing");
        }

        let should_cache = ttl > 0 && parts.status.is_success() && content_length > 0;
        let cache_allowed = should_cache
            && cache_key.is_some()
            && max_cache_object > 0
            && content_length <= max_cache_object
            && (self.max_response_body_bytes == 0 || content_length <= self.max_response_body_bytes);

        // Small objects: cache in memory (and inline to disk for persistence).
        if cache_allowed && content_length <= mem_max_object {
            if let Some(cache_key) = cache_key {
                let body_bytes = body
                    .collect()
                    .await
                    .context("Failed to read response body")?
                    .to_bytes();
                let headers: Vec<(Box<str>, Box<str>)> = parts
                    .headers
                    .iter()
                    .map(|(k, v)| (k.as_str().into(), v.to_str().unwrap_or("").into()))
                    .collect();
                let _ = self.cache_response_bytes(cache_key, parts.status.as_u16(), headers, body_bytes.clone(), ttl);
                let resp = hyper::Response::from_parts(
                    parts,
                    Full::new(body_bytes).map_err(|e| match e {}).boxed(),
                );
                return Ok(resp);
            }
            warn!("skip memory cache because cache_key is missing");
        }

        // Large objects: stream to client while writing to disk (no full-body buffering in RAM).
        if cache_allowed
            && content_length > mem_max_object
            && content_length <= disk_max_object
            && self.cache.disk_enabled()
            && !self.cache.disk_over_limit()
        {
            if let Some(cache_key) = cache_key {
                if let Some(permit) = self.cache.try_acquire_write_permit(content_length) {
                    if let Some(paths) = self.cache.prepare_disk_paths(cache_key)? {
                        let status = parts.status.as_u16();
                        let headers: Vec<(Box<str>, Box<str>)> = parts
                            .headers
                            .iter()
                            .map(|(k, v)| (k.as_str().into(), v.to_str().unwrap_or("").into()))
                            .collect();

                        let cache = self.cache.clone();
                        let cache_key = cache_key.clone();
                        let expected_len = content_length;
                        let (tx, mut rx) = tokio::sync::mpsc::channel::<Result<http_body::Frame<Bytes>, anyhow::Error>>(16);

                        let origin_permit_for_task = origin_permit.take();
                        tokio::spawn(async move {
                        use http_body_util::BodyExt;
                        use tokio::io::AsyncWriteExt;

                        // Hold origin inflight permit for the whole lifetime of streaming from origin.
                        let _origin_permit = origin_permit_for_task;

                        let mut caching_ok = true;
                        let mut written: u64 = 0;
                        let mut f = match tokio::fs::File::create(&paths.tmp_path).await {
                            Ok(file) => Some(file),
                            Err(e) => {
                                caching_ok = false;
                                tracing::warn!("disk cache create failed {}: {}", paths.tmp_path.display(), e);
                                None
                            }
                        };

                        // Client disconnect policy: once the downstream receiver goes
                        // away we stop forwarding frames. If we have already written
                        // at least this fraction of the expected body to disk we keep
                        // draining the origin so the cache entry can be committed for
                        // future requests; otherwise we abandon the origin stream so
                        // the inflight permit is released and upstream bandwidth is
                        // not wasted on a client that is no longer listening.
                        const CONTINUE_CACHE_THRESHOLD_NUM: u64 = 1;
                        const CONTINUE_CACHE_THRESHOLD_DEN: u64 = 2; // 50%

                        let mut client_gone = false;
                        let mut origin_body = body;
                        while let Some(frame_res) = origin_body.frame().await {
                            match frame_res {
                                Ok(frame) => {
                                    if let Some(data) = frame.data_ref() {
                                        if caching_ok {
                                            if let Some(file) = f.as_mut() {
                                                if let Err(e) = file.write_all(data).await {
                                                    caching_ok = false;
                                                    tracing::warn!("disk cache write failed: {}", e);
                                                } else {
                                                    written = written.saturating_add(data.len() as u64);
                                                    if expected_len > 0 && written > expected_len {
                                                        caching_ok = false;
                                                    }
                                                }
                                            }
                                        }
                                    }

                                    // Forward to client if still connected. On send error
                                    // (receiver dropped) decide whether to keep draining
                                    // origin for the cache or bail out early.
                                    if !client_gone {
                                        if tx.send(Ok(frame)).await.is_err() {
                                            client_gone = true;
                                            let keep_draining = caching_ok
                                                && expected_len > 0
                                                && written.saturating_mul(CONTINUE_CACHE_THRESHOLD_DEN)
                                                    >= expected_len.saturating_mul(CONTINUE_CACHE_THRESHOLD_NUM);
                                            if !keep_draining {
                                                // Abandon the origin stream and the partial cache.
                                                caching_ok = false;
                                                break;
                                            }
                                        }
                                    }
                                }
                                Err(e) => {
                                    caching_ok = false;
                                    if !client_gone {
                                        let _ = tx.send(Err(anyhow::anyhow!(e))).await;
                                    }
                                    break;
                                }
                            }
                        }

                        if let Some(mut file) = f.take() {
                            let _ = file.flush().await;
                        }

                        if caching_ok && expected_len > 0 && written == expected_len {
                            // Commit: rename tmp -> final and write metadata.
                            if tokio::fs::metadata(&paths.final_path).await.is_ok() {
                                let _ = tokio::fs::remove_file(&paths.final_path).await;
                            }
                            if let Err(e) = tokio::fs::rename(&paths.tmp_path, &paths.final_path).await {
                                caching_ok = false;
                                tracing::warn!("disk cache rename failed: {}", e);
                            } else {
                                let created_at = unix_now_secs();
                                let file_rel = paths.file_rel.clone();
                                let _ = tokio::task::spawn_blocking(move || {
                                    cache.put_disk_file(cache_key, status, headers, file_rel, written, created_at, ttl)
                                })
                                .await;
                            }
                        }

                        if !caching_ok {
                            let _ = tokio::fs::remove_file(&paths.tmp_path).await;
                        }

                        drop(permit);
                        drop(tx);
                    });

                        let body_stream = async_stream::stream! {
                            while let Some(item) = rx.recv().await {
                                yield item;
                            }
                        };
                        let resp_body = http_body_util::BodyExt::boxed(http_body_util::StreamBody::new(body_stream));
                        let resp = hyper::Response::from_parts(parts, resp_body);
                        return Ok(resp);
                    }
                }
            } else {
                warn!("skip disk cache because cache_key is missing");
            }
        }

        let body = body.map_err(|e| anyhow::anyhow!(e)).boxed();
        let body = if self.max_response_body_bytes > 0 {
            LimitedBody::new(body, self.max_response_body_bytes, "response").boxed()
        } else {
            body
        };

        let body = if let Some(p) = origin_permit {
            PermitBody { inner: body, _permit: p }.boxed()
        } else {
            body
        };

        Ok(hyper::Response::from_parts(parts, body))
    }

    fn cache_response_bytes(
        &self,
        key: &CacheKey,
        status: u16,
        headers: Vec<(Box<str>, Box<str>)>,
        body: Bytes,
        ttl_seconds: u64,
    ) -> Result<()> {
        let key_str = key.to_string();
        let created_at = unix_now_secs();

        // Store in memory (immediately available for subsequent requests).
        let memory_max_object_bytes = self.cache.memory_max_object_bytes();
        if memory_max_object_bytes == 0 || (body.len() as u64) <= memory_max_object_bytes {
            self.cache.put_memory_only(
                key_str.clone(),
                CacheEntry {
                    status,
                    headers: headers.clone(),
                    body: CachedBody::Bytes(body.clone()),
                    created_at,
                    ttl_seconds,
                },
            );
        }

        // Persist to disk in the background to avoid blocking Tokio workers.
        if self.cache.disk_enabled() && !self.cache.disk_over_limit() && ttl_seconds > 0 {
            let expected_bytes = body.len() as u64;
            if let Some(permit) = self.cache.try_acquire_write_permit(expected_bytes) {
                let cache = self.cache.clone();
                let key_for_disk = key_str;
                let body_for_disk = body;
                tokio::task::spawn_blocking(move || {
                    let _permit = permit;
                    if let Err(e) = cache.put_disk_inline(
                        key_for_disk,
                        status,
                        headers,
                        body_for_disk,
                        created_at,
                        ttl_seconds,
                    ) {
                        tracing::debug!("disk cache inline write failed: {}", e);
                    }
                });
            }
        }
        let sz = self.cache.stats().size_bytes as i64;
        self.metrics.cache_size_bytes.set(sz);
        Ok(())
    }

    async fn build_response_from_cache(&self, entry: CacheEntry) -> Result<NodeResponse> {
        let mut builder = hyper::Response::builder().status(entry.status);

        for (key, value) in &entry.headers {
            builder = builder.header(&**key, &**value);
        }

        let body = match entry.body {
            CachedBody::Bytes(b) => Full::new(b).map_err(|e| match e {}).boxed(),
            CachedBody::File { path, .. } => {
                // Avoid loading the whole object into RAM; stream from disk.
                let f = tokio::fs::File::open(&path)
                    .await
                    .with_context(|| format!("open cached file {}", path.display()))?;
                let stream = tokio_util::io::ReaderStream::new(f).map(|res| {
                    res.map(http_body::Frame::data)
                        .map_err(|e| anyhow::anyhow!(e))
                });
                http_body_util::BodyExt::boxed(http_body_util::StreamBody::new(stream))
            }
        };

        builder
            .body(body)
            .map_err(|e| anyhow::anyhow!("build cached response failed: {e}"))
    }

    fn sweep_waf_state(&self) {
        let now = Instant::now();
        {
            let mut total = 0usize;
            for shard in self.waf_counters.iter_shards() {
                let mut guard = shard.lock();
                guard.retain(|_, v| now.duration_since(v.last_seen) < Duration::from_secs(300));
                total = total.saturating_add(guard.len());
            }
            if total > 50000 {
                // Evict oldest half instead of clearing everything
                for shard in self.waf_counters.iter_shards() {
                    let mut guard = shard.lock();
                    guard.retain(|_, v| now.duration_since(v.last_seen) < Duration::from_secs(60));
                }
            }
        }
        {
            let mut total = 0usize;
            for shard in self.waf_fail.iter_shards() {
                let mut guard = shard.lock();
                guard.retain(|_, v| now.duration_since(v.last_seen) < Duration::from_secs(1800));
                total = total.saturating_add(guard.len());
            }
            if total > 200000 {
                for shard in self.waf_fail.iter_shards() {
                    let mut guard = shard.lock();
                    guard.retain(|_, v| now.duration_since(v.last_seen) < Duration::from_secs(300));
                }
            }
        }
        {
            let mut total = 0usize;
            for shard in self.waf_ban.iter_shards() {
                let mut guard = shard.lock();
                guard.retain(|_, v| now < v.until && now.duration_since(v.last_seen) < Duration::from_secs(3600));
                total = total.saturating_add(guard.len());
            }
            if total > 200000 {
                for shard in self.waf_ban.iter_shards() {
                    let mut guard = shard.lock();
                    guard.retain(|_, v| now < v.until && now.duration_since(v.last_seen) < Duration::from_secs(600));
                }
            }
        }
        // Rate limit counters: very short TTL (max 120s)
        {
            let mut total = 0usize;
            for shard in self.waf_rate_limits.iter_shards() {
                let mut guard = shard.lock();
                guard.retain(|_, v| now.duration_since(v.window_start) < Duration::from_secs(120));
                total = total.saturating_add(guard.len());
            }
            if total > 200000 {
                // Under extreme load, keep only very recent entries
                for shard in self.waf_rate_limits.iter_shards() {
                    let mut guard = shard.lock();
                    guard.retain(|_, v| now.duration_since(v.window_start) < Duration::from_secs(5));
                }
            }
        }
        // 清理过期验证码会话
        {
            let mut total = 0usize;
            for shard in self.captcha_sessions.iter_shards() {
                let mut guard = shard.lock();
                guard.retain(|_, v| v.created_at.elapsed() < Duration::from_secs(600));
                total = total.saturating_add(guard.len());
            }
            if total > 10000 {
                self.captcha_sessions.clear_all();
            }
        }
    }

    #[cfg(feature = "legacy_waf")]
    fn enforce_waf(
        &self,
        policies: &[WAFPolicy],
        bans: &[crate::config::WAFBan],
        whitelist: &[crate::config::WAFWhitelist],
        domain_id: &str,
        host: &str,
        path: &str,
        method: &str,
        client_ip: Option<&str>,
        headers: &HeaderMap,
    ) -> Option<NodeResponse> {
        self.maybe_sweep_waf_state();
        if policies.is_empty() {
            return None;
        }
        // 白名单检查 - 白名单IP跳过所有WAF检查
        if self.is_ip_whitelisted(whitelist, client_ip) {
            return None;
        }
        if self.enforce_global_ban(bans, client_ip) {
            return Some(
                Self::full_response(
                    StatusCode::FORBIDDEN,
                    Bytes::from("Blocked by global WAF ban"),
                    "text/plain; charset=utf-8",
                ),
            );
        }
        let mut matched: Vec<&WAFPolicy> = Vec::new();
        for p in policies {
            if !p.enabled {
                continue;
            }
            match p.scope.as_str() {
                "global" => matched.push(p),
                "domain" => {
                    if let Some(ref sid) = p.scope_id {
                        if sid == domain_id {
                            matched.push(p);
                        }
                    }
                }
                _ => {}
            }
        }
        if matched.is_empty() {
            return None;
        }
        let mut rules: Vec<&WAFRule> = matched
            .iter()
            .flat_map(|p| p.rules.iter().filter(|r| r.enabled))
            .collect();
        rules.sort_by_key(|r| r.priority.unwrap_or(0));

        for rule in rules {
            if let Some(expires_at) = rule.expires_at {
                let now = chrono::Utc::now().timestamp();
                if now > expires_at {
                    continue;
                }
            }
            if let Some(ref prefix) = rule.path_prefix {
                if !prefix.is_empty() && !path.starts_with(prefix) {
                    continue;
                }
            }
            if let Some(ref methods) = rule.methods {
                if !methods.is_empty() {
                    if !methods.iter().any(|m| m.eq_ignore_ascii_case(method)) {
                        continue;
                    }
                }
            }
            if let Some(ref ua_sub) = rule.ua_contains {
                if !ua_sub.is_empty() {
                    let ua = headers
                        .get("User-Agent")
                        .and_then(|v| v.to_str().ok())
                        .unwrap_or("")
                        .to_ascii_lowercase();
                    if !ua.contains(&ua_sub.to_ascii_lowercase()) {
                        continue;
                    }
                }
            }
            let mut triggered = false;
            if let Some(thresh) = rule.auto_challenge_qps {
                if thresh > 0 {
                    let (qps, active) = self.current_qps(domain_id, thresh as u64);
                    if active || qps > thresh as u64 {
                        triggered = true;
                    }
                }
            }
            match rule.r#type.as_str() {
                "ip_cidr" => {
                    if let (Some(ip), Ok(net)) = (client_ip, rule.value.parse::<IpNet>()) {
                        if let Ok(addr) = ip.parse::<std::net::IpAddr>() {
                            if net.contains(&addr) && rule.action == "deny" {
                                if rule.log_only.unwrap_or(false) {
                                    warn!("WAF log-only ip_cidr hit ip={}", ip);
                                    continue;
                                }
                                return Some(Self::full_response(
                                    StatusCode::FORBIDDEN,
                                    Bytes::from("Blocked by WAF"),
                                    "text/plain; charset=utf-8",
                                ));
                            }
                        }
                    }
                }
                "geo_block" => {
                    if rule.action == "deny" && !rule.geo_countries.is_empty() {
                        if let Some(ip) = client_ip {
                            if let Some(country) = self.geoip_holder.get().and_then(|r| r.lookup_country(ip)) {
                                let blocked = is_geo_blocked(&rule.geo_countries, &country);
                                if blocked {
                                    if rule.log_only.unwrap_or(false) {
                                        warn!("WAF log-only geo_block hit ip={} country={}", ip, country);
                                        continue;
                                    }
                                    return Some(Self::full_response(
                                        StatusCode::FORBIDDEN,
                                        Bytes::from("Access denied: your region is blocked"),
                                        "text/plain; charset=utf-8",
                                    ));
                                }
                            }
                        }
                    }
                }
                "block_transparent_proxy" => {
                    if rule.action == "deny" && headers.contains_key("x-forwarded-for") {
                        if rule.log_only.unwrap_or(false) {
                            warn!("WAF log-only block_transparent_proxy hit");
                            continue;
                        }
                        return Some(Self::full_response(
                            StatusCode::FORBIDDEN,
                            Bytes::from("Access denied: transparent proxy not allowed"),
                            "text/plain; charset=utf-8",
                        ));
                    }
                }
                "rate_limit" => {
                    let threshold = rule.threshold.unwrap_or(0);
                    let window = rule.window_seconds.unwrap_or(0);
                    if threshold > 0 && window > 0 {
                        if let Some(ip) = client_ip {
                            let key = format!("{}:{}:{}", domain_id, rule.id, ip);
                            let exceeded = {
                                let shard = self.waf_rate_limits.shard(&key);
                                let mut guard = shard.lock();
                                let now = Instant::now();
                                let entry = guard.entry(key).or_insert(RateLimitEntry {
                                    count: 0,
                                    window_start: now,
                                });
                                if now.duration_since(entry.window_start).as_secs() >= window as u64 {
                                    entry.count = 1;
                                    entry.window_start = now;
                                    false
                                } else {
                                    entry.count += 1;
                                    entry.count > threshold as u32
                                }
                            };
                            if exceeded {
                                if rule.log_only.unwrap_or(false) {
                                    warn!("WAF log-only rate_limit hit ip={} threshold={}", ip, threshold);
                                    continue;
                                }
                                let ban_mode = rule.ban_mode.as_deref().unwrap_or("ipset");
                                let ban_seconds = rule.ban_seconds.unwrap_or(300);
                                self.record_fail(client_ip, 1, ban_seconds, ban_mode);
                                return Some(Self::full_response(
                                    StatusCode::TOO_MANY_REQUESTS,
                                    Bytes::from("Rate limit exceeded"),
                                    "text/plain; charset=utf-8",
                                ));
                            }
                        }
                    }
                }
                "challenge_captcha" => {
                    if rule.auto_challenge_qps.is_some() && !triggered {
                        continue;
                    }
                    if rule.log_only.unwrap_or(false) {
                        warn!("WAF log-only challenge hit domain_id={}", domain_id);
                        continue;
                    }
                    let ban_mode = rule.ban_mode.as_deref().unwrap_or("ipset");
                    if let Some(mode) = self.is_banned(client_ip) {
                        return Some(self.ban_response(
                            &mode,
                            rule.ban_template_html
                                .as_deref()
                                .or(rule.template_html.as_deref()),
                        ));
                    }
                    let ban_seconds = rule.ban_seconds.or(rule.shield_seconds).unwrap_or(300);
                    let fail_limit = rule.threshold.unwrap_or(3);
                    let captcha_type = rule.captcha_type.as_deref().unwrap_or("");

                    // 根据 captcha_type 选择验证方式
                    match captcha_type {
                        "slide" | "click" | "rotate" | "slide_region" | "js_challenge" => {
                            // 行为验证码类型
                            let lang = headers
                                .get("Accept-Language")
                                .and_then(|v| v.to_str().ok())
                                .map(|s| if s.starts_with("zh") { "zh" } else { "en" })
                                .unwrap_or("zh");

                            let ct = match captcha_type {
                                "slide" => CaptchaType::Slide,
                                "click" => CaptchaType::Click,
                                "rotate" => CaptchaType::Rotate,
                                "slide_region" => CaptchaType::SlideRegion,
                                "js_challenge" => CaptchaType::JsChallenge,
                                _ => CaptchaType::Slide,
                            };

                            if ct == CaptchaType::JsChallenge {
                                return Some(self.js_challenge_response(
                                    rule.template_html.as_deref().unwrap_or(""),
                                    lang,
                                ));
                            } else {
                                return Some(self.behavioral_captcha_response(
                                    ct,
                                    rule.template_html.as_deref().unwrap_or(""),
                                    lang,
                                ));
                            }
                        }
                        _ => {
                            // 默认使用旧的数学题验证
                            if self.verify_challenge(
                                client_ip,
                                headers,
                                fail_limit,
                                ban_seconds,
                                ban_mode,
                            ) {
                                continue;
                            }
                            return Some(self.challenge_response(
                                host,
                                rule.shield_seconds.unwrap_or(5),
                                rule.template_html.as_deref(),
                                rule.redirect_url.as_deref(),
                            ));
                        }
                    }
                }
                "shield_5s" => {
                    if rule.auto_challenge_qps.is_some() && !triggered {
                        continue;
                    }
                    if rule.log_only.unwrap_or(false) {
                        warn!("WAF log-only shield hit domain_id={}", domain_id);
                        continue;
                    }
                    return Some(self.shield_response(rule.shield_seconds.unwrap_or(5)));
                }
                "default" | "path_match" | "header_match" | "ua_match" => {
                    if !domain_security_rule_matches(rule.r#type.as_str(), &rule.value, path, headers) {
                        continue;
                    }
                    if rule.log_only.unwrap_or(false) {
                        warn!("WAF log-only {} hit domain_id={}", rule.r#type, domain_id);
                        continue;
                    }
                    if rule.action == "shield" {
                        return Some(self.shield_response(rule.shield_seconds.unwrap_or(5)));
                    }
                    if rule.action == "challenge" {
                        let ban_mode = rule.ban_mode.as_deref().unwrap_or("ipset");
                        if let Some(mode) = self.is_banned(client_ip) {
                            return Some(self.ban_response(
                                &mode,
                                rule.ban_template_html
                                    .as_deref()
                                    .or(rule.template_html.as_deref()),
                            ));
                        }
                        let ban_seconds = rule.ban_seconds.or(rule.shield_seconds).unwrap_or(300);
                        let fail_limit = rule.threshold.unwrap_or(3);
                        let captcha_type = rule.captcha_type.as_deref().unwrap_or("");
                        match captcha_type {
                            "slide" | "click" | "rotate" | "slide_region" | "js_challenge" => {
                                let lang = headers
                                    .get("Accept-Language")
                                    .and_then(|v| v.to_str().ok())
                                    .map(|s| if s.starts_with("zh") { "zh" } else { "en" })
                                    .unwrap_or("zh");
                                let ct = match captcha_type {
                                    "slide" => CaptchaType::Slide,
                                    "click" => CaptchaType::Click,
                                    "rotate" => CaptchaType::Rotate,
                                    "slide_region" => CaptchaType::SlideRegion,
                                    "js_challenge" => CaptchaType::JsChallenge,
                                    _ => CaptchaType::Slide,
                                };
                                if ct == CaptchaType::JsChallenge {
                                    return Some(self.js_challenge_response(
                                        rule.template_html.as_deref().unwrap_or(""),
                                        lang,
                                    ));
                                } else {
                                    return Some(self.behavioral_captcha_response(
                                        ct,
                                        rule.template_html.as_deref().unwrap_or(""),
                                        lang,
                                    ));
                                }
                            }
                            _ => {
                                if self.verify_challenge(
                                    client_ip,
                                    headers,
                                    fail_limit,
                                    ban_seconds,
                                    ban_mode,
                                ) {
                                    continue;
                                }
                                return Some(self.challenge_response(
                                    host,
                                    rule.shield_seconds.unwrap_or(5),
                                    rule.template_html.as_deref(),
                                    rule.redirect_url.as_deref(),
                                ));
                            }
                        }
                    }
                }
                _ => {}
            }
        }
        None
    }

    fn enforce_waf_compiled(
        &self,
        waf: &CompiledWaf,
        has_policies: bool,
        domain_id: &str,
        host: &str,
        path: &str,
        method: &str,
        client_ip: Option<&str>,
        client_addr: Option<IpAddr>,
        headers: &HeaderMap,
    ) -> Option<NodeResponse> {
        // Preserve legacy semantics: whitelist/global ban are only enforced when there exists at least
        // one WAF policy entry (even if all of them are disabled).
        if !has_policies {
            return None;
        }

        self.maybe_sweep_waf_state();

        if waf.is_ip_whitelisted(client_ip, client_addr) {
            return None;
        }

        if waf.is_globally_banned(client_ip, client_addr) {
            return Some(Self::full_response(
                StatusCode::FORBIDDEN,
                Bytes::from("Blocked by global WAF ban"),
                "text/plain; charset=utf-8",
            ));
        }

        let global_rules: &[CompiledWafRule] = waf.global_rules.as_slice();
        let domain_rules: &[CompiledWafRule] = waf
            .domain_rules
            .get(domain_id)
            .map(|v| v.as_slice())
            .unwrap_or(&[]);
        if global_rules.is_empty() && domain_rules.is_empty() {
            return None;
        }

        let now_unix = chrono::Utc::now().timestamp();
        let mut ua_lower: Option<String> = None;

        let mut gi = 0usize;
        let mut di = 0usize;
        while gi < global_rules.len() || di < domain_rules.len() {
            let next: &CompiledWafRule = match (global_rules.get(gi), domain_rules.get(di)) {
                (Some(g), Some(d)) => {
                    if (g.priority, g.seq) <= (d.priority, d.seq) {
                        gi += 1;
                        g
                    } else {
                        di += 1;
                        d
                    }
                }
                (Some(g), None) => {
                    gi += 1;
                    g
                }
                (None, Some(d)) => {
                    di += 1;
                    d
                }
                (None, None) => break,
            };

            let rule = &next.rule;
            if let Some(expires_at) = rule.expires_at {
                if now_unix > expires_at {
                    continue;
                }
            }

            if let Some(ref prefix) = rule.path_prefix {
                if !prefix.is_empty() && !path.starts_with(prefix) {
                    continue;
                }
            }

            if let Some(ref methods) = rule.methods {
                if !methods.is_empty() && !methods.iter().any(|m| m.eq_ignore_ascii_case(method)) {
                    continue;
                }
            }

            if let Some(ua_sub) = next.ua_contains_lower.as_deref() {
                if ua_lower.is_none() {
                    let ua = headers
                        .get("User-Agent")
                        .and_then(|v| v.to_str().ok())
                        .unwrap_or("");
                    ua_lower = Some(ua.to_ascii_lowercase());
                }
                if !ua_lower.as_deref().unwrap_or("").contains(ua_sub) {
                    continue;
                }
            }

            let mut triggered = false;
            if let Some(thresh) = rule.auto_challenge_qps {
                if thresh > 0 {
                    let (qps, active) = self.current_qps(domain_id, thresh as u64);
                    if active || qps > thresh as u64 {
                        triggered = true;
                    }
                }
            }

            match rule.r#type.as_str() {
                "ip_cidr" => {
                    let Some(net) = next.cidr.as_ref() else { continue };
                    let Some(addr) = client_addr else { continue };
                    if net.contains(&addr) && rule.action == "deny" {
                        if rule.log_only.unwrap_or(false) {
                            warn!("WAF log-only ip_cidr hit ip={}", client_ip.unwrap_or(""));
                            continue;
                        }
                        return Some(Self::full_response(
                            StatusCode::FORBIDDEN,
                            Bytes::from("Blocked by WAF"),
                            "text/plain; charset=utf-8",
                        ));
                    }
                }
                "geo_block" => {
                    if rule.action == "deny" && !rule.geo_countries.is_empty() {
                        if let Some(ip) = client_ip {
                            if let Some(country) = self.geoip_holder.get().and_then(|r| r.lookup_country(ip)) {
                                let blocked = is_geo_blocked(&rule.geo_countries, &country);
                                if blocked {
                                    if rule.log_only.unwrap_or(false) {
                                        warn!("WAF log-only geo_block hit ip={} country={}", ip, country);
                                        continue;
                                    }
                                    return Some(Self::full_response(
                                        StatusCode::FORBIDDEN,
                                        Bytes::from("Access denied: your region is blocked"),
                                        "text/plain; charset=utf-8",
                                    ));
                                }
                            }
                        }
                    }
                }
                "block_transparent_proxy" => {
                    if rule.action == "deny" && headers.contains_key("x-forwarded-for") {
                        if rule.log_only.unwrap_or(false) {
                            warn!("WAF log-only block_transparent_proxy hit");
                            continue;
                        }
                        return Some(Self::full_response(
                            StatusCode::FORBIDDEN,
                            Bytes::from("Access denied: transparent proxy not allowed"),
                            "text/plain; charset=utf-8",
                        ));
                    }
                }
                "rate_limit" => {
                    let threshold = rule.threshold.unwrap_or(0);
                    let window = rule.window_seconds.unwrap_or(0);
                    if threshold > 0 && window > 0 {
                        if let Some(ip) = client_ip {
                            let key = format!("{}:{}:{}", domain_id, rule.id, ip);
                            let exceeded = {
                                let shard = self.waf_rate_limits.shard(&key);
                                let mut guard = shard.lock();
                                let now = Instant::now();
                                let entry = guard.entry(key).or_insert(RateLimitEntry {
                                    count: 0,
                                    window_start: now,
                                });
                                if now.duration_since(entry.window_start).as_secs() >= window as u64 {
                                    entry.count = 1;
                                    entry.window_start = now;
                                    false
                                } else {
                                    entry.count += 1;
                                    entry.count > threshold as u32
                                }
                            };
                            if exceeded {
                                if rule.log_only.unwrap_or(false) {
                                    warn!("WAF log-only rate_limit hit ip={} threshold={}", ip, threshold);
                                    continue;
                                }
                                let ban_mode = rule.ban_mode.as_deref().unwrap_or("ipset");
                                let ban_seconds = rule.ban_seconds.unwrap_or(300);
                                self.record_fail(client_ip, 1, ban_seconds, ban_mode);
                                return Some(Self::full_response(
                                    StatusCode::TOO_MANY_REQUESTS,
                                    Bytes::from("Rate limit exceeded"),
                                    "text/plain; charset=utf-8",
                                ));
                            }
                        }
                    }
                }
                "challenge_captcha" => {
                    if rule.auto_challenge_qps.is_some() && !triggered {
                        continue;
                    }
                    if rule.log_only.unwrap_or(false) {
                        warn!("WAF log-only challenge hit domain_id={}", domain_id);
                        continue;
                    }
                    let ban_mode = rule.ban_mode.as_deref().unwrap_or("ipset");
                    if let Some(mode) = self.is_banned(client_ip) {
                        return Some(self.ban_response(
                            &mode,
                            rule.ban_template_html
                                .as_deref()
                                .or(rule.template_html.as_deref()),
                        ));
                    }
                    let ban_seconds = rule.ban_seconds.or(rule.shield_seconds).unwrap_or(300);
                    let fail_limit = rule.threshold.unwrap_or(3);
                    let captcha_type = rule.captcha_type.as_deref().unwrap_or("");

                    // 鏍规嵁 captcha_type 閫夋嫨楠岃瘉鏂瑰紡
                    match captcha_type {
                        "slide" | "click" | "rotate" | "slide_region" | "js_challenge" => {
                            let lang = headers
                                .get("Accept-Language")
                                .and_then(|v| v.to_str().ok())
                                .map(|s| if s.starts_with("zh") { "zh" } else { "en" })
                                .unwrap_or("zh");

                            let ct = match captcha_type {
                                "slide" => CaptchaType::Slide,
                                "click" => CaptchaType::Click,
                                "rotate" => CaptchaType::Rotate,
                                "slide_region" => CaptchaType::SlideRegion,
                                "js_challenge" => CaptchaType::JsChallenge,
                                _ => CaptchaType::Slide,
                            };

                            if ct == CaptchaType::JsChallenge {
                                return Some(self.js_challenge_response(
                                    rule.template_html.as_deref().unwrap_or(""),
                                    lang,
                                ));
                            } else {
                                return Some(self.behavioral_captcha_response(
                                    ct,
                                    rule.template_html.as_deref().unwrap_or(""),
                                    lang,
                                ));
                            }
                        }
                        _ => {
                            if self.verify_challenge(
                                client_ip,
                                headers,
                                fail_limit,
                                ban_seconds,
                                ban_mode,
                            ) {
                                continue;
                            }
                            return Some(self.challenge_response(
                                host,
                                rule.shield_seconds.unwrap_or(5),
                                rule.template_html.as_deref(),
                                rule.redirect_url.as_deref(),
                            ));
                        }
                    }
                }
                "shield_5s" => {
                    if rule.auto_challenge_qps.is_some() && !triggered {
                        continue;
                    }
                    if rule.log_only.unwrap_or(false) {
                        warn!("WAF log-only shield hit domain_id={}", domain_id);
                        continue;
                    }
                    return Some(self.shield_response(rule.shield_seconds.unwrap_or(5)));
                }
                "default" | "path_match" | "header_match" | "ua_match" => {
                    if !domain_security_rule_matches(rule.r#type.as_str(), &rule.value, path, headers) {
                        continue;
                    }
                    if rule.log_only.unwrap_or(false) {
                        warn!("WAF log-only {} hit domain_id={}", rule.r#type, domain_id);
                        continue;
                    }
                    if rule.action == "shield" {
                        return Some(self.shield_response(rule.shield_seconds.unwrap_or(5)));
                    }
                    if rule.action == "challenge" {
                        let ban_mode = rule.ban_mode.as_deref().unwrap_or("ipset");
                        if let Some(mode) = self.is_banned(client_ip) {
                            return Some(self.ban_response(
                                &mode,
                                rule.ban_template_html
                                    .as_deref()
                                    .or(rule.template_html.as_deref()),
                            ));
                        }
                        let ban_seconds = rule.ban_seconds.or(rule.shield_seconds).unwrap_or(300);
                        let fail_limit = rule.threshold.unwrap_or(3);
                        let captcha_type = rule.captcha_type.as_deref().unwrap_or("");
                        match captcha_type {
                            "slide" | "click" | "rotate" | "slide_region" | "js_challenge" => {
                                let lang = headers
                                    .get("Accept-Language")
                                    .and_then(|v| v.to_str().ok())
                                    .map(|s| if s.starts_with("zh") { "zh" } else { "en" })
                                    .unwrap_or("zh");
                                let ct = match captcha_type {
                                    "slide" => CaptchaType::Slide,
                                    "click" => CaptchaType::Click,
                                    "rotate" => CaptchaType::Rotate,
                                    "slide_region" => CaptchaType::SlideRegion,
                                    "js_challenge" => CaptchaType::JsChallenge,
                                    _ => CaptchaType::Slide,
                                };
                                if ct == CaptchaType::JsChallenge {
                                    return Some(self.js_challenge_response(
                                        rule.template_html.as_deref().unwrap_or(""),
                                        lang,
                                    ));
                                } else {
                                    return Some(self.behavioral_captcha_response(
                                        ct,
                                        rule.template_html.as_deref().unwrap_or(""),
                                        lang,
                                    ));
                                }
                            }
                            _ => {
                                if self.verify_challenge(
                                    client_ip,
                                    headers,
                                    fail_limit,
                                    ban_seconds,
                                    ban_mode,
                                ) {
                                    continue;
                                }
                                return Some(self.challenge_response(
                                    host,
                                    rule.shield_seconds.unwrap_or(5),
                                    rule.template_html.as_deref(),
                                    rule.redirect_url.as_deref(),
                                ));
                            }
                        }
                    }
                }
                _ => {}
            }
        }

        None
    }

    #[cfg(feature = "legacy_waf")]
    fn enforce_global_ban(&self, bans: &[crate::config::WAFBan], client_ip: Option<&str>) -> bool {
        let Some(ip) = client_ip else { return false };
        for b in bans {
            if b.expires_at > 0 && chrono::Utc::now().timestamp() > b.expires_at {
                continue;
            }
            if b.ip == ip {
                return true;
            }
            // CIDR ban support (optional)
            if b.ip.contains('/') {
                if let Ok(net) = b.ip.parse::<IpNet>() {
                    if let Ok(addr) = ip.parse::<std::net::IpAddr>() {
                        if net.contains(&addr) {
                            return true;
                        }
                    }
                }
            }
        }
        false
    }

    /// 检查IP是否在白名单中
    #[allow(dead_code)]
    fn is_ip_whitelisted(&self, whitelist: &[crate::config::WAFWhitelist], client_ip: Option<&str>) -> bool {
        let Some(ip) = client_ip else { return false };
        let Ok(addr) = ip.parse::<std::net::IpAddr>() else { return false };

        for w in whitelist {
            // CIDR 匹配
            if w.ip.contains('/') {
                if let Ok(net) = w.ip.parse::<ipnet::IpNet>() {
                    if net.contains(&addr) {
                        return true;
                    }
                }
            } else {
                // 直接 IP 匹配
                if w.ip == ip {
                    return true;
                }
            }
        }
        false
    }

    fn current_qps(&self, domain_id: &str, threshold: u64) -> (u64, bool) {
        let now = Instant::now();
        let shard = self.waf_counters.shard(domain_id);
        let mut guard = shard.lock();
        let stat = guard.entry(domain_id.to_string()).or_insert(WafDomainStat {
            count: 0,
            window_start: now,
            challenge_active: false,
            last_over: now,
            last_under: now,
            last_seen: now,
        });
        stat.last_seen = now;
        let elapsed = now.duration_since(stat.window_start).as_secs_f64();
        if elapsed >= 1.0 {
            stat.count = 0;
            stat.window_start = now;
        }
        stat.count += 1;
        let qps = (stat.count as f64 / now.duration_since(stat.window_start).as_secs_f64().max(0.1)) as u64;

        if qps > threshold {
            stat.challenge_active = true;
            stat.last_over = now;
        } else {
            stat.last_under = now;
            if stat.challenge_active && now.duration_since(stat.last_over) >= Duration::from_secs(180) {
                stat.challenge_active = false;
            }
        }
        (qps, stat.challenge_active)
    }

    fn ban_response(&self, mode: &str, template_html: Option<&str>) -> NodeResponse {
        match mode.to_ascii_lowercase().as_str() {
            "page" => {
                let state = self.config_holder.get_state();
                let fallback_tpl = state
                    .as_ref()
                    .map(|s| s.config.templates.waf_ban_default.as_str())
                    .filter(|s| !s.is_empty());
                let chosen = template_html
                    .filter(|s| !s.is_empty())
                    .or(fallback_tpl);
                if let Some(tpl) = chosen {
                    let builder = hyper::Response::builder()
                        .status(StatusCode::FORBIDDEN)
                        .header("Content-Type", "text/html; charset=utf-8")
                        .header("X-WAF-Ban", "page");
                    return build_response_or_fallback(
                        builder,
                        Full::new(Bytes::from(tpl.to_string())).map_err(|e| match e {}).boxed(),
                        StatusCode::FORBIDDEN,
                        "Access denied by WAF ban",
                    );
                }
                let builder = hyper::Response::builder()
                    .status(StatusCode::FORBIDDEN)
                    .header("Content-Type", "text/plain; charset=utf-8")
                    .header("X-WAF-Ban", "page");
                build_response_or_fallback(
                    builder,
                    Full::new(Bytes::from("Access denied by WAF ban")).map_err(|e| match e {}).boxed(),
                    StatusCode::FORBIDDEN,
                    "Access denied by WAF ban",
                )
            }
            "drop" => {
                let builder = hyper::Response::builder()
                    .status(StatusCode::FORBIDDEN)
                    .header("X-WAF-Ban", "drop");
                build_response_or_fallback(
                    builder,
                    Full::new(Bytes::new()).map_err(|e| match e {}).boxed(),
                    StatusCode::FORBIDDEN,
                    "forbidden",
                )
            }
            _ => {
                let builder = hyper::Response::builder()
                    .status(StatusCode::FORBIDDEN)
                    .header("X-WAF-Ban", "ipset");
                build_response_or_fallback(
                    builder,
                    Full::new(Bytes::new()).map_err(|e| match e {}).boxed(),
                    StatusCode::FORBIDDEN,
                    "forbidden",
                )
            }
        }
    }

    fn challenge_response(
        &self,
        host: &str,
        shield_seconds: i64,
        template_html: Option<&str>,
        redirect_url: Option<&str>,
    ) -> NodeResponse {
        let (question, token) = self.issue_challenge(host);
        if let Some(url) = redirect_url {
            let builder = hyper::Response::builder()
                .status(StatusCode::FOUND)
                .header("Location", url)
                .header("X-WAF-Challenge", "redirect")
                .header("X-WAF-Token", token.clone());
            return build_response_or_fallback(
                builder,
                Full::new(Bytes::from(""))
                    .map_err(|e| match e {})
                    .boxed(),
                StatusCode::FOUND,
                "redirect",
            );
        }
        if let Some(tpl) = template_html {
            let body = tpl
                .replace("{{TOKEN}}", &token)
                .replace("{{QUESTION}}", &question)
                .replace("{{WAIT_SECONDS}}", &shield_seconds.to_string());
            let builder = hyper::Response::builder()
                .status(StatusCode::FORBIDDEN)
                .header("Content-Type", "text/html; charset=utf-8")
                .header("X-WAF-Challenge", "html")
                .header("X-WAF-Token", token.clone());
            return build_response_or_fallback(
                builder,
                Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
                StatusCode::FORBIDDEN,
                "Captcha required",
            );
        }
        if let Some(state) = self.config_holder.get_state() {
            let tpl = &state.config.templates.waf_challenge_default_json;
            if !tpl.is_empty() {
                let body = tpl
                    .replace("{{TOKEN}}", &token)
                    .replace("{{QUESTION}}", &question)
                    .replace("{{WAIT_SECONDS}}", &shield_seconds.to_string());
                let builder = hyper::Response::builder()
                    .status(StatusCode::FORBIDDEN)
                    .header("Content-Type", "application/json")
                    .header("X-WAF-Challenge", "captcha")
                    .header("X-WAF-Token", token.clone());
                return build_response_or_fallback(
                    builder,
                    Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
                    StatusCode::FORBIDDEN,
                    "Captcha required",
                );
            }
        }
        let body = serde_json::json!({
            "waf_challenge": true,
            "question": question,
            "token": token,
            "wait_seconds": shield_seconds,
            "msg": "Captcha required"
        });
        let builder = hyper::Response::builder()
            .status(StatusCode::FORBIDDEN)
            .header("Content-Type", "application/json")
            .header("X-WAF-Challenge", "captcha");
        build_response_or_fallback(
            builder,
            Full::new(Bytes::from(body.to_string()))
                .map_err(|e| match e {})
                .boxed(),
            StatusCode::FORBIDDEN,
            "Captcha required",
        )
    }

    fn shield_response(&self, shield_seconds: i64) -> NodeResponse {
        if let Some(state) = self.config_holder.get_state() {
            let tpl = &state.config.templates.waf_shield_page;
            if !tpl.is_empty() {
                let body = tpl.replace("{{WAIT_SECONDS}}", &shield_seconds.to_string());
                let builder = hyper::Response::builder()
                    .status(StatusCode::SERVICE_UNAVAILABLE)
                    .header("Retry-After", shield_seconds.to_string())
                    .header("Content-Type", "text/html; charset=utf-8")
                    .header("X-WAF-Shield", "5s");
                return build_response_or_fallback(
                    builder,
                    Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
                    StatusCode::SERVICE_UNAVAILABLE,
                    "Please retry later",
                );
            }
        }
        let builder = hyper::Response::builder()
            .status(StatusCode::SERVICE_UNAVAILABLE)
            .header("Retry-After", shield_seconds.to_string())
            .header("X-WAF-Shield", "5s");
        build_response_or_fallback(
            builder,
            Full::new(Bytes::from(format!(
                "Please wait {} seconds before retrying",
                shield_seconds
            )))
            .map_err(|e| match e {})
            .boxed(),
            StatusCode::SERVICE_UNAVAILABLE,
            "Please retry later",
        )
    }

    /// 生成行为验证码响应
    fn behavioral_captcha_response(
        &self,
        captcha_type: CaptchaType,
        template_html: &str,
        lang: &str,
    ) -> NodeResponse {
        let (token, captcha_data, _) = self.create_captcha_session(captcha_type);

        // 获取会话中的 PoW 参数
        let (pow_challenge, pow_difficulty) = {
            let shard = self.captcha_sessions.shard(&token);
            let sessions = shard.lock();
            if let Some(session) = sessions.get(&token) {
                (session.pow_challenge.clone(), session.pow_difficulty)
            } else {
                (String::new(), 0u8)
            }
        };

        let body = template_html
            .replace("{{TOKEN}}", &token)
            .replace("{{CAPTCHA_DATA}}", &captcha_data)
            .replace("{{POW_DIFFICULTY}}", &pow_difficulty.to_string())
            .replace("{{POW_CHALLENGE}}", &pow_challenge)
            .replace("{{LANG}}", lang);

        let builder = hyper::Response::builder()
            .status(StatusCode::FORBIDDEN)
            .header("Content-Type", "text/html; charset=utf-8")
            .header("X-WAF-Challenge", "behavioral")
            .header("X-WAF-Token", &token);
        build_response_or_fallback(
            builder,
            Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
            StatusCode::FORBIDDEN,
            "Captcha required",
        )
    }

    /// 生成无感验证响应 (JS Challenge)
    fn js_challenge_response(
        &self,
        template_html: &str,
        lang: &str,
    ) -> NodeResponse {
        let (token, _, _) = self.create_captcha_session(CaptchaType::JsChallenge);

        let (pow_challenge, pow_difficulty) = {
            let shard = self.captcha_sessions.shard(&token);
            let sessions = shard.lock();
            if let Some(session) = sessions.get(&token) {
                (session.pow_challenge.clone(), session.pow_difficulty)
            } else {
                (String::new(), 0u8)
            }
        };

        let body = template_html
            .replace("{{TOKEN}}", &token)
            .replace("{{POW_DIFFICULTY}}", &pow_difficulty.to_string())
            .replace("{{POW_CHALLENGE}}", &pow_challenge)
            .replace("{{LANG}}", lang);

        let builder = hyper::Response::builder()
            .status(StatusCode::FORBIDDEN)
            .header("Content-Type", "text/html; charset=utf-8")
            .header("X-WAF-Challenge", "js")
            .header("X-WAF-Token", &token);
        build_response_or_fallback(
            builder,
            Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
            StatusCode::FORBIDDEN,
            "Captcha required",
        )
    }

    fn issue_challenge(&self, host: &str) -> (String, String) {
        let mut rng = thread_rng();
        let a: i32 = rng.gen_range(10..99);
        let b: i32 = rng.gen_range(1..9);
        let answer = a + b;
        let ts = chrono::Utc::now().timestamp();
        let payload = format!("{}|{}|{}|{}|{}", host, a, b, answer, ts);
        let mut mac = match Hmac::<Sha256>::new_from_slice(&self.challenge_secret) {
            Ok(v) => v,
            Err(e) => {
                warn!(error = %e, "failed to init challenge hmac");
                return (format!("{} + {} = ?", a, b), String::new());
            }
        };
        mac.update(payload.as_bytes());
        let sig = mac.finalize().into_bytes();
        let sig_hex = hex::encode(sig);
        let token = general_purpose::STANDARD.encode([payload.as_bytes(), b"|", sig_hex.as_bytes()].concat());
        (format!("{} + {} = ?", a, b), token)
    }

    fn verify_challenge(&self, client_ip: Option<&str>, headers: &HeaderMap, fail_limit: i64, ban_seconds: i64, ban_mode: &str) -> bool {
        let token = headers
            .get("X-WAF-Token")
            .and_then(|v| v.to_str().ok())
            .or_else(|| headers.get("x-waf-token").and_then(|v| v.to_str().ok()));
        let answer = headers
            .get("X-WAF-Answer")
            .and_then(|v| v.to_str().ok())
            .or_else(|| headers.get("x-waf-answer").and_then(|v| v.to_str().ok()));
        let (token, answer) = match (token, answer) {
            (Some(t), Some(a)) => (t, a),
            _ => {
                self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
                return false;
            }
        };
        let raw = match general_purpose::STANDARD.decode(token) {
            Ok(v) => v,
            Err(_) => {
                self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
                return false;
            }
        };
        let raw_str = match String::from_utf8(raw) {
            Ok(s) => s,
            Err(_) => {
                self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
                return false;
            }
        };
        let parts: Vec<&str> = raw_str.split('|').collect();
        if parts.len() != 6 {
            self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
            return false;
        }
        let a: i32 = parts[1].parse().unwrap_or(0);
        let b: i32 = parts[2].parse().unwrap_or(0);
        let expect: i32 = parts[3].parse().unwrap_or(i32::MAX);
        let ts: i64 = parts[4].parse().unwrap_or(0);
        let sig_hex = parts[5];
        if chrono::Utc::now().timestamp() - ts > 900 {
            self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
            return false;
        }
        let payload = format!("{}|{}|{}|{}|{}", parts[0], a, b, expect, ts);
        let mut mac = match Hmac::<Sha256>::new_from_slice(&self.challenge_secret) {
            Ok(v) => v,
            Err(e) => {
                warn!(error = %e, "failed to init challenge hmac");
                self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
                return false;
            }
        };
        mac.update(payload.as_bytes());
        let sig = mac.finalize().into_bytes();
        let sig_bytes = match hex::decode(sig_hex) {
            Ok(v) => v,
            Err(_) => {
                self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
                return false;
            }
        };
        if sig.to_vec() != sig_bytes {
            self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
            return false;
        }
        let ans: i32 = answer.trim().parse().unwrap_or(i32::MAX);
        let ok = ans == expect || ans == a + b;
        if ok {
            self.clear_fail(client_ip);
        } else {
            self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
        }
        ok
    }

    fn is_banned(&self, client_ip: Option<&str>) -> Option<String> {
        let Some(ip) = client_ip else { return None };
        let shard = self.waf_ban.shard(ip);
        let mut guard = shard.lock();
        if let Some(state) = guard.get_mut(ip) {
            let now = Instant::now();
            if now < state.until {
                state.last_seen = now;
                return Some(state.mode.as_str().to_string());
            }
            guard.remove(ip);
        }
        None
    }

    fn record_fail(&self, client_ip: Option<&str>, limit: i64, ban_seconds: i64, ban_mode: &str) {
        let Some(ip) = client_ip else { return };
        let limit = if limit <= 0 { 3 } else { limit };
        let shard = self.waf_fail.shard(ip);
        let mut fail = shard.lock();
        let entry = fail.entry(ip.to_string()).or_insert(FailState {
            count: 0,
            first: Instant::now(),
            last_seen: Instant::now(),
        });
        entry.count += 1;
        entry.last_seen = Instant::now();
        if entry.count >= limit as u32 {
            let bans_shard = self.waf_ban.shard(ip);
            let mut bans = bans_shard.lock();
            let mode = BanMode::from_str(ban_mode);
            let base = if ban_seconds > 0 { ban_seconds } else { 600 };
            let base = base.max(600);
            let strikes = bans.get(ip).map(|b| b.strikes + 1).unwrap_or(1);
            let dur = base as u64 * strikes as u64;
            bans.insert(
                ip.to_string(),
                BanState {
                    until: Instant::now() + Duration::from_secs(dur),
                    strikes,
                    mode,
                    last_seen: Instant::now(),
                },
            );
            entry.count = 0;
            entry.first = Instant::now();

            let mode_str = mode.as_str();

            // 记录拉黑事件到日志
            self.log_ban_event(ip, strikes, dur, mode_str);

            // 实时上报拉黑事件到主控
            if let Some(tx) = &self.ban_event_tx {
                let expires_at_unix = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .map(|d| d.as_secs() as i64 + dur as i64)
                    .unwrap_or(0);
                let event = BanEvent {
                    ip: ip.to_string(),
                    reason: mode_str.to_string(),
                    strikes,
                    expires_at_unix,
                };
                let _ = tx.send(event);
            }
        }
    }

    fn clear_fail(&self, client_ip: Option<&str>) {
        if let Some(ip) = client_ip {
            let shard = self.waf_fail.shard(ip);
            shard.lock().remove(ip);
        }
    }

    /// 记录IP拉黑事件到日志（用于ES分析）
    fn log_ban_event(&self, ip: &str, strikes: u32, duration_secs: u64, mode: &str) {
        let Some(logger) = self.access_logger.as_ref() else { return };

        #[derive(Serialize)]
        struct BanLog<'a> {
            #[serde(rename = "@timestamp")]
            ts: String,
            event_type: &'static str,
            client_ip: &'a str,
            strikes: u32,
            duration_secs: u64,
            ban_mode: &'a str,
            node_id: &'a str,
            node: &'a str,
        }

        let ts = Utc::now().to_rfc3339_opts(SecondsFormat::Millis, true);
        let node_id = self.node_id.as_deref().unwrap_or("");

        let entry = BanLog {
            ts,
            event_type: "waf_ban",
            client_ip: ip,
            strikes,
            duration_secs,
            ban_mode: mode,
            node_id,
            node: &self.node_hostname,
        };

        logger.log(&entry);
        warn!("WAF banned IP: {} strikes={} duration={}s mode={}", ip, strikes, duration_secs, mode);
    }

    /// 收集本地拉黑的IP用于上报到主控
    pub fn collect_waf_bans(&self) -> Vec<(String, String, u32, u64)> {
        let now = Instant::now();
        let mut result = Vec::new();
        for shard in self.waf_ban.iter_shards() {
            let guard = shard.lock();
            for (ip, state) in guard.iter() {
                if now < state.until {
                    let remaining_secs = state.until.duration_since(now).as_secs();
                    result.push((
                        ip.clone(),
                        state.mode.as_str().to_string(),
                        state.strikes,
                        remaining_secs,
                    ));
                }
            }
        }
        result
    }

    fn generate_request_id() -> String {
        let mut rng = thread_rng();
        let rand: u64 = rng.gen();
        format!("{:016x}", rand)
    }

    /// 生成行为验证码会话
    fn create_captcha_session(&self, captcha_type: CaptchaType) -> (String, String, CaptchaType) {
        let mut rng = thread_rng();

        // 生成 token
        let token_bytes: [u8; 16] = rng.gen();
        let token = hex::encode(token_bytes);

        // 生成 PoW challenge
        let pow_challenge: [u8; 8] = rng.gen();
        let pow_challenge_str = hex::encode(pow_challenge);
        let pow_difficulty: u8 = 2; // 默认难度

        // 生成验证码数据
        let (captcha_data, answer) = match captcha_type {
            CaptchaType::Slide => {
                let (data, ans) = if self.captcha_pool.enabled {
                    self.captcha_pool
                        .try_take_slide()
                        .unwrap_or_else(captcha::generate_slide_captcha)
                } else {
                    captcha::generate_slide_captcha()
                };
                (serde_json::to_string(&data).unwrap_or_default(), ans)
            }
            CaptchaType::Click => {
                let (data, ans) = if self.captcha_pool.enabled {
                    self.captcha_pool
                        .try_take_click()
                        .unwrap_or_else(captcha::generate_click_captcha)
                } else {
                    captcha::generate_click_captcha()
                };
                (serde_json::to_string(&data).unwrap_or_default(), ans)
            }
            CaptchaType::Rotate => {
                let (data, ans) = if self.captcha_pool.enabled {
                    self.captcha_pool
                        .try_take_rotate()
                        .unwrap_or_else(captcha::generate_rotate_captcha)
                } else {
                    captcha::generate_rotate_captcha()
                };
                (serde_json::to_string(&data).unwrap_or_default(), ans)
            }
            CaptchaType::SlideRegion => {
                let (data, ans) = if self.captcha_pool.enabled {
                    self.captcha_pool
                        .try_take_slide()
                        .unwrap_or_else(captcha::generate_slide_captcha)
                } else {
                    captcha::generate_slide_captcha()
                };
                (serde_json::to_string(&data).unwrap_or_default(), ans)
            }
            CaptchaType::JsChallenge => {
                // 无感验证不需要图片数据
                let ans = CaptchaAnswer {
                    captcha_type: CaptchaType::JsChallenge,
                    slide_x: None,
                    click_dots: None,
                    rotate_angle: None,
                };
                (String::new(), ans)
            }
        };

        // 存储会话
        let session = CaptchaSession {
            captcha_type,
            answer,
            pow_challenge: pow_challenge_str.clone(),
            pow_difficulty,
            created_at: Instant::now(),
            used: false,
        };

        let shard = self.captcha_sessions.shard(&token);
        shard.lock().insert(token.clone(), session);

        (token, captcha_data, captcha_type)
    }

    /// 验证行为验证码
    fn verify_captcha_session(&self, req: &CaptchaVerifyRequest) -> bool {
        // Snapshot session data quickly; avoid holding the lock during verification logic.
        let (captcha_type, answer, pow_challenge, pow_difficulty) = {
            let shard = self.captcha_sessions.shard(&req.token);
            let mut sessions = shard.lock();
            let session = match sessions.get_mut(&req.token) {
                Some(s) => s,
                None => return false,
            };

            if session.used {
                return false;
            }
            if session.created_at.elapsed() > Duration::from_secs(300) {
                sessions.remove(&req.token);
                return false;
            }

            (
                session.captcha_type,
                session.answer.clone(),
                session.pow_challenge.clone(),
                session.pow_difficulty,
            )
        };

        // 验证 PoW
        if let Some(ref pow) = req.pow {
            if !self.verify_pow(&pow_challenge, pow_difficulty, pow) {
                return false;
            }
        }

        // 验证轨迹
        if let Some(ref trajectory) = req.trajectory {
            if !Self::analyze_trajectory(trajectory) {
                return false;
            }
        }

        // 根据类型验证答案
        let valid = match captcha_type {
            CaptchaType::Slide | CaptchaType::SlideRegion => {
                self.verify_slide_answer(&answer, req.point.as_ref())
            }
            CaptchaType::Click => {
                self.verify_click_answer(&answer, req.dots.as_ref())
            }
            CaptchaType::Rotate => {
                self.verify_rotate_answer(&answer, req.angle)
            }
            CaptchaType::JsChallenge => {
                // 无感验证只需要 PoW 和指纹验证
                self.verify_js_challenge(req.fingerprint.as_ref())
            }
        };

        if valid {
            let shard = self.captcha_sessions.shard(&req.token);
            let mut sessions = shard.lock();
            let session = match sessions.get_mut(&req.token) {
                Some(s) => s,
                None => return false,
            };
            if session.used {
                return false;
            }
            if session.created_at.elapsed() > Duration::from_secs(300) {
                sessions.remove(&req.token);
                return false;
            }
            session.used = true;
        }

        valid
    }

    /// 处理验证码验证请求
    async fn handle_captcha_verify(
        &self,
        req: Request<Incoming>,
        client_ip: Option<&str>,
        started: &std::time::Instant,
        host: &str,
        uri: &http::Uri,
    ) -> Result<NodeResponse> {
        // 默认失败限制和封禁参数
        let fail_limit = 5i64;
        let ban_seconds = 300i64;
        let ban_mode = "ipset";

        // 读取请求体
        let body_bytes = match http_body_util::BodyExt::collect(req.into_body()).await {
            Ok(collected) => collected.to_bytes(),
            Err(_) => {
                self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
                let resp = Self::json_response(StatusCode::BAD_REQUEST, r#"{"ok":false,"error":"invalid body"}"#);
                self.log_access(started, client_ip.and_then(|s| s.parse::<IpAddr>().ok()), host, uri, "POST", 400, "BYPASS", 0, "captcha_invalid_body");
                return Ok(resp);
            }
        };

        // 解析验证请求
        let verify_req: CaptchaVerifyRequest = match serde_json::from_slice(&body_bytes) {
            Ok(r) => r,
            Err(_) => {
                self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
                let resp = Self::json_response(StatusCode::BAD_REQUEST, r#"{"ok":false,"error":"invalid json"}"#);
                self.log_access(started, client_ip.and_then(|s| s.parse::<IpAddr>().ok()), host, uri, "POST", 400, "BYPASS", 0, "captcha_invalid_json");
                return Ok(resp);
            }
        };

        // 验证
        let valid = self.verify_captcha_session(&verify_req);

        if valid {
            // 验证成功，清除失败记录
            self.clear_fail(client_ip);
            let resp = Self::json_response(StatusCode::OK, r#"{"ok":true}"#);
            self.log_access(started, client_ip.and_then(|s| s.parse::<IpAddr>().ok()), host, uri, "POST", 200, "BYPASS", 0, "captcha_success");
            Ok(resp)
        } else {
            // 验证失败，记录失败并可能拉黑IP
            self.record_fail(client_ip, fail_limit, ban_seconds, ban_mode);
            let resp = Self::json_response(StatusCode::FORBIDDEN, r#"{"ok":false,"error":"verification failed"}"#);
            self.log_access(started, client_ip.and_then(|s| s.parse::<IpAddr>().ok()), host, uri, "POST", 403, "BYPASS", 0, "captcha_fail");
            Ok(resp)
        }
    }

    fn json_response(status: StatusCode, body: &str) -> NodeResponse {
        let builder = hyper::Response::builder()
            .status(status)
            .header("Content-Type", "application/json; charset=utf-8");
        build_response_or_fallback(
            builder,
            Full::new(Bytes::from(body.to_string()))
                .map_err(|e| match e {})
                .boxed(),
            status,
            body,
        )
    }

    /// 验证 PoW
    fn verify_pow(&self, challenge: &str, difficulty: u8, pow: &PowResult) -> bool {
        if difficulty == 0 {
            return true;
        }
        let data = format!("{}:{}", challenge, pow.nonce);
        let hash = Self::simple_hash(&data);
        let prefix = "0".repeat(difficulty as usize);
        hash.starts_with(&prefix)
    }

    fn simple_hash(s: &str) -> String {
        let mut hash: u32 = 0x811c9dc5;
        for b in s.bytes() {
            hash ^= b as u32;
            hash = hash.wrapping_mul(0x01000193);
        }
        format!("{:08x}", hash)
    }

    /// 轨迹分析 - 检测机器人行为
    fn analyze_trajectory(trajectory: &[TrajectoryPoint]) -> bool {
        if trajectory.len() < 5 {
            return false; // 轨迹点太少
        }

        // 检查时间间隔是否合理
        let mut prev_t = trajectory[0].t;
        let mut same_interval_count = 0;
        for pt in trajectory.iter().skip(1) {
            let interval = pt.t - prev_t;
            if interval == 0 {
                same_interval_count += 1;
            }
            prev_t = pt.t;
        }
        // 如果超过50%的点时间间隔相同，可能是机器人
        if same_interval_count > trajectory.len() / 2 {
            return false;
        }

        // 检查是否有速度变化（人类操作通常有加速减速）
        let mut speeds: Vec<f64> = Vec::new();
        for i in 1..trajectory.len() {
            let dx = (trajectory[i].x - trajectory[i - 1].x) as f64;
            let dy = (trajectory[i].y - trajectory[i - 1].y) as f64;
            let dt = (trajectory[i].t - trajectory[i - 1].t).max(1) as f64;
            let speed = (dx * dx + dy * dy).sqrt() / dt;
            speeds.push(speed);
        }

        if speeds.is_empty() {
            return false;
        }

        // 计算速度标准差
        let avg_speed: f64 = speeds.iter().sum::<f64>() / speeds.len() as f64;
        let variance: f64 = speeds.iter()
            .map(|s| (s - avg_speed).powi(2))
            .sum::<f64>() / speeds.len() as f64;
        let std_dev = variance.sqrt();

        // 如果速度完全一致（标准差接近0），可能是机器人
        if std_dev < 0.01 && speeds.len() > 10 {
            return false;
        }

        true
    }

    /// 验证滑块答案
    fn verify_slide_answer(&self, answer: &CaptchaAnswer, point: Option<&CaptchaPoint>) -> bool {
        let point = match point {
            Some(p) => p,
            None => return false,
        };
        let expected_x = match answer.slide_x {
            Some(x) => x as i32,
            None => return false,
        };
        // 允许 5 像素误差
        (point.x - expected_x).abs() <= 5
    }

    /// 验证点选答案
    fn verify_click_answer(&self, answer: &CaptchaAnswer, dots: Option<&Vec<CaptchaPoint>>) -> bool {
        let dots = match dots {
            Some(d) => d,
            None => return false,
        };
        let expected = match &answer.click_dots {
            Some(e) => e,
            None => return false,
        };
        if dots.len() != expected.len() {
            return false;
        }
        // 检查每个点是否在允许范围内（20像素）
        for (i, dot) in dots.iter().enumerate() {
            let (ex, ey) = expected[i];
            let dx = (dot.x - ex as i32).abs();
            let dy = (dot.y - ey as i32).abs();
            if dx > 20 || dy > 20 {
                return false;
            }
        }
        true
    }

    /// 验证旋转答案
    fn verify_rotate_answer(&self, answer: &CaptchaAnswer, angle: Option<u32>) -> bool {
        let angle = match angle {
            Some(a) => a,
            None => return false,
        };
        let expected = match answer.rotate_angle {
            Some(e) => e,
            None => return false,
        };
        // 允许 10 度误差
        let diff = if angle > expected {
            angle - expected
        } else {
            expected - angle
        };
        diff <= 10 || diff >= 350 // 处理 0/360 边界
    }

    /// 验证无感验证 (JS Challenge)
    fn verify_js_challenge(&self, fp: Option<&BrowserFingerprint>) -> bool {
        let fp = match fp {
            Some(f) => f,
            None => return false,
        };

        // 检查必要字段
        if fp.ua.is_none() || fp.ts.is_none() {
            return false;
        }

        // 检查 UA 是否合理
        let ua = match fp.ua.as_ref() {
            Some(v) => v,
            None => return false,
        };
        if ua.is_empty() || ua.len() < 10 {
            return false;
        }

        // 检查时间戳是否合理 (不能太旧)
        let ts = match fp.ts {
            Some(v) => v,
            None => return false,
        };
        let now = chrono::Utc::now().timestamp_millis();
        if (now - ts).abs() > 60000 {
            return false;
        }

        // 基本的机器人检测
        let ua_lower = ua.to_lowercase();
        let bot_keywords = ["bot", "crawler", "spider", "headless", "phantom"];
        for kw in &bot_keywords {
            if ua_lower.contains(kw) {
                return false;
            }
        }

        true
    }

    /// 清理过期的验证码会话
    fn sweep_captcha_sessions(&self) {
        for shard in self.captcha_sessions.iter_shards() {
            let mut sessions = shard.lock();
            sessions.retain(|_, s| s.created_at.elapsed() < Duration::from_secs(600));
        }
    }
}

fn build_friendly_error(
    status: StatusCode,
    title: &str,
    message: &str,
    host: &str,
    path: &str,
    accept_json: bool,
    custom_pages: Option<&Vec<crate::config::ErrorPage>>,
    global_templates: Option<&crate::config::GlobalTemplates>,
) -> NodeResponse {
    let req_id = ProxyService::generate_request_id();
    let placeholders = |text: &str| apply_placeholders(text, status.as_u16(), host, path, &req_id);
    let builder = hyper::Response::builder();

    if let Some(pages) = custom_pages {
        if let Some(page) = pages.iter().find(|p| p.status as u16 == status.as_u16()) {
            match page.mode.as_str() {
                "redirect" => {
                    return build_response_or_fallback(
                        builder
                        .status(StatusCode::FOUND)
                        .header(http::header::LOCATION, page.content.as_str())
                        .header("X-CDN-Request-ID", req_id.as_str())
                        .header("X-CDN-Error-Page", format!("custom-{}", status.as_u16())),
                        Full::new(Bytes::from_static(b""))
                            .map_err(|e| match e {})
                            .boxed(),
                        StatusCode::FOUND,
                        "redirect",
                    );
                }
                "json" => {
                    let body = placeholders(&page.content);
                    return build_response_or_fallback(
                        builder
                        .status(status)
                        .header(CONTENT_TYPE, "application/json; charset=utf-8")
                        .header(CACHE_CONTROL, "no-cache")
                        .header("X-CDN-Request-ID", req_id.as_str())
                        .header("X-CDN-Error-Page", format!("custom-{}", status.as_u16())),
                        Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
                        status,
                        "custom error page",
                    );
                }
                _ => {
                    let body = placeholders(&page.content);
                    return build_response_or_fallback(
                        builder
                        .status(status)
                        .header(CONTENT_TYPE, "text/html; charset=utf-8")
                        .header(CACHE_CONTROL, "no-cache")
                        .header("X-CDN-Request-ID", req_id.as_str())
                        .header("X-CDN-Error-Page", format!("custom-{}", status.as_u16())),
                        Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
                        status,
                        "custom error page",
                    );
                }
            }
        }
    }

    if !accept_json {
        if let Some(tpl) = global_templates {
            let status_code = status.as_u16() as i32;
            let template_html = tpl
                .error_pages
                .get(&status_code)
                .map(|s| s.as_str())
                .filter(|s| !s.trim().is_empty())
                .or_else(|| {
                    let d = tpl.error_default.as_str();
                    if d.trim().is_empty() { None } else { Some(d) }
                });
            if let Some(raw) = template_html {
                let body = placeholders(raw);
                return build_response_or_fallback(
                    builder
                    .status(status)
                    .header(CONTENT_TYPE, "text/html; charset=utf-8")
                    .header(CACHE_CONTROL, "no-cache")
                    .header("X-CDN-Request-ID", req_id.as_str())
                    .header("X-CDN-Error-Page", format!("global-{}", status.as_u16())),
                    Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
                    status,
                    "global error template",
                );
            }
        }
    }

    let body_json = serde_json::json!({
        "error": status.as_u16(),
        "title": title,
        "message": message,
        "host": host,
        "path": path,
        "request_id": req_id,
    });
    if accept_json {
        let body = serde_json::to_vec_pretty(&body_json).unwrap_or_default();
        return build_response_or_fallback(
            builder
            .status(status)
            .header(CONTENT_TYPE, "application/json; charset=utf-8")
            .header(CACHE_CONTROL, "no-cache")
            .header("X-CDN-Request-ID", req_id.as_str())
            .header("X-CDN-Error-Page", format!("default-{}", status.as_u16())),
            Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
            status,
            "json error",
        );
    }

    let html = format!(
        "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>{status}</title><style>\
        body{{background:#0b1224;color:#e2e8f0;font-family:Inter,system-ui,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;}}\
        .card{{background:#0f172a;padding:32px;border-radius:16px;max-width:640px;box-shadow:0 10px 40px rgba(0,0,0,0.3);}}\
        h1{{margin:0 0 12px;font-size:24px;}}p{{margin:4px 0;color:#cbd5e1;}}code{{background:#111827;padding:2px 6px;border-radius:6px;}}\
        </style></head><body><div class=\"card\"><h1>{title}</h1><p>{message}</p><p>主机：<code>{host}</code></p><p>路径：<code>{path}</code></p><p>请求编号：<code>{req_id}</code></p></div></body></html>",
        status = status.as_u16(),
        title = title,
        message = message,
        host = host,
        path = path,
        req_id = req_id
    );
    build_response_or_fallback(
        builder
        .status(status)
        .header(CONTENT_TYPE, "text/html; charset=utf-8")
        .header(CACHE_CONTROL, "no-cache")
        .header("X-CDN-Request-ID", req_id.as_str())
        .header("X-CDN-Error-Page", format!("default-{}", status.as_u16())),
        Full::new(Bytes::from(html)).map_err(|e| match e {}).boxed(),
        status,
        "html error",
    )
}

/// Render a page for requests whose Host header does not match any configured
/// domain. When the Host looks like an IP address (direct node access), we
/// show the `direct_ip` template; otherwise (CNAME pointing here but domain
/// not configured) we show the `cname_not_found` template. Both templates
/// are admin-editable through the global template system.
fn build_unmatched_host_page(
    host: &str,
    path: &str,
    accept_json: bool,
    templates: &crate::config::GlobalTemplates,
) -> NodeResponse {
    let req_id = ProxyService::generate_request_id();
    let is_ip = host.parse::<std::net::IpAddr>().is_ok();

    // JSON callers get a structured response regardless.
    if accept_json {
        let title = if is_ip { "CDN 节点" } else { "站点未找到" };
        let message = if is_ip {
            "该节点仅为已配置的域名提供加速服务，不支持通过 IP 直接访问"
        } else {
            "请求的域名未配置或已停用"
        };
        let body_json = serde_json::json!({
            "error": if is_ip { 403u16 } else { 404u16 },
            "title": title,
            "message": message,
            "host": host,
            "path": path,
            "request_id": req_id,
        });
        let body = serde_json::to_vec_pretty(&body_json).unwrap_or_default();
        let status = if is_ip { StatusCode::FORBIDDEN } else { StatusCode::NOT_FOUND };
        return build_response_or_fallback(
            hyper::Response::builder()
                .status(status)
                .header(CONTENT_TYPE, "application/json; charset=utf-8")
                .header(CACHE_CONTROL, "no-cache")
                .header("X-CDN-Request-ID", req_id.as_str()),
            Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
            status,
            "json unmatched host",
        );
    }

    let (template_html, status, fallback_title, fallback_msg) = if is_ip {
        (
            &templates.direct_ip,
            StatusCode::FORBIDDEN,
            "CDN 节点运行中",
            "您正在直接访问 CDN 加速节点。本节点仅为已配置的域名提供加速服务，不支持通过 IP 直接访问。",
        )
    } else {
        (
            &templates.cname_not_found,
            StatusCode::NOT_FOUND,
            "站点未找到",
            "请求的域名未配置或已停用",
        )
    };

    // Use the admin-customized template if available.
    if !template_html.trim().is_empty() {
        let body = apply_placeholders(template_html, status.as_u16(), host, path, &req_id);
        return build_response_or_fallback(
            hyper::Response::builder()
                .status(status)
                .header(CONTENT_TYPE, "text/html; charset=utf-8")
                .header(CACHE_CONTROL, "no-cache")
                .header("X-CDN-Request-ID", req_id.as_str())
                .header("X-CDN-Error-Page", if is_ip { "template-direct-ip" } else { "template-cname-not-found" }),
            Full::new(Bytes::from(body)).map_err(|e| match e {}).boxed(),
            status,
            "template unmatched host",
        );
    }

    // Hardcoded fallback when no template is configured.
    let html = format!(
        "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>{status}</title><style>\
        body{{background:#0b1224;color:#e2e8f0;font-family:Inter,system-ui,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;}}\
        .card{{background:#0f172a;padding:32px;border-radius:16px;max-width:640px;box-shadow:0 10px 40px rgba(0,0,0,0.3);}}\
        h1{{margin:0 0 12px;font-size:24px;}}p{{margin:4px 0;color:#cbd5e1;}}code{{background:#111827;padding:2px 6px;border-radius:6px;}}\
        </style></head><body><div class=\"card\"><h1>{title}</h1><p>{message}</p><p>主机：<code>{host}</code></p><p>路径：<code>{path}</code></p><p>请求编号：<code>{req_id}</code></p></div></body></html>",
        status = status.as_u16(),
        title = fallback_title,
        message = fallback_msg,
        host = host,
        path = path,
        req_id = req_id
    );
    build_response_or_fallback(
        hyper::Response::builder()
            .status(status)
            .header(CONTENT_TYPE, "text/html; charset=utf-8")
            .header(CACHE_CONTROL, "no-cache")
            .header("X-CDN-Request-ID", req_id.as_str()),
        Full::new(Bytes::from(html)).map_err(|e| match e {}).boxed(),
        status,
        "fallback unmatched host",
    )
}

fn domain_security_rule_matches(
	rule_type: &str,
	value: &str,
	path: &str,
	headers: &HeaderMap,
) -> bool {
	match rule_type {
		"default" => true,
		"path_match" => {
			!value.is_empty() && path.starts_with(value)
		}
		"header_match" => {
			let name = value.strip_prefix("header:").unwrap_or(value);
			!name.is_empty() && headers.contains_key(name.trim())
		}
		"ua_match" => {
			let sub = value.strip_prefix("ua:").unwrap_or(value).to_ascii_lowercase();
			if sub.is_empty() {
				return false;
			}
			headers
				.get("User-Agent")
				.and_then(|v| v.to_str().ok())
				.map(|ua| ua.to_ascii_lowercase().contains(&sub))
				.unwrap_or(false)
		}
		_ => false,
	}
}

/// Check if a visitor's country should be blocked given the geo_countries list.
/// Supports magic tokens:
///   `__FOREIGN_EXCLUDE_HKMOTW__` → block everything except CN, HK, MO, TW
///   `__FOREIGN_INCLUDE_HKMOTW__` → block everything except CN (HK/MO/TW ARE blocked)
/// Otherwise falls back to a simple membership check.
fn is_geo_blocked(geo_countries: &[String], country: &str) -> bool {
    let uc = country.to_ascii_uppercase();
    for code in geo_countries {
        match code.as_str() {
            "__FOREIGN_EXCLUDE_HKMOTW__" => {
                // Allow CN, HK, MO, TW — block everything else
                return !matches!(uc.as_str(), "CN" | "HK" | "MO" | "TW");
            }
            "__FOREIGN_INCLUDE_HKMOTW__" => {
                // Allow CN only — block everything else (including HK, MO, TW)
                return uc != "CN";
            }
            _ => {}
        }
    }
    // Plain country code list
    geo_countries.iter().any(|c| c.eq_ignore_ascii_case(&uc))
}

fn apply_placeholders(text: &str, status: u16, host: &str, path: &str, req_id: &str) -> String {
    text.replace("{{status}}", &status.to_string())
        .replace("{{host}}", host)
        .replace("{{path}}", path)
        .replace("{{request_id}}", req_id)
}

struct WafDomainStat {
    count: u64,
    window_start: Instant,
    challenge_active: bool,
    last_over: Instant,
    last_under: Instant,
    last_seen: Instant,
}

struct FailState {
    count: u32,
    first: Instant,
    last_seen: Instant,
}

#[derive(Clone, Copy)]
enum BanMode {
    Ipset,
    Drop,
    Page,
}

impl BanMode {
    fn from_str(s: &str) -> Self {
        match s.to_ascii_lowercase().as_str() {
            "drop" => BanMode::Drop,
            "page" => BanMode::Page,
            _ => BanMode::Ipset,
        }
    }
    fn as_str(&self) -> &'static str {
        match self {
            BanMode::Ipset => "ipset",
            BanMode::Drop => "drop",
            BanMode::Page => "page",
        }
    }
}

struct BanState {
    until: Instant,
    strikes: u32,
    mode: BanMode,
    last_seen: Instant,
}

/// Per-IP rate limit counter with short-lived window.
struct RateLimitEntry {
    count: u32,
    window_start: Instant,
}

/// 验证码会话
struct CaptchaSession {
    captcha_type: CaptchaType,
    answer: CaptchaAnswer,
    pow_challenge: String,
    pow_difficulty: u8,
    created_at: Instant,
    used: bool,
}

/// 验证请求体
#[derive(Debug, Deserialize)]
struct CaptchaVerifyRequest {
    token: String,
    #[serde(rename = "type")]
    #[allow(dead_code)]
    captcha_type: String,
    // 滑块验证
    point: Option<CaptchaPoint>,
    // 点选验证
    dots: Option<Vec<CaptchaPoint>>,
    // 旋转验证
    angle: Option<u32>,
    // 轨迹数据
    trajectory: Option<Vec<TrajectoryPoint>>,
    // PoW 结果
    pow: Option<PowResult>,
    // 无感验证 - 浏览器指纹
    fingerprint: Option<BrowserFingerprint>,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct BrowserFingerprint {
    ua: Option<String>,
    lang: Option<String>,
    platform: Option<String>,
    cores: Option<u32>,
    memory: Option<u32>,
    screen: Option<String>,
    tz: Option<String>,
    touch: Option<bool>,
    webgl: Option<String>,
    canvas: Option<String>,
    ts: Option<i64>,
}

#[derive(Debug, Deserialize)]
struct CaptchaPoint {
    x: i32,
    y: i32,
}

#[derive(Debug, Deserialize)]
struct TrajectoryPoint {
    x: i32,
    y: i32,
    t: i64,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct PowResult {
    nonce: u64,
    hash: String,
}
