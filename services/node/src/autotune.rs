use std::env;
use std::sync::OnceLock;

#[derive(Clone, Copy, Debug)]
pub struct SystemProfile {
    pub total_mem_bytes: u64,
    pub cpu_count: usize,
}

pub fn enabled() -> bool {
    match env::var("NODE_AUTOTUNE") {
        Ok(v) => {
            let v = v.trim().to_ascii_lowercase();
            !matches!(v.as_str(), "0" | "false" | "no" | "off")
        }
        Err(_) => true,
    }
}

pub fn profile() -> SystemProfile {
    static PROFILE: OnceLock<SystemProfile> = OnceLock::new();
    *PROFILE.get_or_init(|| {
        let cpu_count = std::thread::available_parallelism()
            .map(|n| n.get())
            .unwrap_or(1);

        let total_mem_bytes = total_memory_bytes().unwrap_or(0);

        SystemProfile {
            total_mem_bytes,
            cpu_count,
        }
    })
}

fn total_memory_bytes() -> Option<u64> {
    // sysinfo returns memory in KiB.
    let mut sys = sysinfo::System::new();
    sys.refresh_memory();
    Some(sys.total_memory().saturating_mul(1024))
}

pub fn default_memory_cache_max_bytes() -> u64 {
    // Old fixed default (2G-class safe): 512MB.
    const FALLBACK: u64 = 512 * 1024 * 1024;
    if !enabled() {
        return FALLBACK;
    }

    let p = profile();
    if p.total_mem_bytes == 0 {
        return FALLBACK;
    }

    // 25% of total RAM keeps RSS stable while letting bigger machines benefit automatically.
    (p.total_mem_bytes / 4).max(256 * 1024 * 1024)
}

pub fn default_cache_write_inflight_max_bytes() -> u64 {
    // Old fixed default (2G-class safe): 256MB.
    const FALLBACK: u64 = 256 * 1024 * 1024;
    if !enabled() {
        return FALLBACK;
    }

    let p = profile();
    if p.total_mem_bytes == 0 {
        return FALLBACK;
    }

    // 12.5% of total RAM bounds concurrent disk cache fills.
    (p.total_mem_bytes / 8).max(FALLBACK)
}

pub fn default_cache_write_concurrency() -> usize {
    // Old fixed default: 2.
    const FALLBACK: usize = 2;
    if !enabled() {
        return FALLBACK;
    }

    let p = profile();
    let mem_gb = (p.total_mem_bytes / (1024 * 1024 * 1024)).max(1);
    // A conservative curve: 2*log2(mem_gb) -> 2,4,6,8,10,12...
    let mut c = (mem_gb as f64).log2().floor() as usize;
    c = c.saturating_mul(2);
    c = c.clamp(2, 12);
    c = c.min(p.cpu_count.max(2));
    c.max(FALLBACK)
}

pub fn default_tls_cert_cache_capacity() -> usize {
    // Old fixed default: 1024.
    const FALLBACK: usize = 1024;
    if !enabled() {
        return FALLBACK;
    }

    let p = profile();
    if p.total_mem_bytes == 0 {
        return FALLBACK;
    }

    // Scale: 2GB -> 1024, 4GB -> 2048, 8GB -> 4096, ...
    let cap = (p.total_mem_bytes / (2 * 1024 * 1024 * 1024)) as usize * 1024;
    cap.clamp(1024, 65536)
}

pub fn default_cert_prefetch_concurrency() -> usize {
    // Old fixed default: 4.
    const FALLBACK: usize = 4;
    if !enabled() {
        return FALLBACK;
    }

    let p = profile();
    if p.total_mem_bytes == 0 {
        return FALLBACK;
    }

    // Use the same curve as disk cache write concurrency, but don't exceed CPU count.
    default_cache_write_concurrency().min(p.cpu_count.max(2)).max(2)
}

