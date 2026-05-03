use anyhow::{Context, Result};
use bytes::Bytes;
use lru::LruCache;
use parking_lot::RwLock;
use sled::Db;
use std::num::NonZeroUsize;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};
use tracing::{debug, info};
use std::env;
use rand::{thread_rng, Rng};
use sha2::{Digest, Sha256};
use tokio::sync::{OwnedSemaphorePermit, Semaphore};

/// Cache key for storing responses
#[derive(Debug, Clone, Hash, Eq, PartialEq)]
pub struct CacheKey {
    pub host: String,
    pub path: String,
    pub query: Option<String>,
}

impl std::fmt::Display for CacheKey {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match &self.query {
            Some(q) => write!(f, "{}{}?{}", self.host, self.path, q),
            None => write!(f, "{}{}", self.host, self.path),
        }
    }
}

impl CacheKey {
    pub fn new(host: String, path: String, query: Option<String>) -> Self {
        Self { host, path, query }
    }

    pub fn to_string(&self) -> String {
        format!("{}", self)
    }

    #[allow(dead_code)]
    pub fn from_url(url: &str) -> Result<Self> {
        let parsed = url::Url::parse(url).context("Invalid URL")?;
        let host = parsed.host_str().unwrap_or("").to_string();
        let path = parsed.path().to_string();
        let query = parsed.query().map(|q| q.to_string());
        Ok(Self::new(host, path, query))
    }
}

/// Cached response entry
#[derive(Debug, Clone)]
pub struct CacheEntry {
    pub status: u16,
    pub headers: Vec<(Box<str>, Box<str>)>,
    pub body: CachedBody,
    pub created_at: u64,
    pub ttl_seconds: u64,
}

#[derive(Debug, Clone)]
pub enum CachedBody {
    Bytes(Bytes),
    File { path: PathBuf, len: u64 },
}

impl CacheEntry {
    pub fn is_expired(&self) -> bool {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        now > self.created_at + self.ttl_seconds
    }

    pub fn body_len(&self) -> u64 {
        match &self.body {
            CachedBody::Bytes(b) => b.len() as u64,
            CachedBody::File { len, .. } => *len,
        }
    }

    pub fn memory_size_bytes(&self) -> u64 {
        match &self.body {
            CachedBody::Bytes(b) => b.len() as u64,
            CachedBody::File { .. } => 0,
        }
    }
}

fn now_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
enum DiskValue {
    Inline {
        status: u16,
        headers: Vec<(Box<str>, Box<str>)>,
        body: Vec<u8>,
        created_at: u64,
        ttl_seconds: u64,
    },
    File {
        status: u16,
        headers: Vec<(Box<str>, Box<str>)>,
        file_rel: String,
        len: u64,
        created_at: u64,
        ttl_seconds: u64,
    },
}

pub struct DiskCachePaths {
    pub tmp_path: PathBuf,
    pub final_path: PathBuf,
    pub file_rel: String,
}

/// Two-tier cache: memory (LRU) + disk (sled)
pub struct Cache {
    memory: Arc<RwLock<LruCache<String, CacheEntry>>>,
    memory_enabled: bool,
    disk: Arc<RwLock<Option<Db>>>,
    disk_root: Option<PathBuf>,
    #[allow(dead_code)]
    // 预留：磁盘缓存元数据目录
    disk_meta_dir: Option<PathBuf>,
    disk_objects_dir: Option<PathBuf>,
    disk_usage_bytes: Arc<AtomicU64>,
    stats: Arc<RwLock<CacheStats>>,
    disk_over_limit: Arc<AtomicBool>,
    // Optional byte cap for the in-memory cache (in addition to the entry-count LRU cap).
    // This protects the node from OOM when caching large objects.
    memory_max_bytes: u64,
    // Optional cutoff: objects larger than this should not be cached in memory.
    memory_max_object_bytes: u64,
    // Optional cutoff: objects larger than this should not be cached to disk.
    disk_max_object_bytes: u64,

    // Admission control for concurrent cache writes (especially for large objects).
    write_inflight_bytes: Arc<std::sync::atomic::AtomicU64>,
    write_inflight_max_bytes: u64,
    write_semaphore: Option<Arc<Semaphore>>,
}

pub struct CacheWritePermit {
    bytes: u64,
    counter: Arc<std::sync::atomic::AtomicU64>,
    _permit: Option<OwnedSemaphorePermit>,
}

impl Drop for CacheWritePermit {
    fn drop(&mut self) {
        if self.bytes > 0 {
            self.counter.fetch_sub(self.bytes, Ordering::Relaxed);
        }
    }
}

#[derive(Debug, Default, Clone)]
pub struct CacheStats {
    pub hits: u64,
    pub misses: u64,
    pub memory_hits: u64,
    pub disk_hits: u64,
    #[allow(dead_code)]
    pub evictions: u64,
    #[allow(dead_code)]
    pub size_bytes: u64,
}

impl CacheStats {
    #[allow(dead_code)]
    pub fn hit_rate(&self) -> f64 {
        let total = self.hits + self.misses;
        if total == 0 {
            0.0
        } else {
            self.hits as f64 / total as f64
        }
    }
}

impl Cache {
    pub fn new(memory_capacity: usize, disk_path: Option<&Path>) -> Result<Self> {
        let memory_enabled = memory_capacity > 0;
        let memory_capacity = memory_capacity.max(1);
        let memory = Arc::new(RwLock::new(
            LruCache::new(NonZeroUsize::new(memory_capacity)
                .expect("memory_capacity must be > 0 after max(1)")),
        ));

        let (disk, disk_root, disk_meta_dir, disk_objects_dir) = if let Some(root) = disk_path {
            info!("Opening disk cache at {:?}", root);
            std::fs::create_dir_all(root).context("Failed to create disk cache dir")?;

            let meta_dir = root.join("meta");
            let objects_dir = root.join("objects");
            std::fs::create_dir_all(&meta_dir).context("Failed to create disk cache meta dir")?;
            std::fs::create_dir_all(&objects_dir).context("Failed to create disk cache objects dir")?;

            let db = sled::open(&meta_dir).context("Failed to open disk cache meta db")?;
            (Some(db), Some(root.to_path_buf()), Some(meta_dir), Some(objects_dir))
        } else {
            (None, None, None, None)
        };

        let memory_max_bytes = env::var("MEMORY_CACHE_MAX_BYTES")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            // Default scales with RAM unless NODE_AUTOTUNE=0/false.
            .unwrap_or_else(|| crate::autotune::default_memory_cache_max_bytes());

        let memory_max_object_bytes = env::var("MEMORY_CACHE_MAX_OBJECT_BYTES")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            // Default: keep large objects off-heap (disk cache) to avoid buffering spikes.
            .unwrap_or(1024 * 1024);

        let disk_max_object_bytes = env::var("DISK_CACHE_MAX_OBJECT_BYTES")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            .unwrap_or(0);

        let write_inflight_max_bytes = env::var("CACHE_WRITE_INFLIGHT_MAX_BYTES")
            .ok()
            .and_then(|v| v.trim().parse::<u64>().ok())
            // Default scales with RAM unless NODE_AUTOTUNE=0/false.
            .unwrap_or_else(|| crate::autotune::default_cache_write_inflight_max_bytes());

        let write_concurrency = env::var("CACHE_WRITE_MAX_CONCURRENCY")
            .ok()
            .and_then(|v| v.trim().parse::<usize>().ok())
            // Default scales with RAM/CPU unless NODE_AUTOTUNE=0/false.
            .unwrap_or_else(|| crate::autotune::default_cache_write_concurrency());
        let write_semaphore = if write_concurrency > 0 {
            Some(Arc::new(Semaphore::new(write_concurrency)))
        } else {
            None
        };

        Ok(Self {
            memory,
            memory_enabled,
            disk: Arc::new(RwLock::new(disk)),
            disk_root,
            disk_meta_dir,
            disk_objects_dir,
            disk_usage_bytes: Arc::new(AtomicU64::new(0)),
            stats: Arc::new(RwLock::new(CacheStats::default())),
            disk_over_limit: Arc::new(AtomicBool::new(false)),
            memory_max_bytes,
            memory_max_object_bytes,
            disk_max_object_bytes,
            write_inflight_bytes: Arc::new(std::sync::atomic::AtomicU64::new(0)),
            write_inflight_max_bytes,
            write_semaphore,
        })
    }

    pub fn disk_enabled(&self) -> bool {
        self.disk.read().is_some() && self.disk_root.is_some() && self.disk_objects_dir.is_some()
    }

    #[allow(dead_code)]
    // 预留：用于监控当前磁盘缓存占用
    pub fn disk_usage_bytes(&self) -> u64 {
        self.disk_usage_bytes.load(Ordering::Relaxed)
    }

    fn disk_usage_add(&self, bytes: u64) {
        if bytes == 0 {
            return;
        }
        self.disk_usage_bytes.fetch_add(bytes, Ordering::Relaxed);
    }

    fn disk_usage_sub(&self, bytes: u64) {
        if bytes == 0 {
            return;
        }
        loop {
            let cur = self.disk_usage_bytes.load(Ordering::Relaxed);
            let next = cur.saturating_sub(bytes);
            if self
                .disk_usage_bytes
                .compare_exchange(cur, next, Ordering::Relaxed, Ordering::Relaxed)
                .is_ok()
            {
                break;
            }
        }
    }

    #[allow(dead_code)]
    // 预留：需要强制重扫磁盘缓存时使用
    /// Recompute the on-disk cache size by scanning the filesystem (blocking).
    ///
    /// This only accounts for the cache-owned directories (meta + objects), not other files under cache_dir.
    pub fn recalc_disk_usage_from_fs(&self) -> Result<u64> {
        if !self.disk_enabled() {
            self.disk_usage_bytes.store(0, Ordering::Relaxed);
            return Ok(0);
        }

        let mut total: u64 = 0;
        if let Some(ref meta) = self.disk_meta_dir {
            total = total.saturating_add(dir_size(meta)?);
        }
        if let Some(ref objects) = self.disk_objects_dir {
            total = total.saturating_add(dir_size(objects)?);
        }

        self.disk_usage_bytes.store(total, Ordering::Relaxed);
        Ok(total)
    }

    pub fn disk_over_limit(&self) -> bool {
        self.disk_over_limit.load(Ordering::Relaxed)
    }

    pub fn memory_max_object_bytes(&self) -> u64 {
        self.memory_max_object_bytes
    }

    pub fn disk_max_object_bytes(&self) -> u64 {
        self.disk_max_object_bytes
    }

    pub fn try_acquire_write_permit(&self, expected_bytes: u64) -> Option<CacheWritePermit> {
        let sem_permit = if let Some(sem) = &self.write_semaphore {
            sem.clone().try_acquire_owned().ok()
        } else {
            None
        };

        let bytes = if self.write_inflight_max_bytes > 0 {
            expected_bytes
        } else {
            0
        };

        if self.write_inflight_max_bytes > 0 && bytes > 0 {
            loop {
                let cur = self.write_inflight_bytes.load(Ordering::Relaxed);
                if cur.saturating_add(bytes) > self.write_inflight_max_bytes {
                    return None;
                }
                if self
                    .write_inflight_bytes
                    .compare_exchange(cur, cur.saturating_add(bytes), Ordering::Relaxed, Ordering::Relaxed)
                    .is_ok()
                {
                    break;
                }
            }
        }

        Some(CacheWritePermit {
            bytes,
            counter: self.write_inflight_bytes.clone(),
            _permit: sem_permit,
        })
    }

    pub fn prepare_disk_paths(&self, key: &CacheKey) -> Result<Option<DiskCachePaths>> {
        if !self.disk_enabled() {
            return Ok(None);
        }
        let root = self.disk_root.as_ref().context("disk_root missing")?;
        let objects_dir = self.disk_objects_dir.as_ref().context("disk_objects_dir missing")?;

        let key_str = key.to_string();
        let hash = {
            let mut h = Sha256::new();
            h.update(key_str.as_bytes());
            hex::encode(h.finalize())
        };
        let prefix = &hash[..2];
        let subdir = objects_dir.join(prefix);
        std::fs::create_dir_all(&subdir).context("Failed to create cache object subdir")?;

        let file_name = format!("{}.bin", hash);
        let file_rel = format!("objects/{}/{}", prefix, file_name);
        let final_path = subdir.join(&file_name);
        let tmp_path = subdir.join(format!("{}.tmp-{:016x}", hash, thread_rng().gen::<u64>()));

        // Ensure the returned final path lives under disk_root.
        let _ = root;

        Ok(Some(DiskCachePaths {
            tmp_path,
            final_path,
            file_rel,
        }))
    }

    pub fn put_disk_file(
        &self,
        key: CacheKey,
        status: u16,
        headers: Vec<(Box<str>, Box<str>)>,
        file_rel: String,
        len: u64,
        created_at: u64,
        ttl_seconds: u64,
    ) -> Result<()> {
        if self.disk_over_limit.load(Ordering::Relaxed) {
            return Ok(());
        }
        let Some(ref disk) = *self.disk.read() else {
            return Ok(());
        };

        // If we are overwriting an existing entry, account for the previous size.
        if let Ok(Some(old)) = disk.get(key.to_string().as_bytes()) {
            if let Ok(prev) = bincode::deserialize::<DiskValue>(&old) {
                match prev {
                    DiskValue::Inline { body, .. } => self.disk_usage_sub(body.len() as u64),
                    DiskValue::File { len, .. } => self.disk_usage_sub(len),
                }
            }
        }

        let dv = DiskValue::File {
            status,
            headers,
            file_rel,
            len,
            created_at,
            ttl_seconds,
        };
        let data = bincode::serialize(&dv).context("Failed to serialize disk cache entry")?;
        disk.insert(key.to_string().as_bytes(), data.as_slice())
            .context("Failed to write to disk cache")?;
        self.disk_usage_add(len);
        Ok(())
    }

    fn disk_absolute_path(&self, file_rel: &str) -> Option<PathBuf> {
        let root = self.disk_root.as_ref()?;
        Some(root.join(file_rel))
    }

    pub(crate) fn put_memory_only(&self, key_str: String, entry: CacheEntry) {
        if !self.memory_enabled {
            return;
        }
        let entry_bytes = entry.memory_size_bytes();
        if entry_bytes == 0 {
            return;
        }

        let mut mem = self.memory.write();
        if let Some((old_key, old_entry)) = mem.push(key_str.clone(), entry) {
            let mut stats = self.stats.write();
            stats.size_bytes = stats.size_bytes.saturating_sub(old_entry.memory_size_bytes());
            if old_key != key_str {
                stats.evictions += 1;
            }
        }

        {
            let mut stats = self.stats.write();
            stats.size_bytes = stats.size_bytes.saturating_add(entry_bytes);
        }

        if self.memory_max_bytes > 0 {
            loop {
                let over = { self.stats.read().size_bytes > self.memory_max_bytes };
                if !over {
                    break;
                }
                if let Some((_k, evicted)) = mem.pop_lru() {
                    let mut stats = self.stats.write();
                    stats.size_bytes = stats.size_bytes.saturating_sub(evicted.memory_size_bytes());
                    stats.evictions += 1;
                } else {
                    break;
                }
            }
        }
    }

    pub(crate) fn put_disk_inline(
        &self,
        key_str: String,
        status: u16,
        headers: Vec<(Box<str>, Box<str>)>,
        body: Bytes,
        created_at: u64,
        ttl_seconds: u64,
    ) -> Result<()> {
        if self.disk_over_limit.load(Ordering::Relaxed) {
            return Ok(());
        }
        let Some(ref disk) = *self.disk.read() else {
            return Ok(());
        };

        // If we are overwriting an existing entry, account for the previous size.
        if let Ok(Some(old)) = disk.get(key_str.as_bytes()) {
            if let Ok(prev) = bincode::deserialize::<DiskValue>(&old) {
                match prev {
                    DiskValue::Inline { body, .. } => self.disk_usage_sub(body.len() as u64),
                    DiskValue::File { len, .. } => self.disk_usage_sub(len),
                }
            }
        }

        let dv = DiskValue::Inline {
            status,
            headers,
            body: body.to_vec(),
            created_at,
            ttl_seconds,
        };
        let data = bincode::serialize(&dv).context("Failed to serialize disk cache entry")?;
        disk.insert(key_str.as_bytes(), data.as_slice())
            .context("Failed to write to disk cache")?;
        self.disk_usage_add(body.len() as u64);
        Ok(())
    }

    fn get_memory_hit(&self, key_str: &str) -> Option<CacheEntry> {
        if !self.memory_enabled {
            return None;
        }
        let mut mem = self.memory.write();
        if let Some(entry) = mem.get(key_str) {
            if entry.is_expired() {
                if let Some(expired) = mem.pop(key_str) {
                    let mut stats = self.stats.write();
                    stats.size_bytes = stats.size_bytes.saturating_sub(expired.memory_size_bytes());
                }
                return None;
            }

            debug!("Cache hit (memory): {}", key_str);
            let mut stats = self.stats.write();
            stats.hits += 1;
            stats.memory_hits += 1;
            return Some(entry.clone());
        }
        None
    }

    fn get_memory_hit_with_stale(&self, key_str: &str, max_stale: u64) -> Option<(CacheEntry, bool)> {
        if !self.memory_enabled {
            return None;
        }
        let mut mem = self.memory.write();
        if let Some(entry) = mem.get(key_str) {
            let now = now_secs();
            let expires_at = entry.created_at.saturating_add(entry.ttl_seconds);
            if now <= expires_at {
                debug!("Cache hit (memory): {}", key_str);
                let mut stats = self.stats.write();
                stats.hits += 1;
                stats.memory_hits += 1;
                return Some((entry.clone(), false));
            }
            if max_stale > 0 && now <= expires_at.saturating_add(max_stale) {
                debug!("Cache hit (memory/stale): {}", key_str);
                let mut stats = self.stats.write();
                stats.hits += 1;
                stats.memory_hits += 1;
                return Some((entry.clone(), true));
            }
            if let Some(expired) = mem.pop(key_str) {
                let mut stats = self.stats.write();
                stats.size_bytes = stats.size_bytes.saturating_sub(expired.memory_size_bytes());
            }
        }
        None
    }

    fn get_disk_hit(&self, key_str: &str) -> Option<CacheEntry> {
        let Some(ref disk) = *self.disk.read() else { return None };
        let Ok(Some(data)) = disk.get(key_str.as_bytes()) else { return None };

        let val: DiskValue = match bincode::deserialize(&data) {
            Ok(v) => v,
            Err(_) => {
                // Old/unknown format: drop it to avoid repeated decode attempts.
                let _ = disk.remove(key_str.as_bytes());
                return None;
            }
        };

        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs();

        match val {
            DiskValue::Inline { status, headers, body, created_at, ttl_seconds } => {
                if now > created_at.saturating_add(ttl_seconds) {
                    let _ = disk.remove(key_str.as_bytes());
                    self.disk_usage_sub(body.len() as u64);
                    return None;
                }

                debug!("Cache hit (disk/inline): {}", key_str);
                let entry = CacheEntry {
                    status,
                    headers,
                    body: CachedBody::Bytes(Bytes::from(body)),
                    created_at,
                    ttl_seconds,
                };

                // Promote to memory if under the optional per-object cutoff.
                if self.memory_max_object_bytes == 0 || entry.memory_size_bytes() <= self.memory_max_object_bytes {
                    self.put_memory_only(key_str.to_string(), entry.clone());
                }

                let mut stats = self.stats.write();
                stats.hits += 1;
                stats.disk_hits += 1;
                Some(entry)
            }
            DiskValue::File { status, headers, file_rel, len, created_at, ttl_seconds } => {
                if now > created_at.saturating_add(ttl_seconds) {
                    let _ = disk.remove(key_str.as_bytes());
                    if let Some(abs) = self.disk_absolute_path(&file_rel) {
                        let _ = std::fs::remove_file(abs);
                    }
                    self.disk_usage_sub(len);
                    return None;
                }

                let Some(abs) = self.disk_absolute_path(&file_rel) else {
                    let _ = disk.remove(key_str.as_bytes());
                    self.disk_usage_sub(len);
                    return None;
                };
                if !abs.exists() {
                    // Dangling meta - remove.
                    let _ = disk.remove(key_str.as_bytes());
                    self.disk_usage_sub(len);
                    return None;
                }

                debug!("Cache hit (disk/file): {}", key_str);
                let entry = CacheEntry {
                    status,
                    headers,
                    body: CachedBody::File { path: abs, len },
                    created_at,
                    ttl_seconds,
                };
                let mut stats = self.stats.write();
                stats.hits += 1;
                stats.disk_hits += 1;
                Some(entry)
            }
        }
    }

    fn get_disk_hit_with_stale(&self, key_str: &str, max_stale: u64) -> Option<(CacheEntry, bool)> {
        let Some(ref disk) = *self.disk.read() else { return None };
        let Ok(Some(data)) = disk.get(key_str.as_bytes()) else { return None };

        let val: DiskValue = match bincode::deserialize(&data) {
            Ok(v) => v,
            Err(_) => {
                let _ = disk.remove(key_str.as_bytes());
                return None;
            }
        };

        let now = now_secs();

        let mut is_stale = false;
        let mut expired = false;

        match &val {
            DiskValue::Inline { created_at, ttl_seconds, body, .. } => {
                let expires_at = created_at.saturating_add(*ttl_seconds);
                if now > expires_at {
                    if max_stale > 0 && now <= expires_at.saturating_add(max_stale) {
                        is_stale = true;
                    } else {
                        let _ = disk.remove(key_str.as_bytes());
                        self.disk_usage_sub(body.len() as u64);
                        expired = true;
                    }
                }
            }
            DiskValue::File { created_at, ttl_seconds, file_rel, len, .. } => {
                let expires_at = created_at.saturating_add(*ttl_seconds);
                if now > expires_at {
                    if max_stale > 0 && now <= expires_at.saturating_add(max_stale) {
                        is_stale = true;
                    } else {
                        let _ = disk.remove(key_str.as_bytes());
                        if let Some(abs) = self.disk_absolute_path(file_rel) {
                            let _ = std::fs::remove_file(abs);
                        }
                        self.disk_usage_sub(*len);
                        expired = true;
                    }
                }
            }
        }
        if expired {
            return None;
        }

        match val {
            DiskValue::Inline { status, headers, body, created_at, ttl_seconds } => {
                debug!(
                    "Cache hit (disk/inline{}): {}",
                    if is_stale { "/stale" } else { "" },
                    key_str
                );
                let entry = CacheEntry {
                    status,
                    headers,
                    body: CachedBody::Bytes(Bytes::from(body)),
                    created_at,
                    ttl_seconds,
                };

                if self.memory_max_object_bytes == 0 || entry.memory_size_bytes() <= self.memory_max_object_bytes {
                    self.put_memory_only(key_str.to_string(), entry.clone());
                }

                let mut stats = self.stats.write();
                stats.hits += 1;
                stats.disk_hits += 1;
                Some((entry, is_stale))
            }
            DiskValue::File { status, headers, file_rel, len, created_at, ttl_seconds } => {
                let Some(abs) = self.disk_absolute_path(&file_rel) else {
                    let _ = disk.remove(key_str.as_bytes());
                    self.disk_usage_sub(len);
                    return None;
                };
                if !abs.exists() {
                    let _ = disk.remove(key_str.as_bytes());
                    self.disk_usage_sub(len);
                    return None;
                }
                debug!(
                    "Cache hit (disk/file{}): {}",
                    if is_stale { "/stale" } else { "" },
                    key_str
                );
                let entry = CacheEntry {
                    status,
                    headers,
                    body: CachedBody::File { path: abs, len },
                    created_at,
                    ttl_seconds,
                };
                let mut stats = self.stats.write();
                stats.hits += 1;
                stats.disk_hits += 1;
                Some((entry, is_stale))
            }
        }
    }

    #[allow(dead_code)]
    // 预留：对外读取缓存条目
    pub fn get(&self, key: &CacheKey) -> Option<CacheEntry> {
        let key_str = key.to_string();

        if let Some(entry) = self.get_memory_hit(&key_str) {
            return Some(entry);
        }
        if let Some(entry) = self.get_disk_hit(&key_str) {
            return Some(entry);
        }

        debug!("Cache miss: {}", key_str);
        self.stats.write().misses += 1;
        None
    }

    /// Non-blocking cache lookup for async request paths.
    ///
    /// Memory checks are performed inline; disk I/O (sled + filesystem cleanup) is offloaded to
    /// a blocking thread pool to avoid stalling Tokio worker threads under high concurrency.
    pub async fn get_async(self: &Arc<Self>, key: &CacheKey) -> Option<CacheEntry> {
        let key_str = key.to_string();

        if let Some(entry) = self.get_memory_hit(&key_str) {
            return Some(entry);
        }

        // Skip spawning if disk is disabled.
        if !self.disk_enabled() {
            debug!("Cache miss: {}", key_str);
            self.stats.write().misses += 1;
            return None;
        }

        let cache = self.clone();
        let key_for_disk = key_str.clone();
        let disk_entry = tokio::task::spawn_blocking(move || cache.get_disk_hit(&key_for_disk))
            .await
            .ok()
            .flatten();

        if disk_entry.is_some() {
            return disk_entry;
        }

        debug!("Cache miss: {}", key_str);
        self.stats.write().misses += 1;
        None
    }

    /// Return cache entry even if it is stale, as long as it is within max_stale seconds.
    pub async fn get_async_with_stale(
        self: &Arc<Self>,
        key: &CacheKey,
        max_stale: u64,
    ) -> Option<(CacheEntry, bool)> {
        let key_str = key.to_string();

        if let Some(entry) = self.get_memory_hit_with_stale(&key_str, max_stale) {
            return Some(entry);
        }

        if !self.disk_enabled() {
            return None;
        }

        let cache = self.clone();
        let key_for_disk = key_str.clone();
        let disk_entry = tokio::task::spawn_blocking(move || {
            cache.get_disk_hit_with_stale(&key_for_disk, max_stale)
        })
        .await
        .ok()
        .flatten();

        disk_entry
    }

    #[allow(dead_code)]
    // 预留：对外写入缓存条目
    pub fn put(&self, key: CacheKey, entry: CacheEntry) -> Result<()> {
        let key_str = key.to_string();
        let body_bytes = match &entry.body {
            CachedBody::Bytes(b) => b.clone(),
            CachedBody::File { .. } => {
                return Err(anyhow::anyhow!("Cache::put only supports in-memory bodies"));
            }
        };

        // Store in memory (capture replacement/eviction so we can keep size_bytes accurate).
        {
            let mut mem = self.memory.write();
            if let Some((old_key, old_entry)) = mem.push(key_str.clone(), entry.clone()) {
                let mut stats = self.stats.write();
                stats.size_bytes = stats.size_bytes.saturating_sub(old_entry.memory_size_bytes());
                if old_key != key_str {
                    stats.evictions += 1;
                }
            }

            {
                let mut stats = self.stats.write();
                stats.size_bytes = stats.size_bytes.saturating_add(entry.memory_size_bytes());
            }

            // Optional safety valve: evict LRU entries until within the byte cap.
            if self.memory_max_bytes > 0 {
                loop {
                    let over = { self.stats.read().size_bytes > self.memory_max_bytes };
                    if !over {
                        break;
                    }
                    if let Some((_k, evicted)) = mem.pop_lru() {
                        let mut stats = self.stats.write();
                        stats.size_bytes = stats.size_bytes.saturating_sub(evicted.memory_size_bytes());
                        stats.evictions += 1;
                    } else {
                        break;
                    }
                }
            }
        }

        // Store in disk if available.
        self.put_disk_inline(
            key_str.clone(),
            entry.status,
            entry.headers.clone(),
            body_bytes,
            entry.created_at,
            entry.ttl_seconds,
        )?;

        Ok(())
    }

    pub fn remove(&self, key: &CacheKey) -> Result<bool> {
        let key_str = key.to_string();
        let mut removed = false;

        // Remove from memory
        if let Some(entry) = self.memory.write().pop(&key_str) {
            let mut stats = self.stats.write();
            stats.size_bytes = stats.size_bytes.saturating_sub(entry.memory_size_bytes());
            removed = true;
        }

        // Remove from disk
        if let Some(ref disk) = *self.disk.read() {
            if let Some(data) = disk.remove(key_str.as_bytes())? {
                if let Ok(dv) = bincode::deserialize::<DiskValue>(&data) {
                    match dv {
                        DiskValue::Inline { body, .. } => self.disk_usage_sub(body.len() as u64),
                        DiskValue::File { file_rel, len, .. } => {
                            if let Some(abs) = self.disk_absolute_path(&file_rel) {
                                let _ = std::fs::remove_file(abs);
                            }
                            self.disk_usage_sub(len);
                        }
                    }
                }
                removed = true;
            }
        }

        Ok(removed)
    }

    #[allow(dead_code)]
    pub fn purge_by_url(&self, url: &str) -> Result<bool> {
        let key = CacheKey::from_url(url)?;
        self.remove(&key)
    }

    #[allow(dead_code)]
    pub fn stats(&self) -> CacheStats {
        self.stats.read().clone()
    }

    #[allow(dead_code)]
    pub fn clear(&self) -> Result<()> {
        self.memory.write().clear();
        self.stats.write().size_bytes = 0;
        if let Some(ref disk) = *self.disk.read() {
            disk.clear()?;
        }
        if let Some(ref objects_dir) = self.disk_objects_dir {
            let _ = std::fs::remove_dir_all(objects_dir);
            let _ = std::fs::create_dir_all(objects_dir);
        }
        self.disk_usage_bytes.store(0, Ordering::Relaxed);
        Ok(())
    }

    /// Flush pending disk writes to stable storage.
    pub fn flush(&self) -> Result<()> {
        if let Some(ref disk) = *self.disk.read() {
            disk.flush()?;
        }
        Ok(())
    }

    pub fn set_disk_over_limit(&self, over: bool) {
        self.disk_over_limit.store(over, Ordering::Relaxed);
    }

    pub fn recreate_disk_cache(&self, disk_path: &Path) -> Result<()> {
        let mut guard = self.disk.write();
        if let Some(db) = guard.take() {
            drop(db);
        }
        if disk_path.exists() {
            std::fs::remove_dir_all(disk_path).context("Failed to remove disk cache dir")?;
        }
        std::fs::create_dir_all(disk_path).context("Failed to create disk cache dir")?;
        let meta_dir = disk_path.join("meta");
        let objects_dir = disk_path.join("objects");
        std::fs::create_dir_all(&meta_dir).context("Failed to create disk cache meta dir")?;
        std::fs::create_dir_all(&objects_dir).context("Failed to create disk cache objects dir")?;
        let db = sled::open(&meta_dir).context("Failed to open disk cache meta db")?;
        *guard = Some(db);
        self.disk_usage_bytes.store(0, Ordering::Relaxed);
        Ok(())
    }
}

#[allow(dead_code)]
// 预留：用于统计目录大小
fn dir_size(path: &Path) -> Result<u64> {
    let meta = std::fs::symlink_metadata(path)?;
    if meta.is_file() {
        return Ok(meta.len());
    }
    if !meta.is_dir() {
        return Ok(0);
    }

    let mut total: u64 = 0;
    for entry in std::fs::read_dir(path)? {
        let entry = entry?;
        total = total.saturating_add(dir_size(&entry.path())?);
    }
    Ok(total)
}
