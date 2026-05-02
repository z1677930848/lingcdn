use anyhow::Result;
use std::sync::Arc;
use std::time::Duration;
use tokio::time::{sleep, interval};
use tokio::sync::broadcast;
use tracing::{debug, error, info};
use sysinfo::{Disks, System};

use crate::grpc_client::GrpcClient;
use crate::metrics::Metrics;
use crate::proto::node::{Metric, MetricsBatch};
use crate::xdp::XdpController;
use tokio::sync::RwLock;

pub struct TelemetryReporter {
    metrics: Arc<Metrics>,
    report_interval: Duration,
}

impl TelemetryReporter {
    pub fn new(metrics: Arc<Metrics>, report_interval_secs: u64) -> Self {
        Self {
            metrics,
            report_interval: Duration::from_secs(report_interval_secs),
        }
    }

    pub async fn start(
        self,
        mut grpc_client: GrpcClient,
        node_id: String,
        token: String,
        xdp_controller: Option<Arc<RwLock<XdpController>>>,
        mut shutdown: broadcast::Receiver<()>,
    ) {
        info!("Starting telemetry reporter with interval: {:?}", self.report_interval);

        let mut ticker = interval(self.report_interval);
        let mut consecutive_failures: u32 = 0;

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    info!("Telemetry reporter shutdown requested");
                    break;
                }
                _ = ticker.tick() => {}
            }

            if let Err(e) = self.report_metrics(&mut grpc_client, &node_id, &token, xdp_controller.as_ref()).await {
                error!("Failed to report metrics: {}", e);
                consecutive_failures = consecutive_failures.saturating_add(1);
                let backoff = Duration::from_secs(2u64.saturating_pow(consecutive_failures.min(5)));
                sleep(backoff).await;
            } else {
                consecutive_failures = 0;
            }
        }
    }

    async fn report_metrics(&self, grpc_client: &mut GrpcClient, node_id: &str, token: &str, xdp_controller: Option<&Arc<RwLock<XdpController>>>) -> Result<()> {
        let timestamp_ms = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)?
            .as_millis() as i64;

        let (cpu_usage_pct, mem_usage_pct, disk_usage_pct, cpu_count, mem_total_bytes, disk_total_bytes, nginx_running) = system_snapshot();
        let (tcp_established, tcp_syn_recv, tcp_time_wait) = tcp_state_counts();

        let mut metrics = vec![
            Metric {
                name: "requests_total".to_string(),
                value: self.metrics.requests_total.get() as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "cache_hits".to_string(),
                value: self.metrics.cache_hits.get() as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "cache_misses".to_string(),
                value: self.metrics.cache_misses.get() as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "cache_hit_rate".to_string(),
                value: self.metrics.cache_hit_rate(),
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "origin_requests".to_string(),
                value: self.metrics.origin_requests_total.get() as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "origin_errors".to_string(),
                value: self.metrics.origin_errors.get() as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "bytes_sent".to_string(),
                value: self.metrics.bytes_sent.get() as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "bytes_received".to_string(),
                value: self.metrics.bytes_received.get() as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "active_connections".to_string(),
                value: self.metrics.active_connections.get() as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "cpu_usage_pct".to_string(),
                value: cpu_usage_pct,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "mem_usage_pct".to_string(),
                value: mem_usage_pct,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "disk_usage_pct".to_string(),
                value: disk_usage_pct,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "cpu_count".to_string(),
                value: cpu_count as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "mem_total_bytes".to_string(),
                value: mem_total_bytes as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "disk_total_bytes".to_string(),
                value: disk_total_bytes as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "tcp_established".to_string(),
                value: tcp_established as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "tcp_syn_recv".to_string(),
                value: tcp_syn_recv as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "tcp_time_wait".to_string(),
                value: tcp_time_wait as f64,
                labels: Default::default(),
                timestamp_ms,
            },
            Metric {
                name: "nginx_running".to_string(),
                value: if nginx_running { 1.0 } else { 0.0 },
                labels: Default::default(),
                timestamp_ms,
            },
        ];

        if let Some(xdp) = xdp_controller {
            let guard = xdp.read().await;
            let enabled = guard.is_enabled();
            let iface = guard.interface().to_string();
            let mut labels = std::collections::HashMap::new();
            if !iface.is_empty() {
                labels.insert("iface".to_string(), iface);
            }

            metrics.push(Metric {
                name: "xdp_enabled".to_string(),
                value: if enabled { 1.0 } else { 0.0 },
                labels: labels.clone(),
                timestamp_ms,
            });

            if enabled {
                if let Ok(st) = guard.get_stats() {
                    let push = |name: &str, value: u64| Metric {
                        name: format!("xdp_{}", name),
                        value: value as f64,
                        labels: labels.clone(),
                        timestamp_ms,
                    };

                    metrics.push(push("packets_total", st.packets_total));
                    metrics.push(push("packets_passed", st.packets_passed));
                    metrics.push(push("packets_dropped_blacklist", st.packets_dropped_blacklist));
                    metrics.push(push("packets_whitelisted", st.packets_whitelisted));
                    metrics.push(push("packets_non_ip", st.packets_non_ip));
                    metrics.push(push("packets_dropped_rate_limit", st.packets_dropped_rate_limit));
                    metrics.push(push("packets_dropped_syn_flood", st.packets_dropped_syn_flood));
                    metrics.push(push("packets_dropped_invalid", st.packets_dropped_invalid));
                    metrics.push(push("syn_packets", st.syn_packets));
                    metrics.push(push("udp_packets", st.udp_packets));
                    metrics.push(push("tcp_packets", st.tcp_packets));
                    metrics.push(push("icmp_packets", st.icmp_packets));
                }
            }
        } else {
            metrics.push(Metric {
                name: "xdp_enabled".to_string(),
                value: 0.0,
                labels: Default::default(),
                timestamp_ms,
            });
        }

        let batch = MetricsBatch { metrics };

        debug!("Reporting {} metrics", batch.metrics.len());
        grpc_client.report_metrics(node_id, token, batch).await?;

        Ok(())
    }
}

fn system_snapshot() -> (f64, f64, f64, usize, u64, u64, bool) {
    let mut sys = System::new_all();
    sys.refresh_cpu();
    sys.refresh_memory();
    sys.refresh_processes();
    let mut disks = Disks::new_with_refreshed_list();
    disks.refresh();

    let cpu_usage_pct = sys.global_cpu_info().cpu_usage() as f64;

    let total_mem_bytes = sys.total_memory();
    let used_mem_bytes = sys.used_memory();
    let mem_usage_pct = if total_mem_bytes > 0 {
        (used_mem_bytes as f64) * 100.0 / (total_mem_bytes as f64)
    } else {
        0.0
    };

    let mut disk_total_bytes = 0u64;
    let mut disk_used_pct_max = 0f64;
    for d in disks.list() {
        let total = d.total_space();
        let avail = d.available_space();
        disk_total_bytes = disk_total_bytes.saturating_add(total);
        if total > 0 {
            let used = total.saturating_sub(avail);
            let pct = (used as f64) * 100.0 / (total as f64);
            if pct > disk_used_pct_max {
                disk_used_pct_max = pct;
            }
        }
    }

    let nginx_running = sys.processes().values().any(|p| p.name() == "nginx");

    (
        cpu_usage_pct,
        mem_usage_pct,
        disk_used_pct_max,
        sys.cpus().len(),
        total_mem_bytes,
        disk_total_bytes,
        nginx_running,
    )
}

#[cfg(target_os = "linux")]
fn tcp_state_counts() -> (u32, u32, u32) {
    fn scan(path: &str, est: &mut u32, syn: &mut u32, tw: &mut u32) {
        let Ok(s) = std::fs::read_to_string(path) else { return };
        for (i, line) in s.lines().enumerate() {
            if i == 0 {
                continue;
            }
            let cols: Vec<&str> = line.split_whitespace().collect();
            if cols.len() < 4 {
                continue;
            }
            match cols[3] {
                "01" => *est = est.saturating_add(1),
                "03" => *syn = syn.saturating_add(1),
                "06" => *tw = tw.saturating_add(1),
                _ => {}
            }
        }
    }

    let mut est = 0u32;
    let mut syn = 0u32;
    let mut tw = 0u32;
    scan("/proc/net/tcp", &mut est, &mut syn, &mut tw);
    scan("/proc/net/tcp6", &mut est, &mut syn, &mut tw);
    (est, syn, tw)
}

#[cfg(not(target_os = "linux"))]
fn tcp_state_counts() -> (u32, u32, u32) {
    (0, 0, 0)
}
