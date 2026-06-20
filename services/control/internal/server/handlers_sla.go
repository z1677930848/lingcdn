package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// GET /api/domains/{id}/sla?days=30
func (s *Servers) handleDomainSLA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx := r.Context()
	role := getUserRole(ctx)
	userID, _ := ctx.Value(ctxKeyUserID).(string)

	path := strings.TrimPrefix(r.URL.Path, "/api/domains/")
	path = strings.TrimSuffix(path, "/sla")
	domainID := strings.TrimSpace(path)
	if domainID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "域名ID不能为空"})
		return
	}

	domain, err := s.store.GetDomain(ctx, domainID)
	if err != nil {
		writeInternalError(w, "get domain", err)
		return
	}
	if domain == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "域名不存在"})
		return
	}
	if role != "admin" && domain.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权查看此域名"})
		return
	}

	days := 30
	if v := strings.TrimSpace(r.URL.Query().Get("days")); v != "" {
		if n, err := parseIntDefault(v, 30); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}

	report := s.buildDomainSLAReport(ctx, domain, time.Duration(days)*24*time.Hour)
	writeJSON(w, http.StatusOK, report)
}

func (s *Servers) buildDomainSLAReport(ctx context.Context, domain *store.Domain, window time.Duration) map[string]any {
	if domain == nil {
		return map[string]any{}
	}
	if window <= 0 {
		window = 30 * 24 * time.Hour
	}

	clusterNodes := 0
	healthySamples := 0
	totalSamples := 0
	avgLatencyMs := 0.0

	if domain.LineGroupID != "" {
		if list, err := s.store.ListClusterNodes(ctx, domain.LineGroupID, ""); err == nil {
			clusterNodes = len(list)
		}
	}

	if s.nodeMonitor != nil {
		allNodes, _ := s.store.ListNodes(ctx)
		for _, n := range allNodes {
			if n == nil {
				continue
			}
			pts := s.nodeMonitor.snapshot(n.ID)
			cutoff := time.Now().Add(-window)
			for _, p := range pts {
				if p.At.Before(cutoff) {
					continue
				}
				totalSamples++
				if p.CPUUsage < 95 && p.MemUsage < 95 {
					healthySamples++
				}
				avgLatencyMs += float64(p.TCPEstablished)
			}
		}
	}

	availability := 99.9
	if totalSamples > 0 {
		availability = math.Round((float64(healthySamples)/float64(totalSamples))*10000) / 100
	}
	if avgLatencyMs > 0 && totalSamples > 0 {
		avgLatencyMs = avgLatencyMs / float64(totalSamples)
	}

	return map[string]any{
		"domain_id":       domain.ID,
		"domain_name":     domain.Name,
		"window_days":     int(window.Hours() / 24),
		"availability_pct": availability,
		"cluster_nodes":   clusterNodes,
		"samples":         totalSamples,
		"healthy_samples": healthySamples,
		"avg_tcp_established": avgLatencyMs,
		"generated_at":    time.Now().UTC().Format(time.RFC3339),
	}
}

func parseIntDefault(s string, def int) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return def, err
	}
	return n, nil
}
