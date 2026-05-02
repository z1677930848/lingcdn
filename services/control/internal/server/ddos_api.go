package server

import (
	"net/http"
	"time"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleDdosXdpStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	type ddosNodeView struct {
		*store.Node
		Geo *GeoLocation `json:"geo,omitempty"`
	}
	type item struct {
		Node any `json:"node"`
		XDP  any `json:"xdp,omitempty"`
	}
	items := make([]item, 0, len(nodes))

	nodesWithXDP := 0
	nodesXDPEnabled := 0
	latestXDPAt := time.Time{}
	totals := map[string]uint64{}

	for _, n := range nodes {
		if n == nil {
			continue
		}
		nodeView := ddosNodeView{Node: n}
		if s.geoip != nil {
			if geo := s.geoip.Lookup(n.PublicIP); geo != nil {
				nodeView.Geo = geo
			}
		}
		it := item{Node: nodeView}
		if snap, ok := s.xdpStore.Get(n.ID); ok {
			it.XDP = snap
			nodesWithXDP++
			if snap.Enabled {
				nodesXDPEnabled++
			}
			if snap.UpdatedAt.After(latestXDPAt) {
				latestXDPAt = snap.UpdatedAt
			}
			for k, v := range snap.Stats {
				totals[k] += v
			}
		}
		items = append(items, it)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": time.Now().UTC(),
		"summary": map[string]any{
			"nodes_total":       len(nodes),
			"nodes_with_xdp":    nodesWithXDP,
			"nodes_xdp_enabled": nodesXDPEnabled,
			"latest_xdp_at":     latestXDPAt,
			"totals":            totals,
		},
		"items": items,
	})
}
