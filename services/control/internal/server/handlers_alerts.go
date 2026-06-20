package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleAlertRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		rules, err := s.store.ListAlertRules(ctx)
		if err != nil {
			writeInternalError(w, "list alert rules", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"alert_rules": rules})
	case http.MethodPost:
		var rule store.AlertRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if strings.TrimSpace(rule.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		if rule.ID == "" {
			rule.ID = uuid.NewString()
		}
		if rule.Metric == "" {
			rule.Metric = "cpu_usage"
		}
		if rule.Severity == "" {
			rule.Severity = "warning"
		}
		if rule.WindowSeconds <= 0 {
			rule.WindowSeconds = 60
		}
		now := time.Now()
		rule.CreatedAt = now
		rule.UpdatedAt = now
		if err := s.store.CreateAlertRule(ctx, &rule); err != nil {
			writeInternalError(w, "create alert rule", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"alert_rule": rule})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleAlertRuleByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/alerts/")
	id = strings.TrimSpace(id)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "规则ID不能为空"})
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		rule, err := s.store.GetAlertRule(ctx, id)
		if err != nil {
			writeInternalError(w, "get alert rule", err)
			return
		}
		if rule == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "告警规则不存在"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"alert_rule": rule})
	case http.MethodPut, http.MethodPatch:
		existing, err := s.store.GetAlertRule(ctx, id)
		if err != nil {
			writeInternalError(w, "get alert rule", err)
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "告警规则不存在"})
			return
		}
		var rule store.AlertRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		rule.ID = id
		if rule.CreatedAt.IsZero() {
			rule.CreatedAt = existing.CreatedAt
		}
		rule.UpdatedAt = time.Now()
		if err := s.store.UpdateAlertRule(ctx, &rule); err != nil {
			writeInternalError(w, "update alert rule", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"alert_rule": rule})
	case http.MethodDelete:
		if err := s.store.DeleteAlertRule(ctx, id); err != nil {
			writeInternalError(w, "delete alert rule", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}
