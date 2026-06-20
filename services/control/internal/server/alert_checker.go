package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

// alertCheckerLoop evaluates enabled alert rules against node telemetry.
func (s *Servers) alertCheckerLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	var lastFired sync.Map // ruleID+nodeID -> time.Time

	run := func() {
		runBGTask("alerts.check", func() (string, error) {
			return s.runAlertChecks(&lastFired)
		})
	}
	run()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (s *Servers) runAlertChecks(lastFired *sync.Map) (string, error) {
	if s == nil || s.store == nil {
		return "skip: store missing", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rules, err := s.store.ListAlertRules(ctx)
	if err != nil {
		return "", err
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return "", err
	}

	var fired int
	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
			continue
		}
		window := time.Duration(rule.WindowSeconds) * time.Second
		if window <= 0 {
			window = 60 * time.Second
		}
		for _, node := range nodes {
			if node == nil || strings.EqualFold(node.Status, "disabled") {
				continue
			}
			val, ok := s.metricValueForAlert(node, rule.Metric, window)
			if !ok {
				continue
			}
			if val < rule.Threshold {
				continue
			}
			key := rule.ID + ":" + node.ID
			if t, loaded := lastFired.Load(key); loaded {
				if time.Since(t.(time.Time)) < 5*time.Minute {
					continue
				}
			}
			lastFired.Store(key, time.Now())
			fired++
			title := fmt.Sprintf("[LingCDN %s] %s", strings.ToUpper(rule.Severity), rule.Name)
			body := fmt.Sprintf("节点 %s 指标 %s=%.2f 超过阈值 %.2f (窗口 %ds)",
				node.Hostname, rule.Metric, val, rule.Threshold, rule.WindowSeconds)
			s.notifyAdmin(title, body)
			s.sendWebhookNotification(ctx, title, body)
			_ = s.store.CreateSystemLog(ctx, &store.SystemLog{
				Type:    "alert",
				Status:  rule.Severity,
				Message: body,
			})
		}
	}
	if fired > 0 {
		log.Info().Int("fired", fired).Msg("alert rules triggered")
	}
	return fmt.Sprintf("checked=%d fired=%d", len(rules), fired), nil
}

func (s *Servers) metricValueForAlert(node *store.Node, metric string, window time.Duration) (float64, bool) {
	if node == nil {
		return 0, false
	}
	metric = strings.ToLower(strings.TrimSpace(metric))
	if s.nodeMonitor != nil {
		if agg, ok := s.nodeMonitor.aggregate(node.ID, window); ok {
			switch metric {
			case "cpu_usage", "cpu":
				return agg.AvgCPUUsage, true
			case "mem_usage", "memory", "mem":
				return agg.AvgMemUsage, true
			case "disk_usage", "disk":
				return agg.AvgDiskUsage, true
			case "bandwidth_up", "bandwidth":
				return agg.AvgUpBps, true
			case "tcp_established", "tcp":
				return agg.AvgTCPEstablished, true
			}
		}
	}
	switch metric {
	case "cpu_usage", "cpu":
		return node.CPUUsage, true
	case "mem_usage", "memory", "mem":
		return node.MemUsage, true
	case "disk_usage", "disk":
		return node.DiskUsage, true
	case "bandwidth_up", "bandwidth":
		return float64(node.BandwidthUpBps), true
	case "tcp_established", "tcp":
		return float64(node.TCPEstablished), true
	default:
		return 0, false
	}
}
