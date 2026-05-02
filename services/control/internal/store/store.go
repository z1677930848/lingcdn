package store

import (
	"context"
	"errors"
	"time"
)

// ErrNodeDisabled is returned by RegisterOrRefreshNode when the hostname
// already exists but is in the "disabled" state. Callers should surface this
// as PermissionDenied rather than retrying, because a disabled node is an
// operator-driven lockout (typically after license revocation or abuse).
var ErrNodeDisabled = errors.New("node disabled by control plane")

// Store abstracts persistent storage operations.
type Store interface {
	Ping(ctx context.Context) error
	Migrate(ctx context.Context) error
	Seed(ctx context.Context) error
	Close()

	// User operations
	CreateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByLogin(ctx context.Context, login string) (*User, error)
	ListUsers(ctx context.Context, limit int) ([]*User, error)
	CountUsers(ctx context.Context) (int, error)
	UpdateUserLastLogin(ctx context.Context, id string, lastLoginAt time.Time, ip string, location string) error
	UpdateUserStatus(ctx context.Context, id string, status string) error
	UpdateUserRole(ctx context.Context, id string, role string) error
	UpdateUserPasswordHash(ctx context.Context, id string, passwordHash string) error
	DeleteUser(ctx context.Context, id string) error

	// Node operations
	CreateNode(ctx context.Context, node *Node) error
	GetNode(ctx context.Context, id string) (*Node, error)
	GetNodeByHostname(ctx context.Context, hostname string) (*Node, error)
	ListNodes(ctx context.Context) ([]*Node, error)
	CountNodes(ctx context.Context) (int, error)
	UpdateNodeStatus(ctx context.Context, id, status, configVersion string) error
	UpdateNodeToken(ctx context.Context, id, tokenHash string) error
	UpdateNode(ctx context.Context, node *Node) error
	// RegisterOrRefreshNode atomically performs node registration or
	// re-registration keyed by hostname:
	//   - no existing row: inserts node with all fields (id, token, capabilities, ...)
	//   - existing row, not disabled: refreshes public_ip/version/region/capabilities/token and sets status='online'
	//   - existing row, disabled: returns ErrNodeDisabled and leaves the row unchanged
	// The returned id is the node's final id (the input id when inserting, the existing id when refreshing).
	// Callers must still perform any admission checks (bootstrap token, license) beforehand.
	RegisterOrRefreshNode(ctx context.Context, node *Node) (string, error)
	// UpdateNodeHeartbeatInfo updates only the fields that change on every heartbeat.
	// Empty string values are ignored (existing column retained) to prevent accidental data loss.
	UpdateNodeHeartbeatInfo(ctx context.Context, id, publicIP, version, region string) error
	UpdateNodeMonitorConfig(ctx context.Context, id string, cfg NodeMonitorConfig) error
	UpdateNodeMonitorResult(ctx context.Context, id string, res NodeMonitorResult) error
	UpdateNodeTelemetry(ctx context.Context, id string, t NodeTelemetry) error
	DeleteNode(ctx context.Context, id string) error

	// Cluster operations
	CreateCluster(ctx context.Context, c *Cluster) error
	GetCluster(ctx context.Context, id string) (*Cluster, error)
	ListClusters(ctx context.Context) ([]*Cluster, error)
	UpdateCluster(ctx context.Context, c *Cluster) error
	DeleteCluster(ctx context.Context, id string) error
	ListClusterNodes(ctx context.Context, clusterID, line string) ([]*ClusterNode, error)
	UpsertClusterNode(ctx context.Context, n *ClusterNode) error
	DeleteClusterNode(ctx context.Context, clusterID, line, nodeID string) error

	// Domain operations
	CreateDomain(ctx context.Context, domain *Domain) error
	GetDomain(ctx context.Context, id string) (*Domain, error)
	GetDomainByName(ctx context.Context, name string) (*Domain, error)
	ListDomains(ctx context.Context) ([]*Domain, error)
	ListDomainsByUser(ctx context.Context, userID string) ([]*Domain, error)
	CountDomains(ctx context.Context) (int, error)
	CountDomainsByUser(ctx context.Context, userID string) (int, error)
	UpdateDomain(ctx context.Context, domain *Domain) error
	DeleteDomain(ctx context.Context, id string) error

	// Origin operations
	CreateOrigin(ctx context.Context, origin *Origin) error
	GetOrigin(ctx context.Context, id string) (*Origin, error)
	ListOrigins(ctx context.Context) ([]*Origin, error)
	UpdateOrigin(ctx context.Context, origin *Origin) error
	DeleteOrigin(ctx context.Context, id string) error

	// DomainOrigin operations (per-domain origin addresses, authoritative)
	ListDomainOrigins(ctx context.Context, domainID string) ([]*DomainOrigin, error)
	ListAllDomainOrigins(ctx context.Context) ([]*DomainOrigin, error)
	ReplaceDomainOrigins(ctx context.Context, domainID string, entries []*DomainOrigin) error
	DeleteDomainOrigins(ctx context.Context, domainID string) error

	// Certificate operations
	CreateCertificate(ctx context.Context, cert *Certificate) error
	GetCertificate(ctx context.Context, id int64) (*Certificate, error)
	GetCertificateByDomain(ctx context.Context, domain string) (*Certificate, error)
	ListCertificates(ctx context.Context) ([]*Certificate, error)
	ListCertificatesByUser(ctx context.Context, userID string) ([]*Certificate, error)
	UpdateCertificate(ctx context.Context, cert *Certificate) error
	DeleteCertificate(ctx context.Context, id int64) error

	// Config version operations
	CreateConfigVersion(ctx context.Context, cv *ConfigVersion) error
	GetConfigVersion(ctx context.Context, version string) (*ConfigVersion, error)
	GetLatestConfigVersion(ctx context.Context) (*ConfigVersion, error)
	ListConfigVersions(ctx context.Context, limit int) ([]*ConfigVersion, error)

	// Cache rule operations
	CreateCacheRule(ctx context.Context, rule *CacheRule) error
	GetCacheRule(ctx context.Context, id string) (*CacheRule, error)
	ListCacheRules(ctx context.Context) ([]*CacheRule, error)
	UpdateCacheRule(ctx context.Context, rule *CacheRule) error
	DeleteCacheRule(ctx context.Context, id string) error

	// Token operations
	ValidateServiceToken(ctx context.Context, token string) (bool, error)
	ValidateBootstrapToken(ctx context.Context, token string) (bool, error)
	CreateBootstrapToken(ctx context.Context, description string, ttl time.Duration) (token string, expiresAt time.Time, err error)

	// License state (single row)
	SetLicenseState(ctx context.Context, st *LicenseState) error
	GetLicenseState(ctx context.Context) (*LicenseState, error)

	// System settings (single row)
	GetSettings(ctx context.Context) (*Settings, error)
	UpdateSettings(ctx context.Context, s *Settings) error

	// DNS config
	GetDNSConfig(ctx context.Context) (*DNSConfig, error)
	SaveDNSConfig(ctx context.Context, cfg *DNSConfig) error

	// Upgrade tasks/logs
	CreateUpgradeTask(ctx context.Context, t *UpgradeTask) error
	UpdateUpgradeTaskStatus(ctx context.Context, id, status string) error
	ListUpgradeTasks(ctx context.Context, limit int) ([]*UpgradeTask, error)
	GetUpgradeTask(ctx context.Context, id string) (*UpgradeTask, error)
	AppendUpgradeLog(ctx context.Context, id string, log UpgradeLog) error
	ListUpgradeLogs(ctx context.Context, id, nodeID string, limit int) ([]UpgradeLog, error)

	GetBalanceAccount(ctx context.Context, userID string) (*BalanceAccount, error)
	ListBalanceTransactions(ctx context.Context, userID string, page, pageSize int) ([]*BalanceTransaction, int64, error)
	AdminListBalanceAccounts(ctx context.Context, userID string, page, pageSize int) ([]*BalanceAccount, int64, error)
	AdminListBalanceRecharges(ctx context.Context, userID, status string, page, pageSize int) ([]*BalanceRecharge, int64, error)
	AdminUpdateBalanceRecharge(ctx context.Context, id, status, tradeNo string, paidAt time.Time) error
	AdminListBalanceWithdrawals(ctx context.Context, userID, status string, page, pageSize int) ([]*BalanceWithdrawal, int64, error)
	AdminUpdateBalanceWithdrawal(ctx context.Context, id, status, note string, reviewedAt time.Time) error
	AdminAdjustBalance(ctx context.Context, userID string, amountCents int64, note string) error
	AdminRechargeStats(ctx context.Context, from, to time.Time) ([]*BalanceRechargeStats, error)

	// Product / order operations
	ListProductGroups(ctx context.Context) ([]*ProductGroup, error)
	GetProductGroup(ctx context.Context, id string) (*ProductGroup, error)
	CreateProductGroup(ctx context.Context, g *ProductGroup) error
	UpdateProductGroup(ctx context.Context, g *ProductGroup) error
	DeleteProductGroup(ctx context.Context, id string) error

	ListProducts(ctx context.Context) ([]*Product, error)
	GetProduct(ctx context.Context, id string) (*Product, error)
	CreateProduct(ctx context.Context, p *Product) error
	UpdateProduct(ctx context.Context, p *Product) error
	DeleteProduct(ctx context.Context, id string) error

	ListOrders(ctx context.Context, userID string) ([]*Order, error)
	GetOrder(ctx context.Context, id string) (*Order, error)
	CreateOrder(ctx context.Context, o *Order) error
	UpdateOrder(ctx context.Context, o *Order) error
	DeleteOrder(ctx context.Context, id string) error

	// User traffic tracking
	GetUserTraffic(ctx context.Context, userID, month string) (int64, error)
	IncrementUserTraffic(ctx context.Context, userID, month string, bytes int64) error

	ListAnnouncements(ctx context.Context, status, q string, page, pageSize int) ([]*Announcement, int64, error)
	GetAnnouncement(ctx context.Context, id string) (*Announcement, error)
	CreateAnnouncement(ctx context.Context, a *Announcement) error
	UpdateAnnouncement(ctx context.Context, a *Announcement) error
	DeleteAnnouncement(ctx context.Context, id string) error

	// System logs (audit)
	CreateSystemLog(ctx context.Context, log *SystemLog) error
	ListSystemLogs(ctx context.Context, logType, status, q string, page, pageSize int) ([]*SystemLog, int64, error)

	DeleteSystemLogsOlderThan(ctx context.Context, before time.Time) (int64, error)
	DeleteExpiredWafBansOlderThan(ctx context.Context, before time.Time) (int64, error)
	DeleteUpgradeTasksOlderThan(ctx context.Context, before time.Time) (int64, error)

	// WAF / ACL
	ListWAFPolicies(ctx context.Context) ([]*WAFPolicy, error)
	GetWAFPolicy(ctx context.Context, id string) (*WAFPolicy, error)
	CreateWAFPolicy(ctx context.Context, p *WAFPolicy) error
	UpdateWAFPolicy(ctx context.Context, p *WAFPolicy) error
	DeleteWAFPolicy(ctx context.Context, id string) error
	ListWAFRules(ctx context.Context, policyID string) ([]*WAFRule, error)
	ReplaceWAFRules(ctx context.Context, policyID string, rules []*WAFRule) error

	// WAF bans (global distribution)
	ListWAFBans(ctx context.Context, limit int) ([]*WAFBan, error)
	CreateOrUpdateWAFBan(ctx context.Context, ban *WAFBan) error
	DeleteWAFBan(ctx context.Context, ip string) error
	GetWAFBan(ctx context.Context, ip string) (*WAFBan, error)

	// WAF whitelist (global)
	ListWAFWhitelist(ctx context.Context) ([]*WAFWhitelist, error)
	CreateWAFWhitelist(ctx context.Context, w *WAFWhitelist) error
	DeleteWAFWhitelist(ctx context.Context, id string) error
	IsIPWhitelisted(ctx context.Context, ip string) (bool, error)

	// Email verifications
	CreateEmailVerification(ctx context.Context, v *EmailVerification) error
	GetLatestEmailVerificationByEmail(ctx context.Context, email string) (*EmailVerification, error)
	MarkEmailVerificationUsed(ctx context.Context, id string, usedAt time.Time) (bool, error)

	// API tokens
	CreateAPIToken(ctx context.Context, description string, ttl time.Duration) (token string, t *APIToken, err error)
	ListAPITokens(ctx context.Context) ([]*APIToken, error)
	DeleteAPIToken(ctx context.Context, id string) error
	ValidateAPIToken(ctx context.Context, token string) (bool, error)

	// Domain blacklist
	CreateDomainBlacklist(ctx context.Context, b *DomainBlacklist) error
	ListDomainBlacklist(ctx context.Context) ([]*DomainBlacklist, error)
	DeleteDomainBlacklist(ctx context.Context, id string) error
	IsDomainBlacklisted(ctx context.Context, domain string) (bool, string, error)

	ListGlobalTemplateOverrides(ctx context.Context) ([]*GlobalTemplateOverride, error)
	GetGlobalTemplateOverride(ctx context.Context, key string) (*GlobalTemplateOverride, error)
	UpsertGlobalTemplateOverride(ctx context.Context, t *GlobalTemplateOverride) error
	DeleteGlobalTemplateOverride(ctx context.Context, key string) error
}

// Node represents a CDN edge node.
type Node struct {
	ID                   string    `json:"id"`
	Hostname             string    `json:"hostname"`
	PublicIP             string    `json:"public_ip,omitempty"`
	Version              string    `json:"version"`
	Status               string    `json:"status"`
	Region               string    `json:"region,omitempty"`
	Cluster              string    `json:"cluster,omitempty"`
	Capabilities         []string  `json:"capabilities"`
	ConfigVersion        string    `json:"config_version,omitempty"`
	Token                string    `json:"token,omitempty"`
	LastHeartbeat        time.Time `json:"last_heartbeat,omitempty"`
	MonitorEnabled       bool      `json:"monitor_enabled"`
	MonitorProtocol      string    `json:"monitor_protocol,omitempty"`
	MonitorTimeout       int       `json:"monitor_timeout_seconds,omitempty"`
	MonitorPort          int       `json:"monitor_port,omitempty"`
	MonitorFailThreshold int       `json:"monitor_fail_threshold,omitempty"`
	MonitorFailCount     int       `json:"monitor_fail_count,omitempty"`
	MonitorLastOK        bool      `json:"monitor_last_ok,omitempty"`
	MonitorLastError     string    `json:"monitor_last_error,omitempty"`
	MonitorLastAt        time.Time `json:"monitor_last_at,omitempty"`
	MonitorLastLatencyMs int       `json:"monitor_last_latency_ms,omitempty"`
	CPUUsage             float64   `json:"cpu_usage"`
	MemUsage             float64   `json:"mem_usage"`
	DiskUsage            float64   `json:"disk_usage"`
	CPUCount             int32     `json:"cpu_count,omitempty"`
	MemTotal             int64     `json:"mem_total,omitempty"`
	DiskTotal            int64     `json:"disk_total,omitempty"`
	LastMetricsAt        time.Time `json:"last_metrics_at,omitempty"`
	BytesSent            int64     `json:"bytes_sent,omitempty"`
	BytesReceived        int64     `json:"bytes_received,omitempty"`
	BandwidthUpBps       float64   `json:"bandwidth_up_bps"`
	BandwidthDownBps     float64   `json:"bandwidth_down_bps"`
	TCPEstablished       int32     `json:"tcp_established"`
	TCPSynRecv           int32     `json:"tcp_syn_recv,omitempty"`
	TCPTimeWait          int32     `json:"tcp_time_wait,omitempty"`
	NginxRunning         bool      `json:"nginx_running,omitempty"`
	MonthBytesSent       int64     `json:"month_bytes_sent,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type NodeMonitorConfig struct {
	Enabled        bool
	Protocol       string
	TimeoutSeconds int
	Port           int
	FailThreshold  int
}

type NodeMonitorResult struct {
	LastOK        bool
	LastError     string
	LastAt        time.Time
	LastLatencyMs int
	FailCount     int
}

type NodeTelemetry struct {
	CPUUsage       float64
	MemUsage       float64
	DiskUsage      float64
	CPUCount       int32
	MemTotal       int64
	DiskTotal      int64
	BytesSent      int64
	BytesReceived  int64
	TCPEstablished int32
	TCPSynRecv     int32
	TCPTimeWait    int32
	NginxRunning   bool
}

// User represents an operator or tenant user.
type User struct {
	ID                string     `json:"id"`
	NumericID         int64      `json:"numeric_id"`
	Username          string     `json:"username"`
	Email             string     `json:"email"`
	PasswordHash      string     `json:"-"`
	Role              string     `json:"role"`   // admin | user
	Status            string     `json:"status"` // active | disabled
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP       string     `json:"last_login_ip,omitempty"`
	LastLoginLocation string     `json:"last_login_location,omitempty"`
}

type BalanceAccount struct {
	UserID       string    `json:"user_id"`
	BalanceCents int64     `json:"balance_cents"`
	Currency     string    `json:"currency"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BalanceTransaction struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Type         string    `json:"type"`
	AmountCents  int64     `json:"amount_cents"`
	BalanceCents int64     `json:"balance_cents"`
	Note         string    `json:"note"`
	RefType      string    `json:"ref_type"`
	RefID        string    `json:"ref_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type BalanceRecharge struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	OutTradeNo    string    `json:"out_trade_no"`
	AmountCents   int64     `json:"amount_cents"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	Status        string    `json:"status"`
	TradeNo       string    `json:"trade_no"`
	PaidAt        time.Time `json:"paid_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type BalanceWithdrawal struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	Method      string    `json:"method"`
	AccountName string    `json:"account_name"`
	AccountNo   string    `json:"account_no"`
	Status      string    `json:"status"`
	Note        string    `json:"note"`
	ReviewedAt  time.Time `json:"reviewed_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BalanceRechargeStats struct {
	Day           string `json:"day"`
	RechargeCents int64  `json:"recharge_cents"`
	RechargeCount int64  `json:"recharge_count"`
	AdjustCents   int64  `json:"adjust_cents"`
	AdjustCount   int64  `json:"adjust_count"`
	TotalCents    int64  `json:"total_cents"`
	PaidCents     int64  `json:"paid_cents"`
	PaidCount     int64  `json:"paid_count"`
	PendingCount  int64  `json:"pending_count"`
	TotalCount    int64  `json:"total_count"`
}

type Announcement struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SystemLog 表示管理端/系统操作审计日志。
type SystemLog struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`   // login | action | backup | email
	Status    string    `json:"status"` // success | failed
	Message   string    `json:"message"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	IP        string    `json:"ip"`
	Location  string    `json:"location"`
	CreatedAt time.Time `json:"created_at"`
}

type Product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	GroupID     string `json:"group_id,omitempty"`
	ClusterID   string `json:"cluster_id,omitempty"`
	Sort        int    `json:"sort"`
	Region      string `json:"region,omitempty"`
	LineGroupID string `json:"line_group_id,omitempty"`

	MonthlyTrafficBytes *int64 `json:"monthly_traffic_bytes,omitempty"`
	BandwidthBps        *int64 `json:"bandwidth_bps,omitempty"`
	ConnLimit           *int64 `json:"conn_limit,omitempty"`

	DomainLimit        *int32 `json:"domain_limit,omitempty"`
	PrimaryDomainLimit *int32 `json:"primary_domain_limit,omitempty"`
	HTTPPortLimit      *int32 `json:"http_port_limit,omitempty"`
	StreamPortLimit    *int32 `json:"stream_port_limit,omitempty"`
	NonStdPortLimit    *int32 `json:"non_std_port_limit,omitempty"`

	Websocket      bool   `json:"websocket"`
	CustomCCRules  bool   `json:"custom_cc_rules"`
	HTTP3          bool   `json:"http3"`
	L2Origin       bool   `json:"l2_origin"`
	CCProtection   string `json:"cc_protection,omitempty"`
	DDoSProtection string `json:"ddos_protection,omitempty"`

	PriceCents        int64     `json:"price_cents"`
	PriceMonthCents   int64     `json:"price_month_cents"`
	PriceQuarterCents int64     `json:"price_quarter_cents"`
	PriceYearCents    int64     `json:"price_year_cents"`
	Currency          string    `json:"currency"`
	Enabled           bool      `json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ProductGroup struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Sort        int       `json:"sort"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Order struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	ProductID   string     `json:"product_id"`
	ProductName string     `json:"product_name"`
	AmountCents int64      `json:"amount_cents"`
	Currency    string     `json:"currency"`
	Status      string     `json:"status"`
	Period      string     `json:"period"`
	Quantity    int32      `json:"quantity"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	Note        string     `json:"note,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Domain represents a CDN domain configuration.
type Domain struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CNAME       string `json:"cname,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	LineGroupID string `json:"line_group_id,omitempty"`
	OriginID    string `json:"origin_id,omitempty"`
	CertID      string `json:"cert_id,omitempty"`
	// HTTPSEnabled is the explicit on/off switch for 443 listening. Previously
	// the system derived this from `CertID != ""`, which meant the UI's HTTPS
	// toggle was effectively cosmetic (it could not represent "have a cert
	// but don't use it" and could not turn HTTPS off without also unbinding
	// the certificate). Persisting the flag directly fixes that.
	HTTPSEnabled           bool            `json:"https_enabled"`
	HTTP2Enabled           bool            `json:"http2_enabled"`
	OriginScheme           string          `json:"origin_scheme,omitempty"` // http | https | follow_protocol | follow_port | follow_both
	OriginPort             int32           `json:"origin_port,omitempty"`
	OriginHostMode         string          `json:"origin_host_mode,omitempty"` // request_host | request_host_port | custom
	OriginHost             string          `json:"origin_host,omitempty"`
	OriginTimeoutMs        int64           `json:"origin_timeout_ms,omitempty"`         // total timeout
	OriginConnectTimeoutMs int64           `json:"origin_connect_timeout_ms,omitempty"` // dial timeout
	// OriginAuth configures authentication that the CDN node presents to the
	// origin on every back-to-origin request. Supported modes:
	//   - "header": node injects one or more custom HTTP headers (e.g.
	//     X-Origin-Auth: <secret>) so the origin can verify the request
	//     came through the CDN and reject direct-IP access.
	//   - "basic": node uses HTTP Basic Authentication (username + password).
	// When Enabled is false (or the struct is nil/zero), no auth headers
	// are added and the origin fetch is plain.
	OriginAuth *OriginAuth `json:"origin_auth,omitempty"`
	ErrorPages []ErrorPage `json:"error_pages,omitempty"`
	WebsocketEnabled       bool            `json:"websocket_enabled"`
	Enabled                bool            `json:"enabled"`
	CacheEnabled           bool            `json:"cache_enabled"`
	// LoadBalanceMethod 控制节点上多个 origin 地址之间的请求分配策略：
	//   - "round_robin"：按权重随机选择（默认，等权时为均匀随机）
	//   - "ip_hash"：按客户端 IP 的一致性哈希固定到某个 origin，常用于会话保持
	// 空字符串视为 "round_robin"，与历史行为兼容。
	LoadBalanceMethod      string          `json:"load_balance_method,omitempty"`
	// OriginHealthCheck 控制节点对每个 origin 地址的主动健康检查。
	// 启用后，节点周期性发送 HTTP 探测请求，连续失败次数达到阈值时
	// 自动将该 origin 从可选池中摘除（请求会落到其他 origin），探测
	// 恢复成功达到阈值后再重新上线。当所有 origin 都不健康时，节点
	// 会降级回全量列表以避免出现"无源站可选"的故障。
	OriginHealthCheck      *OriginHealthCheck `json:"origin_health_check,omitempty"`
	Security               *DomainSecurity `json:"security,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

// OriginHealthCheck 是单个域名级别的源站健康检查配置。
// 节点收到配置后会为每个 enabled 的 origin 地址独立维护一个状态机。
type OriginHealthCheck struct {
	Enabled        bool   `json:"enabled"`
	IntervalSec    int32  `json:"interval_sec,omitempty"`    // 探测间隔，默认 30 秒，最小 5 秒
	TimeoutMs      int64  `json:"timeout_ms,omitempty"`      // 单次探测超时，默认 5000 毫秒
	Path           string `json:"path,omitempty"`             // 探测路径，默认 "/"
	ExpectedStatus int32  `json:"expected_status,omitempty"` // 期望状态码，默认 0 表示 2xx/3xx 均视为成功
	FailThreshold  int32  `json:"fail_threshold,omitempty"`  // 连续失败 N 次判定下线，默认 3
	PassThreshold  int32  `json:"pass_threshold,omitempty"`  // 连续成功 N 次恢复，默认 2
}

// DomainSecurity carries per-domain CC-protection settings that the
// "Security" tab in the UI edits. The compiler translates this struct
// into a scope=domain WAFPolicyConfig delivered to edge nodes — nodes
// don't need to know about this shape, they just consume WAF policies.
type DomainSecurity struct {
	// Enabled is the master switch for the whole per-domain security block.
	// When false, the compiler skips this domain entirely regardless of
	// default_mode / custom_rules / blacklist contents — so users can
	// disable protection without having to clear every sub-setting.
	// Defaults to true for backward compatibility with records created
	// before this field existed (see storage layer upgrades).
	Enabled bool `json:"enabled"`

	// DefaultMode is the single-preset fallback when no custom rule
	// matches and auto-switch is not in effect. Valid values:
	//   off | loose | js | shield5s | click | slide | captcha | rotate |
	//   click_easy | slide_easy | custom
	DefaultMode string `json:"default_mode,omitempty"`

	// AutoSwitch tells the node to dynamically raise/lower the effective
	// mode based on live QPS + error-rate signals.
	AutoSwitch bool `json:"auto_switch,omitempty"`

	// SearchBot controls crawler handling for Google/Baidu/Sogou/360.
	// Valid values: off | allow | deny.
	SearchBot string `json:"search_bot,omitempty"`

	// BanSeconds is how long a challenged client stays challenged.
	BanSeconds int64 `json:"ban_seconds,omitempty"`

	// FailLimit is how many consecutive challenge failures trigger a ban.
	FailLimit int32 `json:"fail_limit,omitempty"`

	// CustomRules evaluate top-down — the UI wording "rules match bottom
	// to top" is reconciled by reversing the slice before execution.
	// Each rule carries its own mode so different URIs can have
	// different protection levels on the same domain.
	CustomRules []DomainCCRule `json:"custom_rules,omitempty"`

	// Plain-line IP lists, parsed into CIDR at compile time. Supports
	// "#" for inline comments like "1.1.1.1 # office gateway".
	IPBlacklist []string `json:"ip_blacklist,omitempty"`
	IPWhitelist []string `json:"ip_whitelist,omitempty"`

	// BlockedRegions holds country/region codes (ISO 3166-1 alpha-2)
	// that should be denied access to this domain. The compiler
	// translates each code into a WAF rule of type "geo_block".
	// Example values: ["CN", "US", "RU", "HK", "TW"].
	// Magic tokens: "__FOREIGN_EXCLUDE_HKMOTW__" and
	// "__FOREIGN_INCLUDE_HKMOTW__" are expanded at the node.
	BlockedRegions []string `json:"blocked_regions,omitempty"`

	// BlockTransparentProxy when true denies requests that carry
	// x-forwarded-for headers (transparent / open proxies).
	BlockTransparentProxy bool `json:"block_transparent_proxy,omitempty"`
}

// OriginAuth configures per-domain back-to-origin authentication.
// The CDN node inspects this at proxy time and injects the configured
// credentials into the upstream request, preventing the origin from
// being accessed directly (bypassing CDN).
type OriginAuth struct {
	// Enabled is the master switch. When false, no auth is applied.
	Enabled bool `json:"enabled"`
	// Mode selects the auth mechanism: "header" or "basic".
	//   - "header": inject custom HTTP headers listed in Headers.
	//   - "basic": inject an Authorization: Basic header from
	//     BasicUser / BasicPass.
	Mode string `json:"mode,omitempty"`
	// Headers is used when Mode == "header". Each entry is a
	// Name-Value pair injected into the upstream request.
	Headers []OriginAuthHeader `json:"headers,omitempty"`
	// BasicUser / BasicPass are used when Mode == "basic".
	BasicUser string `json:"basic_user,omitempty"`
	BasicPass string `json:"basic_pass,omitempty"`
}

// OriginAuthHeader is a single custom header injected on origin fetch.
type OriginAuthHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DomainCCRule is one row in the "自定义规则" table.
type DomainCCRule struct {
	ID      string `json:"id"`
	Match   string `json:"match"`  // DSL: "uri contains /api" / "ua contains curl"
	Filter  string `json:"filter"` // allow | deny | challenge
	Mode    string `json:"mode"`   // preset key same as DefaultMode
	Note    string `json:"note,omitempty"`
	Enabled bool   `json:"enabled"`
}

// ErrorPage represents a custom error page setting per status code.
type ErrorPage struct {
	Status  int    `json:"status"`            // e.g., 403/404/502/504
	Mode    string `json:"mode"`              // html | json | redirect
	Content string `json:"content,omitempty"` // html/json template or redirect URL
}

// Origin represents a legacy global origin server configuration.
//
// As of the domain-scoped origin refactor, new origins are stored in
// domain_origins (one row per address, weighted, per-domain). The Origin
// type and its backing `origins` table are kept only so existing data
// from before the refactor can still be read during the transition;
// they are no longer written to by the UI or the public API. New code
// should use DomainOrigin instead.
type Origin struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Addresses  []string  `json:"addresses"`
	TimeoutMs  int64     `json:"timeout_ms"`
	MaxRetries int32     `json:"max_retries"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DomainOrigin is a single upstream address bound to a domain. It is the
// authoritative origin model after the "per-domain origins" refactor:
// each domain owns N addresses, each carries an individual weight and
// enable flag, and the node performs weighted failover across the set.
//
//   - Address: "host" or "host:port". Port is optional; when absent, the
//     scheme-resolved port from the Domain is used.
//   - Weight: 1..100. Picks are weighted random across enabled entries;
//     a disabled entry never participates.
//   - SortOrder: UI display order only; does not affect picking.
type DomainOrigin struct {
	ID        string    `json:"id"`
	DomainID  string    `json:"domain_id"`
	Address   string    `json:"address"`
	Weight    int32     `json:"weight"`
	Enabled   bool      `json:"enabled"`
	SortOrder int32     `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Certificate represents a TLS certificate.
type Certificate struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Domain     string    `json:"domain"`
	UserID     string    `json:"user_id,omitempty"`
	Type       string    `json:"type"` // "acme" | "upload"
	AutoRenew  bool      `json:"auto_renew"`
	Status     string    `json:"status"` // "pending" | "active" | "failed" | "expired" | "expiring"
	FailReason string    `json:"fail_reason,omitempty"`
	CertPEM    []byte    `json:"cert_pem,omitempty"`
	KeyPEM     []byte    `json:"key_pem,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ConfigVersion represents a published configuration version.
type ConfigVersion struct {
	Version   string    `json:"version"`
	Checksum  string    `json:"checksum"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

// CacheRule represents a caching rule.
type CacheRule struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	HostPattern      string    `json:"host_pattern"`
	PathPattern      string    `json:"path_pattern"`
	Methods          []string  `json:"methods"`
	TTLSeconds       int64     `json:"ttl_seconds"`
	CacheQueryParams bool      `json:"cache_query_params"`
	Priority         int32     `json:"priority"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Cluster represents a logical group of nodes with its own DNS zone.
type Cluster struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DNSZone     string    `json:"dns_zone"`
	DNSMode     string    `json:"dns_mode"`
	CNAME       string    `json:"cname,omitempty"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ClusterNode stores per-node DNS settings in a cluster.
type ClusterNode struct {
	ClusterID string    `json:"cluster_id"`
	Line      string    `json:"line"`
	NodeID    string    `json:"node_id"`
	Enabled   bool      `json:"enabled"`
	Weight    int32     `json:"weight"`
	Backup    bool      `json:"backup"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DNSConfig stores DNS provider credentials/settings.
type DNSConfig struct {
	Provider       string    `json:"provider"`
	AccountID      string    `json:"account_id"`
	Token          string    `json:"token"`
	Secret         string    `json:"secret"`
	TTL            int64     `json:"ttl"`
	EnableIPWeight bool      `json:"enable_ip_weight"`
	LastError      string    `json:"last_error"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// WAFPolicy represents an ACL/WAF policy that can be bound to domain/线路组/全局.
type WAFPolicy struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Scope       string     `json:"scope"` // global | domain | line_group
	ScopeID     string     `json:"scope_id,omitempty"`
	Description string     `json:"description,omitempty"`
	Enabled     bool       `json:"enabled"`
	Rules       []*WAFRule `json:"rules,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// WAFRule defines a single rule inside a policy.
type WAFRule struct {
	ID               string    `json:"id"`
	PolicyID         string    `json:"policy_id"`
	Type             string    `json:"type"`                         // ip_cidr | rate_limit
	Action           string    `json:"action"`                       // allow | deny
	Value            string    `json:"value"`                        // CIDR for ip_cidr; optional key for rate_limit
	Threshold        int64     `json:"threshold,omitempty"`          // used by rate_limit
	WindowSeconds    int64     `json:"window_seconds,omitempty"`     // used by rate_limit
	ShieldSeconds    int64     `json:"shield_seconds,omitempty"`     // used by shield/5s
	AutoChallengeQPS int64     `json:"auto_challenge_qps,omitempty"` // trigger challenge when qps exceeds
	BanSeconds       int64     `json:"ban_seconds,omitempty"`        // base ban seconds (escalates)
	TemplateHTML     string    `json:"template_html,omitempty"`      // custom challenge HTML
	BanTemplateHTML  string    `json:"ban_template_html,omitempty"`  // custom ban HTML
	RedirectURL      string    `json:"redirect_url,omitempty"`       // optional redirect for challenge
	BanMode          string    `json:"ban_mode,omitempty"`           // ipset | drop | page
	CaptchaType      string    `json:"captcha_type,omitempty"`       // slide | click | rotate | slide_region | js_challenge
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	PathPrefix       string    `json:"path_prefix,omitempty"`
	Methods          []string  `json:"methods,omitempty"`
	UAContains       string    `json:"ua_contains,omitempty"`
	LogOnly          bool      `json:"log_only,omitempty"`
	Note             string    `json:"note,omitempty"`
	Priority         int32     `json:"priority,omitempty"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// WAFBan represents a distributed ban entry.
type WAFBan struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason,omitempty"`
	Strikes   int       `json:"strikes"`
	NodeID    string    `json:"node_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WAFWhitelist represents a whitelisted IP or CIDR.
type WAFWhitelist struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`         // IP address or CIDR (e.g., "192.168.1.1" or "10.0.0.0/8")
	Note      string    `json:"note"`       // Description/reason for whitelist
	CreatedBy string    `json:"created_by"` // User who created this entry
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpgradeTask represents an upgrade job.
type UpgradeTask struct {
	ID            string    `json:"id"`
	TargetVersion string    `json:"target_version"`
	Channel       string    `json:"channel"`
	NodeIDs       []string  `json:"node_ids"`
	Status        string    `json:"status"`
	Type          string    `json:"type"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpgradeLog captures node/task log lines.
type UpgradeLog struct {
	TaskID    string    `json:"task_id"`
	NodeID    string    `json:"node_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// Redis abstracts cache/coordination (placeholder).
type RedisLock interface {
	Release(ctx context.Context) error
}

type Redis interface {
	Ping(ctx context.Context) error
	SetJSON(ctx context.Context, key string, v any, ttl time.Duration) error
	GetJSON(ctx context.Context, key string, dest any) (bool, error)
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (RedisLock, bool, error)
}

// LicenseState represents cached license info.
type LicenseState struct {
	Status      string    `json:"status"`
	LicenseKey  string    `json:"license_key"`
	ExpiresAt   time.Time `json:"expires_at"`
	MaxNodes    int       `json:"max_nodes"`
	LastChecked time.Time `json:"last_checked"`
	GraceUntil  time.Time `json:"grace_until"`
	Reason      string    `json:"reason"`
	UpdatedAt   time.Time `json:"updated_at"`
	PubKey      string    `json:"pubkey,omitempty"`
}

// Settings stores system configuration.
type Settings struct {
	ID                       string `json:"id"`
	SystemName               string `json:"system_name"`
	FooterLinks              string `json:"footer_links"`
	FooterCopyright          string `json:"footer_copyright"`
	Favicon                  string `json:"favicon"`
	Logo                     string `json:"logo"`
	SMTPHost                 string `json:"smtp_host"`
	SMTPPort                 int    `json:"smtp_port"`
	SMTPUsername             string `json:"smtp_username"`
	SMTPPassword             string `json:"smtp_password"`
	SMTPFrom                 string `json:"smtp_from"`
	SMTPFromName             string `json:"smtp_from_name"`
	ElasticsearchURL         string `json:"elasticsearch_url"`
	ElasticsearchUser        string `json:"elasticsearch_user"`
	ElasticsearchPass        string `json:"elasticsearch_pass"`
	ElasticsearchIndex       string `json:"elasticsearch_index"`
	ElasticsearchTSField     string `json:"elasticsearch_ts_field"`
	ElasticsearchDomainField string `json:"elasticsearch_domain_field"`
	ElasticsearchBytesField  string `json:"elasticsearch_bytes_field"`
	SalesEmail                string    `json:"sales_email"`
	SupportEmail              string    `json:"support_email"`
	RegisterEnabled           bool      `json:"register_enabled"`
	UpgradeChannel            string    `json:"upgrade_channel"`
	NotifyNewBuild            bool      `json:"notify_new_build"`
	RegisterEmailVerification bool      `json:"register_email_verification"`
	EmailEnabled              bool      `json:"email_enabled"`
	DingtalkEnabled           bool      `json:"dingtalk_enabled"`
	DingtalkWebhook           string    `json:"dingtalk_webhook"`
	WechatEnabled             bool      `json:"wechat_enabled"`
	WechatWebhook             string    `json:"wechat_webhook"`
	FeishuEnabled             bool      `json:"feishu_enabled"`
	FeishuWebhook             string    `json:"feishu_webhook"`
	NotifyNodeResource        bool      `json:"notify_node_resource"`
	NotifyNodeMonitor         bool      `json:"notify_node_monitor"`
	NotifyTicketReply         bool      `json:"notify_ticket_reply"`
	NotifyInterval            int       `json:"notify_interval"`
	ThresholdCPU              int       `json:"threshold_cpu"`
	ThresholdMemory           int       `json:"threshold_memory"`
	ThresholdDisk             int       `json:"threshold_disk"`
	ThresholdBandwidthUp      int       `json:"threshold_bandwidth_up"`
	ThresholdBandwidthDown    int       `json:"threshold_bandwidth_down"`
	RetentionSystemLogs       int       `json:"retention_system_logs"`
	RetentionESLogs           int       `json:"retention_es_logs"`
	RetentionWafBans          int       `json:"retention_waf_bans"`
	RetentionUpgradeLogs      int       `json:"retention_upgrade_logs"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// EmailVerification represents a one-time email verification for registration.
type EmailVerification struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	TokenHash string     `json:"token_hash"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// APIToken represents an API token for programmatic access.
type APIToken struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	TokenPrefix string     `json:"token_prefix"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// DomainBlacklist represents a blacklisted domain.
type DomainBlacklist struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GlobalTemplateOverride struct {
	Key       string    `json:"key"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}
