//! Optional edge enhancements: bot scoring, response compression, smart routing hooks.

use http::header::{ACCEPT_ENCODING, CONTENT_ENCODING, USER_AGENT};
use http::{HeaderMap, HeaderValue, Response, StatusCode};
use http_body_util::Full;
use bytes::Bytes;

/// Score a request for bot-like behavior (0=human, 100=bot).
pub fn bot_score(headers: &HeaderMap, path: &str, req_rate: u32) -> u32 {
    let mut score = 0u32;
    if let Some(ua) = headers.get(USER_AGENT).and_then(|v| v.to_str().ok()) {
        let ua_lower = ua.to_lowercase();
        if ua_lower.is_empty() {
            score += 40;
        }
        for kw in ["bot", "spider", "crawl", "curl", "wget", "python", "scrapy"] {
            if ua_lower.contains(kw) {
                score += 30;
                break;
            }
        }
    } else {
        score += 25;
    }
    if path.contains("wp-admin") || path.contains(".env") || path.contains("phpmyadmin") {
        score += 20;
    }
    score += req_rate.min(30);
    score.min(100)
}

/// Apply gzip/br compression to small text responses when client accepts it.
pub fn maybe_compress_response(
    headers: &HeaderMap,
    status: StatusCode,
    body: Bytes,
    content_type: Option<&str>,
) -> Response<Full<Bytes>> {
    let ct = content_type.unwrap_or("");
    let compressible = status.is_success()
        && (ct.starts_with("text/") || ct.contains("json") || ct.contains("javascript") || ct.contains("xml"));
    if !compressible || body.len() < 256 {
        return Response::builder()
            .status(status)
            .body(Full::new(body))
            .unwrap_or_else(|_| Response::new(Full::new(Bytes::new())));
    }

    let accept = headers
        .get(ACCEPT_ENCODING)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .to_lowercase();

    if accept.contains("gzip") {
        if let Ok(compressed) = compress_gzip(&body) {
            return Response::builder()
                .status(status)
                .header(CONTENT_ENCODING, HeaderValue::from_static("gzip"))
                .body(Full::new(compressed))
                .unwrap_or_else(|_| Response::new(Full::new(body)));
        }
    }
    Response::builder()
        .status(status)
        .body(Full::new(body))
        .unwrap_or_else(|_| Response::new(Full::new(Bytes::new())))
}

/// Try gzip compression when the client accepts it.
pub fn try_gzip(body: &Bytes, accept_encoding: &str) -> Option<Bytes> {
    if !accept_encoding.to_ascii_lowercase().contains("gzip") || body.len() < 256 {
        return None;
    }
    compress_gzip(body).ok()
}

fn compress_gzip(data: &[u8]) -> Result<Bytes, std::io::Error> {
    use std::io::Write;
    let mut enc = flate2::write::GzEncoder::new(Vec::new(), flate2::Compression::default());
    enc.write_all(data)?;
    Ok(Bytes::from(enc.finish()?))
}

/// Pick origin index using client IP hash for sticky routing, or weighted random.
pub fn select_origin_index(client_ip: Option<&str>, method: &str, weights: &[i32], len: usize) -> usize {
    if len == 0 {
        return 0;
    }
    if method == "ip_hash" {
        if let Some(ip) = client_ip {
            let h = ip.bytes().fold(0u64, |a, b| a.wrapping_add(b as u64));
            return (h as usize) % len;
        }
    }
    // weighted random fallback
    let total: i32 = weights.iter().copied().filter(|w| *w > 0).sum();
    if total <= 0 {
        return 0;
    }
    let pick = (rand::random::<u32>() % total as u32) as i32;
    let mut acc = 0i32;
    for (i, w) in weights.iter().enumerate().take(len) {
        if *w <= 0 {
            continue;
        }
        acc += *w;
        if pick < acc {
            return i;
        }
    }
    0
}
