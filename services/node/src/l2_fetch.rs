//! L2 sibling cache fetch: on local miss, try peer nodes before origin.

use bytes::Bytes;
use http::header::{HOST, USER_AGENT};
use http::Request;
use http_body_util::{BodyExt, Full};
use hyper_util::client::legacy::{connect::HttpConnector, Client};
use std::time::Duration;
use tracing::debug;

use crate::config::L2PeerConfig;
use crate::http_types::NodeBody;

pub struct L2FetchResult {
    pub status: u16,
    pub headers: Vec<(Box<str>, Box<str>)>,
    pub body: Bytes,
}

/// Try each configured peer (excluding self) until one returns a cacheable 2xx GET response.
pub async fn try_fetch_from_peers(
    client: &Client<hyper_rustls::HttpsConnector<HttpConnector>, NodeBody>,
    peers: &[L2PeerConfig],
    self_node_id: Option<&str>,
    host: &str,
    path: &str,
    query: Option<&str>,
    timeout: Duration,
) -> Option<L2FetchResult> {
    if peers.is_empty() || host.trim().is_empty() {
        return None;
    }
    let path_q = match query {
        Some(q) if !q.is_empty() => format!("{path}?{q}"),
        _ => path.to_string(),
    };

    for peer in peers {
        if peer.address.trim().is_empty() {
            continue;
        }
        if self_node_id
            .map(|id| id == peer.node_id.as_str())
            .unwrap_or(false)
        {
            continue;
        }
        let base = peer.address.trim_end_matches('/');
        let uri = format!("{base}{}", ensure_leading_slash(&path_q));
        let req = match Request::builder()
            .method(http::Method::GET)
            .uri(&uri)
            .header(HOST, host)
            .header(USER_AGENT, "LingCDN-L2/1.0")
            .body(Full::new(Bytes::new()).map_err(|e| match e {}).boxed())
        {
            Ok(r) => r,
            Err(_) => continue,
        };

        let resp = match tokio::time::timeout(timeout, client.request(req)).await {
            Ok(Ok(r)) => r,
            Ok(Err(e)) => {
                debug!("L2 peer {} fetch error: {}", peer.node_id, e);
                continue;
            }
            Err(_) => {
                debug!("L2 peer {} fetch timeout", peer.node_id);
                continue;
            }
        };

        let status = resp.status();
        if !status.is_success() {
            continue;
        }
        let cache_control = resp
            .headers()
            .get(http::header::CACHE_CONTROL)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("");
        if cache_control.contains("no-store") || cache_control.contains("private") {
            continue;
        }

        let (parts, body) = resp.into_parts();
        let body_bytes = match body.collect().await {
            Ok(collected) => collected.to_bytes(),
            Err(_) => continue,
        };
        if body_bytes.is_empty() {
            continue;
        }

        let headers: Vec<(Box<str>, Box<str>)> = parts
            .headers
            .iter()
            .map(|(k, v)| (k.as_str().into(), v.to_str().unwrap_or("").into()))
            .collect();

        debug!(
            "L2 hit from peer {} status={} bytes={}",
            peer.node_id,
            status.as_u16(),
            body_bytes.len()
        );
        return Some(L2FetchResult {
            status: status.as_u16(),
            headers,
            body: body_bytes,
        });
    }
    None
}

fn ensure_leading_slash(path: &str) -> String {
    if path.starts_with('/') {
        path.to_string()
    } else {
        format!("/{path}")
    }
}
