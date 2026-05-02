// Local error-log appender. Captures WARN/ERROR tracing events and
// serialises each one as a JSON line into a local file (default
// /var/log/lingcdn/error.log). A node-side Filebeat tails the file and
// ships entries to Elasticsearch under the cdn-error-YYYY.MM.DD index.
//
// Shape of the on-disk JSON (one object per line):
//   {
//     "@timestamp": "2026-01-02T03:04:05.123Z",
//     "level": "warn" | "error",
//     "message": "...",
//     "target": "lingcdn_node::proxy",
//     "source": "src/proxy.rs:1234",
//     "node_id": "...",   (when known)
//     "node":    "...",   (hostname; when known)
//     ...other tracing fields flattened as string labels
//   }
//
// Lifecycle: `new()` returns the Layer + a Handle. Layer is plugged
// into tracing_subscriber early; the Handle is consumed by
// `spawn_writer()` once main has loaded NodeConfig and built Metrics,
// at which point the writer task starts draining the channel into the
// configured file path. WARN/ERROR events emitted between tracing init
// and spawn_writer sit in the bounded mpsc buffer; if we exceed
// capacity the over-flow is counted as dropped (atomic counter, later
// reflected into prometheus once the writer task starts).

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::sync::OnceLock;

use chrono::{SecondsFormat, Utc};
use parking_lot::RwLock;
use prometheus::IntCounter;
use serde::Serialize;
use tokio::fs::OpenOptions;
use tokio::io::AsyncWriteExt;
use tokio::sync::{broadcast, mpsc};
use tokio::time::Duration;
use tracing::field::{Field, Visit};
use tracing_subscriber::layer::Context;
use tracing_subscriber::Layer;

const DEFAULT_CHANNEL_SIZE: usize = 4096;
const DEFAULT_FLUSH_MS: u64 = 500;
const DEFAULT_BATCH_BYTES: usize = 256 * 1024;
const DEFAULT_MAX_BUFFER_BYTES: usize = 4 * 1024 * 1024;

/// Best-effort node identity propagated in every emitted document.
/// Populated by `set_hostname` (right after node config load) and
/// `set_node_id` (after gRPC register completes). Never required —
/// missing fields are simply omitted from the JSON output.
#[derive(Clone, Default)]
struct NodeIdentity {
    node_id: Option<String>,
    hostname: Option<String>,
}

static NODE_IDENTITY: OnceLock<RwLock<NodeIdentity>> = OnceLock::new();

fn identity_cell() -> &'static RwLock<NodeIdentity> {
    NODE_IDENTITY.get_or_init(|| RwLock::new(NodeIdentity::default()))
}

pub fn set_hostname(hostname: String) {
    let mut g = identity_cell().write();
    g.hostname = Some(hostname);
}

pub fn set_node_id(node_id: String) {
    let mut g = identity_cell().write();
    g.node_id = Some(node_id);
}

fn snapshot_identity() -> NodeIdentity {
    identity_cell().read().clone()
}

/// tracing Layer that captures WARN / ERROR events and forwards them
/// (as JSON lines) over an mpsc channel to the writer task.
pub struct LogReporterLayer {
    tx: mpsc::Sender<String>,
    dropped: Arc<AtomicU64>,
}

/// Receiver side of the channel, plus the dropped-counter shared with
/// the Layer. Consumed by `spawn_writer`.
pub struct LogReporterHandle {
    rx: mpsc::Receiver<String>,
    dropped: Arc<AtomicU64>,
}

impl LogReporterLayer {
    /// Construct a Layer + Handle pair. The channel size is bounded —
    /// when the writer task lags or has not started yet, surplus events
    /// are counted via the atomic dropped counter rather than blocking
    /// the hot tracing path.
    pub fn new() -> (Self, LogReporterHandle) {
        let cap = std::env::var("ERROR_LOG_CHANNEL_SIZE")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or(DEFAULT_CHANNEL_SIZE);
        let (tx, rx) = mpsc::channel(cap.max(64));
        let dropped = Arc::new(AtomicU64::new(0));
        let layer = Self {
            tx,
            dropped: dropped.clone(),
        };
        let handle = LogReporterHandle { rx, dropped };
        (layer, handle)
    }
}

#[derive(Serialize)]
struct LogEntryDoc<'a> {
    #[serde(rename = "@timestamp")]
    ts: &'a str,
    level: &'a str,
    message: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    target: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    source: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    node_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    node: Option<String>,
    #[serde(flatten)]
    labels: &'a HashMap<String, String>,
}

struct FieldVisitor {
    message: String,
    labels: HashMap<String, String>,
}

impl FieldVisitor {
    fn new() -> Self {
        Self {
            message: String::new(),
            labels: HashMap::new(),
        }
    }
}

impl Visit for FieldVisitor {
    fn record_debug(&mut self, field: &Field, value: &dyn std::fmt::Debug) {
        let val = format!("{:?}", value);
        if field.name() == "message" {
            self.message = val;
        } else {
            self.labels.insert(field.name().to_string(), val);
        }
    }

    fn record_str(&mut self, field: &Field, value: &str) {
        if field.name() == "message" {
            self.message = value.to_string();
        } else {
            self.labels.insert(field.name().to_string(), value.to_string());
        }
    }
}

impl<S> Layer<S> for LogReporterLayer
where
    S: tracing::Subscriber,
{
    fn on_event(&self, event: &tracing::Event<'_>, _ctx: Context<'_, S>) {
        let meta = event.metadata();
        let level = meta.level();

        let level_str = match *level {
            tracing::Level::ERROR => "error",
            tracing::Level::WARN => "warn",
            _ => return,
        };

        let mut visitor = FieldVisitor::new();
        event.record(&mut visitor);

        if visitor.message.is_empty() {
            return;
        }

        let source = meta.file().map(|f| match meta.line() {
            Some(line) => format!("{}:{}", f, line),
            None => f.to_string(),
        });
        let target = meta.target();

        let ts = Utc::now().to_rfc3339_opts(SecondsFormat::Millis, true);
        let ident = snapshot_identity();

        let doc = LogEntryDoc {
            ts: &ts,
            level: level_str,
            message: &visitor.message,
            target: Some(target),
            source,
            node_id: ident.node_id,
            node: ident.hostname,
            labels: &visitor.labels,
        };

        let Ok(line) = serde_json::to_string(&doc) else { return };

        // try_send — never block the emitter; dropped events are
        // counted and later surfaced via prometheus.
        if self.tx.try_send(line).is_err() {
            self.dropped.fetch_add(1, Ordering::Relaxed);
        }
    }
}

/// Spawn the writer task. Drains the Layer's channel, batches lines,
/// and appends to the on-disk file with periodic flushes. Mirrors the
/// design of the access-log writer (mpsc + interval ticker + size
/// threshold + best-effort final flush on shutdown).
///
/// `dropped_counter` is the prometheus counter for "events that could
/// not be queued because the channel was full". The atomic counter
/// shared with the Layer is reconciled into prometheus on each flush
/// tick.
pub fn spawn_writer(
    handle: LogReporterHandle,
    path: PathBuf,
    dropped_counter: Option<IntCounter>,
    mut shutdown: broadcast::Receiver<()>,
) -> tokio::task::JoinHandle<()> {
    let flush_ms = std::env::var("ERROR_LOG_FLUSH_MS")
        .ok()
        .and_then(|v| v.trim().parse::<u64>().ok())
        .unwrap_or(DEFAULT_FLUSH_MS);
    let batch_bytes = std::env::var("ERROR_LOG_BATCH_BYTES")
        .ok()
        .and_then(|v| v.trim().parse::<usize>().ok())
        .unwrap_or(DEFAULT_BATCH_BYTES);
    let max_buffer_bytes = std::env::var("ERROR_LOG_MAX_BUFFER_BYTES")
        .ok()
        .and_then(|v| v.trim().parse::<usize>().ok())
        .unwrap_or(DEFAULT_MAX_BUFFER_BYTES);

    let LogReporterHandle { mut rx, dropped: layer_dropped } = handle;

    tokio::spawn(async move {
        let mut file: Option<tokio::fs::File> = None;
        let mut buf: Vec<u8> = Vec::with_capacity(batch_bytes.min(max_buffer_bytes));
        let mut ticker = tokio::time::interval(Duration::from_millis(flush_ms.max(10)));
        ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);

        async fn flush(path: &PathBuf, file: &mut Option<tokio::fs::File>, buf: &mut Vec<u8>) {
            if buf.is_empty() {
                return;
            }
            if file.is_none() {
                match OpenOptions::new().create(true).append(true).open(path).await {
                    Ok(f) => *file = Some(f),
                    Err(e) => {
                        // We are the error-log writer; recursing into
                        // tracing here would deadlock. Use eprintln.
                        eprintln!(
                            "[log_reporter] open {} failed: {}",
                            path.display(),
                            e
                        );
                        return;
                    }
                }
            }
            if let Some(f) = file.as_mut() {
                if let Err(e) = f.write_all(buf).await {
                    eprintln!("[log_reporter] write failed: {}", e);
                    buf.clear();
                    *file = None;
                    return;
                }
                buf.clear();
            }
        }

        let sync_drops = |c: &Option<IntCounter>, last: &mut u64, atomic: &Arc<AtomicU64>| {
            if let Some(counter) = c.as_ref() {
                let cur = atomic.load(Ordering::Relaxed);
                let delta = cur.saturating_sub(*last);
                if delta > 0 {
                    counter.inc_by(delta);
                    *last = cur;
                }
            }
        };
        let mut last_synced_drop: u64 = 0;

        loop {
            tokio::select! {
                _ = shutdown.recv() => {
                    flush(&path, &mut file, &mut buf).await;
                    sync_drops(&dropped_counter, &mut last_synced_drop, &layer_dropped);
                    break;
                }
                _ = ticker.tick() => {
                    flush(&path, &mut file, &mut buf).await;
                    sync_drops(&dropped_counter, &mut last_synced_drop, &layer_dropped);
                }
                maybe = rx.recv() => {
                    let Some(line) = maybe else { break };
                    let bytes_needed = line.len().saturating_add(1);
                    if buf.len().saturating_add(bytes_needed) > max_buffer_bytes {
                        layer_dropped.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                    buf.extend_from_slice(line.as_bytes());
                    buf.push(b'\n');
                    if buf.len() >= batch_bytes {
                        flush(&path, &mut file, &mut buf).await;
                    }
                }
            }
        }

        // Best-effort final flush before exit.
        flush(&path, &mut file, &mut buf).await;
        sync_drops(&dropped_counter, &mut last_synced_drop, &layer_dropped);
    })
}
