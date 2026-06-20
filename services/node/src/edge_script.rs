//! Lightweight edge script rules (JSON array) for request/response mutation.

use http::header::{HeaderName, HeaderValue};
use http::{HeaderMap, StatusCode};
use serde::Deserialize;

#[derive(Debug, Deserialize)]
struct EdgeRule {
    op: String,
    #[serde(default)]
    name: String,
    #[serde(default)]
    value: String,
    #[serde(default = "default_block_status")]
    status: u16,
    #[serde(default)]
    location: String,
}

fn default_block_status() -> u16 {
    403
}

pub struct EdgeEarlyResponse {
    pub status: StatusCode,
    pub location: Option<String>,
    pub tag: &'static str,
}

/// Apply request-phase rules. Returns early status when block/redirect triggers.
pub fn apply_request_rules(
    rules_json: &str,
    headers: &mut HeaderMap,
) -> Option<EdgeEarlyResponse> {
    let rules: Vec<EdgeRule> = match serde_json::from_str::<Vec<EdgeRule>>(rules_json) {
        Ok(v) if !v.is_empty() => v,
        _ => return None,
    };
    for rule in rules {
        match rule.op.as_str() {
            "set_header" | "set_request_header" => {
                if rule.name.is_empty() {
                    continue;
                }
                if let (Ok(name), Ok(value)) = (
                    HeaderName::from_bytes(rule.name.as_bytes()),
                    HeaderValue::from_str(&rule.value),
                ) {
                    headers.insert(name, value);
                }
            }
            "block" => {
                let status = StatusCode::from_u16(rule.status).unwrap_or(StatusCode::FORBIDDEN);
                return Some(EdgeEarlyResponse {
                    status,
                    location: None,
                    tag: "edge_script_block",
                });
            }
            "redirect" if !rule.location.is_empty() => {
                let status = StatusCode::from_u16(rule.status).unwrap_or(StatusCode::FOUND);
                return Some(EdgeEarlyResponse {
                    status,
                    location: Some(rule.location.clone()),
                    tag: "edge_script_redirect",
                });
            }
            _ => {}
        }
    }
    None
}

/// Apply response-phase rules (header injection only).
pub fn apply_response_rules(rules_json: &str, headers: &mut HeaderMap) {
    let rules: Vec<EdgeRule> = match serde_json::from_str::<Vec<EdgeRule>>(rules_json) {
        Ok(v) if !v.is_empty() => v,
        _ => return,
    };
    for rule in rules {
        if rule.op != "set_response_header" || rule.name.is_empty() {
            continue;
        }
        if let (Ok(name), Ok(value)) = (
            HeaderName::from_bytes(rule.name.as_bytes()),
            HeaderValue::from_str(&rule.value),
        ) {
            headers.insert(name, value);
        }
    }
}
