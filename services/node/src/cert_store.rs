use anyhow::{Context, Result};
use lru::LruCache;
use parking_lot::Mutex;
use rustls::crypto::ring::sign::any_supported_type;
use rustls::pki_types::{CertificateDer, PrivateKeyDer};
use rustls::sign::CertifiedKey;
use sha2::{Digest, Sha256};
use std::io::BufReader;
use std::num::NonZeroUsize;
use std::path::{Path, PathBuf};
use std::sync::Arc;

/// Disk-backed certificate store with a small in-memory LRU cache.
///
/// We store certificate+key PEM on disk to keep node memory stable when serving
/// thousands of sites on 2G-class machines.
pub struct CertStore {
    dir: PathBuf,
    cache: Mutex<LruCache<String, Arc<CertifiedKey>>>,
}

impl CertStore {
    pub fn new(cache_root: &Path, memory_capacity: usize) -> Result<Self> {
        let dir = cache_root.join("certs");
        std::fs::create_dir_all(&dir).context("create certs dir")?;

        let cap = NonZeroUsize::new(memory_capacity.max(1)).unwrap();
        Ok(Self {
            dir,
            cache: Mutex::new(LruCache::new(cap)),
        })
    }

    pub fn has_pem(&self, cert_id: &str) -> bool {
        self.cert_path(cert_id).is_file()
    }

    pub fn put_pem_if_absent(&self, cert_id: &str, pem: &[u8]) -> Result<()> {
        let path = self.cert_path(cert_id);
        if path.is_file() {
            return Ok(());
        }

        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).context("create cert shard dir")?;
        }

        // Best-effort atomic write: temp file in same dir, then rename.
        let tmp = path.with_extension("pem.tmp");
        std::fs::write(&tmp, pem).with_context(|| format!("write tmp cert {}", tmp.display()))?;
        match std::fs::rename(&tmp, &path) {
            Ok(()) => {}
            Err(e) => {
                // If another task won the race, accept it.
                if path.is_file() {
                    let _ = std::fs::remove_file(&tmp);
                } else {
                    return Err(e).with_context(|| format!("rename cert {} -> {}", tmp.display(), path.display()));
                }
            }
        }

        // Invalidate any previously cached key (e.g. if file already existed due to race).
        self.cache.lock().pop(cert_id);
        Ok(())
    }

    pub fn get_certified_key(&self, cert_id: &str) -> Result<Option<Arc<CertifiedKey>>> {
        if let Some(v) = self.cache.lock().get(cert_id).cloned() {
            return Ok(Some(v));
        }

        let path = self.cert_path(cert_id);
        if !path.is_file() {
            return Ok(None);
        }

        let pem = std::fs::read(&path).with_context(|| format!("read cert {}", path.display()))?;
        let key = Self::parse_certified_key(&pem).with_context(|| format!("parse cert_id={}", cert_id))?;

        self.cache
            .lock()
            .put(cert_id.to_string(), key.clone());
        Ok(Some(key))
    }

    fn cert_path(&self, cert_id: &str) -> PathBuf {
        let digest = Sha256::digest(cert_id.as_bytes());
        let hex = hex::encode(digest);
        let shard = &hex[..2];
        self.dir.join(shard).join(format!("{}.pem", hex))
    }

    fn parse_certified_key(pem: &[u8]) -> Result<Arc<CertifiedKey>> {
        let mut cert_reader = BufReader::new(pem);
        let certs: Vec<CertificateDer<'static>> = rustls_pemfile::certs(&mut cert_reader)
            .collect::<Result<Vec<_>, _>>()
            .context("parse certificate chain")?;

        let mut key_reader = BufReader::new(pem);
        let key: PrivateKeyDer<'static> = rustls_pemfile::private_key(&mut key_reader)
            .context("parse private key")?
            .context("no private key found")?;

        let signing_key = any_supported_type(&key).context("unsupported key type")?;
        Ok(Arc::new(CertifiedKey::new(certs, signing_key)))
    }
}

