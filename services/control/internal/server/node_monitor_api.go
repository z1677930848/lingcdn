package server

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type monitorRankEntry struct {
	Rank           int     `json:"rank"`
	NodeID         string  `json:"node_id"`
	Hostname       string  `json:"hostname"`
	Iface          string  `json:"iface"`
	OutBps         float64 `json:"out_bps"`
	InBps          float64 `json:"in_bps"`
	Connections    float64 `json:"connections"`
	CPUUsage       float64 `json:"cpu_usage"`
	MemUsage       float64 `json:"mem_usage"`
	DiskUsage      float64 `json:"disk_usage"`
	DeltaBytesSent int64   `json:"delta_bytes_sent"`
	DeltaBytesRecv int64   `json:"delta_bytes_recv"`
}

type monitorRankResponse struct {
	Group         string             `json:"group"`
	WindowSeconds int                `json:"window_seconds"`
	Nodes         []monitorRankEntry `json:"nodes"`
}

func (s *Servers) handleNodeMonitorRank(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx := r.Context()
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	group := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("group")))
	if group == "" {
		group = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("metric")))
	}
	if group == "" {
		group = "bandwidth"
	}
	if group == "conn" || group == "connections" {
		group = "connect"
	}

	windowSeconds := 60
	if v := strings.TrimSpace(r.URL.Query().Get("window_seconds")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windowSeconds = n
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("window")); v != "" && strings.HasSuffix(v, "s") == false {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windowSeconds = n
		}
	}
	window := time.Duration(windowSeconds) * time.Second

	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	list := make([]monitorRankEntry, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		entry := monitorRankEntry{
			NodeID:    n.ID,
			Hostname:  n.Hostname,
			Iface:     "all",
			OutBps:    n.BandwidthUpBps,
			InBps:     n.BandwidthDownBps,
			CPUUsage:  n.CPUUsage,
			MemUsage:  n.MemUsage,
			DiskUsage: n.DiskUsage,
			Connections: float64(n.TCPEstablished),
		}

		if s.nodeMonitor != nil {
			if agg, ok := s.nodeMonitor.aggregate(n.ID, window); ok {
				entry.OutBps = agg.AvgUpBps
				entry.InBps = agg.AvgDownBps
				entry.CPUUsage = agg.AvgCPUUsage
				entry.MemUsage = agg.AvgMemUsage
				entry.DiskUsage = agg.AvgDiskUsage
				entry.Connections = agg.AvgTCPEstablished
				entry.DeltaBytesSent = agg.DeltaBytesSent
				entry.DeltaBytesRecv = agg.DeltaBytesRecv
			}
		}
		list = append(list, entry)
	}

	sort.Slice(list, func(i, j int) bool {
		switch group {
		case "connect":
			return list[i].Connections > list[j].Connections
		case "load":
			return list[i].CPUUsage > list[j].CPUUsage
		case "disk":
			return list[i].DiskUsage > list[j].DiskUsage
		default:
			return list[i].OutBps > list[j].OutBps
		}
	})

	if len(list) > limit {
		list = list[:limit]
	}
	for i := range list {
		list[i].Rank = i + 1
	}

	writeJSON(w, http.StatusOK, monitorRankResponse{
		Group:         group,
		WindowSeconds: windowSeconds,
		Nodes:         list,
	})
}

type monitorSeriesPoint struct {
	Timestamp int64   `json:"ts"`
	Value     float64 `json:"value"`
}

type monitorSeriesResponse struct {
	Metric        string               `json:"metric"`
	WindowSeconds int                  `json:"window_seconds"`
	StepSeconds   int                  `json:"step_seconds"`
	Points        []monitorSeriesPoint `json:"points"`
}

func (s *Servers) handleNodeMonitorSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	nodes, err := s.store.ListNodes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	metric := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("metric")))
	if metric == "" {
		metric = "bandwidth_out"
	}
	windowSeconds := 60
	if v := strings.TrimSpace(r.URL.Query().Get("window_seconds")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windowSeconds = n
		}
	}
	points := 30
	if v := strings.TrimSpace(r.URL.Query().Get("points")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 5 && n <= 120 {
			points = n
		}
	}
	window := time.Duration(windowSeconds) * time.Second
	step := time.Duration(int(math.Max(1, float64(windowSeconds/(points-1))))) * time.Second

	end := time.Now()
	start := end.Add(-window)

	nodePoints := map[string][]nodeMetricPoint{}
	if s.nodeMonitor != nil {
		for _, n := range nodes {
			if n == nil {
				continue
			}
			nodePoints[n.ID] = s.nodeMonitor.snapshot(n.ID)
		}
	}

	valueAt := func(pts []nodeMetricPoint, at time.Time, metric string) float64 {
		if len(pts) == 0 {
			return 0
		}
		i := sort.Search(len(pts), func(i int) bool { return !pts[i].At.Before(at) })
		idx := i
		if idx >= len(pts) {
			idx = len(pts) - 1
		} else if idx > 0 && pts[idx].At.After(at) {
			idx = idx - 1
		}
		if idx < 0 {
			return 0
		}
		p := pts[idx]
		switch metric {
		case "cpu":
			return p.CPUUsage
		case "mem":
			return p.MemUsage
		case "disk":
			return p.DiskUsage
		case "connections", "conn":
			return float64(p.TCPEstablished)
		case "bandwidth_in":
			if idx == 0 {
				return 0
			}
			prev := pts[idx-1]
			secs := p.At.Sub(prev.At).Seconds()
			if secs <= 0 {
				return 0
			}
			return float64(p.BytesReceived-prev.BytesReceived) / secs
		default:
			if idx == 0 {
				return 0
			}
			prev := pts[idx-1]
			secs := p.At.Sub(prev.At).Seconds()
			if secs <= 0 {
				return 0
			}
			return float64(p.BytesSent-prev.BytesSent) / secs
		}
	}

	var out []monitorSeriesPoint
	for t := start; !t.After(end); t = t.Add(step) {
		total := 0.0
		if s.nodeMonitor != nil {
			for _, n := range nodes {
				if n == nil {
					continue
				}
				total += valueAt(nodePoints[n.ID], t, metric)
			}
		} else {
			for _, n := range nodes {
				if n == nil {
					continue
				}
				switch metric {
				case "cpu":
					total += n.CPUUsage
				case "mem":
					total += n.MemUsage
				case "disk":
					total += n.DiskUsage
				case "connections", "conn":
					total += float64(n.TCPEstablished)
				case "bandwidth_in":
					total += n.BandwidthDownBps
				default:
					total += n.BandwidthUpBps
				}
			}
		}
		out = append(out, monitorSeriesPoint{Timestamp: t.Unix(), Value: total})
	}

	writeJSON(w, http.StatusOK, monitorSeriesResponse{
		Metric:        metric,
		WindowSeconds: windowSeconds,
		StepSeconds:   int(step.Seconds()),
		Points:        out,
	})
}
