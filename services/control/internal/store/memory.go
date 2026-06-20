package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Memory implements Store in memory (for dev/test).
type Memory struct {
	mu                      sync.RWMutex
	users                   map[string]*User // key: email lowercase
	usersByUsername         map[string]*User // key: username lowercase
	usersOrdered            []*User
	nodes                   map[string]*Node
	nodeMetricSamples       map[string][]NodeMetricSample
	domains                 map[string]*Domain
	origins                 map[string]*Origin
	domainOrigins           map[string][]*DomainOrigin // key: domain_id
	streamForwards          map[string]*StreamForward
	certs                   map[int64]*Certificate
	certSeq                 int64
	configVersions          map[string]*ConfigVersion
	cacheRules              map[string]*CacheRule
	clusterNodes            map[string]map[string]map[string]*ClusterNode // cluster_id -> line -> node_id -> meta
	serviceToken            string
	bootstrapToken          string
	latestConfigVer         string
	dnsConfig               *DNSConfig
	upgradeTasks            map[string]*UpgradeTask
	upgradeLogs             map[string][]UpgradeLog // key: task_id
	wafPolicies             map[string]*WAFPolicy
	wafRules                map[string][]*WAFRule
	wafBans                 map[string]*WAFBan
	wafWhitelist            map[string]*WAFWhitelist
	license                 *LicenseState
	pubKey                  string
	settings                *Settings
	emailVerifications      map[string]*EmailVerification
	apiTokens               map[string]*APIToken
	apiTokenHashes          map[string]string // hash -> id
	domainBlacklist         map[string]*DomainBlacklist
	globalTemplateOverrides map[string]*GlobalTemplateOverride
	productGroups           map[string]*ProductGroup
	products                map[string]*Product
	orders                  map[string]*Order
	balanceAccounts         map[string]*BalanceAccount
	balanceTransactions     map[string][]*BalanceTransaction
	balanceRecharges        map[string]*BalanceRecharge
	balanceWithdrawals      map[string]*BalanceWithdrawal
	announcements           map[string]*Announcement
	userGroups              map[string]*UserGroup
	clusters                map[string]*Cluster
	systemLogs              []*SystemLog
	userTraffic             map[string]int64 // key: "userID:month"
	alertRules              map[string]*AlertRule
	tickets                 map[string]*Ticket
	ticketReplies           map[string][]*TicketReply
}

func NewMemory(serviceToken, bootstrapToken string) *Memory {
	return &Memory{
		users:                   make(map[string]*User),
		usersByUsername:         make(map[string]*User),
		usersOrdered:            make([]*User, 0),
		nodes:                   make(map[string]*Node),
		domains:                 make(map[string]*Domain),
		origins:                 make(map[string]*Origin),
		domainOrigins:           make(map[string][]*DomainOrigin),
		streamForwards:          make(map[string]*StreamForward),
		certs:                   make(map[int64]*Certificate),
		configVersions:          make(map[string]*ConfigVersion),
		cacheRules:              make(map[string]*CacheRule),
		clusterNodes:            make(map[string]map[string]map[string]*ClusterNode),
		serviceToken:            serviceToken,
		bootstrapToken:          bootstrapToken,
		upgradeTasks:            make(map[string]*UpgradeTask),
		upgradeLogs:             make(map[string][]UpgradeLog),
		wafPolicies:             make(map[string]*WAFPolicy),
		wafRules:                make(map[string][]*WAFRule),
		wafBans:                 make(map[string]*WAFBan),
		wafWhitelist:            make(map[string]*WAFWhitelist),
		license:                 nil,
		pubKey:                  "",
		emailVerifications:      make(map[string]*EmailVerification),
		apiTokens:               make(map[string]*APIToken),
		apiTokenHashes:          make(map[string]string),
		domainBlacklist:         make(map[string]*DomainBlacklist),
		globalTemplateOverrides: make(map[string]*GlobalTemplateOverride),
		productGroups:           make(map[string]*ProductGroup),
		products:                make(map[string]*Product),
		orders:                  make(map[string]*Order),
		balanceAccounts:         make(map[string]*BalanceAccount),
		balanceTransactions:     make(map[string][]*BalanceTransaction),
		balanceRecharges:        make(map[string]*BalanceRecharge),
		balanceWithdrawals:      make(map[string]*BalanceWithdrawal),
		announcements:           make(map[string]*Announcement),
		userGroups:              make(map[string]*UserGroup),
		clusters:                make(map[string]*Cluster),
		systemLogs:              make([]*SystemLog, 0, 32),
		userTraffic:             make(map[string]int64),
		settings:                DefaultSettings(),
		alertRules:              make(map[string]*AlertRule),
		tickets:                 make(map[string]*Ticket),
		ticketReplies:           make(map[string][]*TicketReply),
	}
}

func (m *Memory) Ping(ctx context.Context) error    { _ = ctx; return nil }
func (m *Memory) Migrate(ctx context.Context) error { _ = ctx; return nil }
func (m *Memory) Seed(ctx context.Context) error    { _ = ctx; return nil }
func (m *Memory) Close()                            {}

// User operations
func (m *Memory) CreateUser(ctx context.Context, user *User) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	if strings.TrimSpace(user.Status) == "" {
		user.Status = "active"
	}
	var nextNumericID int64
	for _, existing := range m.usersOrdered {
		if existing.NumericID > nextNumericID {
			nextNumericID = existing.NumericID
		}
	}
	user.NumericID = nextNumericID + 1
	email := strings.ToLower(user.Email)
	username := strings.ToLower(user.Username)
	m.users[email] = user
	m.usersByUsername[username] = user
	m.usersOrdered = append(m.usersOrdered, user)
	return nil
}

func (m *Memory) GetUserByID(ctx context.Context, id string) (*User, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.usersOrdered {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *Memory) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if u, ok := m.users[strings.ToLower(email)]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if u, ok := m.usersByUsername[strings.ToLower(username)]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) GetUserByLogin(ctx context.Context, login string) (*User, error) {
	if strings.Contains(login, "@") {
		return m.GetUserByEmail(ctx, login)
	}
	return m.GetUserByUsername(ctx, login)
}

func (m *Memory) ListUsers(ctx context.Context, limit int) ([]*User, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*User
	for i, u := range m.usersOrdered {
		if limit > 0 && i >= limit {
			break
		}
		cp := *u
		res = append(res, &cp)
	}
	return res, nil
}

func (m *Memory) CountUsers(ctx context.Context) (int, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.usersOrdered), nil
}

func (m *Memory) UpdateUserLastLogin(ctx context.Context, id string, lastLoginAt time.Time, ip string, location string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.LastLoginAt = &lastLoginAt
			u.LastLoginIP = ip
			u.LastLoginLocation = location
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *Memory) UpdateUserStatus(ctx context.Context, id string, status string) error {
	_ = ctx
	status = strings.TrimSpace(status)
	if status == "" {
		return sql.ErrNoRows
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.Status = status
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *Memory) UpdateUserRole(ctx context.Context, id string, role string) error {
	_ = ctx
	role = strings.TrimSpace(role)
	if role == "" {
		return sql.ErrNoRows
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.Role = role
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *Memory) UpdateUserGroupID(ctx context.Context, id, groupID string) error {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return sql.ErrNoRows
	}
	groupID = strings.TrimSpace(groupID)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.GroupID = groupID
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *Memory) UpdateUserEmail(ctx context.Context, id, email string) error {
	_ = ctx
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return sql.ErrNoRows
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var target *User
	var oldKey string
	for key, u := range m.users {
		if u.ID == id {
			target = u
			oldKey = key
			break
		}
	}
	if target == nil {
		return sql.ErrNoRows
	}
	if oldKey != email {
		if _, exists := m.users[email]; exists {
			return fmt.Errorf("email already exists")
		}
		delete(m.users, oldKey)
		target.Email = email
		m.users[email] = target
	} else {
		target.Email = email
	}
	target.UpdatedAt = time.Now()
	return nil
}

func (m *Memory) UpdateUserUsername(ctx context.Context, id, username string) error {
	_ = ctx
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return sql.ErrNoRows
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var target *User
	var oldKey string
	for key, u := range m.usersByUsername {
		if u.ID == id {
			target = u
			oldKey = key
			break
		}
	}
	if target == nil {
		return sql.ErrNoRows
	}
	if oldKey != username {
		if _, exists := m.usersByUsername[username]; exists {
			return fmt.Errorf("username already exists")
		}
		delete(m.usersByUsername, oldKey)
		target.Username = username
		m.usersByUsername[username] = target
	} else {
		target.Username = username
	}
	target.UpdatedAt = time.Now()
	return nil
}

func (m *Memory) UpdateUserPasswordHash(ctx context.Context, id string, passwordHash string) error {
	_ = ctx
	passwordHash = strings.TrimSpace(passwordHash)
	if passwordHash == "" {
		return sql.ErrNoRows
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.PasswordHash = passwordHash
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *Memory) DeleteUser(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var emailKey, userKey string
	for e, u := range m.users {
		if u.ID == id {
			emailKey = e
			userKey = strings.ToLower(u.Username)
			break
		}
	}
	if emailKey != "" {
		delete(m.users, emailKey)
	}
	if userKey != "" {
		delete(m.usersByUsername, userKey)
	}
	for i, u := range m.usersOrdered {
		if u.ID == id {
			m.usersOrdered = append(m.usersOrdered[:i], m.usersOrdered[i+1:]...)
			break
		}
	}
	return nil
}

// Node operations
func (m *Memory) CreateNode(ctx context.Context, node *Node) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(node.MonitorProtocol) == "" {
		node.MonitorProtocol = "http"
	}
	if node.MonitorTimeout == 0 {
		node.MonitorTimeout = 5
	}
	if node.MonitorPort == 0 {
		node.MonitorPort = 80
	}
	if node.MonitorFailThreshold == 0 {
		node.MonitorFailThreshold = 3
	}
	now := time.Now()
	node.CreatedAt = now
	node.UpdatedAt = now
	m.nodes[node.ID] = node
	return nil
}

func (m *Memory) GetNode(ctx context.Context, id string) (*Node, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if n, ok := m.nodes[id]; ok {
		cp := *n
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) GetNodeByHostname(ctx context.Context, hostname string) (*Node, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, n := range m.nodes {
		if n.Hostname == hostname {
			cp := *n
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *Memory) ListNodes(ctx context.Context) ([]*Node, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*Node
	for _, n := range m.nodes {
		cp := *n
		res = append(res, &cp)
	}
	return res, nil
}

func (m *Memory) CountNodes(ctx context.Context) (int, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodes), nil
}

func (m *Memory) UpdateNodeStatus(ctx context.Context, id, status, configVersion string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.nodes[id]; ok {
		n.Status = status
		n.ConfigVersion = configVersion
		n.LastHeartbeat = time.Now()
		n.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) UpdateNodeToken(ctx context.Context, id, tokenHash string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.nodes[id]; ok {
		n.Token = tokenHash
		n.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) UpdateNodeHeartbeatInfo(ctx context.Context, id, publicIP, version, region string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[id]
	if !ok {
		return nil
	}
	now := time.Now()
	if publicIP != "" {
		n.PublicIP = publicIP
	}
	if version != "" {
		n.Version = version
	}
	if region != "" {
		n.Region = region
	}
	n.LastHeartbeat = now
	n.UpdatedAt = now
	return nil
}

func (m *Memory) UpdateNode(ctx context.Context, node *Node) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.nodes[node.ID]; ok {
		if node.Hostname != "" {
			n.Hostname = node.Hostname
		}
		if node.PublicIP != "" {
			n.PublicIP = node.PublicIP
		}
		if node.Version != "" {
			n.Version = node.Version
		}
		if node.Status != "" {
			n.Status = node.Status
		}
		if node.Region != "" {
			n.Region = node.Region
		}
		if node.Cluster != "" {
			n.Cluster = node.Cluster
		}
		if len(node.Capabilities) > 0 {
			n.Capabilities = node.Capabilities
		}
		if node.ConfigVersion != "" {
			n.ConfigVersion = node.ConfigVersion
		}
		if node.Token != "" {
			n.Token = node.Token
		}
		n.UpdatedAt = time.Now()
	}
	return nil
}

// RegisterOrRefreshNode mirrors the Postgres UPSERT-by-hostname semantics under
// the in-memory store's single mutex. The whole operation runs under m.mu so
// concurrent callers with the same hostname see serialized behaviour (matching
// the Postgres UNIQUE(hostname) constraint).
func (m *Memory) RegisterOrRefreshNode(ctx context.Context, node *Node) (string, error) {
	_ = ctx
	if node == nil {
		return "", errors.New("node is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Look up an existing row by hostname.
	var existing *Node
	for _, n := range m.nodes {
		if n.Hostname == node.Hostname {
			existing = n
			break
		}
	}

	if existing == nil {
		// Insert path — mirror CreateNode defaults without re-acquiring the lock.
		if strings.TrimSpace(node.MonitorProtocol) == "" {
			node.MonitorProtocol = "http"
		}
		if node.MonitorTimeout == 0 {
			node.MonitorTimeout = 5
		}
		if node.MonitorPort == 0 {
			node.MonitorPort = 80
		}
		if node.MonitorFailThreshold == 0 {
			node.MonitorFailThreshold = 3
		}
		now := time.Now()
		if node.CreatedAt.IsZero() {
			node.CreatedAt = now
		}
		node.UpdatedAt = now
		m.nodes[node.ID] = node
		return node.ID, nil
	}

	// Refresh path — disabled nodes are locked out.
	if strings.EqualFold(existing.Status, "disabled") {
		return "", ErrNodeDisabled
	}
	existing.PublicIP = node.PublicIP
	existing.Version = node.Version
	existing.Status = "online"
	existing.Region = node.Region
	existing.Capabilities = node.Capabilities
	if node.Token != "" {
		existing.Token = node.Token
	}
	existing.UpdatedAt = time.Now()
	return existing.ID, nil
}

func (m *Memory) UpdateNodeMonitorConfig(ctx context.Context, id string, cfg NodeMonitorConfig) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.nodes[id]; ok {
		if strings.TrimSpace(cfg.Protocol) == "" {
			cfg.Protocol = "http"
		}
		if cfg.TimeoutSeconds <= 0 {
			cfg.TimeoutSeconds = 5
		}
		if cfg.Port <= 0 {
			cfg.Port = 80
		}
		if cfg.FailThreshold <= 0 {
			cfg.FailThreshold = 3
		}
		n.MonitorEnabled = cfg.Enabled
		n.MonitorProtocol = cfg.Protocol
		n.MonitorTimeout = cfg.TimeoutSeconds
		n.MonitorPort = cfg.Port
		n.MonitorFailThreshold = cfg.FailThreshold
		n.MonitorFailCount = 0
		n.MonitorLastOK = false
		n.MonitorLastError = ""
		n.MonitorLastAt = time.Time{}
		n.MonitorLastLatencyMs = 0
		n.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) UpdateNodeMonitorResult(ctx context.Context, id string, res NodeMonitorResult) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.nodes[id]; ok {
		n.MonitorLastOK = res.LastOK
		n.MonitorLastError = res.LastError
		n.MonitorLastAt = res.LastAt
		n.MonitorLastLatencyMs = res.LastLatencyMs
		n.MonitorFailCount = res.FailCount
		n.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) UpdateNodeTelemetry(ctx context.Context, id string, t NodeTelemetry) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[id]
	if !ok || n == nil {
		return nil
	}

	now := time.Now()
	deltaBytesSent := int64(0)
	deltaBytesRecv := int64(0)
	if n.BytesSent > 0 && t.BytesSent >= n.BytesSent {
		deltaBytesSent = t.BytesSent - n.BytesSent
	}
	if n.BytesReceived > 0 && t.BytesReceived >= n.BytesReceived {
		deltaBytesRecv = t.BytesReceived - n.BytesReceived
	}

	if !n.LastMetricsAt.IsZero() {
		secs := now.Sub(n.LastMetricsAt).Seconds()
		if secs > 0 {
			n.BandwidthUpBps = float64(deltaBytesSent) / secs
			n.BandwidthDownBps = float64(deltaBytesRecv) / secs
		}
		if n.LastMetricsAt.Year() == now.Year() && n.LastMetricsAt.Month() == now.Month() {
			n.MonthBytesSent += deltaBytesSent
		} else {
			n.MonthBytesSent = deltaBytesSent
		}
	} else {
		n.MonthBytesSent = deltaBytesSent
	}

	n.CPUUsage = t.CPUUsage
	n.MemUsage = t.MemUsage
	n.DiskUsage = t.DiskUsage
	n.CPUCount = t.CPUCount
	n.MemTotal = t.MemTotal
	n.DiskTotal = t.DiskTotal
	n.BytesSent = t.BytesSent
	n.BytesReceived = t.BytesReceived
	n.TCPEstablished = t.TCPEstablished
	n.TCPSynRecv = t.TCPSynRecv
	n.TCPTimeWait = t.TCPTimeWait
	n.NginxRunning = t.NginxRunning
	n.RequestsTotal = t.RequestsTotal
	n.CacheHits = t.CacheHits
	n.CacheMisses = t.CacheMisses
	n.LastMetricsAt = now
	n.UpdatedAt = now
	return nil
}

func (m *Memory) DeleteNode(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nodes, id)
	delete(m.nodeMetricSamples, id)
	// Mirror the Postgres ON DELETE CASCADE on cluster_nodes(node_id):
	// remove this node from every cluster/line it is a member of so
	// memory-backed tests observe the same behavior as production.
	for clusterID, lineMap := range m.clusterNodes {
		for line, nodes := range lineMap {
			if _, ok := nodes[id]; ok {
				delete(nodes, id)
				if len(nodes) == 0 {
					delete(lineMap, line)
				}
			}
		}
		if len(lineMap) == 0 {
			delete(m.clusterNodes, clusterID)
		}
	}
	return nil
}

// Cluster nodes

func (m *Memory) ListClusterNodes(ctx context.Context, clusterID, line string) ([]*ClusterNode, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	group := m.clusterNodes[clusterID]
	out := make([]*ClusterNode, 0)
	line = strings.TrimSpace(line)
	if line == "default" {
		line = "默认"
	}
	if line == "" || line == "all" {
		for _, lineMap := range group {
			for _, v := range lineMap {
				cp := *v
				out = append(out, &cp)
			}
		}
	} else {
		lineMap := group[line]
		for _, v := range lineMap {
			cp := *v
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) UpsertClusterNode(ctx context.Context, n *ClusterNode) error {
	_ = ctx
	if n == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if n.Weight <= 0 {
		n.Weight = 1
	}
	n.Line = strings.TrimSpace(n.Line)
	if n.Line == "" || n.Line == "default" {
		n.Line = "默认"
	}
	now := time.Now()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	n.UpdatedAt = now
	group := m.clusterNodes[n.ClusterID]
	if group == nil {
		group = make(map[string]map[string]*ClusterNode)
		m.clusterNodes[n.ClusterID] = group
	}
	lineMap := group[n.Line]
	if lineMap == nil {
		lineMap = make(map[string]*ClusterNode)
		group[n.Line] = lineMap
	}
	cp := *n
	lineMap[n.NodeID] = &cp
	return nil
}

func (m *Memory) DeleteClusterNode(ctx context.Context, clusterID, line, nodeID string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	group := m.clusterNodes[clusterID]
	if group == nil {
		return nil
	}
	line = strings.TrimSpace(line)
	if line == "" || line == "default" {
		line = "默认"
	}
	lineMap := group[line]
	if lineMap == nil {
		return nil
	}
	delete(lineMap, nodeID)
	return nil
}

// Domain operations
func (m *Memory) CreateDomain(ctx context.Context, domain *Domain) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	domain.CreatedAt = now
	domain.UpdatedAt = now
	domain.ErrorPages = copyErrorPages(domain.ErrorPages)
	domain.Security = copyDomainSecurity(domain.Security)
	domain.OriginHealthCheck = copyOriginHealthCheck(domain.OriginHealthCheck)
	domain.LoadBalanceMethod = normalizeLoadBalanceMethod(domain.LoadBalanceMethod)
	if strings.TrimSpace(domain.OriginScheme) == "" {
		domain.OriginScheme = "http"
	}
	if domain.OriginPort <= 0 {
		domain.OriginPort = 80
	}
	domain.ListenPort = normalizeDomainListenPort(domain.ListenPort)
	if strings.TrimSpace(domain.OriginHostMode) == "" {
		domain.OriginHostMode = "request_host"
	}
	if domain.OriginTimeoutMs <= 0 {
		domain.OriginTimeoutMs = 60000
	}
	if domain.OriginConnectTimeoutMs <= 0 {
		domain.OriginConnectTimeoutMs = 10000
	}
	m.domains[domain.ID] = domain
	return nil
}

func (m *Memory) GetDomain(ctx context.Context, id string) (*Domain, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if d, ok := m.domains[id]; ok {
		cp := *d
		cp.ErrorPages = copyErrorPages(d.ErrorPages)
		cp.Security = copyDomainSecurity(d.Security)
		cp.OriginHealthCheck = copyOriginHealthCheck(d.OriginHealthCheck)
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) GetDomainByName(ctx context.Context, name string) (*Domain, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, d := range m.domains {
		if d.Name == name {
			cp := *d
			cp.ErrorPages = copyErrorPages(d.ErrorPages)
			cp.Security = copyDomainSecurity(d.Security)
			cp.OriginHealthCheck = copyOriginHealthCheck(d.OriginHealthCheck)
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *Memory) ListDomains(ctx context.Context) ([]*Domain, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*Domain
	for _, d := range m.domains {
		cp := *d
		cp.ErrorPages = copyErrorPages(d.ErrorPages)
		cp.Security = copyDomainSecurity(d.Security)
		cp.OriginHealthCheck = copyOriginHealthCheck(d.OriginHealthCheck)
		res = append(res, &cp)
	}
	return res, nil
}

func (m *Memory) CountDomains(ctx context.Context) (int, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.domains), nil
}

func (m *Memory) CountDomainsByUser(ctx context.Context, userID string) (int, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, d := range m.domains {
		if d.UserID == userID {
			n++
		}
	}
	return n, nil
}

func (m *Memory) ListDomainsByUser(ctx context.Context, userID string) ([]*Domain, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*Domain
	for _, d := range m.domains {
		if d.UserID == userID {
			cp := *d
			cp.ErrorPages = copyErrorPages(d.ErrorPages)
			cp.Security = copyDomainSecurity(d.Security)
			cp.OriginHealthCheck = copyOriginHealthCheck(d.OriginHealthCheck)
			res = append(res, &cp)
		}
	}
	return res, nil
}

func (m *Memory) UpdateDomain(ctx context.Context, domain *Domain) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.domains[domain.ID]; ok {
		if domain.Name != "" {
			d.Name = domain.Name
		}
		if domain.CNAME != "" {
			d.CNAME = domain.CNAME
		}
		d.LineGroupID = domain.LineGroupID
		d.OriginID = domain.OriginID
		if domain.CertID != "" {
			d.CertID = domain.CertID
		}
		d.ListenPort = normalizeDomainListenPort(domain.ListenPort)
		if strings.TrimSpace(domain.OriginScheme) != "" {
			d.OriginScheme = domain.OriginScheme
		}
		if domain.OriginPort > 0 {
			d.OriginPort = domain.OriginPort
		}
		if domain.OriginHostMode != "" {
			d.OriginHostMode = domain.OriginHostMode
		}
		if domain.OriginHost != "" || domain.OriginHostMode == "custom" {
			d.OriginHost = domain.OriginHost
		}
		if domain.OriginTimeoutMs > 0 {
			d.OriginTimeoutMs = domain.OriginTimeoutMs
		}
		if domain.OriginConnectTimeoutMs > 0 {
			d.OriginConnectTimeoutMs = domain.OriginConnectTimeoutMs
		}
		if domain.ErrorPages != nil {
			d.ErrorPages = copyErrorPages(domain.ErrorPages)
		}
		// Security is treated as a full replace when provided — the
		// handler always sends the current snapshot, never partial
		// patches. nil means "leave as-is", which lets /api/domains
		// PATCH paths that don't touch security keep the old value.
		if domain.Security != nil {
			d.Security = copyDomainSecurity(domain.Security)
		}
		if domain.OriginAuth != nil {
			cp := *domain.OriginAuth
			if domain.OriginAuth.Headers != nil {
				cp.Headers = make([]OriginAuthHeader, len(domain.OriginAuth.Headers))
				copy(cp.Headers, domain.OriginAuth.Headers)
			}
			d.OriginAuth = &cp
		}
		// LoadBalanceMethod / OriginHealthCheck are full-replace fields:
		// the handler always sends the current snapshot of the form, so
		// blank/zero values legitimately mean "reset to defaults".
		d.LoadBalanceMethod = normalizeLoadBalanceMethod(domain.LoadBalanceMethod)
		if domain.OriginHealthCheck != nil {
			d.OriginHealthCheck = copyOriginHealthCheck(domain.OriginHealthCheck)
		}
		d.HTTPSEnabled = domain.HTTPSEnabled
		d.HTTP2Enabled = domain.HTTP2Enabled
		d.WebsocketEnabled = domain.WebsocketEnabled
		d.CacheEnabled = domain.CacheEnabled
		d.Enabled = domain.Enabled
		d.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) DeleteDomain(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.domains, id)
	return nil
}

// Origin operations
func (m *Memory) CreateOrigin(ctx context.Context, origin *Origin) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	origin.CreatedAt = now
	origin.UpdatedAt = now
	m.origins[origin.ID] = origin
	return nil
}

func (m *Memory) GetOrigin(ctx context.Context, id string) (*Origin, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if o, ok := m.origins[id]; ok {
		cp := *o
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) ListOrigins(ctx context.Context) ([]*Origin, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*Origin
	for _, o := range m.origins {
		cp := *o
		res = append(res, &cp)
	}
	return res, nil
}

func (m *Memory) UpdateOrigin(ctx context.Context, origin *Origin) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if o, ok := m.origins[origin.ID]; ok {
		*o = *origin
		o.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) DeleteOrigin(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.origins, id)
	return nil
}

// DomainOrigin operations

func (m *Memory) ListDomainOrigins(ctx context.Context, domainID string) ([]*DomainOrigin, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	src := m.domainOrigins[domainID]
	out := make([]*DomainOrigin, 0, len(src))
	for _, e := range src {
		cp := *e
		out = append(out, &cp)
	}
	return out, nil
}

func (m *Memory) ListAllDomainOrigins(ctx context.Context) ([]*DomainOrigin, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*DomainOrigin
	for _, list := range m.domainOrigins {
		for _, e := range list {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *Memory) ReplaceDomainOrigins(ctx context.Context, domainID string, entries []*DomainOrigin) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	copied := make([]*DomainOrigin, 0, len(entries))
	for i, e := range entries {
		cp := *e
		cp.DomainID = domainID
		if cp.ID == "" {
			cp.ID = fmt.Sprintf("%s:%d", domainID, i)
		}
		if cp.Weight <= 0 {
			cp.Weight = 1
		}
		if cp.Weight > 100 {
			cp.Weight = 100
		}
		if cp.SortOrder == 0 {
			cp.SortOrder = int32(i)
		}
		if cp.CreatedAt.IsZero() {
			cp.CreatedAt = now
		}
		cp.UpdatedAt = now
		copied = append(copied, &cp)
	}
	m.domainOrigins[domainID] = copied
	return nil
}

func (m *Memory) DeleteDomainOrigins(ctx context.Context, domainID string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.domainOrigins, domainID)
	return nil
}

// StreamForward operations

func (m *Memory) ListStreamForwards(ctx context.Context, userID string) ([]*StreamForward, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*StreamForward
	for _, sf := range m.streamForwards {
		if sf.UserID == userID {
			cp := *sf
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ListenPort != out[j].ListenPort {
			return out[i].ListenPort < out[j].ListenPort
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Memory) ListStreamForwardsByDomain(ctx context.Context, domainID string) ([]*StreamForward, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*StreamForward
	for _, sf := range m.streamForwards {
		if sf.DomainID == domainID {
			cp := *sf
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ListenPort != out[j].ListenPort {
			return out[i].ListenPort < out[j].ListenPort
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Memory) ListAllStreamForwards(ctx context.Context) ([]*StreamForward, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*StreamForward, 0, len(m.streamForwards))
	for _, sf := range m.streamForwards {
		cp := *sf
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UserID != out[j].UserID {
			return out[i].UserID < out[j].UserID
		}
		if out[i].ListenPort != out[j].ListenPort {
			return out[i].ListenPort < out[j].ListenPort
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Memory) GetStreamForward(ctx context.Context, id string) (*StreamForward, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	sf, ok := m.streamForwards[id]
	if !ok {
		return nil, nil
	}
	cp := *sf
	return &cp, nil
}

func (m *Memory) CreateStreamForward(ctx context.Context, sf *StreamForward) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cp := *sf
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	m.streamForwards[cp.ID] = &cp
	return nil
}

func (m *Memory) UpdateStreamForward(ctx context.Context, sf *StreamForward) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.streamForwards[sf.ID]; !ok {
		return fmt.Errorf("stream forward not found")
	}
	cp := *sf
	cp.UpdatedAt = time.Now()
	m.streamForwards[cp.ID] = &cp
	return nil
}

func (m *Memory) DeleteStreamForward(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.streamForwards, id)
	return nil
}

func (m *Memory) CountEnabledStreamForwardsByUser(ctx context.Context, userID string) (int, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, sf := range m.streamForwards {
		if sf.UserID == userID && sf.Enabled {
			n++
		}
	}
	return n, nil
}

// Certificate operations
func (m *Memory) CreateCertificate(ctx context.Context, cert *Certificate) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cert.CreatedAt = now
	cert.UpdatedAt = now
	m.certSeq++
	cert.ID = m.certSeq
	m.certs[cert.ID] = cert
	return nil
}

func (m *Memory) GetCertificate(ctx context.Context, id int64) (*Certificate, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if c, ok := m.certs[id]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) GetCertificateByDomain(ctx context.Context, domain string) (*Certificate, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var best *Certificate
	for _, c := range m.certs {
		if c.Domain == domain {
			if best == nil || c.ExpiresAt.After(best.ExpiresAt) {
				cp := *c
				best = &cp
			}
		}
	}
	return best, nil
}

func (m *Memory) ListCertificates(ctx context.Context) ([]*Certificate, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*Certificate
	for _, c := range m.certs {
		cp := *c
		res = append(res, &cp)
	}
	return res, nil
}

func (m *Memory) ListCertificatesByUser(ctx context.Context, userID string) ([]*Certificate, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*Certificate
	for _, c := range m.certs {
		if c.UserID == userID {
			cp := *c
			res = append(res, &cp)
		}
	}
	return res, nil
}

func (m *Memory) UpdateCertificate(ctx context.Context, cert *Certificate) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.certs[cert.ID]; ok {
		*c = *cert
		c.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) DeleteCertificate(ctx context.Context, id int64) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.certs, id)
	return nil
}

// Config versions
func (m *Memory) CreateConfigVersion(ctx context.Context, cv *ConfigVersion) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if cv.CreatedAt.IsZero() {
		cv.CreatedAt = time.Now()
	}
	m.configVersions[cv.Version] = cv
	m.latestConfigVer = cv.Version
	return nil
}

func (m *Memory) GetConfigVersion(ctx context.Context, version string) (*ConfigVersion, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if cv, ok := m.configVersions[version]; ok {
		cp := *cv
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) GetLatestConfigVersion(ctx context.Context) (*ConfigVersion, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if cv, ok := m.configVersions[m.latestConfigVer]; ok {
		cp := *cv
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) ListConfigVersions(ctx context.Context, limit int) ([]*ConfigVersion, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*ConfigVersion
	count := 0
	for _, cv := range m.configVersions {
		cp := *cv
		res = append(res, &cp)
		count++
		if limit > 0 && count >= limit {
			break
		}
	}
	return res, nil
}

// Cache rules
func (m *Memory) CreateCacheRule(ctx context.Context, rule *CacheRule) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	m.cacheRules[rule.ID] = rule
	return nil
}

func (m *Memory) GetCacheRule(ctx context.Context, id string) (*CacheRule, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if r, ok := m.cacheRules[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) ListCacheRules(ctx context.Context) ([]*CacheRule, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*CacheRule
	for _, r := range m.cacheRules {
		cp := *r
		res = append(res, &cp)
	}
	return res, nil
}

func (m *Memory) UpdateCacheRule(ctx context.Context, rule *CacheRule) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.cacheRules[rule.ID]; ok {
		*r = *rule
		r.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) DeleteCacheRule(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cacheRules, id)
	return nil
}

// Token validation
func (m *Memory) ValidateServiceToken(ctx context.Context, token string) (bool, error) {
	_ = ctx
	if m.serviceToken == "" {
		return true, nil
	}
	return token == m.serviceToken, nil
}

func (m *Memory) ValidateBootstrapToken(ctx context.Context, token string) (bool, error) {
	_ = ctx
	if m.bootstrapToken == "" {
		return true, nil
	}
	return token == m.bootstrapToken, nil
}

func (m *Memory) CreateBootstrapToken(ctx context.Context, description string, ttl time.Duration) (string, time.Time, error) {
	_ = ctx
	_ = description
	token := generateToken()
	m.bootstrapToken = token
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	return token, exp, nil
}

// License state
func (m *Memory) SetLicenseState(ctx context.Context, st *LicenseState) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if st == nil {
		m.license = nil
		return nil
	}
	cp := *st
	m.license = &cp
	return nil
}

func (m *Memory) GetLicenseState(ctx context.Context) (*LicenseState, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.license == nil {
		return nil, nil
	}
	cp := *m.license
	cp.PubKey = m.pubKey
	return &cp, nil
}

// Settings
func (m *Memory) GetSettings(ctx context.Context) (*Settings, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.settings == nil {
		return nil, nil
	}
	cp := *m.settings
	return &cp, nil
}

func (m *Memory) UpdateSettings(ctx context.Context, s *Settings) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if s == nil {
		m.settings = nil
		return nil
	}
	if s.ID == "" {
		s.ID = "default"
	}
	cp := *s
	cp.UpdatedAt = time.Now()
	m.settings = &cp
	return nil
}

func (m *Memory) GetBalanceAccount(ctx context.Context, userID string) (*BalanceAccount, error) {
	_ = ctx
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	m.mu.RLock()
	a := m.balanceAccounts[userID]
	m.mu.RUnlock()
	if a == nil {
		return &BalanceAccount{UserID: userID, BalanceCents: 0, Currency: "CNY"}, nil
	}
	cp := *a
	if strings.TrimSpace(cp.Currency) == "" {
		cp.Currency = "CNY"
	}
	return &cp, nil
}

func (m *Memory) ListBalanceTransactions(ctx context.Context, userID string, page, pageSize int) ([]*BalanceTransaction, int64, error) {
	_ = ctx
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return []*BalanceTransaction{}, 0, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	m.mu.RLock()
	list := m.balanceTransactions[userID]
	m.mu.RUnlock()
	total := int64(len(list))
	start := (page - 1) * pageSize
	if start >= len(list) {
		return []*BalanceTransaction{}, total, nil
	}
	end := start + pageSize
	if end > len(list) {
		end = len(list)
	}
	res := make([]*BalanceTransaction, 0, end-start)
	for _, t := range list[start:end] {
		cp := *t
		res = append(res, &cp)
	}
	return res, total, nil
}

func (m *Memory) AdminListBalanceAccounts(ctx context.Context, userID string, page, pageSize int) ([]*BalanceAccount, int64, error) {
	_ = ctx
	userID = strings.TrimSpace(userID)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	m.mu.RLock()
	all := make([]*BalanceAccount, 0, len(m.balanceAccounts))
	for _, a := range m.balanceAccounts {
		if a == nil {
			continue
		}
		if userID != "" && a.UserID != userID {
			continue
		}
		cp := *a
		if strings.TrimSpace(cp.Currency) == "" {
			cp.Currency = "CNY"
		}
		all = append(all, &cp)
	}
	m.mu.RUnlock()
	sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	total := int64(len(all))
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []*BalanceAccount{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

func (m *Memory) CreateBalanceRecharge(ctx context.Context, r *BalanceRecharge) error {
	_ = ctx
	if r == nil || r.ID == "" || r.UserID == "" || r.OutTradeNo == "" {
		return fmt.Errorf("invalid recharge")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balanceRecharges[r.ID] = r
	return nil
}

func (m *Memory) GetBalanceRechargeByOutTradeNo(ctx context.Context, outTradeNo string) (*BalanceRecharge, error) {
	_ = ctx
	outTradeNo = strings.TrimSpace(outTradeNo)
	if outTradeNo == "" {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.balanceRecharges {
		if r != nil && r.OutTradeNo == outTradeNo {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *Memory) AdminListBalanceRecharges(ctx context.Context, userID, status string, page, pageSize int) ([]*BalanceRecharge, int64, error) {
	_ = ctx
	userID = strings.TrimSpace(userID)
	status = strings.TrimSpace(status)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	m.mu.RLock()
	all := make([]*BalanceRecharge, 0, len(m.balanceRecharges))
	for _, r := range m.balanceRecharges {
		if r == nil {
			continue
		}
		if userID != "" && r.UserID != userID {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		cp := *r
		all = append(all, &cp)
	}
	m.mu.RUnlock()
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	total := int64(len(all))
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []*BalanceRecharge{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

func (m *Memory) UpdateBalanceRechargePayment(ctx context.Context, id, payURL, qrCode, formHTML string) error {
	_ = ctx
	_ = formHTML
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.balanceRecharges[id]
	if r == nil {
		return nil
	}
	if payURL != "" {
		r.PaymentURL = payURL
	}
	if qrCode != "" {
		r.QRCode = qrCode
	}
	r.UpdatedAt = time.Now()
	return nil
}

func (m *Memory) AdminUpdateBalanceRecharge(ctx context.Context, id, status, tradeNo, notifyRaw string, paidAt time.Time) error {
	_ = ctx
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" || status == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.balanceRecharges[id]
	if r == nil {
		return nil
	}
	if status == "paid" && paidAt.IsZero() {
		paidAt = time.Now()
	}
	if r.Status == "pending" && status == "paid" {
		a := m.balanceAccounts[r.UserID]
		if a == nil {
			a = &BalanceAccount{UserID: r.UserID, BalanceCents: 0, Currency: "CNY", UpdatedAt: time.Now()}
			m.balanceAccounts[r.UserID] = a
		}
		a.BalanceCents += r.AmountCents
		a.UpdatedAt = time.Now()
		t := &BalanceTransaction{
			ID:           generateID(),
			UserID:       r.UserID,
			Type:         "recharge",
			AmountCents:  r.AmountCents,
			BalanceCents: a.BalanceCents,
			Note:         "recharge paid",
			RefType:      "recharge",
			RefID:        r.ID,
			CreatedAt:    paidAt,
		}
		m.balanceTransactions[r.UserID] = append([]*BalanceTransaction{t}, m.balanceTransactions[r.UserID]...)
	}
	r.Status = status
	if tradeNo != "" {
		r.TradeNo = tradeNo
	}
	if notifyRaw != "" {
		r.NotifyRaw = notifyRaw
	}
	if status == "paid" {
		r.PaidAt = paidAt
	}
	if status == "closed" || status == "cancelled" {
		r.ClosedAt = time.Now()
	}
	r.UpdatedAt = time.Now()
	return nil
}

func (m *Memory) AdminListBalanceWithdrawals(ctx context.Context, userID, status string, page, pageSize int) ([]*BalanceWithdrawal, int64, error) {
	_ = ctx
	userID = strings.TrimSpace(userID)
	status = strings.TrimSpace(status)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	m.mu.RLock()
	all := make([]*BalanceWithdrawal, 0, len(m.balanceWithdrawals))
	for _, w := range m.balanceWithdrawals {
		if w == nil {
			continue
		}
		if userID != "" && w.UserID != userID {
			continue
		}
		if status != "" && w.Status != status {
			continue
		}
		cp := *w
		all = append(all, &cp)
	}
	m.mu.RUnlock()
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	total := int64(len(all))
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []*BalanceWithdrawal{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

func (m *Memory) CreateBalanceWithdrawal(ctx context.Context, w *BalanceWithdrawal) error {
	_ = ctx
	if w == nil || w.ID == "" || w.UserID == "" {
		return fmt.Errorf("invalid withdrawal")
	}
	if w.AmountCents <= 0 {
		return fmt.Errorf("invalid withdrawal amount")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.balanceAccounts[w.UserID]
	if a == nil || a.BalanceCents < w.AmountCents {
		return errors.New("insufficient balance")
	}
	a.BalanceCents -= w.AmountCents
	a.UpdatedAt = time.Now()
	cp := *w
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = cp.CreatedAt
	}
	if cp.Status == "" {
		cp.Status = "pending"
	}
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	t := &BalanceTransaction{
		ID:           generateID(),
		UserID:       w.UserID,
		Type:         "withdraw",
		AmountCents:  -w.AmountCents,
		BalanceCents: a.BalanceCents,
		Note:         "withdrawal reserved (pending review)",
		RefType:      "withdrawal",
		RefID:        cp.ID,
		CreatedAt:    cp.CreatedAt,
	}
	m.balanceTransactions[w.UserID] = append([]*BalanceTransaction{t}, m.balanceTransactions[w.UserID]...)
	m.balanceWithdrawals[cp.ID] = &cp
	return nil
}

func (m *Memory) withdrawalHeldLocked(w *BalanceWithdrawal) bool {
	if w == nil {
		return false
	}
	for _, txs := range m.balanceTransactions {
		for _, t := range txs {
			if t != nil && t.RefType == "withdrawal" && t.RefID == w.ID && t.AmountCents < 0 {
				return true
			}
		}
	}
	return false
}

func (m *Memory) AdminUpdateBalanceWithdrawal(ctx context.Context, id, status, note string, reviewedAt time.Time) error {
	_ = ctx
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" || status == "" {
		return nil
	}
	if reviewedAt.IsZero() {
		reviewedAt = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.balanceWithdrawals[id]
	if w == nil {
		return nil
	}
	held := m.withdrawalHeldLocked(w)
	if w.Status == "pending" && (status == "approved" || status == "paid") {
		if !held {
			a := m.balanceAccounts[w.UserID]
			if a == nil || a.BalanceCents < w.AmountCents {
				return errors.New("insufficient balance")
			}
			a.BalanceCents -= w.AmountCents
			a.UpdatedAt = time.Now()
			t := &BalanceTransaction{
				ID:           generateID(),
				UserID:       w.UserID,
				Type:         "withdraw",
				AmountCents:  -w.AmountCents,
				BalanceCents: a.BalanceCents,
				Note:         note,
				RefType:      "withdrawal",
				RefID:        w.ID,
				CreatedAt:    reviewedAt,
			}
			m.balanceTransactions[w.UserID] = append([]*BalanceTransaction{t}, m.balanceTransactions[w.UserID]...)
		}
	}
	if w.Status == "pending" && (status == "rejected" || status == "cancelled" || status == "failed") && held {
		a := m.balanceAccounts[w.UserID]
		if a == nil {
			a = &BalanceAccount{UserID: w.UserID, BalanceCents: 0, Currency: "CNY", UpdatedAt: time.Now()}
			m.balanceAccounts[w.UserID] = a
		}
		a.BalanceCents += w.AmountCents
		a.UpdatedAt = time.Now()
		refundNote := strings.TrimSpace(note)
		if refundNote == "" {
			refundNote = "withdrawal rejected, balance restored"
		}
		t := &BalanceTransaction{
			ID:           generateID(),
			UserID:       w.UserID,
			Type:         "adjust",
			AmountCents:  w.AmountCents,
			BalanceCents: a.BalanceCents,
			Note:         refundNote,
			RefType:      "withdrawal",
			RefID:        w.ID,
			CreatedAt:    reviewedAt,
		}
		m.balanceTransactions[w.UserID] = append([]*BalanceTransaction{t}, m.balanceTransactions[w.UserID]...)
	}
	w.Status = status
	w.Note = note
	w.ReviewedAt = reviewedAt
	w.UpdatedAt = time.Now()
	return nil
}

func (m *Memory) AdminAdjustBalance(ctx context.Context, userID string, amountCents int64, note string) error {
	_ = ctx
	userID = strings.TrimSpace(userID)
	if userID == "" || amountCents == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.balanceAccounts[userID]
	if a == nil {
		a = &BalanceAccount{UserID: userID, BalanceCents: 0, Currency: "CNY", UpdatedAt: time.Now()}
		m.balanceAccounts[userID] = a
	}
	next := a.BalanceCents + amountCents
	if next < 0 {
		return errors.New("insufficient balance")
	}
	a.BalanceCents = next
	a.UpdatedAt = time.Now()
	t := &BalanceTransaction{
		ID:           generateID(),
		UserID:       userID,
		Type:         "adjust",
		AmountCents:  amountCents,
		BalanceCents: next,
		Note:         note,
		RefType:      "adjust",
		RefID:        "",
		CreatedAt:    time.Now(),
	}
	m.balanceTransactions[userID] = append([]*BalanceTransaction{t}, m.balanceTransactions[userID]...)
	return nil
}

func (m *Memory) AdminRechargeStats(ctx context.Context, from, to time.Time) ([]*BalanceRechargeStats, error) {
	_ = ctx
	if from.IsZero() || to.IsZero() {
		return []*BalanceRechargeStats{}, nil
	}
	to = to.Add(24 * time.Hour)
	stats := map[string]*BalanceRechargeStats{}
	m.mu.RLock()
	for _, r := range m.balanceRecharges {
		if r == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(r.Status)) != "paid" {
			continue
		}
		ts := r.PaidAt
		if ts.IsZero() {
			ts = r.CreatedAt
		}
		if ts.Before(from) || !ts.Before(to) {
			continue
		}
		day := ts.Format("2006-01-02")
		s := stats[day]
		if s == nil {
			s = &BalanceRechargeStats{Day: day}
			stats[day] = s
		}
		s.RechargeCents += r.AmountCents
		s.RechargeCount++
		s.PaidCents += r.AmountCents
		s.PaidCount++
	}
	for _, userTxs := range m.balanceTransactions {
		for _, tx := range userTxs {
			if tx == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(tx.Type)) != "adjust" {
				continue
			}
			ts := tx.CreatedAt
			if ts.Before(from) || !ts.Before(to) {
				continue
			}
			day := ts.Format("2006-01-02")
			s := stats[day]
			if s == nil {
				s = &BalanceRechargeStats{Day: day}
				stats[day] = s
			}
			s.AdjustCents += tx.AmountCents
			s.AdjustCount++
		}
	}
	m.mu.RUnlock()
	res := make([]*BalanceRechargeStats, 0, len(stats))
	for _, v := range stats {
		v.TotalCents = v.RechargeCents + v.AdjustCents
		v.TotalCount = v.RechargeCount + v.AdjustCount
		res = append(res, v)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Day < res[j].Day })
	return res, nil
}

func (m *Memory) ListAnnouncements(ctx context.Context, status, q string, page, pageSize int) ([]*Announcement, int64, error) {
	_ = ctx
	status = strings.TrimSpace(status)
	q = strings.ToLower(strings.TrimSpace(q))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	m.mu.RLock()
	list := make([]*Announcement, 0, len(m.announcements))
	for _, a := range m.announcements {
		if a == nil {
			continue
		}
		if status != "" && strings.TrimSpace(a.Status) != status {
			continue
		}
		if q != "" {
			title := strings.ToLower(a.Title)
			content := strings.ToLower(a.Content)
			if !strings.Contains(title, q) && !strings.Contains(content, q) {
				continue
			}
		}
		cp := *a
		list = append(list, &cp)
	}
	m.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool {
		if list[i].Pinned != list[j].Pinned {
			return list[i].Pinned
		}
		return list[i].UpdatedAt.After(list[j].UpdatedAt)
	})
	total := int64(len(list))
	start := (page - 1) * pageSize
	if start >= len(list) {
		return []*Announcement{}, total, nil
	}
	end := start + pageSize
	if end > len(list) {
		end = len(list)
	}
	return list[start:end], total, nil
}

func (m *Memory) GetAnnouncement(ctx context.Context, id string) (*Announcement, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	a := m.announcements[strings.TrimSpace(id)]
	if a == nil {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (m *Memory) CreateAnnouncement(ctx context.Context, a *Announcement) error {
	_ = ctx
	if a == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.ID == "" {
		a.ID = generateID()
	}
	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	if strings.TrimSpace(a.Status) == "" {
		a.Status = "draft"
	}
	cp := *a
	m.announcements[cp.ID] = &cp
	return nil
}

func (m *Memory) UpdateAnnouncement(ctx context.Context, a *Announcement) error {
	_ = ctx
	if a == nil || strings.TrimSpace(a.ID) == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := m.announcements[strings.TrimSpace(a.ID)]
	if existing == nil {
		return nil
	}
	a.UpdatedAt = time.Now()
	cp := *a
	m.announcements[cp.ID] = &cp
	return nil
}

func (m *Memory) DeleteAnnouncement(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.announcements, strings.TrimSpace(id))
	return nil
}

func (m *Memory) CreateSystemLog(ctx context.Context, log *SystemLog) error {
	_ = ctx
	if log == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if log.ID == "" {
		log.ID = generateID()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	cp := *log
	m.systemLogs = append([]*SystemLog{&cp}, m.systemLogs...)
	return nil
}

func (m *Memory) ListSystemLogs(ctx context.Context, logType, status, q string, page, pageSize int) ([]*SystemLog, int64, error) {
	_ = ctx
	logType = strings.TrimSpace(logType)
	status = strings.TrimSpace(status)
	q = strings.ToLower(strings.TrimSpace(q))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	m.mu.RLock()
	list := make([]*SystemLog, 0, len(m.systemLogs))
	for _, l := range m.systemLogs {
		if l == nil {
			continue
		}
		if logType != "" && strings.TrimSpace(l.Type) != logType {
			continue
		}
		if status != "" && strings.TrimSpace(l.Status) != status {
			continue
		}
		if q != "" {
			username := strings.ToLower(l.Username)
			message := strings.ToLower(l.Message)
			ip := strings.ToLower(l.IP)
			if !strings.Contains(username, q) && !strings.Contains(message, q) && !strings.Contains(ip, q) {
				continue
			}
		}
		cp := *l
		list = append(list, &cp)
	}
	m.mu.RUnlock()
	total := int64(len(list))
	start := (page - 1) * pageSize
	if start >= len(list) {
		return []*SystemLog{}, total, nil
	}
	end := start + pageSize
	if end > len(list) {
		end = len(list)
	}
	return list[start:end], total, nil
}

func (m *Memory) DeleteSystemLogsOlderThan(ctx context.Context, before time.Time) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []*SystemLog
	var deleted int64
	for _, l := range m.systemLogs {
		if l.CreatedAt.Before(before) {
			deleted++
		} else {
			kept = append(kept, l)
		}
	}
	m.systemLogs = kept
	return deleted, nil
}

func (m *Memory) DeleteExpiredWafBansOlderThan(ctx context.Context, before time.Time) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted int64
	for ip, b := range m.wafBans {
		if !b.ExpiresAt.IsZero() && b.ExpiresAt.Before(before) {
			delete(m.wafBans, ip)
			deleted++
		}
	}
	return deleted, nil
}

func (m *Memory) DeleteUpgradeTasksOlderThan(ctx context.Context, before time.Time) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted int64
	for id, t := range m.upgradeTasks {
		if t.CreatedAt.Before(before) {
			delete(m.upgradeTasks, id)
			deleted++
		}
	}
	return deleted, nil
}

// DNS config
func (m *Memory) GetDNSConfig(ctx context.Context) (*DNSConfig, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.dnsConfig == nil {
		return nil, nil
	}
	cp := *m.dnsConfig
	return &cp, nil
}

func (m *Memory) SaveDNSConfig(ctx context.Context, cfg *DNSConfig) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *cfg
	cp.UpdatedAt = time.Now()
	m.dnsConfig = &cp
	return nil
}

// Upgrade tasks/logs (in-memory)
func (m *Memory) CreateUpgradeTask(ctx context.Context, t *UpgradeTask) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.ID == "" {
		t.ID = generateID()
	}
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.Type == "" {
		t.Type = "node"
	}
	if t.Channel == "" {
		t.Channel = "stable"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	m.upgradeTasks[t.ID] = t
	return nil
}

func (m *Memory) UpdateUpgradeTaskStatus(ctx context.Context, id, status string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.upgradeTasks[id]; ok && t != nil {
		t.Status = status
	}
	return nil
}

func (m *Memory) ListUpgradeTasks(ctx context.Context, limit int) ([]*UpgradeTask, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*UpgradeTask, 0, len(m.upgradeTasks))
	for _, t := range m.upgradeTasks {
		res = append(res, t)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].CreatedAt.After(res[j].CreatedAt) })
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (m *Memory) GetUpgradeTask(ctx context.Context, id string) (*UpgradeTask, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.upgradeTasks[id], nil
}

func (m *Memory) AppendUpgradeLog(ctx context.Context, id string, log UpgradeLog) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}
	log.TaskID = id
	m.upgradeLogs[id] = append(m.upgradeLogs[id], log)
	return nil
}

func (m *Memory) ListUpgradeLogs(ctx context.Context, id, nodeID string, limit int) ([]UpgradeLog, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	logs := m.upgradeLogs[id]
	var res []UpgradeLog
	for _, l := range logs {
		if nodeID == "" || l.NodeID == "" || l.NodeID == nodeID {
			res = append(res, l)
		}
	}
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

// WAF policies/rules
func (m *Memory) ListWAFPolicies(ctx context.Context) ([]*WAFPolicy, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*WAFPolicy
	for _, p := range m.wafPolicies {
		cp := *p
		cp.Rules = m.copyRulesLocked(p.ID)
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) GetWAFPolicy(ctx context.Context, id string) (*WAFPolicy, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.wafPolicies[id]; ok {
		cp := *p
		cp.Rules = m.copyRulesLocked(id)
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) CreateWAFPolicy(ctx context.Context, p *WAFPolicy) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *p
	m.wafPolicies[p.ID] = &cp
	m.wafRules[p.ID] = m.copyRules(p.Rules)
	return nil
}

func (m *Memory) UpdateWAFPolicy(ctx context.Context, p *WAFPolicy) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.wafPolicies[p.ID]; !ok {
		return sql.ErrNoRows
	}
	cp := *p
	m.wafPolicies[p.ID] = &cp
	return nil
}

func (m *Memory) DeleteWAFPolicy(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wafPolicies, id)
	delete(m.wafRules, id)
	return nil
}

func (m *Memory) ListWAFRules(ctx context.Context, policyID string) ([]*WAFRule, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.copyRulesLocked(policyID), nil
}

func (m *Memory) ReplaceWAFRules(ctx context.Context, policyID string, rules []*WAFRule) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wafRules[policyID] = m.copyRules(rules)
	return nil
}

func (m *Memory) copyRulesLocked(policyID string) []*WAFRule {
	return m.copyRules(m.wafRules[policyID])
}

func (m *Memory) copyRules(rules []*WAFRule) []*WAFRule {
	out := make([]*WAFRule, 0, len(rules))
	for _, r := range rules {
		if r == nil {
			continue
		}
		cp := *r
		if !cp.ExpiresAt.IsZero() && cp.ExpiresAt.Before(time.Now()) {
			continue
		}
		out = append(out, &cp)
	}
	return out
}

// WAF bans
func (m *Memory) ListWAFBans(ctx context.Context, limit int) ([]*WAFBan, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var bans []*WAFBan
	for _, b := range m.wafBans {
		cp := *b
		bans = append(bans, &cp)
	}
	sort.Slice(bans, func(i, j int) bool { return bans[i].ExpiresAt.Before(bans[j].ExpiresAt) })
	if limit > 0 && len(bans) > limit {
		bans = bans[:limit]
	}
	return bans, nil
}

func (m *Memory) CreateOrUpdateWAFBan(ctx context.Context, ban *WAFBan) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *ban
	m.wafBans[ban.IP] = &cp
	return nil
}

func (m *Memory) DeleteWAFBan(ctx context.Context, ip string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wafBans, ip)
	return nil
}

func (m *Memory) GetWAFBan(ctx context.Context, ip string) (*WAFBan, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if b, ok := m.wafBans[ip]; ok {
		cp := *b
		return &cp, nil
	}
	return nil, nil
}

// WAF Whitelist
func (m *Memory) ListWAFWhitelist(ctx context.Context) ([]*WAFWhitelist, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*WAFWhitelist
	for _, w := range m.wafWhitelist {
		cp := *w
		list = append(list, &cp)
	}
	return list, nil
}

func (m *Memory) CreateWAFWhitelist(ctx context.Context, w *WAFWhitelist) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if w.ID == "" {
		w.ID = generateID()
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}
	w.UpdatedAt = time.Now()
	cp := *w
	m.wafWhitelist[w.ID] = &cp
	return nil
}

func (m *Memory) DeleteWAFWhitelist(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wafWhitelist, id)
	return nil
}

func (m *Memory) IsIPWhitelisted(ctx context.Context, ip string) (bool, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false, nil
	}

	for _, w := range m.wafWhitelist {
		// Check if it's a CIDR
		if strings.Contains(w.IP, "/") {
			_, cidr, err := net.ParseCIDR(w.IP)
			if err == nil && cidr.Contains(parsedIP) {
				return true, nil
			}
		} else {
			// Direct IP match
			if w.IP == ip {
				return true, nil
			}
		}
	}
	return false, nil
}

// Email verifications
func (m *Memory) CreateEmailVerification(ctx context.Context, v *EmailVerification) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if v.ID == "" {
		v.ID = generateID()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	v.Email = strings.ToLower(strings.TrimSpace(v.Email))
	m.emailVerifications[v.ID] = v
	return nil
}

func (m *Memory) GetLatestEmailVerificationByEmail(ctx context.Context, email string) (*EmailVerification, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	target := strings.ToLower(strings.TrimSpace(email))
	var latest *EmailVerification
	for _, v := range m.emailVerifications {
		if v.Email != target {
			continue
		}
		if latest == nil || v.CreatedAt.After(latest.CreatedAt) {
			cp := *v
			latest = &cp
		}
	}
	return latest, nil
}

func (m *Memory) MarkEmailVerificationUsed(ctx context.Context, id string, usedAt time.Time) (bool, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.emailVerifications[id]
	if !ok {
		return false, nil
	}
	v.UsedAt = &usedAt
	return true, nil
}

func copyErrorPages(src []ErrorPage) []ErrorPage {
	if len(src) == 0 {
		return nil
	}
	dst := make([]ErrorPage, len(src))
	copy(dst, src)
	return dst
}

// copyDomainSecurity returns a deep copy so stored rows and returned
// copies can't alias each other's slices. We intentionally preserve
// nil-vs-empty distinction for IPBlacklist/Whitelist because "nil"
// means "leave as-is on update" at the handler layer.
func copyDomainSecurity(src *DomainSecurity) *DomainSecurity {
	if src == nil {
		return nil
	}
	out := *src
	if src.CustomRules != nil {
		out.CustomRules = make([]DomainCCRule, len(src.CustomRules))
		copy(out.CustomRules, src.CustomRules)
	}
	if src.IPBlacklist != nil {
		out.IPBlacklist = append([]string(nil), src.IPBlacklist...)
	}
	if src.IPWhitelist != nil {
		out.IPWhitelist = append([]string(nil), src.IPWhitelist...)
	}
	return &out
}

// copyOriginHealthCheck returns a defensive copy of the health-check
// config so stored rows and API responses can't alias each other.
// The struct currently holds only scalars, but a copy keeps the
// caller honest if we add slice/map fields later.
func copyOriginHealthCheck(src *OriginHealthCheck) *OriginHealthCheck {
	if src == nil {
		return nil
	}
	out := *src
	return &out
}

// normalizeLoadBalanceMethod canonicalises the load-balance method
// string. Empty/unknown values map back to "round_robin" so legacy
// code paths and zero-value structs keep their previous semantics.
func normalizeLoadBalanceMethod(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ip_hash":
		return "ip_hash"
	default:
		return "round_robin"
	}
}

func normalizeDomainListenPort(p int32) int32 {
	if p < 0 || p > 65535 {
		return 0
	}
	return p
}

// API Tokens
func (m *Memory) CreateAPIToken(ctx context.Context, description string, ttl time.Duration) (string, *APIToken, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	token := generateToken()
	id := generateID()
	now := time.Now()

	var expiresAt *time.Time
	if ttl > 0 {
		exp := now.Add(ttl)
		expiresAt = &exp
	}

	tokenPrefix := token[:8]
	t := &APIToken{
		ID:          id,
		Description: description,
		TokenPrefix: tokenPrefix,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}

	m.apiTokens[id] = t
	m.apiTokenHashes[hashTokenMem(token)] = id

	return token, t, nil
}

func (m *Memory) ListAPITokens(ctx context.Context) ([]*APIToken, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tokens []*APIToken
	for _, t := range m.apiTokens {
		cp := *t
		tokens = append(tokens, &cp)
	}
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].CreatedAt.After(tokens[j].CreatedAt)
	})
	return tokens, nil
}

func (m *Memory) DeleteAPIToken(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.apiTokens, id)
	for hash, tid := range m.apiTokenHashes {
		if tid == id {
			delete(m.apiTokenHashes, hash)
			break
		}
	}
	return nil
}

func (m *Memory) ValidateAPIToken(ctx context.Context, token string) (bool, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()

	hash := hashTokenMem(token)
	id, ok := m.apiTokenHashes[hash]
	if !ok {
		return false, nil
	}
	t, ok := m.apiTokens[id]
	if !ok {
		return false, nil
	}
	if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
		return false, nil
	}
	return true, nil
}

func hashTokenMem(token string) string {
	// Simple hash for memory store
	return token
}

// --- Domain Blacklist ---

func (m *Memory) CreateDomainBlacklist(ctx context.Context, b *DomainBlacklist) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if b.ID == "" {
		b.ID = generateID()
	}
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now
	b.Domain = strings.ToLower(strings.TrimSpace(b.Domain))

	cp := *b
	m.domainBlacklist[b.ID] = &cp
	return nil
}

func (m *Memory) ListDomainBlacklist(ctx context.Context) ([]*DomainBlacklist, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []*DomainBlacklist
	for _, b := range m.domainBlacklist {
		cp := *b
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (m *Memory) DeleteDomainBlacklist(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.domainBlacklist, id)
	return nil
}

func (m *Memory) IsDomainBlacklisted(ctx context.Context, domain string) (bool, string, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSpace(domain))

	for _, b := range m.domainBlacklist {
		// Exact match
		if b.Domain == domain {
			return true, b.Reason, nil
		}
		// Wildcard match (*.example.com)
		if strings.HasPrefix(b.Domain, "*.") {
			suffix := strings.TrimPrefix(b.Domain, "*")
			if strings.HasSuffix(domain, suffix) || domain == strings.TrimPrefix(suffix, ".") {
				return true, b.Reason, nil
			}
		}
	}
	return false, "", nil
}

func (m *Memory) ListGlobalTemplateOverrides(ctx context.Context) ([]*GlobalTemplateOverride, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*GlobalTemplateOverride
	for _, t := range m.globalTemplateOverrides {
		cp := *t
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Key < list[j].Key
	})
	return list, nil
}

func (m *Memory) GetGlobalTemplateOverride(ctx context.Context, key string) (*GlobalTemplateOverride, error) {
	_ = ctx
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.globalTemplateOverrides[key]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) UpsertGlobalTemplateOverride(ctx context.Context, t *GlobalTemplateOverride) error {
	_ = ctx
	if t == nil {
		return nil
	}
	key := strings.TrimSpace(t.Key)
	if key == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cp := *t
	cp.Key = key
	cp.UpdatedAt = now
	m.globalTemplateOverrides[key] = &cp
	return nil
}

func (m *Memory) DeleteGlobalTemplateOverride(ctx context.Context, key string) error {
	_ = ctx
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.globalTemplateOverrides, key)
	return nil
}

func (m *Memory) ListProductGroups(ctx context.Context) ([]*ProductGroup, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*ProductGroup
	for _, g := range m.productGroups {
		cp := *g
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Sort != list[j].Sort {
			return list[i].Sort < list[j].Sort
		}
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (m *Memory) GetProductGroup(ctx context.Context, id string) (*ProductGroup, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if g, ok := m.productGroups[id]; ok && g != nil {
		cp := *g
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) CreateProductGroup(ctx context.Context, g *ProductGroup) error {
	_ = ctx
	if g == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cp := *g
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = generateID()
	}
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Description = strings.TrimSpace(cp.Description)
	if cp.Sort == 0 {
		cp.Sort = 100
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	m.productGroups[cp.ID] = &cp
	return nil
}

func (m *Memory) UpdateProductGroup(ctx context.Context, g *ProductGroup) error {
	_ = ctx
	if g == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.productGroups[g.ID]
	if !ok || existing == nil {
		return sql.ErrNoRows
	}
	cp := *g
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Description = strings.TrimSpace(cp.Description)
	if cp.Sort == 0 {
		cp.Sort = 100
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = existing.CreatedAt
	}
	cp.UpdatedAt = time.Now()
	m.productGroups[cp.ID] = &cp
	return nil
}

func (m *Memory) DeleteProductGroup(ctx context.Context, id string) error {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.productGroups, id)
	for pid, p := range m.products {
		if p != nil && p.GroupID == id {
			cp := *p
			cp.GroupID = ""
			cp.UpdatedAt = time.Now()
			m.products[pid] = &cp
		}
	}
	return nil
}

func (m *Memory) ListUserGroups(ctx context.Context) ([]*UserGroup, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*UserGroup
	for _, g := range m.userGroups {
		cp := *g
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		if !strings.EqualFold(list[i].Name, list[j].Name) {
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		}
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (m *Memory) GetUserGroup(ctx context.Context, id string) (*UserGroup, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if g, ok := m.userGroups[id]; ok && g != nil {
		cp := *g
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) CreateUserGroup(ctx context.Context, g *UserGroup) error {
	_ = ctx
	if g == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cp := *g
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = generateID()
	}
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Description = strings.TrimSpace(cp.Description)
	if cp.Permissions != nil {
		cp.Permissions = append([]string(nil), cp.Permissions...)
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	m.userGroups[cp.ID] = &cp
	return nil
}

func (m *Memory) UpdateUserGroup(ctx context.Context, g *UserGroup) error {
	_ = ctx
	if g == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.userGroups[g.ID]
	if !ok || existing == nil {
		return sql.ErrNoRows
	}
	cp := *g
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Description = strings.TrimSpace(cp.Description)
	if cp.Permissions != nil {
		cp.Permissions = append([]string(nil), cp.Permissions...)
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = existing.CreatedAt
	}
	cp.UpdatedAt = time.Now()
	m.userGroups[cp.ID] = &cp
	return nil
}

func (m *Memory) DeleteUserGroup(ctx context.Context, id string) error {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.userGroups, id)
	for _, u := range m.users {
		if u != nil && u.GroupID == id {
			u.GroupID = ""
			u.UpdatedAt = time.Now()
		}
	}
	return nil
}

func (m *Memory) ListProducts(ctx context.Context) ([]*Product, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*Product
	for _, p := range m.products {
		cp := *p
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (m *Memory) GetProduct(ctx context.Context, id string) (*Product, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.products[id]; ok && p != nil {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) CreateProduct(ctx context.Context, p *Product) error {
	_ = ctx
	if p == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cp := *p
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = generateID()
	}
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Slug = strings.ToLower(strings.TrimSpace(cp.Slug))
	cp.Description = strings.TrimSpace(cp.Description)
	cp.GroupID = strings.TrimSpace(cp.GroupID)
	cp.Region = strings.TrimSpace(cp.Region)
	cp.LineGroupID = strings.TrimSpace(cp.LineGroupID)
	if cp.Sort == 0 {
		cp.Sort = 100
	}
	cp.Currency = strings.ToUpper(strings.TrimSpace(cp.Currency))
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	if cp.PriceMonthCents == 0 && cp.PriceCents != 0 {
		cp.PriceMonthCents = cp.PriceCents
	}
	if cp.PriceCents == 0 && cp.PriceMonthCents != 0 {
		cp.PriceCents = cp.PriceMonthCents
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	m.products[cp.ID] = &cp
	return nil
}

func (m *Memory) UpdateProduct(ctx context.Context, p *Product) error {
	_ = ctx
	if p == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.products[p.ID]
	if !ok || existing == nil {
		return sql.ErrNoRows
	}
	cp := *p
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Slug = strings.ToLower(strings.TrimSpace(cp.Slug))
	cp.Description = strings.TrimSpace(cp.Description)
	cp.GroupID = strings.TrimSpace(cp.GroupID)
	cp.Region = strings.TrimSpace(cp.Region)
	cp.LineGroupID = strings.TrimSpace(cp.LineGroupID)
	if cp.Sort == 0 {
		cp.Sort = 100
	}
	cp.Currency = strings.ToUpper(strings.TrimSpace(cp.Currency))
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	if cp.PriceMonthCents == 0 && cp.PriceCents != 0 {
		cp.PriceMonthCents = cp.PriceCents
	}
	if cp.PriceCents == 0 && cp.PriceMonthCents != 0 {
		cp.PriceCents = cp.PriceMonthCents
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = existing.CreatedAt
	}
	cp.UpdatedAt = time.Now()
	m.products[cp.ID] = &cp
	return nil
}

func (m *Memory) DeleteProduct(ctx context.Context, id string) error {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.products, id)
	return nil
}

func (m *Memory) ListOrders(ctx context.Context, userID string) ([]*Order, error) {
	_ = ctx
	userID = strings.TrimSpace(userID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*Order
	for _, o := range m.orders {
		if userID != "" && o.UserID != userID {
			continue
		}
		cp := *o
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (m *Memory) GetOrder(ctx context.Context, id string) (*Order, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if o, ok := m.orders[id]; ok && o != nil {
		cp := *o
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) CreateOrder(ctx context.Context, o *Order) error {
	_ = ctx
	if o == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cp := *o
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = generateID()
	}
	cp.UserID = strings.TrimSpace(cp.UserID)
	cp.ProductID = strings.TrimSpace(cp.ProductID)
	cp.ProductName = strings.TrimSpace(cp.ProductName)
	cp.Status = strings.ToLower(strings.TrimSpace(cp.Status))
	if cp.Status == "" {
		cp.Status = "pending"
	}
	cp.Period = strings.ToLower(strings.TrimSpace(cp.Period))
	if cp.Period == "" {
		cp.Period = "month"
	}
	if cp.Quantity <= 0 {
		cp.Quantity = 1
	}
	cp.Note = strings.TrimSpace(cp.Note)
	cp.Currency = strings.ToUpper(strings.TrimSpace(cp.Currency))
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	m.orders[cp.ID] = &cp
	return nil
}

func (m *Memory) UpdateOrder(ctx context.Context, o *Order) error {
	_ = ctx
	if o == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.orders[o.ID]
	if !ok || existing == nil {
		return sql.ErrNoRows
	}
	cp := *o
	cp.UserID = strings.TrimSpace(cp.UserID)
	cp.ProductID = strings.TrimSpace(cp.ProductID)
	cp.ProductName = strings.TrimSpace(cp.ProductName)
	cp.Status = strings.ToLower(strings.TrimSpace(cp.Status))
	if cp.Status == "" {
		cp.Status = "pending"
	}
	cp.Period = strings.ToLower(strings.TrimSpace(cp.Period))
	if cp.Period == "" {
		cp.Period = "month"
	}
	if cp.Quantity <= 0 {
		cp.Quantity = 1
	}
	cp.Note = strings.TrimSpace(cp.Note)
	cp.Currency = strings.ToUpper(strings.TrimSpace(cp.Currency))
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = existing.CreatedAt
	}
	cp.UpdatedAt = time.Now()
	m.orders[cp.ID] = &cp
	return nil
}

func (m *Memory) DeleteOrder(ctx context.Context, id string) error {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.orders, id)
	return nil
}

// Cluster operations
func (m *Memory) CreateCluster(ctx context.Context, c *Cluster) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	m.clusters[c.ID] = c
	return nil
}

func (m *Memory) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	if c, ok := m.clusters[id]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (m *Memory) ListClusters(ctx context.Context) ([]*Cluster, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*Cluster
	for _, c := range m.clusters {
		cp := *c
		res = append(res, &cp)
	}
	return res, nil
}

func (m *Memory) UpdateCluster(ctx context.Context, c *Cluster) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.clusters[c.ID]; ok {
		if c.Name != "" {
			existing.Name = c.Name
		}
		if c.DNSZone != "" {
			existing.DNSZone = c.DNSZone
		}
		if c.DNSMode != "" {
			existing.DNSMode = c.DNSMode
		}
		if c.CNAME != "" {
			existing.CNAME = c.CNAME
		}
		if c.Description != "" {
			existing.Description = c.Description
		}
		existing.Enabled = c.Enabled
		existing.UpdatedAt = time.Now()
	}
	return nil
}

func (m *Memory) DeleteCluster(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clusters, id)
	delete(m.clusterNodes, id)
	return nil
}

func (m *Memory) GetUserTraffic(ctx context.Context, userID, month string) (int64, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.userTraffic[userID+":"+month], nil
}

func (m *Memory) IncrementUserTraffic(ctx context.Context, userID, month string, bytes int64) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userTraffic[userID+":"+month] += bytes
	return nil
}
