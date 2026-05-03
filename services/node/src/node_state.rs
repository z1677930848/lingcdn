// Persisted node identity (node_id + per-node token) so the service can
// re-register on restart without consuming another bootstrap token. The
// control plane's authorizeNodeRegistration accepts either a valid bootstrap
// token OR the raw per-node token matching the stored hash — so we just
// re-submit the last known per-node token as the "bootstrap_token" field.
//
// State is written atomically (tmp + rename) to NODE_STATE_PATH (default
// /etc/lingcdn/node.state.json) with 0600 permissions so the token raw is
// readable only by the service user.

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};

const DEFAULT_STATE_PATH: &str = "/etc/lingcdn/node.state.json";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NodeState {
    pub node_id: String,
    /// Raw per-node token as returned by RegisterNodeResponse. The control
    /// plane stores only its sha256, but also accepts this raw value as the
    /// "bootstrap_token" on re-registration (see authorizeNodeRegistration).
    pub node_token: String,
}

/// Path where node state is persisted. Overridable via NODE_STATE_PATH for
/// tests and non-standard deployments.
pub fn state_path() -> PathBuf {
    if let Ok(v) = std::env::var("NODE_STATE_PATH") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            return PathBuf::from(trimmed);
        }
    }
    PathBuf::from(DEFAULT_STATE_PATH)
}

/// Read persisted state. Returns Ok(None) when the file is missing (first
/// start); Err only on unexpected IO / parse errors.
pub fn load() -> Result<Option<NodeState>> {
    load_from(&state_path())
}

pub fn load_from(path: &Path) -> Result<Option<NodeState>> {
    match fs::read(path) {
        Ok(bytes) => {
            let s: NodeState = serde_json::from_slice(&bytes)
                .with_context(|| format!("parse node state file {}", path.display()))?;
            if s.node_id.trim().is_empty() || s.node_token.trim().is_empty() {
                return Ok(None);
            }
            Ok(Some(s))
        }
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(e) => Err(anyhow::Error::from(e)
            .context(format!("read node state file {}", path.display()))),
    }
}

/// Persist state atomically. Writes <path>.tmp, fsyncs, then renames over
/// the destination so a crash mid-write cannot leave a truncated file.
pub fn save(state: &NodeState) -> Result<()> {
    save_to(&state_path(), state)
}

pub fn save_to(path: &Path, state: &NodeState) -> Result<()> {
    if let Some(parent) = path.parent() {
        if !parent.as_os_str().is_empty() {
            fs::create_dir_all(parent)
                .with_context(|| format!("create parent dir {}", parent.display()))?;
        }
    }

    let bytes = serde_json::to_vec_pretty(state).context("serialize node state")?;

    let tmp = path.with_extension("json.tmp");
    {
        let mut opts = fs::OpenOptions::new();
        opts.write(true).create(true).truncate(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            opts.mode(0o600);
        }
        let mut f = opts
            .open(&tmp)
            .with_context(|| format!("open tmp {}", tmp.display()))?;
        f.write_all(&bytes)
            .with_context(|| format!("write tmp {}", tmp.display()))?;
        f.sync_all()
            .with_context(|| format!("fsync tmp {}", tmp.display()))?;
    }

    fs::rename(&tmp, path)
        .with_context(|| format!("rename {} -> {}", tmp.display(), path.display()))?;
    Ok(())
}
