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
	"net/http"
	"net/url"
	"strings"
	"time"
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
		"status":             resp.StatusCode,
		"body":               data,
		"templates_applied":  applied,
	})
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
	settings := s.resolveSettings(r.Context())
	esURL := strings.TrimSpace(settings.ElasticsearchURL)
	if esURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ES_URL未配置"})
		return
	}

	var req struct {
		Index     string   `json:"index"`
		Query     string   `json:"query"`
		Domain    string   `json:"domain"`
		IP        string   `json:"ip"`
		Status    int      `json:"status"`
		From      string   `json:"from"`
		To        string   `json:"to"`
		Size      int      `json:"size"`
		FromHit   int      `json:"from_hit"`
		Fields    []string `json:"fields"`
		Highlight bool     `json:"highlight"`
		TimeoutMs int      `json:"timeout_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
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
	domainField := strings.TrimSpace(settings.ElasticsearchDomainField)
	if domainField == "" {
		domainField = "domain.keyword"
	}

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
				"client_ip.keyword": req.IP,
			},
		})
	}
	if req.Status > 0 {
		must = append(must, map[string]any{
			"term": map[string]any{
				"status": req.Status,
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
	// The full raw response is also returned so consumers can access aggs
	// or _shards info without a second request.
	hits := []map[string]any{}
	if h, ok := data["hits"].(map[string]any); ok {
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

	writeJSON(w, http.StatusOK, map[string]any{
		"status": resp.StatusCode,
		"hits":   hits,
		"raw":    data,
	})
}
