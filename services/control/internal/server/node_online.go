package server

import (
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// Node heartbeat cadence on the edge (see node-side config). All comm/metrics
// freshness checks derive from this so the UI, upgrade gate, and DNS sync
// agree on what "online" means.
const (
	NodeHeartbeatInterval = 30 * time.Second
	// NodeCommOKWindow: 4× heartbeat — tolerates one missed beat without
	// flipping to offline (matches isNodeOnlineForUpgrade).
	NodeCommOKWindow = 4 * NodeHeartbeatInterval
	// NodeMetricsOKWindow uses the same window as comm for consistency.
	NodeMetricsOKWindow = NodeCommOKWindow
	// NodeDNSStaleWindow is intentionally wider: DNS sync is not latency-sensitive.
	NodeDNSStaleWindow = 5 * time.Minute
)

func nodeStatusDisabled(n *store.Node) bool {
	return n != nil && strings.EqualFold(strings.TrimSpace(n.Status), "disabled")
}

func nodeCommOK(n *store.Node, now time.Time) bool {
	if n == nil || nodeStatusDisabled(n) {
		return false
	}
	if n.LastHeartbeat.IsZero() {
		return false
	}
	return now.Sub(n.LastHeartbeat) <= NodeCommOKWindow
}

func nodeMetricsOK(n *store.Node, now time.Time) bool {
	if n == nil || nodeStatusDisabled(n) {
		return false
	}
	if n.LastMetricsAt.IsZero() {
		return false
	}
	return now.Sub(n.LastMetricsAt) <= NodeMetricsOKWindow
}

func nodeHeartbeatFresh(n *store.Node, now time.Time) bool {
	if n == nil || n.LastHeartbeat.IsZero() {
		return false
	}
	return now.Sub(n.LastHeartbeat) <= NodeCommOKWindow
}
