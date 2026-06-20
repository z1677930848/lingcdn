use crate::config::RuntimeConfig;
use regex::Regex;
use std::collections::HashMap;
use std::sync::Arc;
use tracing::debug;

/// Route matching result
#[derive(Debug, Clone)]
pub struct RouteMatch {
    pub domain_idx: usize,
}

#[derive(Debug, Clone, Copy)]
pub struct CacheDecision {
    pub ttl_seconds: u64,
    pub cache_query_params: bool,
    pub stale_while_revalidate_seconds: u64,
    pub stale_if_error_seconds: u64,
}

/// Router for matching requests to domains and origins
pub struct Router {
    config: Arc<RuntimeConfig>,
    exact_domains: HashMap<String, usize>,
    wildcard_domains: HashMap<String, usize>,
    compiled_cache_rules: Vec<CompiledCacheRule>,
}

struct CompiledCacheRule {
    rule: crate::config::CacheRule,
    host_re: Option<Regex>,
    path_re: Option<Regex>,
}

impl Router {
    pub fn new(config: Arc<RuntimeConfig>) -> Self {
        let mut exact_domains: HashMap<String, usize> =
            HashMap::with_capacity(config.domains.len());
        let mut wildcard_domains: HashMap<String, usize> = HashMap::new();

        for (idx, domain) in config.domains.iter().enumerate() {
            let name = domain.name.to_ascii_lowercase();
            if name.starts_with("*.") {
                // Preserve "first match wins" semantics.
                wildcard_domains.entry(name[2..].to_string()).or_insert(idx);
            } else {
                // Preserve "first match wins" semantics from the linear scan implementation.
                exact_domains.entry(name).or_insert(idx);
            }
        }

        let mut compiled_cache_rules: Vec<CompiledCacheRule> =
            Vec::with_capacity(config.cache_rules.len());
        for rule in &config.cache_rules {
            let host_re = rule
                .host_pattern
                .as_deref()
                .and_then(Self::compile_wildcard_pattern);
            let path_re = rule
                .path_pattern
                .as_deref()
                .and_then(Self::compile_wildcard_pattern);
            compiled_cache_rules.push(CompiledCacheRule {
                rule: rule.clone(),
                host_re,
                path_re,
            });
        }

        Self {
            config,
            exact_domains,
            wildcard_domains,
            compiled_cache_rules,
        }
    }

    /// Match a request by host, path, method, and optional listener port.
    pub fn route_with_port(
        &self,
        host: &str,
        path: &str,
        method: &str,
        incoming_port: Option<u16>,
    ) -> Option<RouteMatch> {
        debug!(
            "Routing request: host={}, path={}, method={}",
            host, path, method
        );

        // Fast path: exact match.
        if let Some(&idx) = self.exact_domains.get(host) {
            if let Some(domain) = self.config.domains.get(idx) {
                if Self::domain_accepts_port(domain, incoming_port) {
                    debug!("Matched domain: {}", domain.name);
                    return Some(RouteMatch { domain_idx: idx });
                }
            }
        }

        // Wildcard match (e.g., *.example.com).
        // 1) The apex itself: a config of "*.example.com" also accepts a request
        //    for the bare "example.com" when no exact entry exists.
        // 2) Sub-label match: a `*.example.com` config matches exactly one
        //    DNS label below the apex (e.g. "a.example.com"), per RFC 6125
        //    §6.4.3 — *not* arbitrary depth. The previous suffix walk was
        //    inconsistent with the listener's TLS SNI resolver, which only
        //    matches a single label, and would route requests for
        //    "a.b.example.com" through the `*.example.com` domain config
        //    even though the TLS handshake would refuse the cert. That gap
        //    let an attacker reach domain config / WAF / origin behavior
        //    bound to `*.example.com` from any deeper sub-label.
        if let Some(&idx) = self.wildcard_domains.get(host) {
            if let Some(domain) = self.config.domains.get(idx) {
                if Self::domain_accepts_port(domain, incoming_port) {
                    debug!("Matched wildcard apex: {}", domain.name);
                    return Some(RouteMatch { domain_idx: idx });
                }
            }
        }
        if let Some(dot_pos) = host.find('.') {
            let suffix = &host[dot_pos + 1..];
            if let Some(&idx) = self.wildcard_domains.get(suffix) {
                if let Some(domain) = self.config.domains.get(idx) {
                    if Self::domain_accepts_port(domain, incoming_port) {
                        debug!("Matched wildcard domain: {}", domain.name);
                        return Some(RouteMatch { domain_idx: idx });
                    }
                }
            }
        }

        debug!("No matching domain found for host: {}", host);
        None
    }

    fn domain_accepts_port(
        domain: &crate::config::DomainConfig,
        incoming_port: Option<u16>,
    ) -> bool {
        match (domain.effective_listen_port(), incoming_port) {
            (Some(expected), Some(actual)) => expected == actual,
            (Some(_), None) => false,
            (None, _) => true,
        }
    }

    /// Check if a request should be cached based on cache rules
    #[allow(dead_code)]
    // 预留：简化调用方只关心 TTL 的场景
    pub fn should_cache(&self, host: &str, path: &str, method: &str) -> Option<u64> {
        self.cache_decision(host, path, method)
            .map(|d| d.ttl_seconds)
    }

    /// Return cache behavior for a request (TTL + whether to include query params in cache key).
    pub fn cache_decision(&self, host: &str, path: &str, method: &str) -> Option<CacheDecision> {
        for rule in &self.compiled_cache_rules {
            if self.matches_cache_rule(rule, host, path, method) {
                debug!("Cache rule matched: ttl={}s", rule.rule.ttl_seconds);
                return Some(CacheDecision {
                    ttl_seconds: rule.rule.ttl_seconds,
                    cache_query_params: rule.rule.cache_query_params,
                    stale_while_revalidate_seconds: rule.rule.stale_while_revalidate_seconds,
                    stale_if_error_seconds: rule.rule.stale_if_error_seconds,
                });
            }
        }
        None
    }

    fn matches_cache_rule(
        &self,
        rule: &CompiledCacheRule,
        host: &str,
        path: &str,
        method: &str,
    ) -> bool {
        // Check host pattern
        if let Some(ref pattern) = rule.rule.host_pattern {
            if !Self::matches_pattern(pattern, host, rule.host_re.as_ref()) {
                return false;
            }
        }

        // Check path pattern
        if let Some(ref pattern) = rule.rule.path_pattern {
            if !Self::matches_pattern(pattern, path, rule.path_re.as_ref()) {
                return false;
            }
        }

        // Check method (avoid per-request allocation)
        if !rule.rule.methods.is_empty()
            && !rule
                .rule
                .methods
                .iter()
                .any(|m| m.eq_ignore_ascii_case(method))
        {
            return false;
        }

        true
    }

    fn matches_pattern(pattern: &str, value: &str, compiled: Option<&Regex>) -> bool {
        // Simple wildcard matching (can be enhanced with regex)
        if pattern == "*" {
            return true;
        }

        if let Some(re) = compiled {
            return re.is_match(value);
        }

        pattern == value
    }

    fn compile_wildcard_pattern(pattern: &str) -> Option<Regex> {
        if pattern == "*" {
            return None;
        }
        if !pattern.contains('*') {
            return None;
        }
        // Keep parity with the previous implementation: only escape '.' and expand '*'.
        let regex_pattern = pattern.replace(".", r"\.").replace("*", ".*");
        Regex::new(&format!("^{}$", regex_pattern)).ok()
    }
}
