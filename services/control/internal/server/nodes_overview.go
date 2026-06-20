package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Servers) handleNodesOverview(w http.ResponseWriter, r *http.Request) {
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

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	region := strings.TrimSpace(r.URL.Query().Get("region"))

	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := parsePositiveInt(r.URL.Query().Get("page_size"), 10)
	if pageSize > 100 {
		pageSize = 100
	}

	filtered := make([]*map[string]any, 0, len(nodes))
	now := time.Now()
	for _, n := range nodes {
		if n == nil {
			continue
		}
		sn := strings.ToLower(strings.TrimSpace(n.Status))
		if status != "" && sn != status {
			continue
		}
		if region != "" && strings.TrimSpace(n.Region) != region {
			continue
		}
		if q != "" {
			hay := strings.ToLower(strings.Join([]string{
				n.ID,
				n.Hostname,
				n.PublicIP,
				n.Region,
				n.Cluster,
				n.Version,
				n.Status,
			}, " "))
			if !strings.Contains(hay, q) {
				continue
			}
		}

		commOK := nodeCommOK(n, now)
		reportOK := nodeMetricsOK(n, now)

		item := map[string]any{
			"id":              n.ID,
			"hostname":         n.Hostname,
			"public_ip":        n.PublicIP,
			"version":          n.Version,
			"status":           n.Status,
			"region":           n.Region,
			"cluster":          n.Cluster,
			"capabilities":     n.Capabilities,
			"config_version":   n.ConfigVersion,
			"last_heartbeat":   n.LastHeartbeat,
			"monitor_enabled":  n.MonitorEnabled,
			"monitor_protocol": n.MonitorProtocol,
			"monitor_timeout_seconds": n.MonitorTimeout,
			"monitor_port":     n.MonitorPort,
			"monitor_fail_threshold": n.MonitorFailThreshold,
			"monitor_fail_count": n.MonitorFailCount,
			"monitor_last_ok":  n.MonitorLastOK,
			"monitor_last_error": n.MonitorLastError,
			"monitor_last_at":  n.MonitorLastAt,
			"monitor_last_latency_ms": n.MonitorLastLatencyMs,
			"cpu_usage":        n.CPUUsage,
			"mem_usage":        n.MemUsage,
			"disk_usage":       n.DiskUsage,
			"cpu_count":        n.CPUCount,
			"mem_total":        n.MemTotal,
			"disk_total":       n.DiskTotal,
			"last_metrics_at":  n.LastMetricsAt,
			"bytes_sent":       n.BytesSent,
			"bytes_received":   n.BytesReceived,
			"bandwidth_up_bps": n.BandwidthUpBps,
			"bandwidth_down_bps": n.BandwidthDownBps,
			"tcp_established":    n.TCPEstablished,
			"tcp_syn_recv":       n.TCPSynRecv,
			"tcp_time_wait":      n.TCPTimeWait,
			"nginx_running":      n.NginxRunning,
			"month_bytes_sent":   n.MonthBytesSent,
			"comm_ok":            commOK,
			"report_ok":          reportOK,
			"created_at":         n.CreatedAt,
			"updated_at":         n.UpdatedAt,
		}
		filtered = append(filtered, &item)
	}

	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	out := make([]map[string]any, 0, end-start)
	for _, p := range filtered[start:end] {
		out = append(out, *p)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes":     out,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func parsePositiveInt(v string, def int) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
