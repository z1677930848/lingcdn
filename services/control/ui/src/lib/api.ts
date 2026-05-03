// API Client for LingCDN Portal Backend

const rawBase = import.meta.env.VITE_API_BASE || ""
const API_BASE = (/^auto$/i.test(rawBase) ? "" : rawBase).replace(/\/+$/, "")

function normalizeHeaders(input: HeadersInit | undefined): Record<string, string> {
  if (!input) {
    return {}
  }
  if (input instanceof Headers) {
    const record: Record<string, string> = {}
    input.forEach((value, key) => {
      record[key] = value
    })
    return record
  }
  if (Array.isArray(input)) {
    return Object.fromEntries(input.map(([key, value]) => [key, String(value)]))
  }
  const record: Record<string, string> = {}
  for (const [key, value] of Object.entries(input)) {
    if (typeof value !== "undefined") {
      record[key] = String(value)
    }
  }
  return record
}

export interface User {
  id: string
  numeric_id?: number
  username: string
  email: string
  role: string
  status?: string
  created_at?: string
  last_login_at?: string
  last_login_ip?: string
  last_login_location?: string
}

export interface AuthResponse {
  token: string
  user: User
}

export const AUTH_TOKEN_KEY = 'lingcdn_portal_token'

export const FEATURE_UNAVAILABLE_MESSAGE = "后端暂未开放该能力"

export class FeatureUnavailableError extends Error {
  feature: string

  constructor(feature: string, message: string = FEATURE_UNAVAILABLE_MESSAGE) {
    super(message)
    this.name = "FeatureUnavailableError"
    this.feature = feature
  }
}

export interface CaptchaChallenge {
  question: string
  token: string
  expires_in: number
}

export function setAuthToken(token: string) {
  try {
    localStorage.setItem(AUTH_TOKEN_KEY, token)
  } catch {
    /* ignore */
  }
  // Also set as cookie so EventSource (SSE) can authenticate
  try {
    document.cookie = `lingcdn_token=${encodeURIComponent(token)}; path=/; SameSite=Strict`
  } catch {
    /* ignore */
  }
}

export function getAuthToken(): string | null {
  try {
    const token = localStorage.getItem(AUTH_TOKEN_KEY)
    // Sync cookie for SSE if localStorage has token but cookie is missing
    if (token && typeof document !== 'undefined' && !getCookieValue('lingcdn_token')) {
      document.cookie = `lingcdn_token=${encodeURIComponent(token)}; path=/; SameSite=Strict`
    }
    return token
  } catch {
    return null
  }
}

export function clearAuthToken() {
  try {
    localStorage.removeItem(AUTH_TOKEN_KEY)
  } catch {
    /* ignore */
  }
  // Also clear the cookie
  try {
    document.cookie = 'lingcdn_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT; SameSite=Strict'
  } catch {
    /* ignore */
  }
}

function getCookieValue(name: string): string {
  if (typeof document === "undefined") return ""
  const parts = document.cookie.split(";")
  for (const raw of parts) {
    const [k, ...rest] = raw.trim().split("=")
    if (k === name) return decodeURIComponent(rest.join("="))
  }
  return ""
}

function resolveCSRFToken(): string {
  return getCookieValue("admin_csrf") || getCookieValue("portal_csrf")
}

function isSameOrigin(baseURL: string): boolean {
  if (typeof window === "undefined") return true
  if (!baseURL) return true
  if (baseURL.startsWith("/")) return true
  try {
    return new URL(baseURL, window.location.origin).origin === window.location.origin
  } catch {
    return false
  }
}

export interface Product {
  id: string
  name: string
  slug: string
  description: string
  group_id?: string
  sort?: number
  region?: string
  line_group_id?: string
  cluster_id?: string

  monthly_traffic_bytes?: number | null
  bandwidth_bps?: number | null
  conn_limit?: number | null

  domain_limit?: number | null
  primary_domain_limit?: number | null
  http_port_limit?: number | null
  stream_port_limit?: number | null
  non_std_port_limit?: number | null

  websocket?: boolean
  custom_cc_rules?: boolean
  http3?: boolean
  l2_origin?: boolean
  cc_protection?: string
  ddos_protection?: string

  price_cents?: number
  price_month_cents?: number
  price_quarter_cents?: number
  price_year_cents?: number
  currency?: string
  enabled?: boolean
  created_at: string
  updated_at: string
}

export interface ProductGroup {
  id: string
  name: string
  sort: number
  description: string
  created_at: string
  updated_at: string
}

export interface Order {
  id: string
  user_id: string
  product_id: string
  product_name: string
  amount_cents: number
  currency: string
  status: string
  period?: string
  quantity?: number
  starts_at?: string
  ends_at?: string
  paid_at?: string
  note?: string
  created_at: string
  updated_at: string
}

export interface UpgradeNode {
  id: string
  hostname: string
  current_version: string
  target_version: string
  status: string
}

export interface Node {
  id: string
  hostname: string
  public_ip?: string
  version?: string
  status?: string
  region?: string
  cluster?: string
  geo?: GeoLocation
  capabilities?: string[]
  config_version?: string
  token?: string
  last_heartbeat?: string
  monitor_enabled?: boolean
  monitor_protocol?: string
  monitor_timeout_seconds?: number
  monitor_port?: number
  monitor_fail_threshold?: number
  monitor_fail_count?: number
  monitor_last_ok?: boolean
  monitor_last_error?: string
  monitor_last_at?: string
  monitor_last_latency_ms?: number
  cpu_usage?: number
  mem_usage?: number
  disk_usage?: number
  cpu_count?: number
  mem_total?: number
  disk_total?: number
  last_metrics_at?: string
  bytes_sent?: number
  bytes_received?: number
  bandwidth_up_bps?: number
  bandwidth_down_bps?: number
  tcp_established?: number
  tcp_syn_recv?: number
  tcp_time_wait?: number
  nginx_running?: boolean
  month_bytes_sent?: number
  comm_ok?: boolean
  report_ok?: boolean
  created_at?: string
  updated_at?: string
}

export interface NodeInstallCommandResponse {
  command: string
  master_host: string
  master_token: string
  master_version: string
  expires_at: string
  portal_base: string
  script_url: string
  style: string
}

export interface NodeInstallSSHResult {
  ok: boolean
  status: "installed" | "installed_waiting_register" | "failed"
  message: string
  logs: string[]
  remote_hostname?: string
  master_host: string
  expires_at: string
  node?: {
    id: string
    hostname: string
    public_ip?: string
    status?: string
  }
}

export interface NodeMonitorRankEntry {
  rank: number
  node_id: string
  hostname: string
  iface: string
  out_bps: number
  in_bps: number
  connections: number
  cpu_usage: number
  mem_usage: number
  disk_usage: number
  delta_bytes_sent: number
  delta_bytes_recv: number
}

export interface NodeMonitorRankResponse {
  group: string
  window_seconds: number
  nodes: NodeMonitorRankEntry[]
}

export interface NodeMonitorSeriesPoint {
  ts: number
  value: number
}

export interface NodeMonitorSeriesResponse {
  metric: string
  window_seconds: number
  step_seconds: number
  points: NodeMonitorSeriesPoint[]
}

export interface GeoLocation {
  name?: string
  country?: string
  country_code?: string
  region?: string
  city?: string
  lat?: number
  lon?: number
  timezone?: string
  is_private?: boolean
}

export interface UpgradeInfo {
  current_version: string
  latest_version: string
  channel: string
  checksum?: string
  download_url?: string
  changelog?: string
  signature?: string
  sig_alg?: string
  sig_target?: string
  pubkey?: string
  node_latest_version?: string
  node_latest_amd64?: string
  node_latest_arm64?: string
  nodes: UpgradeNode[]
  notes?: string[]
  checked_at?: string
  source?: string
}

export interface UpgradeTask {
  id: string
  target_version: string
  channel?: string
  node_ids: string[]
  status: string
  type: string
  created_at: string
}

export interface UpgradeLog {
  ts: string
  level: string
  message: string
  node_id?: string
}

export interface SystemLicenseState {
  status: string
  license_key: string
  expires_at: string
  max_nodes: number
  last_checked: string
  grace_until: string
  reason: string
  updated_at: string
  pubkey?: string
}

export interface SystemLicenseStatusResponse {
  license: SystemLicenseState
  active_nodes: number
  now: string
  mode?: string
  control_id?: string
  license_ip?: string
}

export interface SystemInfo {
  started_at: string
  uptime_seconds: number
  build_time: string
  build_hash: string
  debug_mode: boolean
  db_debug_mode: boolean
  app_version: string
  go_version: string
}

export interface PublicLicenseStatusResponse {
  status: string
  reason?: string
  mode?: string
}

export interface OverviewSummary {
  registered_users: number
  total_nodes: number
  online_nodes: number
  domains: number
  certificates: number
}

export interface OverviewStatus {
  name: string
  status: string
  detail?: string
}

export interface OverviewNetworkRegion {
  name: string
  nodes: number
  latency_ms: number
}

export interface OverviewNetwork {
  total_nodes: number
  online_nodes: number
  connected_nodes: number
  offline_nodes: number
  regions: OverviewNetworkRegion[]
}

export interface OverviewTrendPoint {
  timestamp: string
  value: number
}

export interface OverviewTrendSeries {
  key: string
  name: string
  unit: string
  points: OverviewTrendPoint[]
}

export interface OverviewTopDomain {
  domain: string
  url: string
  requests: number
  bandwidth_mbps: number
  cache_hit_rate?: number
  origin_error_pct?: number
}

export interface OverviewLicense {
  authorized_nodes: number
  active_nodes: number
  expires_at: string
  status?: string
  reason?: string
}

export interface OverviewUsage {
  domains: number
  certificates: number
  cache_rules: number
  config_versions: number
}

export interface OverviewTrafficRegion {
  name: string
  region: string
  bytes_sent: number
  requests: number
}

export interface OverviewResponse {
  summary: OverviewSummary
  system_status: OverviewStatus[]
  network: OverviewNetwork
  trends: OverviewTrendSeries[]
  top_domains: OverviewTopDomain[]
  license: OverviewLicense
  usage: OverviewUsage
  traffic_map: OverviewTrafficRegion[]
}

export interface BalanceAccount {
  user_id: string
  balance_cents: number
  updated_at: string
}

export interface BalanceTransaction {
  id: string
  user_id: string
  type: string
  amount_cents: number
  balance_cents: number
  ref_type: string
  ref_id: string
  note: string
  created_at: string
}

export interface BalanceRecharge {
  id: string
  user_id: string
  amount_cents: number
  currency: string
  status: string
  payment_provider: string
  payment_method: string
  payment_url?: string
  qr_code?: string
  notify_raw?: string
  expires_at?: string
  closed_at?: string
  out_trade_no: string
  trade_no: string
  paid_at?: string
  created_at: string
  updated_at: string
}

export interface BalanceWithdrawal {
  id: string
  user_id: string
  amount_cents: number
  currency: string
  status: string
  method: string
  account_name: string
  account_no: string
  note: string
  reviewed_by: string
  reviewed_at?: string
  created_at: string
  updated_at: string
}

export interface BalanceStatsDay {
  day: string
  recharge_cents: number
  recharge_count: number
  adjust_cents: number
  adjust_count: number
  total_cents: number
  total_count: number
  paid_cents?: number
  paid_count?: number
  pending_count?: number
}

export interface Announcement {
  id: string
  title: string
  content: string
  status: string
  pinned: boolean
  created_at: string
  updated_at: string
}

export interface SystemLog {
  id: string
  type: string
  status: string
  message: string
  user_id?: string
  username?: string
  ip?: string
  location?: string
  created_at?: string
}

export interface APIToken {
  id: string
  description: string
  token_prefix: string
  expires_at?: string
  created_at: string
}

export interface DomainBlacklist {
  id: string
  domain: string
  reason: string
  created_at: string
  updated_at: string
}

export interface Cluster {
  id: string
  name: string
  dns_zone: string
  dns_mode: string
  cname?: string
  description?: string
  enabled: boolean
  created_at?: string
  updated_at?: string
}

export interface DNSConfig {
  provider: string
  account_id: string
  token: string
  secret: string
  ttl: number
  enable_ip_weight: boolean
  last_error?: string
  updated_at?: string
}

export interface DNSProviderOption {
  value: string
  label: string
}

export interface DNSTask {
  id: string
  type: string
  provider: string
  status: string
  message: string
  created_at: string
  updated_at: string
}

export interface ErrorPage {
  status: number
  mode: string
  content: string
}

export interface Domain {
  id: string
  name: string
  user_id?: string
  line_group_id?: string
  line_group_name?: string
  origin_id?: string
  origin_name?: string
  origin_addresses?: string[]
  origin_scheme?: string
  origin_port?: number
  origin_host_mode?: string
  origin_host?: string
  origin_timeout_ms?: number
  origin_connect_timeout_ms?: number
  // 回源鉴权配置：节点在回源时注入认证信息，防止源站被绕过 CDN 直接访问。
  origin_auth?: OriginAuth
  // 负载方式："round_robin"（默认，按权重轮循）或 "ip_hash"（按客户端 IP 定源）。
  load_balance_method?: string
  // 源站主动健康检查：节点定期探测每个源站，自动下线/上线不健康的源站。
  origin_health_check?: OriginHealthCheck
  cert_id?: string
  cert_name?: string
  cert_domain?: string
  https_enabled?: boolean
  http2_enabled?: boolean
  listen_port?: number
  cname?: string
  error_pages?: ErrorPage[]
  websocket_enabled?: boolean
  enabled?: boolean
  cache_enabled?: boolean
  security?: DomainSecurity
  created_at?: string
  updated_at?: string
  dns_warnings?: string[]
  sync_task_ids?: string[]
}

// OriginAuth configures per-domain back-to-origin authentication.
export interface OriginAuth {
  enabled: boolean
  // "header" = inject custom HTTP headers; "basic" = HTTP Basic Auth.
  mode?: string
  headers?: OriginAuthHeader[]
  basic_user?: string
  basic_pass?: string
}

export interface OriginAuthHeader {
  name: string
  value: string
}

// OriginHealthCheck 配置节点对源站地址的主动健康探测。
// 探测全部由节点本地完成（节点到源站的网络状况各异），控制面只负责
// 下发本配置；当所有源站都被判定不健康时，节点会回退到全部源站，
// 避免出现"无源站可选"的故障。
export interface OriginHealthCheck {
  enabled: boolean
  interval_sec?: number    // 探测间隔，默认 30 秒，最小 5 秒
  timeout_ms?: number      // 单次探测超时，默认 5000 毫秒
  path?: string            // 探测路径，默认 "/"
  expected_status?: number // 期望状态码，0 表示 2xx/3xx 均视为成功
  fail_threshold?: number  // 连续失败 N 次判定下线，默认 3
  pass_threshold?: number  // 连续成功 N 次恢复，默认 2
}

// SyncTask mirrors server.syncTaskEvent: a single sync operation snapshot
// pushed via SSE (event: sync) and returned by GET /api/sync/active.
export interface SyncTask {
  kind: "publish" | "dns"
  id: string
  subject: string
  status: "running" | "success" | "failed"
  message?: string
  started_at: string
  completed_at?: string
}

// DomainCCRule mirrors store.DomainCCRule: a single row in the 自定义规则
// table under the 安全设置 tab.
export interface DomainCCRule {
  id: string
  match: string
  filter: string
  mode: string
  note?: string
  enabled: boolean
}

// DomainSecurity mirrors store.DomainSecurity: the full per-domain CC
// protection + IP black/white list configuration the detail page edits.
//
// There is no master `enabled` toggle: each sub-field is its own switch
// (default_mode === "off", empty ip_blacklist, per-custom-rule enabled,
// empty blocked_regions). A blank DomainSecurity therefore produces no
// edge policy at all, which matches the previous "disabled" behaviour
// without forcing operators to remember a top-level switch.
export interface DomainSecurity {
  default_mode?: string
  auto_switch?: boolean
  search_bot?: string
  ban_seconds?: number
  fail_limit?: number
  custom_rules?: DomainCCRule[]
  ip_blacklist?: string[]
  ip_whitelist?: string[]
  // ISO 3166-1 alpha-2 country codes to block, e.g. ["CN","US","RU"]
  // or magic tokens like "__FOREIGN_EXCLUDE_HKMOTW__"
  blocked_regions?: string[]
  // Block requests from transparent proxies (x-forwarded-for present)
  block_transparent_proxy?: boolean
}

export interface Certificate {
  id: number
  name: string
  domain: string
  user_id?: string
  type: string         // "acme" | "upload"
  auto_renew: boolean
  status: string       // "pending" | "active" | "failed" | "expired" | "expiring"
  fail_reason?: string
  expires_at?: string
  created_at?: string
  updated_at?: string
}

export interface UserOrder {
  id: string
  user_id: string
  product_id: string
  product_name: string
  amount_cents: number
  currency: string
  status: string
  period?: string
  quantity?: number
  starts_at?: string
  ends_at?: string
  paid_at?: string
  note?: string
  line_group_id?: string
  domain_limit?: number
  domain_count?: number
  created_at: string
  updated_at: string
}

export interface DomainHealthPoint {
  ts: number
  value: number
}

export interface DomainHealthSeries {
  key: string
  name: string
  unit: string
  points: DomainHealthPoint[]
}

export interface DomainHealthMetricsResponse {
  group: string
  window_seconds: number
  step_seconds: number
  from_unix: number
  to_unix: number
  domains: string[]
  port?: number
  series: DomainHealthSeries[]
}

export interface DomainHealthRankEntry {
  rank: number
  domain: string
  bandwidth_bps: number
  bytes: number
  requests: number
  qps: number
  http_4xx: number
  http_5xx: number
  error_rate: number
}

export interface DomainHealthRankResponse {
  metric: string
  window_seconds: number
  from_unix: number
  to_unix: number
  domains: string[]
  port?: number
  items: DomainHealthRankEntry[]
}

export interface ClusterNodeMeta {
  cluster_id: string
  node_id: string
  line: string
  enabled: boolean
  weight: number
  backup: boolean
  created_at?: string
  updated_at?: string
  node?: Node
}

export interface Origin {
  id: string
  name: string
  addresses: string[]
  timeout_ms?: number
  max_retries?: number
  created_at?: string
  updated_at?: string
}

// DomainOrigin: a single upstream address bound to a specific domain.
// This is the authoritative origin model after the "per-domain origins"
// refactor. Each row carries its own weight (1-100) and enabled flag;
// the edge node does weighted-random selection over enabled rows.
export interface DomainOrigin {
  id?: string
  domain_id?: string
  address: string
  weight: number
  enabled: boolean
  sort_order?: number
  created_at?: string
  updated_at?: string
}

// CacheRule mirrors server/internal/store.CacheRule. There is no
// domain_id column — rules are keyed by host/path glob patterns, so a
// "per-domain view" in the UI is produced by filtering host_pattern.
export interface CacheRule {
  id: string
  name: string
  host_pattern: string
  path_pattern: string
  methods?: string[]
  ttl_seconds: number
  cache_query_params?: boolean
  priority: number
  enabled: boolean
  created_at?: string
  updated_at?: string
}

// WAFRule mirrors server/internal/store.WAFRule. Types:
// - ip_cidr: IP/CIDR 黑白名单
// - rate_limit: 频率限制
// - challenge_captcha: 人机验证（滑块/点选/旋转/JS挑战）
// - shield_5s: 5秒盾（浏览器 JS 检测）
export interface WAFRule {
  id: string
  policy_id: string
  type: string            // ip_cidr | rate_limit | challenge_captcha | shield_5s
  action: string          // allow | deny
  value?: string          // CIDR for ip_cidr; optional key
  threshold?: number      // rate_limit: max requests; challenge: fail limit
  window_seconds?: number // rate_limit: time window
  shield_seconds?: number // shield_5s: shield duration
  auto_challenge_qps?: number // trigger challenge when QPS exceeds
  ban_seconds?: number    // base ban seconds (escalates)
  template_html?: string  // custom challenge HTML
  ban_template_html?: string // custom ban HTML
  redirect_url?: string   // optional redirect for challenge
  ban_mode?: string       // ipset | drop | page
  captcha_type?: string   // slide | click | rotate | slide_region | js_challenge
  path_prefix?: string
  methods?: string[]
  ua_contains?: string
  log_only?: boolean
  note?: string
  priority?: number
  enabled: boolean
  created_at?: string
  updated_at?: string
}

// WAFPolicy mirrors server/internal/store.WAFPolicy. scope is one of
// "global" | "domain" | "line_group"; scope_id carries the referenced
// id when scope is not global.
export interface WAFPolicy {
  id: string
  name: string
  scope: string
  scope_id?: string
  description?: string
  enabled: boolean
  rules?: WAFRule[]
  created_at?: string
  updated_at?: string
}

export interface WAFWhitelistEntry {
  id: string
  ip: string
  note?: string
  created_by?: string
  created_at?: string
  updated_at?: string
}

export interface GlobalTemplate {
  key: string
  name: string
  group: string
  mode: string
  default_content: string
  content: string
  customized: boolean
  updated_at?: string
  placeholders?: string[]
}

export interface SystemTask {
  id: string
  rel_id?: string
  source: string
  type: string
  message: string
  status: string
  sub_tasks: number
  created_at: string
  updated_at: string
  retryable: boolean
  detail_url?: string
}

export interface SystemTaskSummary {
  tasks: number
  sub_tasks: number
  pending: number
  running: number
  success: number
  failed: number
  unknown: number
}

export interface PublishTask {
  id: string
  trigger: string
  reason: string
  version: string
  node_ids: string[]
  status: string
  message: string
  started_at: string
  completed_at: string
  total_nodes: number
  success_nodes: number
  failed_nodes: number
  errors?: Record<string, string>
}

export interface PublishTaskNode {
  node_id: string
  status: string
  message: string
}

export interface DdosXdpSnapshot {
  enabled: boolean
  interface: string
  updated_at: string
  stats: Record<string, number>
}

export interface DdosXdpStatsItem {
  node: {
    id: string
    hostname: string
    public_ip?: string
    version?: string
    status?: string
    region?: string
    cluster?: string
    last_heartbeat?: string
    geo?: GeoLocation
  }
  xdp?: DdosXdpSnapshot
}

export interface DdosXdpStatsResponse {
  generated_at: string
  summary: {
    nodes_total: number
    nodes_with_xdp: number
    nodes_xdp_enabled: number
    latest_xdp_at: string
    totals: Record<string, number>
  }
  items: DdosXdpStatsItem[]
}

class APIClient {
  private baseURL: string

  constructor(baseURL: string) {
    this.baseURL = baseURL
  }

  private featureUnavailable(feature: string, message: string = FEATURE_UNAVAILABLE_MESSAGE): never {
    throw new FeatureUnavailableError(feature, message)
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseURL}${endpoint}`
    const method = String(options.method || "GET").toUpperCase()
    const sameOrigin = isSameOrigin(this.baseURL)
    const doFetch = async (token: string | null) => {
      const normalizedHeaders = normalizeHeaders(options.headers)
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...normalizedHeaders,
      }
      if (sameOrigin && method !== "GET" && method !== "HEAD" && method !== "OPTIONS") {
        const csrf = resolveCSRFToken()
        if (csrf && !headers["X-CSRF-Token"]) {
          headers["X-CSRF-Token"] = csrf
        }
      }
      return fetch(url, {
        ...options,
        headers,
        credentials: 'include',
      })
    }

    const token = getAuthToken()
    let response = await doFetch(token)
    if (response.status === 401 && token) {
      clearAuthToken()
      response = await doFetch(null)
    }

    const contentType = response.headers.get("Content-Type") || ""
    const isJSON = contentType.includes("application/json")

    if (!response.ok) {
      if (isJSON) {
        const error = await response.json().catch(() => ({
          error: response.statusText,
        }))
        throw new Error(error.error || `HTTP ${response.status}`)
      }
      const text = await response.text().catch(() => "")
      throw new Error(text || response.statusText || `HTTP ${response.status}`)
    }

    if (!isJSON) {
      const text = await response.text().catch(() => "")
      throw new Error(text ? `Invalid JSON response: ${text}` : "Invalid JSON response")
    }

    return response.json()
  }

  // Auth APIs
  async login(
    identifier: string,
    password: string,
    captcha?: { token: string; answer: string }
  ): Promise<AuthResponse> {
    return this.request<AuthResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({
        identifier,
        password,
        captcha_token: captcha?.token || "",
        captcha_answer: captcha?.answer || "",
      }),
    })
  }

  async adminLogin(
    identifier: string,
    password: string,
    captcha?: { token: string; answer: string }
  ): Promise<AuthResponse> {
    return this.login(identifier, password, captcha)
  }

  async register(
    username: string,
    email: string,
    password: string,
    emailCode?: string,
    captcha?: { token: string; answer: string }
  ): Promise<AuthResponse> {
    return this.request<AuthResponse>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({
        username,
        email,
        password,
        email_code: emailCode,
        captcha_token: captcha?.token || "",
        captcha_answer: captcha?.answer || "",
      }),
    })
  }

  async requestRegisterEmailCode(
    email: string,
    captcha?: { token: string; answer: string }
  ): Promise<{ ok: boolean; expires_in?: number }> {
    return this.request<{ ok: boolean; expires_in?: number }>('/api/auth/register/email/request', {
      method: 'POST',
      body: JSON.stringify({
        email,
        captcha_token: captcha?.token || "",
        captcha_answer: captcha?.answer || "",
      }),
    })
  }

  async logout(): Promise<void> {
    return Promise.resolve()
  }

  async getCaptcha(): Promise<CaptchaChallenge> {
    return this.request<CaptchaChallenge>("/api/auth/captcha")
  }

  async getMe(): Promise<{ user: User }> {
    return this.request<{ user: User }>('/api/auth/me')
  }

  async changePassword(oldPassword: string, newPassword: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>('/api/auth/password/change', {
      method: 'POST',
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    })
  }

  async getPublicAnnouncements(params?: {
    page?: number
    pageSize?: number
    q?: string
  }): Promise<{ announcements: Announcement[]; total: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('page_size', String(params.pageSize))
    if (params?.q) qs.set('q', params.q)
    const q = qs.toString()
    return this.request<{ announcements: Announcement[]; total: number }>(`/api/public/announcements${q ? `?${q}` : ''}`, {
      method: 'GET',
    })
  }

  async listNodes(): Promise<{ nodes: Node[] }> {
    return this.request<{ nodes: Node[] }>('/api/nodes')
  }

  async listNodesOverview(params?: {
    page?: number
    pageSize?: number
    q?: string
    status?: string
    region?: string
  }): Promise<{ nodes: Node[]; total: number; page: number; page_size: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('page_size', String(params.pageSize))
    if (params?.q) qs.set('q', params.q)
    if (params?.status) qs.set('status', params.status)
    if (params?.region) qs.set('region', params.region)
    const q = qs.toString()
    return this.request<{ nodes: Node[]; total: number; page: number; page_size: number }>(`/api/nodes/overview${q ? `?${q}` : ''}`)
  }

  async getNodeMonitorRank(params?: {
    group?: string
    windowSeconds?: number
    limit?: number
  }): Promise<NodeMonitorRankResponse> {
    const qs = new URLSearchParams()
    if (params?.group) qs.set("group", params.group)
    if (params?.windowSeconds) qs.set("window_seconds", String(params.windowSeconds))
    if (params?.limit) qs.set("limit", String(params.limit))
    const suffix = qs.toString() ? `?${qs.toString()}` : ""
    return this.request<NodeMonitorRankResponse>(`/api/nodes/monitor/rank${suffix}`)
  }

  async getNodeMonitorSeries(params?: {
    metric?: string
    windowSeconds?: number
    points?: number
  }): Promise<NodeMonitorSeriesResponse> {
    const qs = new URLSearchParams()
    if (params?.metric) qs.set("metric", params.metric)
    if (params?.windowSeconds) qs.set("window_seconds", String(params.windowSeconds))
    if (params?.points) qs.set("points", String(params.points))
    const suffix = qs.toString() ? `?${qs.toString()}` : ""
    return this.request<NodeMonitorSeriesResponse>(`/api/nodes/monitor/series${suffix}`)
  }

  async updateNode(id: string, data: Partial<Node> & { token?: string }): Promise<{ ok: boolean; token?: string }> {
    return this.request<{ ok: boolean; token?: string }>(`/api/nodes/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async updateNodeMonitorConfig(id: string, data: {
    enabled?: boolean
    protocol?: string
    timeout_seconds?: number
    port?: number
    fail_threshold?: number
  }): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/nodes/${encodeURIComponent(id)}/monitor`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async deleteNode(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/nodes/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
  }

  async getNodeInstallCommand(params?: {
    portalBase?: string
    scriptURL?: string
    masterHost?: string
    masterVersion?: string
    ttlMinutes?: number
  }): Promise<NodeInstallCommandResponse> {
    const qs = new URLSearchParams()
    if (params?.portalBase) qs.set("portal_base", params.portalBase)
    if (params?.scriptURL) qs.set("script_url", params.scriptURL)
    if (params?.masterHost) qs.set("master_host", params.masterHost)
    if (params?.masterVersion) qs.set("master_version", params.masterVersion)
    if (params?.ttlMinutes) qs.set("ttl_minutes", String(params.ttlMinutes))
    const suffix = qs.toString() ? `?${qs.toString()}` : ""
    return this.request<NodeInstallCommandResponse>(`/api/nodes/install-command${suffix}`)
  }

  async installNodeViaSSH(data: {
    ssh_host: string
    ssh_port?: number
    ssh_user: string
    ssh_password: string
    master_host?: string
    ttl_minutes?: number
    portal_base?: string
  }): Promise<NodeInstallSSHResult> {
    return this.request<NodeInstallSSHResult>('/api/nodes/install-ssh', {
      method: 'POST',
      body: JSON.stringify(data || {}),
    })
  }

  // Product APIs
  async getProducts(): Promise<{ products: Product[] }> {
    return this.request<{ products: Product[] }>('/api/products')
  }

  async createProduct(data: {
    name: string
    slug?: string
    description?: string
    group_id?: string
    sort?: number
    region?: string
    line_group_id?: string
    cluster_id?: string

    monthly_traffic_bytes?: number | null
    bandwidth_bps?: number | null
    conn_limit?: number | null
    domain_limit?: number | null
    primary_domain_limit?: number | null
    http_port_limit?: number | null
    stream_port_limit?: number | null
    non_std_port_limit?: number | null
    websocket?: boolean
    custom_cc_rules?: boolean
    http3?: boolean
    l2_origin?: boolean
    cc_protection?: string
    ddos_protection?: string

    price_cents?: number
    price_month_cents?: number
    price_quarter_cents?: number
    price_year_cents?: number
    currency?: string
    enabled?: boolean
  }): Promise<{ product: Product }> {
    return this.request<{ product: Product }>('/api/products', {
      method: 'POST',
      body: JSON.stringify(data || {}),
    })
  }

  async updateProduct(
    id: string,
    data: {
      name?: string
      slug?: string
      description?: string
      group_id?: string
      sort?: number
      region?: string
      line_group_id?: string
      cluster_id?: string

      monthly_traffic_bytes?: number | null
      bandwidth_bps?: number | null
      conn_limit?: number | null
      domain_limit?: number | null
      primary_domain_limit?: number | null
      http_port_limit?: number | null
      stream_port_limit?: number | null
      non_std_port_limit?: number | null
      websocket?: boolean
      custom_cc_rules?: boolean
      http3?: boolean
      l2_origin?: boolean
      cc_protection?: string
      ddos_protection?: string

      price_cents?: number
      price_month_cents?: number
      price_quarter_cents?: number
      price_year_cents?: number
      currency?: string
      enabled?: boolean
    }
  ): Promise<{ product: Product }> {
    return this.request<{ product: Product }>(`/api/products/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(data || {}),
    })
  }

  async deleteProduct(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/products/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
  }

  async getAdminOrders(params?: {
    userId?: string
    status?: string
    page?: number
    pageSize?: number
  }): Promise<{ orders: Order[]; total: number; page?: number; page_size?: number }> {
    const qs = new URLSearchParams()
    if (params?.userId) qs.set("user_id", params.userId)
    if (params?.status) qs.set("status", params.status)
    if (params?.page) qs.set("page", String(params.page))
    if (params?.pageSize) qs.set("page_size", String(params.pageSize))
    const suffix = qs.toString() ? `?${qs.toString()}` : ""
    return this.request<{ orders: Order[]; total: number; page?: number; page_size?: number }>(`/api/admin/orders${suffix}`)
  }

  async createAdminOrder(data: {
    user_id: string
    product_id: string
    period?: string
    quantity?: number
    status?: string
    note?: string
    amount_cents?: number
    currency?: string
    starts_at?: string
    ends_at?: string
    paid_at?: string
  }): Promise<{ order: Order }> {
    return this.request<{ order: Order }>(`/api/admin/orders`, {
      method: "POST",
      body: JSON.stringify(data || {}),
    })
  }

  async updateAdminOrder(id: string, data: Partial<Order>): Promise<{ order: Order }> {
    return this.request<{ order: Order }>(`/api/admin/orders/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(data || {}),
    })
  }

  async deleteAdminOrder(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/admin/orders/${encodeURIComponent(id)}`, {
      method: "DELETE",
    })
  }

  async getProductGroups(): Promise<{ groups: ProductGroup[] }> {
    return this.request<{ groups: ProductGroup[] }>('/api/product-groups')
  }

  async createProductGroup(data: { name: string; sort?: number; description?: string }): Promise<{ group: ProductGroup }> {
    return this.request<{ group: ProductGroup }>('/api/product-groups', {
      method: 'POST',
      body: JSON.stringify(data || {}),
    })
  }

  async updateProductGroup(
    id: string,
    data: { name?: string; sort?: number; description?: string }
  ): Promise<{ group: ProductGroup }> {
    return this.request<{ group: ProductGroup }>(`/api/product-groups/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(data || {}),
    })
  }

  async deleteProductGroup(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/product-groups/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
  }

  // System License APIs
  async getSystemLicenseStatus(): Promise<SystemLicenseStatusResponse> {
    return this.request<SystemLicenseStatusResponse>('/api/license/status')
  }

  async getOverview(window?: string): Promise<OverviewResponse> {
    const query = window ? `?window=${encodeURIComponent(window)}` : ""
    return this.request<OverviewResponse>(`/api/overview${query}`)
  }

  async getSystemInfo(): Promise<{ info: SystemInfo }> {
    return this.request<{ info: SystemInfo }>('/api/system/info')
  }

  async getPublicLicenseStatus(): Promise<PublicLicenseStatusResponse> {
    return this.request<PublicLicenseStatusResponse>('/api/public/license/status')
  }

  async activateSystemLicense(licenseKey: string): Promise<{ license: SystemLicenseState }> {
    return this.request<{ license: SystemLicenseState }>('/api/license/activate', {
      method: 'POST',
      body: JSON.stringify({ license_key: licenseKey }),
    })
  }

  // Balance APIs
  async getBalanceAccount(): Promise<{ account: BalanceAccount }> {
    return this.request<{ account: BalanceAccount }>('/api/balance/account')
  }

  async listBalanceTransactions(params?: {
    page?: number
    pageSize?: number
    type?: string
  }): Promise<{ transactions: BalanceTransaction[]; total?: number; page?: number; page_size?: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('page_size', String(params.pageSize))
    if (params?.type) qs.set('type', params.type)
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return this.request<{ transactions: BalanceTransaction[]; total?: number; page?: number; page_size?: number }>(
      `/api/balance/transactions${suffix}`
    )
  }

  async createBalanceRecharge(data: {
    amount_cents: number
    payment_method?: string
  }): Promise<{ recharge: BalanceRecharge; pay_url?: string; qr_code?: string; form_html?: string }> {
    return this.request<{ recharge: BalanceRecharge; pay_url?: string; qr_code?: string; form_html?: string }>(
      '/api/balance/recharges',
      {
        method: 'POST',
        body: JSON.stringify(data),
      }
    )
  }

  async listBalanceRecharges(params?: {
    page?: number
    pageSize?: number
    status?: string
  }): Promise<{ recharges: BalanceRecharge[]; total?: number; page?: number; page_size?: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('page_size', String(params.pageSize))
    if (params?.status) qs.set('status', params.status)
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return this.request<{ recharges: BalanceRecharge[]; total?: number; page?: number; page_size?: number }>(
      `/api/balance/recharges${suffix}`
    )
  }

  async createBalanceWithdrawal(data: {
    amount_cents: number
    method?: string
    account_name?: string
    account_no?: string
    note?: string
  }): Promise<{ withdrawal: BalanceWithdrawal }> {
    void data
    return this.featureUnavailable("balance.withdrawals.create")
  }

  async listBalanceWithdrawals(params?: {
    page?: number
    pageSize?: number
    status?: string
  }): Promise<{ withdrawals: BalanceWithdrawal[]; total?: number; page?: number; page_size?: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('page_size', String(params.pageSize))
    if (params?.status) qs.set('status', params.status)
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return this.request<{ withdrawals: BalanceWithdrawal[]; total?: number; page?: number; page_size?: number }>(
      `/api/balance/withdrawals${suffix}`
    )
  }

  async adminListBalanceAccounts(params?: {
    page?: number
    pageSize?: number
    userId?: string
  }): Promise<{ accounts: BalanceAccount[]; total?: number; page?: number; page_size?: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('page_size', String(params.pageSize))
    if (params?.userId) qs.set('user_id', params.userId)
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return this.request<{ accounts: BalanceAccount[]; total?: number; page?: number; page_size?: number }>(
      `/api/admin/balance/accounts${suffix}`
    )
  }

  async adminListBalanceRecharges(params?: {
    page?: number
    pageSize?: number
    status?: string
    userId?: string
  }): Promise<{ recharges: BalanceRecharge[]; total?: number; page?: number; page_size?: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('page_size', String(params.pageSize))
    if (params?.status) qs.set('status', params.status)
    if (params?.userId) qs.set('user_id', params.userId)
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return this.request<{ recharges: BalanceRecharge[]; total?: number; page?: number; page_size?: number }>(
      `/api/admin/balance/recharges${suffix}`
    )
  }

  async adminUpdateBalanceRecharge(id: string, data: { status?: string; trade_no?: string }): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/admin/balance/recharges/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async adminListBalanceWithdrawals(params?: {
    page?: number
    pageSize?: number
    status?: string
    userId?: string
  }): Promise<{ withdrawals: BalanceWithdrawal[]; total?: number; page?: number; page_size?: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('page_size', String(params.pageSize))
    if (params?.status) qs.set('status', params.status)
    if (params?.userId) qs.set('user_id', params.userId)
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return this.request<{ withdrawals: BalanceWithdrawal[]; total?: number; page?: number; page_size?: number }>(
      `/api/admin/balance/withdrawals${suffix}`
    )
  }

  async adminUpdateBalanceWithdrawal(id: string, data: { status?: string; note?: string }): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/admin/balance/withdrawals/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    })
  }

  async adminAdjustBalance(data: { user_id: string; amount_cents: number; note?: string }): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>('/api/admin/balance/adjust', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async adminBalanceStats(params?: {
    from?: string
    to?: string
    start_date?: string
    end_date?: string
  }): Promise<{ stats: BalanceStatsDay[] }> {
    const qs = new URLSearchParams()
    if (params?.from) qs.set('from', params.from)
    if (params?.to) qs.set('to', params.to)
    if (params?.start_date) qs.set('start_date', params.start_date)
    if (params?.end_date) qs.set('end_date', params.end_date)
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return this.request<{ stats: BalanceStatsDay[] }>(`/api/admin/balance/stats${suffix}`)
  }

  async getAdminAnnouncements(params?: {
    page?: number
    pageSize?: number
    status?: string
    q?: string
  }): Promise<{ announcements: Announcement[]; total: number }> {
    const qs = new URLSearchParams()
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('pageSize', String(params.pageSize))
    if (params?.status) qs.set('status', params.status)
    if (params?.q) qs.set('q', params.q)
    const q = qs.toString()
    return this.request<{ announcements: Announcement[]; total: number }>(`/api/admin/announcements${q ? `?${q}` : ''}`, {
      method: 'GET',
    })
  }

  async createAnnouncement(data: { title: string; content: string; status?: string; pinned?: boolean }): Promise<{ announcement: Announcement }> {
    return this.request<{ announcement: Announcement }>('/api/admin/announcements', {
      method: 'POST',
      body: JSON.stringify(data || {}),
    })
  }

  async updateAnnouncement(id: string, data: { title?: string; content?: string; status?: string; pinned?: boolean }): Promise<{ announcement: Announcement }> {
    return this.request<{ announcement: Announcement }>(`/api/admin/announcements/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data || {}),
    })
  }

  async deleteAnnouncement(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/admin/announcements/${id}`, { method: 'DELETE' })
  }

  // User APIs (Admin only)
  async getUsers(): Promise<{ users: User[] }> {
    return this.request<{ users: User[] }>('/api/users')
  }

  async createUser(
    username: string,
    email: string,
    password: string,
    role?: string
  ): Promise<User> {
    return this.request<User>('/api/users', {
      method: 'POST',
      body: JSON.stringify({ username, email, password, role }),
    })
  }

  async deleteUser(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/users/${encodeURIComponent(id)}`, {
      method: "DELETE",
    })
  }

  async updateUser(id: string, data: { status?: string; role?: string; password?: string }): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/users/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    })
  }

  async bulkUsers(data: { ids: string[]; action: string; password?: string }): Promise<{ ok: boolean; total: number; success: number; failed: any[] }> {
    return this.request<{ ok: boolean; total: number; success: number; failed: any[] }>(`/api/users/bulk`, {
      method: "POST",
      body: JSON.stringify(data),
    })
  }

  async requestPasswordReset(email: string): Promise<{ ok: boolean; expires_at?: string; code?: string }> {
    return this.request<{ ok: boolean; expires_at?: string; code?: string }>("/api/auth/password/reset/request", {
      method: "POST",
      body: JSON.stringify({ email }),
    })
  }

  async confirmPasswordReset(email: string, code: string, newPassword: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>("/api/auth/password/reset/confirm", {
      method: "POST",
      body: JSON.stringify({ email, code, password: newPassword }),
    })
  }

  // Settings APIs
  async getSettings(): Promise<{ settings: any }> {
    return this.request<{ settings: any }>('/api/settings')
  }

  async getPublicSettings(): Promise<{ settings: any }> {
    return this.request<{ settings: any }>('/api/public/settings')
  }

  async updateSettings(settings: any): Promise<{ settings: any }> {
    return this.request<{ settings: any }>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    })
  }

  async checkESHealth(): Promise<{ status: number; body: any }> {
    return this.request<{ status: number; body: any }>('/api/logs/es/health')
  }

  async getSystemLogs(params?: {
    type?: string
    status?: string
    q?: string
    page?: number
    pageSize?: number
  }): Promise<{ logs: SystemLog[]; total: number }> {
    const qs = new URLSearchParams()
    if (params?.type) qs.set('type', params.type)
    if (params?.status) qs.set('status', params.status)
    if (params?.q) qs.set('q', params.q)
    if (params?.page) qs.set('page', String(params.page))
    if (params?.pageSize) qs.set('pageSize', String(params.pageSize))
    const q = qs.toString()
    return this.request<{ logs: SystemLog[]; total: number }>(`/api/admin/logs${q ? `?${q}` : ''}`, {
      method: 'GET',
    })
  }

  async listGlobalTemplates(): Promise<{ templates: GlobalTemplate[] }> {
    return this.request<{ templates: GlobalTemplate[] }>('/api/system/templates')
  }

  async getGlobalTemplate(key: string): Promise<{ template: GlobalTemplate }> {
    return this.request<{ template: GlobalTemplate }>(`/api/system/templates/${encodeURIComponent(key)}`)
  }

  async updateGlobalTemplate(key: string, content: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/system/templates/${encodeURIComponent(key)}`, {
      method: 'PUT',
      body: JSON.stringify({ content }),
    })
  }

  async resetGlobalTemplate(key: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/system/templates/${encodeURIComponent(key)}/reset`, {
      method: 'POST',
    })
  }

  async listSystemTasks(params?: {
    source?: string
    type?: string
    status?: string
    rel_id?: string
    id?: string
    limit?: number
  }): Promise<{ summary: SystemTaskSummary; tasks: SystemTask[] }> {
    const q = new URLSearchParams()
    if (params?.source) q.set("source", params.source)
    if (params?.type) q.set("type", params.type)
    if (params?.status) q.set("status", params.status)
    if (params?.rel_id) q.set("rel_id", params.rel_id)
    if (params?.id) q.set("id", params.id)
    if (params?.limit) q.set("limit", String(params.limit))
    const query = q.toString() ? `?${q.toString()}` : ""
    return this.request<{ summary: SystemTaskSummary; tasks: SystemTask[] }>(`/api/system/tasks${query}`)
  }

  async retrySystemTask(id: string): Promise<{ ok: boolean; task_id: string }> {
    return this.request<{ ok: boolean; task_id: string }>(`/api/system/tasks/${encodeURIComponent(id)}/retry`, {
      method: "POST",
    })
  }

  async getPublishTask(id: string): Promise<{ task: PublishTask; nodes: PublishTaskNode[] }> {
    return this.request<{ task: PublishTask; nodes: PublishTaskNode[] }>(`/api/system/publish/tasks/${encodeURIComponent(id)}`)
  }

  // listActiveSyncTasks returns running sync tasks and failed tasks within the
  // last 5 minutes. Used to bootstrap per-row sync indicators before the SSE
  // stream catches up. `subjectPrefix` narrows by subject (e.g. "domain:").
  async listActiveSyncTasks(subjectPrefix?: string): Promise<SyncTask[]> {
    const q = new URLSearchParams()
    if (subjectPrefix) q.set("subject", subjectPrefix)
    const query = q.toString() ? `?${q.toString()}` : ""
    const resp = await this.request<{ tasks: SyncTask[] }>(`/api/sync/active${query}`)
    return resp?.tasks || []
  }

  // API Tokens
  async listAPITokens(): Promise<{ tokens: APIToken[] }> {
    return this.request<{ tokens: APIToken[] }>('/api/api-tokens')
  }

  async createAPIToken(data: { description: string; ttl_days?: number }): Promise<{ token: string; api_token: APIToken }> {
    return this.request<{ token: string; api_token: APIToken }>('/api/api-tokens', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async deleteAPIToken(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/api-tokens/${id}`, {
      method: 'DELETE',
    })
  }

  // Domains
  async listDomains(): Promise<{ domains: Domain[]; count: number }> {
    return this.request<{ domains: Domain[]; count: number }>('/api/domains')
  }

  async getDomain(id: string): Promise<Domain> {
    return this.request<Domain>(`/api/domains/${encodeURIComponent(id)}`)
  }

  async createDomain(data: Domain): Promise<Domain> {
    return this.request<Domain>('/api/domains', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateDomain(id: string, data: Domain): Promise<Domain> {
    return this.request<Domain>(`/api/domains/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async deleteDomain(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/domains/${id}`, {
      method: 'DELETE',
    })
  }

  async getDomainSecurity(id: string): Promise<DomainSecurity> {
    return this.request<DomainSecurity>(`/api/domains/${id}/security`)
  }

  async updateDomainSecurity(id: string, data: DomainSecurity): Promise<DomainSecurity> {
    return this.request<DomainSecurity>(`/api/domains/${id}/security`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async getDomainHealthMetrics(params: {
    group?: string
    fromUnix: number
    toUnix: number
    domains?: string[]
    port?: number
    points?: number
    windowSeconds?: number
  }): Promise<DomainHealthMetricsResponse> {
    const qs = new URLSearchParams()
    if (params.group) qs.set("group", params.group)
    qs.set("from_unix", String(params.fromUnix))
    qs.set("to_unix", String(params.toUnix))
    if (params.domains && params.domains.length > 0) qs.set("domains", params.domains.join(","))
    if (params.port) qs.set("port", String(params.port))
    if (params.points) qs.set("points", String(params.points))
    if (params.windowSeconds) qs.set("window_seconds", String(params.windowSeconds))
    return this.request<DomainHealthMetricsResponse>(`/api/domains/health/metrics?${qs.toString()}`)
  }

  async getDomainHealthRank(params: {
    metric?: string
    fromUnix: number
    toUnix: number
    domains?: string[]
    port?: number
    limit?: number
    windowSeconds?: number
  }): Promise<DomainHealthRankResponse> {
    const qs = new URLSearchParams()
    if (params.metric) qs.set("metric", params.metric)
    qs.set("from_unix", String(params.fromUnix))
    qs.set("to_unix", String(params.toUnix))
    if (params.domains && params.domains.length > 0) qs.set("domains", params.domains.join(","))
    if (params.port) qs.set("port", String(params.port))
    if (params.limit) qs.set("limit", String(params.limit))
    if (params.windowSeconds) qs.set("window_seconds", String(params.windowSeconds))
    return this.request<DomainHealthRankResponse>(`/api/domains/health/rank?${qs.toString()}`)
  }

  // User Orders
  async getUserOrders(): Promise<{ orders: UserOrder[]; total: number }> {
    return this.request<{ orders: UserOrder[]; total: number }>('/api/user/orders')
  }

  // Certificates
  async listCertificates(): Promise<{ certificates: Certificate[] }> {
    return this.request<{ certificates: Certificate[] }>('/api/certificates')
  }

  async createCertificate(data: {
    domain: string
    name?: string
    user_id?: string
    cert_pem: string
    key_pem: string
  }): Promise<Certificate> {
    return this.request<Certificate>('/api/certificates', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async deleteCertificate(id: number): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/certificates/${id}`, {
      method: 'DELETE',
    })
  }

  async requestACMECertificate(data: { domain: string; user_id?: string }): Promise<{ ok: boolean; id: number; domain: string; expires_at: string }> {
    return this.request<{ ok: boolean; id: number; domain: string; expires_at: string }>('/api/certificates/acme', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  // Origins
  async listOrigins(): Promise<{ origins: Origin[] }> {
    return this.request<{ origins: Origin[] }>('/api/origins')
  }

  async createOrigin(data: Origin): Promise<Origin> {
    return this.request<Origin>('/api/origins', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  // Per-domain origins (authoritative after the 2026-04 refactor).
  // Replace-the-whole-set semantics: GET returns the current list,
  // PUT swaps it atomically with the supplied array.
  async listDomainOrigins(domainId: string): Promise<{ origins: DomainOrigin[] }> {
    return this.request<{ origins: DomainOrigin[] }>(
      `/api/domains/${encodeURIComponent(domainId)}/origins`,
    )
  }

  async replaceDomainOrigins(
    domainId: string,
    origins: DomainOrigin[],
  ): Promise<{ origins: DomainOrigin[] }> {
    return this.request<{ origins: DomainOrigin[] }>(
      `/api/domains/${encodeURIComponent(domainId)}/origins`,
      {
        method: 'PUT',
        body: JSON.stringify({ origins }),
      },
    )
  }

  // Cluster node APIs
  async listClusterNodes(clusterId: string, line?: string): Promise<{ nodes: ClusterNodeMeta[] }> {
    const query = line ? `?line=${encodeURIComponent(line)}` : ""
    return this.request<{ nodes: ClusterNodeMeta[] }>(`/api/clusters/${encodeURIComponent(clusterId)}/nodes${query}`)
  }

  async listClusterAvailableNodes(clusterId: string): Promise<{ nodes: Node[] }> {
    return this.request<{ nodes: Node[] }>(`/api/clusters/${encodeURIComponent(clusterId)}/nodes/available`)
  }

  async addClusterNodes(clusterId: string, nodeIds: string[], line?: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/clusters/${encodeURIComponent(clusterId)}/nodes`, {
      method: "POST",
      body: JSON.stringify({ node_ids: nodeIds, line }),
    })
  }

  async updateClusterNode(clusterId: string, nodeId: string, data: { enabled?: boolean; weight?: number; backup?: boolean; line?: string }): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/clusters/${encodeURIComponent(clusterId)}/nodes/${encodeURIComponent(nodeId)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    })
  }

  async deleteClusterNode(clusterId: string, nodeId: string, line?: string): Promise<{ ok: boolean }> {
    const query = line ? `?line=${encodeURIComponent(line)}` : ""
    return this.request<{ ok: boolean }>(`/api/clusters/${encodeURIComponent(clusterId)}/nodes/${encodeURIComponent(nodeId)}${query}`, {
      method: "DELETE",
    })
  }

  // Domain Blacklist
  async listDomainBlacklist(): Promise<{ blacklist: DomainBlacklist[] }> {
    return this.request<{ blacklist: DomainBlacklist[] }>('/api/domain-blacklist')
  }

  async createDomainBlacklist(data: { domain: string; reason?: string }): Promise<{ ok: boolean; blacklist: DomainBlacklist }> {
    return this.request<{ ok: boolean; blacklist: DomainBlacklist }>('/api/domain-blacklist', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async deleteDomainBlacklist(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/domain-blacklist/${id}`, {
      method: 'DELETE',
    })
  }

  async getDdosXdpStats(): Promise<DdosXdpStatsResponse> {
    return this.request<DdosXdpStatsResponse>('/api/ddos/xdp/stats')
  }

  // Cluster APIs
  async listClusters(): Promise<{ clusters: Cluster[] }> {
    return this.request<{ clusters: Cluster[] }>('/api/clusters')
  }

  async createCluster(data: Partial<Cluster>): Promise<Cluster> {
    return this.request<Cluster>('/api/clusters', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateCluster(id: string, data: Partial<Cluster>): Promise<Cluster> {
    return this.request<Cluster>(`/api/clusters/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async deleteCluster(id: string): Promise<void> {
    return this.request<void>(`/api/clusters/${id}`, { method: 'DELETE' })
  }

  // DNS APIs
  async getDNSConfig(): Promise<{ config: DNSConfig; providers: DNSProviderOption[] }> {
    return this.request<{ config: DNSConfig; providers: DNSProviderOption[] }>('/api/dns/config')
  }

  async saveDNSConfig(data: {
    provider: string
    account_id: string
    token: string
    secret?: string
    ttl: number
    enable_ip_weight: boolean
  }): Promise<{ config: DNSConfig; providers: DNSProviderOption[]; message?: string }> {
    return this.request<{ config: DNSConfig; providers: DNSProviderOption[]; message?: string }>('/api/dns/config', {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async recoverDNSConfig(): Promise<{ ok: boolean; message?: string; task_id?: string }> {
    return this.request<{ ok: boolean; message?: string; task_id?: string }>('/api/dns/config/recover', {
      method: 'POST',
    })
  }

  async cleanupDNSConfig(): Promise<{ ok: boolean; message?: string; task_id?: string }> {
    return this.request<{ ok: boolean; message?: string; task_id?: string }>('/api/dns/config/cleanup', {
      method: 'POST',
    })
  }

  async syncDNS(): Promise<{ ok: boolean; message?: string; task_id?: string }> {
    return this.request<{ ok: boolean; message?: string; task_id?: string }>('/api/dns/sync', {
      method: 'POST',
    })
  }

  async listDNSTasks(): Promise<{ tasks: DNSTask[] }> {
    return this.request<{ tasks: DNSTask[] }>('/api/dns/tasks')
  }

  async getProviderDomains(): Promise<{ domains: { name: string; record_count: number }[] }> {
    return this.request<{ domains: { name: string; record_count: number }[] }>('/api/dns/provider-domains')
  }

  async getDNSLines(): Promise<{ lines: { value: string; label: string }[]; provider: string; supports_line: boolean }> {
    return this.request<{ lines: { value: string; label: string }[]; provider: string; supports_line: boolean }>('/api/dns/lines')
  }

  // Upgrade APIs (Admin only)
  async getUpgradeInfo(channel?: string): Promise<UpgradeInfo> {
    const query = channel ? `?channel=${encodeURIComponent(channel)}` : ''
    return this.request<UpgradeInfo>(`/api/system/upgrade${query}`)
  }

  async upgradeControl(channel?: string): Promise<{ ok: boolean; task_id: string; message?: string }> {
    return this.request<{ ok: boolean; task_id: string; message?: string }>('/api/system/upgrade/control', {
      method: 'POST',
      body: JSON.stringify({ channel }),
    })
  }

  async upgradeNodes(data: {
    node_ids?: string[]
    target_version?: string
    force?: boolean
    channel?: string
  }): Promise<{ ok: boolean; task_id: string; message?: string; target?: string; scheduled_ids?: string[] }> {
    return this.request<{ ok: boolean; task_id: string; message?: string; target?: string; scheduled_ids?: string[] }>(
      '/api/system/upgrade/nodes',
      {
        method: 'POST',
        body: JSON.stringify(data),
      }
    )
  }

  async getUpgradeTasks(): Promise<{ tasks: UpgradeTask[] }> {
    return this.request<{ tasks: UpgradeTask[] }>('/api/system/upgrade/tasks')
  }

  async getUpgradeTaskLogs(taskId: string, nodeId?: string): Promise<{ logs: UpgradeLog[] }> {
    const query = nodeId ? `?node_id=${encodeURIComponent(nodeId)}` : ''
    return this.request<{ logs: UpgradeLog[] }>(`/api/system/upgrade/tasks/${taskId}${query}`)
  }

  // Purge / Cache
  async purgeURLs(urls: string[]): Promise<{ ok: boolean; request_id?: string; message?: string }> {
    return this.request<{ ok: boolean; request_id?: string; message?: string }>('/api/purge', {
      method: 'POST',
      body: JSON.stringify({ urls }),
    })
  }

  // Log search (ES)
  async searchLogs(params: {
    domain?: string
    ip?: string
    status?: string
    query?: string
    from?: string
    to?: string
    size?: number
  }): Promise<{ hits: any[]; total: number }> {
    return this.request<{ hits: any[]; total: number }>('/api/logs/search', {
      method: 'POST',
      body: JSON.stringify(params),
    })
  }

  // WAF Bans
  async listWAFBans(): Promise<{ bans: any[] }> {
    return this.request<{ bans: any[] }>('/api/waf/bans')
  }

  // Cache rules. The backend endpoint is global (no domain_id filter);
  // callers that want a per-domain view filter client-side by host_pattern.
  async listCacheRules(): Promise<{ cache_rules: CacheRule[] }> {
    return this.request<{ cache_rules: CacheRule[] }>("/api/cache-rules")
  }

  async createCacheRule(rule: Partial<CacheRule>): Promise<CacheRule> {
    return this.request<CacheRule>("/api/cache-rules", {
      method: "POST",
      body: JSON.stringify(rule),
    })
  }

  async updateCacheRule(id: string, rule: Partial<CacheRule>): Promise<CacheRule> {
    return this.request<CacheRule>(`/api/cache-rules/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(rule),
    })
  }

  async deleteCacheRule(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/cache-rules/${encodeURIComponent(id)}`, {
      method: "DELETE",
    })
  }

  // WAF policies. Optional scope/scope_id query filter lets callers ask
  // for only domain-scoped or cluster-scoped policies without pulling
  // the whole list.
  async listWAFPolicies(params?: { scope?: string; scope_id?: string }): Promise<{ policies: WAFPolicy[] }> {
    const q = new URLSearchParams()
    if (params?.scope) q.set("scope", params.scope)
    if (params?.scope_id) q.set("scope_id", params.scope_id)
    const suffix = q.toString() ? `?${q.toString()}` : ""
    return this.request<{ policies: WAFPolicy[] }>(`/api/waf/policies${suffix}`)
  }

  async createWAFPolicy(policy: Partial<WAFPolicy>): Promise<{ policy: WAFPolicy }> {
    return this.request<{ policy: WAFPolicy }>("/api/waf/policies", {
      method: "POST",
      body: JSON.stringify(policy),
    })
  }

  async updateWAFPolicy(id: string, policy: Partial<WAFPolicy>): Promise<{ policy: WAFPolicy }> {
    return this.request<{ policy: WAFPolicy }>(`/api/waf/policies/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(policy),
    })
  }

  async deleteWAFPolicy(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/waf/policies/${encodeURIComponent(id)}`, {
      method: "DELETE",
    })
  }

  // Dedicated "toggle" endpoint — only mutates the `enabled` flag. Avoids the
  // round-trip through the full PUT handler which would re-validate every
  // existing rule; using that for a one-bit change made the list-view switch
  // fail for any policy with legacy (pre-validation) rules.
  async setWAFPolicyEnabled(id: string, enabled: boolean): Promise<{ ok: boolean; enabled: boolean }> {
    return this.request<{ ok: boolean; enabled: boolean }>(
      `/api/waf/policies/${encodeURIComponent(id)}/enabled`,
      {
        method: "PATCH",
        body: JSON.stringify({ enabled }),
      }
    )
  }

  // WAF whitelist is a flat global list — there is no per-domain scope
  // server-side. The detail view consumes it read-only for context.
  async listWAFWhitelist(): Promise<{ whitelist: WAFWhitelistEntry[] }> {
    return this.request<{ whitelist: WAFWhitelistEntry[] }>("/api/waf/whitelist")
  }

  async createWAFWhitelist(entry: { ip: string; note?: string }): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>("/api/waf/whitelist", {
      method: "POST",
      body: JSON.stringify(entry),
    })
  }

  async deleteWAFWhitelist(id: string): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>(`/api/waf/whitelist?id=${encodeURIComponent(id)}`, {
      method: "DELETE",
    })
  }

  // Global CC policy convenience endpoint. Used by the detail-page
  // "Quick CC" panel and the standalone WAF management page.
  async getCCPolicy(): Promise<{ level: string; action: string; ban_seconds: number; fail_limit: number; ban_mode?: string }> {
    return this.request<{ level: string; action: string; ban_seconds: number; fail_limit: number; ban_mode?: string }>("/api/waf/cc")
  }

  async setCCPolicy(payload: { level: string; action: string; ban_seconds: number; fail_limit: number; ban_mode?: string }): Promise<{ ok: boolean }> {
    return this.request<{ ok: boolean }>("/api/waf/cc", {
      method: "POST",
      body: JSON.stringify(payload),
    })
  }

  // User order creation
  async createUserOrder(data: {
    product_id: string
    period: string
  }): Promise<{ order: any }> {
    return this.request<{ order: any }>('/api/user/orders', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }
}

export const api = new APIClient(API_BASE)


