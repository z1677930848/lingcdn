package server

// Node management handlers: admin-only CRUD over edge nodes, monitor config
// subresource, and bootstrap token handout. The install-command builder
// lives in node_install.go.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

// Nodes handlers
func (s *Servers) handleNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		nodes, err := s.store.ListNodes(ctx)
		if err != nil {
			writeInternalError(w, "list nodes", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})

	case http.MethodPost:
		var node store.Node
		if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if node.Hostname == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "主机名不能为空"})
			return
		}
		if existing, _ := s.store.GetNodeByHostname(ctx, node.Hostname); existing != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "主机名已存在"})
			return
		}
		// 与 install-command / install-ssh / RegisterNode 的策略保持一致：
		// 在 license 不允许新增节点时，禁止管理员手工 POST 创建节点。
		if err := s.preInstallNodeLicenseCheck(ctx); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
			return
		}
		if node.ID == "" {
			node.ID = uuid.NewString()
		}
		node.Status = "pending"
		plaintextToken := strings.TrimSpace(node.Token)
		if plaintextToken == "" {
			plaintextToken = uuid.NewString()
		}
		node.Token = hashToken(plaintextToken)
		now := time.Now()
		node.CreatedAt = now
		node.UpdatedAt = now

		if err := s.store.CreateNode(ctx, &node); err != nil {
			writeInternalError(w, "create node", err)
			return
		}
		created := node
		created.Token = plaintextToken
		writeJSON(w, http.StatusCreated, created)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleNodeByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
	if strings.HasSuffix(id, "/monitor") {
		id = strings.TrimSuffix(id, "/monitor")
		s.handleNodeMonitorConfig(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		node, err := s.store.GetNode(ctx, id)
		if err != nil {
			writeInternalError(w, "get node", err)
			return
		}
		if node == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "节点不存在"})
			return
		}
		writeJSON(w, http.StatusOK, node)

	case http.MethodPut:
		var req store.Node
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.ID = id
		var plaintextToken string
		if req.Token == "reset" || req.Token == "RESET" {
			plaintextToken = uuid.NewString()
			req.Token = hashToken(plaintextToken)
		} else {
			// Never persist a raw token from the client body on ordinary updates.
			req.Token = ""
		}
		// Do not allow hostname conflict
		if req.Hostname != "" {
			if existing, _ := s.store.GetNodeByHostname(ctx, req.Hostname); existing != nil && existing.ID != id {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "主机名已存在"})
				return
			}
		}
		if err := s.store.UpdateNode(ctx, &req); err != nil {
			writeInternalError(w, "update node", err)
			return
		}
		if strings.EqualFold(req.Status, "disabled") {
			s.hub.Remove(id)
			s.triggerDNSSync("", "node:disable")
		} else if strings.EqualFold(req.Status, "online") {
			s.triggerDNSSync("", "node:enable")
			// Proactively fan out the latest config to this node so the UI
			// surfaces a visible sync task and the node picks up any changes
			// made while it was disabled. If the node's agent hasn't
			// reconnected yet, hub.SendConfig returns ErrNodeNotConnected and
			// the task records a failure — that's fine: the subsequent
			// StreamConfig first-frame (BuildConfigEnvelope) delivers the
			// same envelope when the agent does come back, so this serves as
			// an immediate push for already-reconnected nodes plus a
			// best-effort nudge for the rest.
			_ = s.startPublishTask(ctx, "auto", "node:"+id, "node:enable:"+id, "", []string{id})
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": plaintextToken})

	case http.MethodDelete:
		if err := s.store.DeleteNode(ctx, id); err != nil {
			writeInternalError(w, "delete node", err)
			return
		}
		s.hub.Remove(id)
		s.triggerDNSSync("", "node:delete")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

type nodeMonitorConfigPayload struct {
	Enabled        *bool   `json:"enabled"`
	Protocol       *string `json:"protocol"`
	TimeoutSeconds *int    `json:"timeout_seconds"`
	Port           *int    `json:"port"`
	FailThreshold  *int    `json:"fail_threshold"`
}

func (s *Servers) handleNodeMonitorConfig(w http.ResponseWriter, r *http.Request, nodeID string) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if strings.TrimSpace(nodeID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "节点ID不能为空"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		node, err := s.store.GetNode(ctx, nodeID)
		if err != nil {
			writeInternalError(w, "get node", err)
			return
		}
		if node == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "节点不存在"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":         node.MonitorEnabled,
			"protocol":        node.MonitorProtocol,
			"timeout_seconds": node.MonitorTimeout,
			"port":            node.MonitorPort,
			"fail_threshold":  node.MonitorFailThreshold,
			"fail_count":      node.MonitorFailCount,
			"last_ok":         node.MonitorLastOK,
			"last_error":      node.MonitorLastError,
			"last_at":         node.MonitorLastAt,
			"last_latency_ms": node.MonitorLastLatencyMs,
		})
	case http.MethodPut:
		var payload nodeMonitorConfigPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}

		node, err := s.store.GetNode(ctx, nodeID)
		if err != nil {
			writeInternalError(w, "get node", err)
			return
		}
		if node == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "节点不存在"})
			return
		}

		cfg := store.NodeMonitorConfig{
			Enabled:        node.MonitorEnabled,
			Protocol:       node.MonitorProtocol,
			TimeoutSeconds: node.MonitorTimeout,
			Port:           node.MonitorPort,
			FailThreshold:  node.MonitorFailThreshold,
		}
		if payload.Enabled != nil {
			cfg.Enabled = *payload.Enabled
		}
		if payload.Protocol != nil {
			cfg.Protocol = strings.ToLower(strings.TrimSpace(*payload.Protocol))
		}
		if payload.TimeoutSeconds != nil {
			cfg.TimeoutSeconds = *payload.TimeoutSeconds
		}
		if payload.Port != nil {
			cfg.Port = *payload.Port
		}
		if payload.FailThreshold != nil {
			cfg.FailThreshold = *payload.FailThreshold
		}

		if err := validateNodeMonitorConfig(cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := s.store.UpdateNodeMonitorConfig(ctx, nodeID, cfg); err != nil {
			writeInternalError(w, "update node monitor config", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func validateNodeMonitorConfig(cfg store.NodeMonitorConfig) error {
	proto := strings.ToLower(strings.TrimSpace(cfg.Protocol))
	if proto == "" {
		proto = "http"
	}
	switch proto {
	case "http", "tcp", "ping":
	default:
		return fmt.Errorf("unsupported protocol: %s", proto)
	}
	if cfg.TimeoutSeconds <= 0 || cfg.TimeoutSeconds > 60 {
		return fmt.Errorf("timeout_seconds must be between 1 and 60")
	}
	if proto != "ping" {
		if cfg.Port <= 0 || cfg.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	}
	if cfg.FailThreshold <= 0 || cfg.FailThreshold > 10 {
		return fmt.Errorf("fail_threshold must be between 1 and 10")
	}
	return nil
}

// handleNodeBootstrapToken returns a short-lived bootstrap token for node installation.
//
// 与 install-command / install-ssh / POST /api/nodes 对齐的 license 预检：
// 如果当前授权已达节点数上限或整体处于非 active 状态，直接拒绝铸 token，
// 否则管理员仍能拿到一个 60 分钟的 bootstrap token，然后节点安装跑到
// gRPC RegisterNode 那一步才被拒 —— 目标主机已经装了一半（审计 #P1-3）。
func (s *Servers) handleNodeBootstrapToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if err := s.preInstallNodeLicenseCheck(ctx); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
		return
	}
	var req struct {
		TTLMinutes  int    `json:"ttl_minutes"`
		Description string `json:"description"`
	}
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
	} else {
		if v := r.URL.Query().Get("ttl_minutes"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				req.TTLMinutes = n
			}
		}
		req.Description = r.URL.Query().Get("description")
	}
	if req.TTLMinutes <= 0 {
		req.TTLMinutes = 60
	}
	token, exp, err := s.store.CreateBootstrapToken(ctx, req.Description, time.Duration(req.TTLMinutes)*time.Minute)
	if err != nil {
		writeInternalError(w, "create bootstrap token", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": exp,
	})
}
