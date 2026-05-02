package server

// Admin-only API tokens (for UI-less CI/CD callers) and the domain
// blacklist used by node install to prevent onboarding known-abuse domains.
// Kept together because both are thin store-passthroughs wired under the
// same admin auth.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// API Tokens handlers
func (s *Servers) handleAPITokens(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		tokens, err := s.store.ListAPITokens(ctx)
		if err != nil {
			writeInternalError(w, "list API tokens", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
	case http.MethodPost:
		var req struct {
			Description string `json:"description"`
			TTLDays     int    `json:"ttl_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		var ttl time.Duration
		if req.TTLDays > 0 {
			ttl = time.Duration(req.TTLDays) * 24 * time.Hour
		}
		token, t, err := s.store.CreateAPIToken(ctx, req.Description, ttl)
		if err != nil {
			writeInternalError(w, "create API token", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token, "api_token": t})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleAPITokenByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/api-tokens/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少令牌ID"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := s.store.DeleteAPIToken(ctx, id); err != nil {
			writeInternalError(w, "delete API token", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// Domain Blacklist handlers
func (s *Servers) handleDomainBlacklist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListDomainBlacklist(ctx)
		if err != nil {
			writeInternalError(w, "list domain blacklist", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"blacklist": list})
	case http.MethodPost:
		var req struct {
			Domain string `json:"domain"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if strings.TrimSpace(req.Domain) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "域名不能为空"})
			return
		}
		b := &store.DomainBlacklist{
			Domain: req.Domain,
			Reason: req.Reason,
		}
		if err := s.store.CreateDomainBlacklist(ctx, b); err != nil {
			writeInternalError(w, "create domain blacklist entry", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "blacklist": b})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleDomainBlacklistByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/domain-blacklist/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少ID"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := s.store.DeleteDomainBlacklist(ctx, id); err != nil {
			writeInternalError(w, "delete domain blacklist entry", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}
