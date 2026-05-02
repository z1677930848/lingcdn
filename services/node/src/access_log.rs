use serde::Serialize;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use tokio::fs::OpenOptions;
use tokio::io::AsyncWriteExt;
use tokio::sync::mpsc;
use tokio::time::Duration;
use tracing::{debug, warn};
use prometheus::IntCounter;

/// AccessLogger writes every accepted log line as JSON to the on-disk
/// rotating access.log. A node-side Filebeat tails the file and ships
/// entries to Elasticsearch — the node binary itself owns no network
/// connection for log delivery.
#[derive(Clone)]
pub struct AccessLogger {
    tx: mpsc::Sender<String>,
    dropped: Option<IntCounter>,
    counter: Arc<AtomicU64>,
    sample_every: u64,
    error_only: bool,
    min_status: u16,
    slow_ms: u64,
}

impl AccessLogger {
    /// Construct the logger. Spawns the background writer task that
    /// drains the mpsc channel into `path`.
    pub fn new(path: PathBuf, dropped: Option<IntCounter>) -> Self {
        let channel_size = std::env::var("ACCESS_LOG_CHANNEL_SIZE")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or(10_000);
        let (tx, mut rx) = mpsc::channel::<String>(channel_size);

        let flush_ms = std::env::var("ACCESS_LOG_FLUSH_MS")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .unwrap_or(200);
        let batch_bytes = std::env::var("ACCESS_LOG_BATCH_BYTES")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or(1024 * 1024);
        let max_buffer_bytes = std::env::var("ACCESS_LOG_MAX_BUFFER_BYTES")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            .unwrap_or(8 * 1024 * 1024);

        let dropped_for_task = dropped.clone();
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
                            warn!("Failed to open access log file {}: {}", path.display(), e);
                            return;
                        }
                    }
                }

                if let Some(f) = file.as_mut() {
                    if let Err(e) = f.write_all(buf).await {
                        warn!("Failed to write access log: {}", e);
                        // We don't know if the write was partial; drop buffered lines to avoid duplicates.
                        buf.clear();
                        *file = None;
                        return;
                    }
                    buf.clear();
                }
            }

            loop {
                tokio::select! {
                    _ = ticker.tick() => {
                        flush(&path, &mut file, &mut buf).await;
                    }
                    maybe = rx.recv() => {
                        let Some(line) = maybe else { break };

                        let bytes_needed = line.len().saturating_add(1);
                        if buf.len().saturating_add(bytes_needed) > max_buffer_bytes {
                            if let Some(c) = &dropped_for_task {
                                c.inc();
                            }
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

            // Best effort final flush.
            flush(&path, &mut file, &mut buf).await;
            debug!("access logger stopped");
        });

        let sample_every = std::env::var("ACCESS_LOG_SAMPLE_EVERY")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .unwrap_or(1)
            .max(1);
        let error_only = match std::env::var("ACCESS_LOG_ERROR_ONLY") {
            Ok(v) => matches!(v.trim().to_ascii_lowercase().as_str(), "1" | "true" | "yes" | "on"),
            Err(_) => false,
        };
        let min_status = std::env::var("ACCESS_LOG_MIN_STATUS")
            .ok()
            .and_then(|v| v.trim().parse::<u16>().ok())
            .unwrap_or(0);
        let slow_ms = std::env::var("ACCESS_LOG_SLOW_MS")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .unwrap_or(0);

        Self {
            tx,
            dropped,
            counter: Arc::new(AtomicU64::new(0)),
            sample_every,
            error_only,
            min_status,
            slow_ms,
        }
    }

    pub fn should_log(&self, status: u16, duration_ms: u64, has_error: bool) -> bool {
        let is_error = has_error || status >= 400;
        let is_slow = self.slow_ms > 0 && duration_ms >= self.slow_ms;

        if self.error_only {
            return is_error || is_slow;
        }

        if self.min_status > 0 && status < self.min_status && !is_error && !is_slow {
            return false;
        }

        // Always keep error/slow logs; only sample the "normal success" path.
        if is_error || is_slow {
            return true;
        }

        if self.sample_every <= 1 {
            return true;
        }

        let n = self.counter.fetch_add(1, Ordering::Relaxed);
        (n % self.sample_every) == 0
    }

    pub fn log<T: Serialize>(&self, entry: &T) {
        let Ok(line) = serde_json::to_string(entry) else { return };
        if self.tx.try_send(line).is_err() {
            if let Some(c) = &self.dropped {
                c.inc();
            }
        }
    }
}
