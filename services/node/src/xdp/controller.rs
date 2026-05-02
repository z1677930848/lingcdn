//! XDP 鐢ㄦ埛鎬佹帶鍒跺櫒
//!
//! 璐熻矗鍔犺浇 eBPF 绋嬪簭銆佺鐞?BPF Maps銆佸悓姝ラ粦鐧藉悕鍗?
use anyhow::{Context, Result};
use std::collections::HashSet;
use std::net::Ipv4Addr;
use std::path::Path;
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{debug, error, info, warn};

#[cfg(target_os = "linux")]
use aya::{
    include_bytes_aligned,
    maps::{HashMap as BpfHashMap, LpmTrie, PerCpuArray},
    programs::{Xdp, XdpFlags},
    Bpf,
};

/// XDP 閰嶇疆缁撴瀯 (涓?eBPF 绋嬪簭涓殑瀹氫箟涓€鑷?
#[repr(C)]
#[derive(Clone, Copy, Debug)]
pub struct XdpConfig {
    pub rate_limit_enabled: u8,
    pub syn_flood_enabled: u8,
    pub rate_limit_pps: u64,
    pub syn_limit_pps: u64,
    pub window_ns: u64,
}

impl Default for XdpConfig {
    fn default() -> Self {
        Self {
            rate_limit_enabled: 1,
            syn_flood_enabled: 1,
            rate_limit_pps: 10000,      // 姣忕 10000 鍖?            syn_limit_pps: 100,         // 姣忕 100 SYN 鍖?            window_ns: 1_000_000_000,   // 1 绉掔獥鍙?        }
    }
}

#[cfg(target_os = "linux")]
unsafe impl aya::Pod for XdpConfig {}

/// XDP 鎺у埗鍣?pub struct XdpController {
    #[cfg(target_os = "linux")]
    bpf: Option<Bpf>,
    interface: String,
    enabled: bool,
    /// 褰撳墠鍔犺浇鐨勯粦鍚嶅崟 IP (鐢ㄤ簬澧為噺鏇存柊)
    blacklist_ips: Arc<RwLock<HashSet<u32>>>,
    /// 褰撳墠鍔犺浇鐨勭櫧鍚嶅崟 IP
    whitelist_ips: Arc<RwLock<HashSet<u32>>>,
    blacklist_cidrs: Arc<RwLock<HashSet<CidrKey>>>,
    whitelist_cidrs: Arc<RwLock<HashSet<CidrKey>>>,
    /// XDP 閰嶇疆
    config: XdpConfig,
}

/// LPM Trie Key 缁撴瀯 (涓?eBPF 绋嬪簭涓殑瀹氫箟涓€鑷?
#[repr(C)]
#[derive(Clone, Copy)]
pub struct Ipv4Key {
    pub prefix_len: u32,
    pub addr: u32,
}

unsafe impl aya::Pod for Ipv4Key {}

/// XDP 缁熻淇℃伅
#[derive(Debug, Default, Clone)]
pub struct XdpStats {
    pub packets_total: u64,
    pub packets_passed: u64,
    pub packets_dropped_blacklist: u64,
    pub packets_whitelisted: u64,
    pub packets_non_ip: u64,
    pub packets_dropped_rate_limit: u64,
    pub packets_dropped_syn_flood: u64,
    pub packets_dropped_invalid: u64,
    pub syn_packets: u64,
    pub udp_packets: u64,
    pub tcp_packets: u64,
    pub icmp_packets: u64,
}

#[derive(Debug, Clone, Copy, Hash, PartialEq, Eq)]
struct CidrKey {
    addr: u32,
    prefix_len: u8,
}

fn ip_to_key(ip: Ipv4Addr) -> u32 {
    u32::from(ip).to_be()
}

fn key_to_ip(key: u32) -> Ipv4Addr {
    Ipv4Addr::from(u32::from_be(key))
}

impl XdpController {
    /// 鍒涘缓鏂扮殑 XDP 鎺у埗鍣?
    pub fn new(interface: &str) -> Self {
        Self {
            #[cfg(target_os = "linux")]
            bpf: None,
            interface: interface.to_string(),
            enabled: false,
            blacklist_ips: Arc::new(RwLock::new(HashSet::new())),
            whitelist_ips: Arc::new(RwLock::new(HashSet::new())),
            blacklist_cidrs: Arc::new(RwLock::new(HashSet::new())),
            whitelist_cidrs: Arc::new(RwLock::new(HashSet::new())),
            config: XdpConfig::default(),
        }
    }

    /// 鍒涘缓甯﹂厤缃殑 XDP 鎺у埗鍣?
    pub fn with_config(interface: &str, config: XdpConfig) -> Self {
        Self {
            #[cfg(target_os = "linux")]
            bpf: None,
            interface: interface.to_string(),
            enabled: false,
            blacklist_ips: Arc::new(RwLock::new(HashSet::new())),
            whitelist_ips: Arc::new(RwLock::new(HashSet::new())),
            blacklist_cidrs: Arc::new(RwLock::new(HashSet::new())),
            whitelist_cidrs: Arc::new(RwLock::new(HashSet::new())),
            config,
        }
    }

    /// 鍔犺浇骞堕檮鍔?XDP 绋嬪簭
    #[cfg(target_os = "linux")]
    pub fn load_and_attach(&mut self) -> Result<()> {
        info!("Loading XDP program on interface: {}", self.interface);

        // 鍔犺浇 eBPF 绋嬪簭
        // 娉ㄦ剰: 闇€瑕佸厛缂栬瘧 xdp-ebpf crate
        let bpf_bytes = include_bytes_aligned!(
            concat!(env!("OUT_DIR"), "/lingcdn-xdp")
        );

        let mut bpf = Bpf::load(bpf_bytes)
            .context("Failed to load eBPF program")?;

        // 鑾峰彇 XDP 绋嬪簭
        let program: &mut Xdp = bpf
            .program_mut("lingcdn_xdp")
            .context("XDP program not found")?
            .try_into()
            .context("Failed to convert to XDP program")?;

        // 鍔犺浇绋嬪簭
        program.load().context("Failed to load XDP program")?;

        // 闄勫姞鍒扮綉鍗?        // 浣跨敤 SKB 妯″紡浠ヨ幏寰楁洿濂界殑鍏煎鎬э紝鐢熶骇鐜鍙互浣跨敤 DRV 妯″紡
        program
            .attach(&self.interface, XdpFlags::SKB_MODE)
            .context("Failed to attach XDP program")?;

        info!("XDP program attached successfully");

        // 鍒濆鍖栫粺璁¤鏁板櫒
        self.init_stats(&mut bpf)?;

        // 鍒濆鍖栭厤缃?        self.init_config(&mut bpf)?;

        self.bpf = Some(bpf);
        self.enabled = true;

        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub fn load_and_attach(&mut self) -> Result<()> {
        warn!("XDP is only supported on Linux, running in no-op mode");
        Ok(())
    }

    /// 鍒濆鍖栫粺璁¤鏁板櫒
    #[cfg(target_os = "linux")]
    fn init_stats(&self, bpf: &mut Bpf) -> Result<()> {
        let mut stats: BpfHashMap<_, u32, u64> = bpf
            .map_mut("STATS")
            .context("STATS map not found")?
            .try_into()
            .context("Failed to convert STATS map")?;

        // 鍒濆鍖栨墍鏈夎鏁板櫒涓?0 (鎵╁睍鍒?12 涓?
        for key in 0..12u32 {
            stats.insert(key, 0u64, 0)?;
        }

        Ok(())
    }

    /// 鍒濆鍖?XDP 閰嶇疆
    #[cfg(target_os = "linux")]
    fn init_config(&self, bpf: &mut Bpf) -> Result<()> {
        let mut config_map: PerCpuArray<_, XdpConfig> = bpf
            .map_mut("XDP_CONFIG")
            .context("XDP_CONFIG map not found")?
            .try_into()
            .context("Failed to convert XDP_CONFIG map")?;

        // 璁剧疆閰嶇疆鍒版墍鏈?CPU
        config_map.set(0, self.config, 0)?;

        info!(
            "XDP config initialized: rate_limit={}, syn_flood={}, rate_limit_pps={}, syn_limit_pps={}",
            self.config.rate_limit_enabled != 0,
            self.config.syn_flood_enabled != 0,
            self.config.rate_limit_pps,
            self.config.syn_limit_pps
        );

        Ok(())
    }

    /// 鏇存柊 XDP 閰嶇疆
    #[cfg(target_os = "linux")]
    pub fn update_config(&mut self, config: XdpConfig) -> Result<()> {
        self.config = config;

        if let Some(ref bpf) = self.bpf {
            let mut config_map: PerCpuArray<_, XdpConfig> = bpf
                .map("XDP_CONFIG")
                .context("XDP_CONFIG map not found")?
                .try_into()
                .context("Failed to convert XDP_CONFIG map")?;

            config_map.set(0, self.config, 0)?;

            info!("XDP config updated");
        }

        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub fn update_config(&mut self, config: XdpConfig) -> Result<()> {
        self.config = config;
        Ok(())
    }

    /// 鑾峰彇褰撳墠閰嶇疆
    pub fn get_config(&self) -> &XdpConfig {
        &self.config
    }

    /// 娣诲姞 IP 鍒伴粦鍚嶅崟
    #[cfg(target_os = "linux")]
    pub async fn add_blacklist_ip(&self, ip: Ipv4Addr, expires_at: u64) -> Result<()> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;
        let ip_u32 = ip_to_key(ip);

        let mut blacklist: BpfHashMap<_, u32, u64> = bpf
            .map("BLACKLIST_V4")
            .context("BLACKLIST_V4 map not found")?
            .try_into()
            .context("Failed to convert map")?;

        blacklist.insert(ip_u32, expires_at, 0)?;

        let mut ips = self.blacklist_ips.write().await;
        ips.insert(ip_u32);

        debug!("Added {} to XDP blacklist (expires_at={})", ip, expires_at);
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub async fn add_blacklist_ip(&self, ip: Ipv4Addr, expires_at: u64) -> Result<()> {
        debug!("XDP not available, skipping blacklist add for {}", ip);
        Ok(())
    }

    /// 浠庨粦鍚嶅崟绉婚櫎 IP
    #[cfg(target_os = "linux")]
    pub async fn remove_blacklist_ip(&self, ip: Ipv4Addr) -> Result<()> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;
        let ip_u32 = ip_to_key(ip);

        let mut blacklist: BpfHashMap<_, u32, u64> = bpf
            .map("BLACKLIST_V4")
            .context("BLACKLIST_V4 map not found")?
            .try_into()
            .context("Failed to convert map")?;

        let _ = blacklist.remove(&ip_u32);

        let mut ips = self.blacklist_ips.write().await;
        ips.remove(&ip_u32);

        debug!("Removed {} from XDP blacklist", ip);
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub async fn remove_blacklist_ip(&self, ip: Ipv4Addr) -> Result<()> {
        Ok(())
    }

    /// 娣诲姞 IP 鍒扮櫧鍚嶅崟
    #[cfg(target_os = "linux")]
    pub async fn add_whitelist_ip(&self, ip: Ipv4Addr) -> Result<()> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;
        let ip_u32 = ip_to_key(ip);

        let mut whitelist: BpfHashMap<_, u32, u8> = bpf
            .map("WHITELIST_V4")
            .context("WHITELIST_V4 map not found")?
            .try_into()
            .context("Failed to convert map")?;

        whitelist.insert(ip_u32, 1u8, 0)?;

        let mut ips = self.whitelist_ips.write().await;
        ips.insert(ip_u32);

        debug!("Added {} to XDP whitelist", ip);
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub async fn add_whitelist_ip(&self, ip: Ipv4Addr) -> Result<()> {
        Ok(())
    }

    /// 浠庣櫧鍚嶅崟绉婚櫎 IP
    #[cfg(target_os = "linux")]
    pub async fn remove_whitelist_ip(&self, ip: Ipv4Addr) -> Result<()> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;
        let ip_u32 = ip_to_key(ip);

        let mut whitelist: BpfHashMap<_, u32, u8> = bpf
            .map("WHITELIST_V4")
            .context("WHITELIST_V4 map not found")?
            .try_into()
            .context("Failed to convert map")?;

        let _ = whitelist.remove(&ip_u32);

        let mut ips = self.whitelist_ips.write().await;
        ips.remove(&ip_u32);

        debug!("Removed {} from XDP whitelist", ip);
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub async fn remove_whitelist_ip(&self, ip: Ipv4Addr) -> Result<()> {
        Ok(())
    }

    /// 娣诲姞 CIDR 鍒伴粦鍚嶅崟
    #[cfg(target_os = "linux")]
    pub async fn add_blacklist_cidr(&self, ip: Ipv4Addr, prefix_len: u8, expires_at: u64) -> Result<()> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;

        let key = Ipv4Key {
            prefix_len: prefix_len as u32,
            addr: ip_to_key(ip),
        };

        let mut blacklist: LpmTrie<_, Ipv4Key, u64> = bpf
            .map("BLACKLIST_CIDR_V4")
            .context("BLACKLIST_CIDR_V4 map not found")?
            .try_into()
            .context("Failed to convert map")?;

        blacklist.insert(&key, expires_at, 0)?;

        let mut cidrs = self.blacklist_cidrs.write().await;
        cidrs.insert(CidrKey {
            addr: key.addr,
            prefix_len,
        });

        debug!("Added {}/{} to XDP CIDR blacklist", ip, prefix_len);
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub async fn add_blacklist_cidr(&self, ip: Ipv4Addr, prefix_len: u8, expires_at: u64) -> Result<()> {
        Ok(())
    }

    #[cfg(target_os = "linux")]
    pub async fn remove_blacklist_cidr(&self, ip: Ipv4Addr, prefix_len: u8) -> Result<()> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;

        let key = Ipv4Key {
            prefix_len: prefix_len as u32,
            addr: ip_to_key(ip),
        };

        let mut blacklist: LpmTrie<_, Ipv4Key, u64> = bpf
            .map("BLACKLIST_CIDR_V4")
            .context("BLACKLIST_CIDR_V4 map not found")?
            .try_into()
            .context("Failed to convert map")?;

        let _ = blacklist.remove(&key);

        let mut cidrs = self.blacklist_cidrs.write().await;
        cidrs.remove(&CidrKey {
            addr: key.addr,
            prefix_len,
        });

        debug!("Removed {}/{} from XDP CIDR blacklist", ip, prefix_len);
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub async fn remove_blacklist_cidr(&self, ip: Ipv4Addr, prefix_len: u8) -> Result<()> {
        Ok(())
    }

    /// 娣诲姞 CIDR 鍒扮櫧鍚嶅崟
    #[cfg(target_os = "linux")]
    pub async fn add_whitelist_cidr(&self, ip: Ipv4Addr, prefix_len: u8) -> Result<()> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;

        let key = Ipv4Key {
            prefix_len: prefix_len as u32,
            addr: ip_to_key(ip),
        };

        let mut whitelist: LpmTrie<_, Ipv4Key, u8> = bpf
            .map("WHITELIST_CIDR_V4")
            .context("WHITELIST_CIDR_V4 map not found")?
            .try_into()
            .context("Failed to convert map")?;

        whitelist.insert(&key, 1u8, 0)?;

        let mut cidrs = self.whitelist_cidrs.write().await;
        cidrs.insert(CidrKey {
            addr: key.addr,
            prefix_len,
        });

        debug!("Added {}/{} to XDP CIDR whitelist", ip, prefix_len);
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub async fn add_whitelist_cidr(&self, ip: Ipv4Addr, prefix_len: u8) -> Result<()> {
        Ok(())
    }

    #[cfg(target_os = "linux")]
    pub async fn remove_whitelist_cidr(&self, ip: Ipv4Addr, prefix_len: u8) -> Result<()> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;

        let key = Ipv4Key {
            prefix_len: prefix_len as u32,
            addr: ip_to_key(ip),
        };

        let mut whitelist: LpmTrie<_, Ipv4Key, u8> = bpf
            .map("WHITELIST_CIDR_V4")
            .context("WHITELIST_CIDR_V4 map not found")?
            .try_into()
            .context("Failed to convert map")?;

        let _ = whitelist.remove(&key);

        let mut cidrs = self.whitelist_cidrs.write().await;
        cidrs.remove(&CidrKey {
            addr: key.addr,
            prefix_len,
        });

        debug!("Removed {}/{} from XDP CIDR whitelist", ip, prefix_len);
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub async fn remove_whitelist_cidr(&self, ip: Ipv4Addr, prefix_len: u8) -> Result<()> {
        Ok(())
    }

    /// 鑾峰彇缁熻淇℃伅
    #[cfg(target_os = "linux")]
    pub fn get_stats(&self) -> Result<XdpStats> {
        let bpf = self.bpf.as_ref().context("XDP not loaded")?;

        let stats: BpfHashMap<_, u32, u64> = bpf
            .map("STATS")
            .context("STATS map not found")?
            .try_into()
            .context("Failed to convert map")?;

        let get_stat = |key: u32| -> u64 {
            stats.get(&key, 0).unwrap_or(0)
        };

        Ok(XdpStats {
            packets_total: get_stat(0),
            packets_passed: get_stat(1),
            packets_dropped_blacklist: get_stat(2),
            packets_whitelisted: get_stat(3),
            packets_non_ip: get_stat(4),
            packets_dropped_rate_limit: get_stat(5),
            packets_dropped_syn_flood: get_stat(6),
            packets_dropped_invalid: get_stat(7),
            syn_packets: get_stat(8),
            udp_packets: get_stat(9),
            tcp_packets: get_stat(10),
            icmp_packets: get_stat(11),
        })
    }

    #[cfg(not(target_os = "linux"))]
    pub fn get_stats(&self) -> Result<XdpStats> {
        Ok(XdpStats::default())
    }

    /// 鍚屾榛戝悕鍗?(浠庨厤缃洿鏂?
    pub async fn sync_blacklist(&self, bans: &[crate::config::WAFBan]) -> Result<()> {
        if !self.enabled {
            return Ok(());
        }

        let mut desired_ips: HashSet<u32> = HashSet::new();
        let mut desired_cidrs: HashSet<CidrKey> = HashSet::new();
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_secs() as i64)
            .unwrap_or(0);

        for ban in bans {
            // 璺宠繃宸茶繃鏈熺殑
            if ban.expires_at > 0 && ban.expires_at < now {
                continue;
            }

            // 瑙ｆ瀽 IP
            if let Ok(ip) = ban.ip.parse::<Ipv4Addr>() {
                let expires = if ban.expires_at <= 0 { 0 } else { ban.expires_at as u64 };
                desired_ips.insert(ip_to_key(ip));
                if let Err(e) = self.add_blacklist_ip(ip, expires).await {
                    warn!("Failed to add {} to XDP blacklist: {}", ban.ip, e);
                }
            } else if ban.ip.contains('/') {
                // CIDR 鏍煎紡
                if let Some((ip_str, prefix_str)) = ban.ip.split_once('/') {
                    if let (Ok(ip), Ok(prefix)) = (ip_str.parse::<Ipv4Addr>(), prefix_str.parse::<u8>()) {
                        let expires = if ban.expires_at <= 0 { 0 } else { ban.expires_at as u64 };
                        desired_cidrs.insert(CidrKey {
                            addr: ip_to_key(ip),
                            prefix_len: prefix,
                        });
                        if let Err(e) = self.add_blacklist_cidr(ip, prefix, expires).await {
                            warn!("Failed to add {}/{} to XDP blacklist: {}", ip, prefix, e);
                        }
                    }
                }
            }
        }

        let current_ips: Vec<u32> = self.blacklist_ips.read().await.iter().copied().collect();
        for ip in current_ips {
            if !desired_ips.contains(&ip) {
                let ip_addr = key_to_ip(ip);
                if let Err(e) = self.remove_blacklist_ip(ip_addr).await {
                    warn!("Failed to remove {} from XDP blacklist: {}", ip_addr, e);
                }
            }
        }

        let current_cidrs: Vec<CidrKey> = self.blacklist_cidrs.read().await.iter().copied().collect();
        for cidr in current_cidrs {
            if !desired_cidrs.contains(&cidr) {
                let ip_addr = key_to_ip(cidr.addr);
                if let Err(e) = self.remove_blacklist_cidr(ip_addr, cidr.prefix_len).await {
                    warn!(
                        "Failed to remove {}/{} from XDP CIDR blacklist: {}",
                        ip_addr,
                        cidr.prefix_len,
                        e
                    );
                }
            }
        }

        info!("Synced {} bans to XDP blacklist", bans.len());
        Ok(())
    }

    /// 鍚屾鐧藉悕鍗?(浠庨厤缃洿鏂?
    pub async fn sync_whitelist(&self, whitelist: &[crate::config::WAFWhitelist]) -> Result<()> {
        if !self.enabled {
            return Ok(());
        }

        let mut desired_ips: HashSet<u32> = HashSet::new();
        let mut desired_cidrs: HashSet<CidrKey> = HashSet::new();

        for entry in whitelist {
            if let Ok(ip) = entry.ip.parse::<Ipv4Addr>() {
                desired_ips.insert(ip_to_key(ip));
                if let Err(e) = self.add_whitelist_ip(ip).await {
                    warn!("Failed to add {} to XDP whitelist: {}", entry.ip, e);
                }
            } else if entry.ip.contains('/') {
                // CIDR 鏍煎紡
                if let Some((ip_str, prefix_str)) = entry.ip.split_once('/') {
                    if let (Ok(ip), Ok(prefix)) = (ip_str.parse::<Ipv4Addr>(), prefix_str.parse::<u8>()) {
                        desired_cidrs.insert(CidrKey {
                            addr: ip_to_key(ip),
                            prefix_len: prefix,
                        });
                        if let Err(e) = self.add_whitelist_cidr(ip, prefix).await {
                            warn!("Failed to add {}/{} to XDP whitelist: {}", ip, prefix, e);
                        }
                    }
                }
            }
        }

        let current_ips: Vec<u32> = self.whitelist_ips.read().await.iter().copied().collect();
        for ip in current_ips {
            if !desired_ips.contains(&ip) {
                let ip_addr = key_to_ip(ip);
                if let Err(e) = self.remove_whitelist_ip(ip_addr).await {
                    warn!("Failed to remove {} from XDP whitelist: {}", ip_addr, e);
                }
            }
        }

        let current_cidrs: Vec<CidrKey> = self.whitelist_cidrs.read().await.iter().copied().collect();
        for cidr in current_cidrs {
            if !desired_cidrs.contains(&cidr) {
                let ip_addr = key_to_ip(cidr.addr);
                if let Err(e) = self.remove_whitelist_cidr(ip_addr, cidr.prefix_len).await {
                    warn!(
                        "Failed to remove {}/{} from XDP CIDR whitelist: {}",
                        ip_addr,
                        cidr.prefix_len,
                        e
                    );
                }
            }
        }

        info!("Synced {} entries to XDP whitelist", whitelist.len());
        Ok(())
    }

    /// 妫€鏌?XDP 鏄惁鍚敤
    pub fn is_enabled(&self) -> bool {
        self.enabled
    }

    pub fn interface(&self) -> &str {
        &self.interface
    }

    /// 鍗歌浇 XDP 绋嬪簭
    #[cfg(target_os = "linux")]
    pub fn detach(&mut self) -> Result<()> {
        if let Some(mut bpf) = self.bpf.take() {
            if let Ok(program) = bpf.program_mut("lingcdn_xdp") {
                if let Ok(xdp) = TryInto::<&mut Xdp>::try_into(program) {
                    // XDP 绋嬪簭浼氬湪 drop 鏃惰嚜鍔ㄥ嵏杞?                    info!("XDP program will be detached");
                }
            }
        }
        self.enabled = false;
        Ok(())
    }

    #[cfg(not(target_os = "linux"))]
    pub fn detach(&mut self) -> Result<()> {
        Ok(())
    }
}

impl Drop for XdpController {
    fn drop(&mut self) {
        if self.enabled {
            if let Err(e) = self.detach() {
                error!("Failed to detach XDP program: {}", e);
            }
        }
    }
}

/// 瑙ｆ瀽 CIDR 瀛楃涓?
    pub fn parse_cidr(cidr: &str) -> Option<(Ipv4Addr, u8)> {
    if let Some((ip_str, prefix_str)) = cidr.split_once('/') {
        if let (Ok(ip), Ok(prefix)) = (ip_str.parse::<Ipv4Addr>(), prefix_str.parse::<u8>()) {
            if prefix <= 32 {
                return Some((ip, prefix));
            }
        }
    }
    None
}



