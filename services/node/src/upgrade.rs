use anyhow::{anyhow, Context, Result};
use base64::Engine;
use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use reqwest::redirect::Policy;
use reqwest::Client;
use semver::Version;
use serde::Deserialize;
use std::env;
use std::fs;
use std::path::PathBuf;
use std::process::Command;
use std::time::Duration;
use tokio::sync::broadcast::Receiver;
use tokio::sync::mpsc;
use tokio::time::interval;
use tracing::{info, warn};

use crate::config::NodeConfig;

const HARDCODED_UPGRADE_HOST: &str = "auth.lingcdn.cloud";

#[derive(Debug, Clone)]
pub struct UpgradeTrigger {
    pub task_id: String,
    pub target_version: String,
    pub channel: String,
    pub force: bool,
}

#[derive(Debug, Deserialize)]
#[allow(dead_code)]
struct LatestResponse {
    version: String,
    download_url: Option<String>,
    checksum: Option<String>,
    signature: Option<String>,
    sig_alg: Option<String>,
    sig_target: Option<String>,
    pubkey: Option<String>,
    size_bytes: Option<u64>,
    changelog: Option<String>,
    channel: Option<String>,
    platform: Option<String>,
    arch: Option<String>,
    build_id: Option<String>,
}

pub struct UpgradeRunner {
    endpoint: String,
    product: String,
    channel: String,
    platform: String,
    arch: String,
    check_every: Duration,
    current_version: String,
    pubkey_b64: String,
    client: Client,
    restart_argv: Option<Vec<String>>,
    max_download_bytes: u64,
}

impl UpgradeRunner {
    pub fn new(endpoint: String, cfg: NodeConfig) -> Self {
        let product = env::var("UPGRADE_PRODUCT").unwrap_or_else(|_| "node".to_string());
        let platform = env::consts::OS.to_string();
        let arch = normalize_arch(env::consts::ARCH);
        let restart_argv = env::var("UPGRADE_RESTART_CMD")
            .ok()
            .map(|s| {
                s.split_whitespace()
                    .map(|x| x.to_string())
                    .collect::<Vec<_>>()
            })
            .filter(|v| !v.is_empty());
        // Build the HTTP client with explicit timeouts. Previously this fell back
        // to `Client::new()` on failure, which has no timeouts at all and can
        // silently hang the upgrade worker on a slow mirror. If we cannot build
        // a properly-configured client we abort loudly so the process manager
        // restarts with visible logs.
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(10))
            .timeout(Duration::from_secs(600))
            .redirect(Policy::limited(3))
            .build()
            .unwrap_or_else(|e| {
                tracing::error!(error = %e, "upgrade: failed to build HTTP client with timeouts; aborting");
                panic!("upgrade: failed to build HTTP client: {}", e);
            });

        Self {
            endpoint,
            product,
            channel: cfg.upgrade_channel.clone(),
            platform,
            arch,
            check_every: Duration::from_secs(cfg.upgrade_check_seconds.max(60)),
            current_version: cfg.version.clone(),
            pubkey_b64: cfg.upgrade_pubkey.clone().unwrap_or_default(),
            client,
            restart_argv,
            max_download_bytes: cfg.upgrade_max_download_bytes.max(1),
        }
    }

    pub async fn run(
        &self,
        mut shutdown: Receiver<()>,
        mut triggers: mpsc::Receiver<UpgradeTrigger>,
    ) {
        let mut ticker = interval(self.check_every);
        let mut last_task_id: Option<String> = None;
        loop {
            tokio::select! {
                _ = ticker.tick() => {
                    if let Err(e) = self.check_once_with(None, false, None).await {
                        warn!("Upgrade check failed: {}", e);
                    }
                }
                maybe = triggers.recv() => {
                    if let Some(t) = maybe {
                        if !t.task_id.is_empty() && last_task_id.as_deref() == Some(t.task_id.as_str()) {
                            continue;
                        }
                        last_task_id = if t.task_id.is_empty() { last_task_id } else { Some(t.task_id.clone()) };

                        let target = t.target_version.trim();
                        let channel_override = t.channel.trim();
                        let req_ver = if target.is_empty() { None } else { Some(target) };
                        info!(
                            "Upgrade command received: task_id={}, target_version={}, channel={}, force={}",
                            t.task_id,
                            target,
                            if channel_override.is_empty() { self.channel.as_str() } else { channel_override },
                            t.force
                        );
                        let channel_override = if channel_override.is_empty() { None } else { Some(channel_override) };
                        if let Err(e) = self.check_once_with(req_ver, t.force, channel_override).await {
                            warn!("Upgrade command failed: {}", e);
                        }
                    }
                }
                _ = shutdown.recv() => {
                    info!("Upgrade watcher shutting down");
                    break;
                }
            }
        }
    }

    async fn check_once_with(
        &self,
        requested_version: Option<&str>,
        force: bool,
        channel_override: Option<&str>,
    ) -> Result<()> {
        let mut url = reqwest::Url::parse(&self.endpoint)
            .with_context(|| format!("invalid upgrade endpoint {}", self.endpoint))?;
        url.set_query(None);
        let channel = channel_override.unwrap_or(self.channel.as_str());
        {
            let mut q = url.query_pairs_mut();
            q.append_pair("product", self.product.as_str());
            q.append_pair("channel", channel);
            q.append_pair("platform", self.platform.as_str());
            q.append_pair("arch", self.arch.as_str());
            if let Some(v) = requested_version {
                let v = v.trim();
                if !v.is_empty() {
                    q.append_pair("version", v);
                }
            }
        }
        let resp = self
            .client
            .get(url.clone())
            .header("User-Agent", "lingcdn-node/upgrade")
            .send()
            .await
            .with_context(|| format!("request upgrade info {}", mask_url(url.as_str())))?;
        if !resp.status().is_success() {
            return Err(anyhow!("upgrade info http {}", resp.status()));
        }
        let latest: LatestResponse = resp.json().await?;

        let latest_version = latest.version.trim().to_string();
        if latest_version.is_empty() || latest.download_url.is_none() {
            return Err(anyhow!("upgrade info incomplete"));
        }
        if !force && !is_newer(&latest_version, &self.current_version) {
            info!("Already up to date: {}", self.current_version);
            return Ok(());
        }

        let checksum = latest
            .checksum
            .as_deref()
            .unwrap_or_default()
            .trim()
            .to_lowercase();
        if checksum.is_empty() {
            return Err(anyhow!("upgrade info missing checksum"));
        }
        let signature = latest
            .signature
            .as_deref()
            .unwrap_or_default()
            .trim()
            .to_string();
        // Security-critical: the pubkey MUST be baked into the node's
        // configuration (UPGRADE_PUBKEY env / config file). The previous
        // implementation fell back to `latest.pubkey` from the HTTP
        // response, which let a compromised (or MITM'd) portal hand the
        // node its own pubkey along with a matching signature — bypassing
        // the entire verification. We now ignore `latest.pubkey` entirely
        // and require the operator-provisioned key.
        let pubkey = self.pubkey_b64.trim();
        if pubkey.is_empty() {
            return Err(anyhow!(
                "upgrade refused: no local pubkey configured (set UPGRADE_PUBKEY to the operator's ed25519 public key); signature from portal cannot be trusted without it"
            ));
        }
        if signature.is_empty() {
            return Err(anyhow!(
                "upgrade refused: portal returned no signature for this release; relying on HTTPS + SHA256 is insufficient because HTTPS only authenticates the transport, not the binary"
            ));
        }
        let sig_alg = latest.sig_alg.as_deref().unwrap_or_default().trim();
        if !sig_alg.is_empty() && sig_alg != "ed25519" {
            return Err(anyhow!("unsupported sig_alg {}", sig_alg));
        }
        let sig_target = latest.sig_target.as_deref().unwrap_or_default().trim();
        if !sig_target.is_empty() && sig_target != "sha256" {
            return Err(anyhow!("unsupported sig_target {}", sig_target));
        }
        verify_checksum_signature_sha256(pubkey, &checksum, &signature)?;

        if is_newer(&latest_version, &self.current_version) {
            info!(
                "Upgrade available: {} -> {}, channel={}",
                self.current_version,
                latest_version,
                latest.channel.as_deref().unwrap_or("")
            );
        } else {
            info!(
                "Force upgrade requested: current={}, target={}, channel={}",
                self.current_version,
                latest_version,
                latest.channel.as_deref().unwrap_or("")
            );
        }

        self.perform_upgrade(&latest_version, latest).await
    }

    async fn perform_upgrade(&self, target_version: &str, latest: LatestResponse) -> Result<()> {
        let mut download_url = latest
            .download_url
            .clone()
            .ok_or_else(|| anyhow!("missing download_url"))?;

        // Resolve relative download_url against upgrade endpoint origin.
        if download_url.starts_with('/') {
            let base = reqwest::Url::parse(&self.endpoint)
                .with_context(|| format!("invalid upgrade endpoint {}", self.endpoint))?;
            download_url = base
                .join(&download_url)
                .with_context(|| format!("resolve download_url {}", download_url))?
                .to_string();
        }

        let parsed = reqwest::Url::parse(&download_url)
            .with_context(|| format!("invalid download_url {}", mask_url(&download_url)))?;
        let host = parsed.domain().unwrap_or_default();
        if parsed.scheme() != "https" || !host.eq_ignore_ascii_case(HARDCODED_UPGRADE_HOST) {
            return Err(anyhow!(
                "untrusted download_url host (only allow https://{}): {}",
                HARDCODED_UPGRADE_HOST,
                mask_url(&download_url)
            ));
        }
        let checksum = latest
            .checksum
            .as_deref()
            .unwrap_or_default()
            .trim()
            .to_ascii_lowercase();
        if checksum.is_empty() {
            return Err(anyhow!("upgrade info missing checksum"));
        }

        let exe = env::current_exe().context("current_exe")?;
        let exe_dir = exe
            .parent()
            .ok_or_else(|| anyhow!("exe has no parent"))?
            .to_path_buf();

        if let Some(sz) = latest.size_bytes {
            if sz > self.max_download_bytes {
                return Err(anyhow!("download too large: {} bytes", sz));
            }
        }

        let script_url = format!("https://{}/node_update.sh", HARDCODED_UPGRADE_HOST);
        info!("Downloading node update script: {}", mask_url(&script_url));
        let script_resp = self
            .client
            .get(script_url.clone())
            .send()
            .await
            .with_context(|| format!("download script {}", mask_url(&script_url)))?;
        if !script_resp.status().is_success() {
            return Err(anyhow!(
                "download node_update.sh http {}",
                script_resp.status()
            ));
        }
        let script_content = script_resp
            .bytes()
            .await
            .with_context(|| format!("read script body {}", mask_url(&script_url)))?;

        let script_path = exe_dir.join("lingcdn-node-update.sh");
        fs::write(&script_path, &script_content).context("write node update script")?;
        set_exec_perm(&script_path)?;

        let service_name =
            env::var("UPGRADE_SERVICE_NAME").unwrap_or_else(|_| "lingcdn-node".to_string());
        info!(
            "Executing node_update.sh for {}, artifact={} ",
            target_version,
            mask_url(&download_url)
        );
        let status = Command::new("bash")
            .arg(&script_path)
            .arg("--download_url")
            .arg(download_url)
            .arg("--checksum")
            .arg(checksum)
            .arg("--version")
            .arg(target_version)
            .arg("--binary_path")
            .arg(exe.as_os_str())
            .arg("--service_name")
            .arg(service_name)
            // 让脚本自己 systemctl restart 收尾。改前传的是 true，再让 Rust 这边
            // std::process::exit(0)；但安装脚本下发的是 Restart=on-failure，systemd
            // 看到 exit 0 不当作失败，根本不会重启，节点直接停机。改成 false 后由
            // systemctl restart 来切换到新 ELF，与 Restart= 设置无关，最稳妥。
            .arg("--skip_restart")
            .arg("false")
            .status()
            .context("execute node_update.sh")?;
        if !status.success() {
            let _ = fs::remove_file(&script_path);
            return Err(anyhow!("node_update.sh failed: {:?}", status));
        }
        let _ = fs::remove_file(&script_path);

        info!(
            "node_update.sh completed, restarting to apply {}",
            target_version
        );
        self.restart(target_version)?;

        Ok(())
    }

    fn restart(&self, target_version: &str) -> Result<()> {
        if let Some(argv) = &self.restart_argv {
            warn!("Using restart argv: {:?}", argv);
            let status = Command::new(&argv[0]).args(&argv[1..]).status()?;
            if !status.success() {
                return Err(anyhow!("restart cmd failed: {:?}", status));
            }
            return Ok(());
        }

        // 默认按 systemd Restart=always 方案：直接退出，让服务管理器拉起新进程。
        // 如需自定义重启方式，请配置 UPGRADE_RESTART_CMD（例如 systemctl restart lingcdn-node）。
        info!(
            "Exiting to apply update {}, waiting supervisor to restart",
            target_version
        );
        std::process::exit(0);
    }
}

fn is_newer(latest: &str, current: &str) -> bool {
    let latest = latest.trim().trim_start_matches('v');
    let current = current.trim().trim_start_matches('v');
    match (Version::parse(latest), Version::parse(current)) {
        (Ok(l), Ok(c)) => l > c,
        _ => latest != current,
    }
}

fn mask_url(url: &str) -> String {
    if let Ok(mut parsed) = reqwest::Url::parse(url) {
        parsed.set_query(None);
        return parsed.to_string();
    }
    url.to_string()
}

fn set_exec_perm(_path: &PathBuf) -> Result<()> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mut perms = fs::metadata(_path)?.permissions();
        perms.set_mode(0o755);
        fs::set_permissions(_path, perms)?;
    }
    Ok(())
}

fn normalize_arch(raw: &str) -> String {
    match raw.trim().to_ascii_lowercase().as_str() {
        "amd64" | "x86_64" => "amd64".to_string(),
        "arm64" | "aarch64" => "arm64".to_string(),
        other => other.to_string(),
    }
}

fn verify_checksum_signature_sha256(
    pubkey_b64: &str,
    checksum_hex: &str,
    signature_b64: &str,
) -> Result<()> {
    let pub_raw = base64::engine::general_purpose::STANDARD
        .decode(pubkey_b64.trim())
        .context("decode pubkey base64")?;
    if pub_raw.len() != 32 {
        return Err(anyhow!("invalid pubkey length {}", pub_raw.len()));
    }
    let sig_raw = base64::engine::general_purpose::STANDARD
        .decode(signature_b64.trim())
        .context("decode signature base64")?;
    if sig_raw.len() != 64 {
        return Err(anyhow!("invalid signature length {}", sig_raw.len()));
    }

    let checksum = checksum_hex.trim().to_ascii_lowercase();
    if checksum.len() != 64 || !checksum.chars().all(|c| c.is_ascii_hexdigit()) {
        return Err(anyhow!("invalid sha256 {}", checksum));
    }
    let msg = format!("lingcdn:v1:sha256:{}", checksum);

    let pub_arr: [u8; 32] = pub_raw
        .as_slice()
        .try_into()
        .map_err(|_| anyhow!("invalid pubkey length {}", pub_raw.len()))?;
    let sig_arr: [u8; 64] = sig_raw
        .as_slice()
        .try_into()
        .map_err(|_| anyhow!("invalid signature length {}", sig_raw.len()))?;

    let key = VerifyingKey::from_bytes(&pub_arr).context("invalid pubkey")?;
    let sig = Signature::from_bytes(&sig_arr);
    key.verify(msg.as_bytes(), &sig)
        .context("signature verify failed")?;
    Ok(())
}
