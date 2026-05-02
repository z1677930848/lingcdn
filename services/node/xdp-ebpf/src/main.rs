#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::xdp_action,
    helpers::bpf_ktime_get_ns,
    macros::{map, xdp},
    maps::{HashMap, LpmTrie, PerCpuArray},
    programs::XdpContext,
};
use aya_log_ebpf::info;
use core::mem;

/// IPv4 黑名单 - 精确 IP 匹配
/// Key: IPv4 地址 (u32, 网络字节序)
/// Value: 过期时间戳 (unix timestamp, 0 表示永久)
#[map]
static BLACKLIST_V4: HashMap<u32, u64> = HashMap::with_max_entries(100_000, 0);

/// IPv4 白名单 - 精确 IP 匹配
/// Key: IPv4 地址 (u32, 网络字节序)
/// Value: 1 表示在白名单中
#[map]
static WHITELIST_V4: HashMap<u32, u8> = HashMap::with_max_entries(10_000, 0);

/// IPv4 CIDR 黑名单 - 使用 LPM Trie 进行前缀匹配
/// Key: (prefix_len, IPv4 地址)
/// Value: 过期时间戳
#[map]
static BLACKLIST_CIDR_V4: LpmTrie<Ipv4Key, u64> = LpmTrie::with_max_entries(10_000, 0);

/// IPv4 CIDR 白名单
#[map]
static WHITELIST_CIDR_V4: LpmTrie<Ipv4Key, u8> = LpmTrie::with_max_entries(1_000, 0);

/// 统计计数器
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// 每 IP 速率限制计数器
/// Key: IPv4 地址 (u32, 网络字节序)
/// Value: RateLimitEntry (包计数和时间窗口)
#[map]
static RATE_LIMIT_V4: HashMap<u32, RateLimitEntry> = HashMap::with_max_entries(100_000, 0);

/// SYN 计数器 (用于 SYN Flood 检测)
/// Key: IPv4 地址
/// Value: SynCountEntry (SYN 包计数和时间窗口)
#[map]
static SYN_COUNT_V4: HashMap<u32, SynCountEntry> = HashMap::with_max_entries(100_000, 0);

/// XDP 配置参数 (从用户态设置)
#[map]
static XDP_CONFIG: PerCpuArray<XdpConfig> = PerCpuArray::with_max_entries(1, 0);

// 统计 key 常量
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_PACKETS_PASSED: u32 = 1;
const STAT_PACKETS_DROPPED_BLACKLIST: u32 = 2;
const STAT_PACKETS_WHITELISTED: u32 = 3;
const STAT_PACKETS_NON_IP: u32 = 4;
const STAT_PACKETS_DROPPED_RATE_LIMIT: u32 = 5;
const STAT_PACKETS_DROPPED_SYN_FLOOD: u32 = 6;
const STAT_PACKETS_DROPPED_INVALID: u32 = 7;
const STAT_SYN_PACKETS: u32 = 8;
const STAT_UDP_PACKETS: u32 = 9;
const STAT_TCP_PACKETS: u32 = 10;
const STAT_ICMP_PACKETS: u32 = 11;

// 协议常量
const IPPROTO_TCP: u8 = 6;
const IPPROTO_UDP: u8 = 17;
const IPPROTO_ICMP: u8 = 1;

// TCP 标志位
const TCP_FLAG_SYN: u8 = 0x02;
const TCP_FLAG_ACK: u8 = 0x10;
const TCP_FLAG_FIN: u8 = 0x01;
const TCP_FLAG_RST: u8 = 0x04;

// 默认配置值
const DEFAULT_RATE_LIMIT_PPS: u64 = 10000;      // 每秒最大包数
const DEFAULT_SYN_LIMIT_PPS: u64 = 100;         // 每秒最大 SYN 包数
const DEFAULT_WINDOW_NS: u64 = 1_000_000_000;   // 1 秒窗口 (纳秒)

/// LPM Trie 的 Key 结构
#[repr(C)]
#[derive(Clone, Copy)]
pub struct Ipv4Key {
    pub prefix_len: u32,
    pub addr: u32,
}

/// 速率限制条目
#[repr(C)]
#[derive(Clone, Copy)]
pub struct RateLimitEntry {
    pub count: u64,           // 当前窗口内的包计数
    pub window_start: u64,    // 窗口开始时间 (纳秒)
}

/// SYN 计数条目
#[repr(C)]
#[derive(Clone, Copy)]
pub struct SynCountEntry {
    pub count: u64,           // 当前窗口内的 SYN 包计数
    pub window_start: u64,    // 窗口开始时间 (纳秒)
}

/// XDP 配置结构
#[repr(C)]
#[derive(Clone, Copy)]
pub struct XdpConfig {
    pub rate_limit_enabled: u8,       // 是否启用速率限制
    pub syn_flood_enabled: u8,        // 是否启用 SYN Flood 防护
    pub rate_limit_pps: u64,          // 每秒最大包数
    pub syn_limit_pps: u64,           // 每秒最大 SYN 包数
    pub window_ns: u64,               // 时间窗口 (纳秒)
}

#[xdp]
pub fn lingcdn_xdp(ctx: XdpContext) -> u32 {
    match try_lingcdn_xdp(ctx) {
        Ok(ret) => ret,
        Err(_) => xdp_action::XDP_PASS,
    }
}

/// 获取配置，如果没有则使用默认值
#[inline(always)]
fn get_config() -> XdpConfig {
    if let Some(config) = unsafe { XDP_CONFIG.get(0) } {
        *config
    } else {
        XdpConfig {
            rate_limit_enabled: 1,
            syn_flood_enabled: 1,
            rate_limit_pps: DEFAULT_RATE_LIMIT_PPS,
            syn_limit_pps: DEFAULT_SYN_LIMIT_PPS,
            window_ns: DEFAULT_WINDOW_NS,
        }
    }
}

#[inline(always)]
fn try_lingcdn_xdp(ctx: XdpContext) -> Result<u32, ()> {
    // 增加总包计数
    inc_stat(STAT_PACKETS_TOTAL);

    let eth_hdr = ptr_at::<EthHdr>(&ctx, 0)?;

    // 只处理 IPv4
    if unsafe { (*eth_hdr).ether_type } != ETH_P_IP.to_be() {
        inc_stat(STAT_PACKETS_NON_IP);
        return Ok(xdp_action::XDP_PASS);
    }

    let ip_hdr = ptr_at::<Ipv4Hdr>(&ctx, ETH_HDR_LEN)?;
    let src_ip = unsafe { (*ip_hdr).src_addr };
    let protocol = unsafe { (*ip_hdr).protocol };
    let ihl = unsafe { (*ip_hdr).version_ihl } & 0x0F;
    let ip_hdr_len = (ihl as usize) * 4;

    // 协议统计
    match protocol {
        IPPROTO_TCP => inc_stat(STAT_TCP_PACKETS),
        IPPROTO_UDP => inc_stat(STAT_UDP_PACKETS),
        IPPROTO_ICMP => inc_stat(STAT_ICMP_PACKETS),
        _ => {}
    }

    // 1. 检查精确白名单 (最高优先级)
    if unsafe { WHITELIST_V4.get(&src_ip).is_some() } {
        inc_stat(STAT_PACKETS_WHITELISTED);
        return Ok(xdp_action::XDP_PASS);
    }

    // 2. 检查 CIDR 白名单
    let key = Ipv4Key {
        prefix_len: 32,
        addr: src_ip,
    };
    if unsafe { WHITELIST_CIDR_V4.get(&key).is_some() } {
        inc_stat(STAT_PACKETS_WHITELISTED);
        return Ok(xdp_action::XDP_PASS);
    }

    // 3. 检查精确黑名单
    if let Some(&expires_at) = unsafe { BLACKLIST_V4.get(&src_ip) } {
        if expires_at == 0 {
            inc_stat(STAT_PACKETS_DROPPED_BLACKLIST);
            return Ok(xdp_action::XDP_DROP);
        }
        inc_stat(STAT_PACKETS_DROPPED_BLACKLIST);
        return Ok(xdp_action::XDP_DROP);
    }

    // 4. 检查 CIDR 黑名单
    if let Some(&expires_at) = unsafe { BLACKLIST_CIDR_V4.get(&key) } {
        if expires_at == 0 {
            inc_stat(STAT_PACKETS_DROPPED_BLACKLIST);
            return Ok(xdp_action::XDP_DROP);
        }
        inc_stat(STAT_PACKETS_DROPPED_BLACKLIST);
        return Ok(xdp_action::XDP_DROP);
    }

    // 获取配置
    let config = get_config();
    let now = unsafe { bpf_ktime_get_ns() };

    // 5. SYN Flood 防护 (仅对 TCP SYN 包)
    if config.syn_flood_enabled != 0 && protocol == IPPROTO_TCP {
        if let Some(action) = check_syn_flood(&ctx, src_ip, ip_hdr_len, now, &config) {
            return Ok(action);
        }
    }

    // 6. 速率限制
    if config.rate_limit_enabled != 0 {
        if let Some(action) = check_rate_limit(src_ip, now, &config) {
            return Ok(action);
        }
    }

    // 7. 协议异常检测
    if protocol == IPPROTO_TCP {
        if let Some(action) = check_tcp_anomaly(&ctx, ip_hdr_len) {
            return Ok(action);
        }
    }

    // 放行
    inc_stat(STAT_PACKETS_PASSED);
    Ok(xdp_action::XDP_PASS)
}

#[inline(always)]
fn inc_stat(key: u32) {
    if let Some(val) = unsafe { STATS.get_ptr_mut(&key) } {
        unsafe { *val += 1 };
    }
}

/// SYN Flood 检测
/// 检查单个 IP 的 SYN 包速率是否超过阈值
#[inline(always)]
fn check_syn_flood(
    ctx: &XdpContext,
    src_ip: u32,
    ip_hdr_len: usize,
    now: u64,
    config: &XdpConfig,
) -> Option<u32> {
    // 解析 TCP 头部
    let tcp_hdr = ptr_at::<TcpHdr>(ctx, ETH_HDR_LEN + ip_hdr_len).ok()?;
    let flags = unsafe { (*tcp_hdr).flags };

    // 只检查 SYN 包 (SYN=1, ACK=0)
    if (flags & TCP_FLAG_SYN) == 0 || (flags & TCP_FLAG_ACK) != 0 {
        return None;
    }

    inc_stat(STAT_SYN_PACKETS);

    // 获取或创建 SYN 计数条目
    let entry = unsafe { SYN_COUNT_V4.get_ptr_mut(&src_ip) };

    if let Some(entry_ptr) = entry {
        let entry = unsafe { &mut *entry_ptr };

        // 检查是否在同一时间窗口内
        if now - entry.window_start < config.window_ns {
            entry.count += 1;

            // 超过阈值则丢弃
            if entry.count > config.syn_limit_pps {
                inc_stat(STAT_PACKETS_DROPPED_SYN_FLOOD);
                return Some(xdp_action::XDP_DROP);
            }
        } else {
            // 新窗口，重置计数
            entry.count = 1;
            entry.window_start = now;
        }
    } else {
        // 新 IP，创建条目
        let new_entry = SynCountEntry {
            count: 1,
            window_start: now,
        };
        let _ = unsafe { SYN_COUNT_V4.insert(&src_ip, &new_entry, 0) };
    }

    None
}

/// 速率限制检测
/// 检查单个 IP 的总包速率是否超过阈值
#[inline(always)]
fn check_rate_limit(src_ip: u32, now: u64, config: &XdpConfig) -> Option<u32> {
    let entry = unsafe { RATE_LIMIT_V4.get_ptr_mut(&src_ip) };

    if let Some(entry_ptr) = entry {
        let entry = unsafe { &mut *entry_ptr };

        // 检查是否在同一时间窗口内
        if now - entry.window_start < config.window_ns {
            entry.count += 1;

            // 超过阈值则丢弃
            if entry.count > config.rate_limit_pps {
                inc_stat(STAT_PACKETS_DROPPED_RATE_LIMIT);
                return Some(xdp_action::XDP_DROP);
            }
        } else {
            // 新窗口，重置计数
            entry.count = 1;
            entry.window_start = now;
        }
    } else {
        // 新 IP，创建条目
        let new_entry = RateLimitEntry {
            count: 1,
            window_start: now,
        };
        let _ = unsafe { RATE_LIMIT_V4.insert(&src_ip, &new_entry, 0) };
    }

    None
}

/// TCP 协议异常检测
/// 检测无效的 TCP 标志组合
#[inline(always)]
fn check_tcp_anomaly(ctx: &XdpContext, ip_hdr_len: usize) -> Option<u32> {
    let tcp_hdr = ptr_at::<TcpHdr>(ctx, ETH_HDR_LEN + ip_hdr_len).ok()?;
    let flags = unsafe { (*tcp_hdr).flags };

    // 检测无效的标志组合
    // 1. SYN+FIN (无效组合)
    if (flags & TCP_FLAG_SYN) != 0 && (flags & TCP_FLAG_FIN) != 0 {
        inc_stat(STAT_PACKETS_DROPPED_INVALID);
        return Some(xdp_action::XDP_DROP);
    }

    // 2. SYN+RST (无效组合)
    if (flags & TCP_FLAG_SYN) != 0 && (flags & TCP_FLAG_RST) != 0 {
        inc_stat(STAT_PACKETS_DROPPED_INVALID);
        return Some(xdp_action::XDP_DROP);
    }

    // 3. FIN+RST (无效组合)
    if (flags & TCP_FLAG_FIN) != 0 && (flags & TCP_FLAG_RST) != 0 {
        inc_stat(STAT_PACKETS_DROPPED_INVALID);
        return Some(xdp_action::XDP_DROP);
    }

    // 4. 无任何标志 (NULL scan)
    if flags == 0 {
        inc_stat(STAT_PACKETS_DROPPED_INVALID);
        return Some(xdp_action::XDP_DROP);
    }

    None
}

#[inline(always)]
fn ptr_at<T>(ctx: &XdpContext, offset: usize) -> Result<*const T, ()> {
    let start = ctx.data();
    let end = ctx.data_end();
    let len = mem::size_of::<T>();

    if start + offset + len > end {
        return Err(());
    }

    Ok((start + offset) as *const T)
}

// 以太网头部
const ETH_HDR_LEN: usize = 14;
const ETH_P_IP: u16 = 0x0800;

#[repr(C)]
struct EthHdr {
    dst_mac: [u8; 6],
    src_mac: [u8; 6],
    ether_type: u16,
}

// IPv4 头部
#[repr(C)]
struct Ipv4Hdr {
    version_ihl: u8,
    tos: u8,
    tot_len: u16,
    id: u16,
    frag_off: u16,
    ttl: u8,
    protocol: u8,
    check: u16,
    src_addr: u32,
    dst_addr: u32,
}

// TCP 头部
#[repr(C)]
struct TcpHdr {
    src_port: u16,
    dst_port: u16,
    seq: u32,
    ack_seq: u32,
    data_off_reserved: u8,  // 高 4 位是数据偏移
    flags: u8,              // TCP 标志位
    window: u16,
    check: u16,
    urg_ptr: u16,
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe { core::hint::unreachable_unchecked() }
}
