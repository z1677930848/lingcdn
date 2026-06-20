package server

// Elasticsearch integration for the control-plane UI. Two endpoints:
//   - /api/es/health   → proxies GET _cluster/health with short 5s timeout
//   - /api/es/search   → builds a bool query with optional domain/IP/status/
//                        time filters, caps size/timeout to safe values
// These read ES config from settings (resolveSettings) rather than s.cfg so
// admins can reconfigure ES at runtime via the Settings page.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// handleESHealth proxies GET _cluster/health to the configured ES cluster.
// The 5s timeout keeps this endpoint safe to call from status pages even
// when ES is degraded.
func (s *Servers) handleESHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	settings := s.resolveSettings(r.Context())
	esURL := strings.TrimSpace(settings.ElasticsearchURL)
	if esURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ES_URL未配置"})
		return
	}

	u, err := url.Parse(esURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的ES_URL"})
		return
	}

	target := strings.TrimRight(esURL, "/") + "/_cluster/health"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeInternalError(w, "build ES health request", err)
		return
	}
	if settings.ElasticsearchUser != "" {
		req.SetBasicAuth(settings.ElasticsearchUser, settings.ElasticsearchPass)
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "ES返回响应无效"})
		return
	}

	// Opportunistically push the cdn-access / cdn-error index templates
	// so a "Test connection" click also seeds the cluster's mappings.
	// Best-effort; failures are logged inside pushESIndexTemplates.
	applied := pushESIndexTemplates(r.Context(), settings)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":            resp.StatusCode,
		"body":              data,
		"templates_applied": applied,
	})
}

// handleLogsStatus exposes whether ES log search is configured (authenticated users).
func (s *Servers) handleLogsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	settings := s.resolveSettings(r.Context())
	configured := strings.TrimSpace(settings.ElasticsearchURL) != ""
	healthy := false
	if configured {
		healthy = s.pingESCluster(r.Context(), settings) == nil
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": configured,
		"healthy":    healthy,
	})
}

func (s *Servers) pingESCluster(ctx context.Context, settings *store.Settings) error {
	if settings == nil {
		return fmt.Errorf("es not configured")
	}
	esURL := strings.TrimSpace(settings.ElasticsearchURL)
	if esURL == "" {
		return fmt.Errorf("es not configured")
	}
	target := strings.TrimRight(esURL, "/") + "/_cluster/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	if settings.ElasticsearchUser != "" {
		req.SetBasicAuth(settings.ElasticsearchUser, settings.ElasticsearchPass)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("es health status %d", resp.StatusCode)
	}
	return nil
}

// handleLogsSearch builds an ES _search query from simple UI-level filters
// (query_string + optional domain/ip/status term filters + time range).
// Defensive limits: query body capped at 1024 chars; size clamped to
// [1, 500]; timeout clamped to [0, 15s].
func (s *Servers) handleLogsSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx := r.Context()
	settings := s.resolveSettings(ctx)
	esURL := strings.TrimSpace(settings.ElasticsearchURL)
	if esURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ES_URL未配置"})
		return
	}

	var req struct {
		Index       string   `json:"index"`
		Query       string   `json:"query"`
		Domain      string   `json:"domain"`
		IP          string   `json:"ip"`
		Status      any      `json:"status"`
		From        string   `json:"from"`
		To          string   `json:"to"`
		Size        int      `json:"size"`
		FromHit     int      `json:"from_hit"`
		Fields      []string `json:"fields"`
		Highlight   bool     `json:"highlight"`
		TimeoutMs   int      `json:"timeout_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	var domainAllowlist []string
	if !isAdmin(ctx) {
		userID := getUserID(ctx)
		if userID == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
			return
		}
		domains, err := s.store.ListDomainsByUser(ctx, userID)
		if err != nil {
			writeInternalError(w, "list user domains", err)
			return
		}
		for _, d := range domains {
			if d == nil {
				continue
			}
			name := strings.TrimSpace(d.Name)
			if name != "" {
				domainAllowlist = append(domainAllowlist, name)
			}
		}
		if len(domainAllowlist) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "total": 0, "hits": []any{}})
			return
		}
		filterDomain := strings.TrimSpace(req.Domain)
		if filterDomain != "" {
			allowed := false
			for _, n := range domainAllowlist {
				if strings.EqualFold(n, filterDomain) {
					allowed = true
					req.Domain = n
					break
				}
			}
			if !allowed {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权查询该域名的访问日志"})
				return
			}
		}
	}
	if len(req.Query) > 1024 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "查询内容过长"})
		return
	}
	if req.Size <= 0 || req.Size > 200 {
		req.Size = 100
	}
	if req.Size > 500 {
		req.Size = 500
	}
	if req.FromHit < 0 {
		req.FromHit = 0
	}
	index := strings.TrimSpace(req.Index)
	if index == "" {
		index = strings.TrimSpace(settings.ElasticsearchIndex)
	}
	if index == "" {
		index = "cdn-access"
	}
	// The shipper writes to date-suffixed indices (cdn-access-YYYY.MM.DD).
	// If the configured value is a bare prefix, expand to a wildcard so
	// _search hits every day's index.
	if !strings.Contains(index, "*") && !strings.Contains(index, ",") {
		index = index + "-*"
	}
	tsField := strings.TrimSpace(settings.ElasticsearchTSField)
	if tsField == "" {
		tsField = "@timestamp"
	}
	domainField := resolveESDomainField(settings)
	clientIPField := resolveESClientIPField()

	must := []map[string]any{}
	if req.Query != "" {
		must = append(must, map[string]any{
			"query_string": map[string]any{
				"query": req.Query,
			},
		})
	}
	if req.Domain != "" {
		must = append(must, map[string]any{
			"term": map[string]any{
				domainField: req.Domain,
			},
		})
	}
	if req.IP != "" {
		must = append(must, map[string]any{
			"term": map[string]any{
				clientIPField: req.IP,
			},
		})
	}
	if statusRange := parseLogStatusFilter(req.Status); statusRange != nil {
		must = append(must, map[string]any{
			"range": map[string]any{
				"status": statusRange,
			},
		})
	}
	if len(domainAllowlist) > 0 && strings.TrimSpace(req.Domain) == "" {
		must = append(must, map[string]any{
			"terms": map[string]any{
				domainField: domainAllowlist,
			},
		})
	}
	timeRange := map[string]any{}
	if req.From != "" {
		timeRange["gte"] = req.From
	}
	if req.To != "" {
		timeRange["lte"] = req.To
	}
	if len(timeRange) > 0 {
		must = append(must, map[string]any{
			"range": map[string]any{
				tsField: timeRange,
			},
		})
	}

	body := map[string]any{
		"size": req.Size,
		"from": req.FromHit,
		"sort": []map[string]any{
			{tsField: map[string]any{"order": "desc"}},
		},
		"query": map[string]any{
			"bool": map[string]any{
				"must": must,
			},
		},
		"track_total_hits": true,
	}
	if len(req.Fields) > 0 {
		body["_source"] = req.Fields
	}
	if req.Highlight {
		fields := map[string]any{}
		for _, f := range []string{"message", "log", "request", "uri", "path", "url"} {
			fields[f] = map[string]any{}
		}
		body["highlight"] = map[string]any{
			"pre_tags":  []string{"<mark>"},
			"post_tags": []string{"</mark>"},
			"fields":    fields,
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		writeInternalError(w, "encode ES query", err)
		return
	}

	target := strings.TrimRight(esURL, "/") + "/" + index + "/_search"
	reqTimeout := 8 * time.Second
	if req.TimeoutMs > 0 && req.TimeoutMs <= 15000 {
		reqTimeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(r.Context(), reqTimeout)
	defer cancel()
	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, target, &buf)
	if err != nil {
		writeInternalError(w, "build ES search request", err)
		return
	}
	reqHTTP.Header.Set("Content-Type", "application/json")
	if settings.ElasticsearchUser != "" {
		reqHTTP.SetBasicAuth(settings.ElasticsearchUser, settings.ElasticsearchPass)
	}

	httpClient := &http.Client{Timeout: reqTimeout}
	resp, err := httpClient.Do(reqHTTP)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "ES返回响应无效"})
		return
	}

	// Flatten hits into a lighter-weight view that the UI renders directly.
	hits := []map[string]any{}
	var total int64
	if h, ok := data["hits"].(map[string]any); ok {
		if t, ok := h["total"].(map[string]any); ok {
			if v, ok := t["value"].(float64); ok {
				total = int64(v)
			}
		} else if v, ok := h["total"].(float64); ok {
			total = int64(v)
		}
		if arr, ok := h["hits"].([]any); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					entry := map[string]any{
						"_id": m["_id"],
					}
					if src, ok := m["_source"].(map[string]any); ok {
						entry["source"] = src
					}
					if hl, ok := m["highlight"].(map[string]any); ok {
						entry["highlight"] = hl
					}
					hits = append(hits, entry)
				}
			}
		}
	}

	entries := flattenLogEntries(hits)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  resp.StatusCode,
		"hits":    hits,
		"entries": entries,
		"total":   total,
		"raw":     data,
	})
}

func parseLogStatusFilter(raw any) map[string]any {
	switch v := raw.(type) {
	case float64:
		n := int(v)
		if n > 0 {
			return map[string]any{"gte": n, "lte": n}
		}
	case int:
		if v > 0 {
			return map[string]any{"gte": v, "lte": v}
		}
	case int64:
		if v > 0 {
			n := int(v)
			return map[string]any{"gte": n, "lte": n}
		}
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		switch s {
		case "2xx":
			return map[string]any{"gte": 200, "lte": 299}
		case "3xx":
			return map[string]any{"gte": 300, "lte": 399}
		case "4xx":
			return map[string]any{"gte": 400, "lte": 499}
		case "5xx":
			return map[string]any{"gte": 500, "lte": 599}
		default:
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				return map[string]any{"gte": n, "lte": n}
			}
		}
	}
	return nil
}

func flattenLogEntries(hits []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		src, _ := hit["source"].(map[string]any)
		if src == nil {
			continue
		}
		entry := map[string]any{
			"id": hit["_id"],
		}
		for _, key := range []string{"domain", "path", "method", "client_ip", "bytes", "timestamp", "ua", "@timestamp"} {
			if val, ok := src[key]; ok && val != nil {
				entry[key] = val
			}
		}
		if ts, ok := entry["timestamp"]; !ok || ts == nil || ts == "" {
			if ts2, ok := src["@timestamp"]; ok {
				entry["timestamp"] = ts2
			}
		}
		if st, ok := src["status"]; ok {
			switch n := st.(type) {
			case float64:
				entry["status"] = int(n)
			case int:
				entry["status"] = n
			case json.Number:
				if i, err := n.Int64(); err == nil {
					entry["status"] = int(i)
				}
			default:
				entry["status"] = st
			}
		}
		out = append(out, entry)
	}
	return out
}
