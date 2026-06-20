//! L4 stream origin health probing.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::time::Duration;

use parking_lot::RwLock;
use tokio::net::{TcpStream, UdpSocket};
use tokio::sync::broadcast;
use tokio::time::timeout;
use tracing::debug;

use crate::config::{ConfigHolder, StreamForwardConfig};

#[derive(Debug, Clone)]
struct HealthState {
    healthy: bool,
    consecutive_fail: u32,
}

impl Default for HealthState {
    fn default() -> Self {
        Self {
            healthy: true,
            consecutive_fail: 0,
        }
    }
}

pub struct StreamHealthChecker {
    states: Arc<RwLock<HashMap<String, HealthState>>>,
    config_holder: Arc<ConfigHolder>,
}

impl StreamHealthChecker {
    pub fn new(config_holder: Arc<ConfigHolder>) -> Self {
        Self {
            states: Arc::new(RwLock::new(HashMap::new())),
            config_holder,
        }
    }

    pub fn is_healthy(&self, rule_id: &str) -> bool {
        let states = self.states.read();
        states.get(rule_id).map(|s| s.healthy).unwrap_or(true)
    }

    pub async fn run(self: Arc<Self>, mut shutdown: broadcast::Receiver<()>) {
        loop {
            tokio::select! {
                _ = shutdown.recv() => break,
                _ = tokio::time::sleep(Duration::from_secs(10)) => {
                    self.probe_all().await;
                }
            }
        }
    }

    async fn probe_all(&self) {
        let rules = match self.config_holder.get() {
            Some(cfg) => cfg.stream_forwards.clone(),
            None => return,
        };

        let active: HashSet<String> = rules
            .iter()
            .filter(|sf| sf.enabled && sf.health_check_enabled)
            .map(|sf| sf.id.clone())
            .collect();

        {
            let mut states = self.states.write();
            states.retain(|id, _| active.contains(id));
        }

        for sf in rules {
            if !sf.enabled || !sf.health_check_enabled {
                continue;
            }
            let ok = probe_rule(&sf).await;
            let mut states = self.states.write();
            let entry = states.entry(sf.id.clone()).or_default();
            if ok {
                entry.consecutive_fail = 0;
                entry.healthy = true;
            } else {
                entry.consecutive_fail = entry.consecutive_fail.saturating_add(1);
                if entry.consecutive_fail >= 2 {
                    entry.healthy = false;
                }
                debug!(
                    "L4 health probe failed for rule {} ({}/{})",
                    sf.id, sf.origin_host, sf.origin_port
                );
            }
        }
    }
}

async fn probe_rule(rule: &StreamForwardConfig) -> bool {
    let origin = format!("{}:{}", rule.origin_host, rule.origin_port);
    if rule.protocol == "udp" {
        return probe_udp(&origin).await;
    }
    probe_tcp(&origin).await
}

async fn probe_tcp(origin: &str) -> bool {
    match timeout(Duration::from_secs(3), TcpStream::connect(origin)).await {
        Ok(Ok(_)) => true,
        _ => false,
    }
}

async fn probe_udp(origin: &str) -> bool {
    let socket = match UdpSocket::bind("0.0.0.0:0").await {
        Ok(s) => s,
        Err(_) => return false,
    };
    match timeout(Duration::from_secs(3), socket.connect(origin)).await {
        Ok(Ok(())) => true,
        _ => false,
    }
}
