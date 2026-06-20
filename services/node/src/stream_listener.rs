//! L4 TCP/UDP stream forwarding listener.

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Result;
use tokio::net::{TcpListener, UdpSocket};
use tokio::sync::broadcast;
use tokio::task::JoinSet;
use tracing::{error, info, warn};

use crate::config::ConfigHolder;
use crate::stream_health::StreamHealthChecker;
use crate::stream_proxy;

pub struct StreamListener {
    config_holder: Arc<ConfigHolder>,
    health_checker: Arc<StreamHealthChecker>,
}

impl StreamListener {
    pub fn new(config_holder: Arc<ConfigHolder>, health_checker: Arc<StreamHealthChecker>) -> Self {
        Self {
            config_holder,
            health_checker,
        }
    }

    pub async fn start(self: Arc<Self>, mut shutdown: broadcast::Receiver<()>) -> Result<()> {
        let mut config_rx = self.config_holder.subscribe();
        let mut active_tcp: HashMap<u16, String> = HashMap::new();
        let mut active_udp: HashMap<u16, String> = HashMap::new();
        let mut accept_tasks = JoinSet::new();

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    info!("Stream listener shutting down");
                    accept_tasks.shutdown().await;
                    break;
                }
                changed = config_rx.changed() => {
                    if changed.is_err() {
                        break;
                    }
                    self.sync_ports(
                        &mut active_tcp,
                        &mut active_udp,
                        &mut accept_tasks,
                        &mut shutdown,
                    ).await;
                }
                _ = tokio::time::sleep(std::time::Duration::from_secs(10)) => {
                    self.sync_ports(
                        &mut active_tcp,
                        &mut active_udp,
                        &mut accept_tasks,
                        &mut shutdown,
                    ).await;
                }
            }
        }
        Ok(())
    }

    async fn sync_ports(
        self: &Arc<Self>,
        active_tcp: &mut HashMap<u16, String>,
        active_udp: &mut HashMap<u16, String>,
        accept_tasks: &mut JoinSet<()>,
        shutdown: &mut broadcast::Receiver<()>,
    ) {
        let cfg = match self.config_holder.get() {
            Some(c) => c,
            None => return,
        };

        let mut needed_tcp: HashMap<u16, crate::config::StreamForwardConfig> = HashMap::new();
        let mut needed_udp: HashMap<u16, crate::config::StreamForwardConfig> = HashMap::new();

        for sf in &cfg.stream_forwards {
            if !sf.enabled {
                continue;
            }
            if sf.health_check_enabled && !self.health_checker.is_healthy(&sf.id) {
                continue;
            }
            if let Some(port) = sf.effective_listen_port() {
                match sf.protocol.as_str() {
                    "udp" => {
                        needed_udp.insert(port, sf.clone());
                    }
                    _ => {
                        needed_tcp.insert(port, sf.clone());
                    }
                }
            }
        }

        for port in active_tcp
            .keys()
            .filter(|p| !needed_tcp.contains_key(p))
            .copied()
            .collect::<Vec<_>>()
        {
            active_tcp.remove(&port);
        }
        for port in active_udp
            .keys()
            .filter(|p| !needed_udp.contains_key(p))
            .copied()
            .collect::<Vec<_>>()
        {
            active_udp.remove(&port);
        }

        for (port, sf) in needed_tcp {
            if active_tcp.contains_key(&port) {
                continue;
            }
            let addr = format!("0.0.0.0:{}", port);
            match TcpListener::bind(&addr).await {
                Ok(listener) => {
                    info!("L4 TCP listener bound on {} (rule {})", addr, sf.id);
                    active_tcp.insert(port, sf.id.clone());
                    let sf = sf.clone();
                    let shutdown_rx = shutdown.resubscribe();
                    accept_tasks.spawn(async move {
                        if let Err(e) =
                            stream_proxy::run_tcp_accept_loop(listener, sf, shutdown_rx).await
                        {
                            warn!("L4 TCP accept loop on port {} ended: {}", port, e);
                        }
                    });
                }
                Err(e) => {
                    error!("Failed to bind L4 TCP port {}: {}", port, e);
                }
            }
        }

        for (port, sf) in needed_udp {
            if active_udp.contains_key(&port) {
                continue;
            }
            let addr = format!("0.0.0.0:{}", port);
            match UdpSocket::bind(&addr).await {
                Ok(socket) => {
                    info!("L4 UDP listener bound on {} (rule {})", addr, sf.id);
                    active_udp.insert(port, sf.id.clone());
                    let sf = sf.clone();
                    let shutdown_rx = shutdown.resubscribe();
                    accept_tasks.spawn(async move {
                        if let Err(e) =
                            stream_proxy::run_udp_relay_loop(socket, sf, shutdown_rx).await
                        {
                            warn!("L4 UDP relay on port {} ended: {}", port, e);
                        }
                    });
                }
                Err(e) => {
                    error!("Failed to bind L4 UDP port {}: {}", port, e);
                }
            }
        }
    }
}
