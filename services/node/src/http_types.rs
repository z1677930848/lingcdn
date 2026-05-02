use anyhow::Error;
use bytes::Bytes;
use http_body_util::combinators::BoxBody;

/// Unified body/error types used across the node (server + client).
///
/// We standardize on `anyhow::Error` so adapters like `map_err(|e| anyhow!(e))` can be used freely.
pub type NodeBody = BoxBody<Bytes, Error>;

/// Common response type for the node HTTP stack.
pub type NodeResponse = hyper::Response<NodeBody>;

