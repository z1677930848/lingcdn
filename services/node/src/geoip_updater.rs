use anyhow::{anyhow, Context, Result};
use reqwest::Client;
use reqwest::redirect::Policy;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::time::Duration;
use tokio::sync::broadcast::Receiver;
use tokio::time::interval;
use tracing::{info, warn};

use crate::config::NodeConfig;
use crate::geoip::GeoIpResolver;
use crate::geoip_holder::GeoIpHolder;

#[derive(Debug, Deserialize)]
struct GeoIpLatestResponse {
	sha256: String,
	download_url: String,
	size_bytes: Option<u64>,
}

pub struct GeoIpUpdater {
	endpoint: String,
	interval: Duration,
	target_path: PathBuf,
	client: Client,
	auth_token: Option<String>,
	max_download_bytes: u64,
	holder: std::sync::Arc<GeoIpHolder>,
	last_sha: tokio::sync::Mutex<Option<String>>,
}

impl GeoIpUpdater {
	pub fn new(cfg: &NodeConfig, holder: std::sync::Arc<GeoIpHolder>) -> Option<Self> {
		let endpoint = cfg.geoip_update_endpoint.as_deref()?.trim().to_string();
		let path = cfg.geoip_db_path.as_deref()?.trim().to_string();
		if endpoint.is_empty() || path.is_empty() {
			return None;
		}

		let auth_token = cfg
			.geoip_update_token
			.clone()
			.or_else(|| if cfg.bootstrap_token.trim().is_empty() { None } else { Some(cfg.bootstrap_token.clone()) });

		let interval_secs = cfg.geoip_update_seconds.max(3600);
		let max_download_bytes = cfg.geoip_update_max_download_bytes.max(1);

		Some(Self {
			endpoint,
			interval: Duration::from_secs(interval_secs),
			target_path: PathBuf::from(path),
			client: Client::builder()
				.connect_timeout(Duration::from_secs(10))
				.timeout(Duration::from_secs(600))
				.redirect(Policy::limited(3))
				.build()
				.unwrap_or_else(|_| Client::new()),
			auth_token,
			max_download_bytes,
			holder,
			last_sha: tokio::sync::Mutex::new(None),
		})
	}

	pub async fn run(self, mut shutdown: Receiver<()>) {
		let mut ticker = interval(self.interval);
		{
			let current = sha256_file(self.target_path.as_path()).ok();
			*self.last_sha.lock().await = current;
		}

		loop {
			tokio::select! {
				_ = shutdown.recv() => break,
				_ = ticker.tick() => {
					if let Err(e) = self.check_once().await {
						warn!("GeoIP update failed: {}", e);
					}
				}
			}
		}
	}

	async fn check_once(&self) -> Result<()> {
		let latest = self.fetch_latest().await?;
		let expected_sha = latest.sha256.trim().to_lowercase();
		if expected_sha.is_empty() {
			return Err(anyhow!("geoip latest missing sha256"));
		}

		{
			let last = self.last_sha.lock().await.clone();
			if last.as_deref() == Some(expected_sha.as_str()) {
				return Ok(());
			}
		}

		let url = self.resolve_download_url(&latest.download_url)?;
		let tmp_path = temp_target_path(self.target_path.as_path())?;
		download_to_file(&self.client, &url, self.auth_token.as_deref(), &tmp_path, self.max_download_bytes, latest.size_bytes).await?;

		let actual_sha = sha256_file(&tmp_path)?.to_lowercase();
		if actual_sha != expected_sha {
			let _ = fs::remove_file(&tmp_path);
			return Err(anyhow!("geoip sha256 mismatch"));
		}

		apply_atomic_replace(self.target_path.as_path(), &tmp_path)?;
		let resolver = GeoIpResolver::from_path(self.target_path.as_path()).context("reload geoip db failed")?;
		self.holder.set(Some(std::sync::Arc::new(resolver)));
		*self.last_sha.lock().await = Some(expected_sha.clone());
		info!("GeoIP updated and reloaded: {}", expected_sha);
		Ok(())
	}

	async fn fetch_latest(&self) -> Result<GeoIpLatestResponse> {
		let mut req = self.client.get(self.endpoint.as_str()).header("User-Agent", "lingcdn-node/geoip");
		if let Some(token) = self.auth_token.as_deref() {
			req = req.header("Authorization", format!("Bearer {}", token));
		}
		let resp = req.send().await.context("request geoip latest")?;
		if !resp.status().is_success() {
			return Err(anyhow!("geoip latest http {}", resp.status()));
		}
		let latest: GeoIpLatestResponse = resp.json().await.context("parse geoip latest")?;
		Ok(latest)
	}

	fn resolve_download_url(&self, raw: &str) -> Result<String> {
		let raw = raw.trim();
		if raw.is_empty() {
			return Err(anyhow!("geoip latest missing download_url"));
		}
		if raw.starts_with("http://") || raw.starts_with("https://") {
			return Ok(raw.to_string());
		}
		let base = reqwest::Url::parse(&self.endpoint).with_context(|| format!("invalid geoip endpoint {}", self.endpoint))?;
		let joined = base.join(raw).with_context(|| format!("invalid download_url {}", raw))?;
		Ok(joined.to_string())
	}
}

async fn download_to_file(
	client: &Client,
	url: &str,
	auth_token: Option<&str>,
	out_path: &Path,
	max_bytes: u64,
	expected_size: Option<u64>,
) -> Result<()> {
	let mut req = client.get(url).header("User-Agent", "lingcdn-node/geoip");
	if let Some(token) = auth_token {
		req = req.header("Authorization", format!("Bearer {}", token));
	}
	let resp = req.send().await.context("download geoip file")?;
	if !resp.status().is_success() {
		return Err(anyhow!("geoip file http {}", resp.status()));
	}

	if let Some(len) = resp.content_length() {
		if len > max_bytes {
			return Err(anyhow!("geoip file too large"));
		}
	}
	if let Some(sz) = expected_size {
		if sz > max_bytes {
			return Err(anyhow!("geoip file too large"));
		}
	}

	if let Some(parent) = out_path.parent() {
		let _ = fs::create_dir_all(parent);
	}
	let mut f = fs::File::create(out_path).context("create tmp file")?;
	let mut stream = resp.bytes_stream();
	let mut written: u64 = 0;
	while let Some(chunk) = stream.next().await {
		let chunk = chunk.context("read chunk")?;
		written = written.saturating_add(chunk.len() as u64);
		if written > max_bytes {
			let _ = fs::remove_file(out_path);
			return Err(anyhow!("geoip file too large"));
		}
		f.write_all(&chunk).context("write chunk")?;
	}
	f.flush().ok();
	Ok(())
}

fn sha256_file(path: &Path) -> Result<String> {
	if !path.exists() {
		return Err(anyhow!("file not found"));
	}
	let data = fs::read(path).with_context(|| format!("read {}", path.display()))?;
	let mut hasher = Sha256::new();
	hasher.update(&data);
	let out = hasher.finalize();
	Ok(hex::encode(out))
}

fn temp_target_path(target: &Path) -> Result<PathBuf> {
	let parent = target.parent().ok_or_else(|| anyhow!("invalid target path"))?;
	let base = target.file_name().and_then(|s| s.to_str()).unwrap_or("geoip.mmdb");
	Ok(parent.join(format!("{}.new", base)))
}

fn apply_atomic_replace(target: &Path, tmp: &Path) -> Result<()> {
	let parent = target.parent().ok_or_else(|| anyhow!("invalid target path"))?;
	let _ = fs::create_dir_all(parent);
	let old = parent.join(format!(
		"{}.old",
		target.file_name().and_then(|s| s.to_str()).unwrap_or("geoip.mmdb")
	));
	let _ = fs::remove_file(&old);
	if target.exists() {
		let _ = fs::rename(target, &old);
	}
	fs::rename(tmp, target).with_context(|| format!("rename {} -> {}", tmp.display(), target.display()))?;
	Ok(())
}

use futures::StreamExt;

