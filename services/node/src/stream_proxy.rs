//! L4 TCP/UDP proxy — bidirectional relay between client and origin.

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::Result;
use parking_lot::Mutex;
use tokio::io::copy_bidirectional;
use tokio::net::{TcpListener, TcpStream, UdpSocket};
use tokio::sync::broadcast;
use tracing::debug;

use crate::config::StreamForwardConfig;

pub async fn run_tcp_accept_loop(
    listener: TcpListener,
    rule: StreamForwardConfig,
    mut shutdown: broadcast::Receiver<()>,
) -> Result<()> {
    loop {
        tokio::select! {
            _ = shutdown.recv() => return Ok(()),
            accept = listener.accept() => {
                match accept {
                    Ok((client, addr)) => {
                        let rule = rule.clone();
                        tokio::spawn(async move {
                            if let Err(e) = relay_tcp(client, &rule).await {
                                debug!("L4 TCP relay from {} failed: {}", addr, e);
                            }
                        });
                    }
                    Err(e) => return Err(e.into()),
                }
            }
        }
    }
}

pub async fn relay_tcp(mut client: TcpStream, rule: &StreamForwardConfig) -> Result<()> {
    let origin_addr = format!("{}:{}", rule.origin_host, rule.origin_port);
    let mut origin = TcpStream::connect(&origin_addr).await?;
    copy_bidirectional(&mut client, &mut origin).await?;
    Ok(())
}

struct UdpSession {
    origin: Arc<UdpSocket>,
    last_seen: Instant,
}

pub async fn run_udp_relay_loop(
    listen_socket: UdpSocket,
    rule: StreamForwardConfig,
    mut shutdown: broadcast::Receiver<()>,
) -> Result<()> {
    let listen = Arc::new(listen_socket);
    let sessions: Arc<Mutex<HashMap<SocketAddr, UdpSession>>> =
        Arc::new(Mutex::new(HashMap::new()));
    let origin_base = format!("{}:{}", rule.origin_host, rule.origin_port);

    let cleanup_sessions = sessions.clone();
    let _cleanup = tokio::spawn(async move {
        loop {
            tokio::time::sleep(Duration::from_secs(30)).await;
            let cutoff = Instant::now() - Duration::from_secs(120);
            cleanup_sessions
                .lock()
                .retain(|_, session| session.last_seen >= cutoff);
        }
    });

    let mut buf = vec![0u8; 65536];
    loop {
        tokio::select! {
            _ = shutdown.recv() => {
                _cleanup.abort();
                return Ok(());
            }
            recv = listen.recv_from(&mut buf) => {
                match recv {
                    Ok((n, peer)) => {
                        if n == 0 {
                            continue;
                        }
                        let data = buf[..n].to_vec();
                        let listen = listen.clone();
                        let sessions = sessions.clone();
                        let origin_base = origin_base.clone();
                        tokio::spawn(async move {
                            if let Err(e) = relay_udp_packet(
                                listen,
                                sessions,
                                &origin_base,
                                peer,
                                data,
                            ).await {
                                debug!("L4 UDP relay from {} failed: {}", peer, e);
                            }
                        });
                    }
                    Err(e) => return Err(e.into()),
                }
            }
        }
    }
}

async fn relay_udp_packet(
    listen: Arc<UdpSocket>,
    sessions: Arc<Mutex<HashMap<SocketAddr, UdpSession>>>,
    origin_base: &str,
    peer: SocketAddr,
    data: Vec<u8>,
) -> Result<()> {
    let origin = {
        let existing = sessions.lock().get(&peer).map(|s| s.origin.clone());
        if let Some(origin) = existing {
            if let Some(session) = sessions.lock().get_mut(&peer) {
                session.last_seen = Instant::now();
            }
            origin
        } else {
            let socket = UdpSocket::bind("0.0.0.0:0").await?;
            socket.connect(origin_base).await?;
            let origin = Arc::new(socket);
            sessions.lock().insert(
                peer,
                UdpSession {
                    origin: origin.clone(),
                    last_seen: Instant::now(),
                },
            );
            origin
        }
    };

    origin.send(&data).await?;

    let listen_for_reply = listen.clone();
    let sessions_for_reply = sessions.clone();
    tokio::spawn(async move {
        let mut reply = [0u8; 65536];
        match origin.recv(&mut reply).await {
            Ok(n) if n > 0 => {
                let _ = listen_for_reply.send_to(&reply[..n], peer).await;
                if let Some(session) = sessions_for_reply.lock().get_mut(&peer) {
                    session.last_seen = Instant::now();
                }
            }
            _ => {}
        }
    });

    Ok(())
}
