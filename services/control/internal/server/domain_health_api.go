package server

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
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

type domainHealthPoint struct {
	TS    int64   `json:"ts"`
	Value float64 `json:"value"`
}

type domainHealthSeries struct {
	Key    string             `json:"key"`
	Name   string             `json:"name"`
	Unit   string             `json:"unit"`
	Points []domainHealthPoint `json:"points"`
}

type domainHealthMetricsResponse struct {
	Group        string             `json:"group"`
	WindowSeconds int               `json:"window_seconds"`
	StepSeconds  int               `json:"step_seconds"`
	FromUnix     int64             `json:"from_unix"`
	ToUnix       int64             `json:"to_unix"`
	Domains      []string          `json:"domains"`
	Port         int              `json:"port,omitempty"`
	Series       []domainHealthSeries `json:"series"`
}

type domainHealthRankEntry struct {
	Rank         int     `json:"rank"`
	Domain       string  `json:"domain"`
	BandwidthBps float64 `json:"bandwidth_bps"`
	Bytes        float64 `json:"bytes"`
	Requests     int64   `json:"requests"`
	QPS          float64 `json:"qps"`
	HTTP4xx      int64   `json:"http_4xx"`
	HTTP5xx      int64   `json:"http_5xx"`
	ErrorRate    float64 `json:"error_rate"`
}

type domainHealthRankResponse struct {
	Metric        string               `json:"metric"`
	WindowSeconds int                  `json:"window_seconds"`
	FromUnix      int64                `json:"from_unix"`
	ToUnix        int64                `json:"to_unix"`
	Domains       []string             `json:"domains"`
	Port          int                  `json:"port,omitempty"`
	Items         []domainHealthRankEntry `json:"items"`
}

func (s *Servers) handleDomainHealthMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	group := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("group")))
	if group == "" {
		group = "basic"
	}
	if group != "basic" && group != "quality" && group != "origin" {
		group = "basic"
	}

	domains := parseDomainList(r.URL.Query().Get("domains"))
	port := parsePort(r.URL.Query().Get("port"))

	points := 60
	if v := strings.TrimSpace(r.URL.Query().Get("points")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10 && n <= 240 {
			points = n
		}
	}

	fromUnix := parseInt64(r.URL.Query().Get("from_unix"))
	toUnix := parseInt64(r.URL.Query().Get("to_unix"))
	windowSeconds := 3600
	if v := strings.TrimSpace(r.URL.Query().Get("window_seconds")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windowSeconds = n
		}
	}
	now := time.Now()
	var from time.Time
	var to time.Time
	if fromUnix > 0 && toUnix > 0 && toUnix > fromUnix {
		from = time.Unix(fromUnix, 0)
		to = time.Unix(toUnix, 0)
		windowSeconds = int(to.Sub(from).Seconds())
	} else {
		to = now
		from = to.Add(-time.Duration(windowSeconds) * time.Second)
		fromUnix = from.Unix()
		toUnix = to.Unix()
	}

	stepSeconds := maxInt(60, windowSeconds/maxInt(10, points-1))
	if stepSeconds > 12*3600 {
		stepSeconds = 12 * 3600
	}

	settings := s.resolveSettings(ctx)
	if strings.TrimSpace(settings.ElasticsearchURL) == "" {
		writeJSON(w, http.StatusOK, domainHealthMetricsResponse{
			Group:         group,
			WindowSeconds: windowSeconds,
			StepSeconds:   stepSeconds,
			FromUnix:      fromUnix,
			ToUnix:        toUnix,
			Domains:       domains,
			Port:          port,
			Series:        buildDomainHealthZeroSeries(group, fromUnix, toUnix, stepSeconds),
		})
		return
	}

	series, err := s.fetchESDomainHealthMetrics(ctx, settings, group, domains, port, from, to, stepSeconds)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch domain health metrics from ES")
		writeJSON(w, http.StatusOK, domainHealthMetricsResponse{
			Group:         group,
			WindowSeconds: windowSeconds,
			StepSeconds:   stepSeconds,
			FromUnix:      fromUnix,
			ToUnix:        toUnix,
			Domains:       domains,
			Port:          port,
			Series:        buildDomainHealthZeroSeries(group, fromUnix, toUnix, stepSeconds),
		})
		return
	}

	writeJSON(w, http.StatusOK, domainHealthMetricsResponse{
		Group:         group,
		WindowSeconds: windowSeconds,
		StepSeconds:   stepSeconds,
		FromUnix:      fromUnix,
		ToUnix:        toUnix,
		Domains:       domains,
		Port:          port,
		Series:        series,
	})
}

func (s *Servers) handleDomainHealthRank(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	metric := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("metric")))
	if metric == "" {
		metric = "bandwidth"
	}
	allowed := map[string]bool{"bandwidth": true, "traffic": true, "requests": true, "qps": true, "4xx": true, "5xx": true, "error_rate": true}
	if !allowed[metric] {
		metric = "bandwidth"
	}

	domains := parseDomainList(r.URL.Query().Get("domains"))
	port := parsePort(r.URL.Query().Get("port"))
	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	fromUnix := parseInt64(r.URL.Query().Get("from_unix"))
	toUnix := parseInt64(r.URL.Query().Get("to_unix"))
	windowSeconds := 3600
	if v := strings.TrimSpace(r.URL.Query().Get("window_seconds")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windowSeconds = n
		}
	}
	now := time.Now()
	var from time.Time
	var to time.Time
	if fromUnix > 0 && toUnix > 0 && toUnix > fromUnix {
		from = time.Unix(fromUnix, 0)
		to = time.Unix(toUnix, 0)
		windowSeconds = int(to.Sub(from).Seconds())
	} else {
		to = now
		from = to.Add(-time.Duration(windowSeconds) * time.Second)
		fromUnix = from.Unix()
		toUnix = to.Unix()
	}

	settings := s.resolveSettings(ctx)
	if strings.TrimSpace(settings.ElasticsearchURL) == "" {
		writeJSON(w, http.StatusOK, domainHealthRankResponse{
			Metric:        metric,
			WindowSeconds: windowSeconds,
			FromUnix:      fromUnix,
			ToUnix:        toUnix,
			Domains:       domains,
			Port:          port,
			Items:         []domainHealthRankEntry{},
		})
		return
	}

	items, err := s.fetchESDomainHealthRank(ctx, settings, metric, domains, port, from, to, windowSeconds, limit)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch domain health rank from ES")
		items = []domainHealthRankEntry{}
	}
	writeJSON(w, http.StatusOK, domainHealthRankResponse{
		Metric:        metric,
		WindowSeconds: windowSeconds,
		FromUnix:      fromUnix,
		ToUnix:        toUnix,
		Domains:       domains,
		Port:          port,
		Items:         items,
	})
}

func parseDomainList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == ',' || r == '，' || r == '\n' || r == '\t' || r == ';' || r == '；'
	})
	uniq := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		d := strings.ToLower(strings.TrimSpace(p))
		if d == "" {
			continue
		}
		if strings.HasPrefix(d, "http://") || strings.HasPrefix(d, "https://") {
			d = strings.TrimPrefix(strings.TrimPrefix(d, "http://"), "https://")
			d = strings.Trim(d, "/")
		}
		if d == "" {
			continue
		}
		if _, ok := uniq[d]; ok {
			continue
		}
		uniq[d] = struct{}{}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func parsePort(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 65535 {
		return 0
	}
	return n
}

func parseInt64(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func buildDomainHealthZeroSeries(group string, fromUnix, toUnix int64, stepSeconds int) []domainHealthSeries {
	if stepSeconds <= 0 {
		stepSeconds = 60
	}
	if toUnix <= 0 {
		toUnix = time.Now().Unix()
	}
	if fromUnix <= 0 || fromUnix >= toUnix {
		fromUnix = toUnix - 3600
	}
	var points []domainHealthPoint
	for ts := fromUnix; ts <= toUnix; ts += int64(stepSeconds) {
		points = append(points, domainHealthPoint{TS: ts, Value: 0})
	}
	if len(points) < 2 {
		points = append(points, domainHealthPoint{TS: toUnix, Value: 0})
	}

	switch group {
	case "quality":
		return []domainHealthSeries{
			{Key: "http_5xx_qps", Name: "5xx QPS", Unit: "qps", Points: points},
			{Key: "http_4xx_qps", Name: "4xx QPS", Unit: "qps", Points: points},
			{Key: "error_rate", Name: "错误率", Unit: "ratio", Points: points},
		}
	case "origin":
		return []domainHealthSeries{
			{Key: "origin_qps", Name: "回源 QPS", Unit: "qps", Points: points},
			{Key: "origin_5xx_qps", Name: "回源 5xx QPS", Unit: "qps", Points: points},
			{Key: "origin_error_rate", Name: "回源错误率", Unit: "ratio", Points: points},
		}
	default:
		return []domainHealthSeries{
			{Key: "bandwidth_bps", Name: "带宽", Unit: "bps", Points: points},
			{Key: "bytes", Name: "流量", Unit: "bytes", Points: points},
			{Key: "requests", Name: "访问次数", Unit: "count", Points: points},
			{Key: "qps", Name: "QPS", Unit: "qps", Points: points},
		}
	}
}

func esFixedInterval(stepSeconds int) string {
	if stepSeconds <= 0 {
		return "60s"
	}
	if stepSeconds%3600 == 0 {
		return fmt.Sprintf("%dh", stepSeconds/3600)
	}
	if stepSeconds%60 == 0 {
		return fmt.Sprintf("%dm", stepSeconds/60)
	}
	return fmt.Sprintf("%ds", stepSeconds)
}

func (s *Servers) fetchESDomainHealthMetrics(ctx context.Context, settings *store.Settings, group string, domains []string, port int, from, to time.Time, stepSeconds int) ([]domainHealthSeries, error) {
	if strings.TrimSpace(settings.ElasticsearchURL) == "" {
		return nil, errors.New("es not configured")
	}
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
	interval := esFixedInterval(stepSeconds)

	filters := []any{
		map[string]any{
			"range": map[string]any{
				tsField: map[string]any{
					"gte": from.Format(time.RFC3339),
					"lte": to.Format(time.RFC3339),
				},
			},
		},
	}
	if len(domains) > 0 {
		filters = append(filters, map[string]any{
			"terms": map[string]any{
				domainField: domains,
			},
		})
	}
	if port > 0 {
		filters = append(filters, map[string]any{
			"bool": map[string]any{
				"should": []any{
					map[string]any{"term": map[string]any{"server_port": port}},
					map[string]any{"term": map[string]any{"port": port}},
					map[string]any{"term": map[string]any{"listen_port": port}},
				},
				"minimum_should_match": 1,
			},
		})
	}

	reqBody := map[string]any{
		"size": 0,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": filters,
			},
		},
		"aggs": map[string]any{
			"ts": map[string]any{
				"date_histogram": map[string]any{
					"field":          tsField,
					"fixed_interval": interval,
					"min_doc_count":  0,
				},
				"aggs": map[string]any{
					"bytes": map[string]any{"sum": map[string]any{"field": bytesField}},
					"http_4xx": map[string]any{
						"filter": map[string]any{
							"range": map[string]any{
								"status": map[string]any{"gte": 400, "lt": 500},
							},
						},
					},
					"http_5xx": map[string]any{
						"filter": map[string]any{
							"range": map[string]any{
								"status": map[string]any{"gte": 500, "lt": 600},
							},
						},
					},
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
			TS struct {
				Buckets []struct {
					Key      int64 `json:"key"`
					DocCount int64 `json:"doc_count"`
					Bytes    struct {
						Value float64 `json:"value"`
					} `json:"bytes"`
					HTTP4xx struct {
						DocCount int64 `json:"doc_count"`
					} `json:"http_4xx"`
					HTTP5xx struct {
						DocCount int64 `json:"doc_count"`
					} `json:"http_5xx"`
				} `json:"buckets"`
			} `json:"ts"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&esResp); err != nil {
		return nil, err
	}

	bw := domainHealthSeries{Key: "bandwidth_bps", Name: "带宽", Unit: "bps"}
	tr := domainHealthSeries{Key: "bytes", Name: "流量", Unit: "bytes"}
	reqs := domainHealthSeries{Key: "requests", Name: "访问次数", Unit: "count"}
	qps := domainHealthSeries{Key: "qps", Name: "QPS", Unit: "qps"}
	q4 := domainHealthSeries{Key: "http_4xx_qps", Name: "4xx QPS", Unit: "qps"}
	q5 := domainHealthSeries{Key: "http_5xx_qps", Name: "5xx QPS", Unit: "qps"}
	errRate := domainHealthSeries{Key: "error_rate", Name: "错误率", Unit: "ratio"}

	for _, b := range esResp.Aggregations.TS.Buckets {
		ts := time.UnixMilli(b.Key).Unix()
		bwVal := 0.0
		if stepSeconds > 0 {
			bwVal = (b.Bytes.Value * 8) / float64(stepSeconds)
		}
		bw.Points = append(bw.Points, domainHealthPoint{TS: ts, Value: bwVal})
		tr.Points = append(tr.Points, domainHealthPoint{TS: ts, Value: b.Bytes.Value})
		reqs.Points = append(reqs.Points, domainHealthPoint{TS: ts, Value: float64(b.DocCount)})
		qps.Points = append(qps.Points, domainHealthPoint{TS: ts, Value: float64(b.DocCount) / float64(stepSeconds)})
		q4.Points = append(q4.Points, domainHealthPoint{TS: ts, Value: float64(b.HTTP4xx.DocCount) / float64(stepSeconds)})
		q5.Points = append(q5.Points, domainHealthPoint{TS: ts, Value: float64(b.HTTP5xx.DocCount) / float64(stepSeconds)})
		errCount := float64(b.HTTP4xx.DocCount + b.HTTP5xx.DocCount)
		den := math.Max(1, float64(b.DocCount))
		errRate.Points = append(errRate.Points, domainHealthPoint{TS: ts, Value: errCount / den})
	}

	switch group {
	case "quality":
		return []domainHealthSeries{q5, q4, errRate}, nil
	case "origin":
		return []domainHealthSeries{}, nil
	default:
		return []domainHealthSeries{bw, tr, reqs, qps}, nil
	}
}

func (s *Servers) fetchESDomainHealthRank(ctx context.Context, settings *store.Settings, metric string, domains []string, port int, from, to time.Time, windowSeconds int, limit int) ([]domainHealthRankEntry, error) {
	if strings.TrimSpace(settings.ElasticsearchURL) == "" {
		return nil, errors.New("es not configured")
	}
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

	filters := []any{
		map[string]any{
			"range": map[string]any{
				tsField: map[string]any{
					"gte": from.Format(time.RFC3339),
					"lte": to.Format(time.RFC3339),
				},
			},
		},
	}
	if len(domains) > 0 {
		filters = append(filters, map[string]any{
			"terms": map[string]any{
				domainField: domains,
			},
		})
	}
	if port > 0 {
		filters = append(filters, map[string]any{
			"bool": map[string]any{
				"should": []any{
					map[string]any{"term": map[string]any{"server_port": port}},
					map[string]any{"term": map[string]any{"port": port}},
					map[string]any{"term": map[string]any{"listen_port": port}},
				},
				"minimum_should_match": 1,
			},
		})
	}

	reqBody := map[string]any{
		"size": 0,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": filters,
			},
		},
		"aggs": map[string]any{
			"by_domain": map[string]any{
				"terms": map[string]any{"field": domainField, "size": limit},
				"aggs": map[string]any{
					"bytes": map[string]any{"sum": map[string]any{"field": bytesField}},
					"http_4xx": map[string]any{
						"filter": map[string]any{
							"range": map[string]any{
								"status": map[string]any{"gte": 400, "lt": 500},
							},
						},
					},
					"http_5xx": map[string]any{
						"filter": map[string]any{
							"range": map[string]any{
								"status": map[string]any{"gte": 500, "lt": 600},
							},
						},
					},
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
			ByDomain struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
					Bytes    struct {
						Value float64 `json:"value"`
					} `json:"bytes"`
					HTTP4xx struct {
						DocCount int64 `json:"doc_count"`
					} `json:"http_4xx"`
					HTTP5xx struct {
						DocCount int64 `json:"doc_count"`
					} `json:"http_5xx"`
				} `json:"buckets"`
			} `json:"by_domain"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&esResp); err != nil {
		return nil, err
	}

	win := float64(maxInt(1, windowSeconds))
	var items []domainHealthRankEntry
	for _, b := range esResp.Aggregations.ByDomain.Buckets {
		bw := (b.Bytes.Value * 8) / win
		qps := float64(b.DocCount) / win
		errCount := b.HTTP4xx.DocCount + b.HTTP5xx.DocCount
		errRate := float64(errCount) / math.Max(1, float64(b.DocCount))
		items = append(items, domainHealthRankEntry{
			Domain:       b.Key,
			BandwidthBps: bw,
			Bytes:        b.Bytes.Value,
			Requests:     b.DocCount,
			QPS:          qps,
			HTTP4xx:      b.HTTP4xx.DocCount,
			HTTP5xx:      b.HTTP5xx.DocCount,
			ErrorRate:    errRate,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		switch metric {
		case "traffic":
			return items[i].Bytes > items[j].Bytes
		case "requests":
			return items[i].Requests > items[j].Requests
		case "qps":
			return items[i].QPS > items[j].QPS
		case "4xx":
			return items[i].HTTP4xx > items[j].HTTP4xx
		case "5xx":
			return items[i].HTTP5xx > items[j].HTTP5xx
		case "error_rate":
			return items[i].ErrorRate > items[j].ErrorRate
		default:
			return items[i].BandwidthBps > items[j].BandwidthBps
		}
	})
	for i := range items {
		items[i].Rank = i + 1
	}
	return items, nil
}

