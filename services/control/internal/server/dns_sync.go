package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/dnsprovider"
	"github.com/lingcdn/control/internal/store"
)

const dnsSyncDebounce = 10 * time.Second
const maxNodeStaleness = NodeDNSStaleWindow

func dnsAutoSyncEnabled() bool {
	v := strings.TrimSpace(os.Getenv("DNS_AUTO_SYNC"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func dnsManagedZone() string {
	if v := strings.TrimSpace(os.Getenv("DNS_ZONE")); v != "" {
		return dnsprovider.NormalizeZone(v)
	}
	return dnsprovider.NormalizeZone(os.Getenv("CNAME_SUFFIX"))
}

func dnsPruneOrphans() bool {
	v := strings.TrimSpace(os.Getenv("DNS_PRUNE_ORPHANS"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// collectDNSZones returns all DNS zones to sync: cluster dns_zones first, then env var fallback.
func (s *Servers) collectDNSZones() []string {
	seen := make(map[string]struct{})
	var zones []string

	// Collect from clusters
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	clusters, err := s.store.ListClusters(ctx)
	if err == nil {
		for _, c := range clusters {
			if c == nil || !c.Enabled {
				continue
			}
			z := dnsprovider.NormalizeZone(c.DNSZone)
			if z == "" {
				continue
			}
			if _, ok := seen[z]; !ok {
				seen[z] = struct{}{}
				zones = append(zones, z)
			}
		}
	}

	// Fallback to env var if no cluster zones configured
	if len(zones) == 0 {
		if z := dnsManagedZone(); z != "" {
			zones = append(zones, z)
		}
	}

	return zones
}

func (s *Servers) handleDNSSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	// Synchronous execution: wait for the result and return it directly.
	zones := s.collectDNSZones()
	if len(zones) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"message": "未配置 DNS 域名区域，请先创建集群并设置 DNS 域名",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	cfg, err := s.store.GetDNSConfig(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("获取 DNS 配置失败: %v", err)})
		return
	}
	if cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "DNS 提供商未配置，跳过同步"})
		return
	}
	if err := ensureDNSSyncReady(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	client, err := dnsprovider.NewClient(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("DNS 客户端初始化失败: %v", err)})
		return
	}

	var messages []string
	var lastErr error
	for _, zone := range zones {
		msg, err := s.syncDNSRecords(ctx, client, cfg, zone)
		if err != nil {
			lastErr = err
			messages = append(messages, fmt.Sprintf("[%s] 失败: %v", zone, err))
		} else {
			messages = append(messages, msg)
		}
	}

	if lastErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"message": strings.Join(messages, "; "),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": strings.Join(messages, "; "),
	})
}

func (s *Servers) triggerDNSSync(subject, reason string) {
	if !dnsAutoSyncEnabled() {
		return
	}
	if s == nil || s.store == nil {
		return
	}

	s.dnsSyncMu.Lock()
	if !s.dnsSyncAt.IsZero() && time.Since(s.dnsSyncAt) < dnsSyncDebounce {
		s.dnsSyncMu.Unlock()
		return
	}
	s.dnsSyncAt = time.Now()
	s.dnsSyncMu.Unlock()

	zones := s.collectDNSZones()
	if len(zones) == 0 {
		return
	}
	for _, zone := range zones {
		_ = s.runDNSSyncTask(zone, subject, reason, "auto")
	}
}

func (s *Servers) runDNSSyncNow(subject, reason string) *dnsTask {
	if s == nil || s.store == nil {
		return &dnsTask{
			ID:        "",
			Type:      "sync",
			Subject:   subject,
			Provider:  "manual",
			Status:    "failed",
			Message:   "存储未初始化",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}
	zones := s.collectDNSZones()
	if len(zones) == 0 {
		return s.runDNSTask("sync", subject, "manual", func() (string, error) { return "未配置 DNS 域名区域，请先创建集群并设置 DNS 域名", nil })
	}
	var lastTask *dnsTask
	for _, zone := range zones {
		lastTask = s.runDNSSyncTask(zone, subject, reason, "manual")
	}
	return lastTask
}

// runDNSCleanupNow prunes orphan DNS records across all configured zones.
func (s *Servers) runDNSCleanupNow(subject, reason string) *dnsTask {
	if s == nil || s.store == nil {
		return &dnsTask{
			ID:        "",
			Type:      "cleanup",
			Subject:   subject,
			Provider:  "manual",
			Status:    "failed",
			Message:   "存储未初始化",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}
	zones := s.collectDNSZones()
	if len(zones) == 0 {
		return s.runDNSTask("cleanup", subject, "manual", func() (string, error) {
			return "未配置 DNS 域名区域", nil
		})
	}
	return s.runDNSTask("cleanup", subject, "manual", func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		cfg, err := s.store.GetDNSConfig(ctx)
		if err != nil {
			return "", err
		}
		if cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
			return "DNS 提供商未配置", nil
		}
		client, err := dnsprovider.NewClient(cfg)
		if err != nil {
			return "", err
		}
		ttl := cfg.TTL
		if ttl <= 0 {
			ttl = 600
		}

		var messages []string
		for _, zone := range zones {
			// syncDNSRecords builds desired set and prunes orphans when enabled
			msg, err := s.syncDNSRecords(ctx, client, cfg, zone)
			if err != nil {
				return "", fmt.Errorf("zone %s: %w", zone, err)
			}
			if dnsPruneOrphans() {
				messages = append(messages, fmt.Sprintf("%s: 已清理孤立记录", zone))
			} else {
				messages = append(messages, fmt.Sprintf("%s: %s (孤立记录清理已禁用)", zone, msg))
			}
		}
		if len(messages) == 0 {
			return "DNS 清理完成", nil
		}
		return strings.Join(messages, "; "), nil
	})
}

func (s *Servers) runDNSSyncTask(zone, subject, reason, provider string) *dnsTask {
	return s.runDNSTask("sync", subject, provider, func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cfg, err := s.store.GetDNSConfig(ctx)
		if err != nil {
			return "", err
		}
		if cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
			return "DNS 提供商未配置，跳过同步", nil
		}

		client, err := dnsprovider.NewClient(cfg)
		if err != nil {
			s.notifyAdmin("DNS 同步失败", fmt.Sprintf("DNS 客户端初始化失败: %v", err))
			return "", err
		}

		msg, err := s.syncDNSRecords(ctx, client, cfg, zone)
		if err != nil {
			_ = s.store.SaveDNSConfig(ctx, &store.DNSConfig{
				Provider:       cfg.Provider,
				AccountID:      cfg.AccountID,
				Token:          cfg.Token,
				Secret:         cfg.Secret,
				TTL:            cfg.TTL,
				EnableIPWeight: cfg.EnableIPWeight,
				LastError:      err.Error(),
				UpdatedAt:      time.Now(),
			})
			s.notifyAdmin("DNS 同步失败", fmt.Sprintf("DNS 同步错误: %v", err))
			return "", err
		}

		_ = s.store.SaveDNSConfig(ctx, &store.DNSConfig{
			Provider:       cfg.Provider,
			AccountID:      cfg.AccountID,
			Token:          cfg.Token,
			Secret:         cfg.Secret,
			TTL:            cfg.TTL,
			EnableIPWeight: cfg.EnableIPWeight,
			LastError:      "",
			UpdatedAt:      time.Now(),
		})

		if reason != "" {
			return reason + ": " + msg, nil
		}
		return msg, nil
	})
}

func (s *Servers) syncDNSRecords(ctx context.Context, client dnsprovider.Client, cfg *store.DNSConfig, zone string) (string, error) {
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 600
	}

	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now()
	nodeIP := make(map[string]net.IP, len(nodes))
	nodeWeights := make(map[string]int32, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if !nodeHealthy(n, now) {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(n.PublicIP))
		if ip == nil {
			continue
		}
		nodeIP[n.ID] = ip
		if w := parseNodeWeight(n.Capabilities); w > 0 {
			nodeWeights[n.ID] = w
		}
	}

	domains, err := s.store.ListDomains(ctx)
	if err != nil {
		return "", err
	}

	defaultTarget := strings.TrimSpace(os.Getenv("DNS_DEFAULT_TARGET"))
	if defaultTarget != "" {
		defaultTarget = strings.Trim(defaultTarget, ".")
	}

	var messages []string
	desiredNames := make(map[string]struct{})
	lineCapable := client.SupportsLine()

	// 1) Cluster A/AAAA records.
	clusters, err := s.store.ListClusters(ctx)
	if err != nil {
		return "", err
	}
	clusterMap := make(map[string]*store.Cluster, len(clusters))
	for _, cl := range clusters {
		if cl == nil || !cl.Enabled {
			continue
		}
		clusterMap[cl.ID] = cl

		fqdn := strings.Trim(strings.TrimSpace(cl.CNAME), ".")
		if fqdn == "" {
			continue
		}
		rr, ok := dnsprovider.SplitByZone(fqdn, zone)
		if !ok {
			continue
		}

		metaAll, _ := s.store.ListClusterNodes(ctx, cl.ID, "")
		lineMeta := make(map[string][]*store.ClusterNode)
		if len(metaAll) > 0 {
			for _, m := range metaAll {
				if m == nil {
					continue
				}
				lineName := normalizeLineName(m.Line)
				lineMeta[lineName] = append(lineMeta[lineName], m)
			}
		} else {
			lineMeta[defaultLineName] = nil
		}

		lines := make([]string, 0, len(lineMeta))
		for line := range lineMeta {
			lines = append(lines, line)
		}
		sort.Strings(lines)
		if !lineCapable {
			merged := make([]*store.ClusterNode, 0)
			for _, list := range lineMeta {
				merged = append(merged, list...)
			}
			lineMeta = map[string][]*store.ClusterNode{
				defaultLineName: merged,
			}
			lines = []string{defaultLineName}
		}

		fqdn = dnsprovider.JoinFQDN(rr, zone)
		for _, line := range lines {
			meta := lineMeta[line]
			var useIDs []string
			groupWeights := make(map[string]int32)
			if len(meta) > 0 {
				primary := make([]string, 0, len(meta))
				backup := make([]string, 0, len(meta))
				for _, m := range meta {
					if m == nil || !m.Enabled {
						continue
					}
					if m.Backup {
						backup = append(backup, m.NodeID)
					} else {
						primary = append(primary, m.NodeID)
					}
					if m.Weight > 0 {
						groupWeights[m.NodeID] = m.Weight
					}
				}
				if len(primary) > 0 {
					useIDs = primary
				} else {
					useIDs = backup
				}
			}

			var ips4 []string
			var ips6 []string
			groupIPToNodes := make(map[string][]string)
			for _, nid := range useIDs {
				ip := nodeIP[nid]
				if ip == nil {
					continue
				}
				ipStr := ip.String()
				groupIPToNodes[ipStr] = append(groupIPToNodes[ipStr], nid)
				if ip4 := ip.To4(); ip4 != nil {
					ips4 = append(ips4, ip4.String())
				} else {
					ips6 = append(ips6, ip.String())
				}
			}
			ips4 = uniqueStrings(ips4)
			ips6 = uniqueStrings(ips6)
			var ipWeights map[string]int32
			if cfg.EnableIPWeight {
				weights := nodeWeights
				if len(groupWeights) > 0 {
					weights = groupWeights
				}
				ipWeights = make(map[string]int32)
				for ip, ids := range groupIPToNodes {
					w := int32(1)
					for _, nid := range ids {
						if weights[nid] > w {
							w = weights[nid]
						}
					}
					ipWeights[ip] = w
				}
			}

			lineParam := line
			lineKey := normalizeLineKey(line)
			if !lineCapable {
				lineParam = defaultLineName
				lineKey = normalizeLineKey(defaultLineName)
			}
			desiredNames[fqdn+"|A|"+lineKey] = struct{}{}
			desiredNames[fqdn+"|AAAA|"+lineKey] = struct{}{}

			msgA, err := client.EnsureRecords(ctx, zone, rr, dnsprovider.RecordTypeA, ips4, ttl, lineParam, ipWeights)
			if err != nil {
				return "", err
			}
			msgAAAA, err := client.EnsureRecords(ctx, zone, rr, dnsprovider.RecordTypeAAAA, ips6, ttl, lineParam, ipWeights)
			if err != nil {
				return "", err
			}
			messages = append(messages, msgA, msgAAAA)
		}
	}

	// 2) Domain CNAME records -> cluster CNAME (or DNS_DEFAULT_TARGET).
	for _, d := range domains {
		if d == nil || !d.Enabled {
			continue
		}
		alias := strings.Trim(strings.TrimSpace(d.CNAME), ".")
		if alias == "" {
			zoneHint := ""
			if d.LineGroupID != "" {
				if cl := clusterMap[d.LineGroupID]; cl != nil {
					zoneHint = strings.Trim(strings.TrimSpace(cl.DNSZone), ".")
				}
			}
			alias = strings.Trim(s.generateDomainCNAMEForZone(d.Name, zoneHint), ".")
		}
		if alias == "" {
			continue
		}
		rrAlias, ok := dnsprovider.SplitByZone(alias, zone)
		if !ok {
			continue
		}

		target := ""
		if d.LineGroupID != "" {
			if cl := clusterMap[d.LineGroupID]; cl != nil {
				target = strings.Trim(strings.TrimSpace(cl.CNAME), ".")
			}
		}
		if target == "" {
			target = defaultTarget
		}
		if target == "" {
			continue
		}

		fqdn := dnsprovider.JoinFQDN(rrAlias, zone)
		desiredNames[fqdn+"|CNAME|"+normalizeLineKey(defaultLineName)] = struct{}{}

		msg, err := client.EnsureRecords(ctx, zone, rrAlias, dnsprovider.RecordTypeCNAME, []string{target}, ttl, defaultLineName, nil)
		if err != nil {
			return "", err
		}
		messages = append(messages, msg)
	}

	// 3) Optional orphan pruning
	if dnsPruneOrphans() {
		if err := s.pruneOrphans(ctx, client, zone, ttl, desiredNames); err != nil {
			return "", err
		}
	}

	sort.Strings(messages)
	log.Ctx(ctx).Info().Str("zone", zone).Int("actions", len(messages)).Msg("dns sync completed")
	return strings.Join(messages, "; "), nil
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func expandByWeight(values []string, ipToNodes map[string][]string, nodeWeights map[string]int32) []string {
	if len(values) == 0 {
		return values
	}
	var result []string
	for _, ip := range values {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		w := int32(1)
		for _, nid := range ipToNodes[ip] {
			if nodeWeights[nid] > w {
				w = nodeWeights[nid]
			}
		}
		if w < 1 {
			w = 1
		}
		for i := int32(0); i < w; i++ {
			result = append(result, ip)
		}
	}
	return result
}

func nodeHealthy(n *store.Node, now time.Time) bool {
	if n == nil {
		return false
	}
	if !n.LastHeartbeat.IsZero() && now.Sub(n.LastHeartbeat) > maxNodeStaleness {
		return false
	}
	if strings.EqualFold(n.Status, "offline") || strings.EqualFold(n.Status, "disabled") {
		return false
	}
	// 当节点开启了主动监控时，检查监控探测结果
	if n.MonitorEnabled {
		threshold := n.MonitorFailThreshold
		if threshold <= 0 {
			threshold = 3
		}
		// 连续失败次数达到阈值，判定为不健康
		if n.MonitorFailCount >= threshold {
			return false
		}
	}
	return true
}

func parseNodeWeight(caps []string) int32 {
	for _, c := range caps {
		clean := strings.TrimSpace(strings.ToLower(c))
		if strings.HasPrefix(clean, "weight=") {
			if w, err := strconv.Atoi(strings.TrimPrefix(clean, "weight=")); err == nil && w > 0 {
				return int32(w)
			}
		}
		if strings.HasPrefix(clean, "weight:") {
			if w, err := strconv.Atoi(strings.TrimPrefix(clean, "weight:")); err == nil && w > 0 {
				return int32(w)
			}
		}
	}
	return 1
}

func (s *Servers) pruneOrphans(ctx context.Context, client dnsprovider.Client, zone string, ttl int64, desired map[string]struct{}) error {
	records, err := client.ListRecords(ctx, zone, "")
	if err != nil {
		return err
	}
	for _, rec := range records {
		if rec.Type != dnsprovider.RecordTypeA && rec.Type != dnsprovider.RecordTypeAAAA && rec.Type != dnsprovider.RecordTypeCNAME {
			continue
		}
		lineKey := normalizeLineKey(rec.Line)
		key := strings.TrimSuffix(rec.Name, ".") + "|" + string(rec.Type) + "|" + lineKey
		if _, ok := desired[key]; ok {
			continue
		}
		rr, ok := dnsprovider.SplitByZone(rec.Name, zone)
		if !ok {
			continue
		}
		// Delete this RR by ensuring empty values
		line := normalizeLineName(rec.Line)
		if _, err := client.EnsureRecords(ctx, zone, rr, rec.Type, []string{}, ttl, line, nil); err != nil {
			return err
		}
	}
	return nil
}

// evaluateDomainDNSHealth returns human-readable warnings about why a
// newly-created or updated domain's CNAME may not actually resolve.
// Empty slice means everything looks good. Used to surface silent DNS
// misconfiguration to the admin API caller.
func (s *Servers) evaluateDomainDNSHealth(ctx context.Context, d *store.Domain) []string {
	if d == nil {
		return nil
	}
	var warns []string

	cfg, err := s.store.GetDNSConfig(ctx)
	if err != nil || cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
		warns = append(warns, "DNS 提供商未配置，CNAME 不会自动发布；请在「DNS 管理」中填写提供商凭据")
	}

	if strings.TrimSpace(d.LineGroupID) == "" {
		warns = append(warns, "域名未绑定集群，DNS 同步将跳过此域名")
		return warns
	}
	cl, err := s.store.GetCluster(ctx, d.LineGroupID)
	if err != nil || cl == nil {
		warns = append(warns, "域名绑定的集群不存在，DNS 无法发布")
		return warns
	}

	zone := strings.Trim(strings.TrimSpace(cl.DNSZone), ".")
	if zone == "" {
		warns = append(warns, fmt.Sprintf("集群 %q 未设置 DNS_ZONE，无法确定受管 zone", cl.Name))
	}

	clusterCNAME := strings.Trim(strings.TrimSpace(cl.CNAME), ".")
	if clusterCNAME == "" {
		warns = append(warns, fmt.Sprintf("集群 %q 未设置 CNAME，domain 的 CNAME 无解析目标", cl.Name))
	} else if zone != "" {
		if _, ok := dnsprovider.SplitByZone(clusterCNAME, zone); !ok {
			warns = append(warns, fmt.Sprintf("集群 CNAME %q 不在 zone %q 内，DNS 同步会跳过 A/AAAA 发布", clusterCNAME, zone))
		}
	}

	alias := strings.Trim(strings.TrimSpace(d.CNAME), ".")
	if alias != "" && zone != "" {
		if _, ok := dnsprovider.SplitByZone(alias, zone); !ok {
			warns = append(warns, fmt.Sprintf("域名 CNAME %q 不在集群 zone %q 内，DNS 同步会跳过", alias, zone))
		}
	}

	nodes, err := s.store.ListClusterNodes(ctx, cl.ID, "")
	if err != nil {
		warns = append(warns, fmt.Sprintf("查询集群 %q 节点失败: %v", cl.Name, err))
	} else if len(nodes) == 0 {
		warns = append(warns, fmt.Sprintf("集群 %q 未绑定任何节点", cl.Name))
	} else {
		now := time.Now()
		healthyWithIP := false
		for _, cn := range nodes {
			if cn == nil || !cn.Enabled {
				continue
			}
			n, err := s.store.GetNode(ctx, cn.NodeID)
			if err != nil || n == nil {
				continue
			}
			if !nodeHealthy(n, now) {
				continue
			}
			ip := strings.TrimSpace(n.PublicIP)
			if ip != "" && net.ParseIP(ip) != nil {
				healthyWithIP = true
				break
			}
		}
		if !healthyWithIP {
			warns = append(warns, fmt.Sprintf("集群 %q 暂无带公网 IP 的健康节点，CNAME 解析将无结果", cl.Name))
		}
	}

	return warns
}
