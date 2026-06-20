package server

// Domain management handlers: admin + per-user CRUD. The GET paths run in
// two scopes (admin = all domains, user = own domains) and join with the
// origin/cert/line-group tables to return a rich domainView payload the UI
// renders directly. Mutations trigger an auto-publish so nodes pick up the
// new config without a separate click.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

// Domains handlers
func (s *Servers) handleDomains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := getUserRole(ctx)
	userID, _ := ctx.Value(ctxKeyUserID).(string)

	switch r.Method {
	case http.MethodGet:
		var domains []*store.Domain
		var err error
		if role == "admin" {
			domains, err = s.store.ListDomains(ctx)
		} else {
			domains, err = s.store.ListDomainsByUser(ctx, userID)
		}
		if err != nil {
			writeInternalError(w, "list domains", err)
			return
		}
		clusterList, err := s.store.ListClusters(ctx)
		if err != nil {
			writeInternalError(w, "list clusters", err)
			return
		}
		if role != "admin" {
			neededClusters := make(map[string]bool)
			for _, d := range domains {
				if strings.TrimSpace(d.LineGroupID) != "" {
					neededClusters[d.LineGroupID] = true
				}
			}
			filtered := make([]*store.Cluster, 0, len(neededClusters))
			for _, cl := range clusterList {
				if neededClusters[cl.ID] {
					filtered = append(filtered, cl)
				}
			}
			clusterList = filtered
		}
		var origins []*store.Origin
		var certs []*store.Certificate
		if role == "admin" {
			origins, err = s.store.ListOrigins(ctx)
			if err != nil {
				writeInternalError(w, "list origins", err)
				return
			}
			certs, err = s.store.ListCertificates(ctx)
			if err != nil {
				writeInternalError(w, "list certificates", err)
				return
			}
		} else {
			certs, err = s.store.ListCertificatesByUser(ctx, userID)
			if err != nil {
				writeInternalError(w, "list certificates", err)
				return
			}
		}
		allDomainOrigins, err := s.store.ListAllDomainOrigins(ctx)
		if err != nil {
			writeInternalError(w, "list domain origins", err)
			return
		}

		lineGroupMap := make(map[string]string)
		clusterZoneMap := make(map[string]string)
		for _, cl := range clusterList {
			lineGroupMap[cl.ID] = cl.Name
			clusterZoneMap[cl.ID] = strings.Trim(strings.TrimSpace(cl.DNSZone), ".")
		}
		originMap := make(map[string]*store.Origin)
		for _, o := range origins {
			originMap[o.ID] = o
		}
		// Index per-domain origin rows (new authoritative source).
		domainOriginsMap := make(map[string][]*store.DomainOrigin)
		for _, e := range allDomainOrigins {
			domainOriginsMap[e.DomainID] = append(domainOriginsMap[e.DomainID], e)
		}
		certMap := make(map[string]*store.Certificate)
		for _, c := range certs {
			certMap[strconv.FormatInt(c.ID, 10)] = c
		}

		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		lineGroupFilter := strings.TrimSpace(r.URL.Query().Get("line_group_id"))
		enabledFilter := strings.TrimSpace(r.URL.Query().Get("enabled"))
		httpsFilter := strings.TrimSpace(r.URL.Query().Get("https"))

		var views []domainView
		for _, d := range domains {
			// Derive the effective HTTPS state for the list view: the
			// persisted flag wins, but we also light it up when a cert
			// is attached on legacy records that predate the dedicated
			// column (old rows default HTTPSEnabled to false after decode
			// but were previously treated as "on" purely by having a cert).
			httpsEnabled := d.HTTPSEnabled || strings.TrimSpace(d.CertID) != ""
			if q != "" {
				nameMatch := strings.Contains(strings.ToLower(d.Name), q)
				originMatch := false
				if o := originMap[d.OriginID]; o != nil {
					originMatch = strings.Contains(strings.ToLower(o.Name), q)
				}
				if !nameMatch && !originMatch {
					continue
				}
			}
			if lineGroupFilter != "" && d.LineGroupID != lineGroupFilter {
				continue
			}
			if enabledFilter != "" {
				target := enabledFilter == "true" || enabledFilter == "1"
				if d.Enabled != target {
					continue
				}
			}
			if httpsFilter != "" {
				target := httpsFilter == "true" || httpsFilter == "1"
				if httpsEnabled != target {
					continue
				}
			}

			var originName string
			var originAddresses []string
			// Prefer per-domain origins. Fall back to the legacy global
			// origin pool only for domains that predate the refactor and
			// haven't had their origins re-saved yet.
			if list := domainOriginsMap[d.ID]; len(list) > 0 {
				originName = d.Name + "-origin"
				for _, e := range list {
					originAddresses = append(originAddresses, e.Address)
				}
			} else if role == "admin" {
				if o := originMap[d.OriginID]; o != nil {
					originName = o.Name
					originAddresses = o.Addresses
				}
			}
			var certName, certDomain string
			if c := certMap[d.CertID]; c != nil {
				certName = c.Name
				certDomain = c.Domain
			}
			listenPort := int(normalizeListenPort(d.ListenPort))
			if listenPort == 0 {
				listenPort = 80
				if httpsEnabled {
					listenPort = 443
				}
			}
			cname := strings.TrimSpace(d.CNAME)
			// Heal legacy/invalid data where CNAME was stored equal to the
			// hostname itself — such a value points back at the user and
			// cannot be used as an alias target. Regenerate a proper one.
			if cname != "" && strings.EqualFold(cname, strings.TrimSpace(d.Name)) {
				cname = ""
			}
			if cname == "" {
				cname = s.generateDomainCNAMEForZone(d.Name, clusterZoneMap[d.LineGroupID])
			}

			views = append(views, domainView{
				ID:                     d.ID,
				Name:                   d.Name,
				UserID:                 d.UserID,
				LineGroupID:            d.LineGroupID,
				LineGroupName:          lineGroupMap[d.LineGroupID],
				OriginID:               d.OriginID,
				OriginName:             originName,
				OriginAddresses:        originAddresses,
				OriginScheme:           defaultOriginScheme(d.OriginScheme),
				OriginPort:             defaultOriginPort(d.OriginPort),
				OriginHostMode:         defaultOriginHostMode(d.OriginHostMode),
				OriginHost:             d.OriginHost,
				OriginTimeoutMs:        defaultOriginTimeout(d.OriginTimeoutMs),
				OriginConnectTimeoutMs: defaultOriginConnectTimeout(d.OriginConnectTimeoutMs),
				CertID:                 d.CertID,
				CertName:               certName,
				CertDomain:             certDomain,
				HTTPSEnabled:           httpsEnabled,
				HTTP2Enabled:           d.HTTP2Enabled,
				ListenPort:             listenPort,
				CNAME:                  cname,
				ErrorPages:             d.ErrorPages,
				Enabled:                d.Enabled,
				CacheEnabled:           d.CacheEnabled,
				CreatedAt:              d.CreatedAt,
				UpdatedAt:              d.UpdatedAt,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"domains": views,
			"count":   len(views),
		})

	case http.MethodPost:
		if role != "admin" && !s.requireUserPermission(w, ctx, PermDomainsWrite) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求内容格式错误"})
			return
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		var domain store.Domain
		if err := json.Unmarshal(body, &domain); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		cacheProvided, cacheEnabled, err := parseOptionalBoolField(payload, "cache_enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的缓存启用值"})
			return
		}
		http2Provided, http2Enabled, err := parseOptionalBoolField(payload, "http2_enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的HTTP2启用值"})
			return
		}
		wsProvided, wsEnabled, err := parseOptionalBoolField(payload, "websocket_enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的WebSocket启用值"})
			return
		}
		httpsProvided, httpsEnabled, err := parseOptionalBoolField(payload, "https_enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的HTTPS启用值"})
			return
		}
		domain.ListenPort = normalizeListenPort(domain.ListenPort)
		domain.Name = strings.TrimSpace(domain.Name)
		domain.CNAME = strings.TrimSpace(domain.CNAME)
		domain.LineGroupID = strings.TrimSpace(domain.LineGroupID)
		domain.OriginID = strings.TrimSpace(domain.OriginID)
		domain.CertID = strings.TrimSpace(domain.CertID)
		if err := normalizeDomainOrigin(&domain); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if pages, err := normalizeErrorPages(domain.ErrorPages); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		} else {
			domain.ErrorPages = pages
		}
		if domain.ID == "" {
			domain.ID = uuid.NewString()
		}
		if domain.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		// For non-admin users: set user_id, validate order, auto-resolve line_group from product
		if role != "admin" {
			domain.UserID = userID
			matchedProduct, err := s.getUserActiveProduct(ctx, userID, domain.LineGroupID)
			if err != nil {
				writeInternalError(w, "get user product", err)
				return
			}
			if matchedProduct == nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "无有效套餐，请先购买套餐"})
				return
			}
			if domain.LineGroupID == "" {
				domain.LineGroupID = matchedProduct.LineGroupID
				if domain.LineGroupID == "" {
					domain.LineGroupID = matchedProduct.ClusterID
				}
			}
			// Check domain_limit
			if matchedProduct.DomainLimit != nil && *matchedProduct.DomainLimit > 0 {
				count, err := s.store.CountDomainsByUser(ctx, userID)
				if err != nil {
					writeInternalError(w, "count user domains", err)
					return
				}
				if count >= int(*matchedProduct.DomainLimit) {
					writeJSON(w, http.StatusForbidden, map[string]any{"error": fmt.Sprintf("已达域名上限 %d 个", *matchedProduct.DomainLimit)})
					return
				}
			}
			// Check primary_domain_limit
			if isPrimaryDomain(domain.Name) && matchedProduct.PrimaryDomainLimit != nil && *matchedProduct.PrimaryDomainLimit > 0 {
				userDomains, err := s.store.ListDomainsByUser(ctx, userID)
				if err != nil {
					writeInternalError(w, "list user domains", err)
					return
				}
				primaryCount := 0
				for _, ud := range userDomains {
					if isPrimaryDomain(ud.Name) {
						primaryCount++
					}
				}
				if primaryCount >= int(*matchedProduct.PrimaryDomainLimit) {
					writeJSON(w, http.StatusForbidden, map[string]any{"error": fmt.Sprintf("已达主域名上限 %d 个", *matchedProduct.PrimaryDomainLimit)})
					return
				}
			}
			// Check websocket
			if domain.WebsocketEnabled && !matchedProduct.Websocket {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "当前套餐不支持 WebSocket"})
				return
			}
			// Check monthly traffic limit
			if matchedProduct.MonthlyTrafficBytes != nil && *matchedProduct.MonthlyTrafficBytes > 0 {
				currentMonth := time.Now().Format("2006-01")
				usedBytes, err := s.store.GetUserTraffic(ctx, userID, currentMonth)
				if err != nil {
					writeInternalError(w, "get user traffic", err)
					return
				}
				if usedBytes >= *matchedProduct.MonthlyTrafficBytes {
					writeJSON(w, http.StatusForbidden, map[string]any{"error": "当月流量已用尽，无法创建新域名"})
					return
				}
			}
		}
		// For admin users: auto-resolve line_group_id from user's product if not provided
		if role == "admin" && domain.LineGroupID == "" && domain.UserID != "" {
			matchedProduct, err := s.getUserActiveProduct(ctx, domain.UserID, "")
			if err == nil && matchedProduct != nil {
				domain.LineGroupID = matchedProduct.LineGroupID
				if domain.LineGroupID == "" {
					domain.LineGroupID = matchedProduct.ClusterID
				}
			}
		}
		// Admin convenience fallback: when the admin payload omits
		// line_group_id (UI bug, missing product cluster binding, or direct API
		// call), bind the new domain to the first enabled cluster found. This
		// turns an opaque 400 into a working default on fresh installs and also
		// covers multi-cluster environments where the admin forgot to pick one.
		// The fallback only triggers when explicit selection is missing — an
		// admin who deliberately supplies a (valid) line_group_id still wins.
		if role == "admin" && domain.LineGroupID == "" {
			if clusters, err := s.store.ListClusters(ctx); err == nil {
				for _, c := range clusters {
					if c == nil {
						continue
					}
					if c.Enabled {
						domain.LineGroupID = c.ID
						break
					}
				}
				// No enabled cluster? Still try any cluster so the admin can at
				// least save and enable it later via the cluster management page.
				if domain.LineGroupID == "" {
					for _, c := range clusters {
						if c == nil {
							continue
						}
						domain.LineGroupID = c.ID
						break
					}
				}
			}
		}
		if domain.LineGroupID == "" {
			// Merge diagnostic context into the single `error` field so both the
			// new UI (which displays { error, hint }) and the older UI (which
			// surfaces only `error`) expose the same actionable guidance.
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": fmt.Sprintf("line_group_id required（role=%s user_id=%q origin_id=%q name=%q）— 系统中未配置任何集群，请先到'集群管理'创建至少一个集群后再创建网站", role, domain.UserID, domain.OriginID, domain.Name),
			})
			return
		}
		if lg, err := s.store.GetCluster(ctx, domain.LineGroupID); err != nil {
			writeInternalError(w, "get cluster", err)
			return
		} else if lg == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "线路组不存在"})
			return
		}
		// Check domain blacklist
		if blacklisted, reason, err := s.store.IsDomainBlacklisted(ctx, domain.Name); err != nil {
			writeInternalError(w, "check domain blacklist", err)
			return
		} else if blacklisted {
			msg := "domain is blacklisted"
			if reason != "" {
				msg = msg + ": " + reason
			}
			writeJSON(w, http.StatusForbidden, map[string]any{"error": msg})
			return
		}
		if domain.CNAME == "" {
			domain.CNAME = s.generateDomainCNAMEForZone(domain.Name, s.lookupClusterZone(ctx, domain.LineGroupID))
		}
		if domain.CNAME == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "生成CNAME失败"})
			return
		}
		if err := s.ensureDomainUnique(ctx, domain, ""); err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		certOwner := domain.UserID
		if role != "admin" {
			certOwner = userID
		}
		if err := s.validateDomainCertBinding(ctx, role, certOwner, domain.Name, domain.CertID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if cacheProvided {
			domain.CacheEnabled = cacheEnabled
		} else {
			domain.CacheEnabled = true
		}
		if http2Provided {
			domain.HTTP2Enabled = http2Enabled
		} else {
			domain.HTTP2Enabled = true
		}
		if wsProvided {
			domain.WebsocketEnabled = wsEnabled
		}
		if httpsProvided {
			domain.HTTPSEnabled = httpsEnabled
		} else {
			// On create, default HTTPS to on when a cert is supplied up-front;
			// otherwise leave it off and let the operator explicitly opt in
			// (e.g. after requesting an ACME cert).
			domain.HTTPSEnabled = strings.TrimSpace(domain.CertID) != ""
		}
		domain.Enabled = true
		domain.CreatedAt = time.Now()
		domain.UpdatedAt = time.Now()

		if err := s.store.CreateDomain(ctx, &domain); err != nil {
			writeInternalError(w, "create domain", err)
			return
		}
		syncIDs := s.triggerDomainSync(ctx, domain.ID, "create", domain.Name)
		writeJSON(w, http.StatusCreated, struct {
			*store.Domain
			DNSWarnings []string `json:"dns_warnings,omitempty"`
			SyncTaskIDs []string `json:"sync_task_ids,omitempty"`
		}{Domain: &domain, DNSWarnings: s.evaluateDomainDNSHealth(ctx, &domain), SyncTaskIDs: syncIDs})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleDomainByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := getUserRole(ctx)
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	id := strings.TrimPrefix(r.URL.Path, "/api/domains/")

	// Sub-resource routing: /api/domains/{id}/security. Split early so the
	// ownership check below still uses the real domain ID.
	var subResource string
	if idx := strings.Index(id, "/"); idx >= 0 {
		subResource = id[idx+1:]
		id = id[:idx]
	}

	// For non-admin users, verify domain ownership
	if role != "admin" {
		existing, err := s.store.GetDomain(ctx, id)
		if err != nil {
			writeInternalError(w, "get domain", err)
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "域名不存在"})
			return
		}
		if existing.UserID != userID {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权操作此域名"})
			return
		}
	}

	// Dispatch security sub-resource before the main CRUD switch.
	if subResource == "security" {
		s.handleDomainSecurity(w, r, id)
		return
	}
	if subResource == "origins" {
		s.handleDomainOrigins(w, r, id)
		return
	}
	if subResource == "sla" {
		s.handleDomainSLA(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		domain, err := s.store.GetDomain(ctx, id)
		if err != nil {
			writeInternalError(w, "get domain", err)
			return
		}
		if domain == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "域名不存在"})
			return
		}
		writeJSON(w, http.StatusOK, domain)

	case http.MethodPut:
		if role != "admin" && !s.requireUserPermission(w, ctx, PermDomainsWrite) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求内容格式错误"})
			return
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		var domain store.Domain
		if err := json.Unmarshal(body, &domain); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		cacheProvided, cacheEnabled, err := parseOptionalBoolField(payload, "cache_enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的缓存启用值"})
			return
		}
		http2Provided, http2Enabled, err := parseOptionalBoolField(payload, "http2_enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的HTTP2启用值"})
			return
		}
		wsProvided, wsEnabled, err := parseOptionalBoolField(payload, "websocket_enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的WebSocket启用值"})
			return
		}
		httpsProvided, httpsEnabled, err := parseOptionalBoolField(payload, "https_enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的HTTPS启用值"})
			return
		}
		listenPortProvided, listenPort, err := parseOptionalInt32Field(payload, "listen_port")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的监听端口"})
			return
		}
		enabledProvided, enabledVal, err := parseOptionalBoolField(payload, "enabled")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的启用状态"})
			return
		}
		_, lineGroupProvided := payload["line_group_id"]
		_, nameProvided := payload["name"]
		domain.ID = id
		// Preserve user_id: non-admin users can't change ownership
		if role != "admin" {
			domain.UserID = userID
			// Validate product limits on update
			matchedProduct, err := s.getUserActiveProduct(ctx, userID, domain.LineGroupID)
			if err != nil {
				writeInternalError(w, "get user product", err)
				return
			}
			if matchedProduct == nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "无有效套餐，请先购买套餐"})
				return
			}
			// Check websocket
			if wsProvided && wsEnabled && !matchedProduct.Websocket {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "当前套餐不支持 WebSocket"})
				return
			}
			// Check primary_domain_limit on name change
			newName := strings.TrimSpace(domain.Name)
			if newName != "" && isPrimaryDomain(newName) && matchedProduct.PrimaryDomainLimit != nil && *matchedProduct.PrimaryDomainLimit > 0 {
				existingDomain, err := s.store.GetDomain(ctx, id)
				if err != nil {
					writeInternalError(w, "get existing domain", err)
					return
				}
				// Only check if changing from non-primary to primary
				if existingDomain != nil && !isPrimaryDomain(existingDomain.Name) {
					userDomains, err := s.store.ListDomainsByUser(ctx, userID)
					if err != nil {
						writeInternalError(w, "list user domains", err)
						return
					}
					primaryCount := 0
					for _, ud := range userDomains {
						if isPrimaryDomain(ud.Name) {
							primaryCount++
						}
					}
					if primaryCount >= int(*matchedProduct.PrimaryDomainLimit) {
						writeJSON(w, http.StatusForbidden, map[string]any{"error": fmt.Sprintf("已达主域名上限 %d 个", *matchedProduct.PrimaryDomainLimit)})
						return
					}
				}
			}
		}
		domain.Name = strings.TrimSpace(domain.Name)
		domain.CNAME = strings.TrimSpace(domain.CNAME)
		domain.LineGroupID = strings.TrimSpace(domain.LineGroupID)
		domain.OriginID = strings.TrimSpace(domain.OriginID)
		domain.CertID = strings.TrimSpace(domain.CertID)
		if err := normalizeDomainOrigin(&domain); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if pages, err := normalizeErrorPages(domain.ErrorPages); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		} else {
			domain.ErrorPages = pages
		}

		var existing *store.Domain
		if !nameProvided || !lineGroupProvided || !cacheProvided || !http2Provided || !wsProvided || !httpsProvided || !listenPortProvided || !enabledProvided {
			var err error
			existing, err = s.store.GetDomain(ctx, id)
			if err != nil {
				writeInternalError(w, "get domain", err)
				return
			}
			if existing == nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "域名不存在"})
				return
			}
		}
		if !nameProvided && existing != nil {
			domain.Name = existing.Name
		}
		if !lineGroupProvided && existing != nil {
			domain.LineGroupID = existing.LineGroupID
		}
		domain.Name = strings.TrimSpace(domain.Name)
		domain.LineGroupID = strings.TrimSpace(domain.LineGroupID)

		if domain.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		if domain.LineGroupID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "线路组ID不能为空"})
			return
		}
		var clusterZone string
		if lg, err := s.store.GetCluster(ctx, domain.LineGroupID); err != nil {
			writeInternalError(w, "get cluster", err)
			return
		} else if lg == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "线路组不存在"})
			return
		} else {
			clusterZone = strings.Trim(strings.TrimSpace(lg.DNSZone), ".")
		}
		if domain.CNAME == "" {
			domain.CNAME = s.generateDomainCNAMEForZone(domain.Name, clusterZone)
		}
		if err := s.ensureDomainUnique(ctx, domain, id); err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		certOwner := domain.UserID
		if role != "admin" {
			certOwner = userID
		}
		if err := s.validateDomainCertBinding(ctx, role, certOwner, domain.Name, domain.CertID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		_, securityProvided := payload["security"]
		_, originAuthProvided := payload["origin_auth"]
		_, loadBalanceProvided := payload["load_balance_method"]
		_, healthCheckProvided := payload["origin_health_check"]

		if existing == nil && (!cacheProvided || !http2Provided || !wsProvided || !httpsProvided || !listenPortProvided || !enabledProvided || !securityProvided || !originAuthProvided ||
			!loadBalanceProvided || !healthCheckProvided) {
			var err error
			existing, err = s.store.GetDomain(ctx, id)
			if err != nil {
				writeInternalError(w, "get domain", err)
				return
			}
		}

		if cacheProvided {
			domain.CacheEnabled = cacheEnabled
		} else if existing != nil {
			domain.CacheEnabled = existing.CacheEnabled
		} else {
			domain.CacheEnabled = true
		}

		if http2Provided {
			domain.HTTP2Enabled = http2Enabled
		} else if existing != nil {
			domain.HTTP2Enabled = existing.HTTP2Enabled
		} else {
			domain.HTTP2Enabled = true
		}
		if wsProvided {
			domain.WebsocketEnabled = wsEnabled
		} else if existing != nil {
			domain.WebsocketEnabled = existing.WebsocketEnabled
		}
		if httpsProvided {
			domain.HTTPSEnabled = httpsEnabled
		} else if existing != nil {
			domain.HTTPSEnabled = existing.HTTPSEnabled
		} else {
			// Legacy compatibility: pre-field records effectively had HTTPS
			// enabled whenever a cert was present. Preserve that assumption
			// for requests that omit the new flag.
			domain.HTTPSEnabled = strings.TrimSpace(domain.CertID) != ""
		}
		if listenPortProvided {
			if listenPort == nil {
				domain.ListenPort = 0
			} else {
				domain.ListenPort = normalizeListenPort(*listenPort)
			}
		} else if existing != nil {
			domain.ListenPort = existing.ListenPort
		} else {
			domain.ListenPort = normalizeListenPort(domain.ListenPort)
		}
		if enabledProvided {
			domain.Enabled = enabledVal
		} else if existing != nil {
			domain.Enabled = existing.Enabled
		} else {
			domain.Enabled = true
		}
		if !securityProvided && existing != nil {
			domain.Security = existing.Security
		}
		if !originAuthProvided && existing != nil {
			domain.OriginAuth = existing.OriginAuth
		}
		if !loadBalanceProvided && existing != nil {
			domain.LoadBalanceMethod = existing.LoadBalanceMethod
		}
		if !healthCheckProvided && existing != nil {
			domain.OriginHealthCheck = existing.OriginHealthCheck
		}
		if role != "admin" && strings.TrimSpace(domain.CertID) != "" {
			certID, err := strconv.ParseInt(domain.CertID, 10, 64)
			if err != nil || certID <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的证书ID"})
				return
			}
			cert, err := s.store.GetCertificate(ctx, certID)
			if err != nil {
				writeInternalError(w, "get certificate", err)
				return
			}
			if cert == nil || (cert.UserID != "" && cert.UserID != userID) {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权使用该证书"})
				return
			}
		}
		domain.UpdatedAt = time.Now()

		if err := s.store.UpdateDomain(ctx, &domain); err != nil {
			writeInternalError(w, "update domain", err)
			return
		}
		syncIDs := s.triggerDomainSync(ctx, domain.ID, "update", domain.Name)
		writeJSON(w, http.StatusOK, struct {
			*store.Domain
			DNSWarnings []string `json:"dns_warnings,omitempty"`
			SyncTaskIDs []string `json:"sync_task_ids,omitempty"`
		}{Domain: &domain, DNSWarnings: s.evaluateDomainDNSHealth(ctx, &domain), SyncTaskIDs: syncIDs})

	case http.MethodDelete:
		if role != "admin" && !s.requireUserPermission(w, ctx, PermDomainsWrite) {
			return
		}
		if err := s.store.DeleteDomain(ctx, id); err != nil {
			writeInternalError(w, "delete domain", err)
			return
		}
		syncIDs := s.triggerDomainSync(ctx, id, "delete", id)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sync_task_ids": syncIDs})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// handleDomainSecurity serves /api/domains/{id}/security:
//
//	GET  → returns Domain.Security (or an empty, zero-valued object if unset
//	       so the UI form can bind deterministically).
//	PUT  → replaces Domain.Security and re-saves the domain; an auto-publish
//	       delivers the synthesised WAF policy to nodes.
func (s *Servers) handleDomainSecurity(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	domain, err := s.store.GetDomain(ctx, id)
	if err != nil {
		writeInternalError(w, "get domain", err)
		return
	}
	if domain == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "域名不存在"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		sec := domain.Security
		if sec == nil {
			// No record yet — return a zero-valued struct so the UI form
			// can bind deterministically. Each sub-field is its own
			// switch, so an all-zero security struct produces no edge
			// policy until the operator configures something.
			sec = &store.DomainSecurity{}
		}
		writeJSON(w, http.StatusOK, sec)

	case http.MethodPut:
		if getUserRole(ctx) != "admin" && !s.requireUserPermission(w, ctx, PermWAFEdit) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求内容格式错误"})
			return
		}
		var sec store.DomainSecurity
		if err := json.Unmarshal(body, &sec); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		before := domain.Security
		// Normalise: strip empties, assign IDs to new custom rules so the
		// compiler can generate stable per-rule keys on nodes.
		cleanList := func(xs []string) []string {
			out := make([]string, 0, len(xs))
			for _, x := range xs {
				x = strings.TrimSpace(x)
				if x == "" || strings.HasPrefix(x, "#") {
					continue
				}
				out = append(out, x)
			}
			return out
		}
		sec.IPBlacklist = cleanList(sec.IPBlacklist)
		sec.IPWhitelist = cleanList(sec.IPWhitelist)
		for i := range sec.CustomRules {
			if sec.CustomRules[i].ID == "" {
				sec.CustomRules[i].ID = uuid.NewString()
			}
		}
		if sec.BanSeconds < 0 {
			sec.BanSeconds = 0
		}
		if sec.FailLimit < 0 {
			sec.FailLimit = 0
		}
		domain.Security = &sec
		domain.UpdatedAt = time.Now()
		if err := s.store.UpdateDomain(ctx, domain); err != nil {
			writeInternalError(w, "update domain security", err)
			return
		}
		actor, _ := ctx.Value(ctxKeyUserID).(string)
		if actor == "" {
			actor = "system"
		}
		s.auditConfigChange(ctx, actor, "update", "domain:"+domain.ID+"/security", before, domain.Security)
		_ = s.startPublishTask(ctx, "auto", "domain:"+domain.ID, "domain:security:"+domain.Name, "", nil)
		writeJSON(w, http.StatusOK, domain.Security)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// triggerDomainSync fans out the publish + DNS sync tasks for a domain change
// and returns the task IDs so the client can subscribe to SSE sync events.
// action is "create" | "update" | "delete"; nameOrID is for the Reason string.
func (s *Servers) triggerDomainSync(ctx context.Context, domainID, action, nameOrID string) []string {
	subject := "domain:" + domainID
	ids := make([]string, 0, 1)

	task := s.startPublishTask(ctx, "auto", subject, "domain:"+action+":"+nameOrID, "", nil)
	if task != nil && task.ID != "" {
		ids = append(ids, task.ID)
	}

	// triggerDNSSync is debounced and fire-and-forget; SSE will still stream
	// any DNS task state changes with the same Subject.
	s.triggerDNSSync(subject, "domain:"+action)

	return ids
}
