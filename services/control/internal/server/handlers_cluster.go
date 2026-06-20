package server

// Cluster admin handlers. Clusters group nodes by DNS zone so the control
// plane can emit per-cluster DNS records; "lines" within a cluster subdivide
// nodes into weighted sub-pools (e.g. telecom vs. unicom). All handlers are
// admin-only.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleClusters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		clusters, err := s.store.ListClusters(ctx)
		if err != nil {
			writeInternalError(w, "list clusters", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"clusters": clusters})
	case http.MethodPost:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		var req store.Cluster
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		if strings.TrimSpace(req.DNSZone) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "DNS区域不能为空"})
			return
		}
		// Explicit pre-flight dup check gives a cleaner 409 than relying on the
		// database unique constraint. The DB check is still kept below as a
		// race-safety net.
		existingClusters, err := s.store.ListClusters(ctx)
		if err != nil {
			writeInternalError(w, "create cluster", err)
			return
		}
		for _, ec := range existingClusters {
			if strings.EqualFold(ec.Name, strings.TrimSpace(req.Name)) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "集群名称已存在"})
				return
			}
		}
		req.ID = uuid.NewString()
		now := time.Now()
		req.CreatedAt = now
		req.UpdatedAt = now
		if strings.TrimSpace(req.CNAME) == "" {
			req.CNAME = s.generateClusterCNAME(req.Name, req.DNSZone)
		}
		if err := s.store.CreateCluster(ctx, &req); err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "23505") {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "集群名称已存在"})
				return
			}
			writeInternalError(w, "create cluster", err)
			return
		}
		writeJSON(w, http.StatusCreated, req)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleClusterByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, "/api/clusters/")

	// Route /api/clusters/{id}/nodes and /api/clusters/{id}/nodes/...
	if idx := strings.Index(path, "/"); idx > 0 {
		id := path[:idx]
		sub := path[idx:]
		if strings.HasPrefix(sub, "/nodes") {
			s.handleClusterNodes(w, r, id, strings.TrimPrefix(sub, "/nodes"))
			return
		}
	}

	id := path

	switch r.Method {
	case http.MethodGet:
		if !isAdmin(ctx) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		cluster, err := s.store.GetCluster(ctx, id)
		if err != nil {
			writeInternalError(w, "get cluster", err)
			return
		}
		if cluster == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "集群不存在"})
			return
		}
		writeJSON(w, http.StatusOK, cluster)
	case http.MethodPut:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		var req store.Cluster
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.ID = id
		req.UpdatedAt = time.Now()
		if strings.TrimSpace(req.CNAME) == "" {
			req.CNAME = s.generateClusterCNAME(req.Name, req.DNSZone)
		}
		if err := s.store.UpdateCluster(ctx, &req); err != nil {
			writeInternalError(w, "update cluster", err)
			return
		}
		s.triggerDNSSync("", "cluster:update")
		updated, _ := s.store.GetCluster(ctx, id)
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		if err := s.store.DeleteCluster(ctx, id); err != nil {
			writeInternalError(w, "delete cluster", err)
			return
		}
		s.triggerDNSSync("", "cluster:delete")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// handleClusterNodes handles the sub-router beneath /api/clusters/{id}/nodes.
// sub carries the remaining path after "/nodes" (so "" for the list/create
// endpoint, "/available" for the unassigned-node helper, or "/{node_id}"
// for per-node PATCH/DELETE).
func (s *Servers) handleClusterNodes(w http.ResponseWriter, r *http.Request, clusterID, sub string) {
	ctx := r.Context()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	// GET /api/clusters/{id}/nodes/available
	if sub == "/available" && r.Method == http.MethodGet {
		allNodes, err := s.store.ListNodes(ctx)
		if err != nil {
			writeInternalError(w, "list nodes", err)
			return
		}
		existing, err := s.store.ListClusterNodes(ctx, clusterID, "")
		if err != nil {
			writeInternalError(w, "list cluster nodes", err)
			return
		}
		usedIDs := make(map[string]bool, len(existing))
		for _, n := range existing {
			usedIDs[n.NodeID] = true
		}
		var available []*store.Node
		for _, n := range allNodes {
			if n != nil && !usedIDs[n.ID] {
				available = append(available, n)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"nodes": available})
		return
	}

	// Route /api/clusters/{id}/nodes/{node_id}
	if len(sub) > 1 && sub[0] == '/' {
		nodeID := sub[1:]
		switch r.Method {
		case http.MethodPatch:
			var req struct {
				Line    *string `json:"line"`
				Enabled *bool   `json:"enabled"`
				Weight  *int32  `json:"weight"`
				Backup  *bool   `json:"backup"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
				return
			}
			line := r.URL.Query().Get("line")
			if line == "" {
				line = "默认"
			}
			existing, err := s.store.ListClusterNodes(ctx, clusterID, line)
			if err != nil {
				writeInternalError(w, "list cluster nodes", err)
				return
			}
			var found *store.ClusterNode
			for _, n := range existing {
				if n.NodeID == nodeID {
					found = n
					break
				}
			}
			if found == nil {
				found = &store.ClusterNode{ClusterID: clusterID, Line: line, NodeID: nodeID, Enabled: true, Weight: 1}
			}
			if req.Enabled != nil {
				found.Enabled = *req.Enabled
			}
			if req.Weight != nil {
				found.Weight = *req.Weight
			}
			if req.Backup != nil {
				found.Backup = *req.Backup
			}
			if req.Line != nil && *req.Line != "" {
				found.Line = *req.Line
			}
			if err := s.store.UpsertClusterNode(ctx, found); err != nil {
				writeInternalError(w, "update cluster node", err)
				return
			}
			s.triggerDNSSync("", "cluster:node_update")
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		case http.MethodDelete:
			line := r.URL.Query().Get("line")
			if line == "" {
				line = "默认"
			}
			if err := s.store.DeleteClusterNode(ctx, clusterID, line, nodeID); err != nil {
				writeInternalError(w, "delete cluster node", err)
				return
			}
			s.triggerDNSSync("", "cluster:unbind")
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		}
		return
	}

	// GET /api/clusters/{id}/nodes
	// POST /api/clusters/{id}/nodes
	switch r.Method {
	case http.MethodGet:
		line := r.URL.Query().Get("line")
		nodes, err := s.store.ListClusterNodes(ctx, clusterID, line)
		if err != nil {
			writeInternalError(w, "list cluster nodes", err)
			return
		}
		allNodes, _ := s.store.ListNodes(ctx)
		nodeMap := make(map[string]*store.Node, len(allNodes))
		for _, n := range allNodes {
			if n != nil {
				nodeMap[n.ID] = n
			}
		}
		type nodeWithMeta struct {
			store.ClusterNode
			Node *store.Node `json:"node,omitempty"`
		}
		out := make([]nodeWithMeta, 0, len(nodes))
		for _, n := range nodes {
			out = append(out, nodeWithMeta{ClusterNode: *n, Node: nodeMap[n.NodeID]})
		}
		writeJSON(w, http.StatusOK, map[string]any{"nodes": out})
	case http.MethodPost:
		var req struct {
			NodeIDs []string `json:"node_ids"`
			Line    string   `json:"line"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if len(req.NodeIDs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "节点ID列表不能为空"})
			return
		}
		line := strings.TrimSpace(req.Line)
		if line == "" {
			line = "默认"
		}
		for _, nid := range req.NodeIDs {
			_ = s.store.UpsertClusterNode(ctx, &store.ClusterNode{
				ClusterID: clusterID,
				Line:      line,
				NodeID:    nid,
				Enabled:   true,
				Weight:    1,
			})
		}
		s.triggerDNSSync("", "cluster:bind")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}
