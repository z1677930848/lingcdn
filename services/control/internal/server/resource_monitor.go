package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lingcdn/control/internal/store"
	"github.com/rs/zerolog/log"
)

// resourceAlertState tracks the last alert time for each node to avoid spamming.
type resourceAlertState struct {
	mu        sync.RWMutex
	lastAlert map[string]time.Time // nodeID -> last alert time
}

var globalResourceAlertState = &resourceAlertState{
	lastAlert: make(map[string]time.Time),
}

// checkResourceThresholds checks if node resource usage exceeds configured thresholds
// and sends notifications if enabled.
func (s *Servers) checkResourceThresholds(ctx context.Context, nodeID string, tele store.NodeTelemetry) {
	if s == nil || s.store == nil {
		return
	}

	settings, err := s.store.GetSettings(ctx)
	if err != nil || settings == nil || !settings.NotifyNodeResource {
		return
	}

	// Get node info for hostname
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil || node == nil {
		return
	}

	hostname := node.Hostname
	if hostname == "" {
		hostname = nodeID
	}

	// Check if we should throttle alerts (avoid spamming)
	globalResourceAlertState.mu.RLock()
	lastAlert, exists := globalResourceAlertState.lastAlert[nodeID]
	globalResourceAlertState.mu.RUnlock()

	// Only send alerts every 5 minutes per node
	if exists && time.Since(lastAlert) < 5*time.Minute {
		return
	}

	var alerts []string

	// Check CPU threshold
	if settings.ThresholdCPU > 0 && tele.CPUUsage > float64(settings.ThresholdCPU) {
		alerts = append(alerts, fmt.Sprintf("CPU 使用率: %.1f%% (阈值: %d%%)", tele.CPUUsage, settings.ThresholdCPU))
	}

	// Check Memory threshold
	if settings.ThresholdMemory > 0 && tele.MemUsage > float64(settings.ThresholdMemory) {
		alerts = append(alerts, fmt.Sprintf("内存使用率: %.1f%% (阈值: %d%%)", tele.MemUsage, settings.ThresholdMemory))
	}

	// Check Disk threshold
	if settings.ThresholdDisk > 0 && tele.DiskUsage > float64(settings.ThresholdDisk) {
		alerts = append(alerts, fmt.Sprintf("磁盘使用率: %.1f%% (阈值: %d%%)", tele.DiskUsage, settings.ThresholdDisk))
	}

	// If any threshold exceeded, send notification
	if len(alerts) > 0 {
		globalResourceAlertState.mu.Lock()
		globalResourceAlertState.lastAlert[nodeID] = time.Now()
		globalResourceAlertState.mu.Unlock()

		title := "节点资源告警"
		content := fmt.Sprintf("节点 %s 资源使用超过阈值：\n\n", hostname)
		for _, alert := range alerts {
			content += "- " + alert + "\n"
		}

		log.Info().Str("node", hostname).Strs("alerts", alerts).Msg("resource threshold exceeded")

		// Send webhook notifications
		s.sendWebhookNotification(ctx, title, content)
	}
}
