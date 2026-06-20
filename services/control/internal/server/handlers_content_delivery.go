package server

// Content-delivery admin handlers: origins, cache rules, config-version
// history, one-shot publish, and cache purge (dispatch + query by request
// id). Mutations on origins/cache rules trigger an auto-publish so changes
// land on edge nodes without a separate click.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

// Origins handlers
//
// NOTE (2026-04 refactor): the global `origins` pool is deprecated in
// favor of per-domain origins (see handlers_domain_origins.go). These
// endpoints remain only for admin-side read access to the legacy table
// during the transition. Non-admin callers get an empty list to fix
// the cross-tenant leak, and writes are rejected with 410.
func (s *Servers) handleOrigins(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if getUserRole(ctx) != "admin" {
			writeJSON(w, http.StatusOK, map[string]any{"origins": []any{}})
			return
		}
		origins, err := s.store.ListOrigins(ctx)
		if err != nil {
			writeInternalError(w, "list origins", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"origins": origins})

	case http.MethodPost:
		writeJSON(w, http.StatusGone, map[string]any{
			"error": "全局源站池已停用，请在域名详情页的\"源站\"配置中直接添加回源地址",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleOriginByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := strings.TrimPrefix(r.URL.Path, "/api/origins/")

	// Read-only + admin-only during the domain-origin transition (see
	// handleOrigins comment). Non-admin GETs see 404, writes are Gone.
	switch r.Method {
	case http.MethodGet:
		if getUserRole(ctx) != "admin" {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "源站不存在"})
			return
		}
		origin, err := s.store.GetOrigin(ctx, id)
		if err != nil {
			writeInternalError(w, "get origin", err)
			return
		}
		if origin == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "源站不存在"})
			return
		}
		writeJSON(w, http.StatusOK, origin)

	case http.MethodPut, http.MethodDelete:
		writeJSON(w, http.StatusGone, map[string]any{
			"error": "全局源站池已停用，请在域名详情页的\"源站\"配置中直接管理回源地址",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// Certificates handlers
// --- Certificates --- handlers extracted to handlers_certificates.go

// Cache Rules handlers
func (s *Servers) handleCacheRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		rules, err := s.store.ListCacheRules(ctx)
		if err != nil {
			writeInternalError(w, "list cache rules", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cache_rules": rules})

	case http.MethodPost:
		var rule store.CacheRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if rule.ID == "" {
			rule.ID = uuid.NewString()
		}
		if rule.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		if rule.TTLSeconds == 0 {
			rule.TTLSeconds = 3600
		}
		if rule.HostPattern == "" {
			rule.HostPattern = "*"
		}
		if rule.PathPattern == "" {
			rule.PathPattern = "*"
		}
		if len(rule.Methods) == 0 {
			rule.Methods = []string{"GET"}
		}
		rule.Enabled = true
		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()

		if err := s.store.CreateCacheRule(ctx, &rule); err != nil {
			writeInternalError(w, "create cache rule", err)
			return
		}
		actor, _ := ctx.Value(ctxKeyUserID).(string)
		if actor == "" {
			actor = "system"
		}
		s.auditConfigChange(ctx, actor, "create", "cache_rule:"+rule.ID, nil, rule)
		_ = s.startPublishTask(ctx, "auto", "", "cache.rule.create:"+rule.ID, "", nil)
		writeJSON(w, http.StatusCreated, rule)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleCacheRuleByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := strings.TrimPrefix(r.URL.Path, "/api/cache-rules/")

	switch r.Method {
	case http.MethodGet:
		rule, err := s.store.GetCacheRule(ctx, id)
		if err != nil {
			writeInternalError(w, "get cache rule", err)
			return
		}
		if rule == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "缓存规则不存在"})
			return
		}
		writeJSON(w, http.StatusOK, rule)

	case http.MethodPut:
		before, _ := s.store.GetCacheRule(ctx, id)
		var rule store.CacheRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		rule.ID = id
		rule.UpdatedAt = time.Now()

		if err := s.store.UpdateCacheRule(ctx, &rule); err != nil {
			writeInternalError(w, "update cache rule", err)
			return
		}
		actor, _ := ctx.Value(ctxKeyUserID).(string)
		if actor == "" {
			actor = "system"
		}
		s.auditConfigChange(ctx, actor, "update", "cache_rule:"+id, before, rule)
		_ = s.startPublishTask(ctx, "auto", "", "cache.rule.update:"+id, "", nil)
		writeJSON(w, http.StatusOK, rule)

	case http.MethodDelete:
		before, _ := s.store.GetCacheRule(ctx, id)
		if err := s.store.DeleteCacheRule(ctx, id); err != nil {
			writeInternalError(w, "delete cache rule", err)
			return
		}
		actor, _ := ctx.Value(ctxKeyUserID).(string)
		if actor == "" {
			actor = "system"
		}
		s.auditConfigChange(ctx, actor, "delete", "cache_rule:"+id, before, nil)
		_ = s.startPublishTask(ctx, "auto", "", "cache.rule.delete:"+id, "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// Config handlers
func (s *Servers) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	ctx := r.Context()

	var req struct {
		Version string   `json:"version"`
		NodeIDs []string `json:"node_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	task := s.startPublishTask(ctx, "manual", "", "manual.publish", req.Version, req.NodeIDs)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"message":         "publish scheduled",
		"publish_task_id": task.ID,
	})
}

func (s *Servers) handleConfigVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	ctx := r.Context()
	versions, err := s.store.ListConfigVersions(ctx, 20)
	if err != nil {
		writeInternalError(w, "list config versions", err)
		return
	}

	// Don't include full payload in list
	safeVersions := make([]map[string]any, len(versions))
	for i, v := range versions {
		safeVersions[i] = map[string]any{
			"version":    v.Version,
			"checksum":   v.Checksum,
			"created_at": v.CreatedAt,
			"created_by": v.CreatedBy,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"versions": safeVersions})
}

// Purge handler
func (s *Servers) handlePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	ctx := r.Context()
	if getUserRole(ctx) != "admin" && !s.requireUserPermission(w, ctx, PermPurgeExecute) {
		return
	}

	var req struct {
		URLs      []string `json:"urls"`
		Type      string   `json:"type"`
		Prefixes  []string `json:"prefixes"`
		Tags      []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	purgeType := strings.TrimSpace(strings.ToLower(req.Type))
	if purgeType == "" {
		purgeType = "url"
	}

	switch purgeType {
	case "url":
		if len(req.URLs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "URL列表不能为空"})
			return
		}
	case "prefix":
		if len(req.Prefixes) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录前缀列表不能为空"})
			return
		}
	case "tag":
		if len(req.Tags) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "标签列表不能为空"})
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "不支持的刷新类型"})
		return
	}

	requestID, err := s.purge.PurgeWithOptions(ctx, purgeType, req.URLs, req.Prefixes, req.Tags)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":         false,
			"request_id": requestID,
			"message":    err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"request_id": requestID,
		"type":       purgeType,
		"message":    "purge dispatched",
	})
}

func (s *Servers) handlePurgeByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/purge/")
	id = strings.TrimSpace(id)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求ID不能为空"})
		return
	}
	req, ok := s.purge.GetRequest(id)
	if !ok || req == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "刷新请求不存在"})
		return
	}

	results := make([]map[string]any, 0, len(req.Results))
	for nodeID, res := range req.Results {
		if res == nil {
			continue
		}
		results = append(results, map[string]any{
			"node_id":   nodeID,
			"ok":        res.OK,
			"reason":    res.Reason,
			"timestamp": res.Timestamp,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           req.ID,
		"urls":         req.URLs,
		"started_at":   req.StartedAt,
		"completed_at": req.CompletedAt,
		"total_nodes":  req.TotalNodes,
		"results":      results,
	})
}

func (s *Servers) handlePreload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if s.preload == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "preload service unavailable"})
		return
	}

	ctx := r.Context()
	var req struct {
		URLs []string `json:"urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}
	if len(req.URLs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "URL列表不能为空"})
		return
	}

	requestID, err := s.preload.PreloadURLs(ctx, req.URLs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":         false,
			"request_id": requestID,
			"message":    err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"request_id": requestID,
		"message":    "preload dispatched",
	})
}

func (s *Servers) handlePreloadByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if s.preload == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "preload service unavailable"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/preload/")
	id = strings.TrimSpace(id)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求ID不能为空"})
		return
	}
	req, ok := s.preload.GetRequest(id)
	if !ok || req == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "预热请求不存在"})
		return
	}

	results := make([]map[string]any, 0, len(req.Results))
	for nodeID, res := range req.Results {
		if res == nil {
			continue
		}
		results = append(results, map[string]any{
			"node_id":   nodeID,
			"ok":        res.OK,
			"reason":    res.Reason,
			"loaded":    res.Loaded,
			"timestamp": res.Timestamp,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           req.ID,
		"urls":         req.URLs,
		"started_at":   req.StartedAt,
		"completed_at": req.CompletedAt,
		"total_nodes":  req.TotalNodes,
		"results":      results,
	})
}
