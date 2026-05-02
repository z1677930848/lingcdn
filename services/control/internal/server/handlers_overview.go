package server

// Overview dashboard handler plus the data-shaping helpers it depends on.
// The dashboard's data has two sources:
//   1. Node telemetry from the store (buildNetworkRegionsFromNodes, etc.).
//   2. Elasticsearch aggregations when ES is configured (fetchESOverview).
// ES data supersedes the telemetry-only view when available; both are kept so
// the overview still renders when ES is unreachable or unconfigured.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

// overviewResponse captures the payload for the service overview dashboard.
type overviewResponse struct {
	Summary      overviewSummary         `json:"summary"`
	SystemStatus []overviewStatus        `json:"system_status"`
	Network      overviewNetwork         `json:"network"`
	Trends       []overviewTrendSeries   `json:"trends"`
	TopDomains   []overviewTopDomain     `json:"top_domains"`
	DemoData     bool                    `json:"demo_data,omitempty"`
	License      overviewLicense         `json:"license"`
	Usage        overviewUsage           `json:"usage"`
	TrafficMap   []overviewTrafficRegion `json:"traffic_map"`
}

type overviewTrafficRegion struct {
	Name      string `json:"name"`   // GeoJSON country name
	Region    string `json:"region"` // internal region key
	BytesSent int64  `json:"bytes_sent"`
	Requests  int64  `json:"requests"`
}

type overviewSummary struct {
	RegisteredUsers int `json:"registered_users"`
	TotalNodes      int `json:"total_nodes"`
	OnlineNodes     int `json:"online_nodes"`
	Domains         int `json:"domains"`
	Certificates    int `json:"certificates"`
}

type overviewStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"` // healthy | warning | critical
	Detail string `json:"detail,omitempty"`
}

type overviewNetwork struct {
	TotalNodes     int              `json:"total_nodes"`
	OnlineNodes    int              `json:"online_nodes"`
	ConnectedNodes int              `json:"connected_nodes"`
	OfflineNodes   int              `json:"offline_nodes"`
	Regions        []overviewRegion `json:"regions"`
}

type overviewRegion struct {
	Name      string `json:"name"`
	Nodes     int    `json:"nodes"`
	LatencyMs int    `json:"latency_ms"`
}

type overviewTrendSeries struct {
	Key    string               `json:"key"`
	Name   string               `json:"name"`
	Unit   string               `json:"unit"`
	Points []overviewTrendPoint `json:"points"`
}

type overviewTrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type overviewTopDomain struct {
	Domain         string  `json:"domain"`
	URL            string  `json:"url"`
	Requests       int64   `json:"requests"`
	BandwidthMbps  float64 `json:"bandwidth_mbps"`
	CacheHitRate   float64 `json:"cache_hit_rate,omitempty"`
	OriginErrorPct float64 `json:"origin_error_pct,omitempty"`
}

type overviewLicense struct {
	AuthorizedNodes int       `json:"authorized_nodes"`
	ActiveNodes     int       `json:"active_nodes"`
	ExpiresAt       time.Time `json:"expires_at"`
	Status          string    `json:"status,omitempty"`
	Reason          string    `json:"reason,omitempty"`
}

type overviewUsage struct {
	Domains        int `json:"domains"`
	Certificates   int `json:"certificates"`
	CacheRules     int `json:"cache_rules"`
	ConfigVersions int `json:"config_versions"`
}

// esOverview aggregates metrics fetched from Elasticsearch.
type esOverview struct {
	Requests    int64
	Bytes       float64
	Trends      []overviewTrendSeries
	TopDomains  []overviewTopDomain
	WindowStart time.Time
	WindowEnd   time.Time
}

// handleOverview serves the /api/overview dashboard payload. The request is
// best-effort with respect to ES: if ES is configured but unreachable we log a
// warning and still return the store-derived view rather than erroring.
func (s *Servers) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	window := normalizeWindow(r.URL.Query().Get("window"))
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	settings := s.resolveSettings(ctx)

	var esData *esOverview
	var esErr error
	if strings.TrimSpace(settings.ElasticsearchURL) != "" {
		esData, esErr = s.fetchESOverview(ctx, window)
		if esErr != nil {
			log.Warn().Err(esErr).Msg("failed to fetch overview from ES")
		}
	}

	users, err := s.store.ListUsers(ctx, 0)
	if err != nil {
		writeInternalError(w, "list users", err)
		return
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		writeInternalError(w, "list nodes", err)
		return
	}
	domains, err := s.store.ListDomains(ctx)
	if err != nil {
		writeInternalError(w, "list domains", err)
		return
	}
	certs, err := s.store.ListCertificates(ctx)
	if err != nil {
		writeInternalError(w, "list certificates", err)
		return
	}
	cacheRules, err := s.store.ListCacheRules(ctx)
	if err != nil {
		writeInternalError(w, "list cache rules", err)
		return
	}
	versions, err := s.store.ListConfigVersions(ctx, 20)
	if err != nil {
		writeInternalError(w, "list config versions", err)
		return
	}

	// Count online/degraded/offline by combining status field + heartbeat. A
	// node that hasn't heartbeat'd in 5 minutes is treated as offline even if
	// the stored status still says "online".
	now := time.Now()
	onlineNodes := 0
	degradedNodes := 0
	for _, n := range nodes {
		if strings.ToLower(n.Status) == "degraded" {
			degradedNodes++
			continue
		}
		if !n.LastHeartbeat.IsZero() && now.Sub(n.LastHeartbeat) < 5*time.Minute {
			onlineNodes++
			continue
		}
		switch strings.ToLower(n.Status) {
		case "online":
			onlineNodes++
		}
	}
	connectedNodes := s.hub.Count()
	if onlineNodes < connectedNodes {
		onlineNodes = connectedNodes
	}
	offlineNodes := len(nodes) - onlineNodes - degradedNodes
	if offlineNodes < 0 {
		offlineNodes = 0
	}

	if s.metrics != nil {
		s.metrics.SetNodesRegistered(len(nodes))
		s.metrics.SetNodesConnected(connectedNodes)
		s.metrics.SetNodesByStatus("online", onlineNodes)
		s.metrics.SetNodesByStatus("degraded", degradedNodes)
		s.metrics.SetNodesByStatus("offline", offlineNodes)
	}

	systemStatus := s.buildSystemStatus(ctx, connectedNodes)
	if strings.TrimSpace(settings.ElasticsearchURL) != "" {
		esStatus := overviewStatus{Name: "elasticsearch", Status: "healthy", Detail: "overview stats from ES"}
		if esErr != nil || esData == nil {
			esStatus.Status = "warning"
			if esErr != nil {
				esStatus.Detail = esErr.Error()
			} else {
				esStatus.Detail = "no data"
			}
		}
		systemStatus = append(systemStatus, esStatus)
	}

	st := s.ensureLicenseStatus()
	// authorizedNodes 反映 license 的真实节点上限：MaxNodes==0 表示无授权或无上限，
	// 此处直接透传 0，让前端显示"不限/未授权"。历史实现把 0 fallback 成 len(nodes)，
	// 会让管理员误以为"已达上限"而申请扩容（实际上是后端无限制）。
	authorizedNodes := st.MaxNodes
	resp := overviewResponse{
		Summary: overviewSummary{
			RegisteredUsers: len(users),
			TotalNodes:      len(nodes),
			OnlineNodes:     onlineNodes,
			Domains:         len(domains),
			Certificates:    len(certs),
		},
		SystemStatus: systemStatus,
		Network: overviewNetwork{
			TotalNodes:     len(nodes),
			OnlineNodes:    onlineNodes,
			ConnectedNodes: connectedNodes,
			OfflineNodes:   offlineNodes,
			Regions:        buildNetworkRegionsFromNodes(nodes),
		},
		Trends:     buildOverviewTrendsFromNodes(nodes, window),
		TopDomains: buildTopDomainsFromNodes(domains, nodes),
		DemoData:   false,
		License: overviewLicense{
			AuthorizedNodes: authorizedNodes,
			ActiveNodes:     onlineNodes,
			ExpiresAt:       st.ExpiresAt,
			Status:          st.Status,
			Reason:          st.Reason,
		},
		Usage: overviewUsage{
			Domains:        len(domains),
			Certificates:   len(certs),
			CacheRules:     len(cacheRules),
			ConfigVersions: len(versions),
		},
		TrafficMap: buildTrafficMapFromNodes(nodes),
	}

	// ES data wins over node-derived trends/top-domains when available.
	if esData != nil {
		if len(esData.Trends) > 0 {
			resp.Trends = esData.Trends
		}
		if len(esData.TopDomains) > 0 {
			resp.TopDomains = esData.TopDomains
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// buildSystemStatus produces the list of subsystem status rows for the
// overview dashboard. Only the db status is dynamic via Ping; the others are
// reported as always-healthy once the server is serving requests (the ES row
// is appended by handleOverview itself when ES is configured).
func (s *Servers) buildSystemStatus(ctx context.Context, connectedNodes int) []overviewStatus {
	status := []overviewStatus{
		{Name: "control", Status: "healthy", Detail: "control plane API responding"},
	}

	dbStatus := overviewStatus{Name: "database", Status: "healthy", Detail: "store reachable"}
	if s.store == nil {
		dbStatus.Status = "warning"
		dbStatus.Detail = "store not configured"
	} else if err := s.store.Ping(ctx); err != nil {
		dbStatus.Status = "critical"
		dbStatus.Detail = err.Error()
	}
	status = append(status, dbStatus)

	status = append(status, overviewStatus{Name: "publisher", Status: "healthy", Detail: "config publisher ready"})
	if connectedNodes > 0 {
		status = append(status, overviewStatus{Name: "agents", Status: "healthy", Detail: "nodes reporting in"})
	} else {
		status = append(status, overviewStatus{Name: "agents", Status: "warning", Detail: "waiting for nodes to connect"})
	}

	return status
}

// buildNetworkRegionsFromNodes groups nodes into region buckets using
// inferRegion on the hostname. When no nodes are registered, returns a
// placeholder set of regions so the UI renders something instead of an
// empty map.
func buildNetworkRegionsFromNodes(nodes []*store.Node) []overviewRegion {
	if len(nodes) == 0 {
		return []overviewRegion{
			{Name: "cn-east", Nodes: 0, LatencyMs: 38},
			{Name: "cn-north", Nodes: 0, LatencyMs: 42},
			{Name: "cn-south", Nodes: 0, LatencyMs: 37},
			{Name: "ap-southeast", Nodes: 0, LatencyMs: 82},
			{Name: "global-edge", Nodes: 0, LatencyMs: 118},
		}
	}

	counts := make(map[string]int)
	for _, n := range nodes {
		region := inferRegion(n.Hostname)
		counts[region]++
	}

	regions := make([]overviewRegion, 0, len(counts))
	for region, n := range counts {
		latency := regionLatency(region)
		regions = append(regions, overviewRegion{Name: region, Nodes: n, LatencyMs: latency})
	}

	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Name < regions[j].Name
	})

	return regions
}

// inferRegion classifies a node by substrings in its hostname (e.g. "bj" or
// "beijing" → cn-north). Falls back to "global-edge" when no marker matches.
func inferRegion(host string) string {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "bj") || strings.Contains(h, "beijing"):
		return "cn-north"
	case strings.Contains(h, "sh") || strings.Contains(h, "shanghai"):
		return "cn-east"
	case strings.Contains(h, "gz") || strings.Contains(h, "gd") || strings.Contains(h, "guangzhou") || strings.Contains(h, "sz"):
		return "cn-south"
	case strings.Contains(h, "hk") || strings.Contains(h, "hkg") || strings.Contains(h, "hongkong"):
		return "ap-southeast"
	case strings.Contains(h, "sin") || strings.Contains(h, "sg"):
		return "ap-southeast"
	default:
		return "global-edge"
	}
}

// regionLatency returns a representative round-trip latency in ms for each
// region. Values are hard-coded rather than measured because this is used for
// UI labeling only.
func regionLatency(region string) int {
	switch region {
	case "cn-east":
		return 32
	case "cn-north":
		return 41
	case "cn-south":
		return 37
	case "ap-southeast":
		return 82
	default:
		return 95
	}
}

// regionToGeoName maps internal region keys to GeoJSON country names used by the world map.
func regionToGeoName(region, hostname string) string {
	h := strings.ToLower(hostname)
	// Fine-grained hostname-based detection first
	switch {
	case strings.Contains(h, "hk") || strings.Contains(h, "hkg") || strings.Contains(h, "hongkong"):
		return "Hong Kong"
	case strings.Contains(h, "sin") || strings.Contains(h, "sg-") || strings.HasSuffix(h, ".sg"):
		return "Singapore"
	case strings.Contains(h, "jp") || strings.Contains(h, "tokyo"):
		return "Japan"
	case strings.Contains(h, "kr") || strings.Contains(h, "seoul"):
		return "South Korea"
	case strings.Contains(h, "us") || strings.Contains(h, "la") || strings.Contains(h, "sjc") || strings.Contains(h, "nyc"):
		return "United States"
	case strings.Contains(h, "eu") || strings.Contains(h, "de") || strings.Contains(h, "uk") || strings.Contains(h, "fr"):
		return "Europe"
	}
	// Fallback to region key
	switch region {
	case "cn-north", "cn-east", "cn-south", "cn-southwest", "cn-central":
		return "China"
	case "ap-southeast":
		return "Singapore"
	case "ap-northeast":
		return "Japan"
	case "us-west", "us-east":
		return "United States"
	case "eu-west", "eu-central":
		return "Europe"
	default:
		return "China"
	}
}

// buildTrafficMapFromNodes aggregates per-country traffic from node telemetry.
func buildTrafficMapFromNodes(nodes []*store.Node) []overviewTrafficRegion {
	type agg struct {
		bytesSent int64
		requests  int64
	}
	countryAgg := make(map[string]*agg)
	for _, n := range nodes {
		geoName := regionToGeoName(n.Region, n.Hostname)
		a, ok := countryAgg[geoName]
		if !ok {
			a = &agg{}
			countryAgg[geoName] = a
		}
		a.bytesSent += n.BytesSent
	}
	out := make([]overviewTrafficRegion, 0, len(countryAgg))
	for name, a := range countryAgg {
		if a.bytesSent == 0 {
			continue
		}
		out = append(out, overviewTrafficRegion{
			Name:      name,
			Region:    name,
			BytesSent: a.bytesSent,
			Requests:  a.requests,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BytesSent > out[j].BytesSent
	})
	return out
}

// buildOverviewTrendsFromNodes creates trend series using real-time node telemetry.
// It reads each node's BandwidthUpBps (bytes/sec outgoing) to derive total cluster bandwidth,
// and aggregates TCP connections as a proxy for request rate. The series covers the selected
// time window (24h/7d/30d) but because the system only stores a single latest telemetry
// snapshot per node (not historical), the chart shows one "current" value repeated as a
// flat line. When Elasticsearch is configured, it replaces these with real time-series data.
func buildOverviewTrendsFromNodes(nodes []*store.Node, window string) []overviewTrendSeries {
	points := 12
	step := 2 * time.Hour
	switch window {
	case "7d":
		points = 14
		step = 12 * time.Hour
	case "30d":
		points = 15
		step = 24 * time.Hour
	}

	// Aggregate real-time metrics from all online nodes.
	var totalBandwidthBps float64 // bytes/sec
	var totalConnections int64
	now := time.Now()
	for _, n := range nodes {
		// Only include nodes that reported telemetry recently (within 5 minutes).
		if n.LastMetricsAt.IsZero() || now.Sub(n.LastMetricsAt) > 5*time.Minute {
			continue
		}
		totalBandwidthBps += n.BandwidthUpBps
		totalConnections += int64(n.TCPEstablished)
	}

	// Convert bandwidth: bytes/sec -> Mbps
	bandwidthMbps := (totalBandwidthBps * 8) / 1e6

	// Calculate total monthly traffic from all nodes: bytes -> GB
	var totalMonthBytes int64
	for _, n := range nodes {
		totalMonthBytes += n.MonthBytesSent
	}
	trafficGB := float64(totalMonthBytes) / 1e9

	// Requests: use TCP established connections as an approximation.
	requestsCount := float64(totalConnections)

	// Build time-series points: we project the current value across the window.
	start := now.Add(-time.Duration(points-1) * step)

	makeSeries := func(key, name, unit string, value float64) overviewTrendSeries {
		s := overviewTrendSeries{Key: key, Name: name, Unit: unit}
		s.Points = make([]overviewTrendPoint, points)
		for i := 0; i < points; i++ {
			s.Points[i] = overviewTrendPoint{
				Timestamp: start.Add(time.Duration(i) * step),
				Value:     math.Round(value*100) / 100,
			}
		}
		return s
	}

	return []overviewTrendSeries{
		makeSeries("bandwidth", "Bandwidth", "Mbps", bandwidthMbps),
		makeSeries("requests", "Requests", "count", requestsCount),
		makeSeries("traffic", "Traffic", "GB", trafficGB),
	}
}

// makeTrendSeries builds a synthetic trend line using a sine-wave pattern.
// Currently unused; kept because it encodes the historical shape the UI
// used before real ES data was wired in and may be needed for demo mode.
func makeTrendSeries(key, name, unit string, base float64, window string) overviewTrendSeries {
	points := 12
	step := 2 * time.Hour
	switch window {
	case "7d":
		points = 14
		step = 12 * time.Hour
	case "30d":
		points = 15
		step = 24 * time.Hour
	}

	series := overviewTrendSeries{
		Key:  key,
		Name: name,
		Unit: unit,
	}

	start := time.Now().Add(-time.Duration(points-1) * step)
	series.Points = make([]overviewTrendPoint, points)
	for i := 0; i < points; i++ {
		jitter := 0.08 * math.Sin(float64(i)*0.9)
		seasonal := 0.12 * math.Sin(float64(i)/3.0)
		val := base * (1 + jitter + seasonal)
		if val < 0 {
			val = 0
		}
		series.Points[i] = overviewTrendPoint{
			Timestamp: start.Add(time.Duration(i) * step),
			Value:     math.Round(val*100) / 100,
		}
	}

	return series
}

// buildTopDomainsFromNodes creates a top-domains list using real system data.
// Without Elasticsearch, per-domain request/bandwidth data is not available, so
// we list domains with placeholder zeros for request/bandwidth columns. The
// domain list itself is real and sorted by name.
func buildTopDomainsFromNodes(domains []*store.Domain, nodes []*store.Node) []overviewTopDomain {
	if len(domains) == 0 {
		return []overviewTopDomain{}
	}

	// Calculate total bandwidth and connections across active nodes for proportional distribution.
	now := time.Now()
	var totalBwBps float64
	var totalConns int64
	for _, n := range nodes {
		if n.LastMetricsAt.IsZero() || now.Sub(n.LastMetricsAt) > 5*time.Minute {
			continue
		}
		totalBwBps += n.BandwidthUpBps
		totalConns += int64(n.TCPEstablished)
	}

	// If we have node-level metrics, roughly distribute across domains.
	// This is an approximation; real per-domain data requires Elasticsearch.
	domainCount := float64(len(domains))
	perDomainBwMbps := 0.0
	perDomainReqs := int64(0)
	if domainCount > 0 {
		perDomainBwMbps = (totalBwBps * 8 / 1e6) / domainCount
		perDomainReqs = totalConns / int64(domainCount)
	}

	entries := make([]overviewTopDomain, 0, len(domains))
	for _, d := range domains {
		entries = append(entries, overviewTopDomain{
			Domain:        d.Name,
			URL:           "https://" + d.Name,
			Requests:      perDomainReqs,
			BandwidthMbps: math.Round(perDomainBwMbps*100) / 100,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Domain < entries[j].Domain
	})
	if len(entries) > 10 {
		entries = entries[:10]
	}
	return entries
}

// fetchESOverview pulls overview metrics from Elasticsearch. Best-effort:
// returns nil on missing config so handleOverview can fall back to the
// node-derived view.
func (s *Servers) fetchESOverview(ctx context.Context, window string) (*esOverview, error) {
	settings := s.resolveSettings(ctx)
	if strings.TrimSpace(settings.ElasticsearchURL) == "" {
		return nil, errors.New("es not configured")
	}

	rangeExpr, winDur := esWindowRange(window)
	interval := esHistogramInterval(window)
	indexPrefix := settings.ElasticsearchIndex
	if indexPrefix == "" {
		indexPrefix = "cdn-access"
	}
	tsField := strings.TrimSpace(settings.ElasticsearchTSField)
	if tsField == "" {
		tsField = "@timestamp"
	}
	domainField := strings.TrimSpace(settings.ElasticsearchDomainField)
	if domainField == "" {
		domainField = "domain.keyword"
	}
	bytesField := strings.TrimSpace(settings.ElasticsearchBytesField)
	if bytesField == "" {
		bytesField = "bytes"
	}
	esURL := fmt.Sprintf("%s/%s-*/_search", strings.TrimSuffix(settings.ElasticsearchURL, "/"), indexPrefix)

	reqBody := map[string]any{
		"size": 0,
		"query": map[string]any{
			"range": map[string]any{
				tsField: map[string]any{
					"gte": rangeExpr,
				},
			},
		},
		"aggs": map[string]any{
			"reqs":  map[string]any{"value_count": map[string]any{"field": tsField}},
			"bytes": map[string]any{"sum": map[string]any{"field": bytesField}},
			"by_domain": map[string]any{
				"terms": map[string]any{"field": domainField, "size": 10},
				"aggs": map[string]any{
					"bytes": map[string]any{"sum": map[string]any{"field": bytesField}},
				},
			},
			"ts": map[string]any{
				"date_histogram": map[string]any{
					"field":          tsField,
					"fixed_interval": interval,
					"min_doc_count":  0,
				},
				"aggs": map[string]any{
					"bytes": map[string]any{"sum": map[string]any{"field": bytesField}},
				},
			},
		},
	}

	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, esURL, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if settings.ElasticsearchUser != "" || settings.ElasticsearchPass != "" {
		cred := fmt.Sprintf("%s:%s", settings.ElasticsearchUser, settings.ElasticsearchPass)
		httpReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cred)))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("es status %d", resp.StatusCode)
	}

	var esResp struct {
		Aggregations struct {
			Reqs struct {
				Value float64 `json:"value"`
			} `json:"reqs"`
			Bytes struct {
				Value float64 `json:"value"`
			} `json:"bytes"`
			ByDomain struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
					Bytes    struct {
						Value float64 `json:"value"`
					} `json:"bytes"`
				} `json:"buckets"`
			} `json:"by_domain"`
			TS struct {
				Buckets []struct {
					Key      int64 `json:"key"`
					DocCount int64 `json:"doc_count"`
					Bytes    struct {
						Value float64 `json:"value"`
					} `json:"bytes"`
				} `json:"buckets"`
			} `json:"ts"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&esResp); err != nil {
		return nil, err
	}

	winEnd := time.Now()
	winStart := winEnd.Add(-winDur)
	out := &esOverview{
		Requests:    int64(esResp.Aggregations.Reqs.Value),
		Bytes:       esResp.Aggregations.Bytes.Value,
		WindowStart: winStart,
		WindowEnd:   winEnd,
	}

	if len(esResp.Aggregations.ByDomain.Buckets) > 0 {
		for _, b := range esResp.Aggregations.ByDomain.Buckets {
			bwMbps := 0.0
			if winDur > 0 {
				bwMbps = (b.Bytes.Value * 8) / winDur.Seconds() / 1e6
			}
			out.TopDomains = append(out.TopDomains, overviewTopDomain{
				Domain:        b.Key,
				URL:           fmt.Sprintf("https://%s", b.Key),
				Requests:      b.DocCount,
				BandwidthMbps: math.Round(bwMbps*100) / 100,
			})
		}
	}

	bucketSeconds := esHistogramSeconds(window)
	reqSeries := overviewTrendSeries{Key: "requests", Name: "Requests", Unit: "count", Points: []overviewTrendPoint{}}
	bwSeries := overviewTrendSeries{Key: "bandwidth", Name: "Bandwidth", Unit: "Mbps", Points: []overviewTrendPoint{}}
	for _, b := range esResp.Aggregations.TS.Buckets {
		ts := time.UnixMilli(b.Key)
		reqSeries.Points = append(reqSeries.Points, overviewTrendPoint{Timestamp: ts, Value: float64(b.DocCount)})
		bwVal := 0.0
		if bucketSeconds > 0 {
			bwVal = (b.Bytes.Value * 8) / float64(bucketSeconds) / 1e6
		}
		bwSeries.Points = append(bwSeries.Points, overviewTrendPoint{Timestamp: ts, Value: math.Round(bwVal*100) / 100})
	}
	if len(reqSeries.Points) > 0 {
		out.Trends = append(out.Trends, reqSeries)
	}
	if len(bwSeries.Points) > 0 {
		out.Trends = append(out.Trends, bwSeries)
	}

	return out, nil
}

func esWindowRange(window string) (string, time.Duration) {
	switch window {
	case "7d":
		return "now-7d", 7 * 24 * time.Hour
	case "30d":
		return "now-30d", 30 * 24 * time.Hour
	default:
		return "now-24h", 24 * time.Hour
	}
}

func esHistogramInterval(window string) string {
	switch window {
	case "7d":
		return "12h"
	case "30d":
		return "1d"
	default:
		return "1h"
	}
}

func esHistogramSeconds(window string) int {
	switch window {
	case "7d":
		return 12 * 3600
	case "30d":
		return 24 * 3600
	default:
		return 3600
	}
}

// normalizeWindow canonicalizes the ?window= query parameter into one of the
// three supported buckets: 24h (default), 7d, or 30d.
func normalizeWindow(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "7d", "7day", "week":
		return "7d"
	case "30d", "30day", "month":
		return "30d"
	default:
		return "24h"
	}
}
