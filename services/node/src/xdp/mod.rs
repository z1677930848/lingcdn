//! XDP (eXpress Data Path) module.
//!
//! The real implementation is Linux-only. On non-Linux platforms we provide a
//! small stub so the node can still build and run.

#[cfg(all(target_os = "linux", feature = "xdp"))]
mod controller;

#[cfg(all(target_os = "linux", feature = "xdp"))]
pub use controller::{XdpConfig, XdpController, XdpStats};

#[cfg(not(all(target_os = "linux", feature = "xdp")))]
#[derive(Debug, Default)]
#[allow(dead_code)]
pub struct XdpController;

#[cfg(not(all(target_os = "linux", feature = "xdp")))]
#[derive(Debug, Clone, Copy, Default)]
#[allow(dead_code)]
pub struct XdpConfig {
    pub rate_limit_enabled: u8,
    pub syn_flood_enabled: u8,
    pub rate_limit_pps: u64,
    pub syn_limit_pps: u64,
    pub window_ns: u64,
}

#[cfg(not(all(target_os = "linux", feature = "xdp")))]
#[derive(Debug, Clone, Copy, Default)]
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

#[cfg(not(all(target_os = "linux", feature = "xdp")))]
#[allow(dead_code)]
impl XdpController {
    pub fn with_config(_iface: &str, _cfg: XdpConfig) -> Self {
        Self
    }

    pub fn load_and_attach(&mut self) -> anyhow::Result<()> {
        Ok(())
    }

    pub async fn sync_blacklist(&mut self, _bans: &[crate::config::WAFBan]) -> anyhow::Result<()> {
        Ok(())
    }

    pub async fn sync_whitelist(&mut self, _wl: &[crate::config::WAFWhitelist]) -> anyhow::Result<()> {
        Ok(())
    }

    pub fn is_enabled(&self) -> bool {
        false
    }

    pub fn interface(&self) -> &str {
        ""
    }

    pub fn get_stats(&self) -> anyhow::Result<XdpStats> {
        Ok(XdpStats::default())
    }

    pub async fn add_blacklist_ip(&self, _ip: std::net::Ipv4Addr, _expires_at_unix: u64) -> anyhow::Result<()> {
        Ok(())
    }
}
