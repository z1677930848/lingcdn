use prometheus::{
    Counter, Histogram, HistogramOpts, IntCounter, IntGauge, Opts, Registry,
};
use std::collections::HashMap;
use std::sync::Arc;
use anyhow::Result;

#[derive(Clone)]
pub struct Metrics {
    registry: Arc<Registry>,

    // Request metrics
    pub requests_total: IntCounter,
    pub requests_by_status: Counter,

    // Cache metrics
    pub cache_hits: IntCounter,
    pub cache_misses: IntCounter,
    pub cache_size_bytes: IntGauge,

    // Origin metrics
    pub origin_requests_total: IntCounter,
    pub origin_request_duration: Histogram,
    #[allow(dead_code)]
    pub origin_errors: IntCounter,

    // Bandwidth metrics
    pub bytes_sent: IntCounter,
    pub bytes_received: IntCounter,

    // Connection metrics
    pub active_connections: IntGauge,

    pub access_log_dropped_total: IntCounter,
    pub error_log_dropped_total: IntCounter,
}

impl Metrics {
    pub fn new() -> Result<Self> {
        let registry = Registry::new();

        let requests_total = IntCounter::with_opts(
            Opts::new("lingcdn_requests_total", "Total number of requests")
        )?;
        registry.register(Box::new(requests_total.clone()))?;

        let requests_by_status = Counter::with_opts(
            Opts::new("lingcdn_requests_by_status", "Requests by status code")
        )?;
        registry.register(Box::new(requests_by_status.clone()))?;

        let cache_hits = IntCounter::with_opts(
            Opts::new("lingcdn_cache_hits_total", "Total cache hits")
        )?;
        registry.register(Box::new(cache_hits.clone()))?;

        let cache_misses = IntCounter::with_opts(
            Opts::new("lingcdn_cache_misses_total", "Total cache misses")
        )?;
        registry.register(Box::new(cache_misses.clone()))?;

        let cache_size_bytes = IntGauge::with_opts(
            Opts::new("lingcdn_cache_size_bytes", "Cache size in bytes")
        )?;
        registry.register(Box::new(cache_size_bytes.clone()))?;

        let origin_requests_total = IntCounter::with_opts(
            Opts::new("lingcdn_origin_requests_total", "Total origin requests")
        )?;
        registry.register(Box::new(origin_requests_total.clone()))?;

        let origin_request_duration = Histogram::with_opts(
            HistogramOpts::new("lingcdn_origin_request_duration_seconds", "Origin request duration")
        )?;
        registry.register(Box::new(origin_request_duration.clone()))?;

        let origin_errors = IntCounter::with_opts(
            Opts::new("lingcdn_origin_errors_total", "Total origin errors")
        )?;
        registry.register(Box::new(origin_errors.clone()))?;

        let bytes_sent = IntCounter::with_opts(
            Opts::new("lingcdn_bytes_sent_total", "Total bytes sent")
        )?;
        registry.register(Box::new(bytes_sent.clone()))?;

        let bytes_received = IntCounter::with_opts(
            Opts::new("lingcdn_bytes_received_total", "Total bytes received")
        )?;
        registry.register(Box::new(bytes_received.clone()))?;

        let active_connections = IntGauge::with_opts(
            Opts::new("lingcdn_active_connections", "Number of active connections")
        )?;
        registry.register(Box::new(active_connections.clone()))?;

        let access_log_dropped_total = IntCounter::with_opts(
            Opts::new("lingcdn_access_log_dropped_total", "Dropped access log entries")
        )?;
        registry.register(Box::new(access_log_dropped_total.clone()))?;

        let error_log_dropped_total = IntCounter::with_opts(
            Opts::new("lingcdn_error_log_dropped_total", "Dropped error log entries (channel full or buffer overflow)")
        )?;
        registry.register(Box::new(error_log_dropped_total.clone()))?;

        Ok(Self {
            registry: Arc::new(registry),
            requests_total,
            requests_by_status,
            cache_hits,
            cache_misses,
            cache_size_bytes,
            origin_requests_total,
            origin_request_duration,
            origin_errors,
            bytes_sent,
            bytes_received,
            active_connections,
            access_log_dropped_total,
            error_log_dropped_total,
        })
    }

    pub fn registry(&self) -> Arc<Registry> {
        self.registry.clone()
    }

    pub fn cache_hit_rate(&self) -> f64 {
        let hits = self.cache_hits.get() as f64;
        let misses = self.cache_misses.get() as f64;
        let total = hits + misses;
        if total == 0.0 {
            0.0
        } else {
            hits / total
        }
    }

    /// Collect a lightweight snapshot for heartbeat payloads.
    pub fn snapshot(&self) -> HashMap<String, String> {
        let mut m = HashMap::new();
        m.insert("requests_total".to_string(), self.requests_total.get().to_string());
        m.insert("requests_by_status".to_string(), self.requests_by_status.get().to_string());
        m.insert("cache_hits".to_string(), self.cache_hits.get().to_string());
        m.insert("cache_misses".to_string(), self.cache_misses.get().to_string());
        m.insert("cache_hit_rate".to_string(), format!("{:.4}", self.cache_hit_rate()));
        m.insert("cache_size_bytes".to_string(), self.cache_size_bytes.get().to_string());
        m.insert("origin_requests_total".to_string(), self.origin_requests_total.get().to_string());
        m.insert("origin_errors".to_string(), self.origin_errors.get().to_string());
        m.insert("bytes_sent".to_string(), self.bytes_sent.get().to_string());
        m.insert("bytes_received".to_string(), self.bytes_received.get().to_string());
        m.insert("active_connections".to_string(), self.active_connections.get().to_string());
        m.insert("access_log_dropped_total".to_string(), self.access_log_dropped_total.get().to_string());
        m.insert("error_log_dropped_total".to_string(), self.error_log_dropped_total.get().to_string());
        m
    }
}

impl Default for Metrics {
    fn default() -> Self {
        match Self::new() {
            Ok(metrics) => metrics,
            Err(err) => panic!("failed to create metrics: {err}"),
        }
    }
}
