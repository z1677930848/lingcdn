//! Image transform path rewriting and video segment cache helpers.

use std::collections::HashMap;
use url::form_urlencoded;

#[derive(Debug, Clone, Default)]
pub struct ImageTransform {
    pub width: Option<u32>,
    pub height: Option<u32>,
    pub quality: Option<u32>,
    pub format: Option<String>,
}

impl ImageTransform {
    pub fn cache_suffix(&self) -> String {
        let mut parts = Vec::new();
        if let Some(w) = self.width {
            parts.push(format!("w{w}"));
        }
        if let Some(h) = self.height {
            parts.push(format!("h{h}"));
        }
        if let Some(q) = self.quality {
            parts.push(format!("q{q}"));
        }
        if let Some(ref f) = self.format {
            if !f.is_empty() {
                parts.push(format!("f{f}"));
            }
        }
        parts.join("_")
    }
}

/// Rewrite `/__img/...?w=&h=` into origin path plus transform metadata.
pub fn normalize_image_request(path: &str, query: Option<&str>) -> (String, Option<ImageTransform>) {
    let mut effective_path = path.to_string();
    if path.starts_with("/__img/") {
        effective_path = path.replacen("/__img", "", 1);
        if effective_path.is_empty() {
            effective_path = "/".to_string();
        }
    }
    let transform = query.and_then(parse_transform_query);
    if transform.is_some() || path.starts_with("/__img/") {
        (effective_path, transform)
    } else {
        (path.to_string(), None)
    }
}

fn parse_transform_query(query: &str) -> Option<ImageTransform> {
    let params: HashMap<_, _> = form_urlencoded::parse(query.as_bytes())
        .into_owned()
        .collect();
    let width = params.get("w").and_then(|v| v.parse().ok());
    let height = params.get("h").and_then(|v| v.parse().ok());
    let quality = params.get("q").and_then(|v| v.parse().ok());
    let format = params.get("format").cloned().or_else(|| params.get("f").cloned());
    if width.is_none() && height.is_none() && quality.is_none() && format.is_none() {
        return None;
    }
    Some(ImageTransform {
        width,
        height,
        quality,
        format,
    })
}

pub fn is_video_segment(path: &str) -> bool {
    let lower = path.to_ascii_lowercase();
    lower.ends_with(".m3u8")
        || lower.ends_with(".ts")
        || lower.ends_with(".m4s")
        || lower.ends_with(".mp4")
        || lower.ends_with(".mov")
}

pub fn video_segment_cache_ttl(default_ttl: u64) -> u64 {
    default_ttl.saturating_mul(4).max(default_ttl)
}
