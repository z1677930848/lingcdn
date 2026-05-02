package server

// Node management handlers: admin-only CRUD over edge nodes, monitor config
// subresource, legacy install command builder, and bootstrap token handout.
// The legacy install handler is retained because older operator runbooks
// curl it directly; new installs use node_install.go.

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/buildinfo"
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
		if node.Token == "" {
			node.Token = uuid.NewString()
		}
		now := time.Now()
		node.CreatedAt = now
		node.UpdatedAt = now

		if err := s.store.CreateNode(ctx, &node); err != nil {
			writeInternalError(w, "create node", err)
			return
		}
		writeJSON(w, http.StatusCreated, node)

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
		if req.Token == "reset" || req.Token == "RESET" {
			req.Token = uuid.NewString()
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
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": req.Token})

	case http.MethodDelete:
		if err := s.store.DeleteNode(ctx, id); err != nil {
			writeInternalError(w, "delete node", err)
			return
		}
		s.hub.Remove(id)
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

// handleNodeInstallCommand returns a ready-to-copy install command (curl + bash) using portal install script.
func (s *Servers) handleNodeInstallCommandLegacy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	shellQuote := func(v string) string {
		return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
	}

	portal := strings.TrimRight(r.URL.Query().Get("portal_base"), "/")
	if portal == "" {
		portal = s.portalBase()
	}
	scriptURL := r.URL.Query().Get("script_url")
	if scriptURL == "" {
		scriptURL = portal + "/node_install.sh"
	}
	masterHost := r.URL.Query().Get("master_host")
	grpcHost, grpcPort, err := net.SplitHostPort(s.cfg.GRPCAddr)
	if err != nil || grpcPort == "" {
		grpcHost = ""
		grpcPort = "9443"
	}
	_ = grpcHost

	resolveIPHostPort := func(hostPort string) (string, bool) {
		host := hostPort
		port := grpcPort
		if h, p, e := net.SplitHostPort(hostPort); e == nil {
			host = h
			port = p
		} else if strings.Contains(hostPort, ":") {
			// likely host:port without brackets for ipv6; keep as-is and validate below
			host = hostPort
		}
		host = strings.TrimSpace(host)
		if ip := net.ParseIP(host); ip == nil {
			return "", false
		}
		if port == "" {
			port = grpcPort
		}
		return net.JoinHostPort(host, port), true
	}

	if masterHost == "" {
		// Prefer explicit public gRPC endpoint, then public IP, then request host.
		if s.cfg.PublicGRPCEndpoint != "" {
			if v, ok := resolveIPHostPort(s.cfg.PublicGRPCEndpoint); ok {
				masterHost = v
			} else {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "PUBLIC_GRPC_ENDPOINT必须为公网IP或IP:端口格式"})
				return
			}
		} else if s.cfg.PublicIP != "" {
			if v, ok := resolveIPHostPort(s.cfg.PublicIP); ok {
				masterHost = v
			} else {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "PUBLIC_IP必须为公网IP"})
				return
			}
		} else {
			hostOnly := r.Host
			if h, _, e := net.SplitHostPort(r.Host); e == nil {
				hostOnly = h
			}
			if v, ok := resolveIPHostPort(hostOnly); ok {
				masterHost = v
			} else {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请配置PUBLIC_IP或PUBLIC_GRPC_ENDPOINT为公网IP"})
				return
			}
		}
	} else {
		// Require master_host to resolve to a public IP address.
		if v, ok := resolveIPHostPort(masterHost); ok {
			masterHost = v
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "master_host必须为公网IP或IP:端口格式"})
			return
		}
	}
	version := r.URL.Query().Get("master_version")
	if version == "" {
		// The install handler stamps the master version into the generated
		// install script so the new node knows which master it's talking to.
		// Pull from buildinfo — never from os.Getenv, which used to be
		// clobberable by a stale /etc/lingcdn/lingcdn.env.
		version = buildinfo.Version()
	}

	ttlMinutes := 60
	if v := r.URL.Query().Get("ttl_minutes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ttlMinutes = n
		}
	}

	token, exp, err := s.store.CreateBootstrapToken(ctx, "install generated", time.Duration(ttlMinutes)*time.Minute)
	if err != nil {
		writeInternalError(w, "create bootstrap token", err)
		return
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	if strings.Contains(scriptURL, "?") {
		scriptURL = scriptURL + "&t=" + ts
	} else {
		scriptURL = scriptURL + "?t=" + ts
	}

	cmd := fmt.Sprintf(
		"curl -fsSL -o node_install.sh %s && bash node_install.sh --master_host %s --master_token %s --master_version %s --portal_base %s%s",
		shellQuote(scriptURL), shellQuote(masterHost), shellQuote(token), shellQuote(version), shellQuote(portal),
		func() string {
			if strings.TrimSpace(s.cfg.UpgradePubKey) == "" {
				return ""
			}
			return " --upgrade_pubkey " + shellQuote(s.cfg.UpgradePubKey)
		}(),
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"command":        cmd,
		"master_host":    masterHost,
		"master_token":   token,
		"master_version": version,
		"expires_at":     exp,
		"portal_base":    portal,
		"script_url":     scriptURL,
		"style":          "master",
	})
}

// handleNodeBootstrapToken returns a short-lived bootstrap token for node installation.
func (s *Servers) handleNodeBootstrapToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	var req struct {
		TTLMinutes  int    `json:"ttl_minutes"`
		Description string `json:"description"`
	}
	if r.Method == http.MethodPost {
		_ = json.NewDecoder(r.Body).Decode(&req)
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
