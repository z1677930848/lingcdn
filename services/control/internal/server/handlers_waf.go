package server

// WAF (Web Application Firewall) admin handlers: policy/rule/ban/whitelist
// CRUD consumed by the UI. Authorization rule: admins act at any scope;
// regular users are restricted to domain-scoped policies for domains they
// own and need an active product with custom_cc_rules enabled.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
	wafrules "github.com/lingcdn/control/internal/waf"
)

func (s *Servers) handleWAFPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		policies, err := s.store.ListWAFPolicies(ctx)
		if err != nil {
			writeInternalError(w, "list WAF policies", err)
			return
		}
		if !isAdmin(ctx) {
			policies, err = s.filterWAFPoliciesForUser(ctx, policies)
			if err != nil {
				writeInternalError(w, "filter WAF policies", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"policies": policies})
	case http.MethodPost:
		var req store.WAFPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		if req.Scope == "" {
			req.Scope = "global"
		}
		if !isValidWAFScope(req.Scope) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的作用域"})
			return
		}
		if !s.requireTenantWAFMutation(w, ctx, req.Scope, req.ScopeID) {
			return
		}
		if req.ID == "" {
			req.ID = uuid.NewString()
		}
		now := time.Now()
		req.CreatedAt = now
		req.UpdatedAt = now
		if err := s.store.CreateWAFPolicy(ctx, &req); err != nil {
			writeInternalError(w, "create WAF policy", err)
			return
		}
		if err := s.replaceWAFRules(ctx, req.ID, req.Rules, now); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		_ = s.startPublishTask(ctx, "auto", "", "waf.policy.create:"+req.ID, "", nil)
		pol, _ := s.store.GetWAFPolicy(ctx, req.ID)
		writeJSON(w, http.StatusCreated, pol)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleWAFPolicyByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := strings.TrimPrefix(r.URL.Path, "/api/waf/policies/")

	// Dedicated "toggle" endpoint: PATCH /api/waf/policies/{id}/enabled
	// The list view calls this from a small switch on each row; we deliberately
	// avoid round-tripping the policy's entire rule set because any pre-existing
	// rule that no longer satisfies replaceWAFRules' stricter validation would
	// reject the toggle — surfacing as a mysterious "保存失败" for users who
	// only wanted to flip a switch.
	if strings.HasSuffix(id, "/enabled") {
		id = strings.TrimSuffix(id, "/enabled")
		s.handleWAFPolicyEnabled(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		pol, err := s.store.GetWAFPolicy(ctx, id)
		if err != nil {
			writeInternalError(w, "get WAF policy", err)
			return
		}
		if pol == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "策略不存在"})
			return
		}
		if !s.canReadWAFPolicy(ctx, pol) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权查看此防护策略"})
			return
		}
		writeJSON(w, http.StatusOK, pol)
	case http.MethodPut:
		existing, err := s.store.GetWAFPolicy(ctx, id)
		if err != nil {
			writeInternalError(w, "get WAF policy", err)
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "策略不存在"})
			return
		}
		if !s.canReadWAFPolicy(ctx, existing) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权管理此防护策略"})
			return
		}
		var req store.WAFPolicy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.ID = id
		if strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		if req.Scope == "" {
			req.Scope = "global"
		}
		if !isValidWAFScope(req.Scope) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的作用域"})
			return
		}
		if !s.requireTenantWAFMutation(w, ctx, req.Scope, req.ScopeID) {
			return
		}
		req.UpdatedAt = time.Now()
		if err := s.store.UpdateWAFPolicy(ctx, &req); err != nil {
			writeInternalError(w, "update WAF policy", err)
			return
		}
		if err := s.replaceWAFRules(ctx, req.ID, req.Rules, req.UpdatedAt); err != nil {
			writeInternalError(w, "replace WAF rules", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "", "waf.policy.update:"+id, "", nil)
		pol, _ := s.store.GetWAFPolicy(ctx, id)
		writeJSON(w, http.StatusOK, pol)
	case http.MethodDelete:
		if !isAdmin(ctx) {
			userID := getUserID(ctx)
			existing, err := s.store.GetWAFPolicy(ctx, id)
			if err != nil {
				writeInternalError(w, "get WAF policy", err)
				return
			}
			if existing == nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "策略不存在"})
				return
			}
			if userID == "" || existing.Scope != "domain" || strings.TrimSpace(existing.ScopeID) == "" {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权删除此防护策略"})
				return
			}
			scopeDomain, _ := s.store.GetDomain(ctx, existing.ScopeID)
			if scopeDomain == nil || scopeDomain.UserID != userID {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权删除此防护策略"})
				return
			}
		}
		if err := s.store.DeleteWAFPolicy(ctx, id); err != nil {
			writeInternalError(w, "delete WAF policy", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "", "waf.policy.delete:"+id, "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// replaceWAFRules normalizes, validates, and atomically replaces all rules
// attached to a policy. Validation errors surface as 400s (via the caller);
// store errors propagate unchanged as 500s.
//
// Historically this function rejected the *entire* batch if any one rule had
// drifted from today's invariants (e.g. legacy rate_limit rows saved before
// window_seconds was enforced, or challenge_captcha rules with an obsolete
// captcha_type). In practice that meant the admin UI's "enable" toggle — which
// round-trips the whole policy including existing rules — would fail with a
// validation error for a field the user never touched. To make ordinary
// interactions robust, we now *repair* such fields to sensible defaults rather
// than refusing the whole PUT. Hard-invalid rule types/actions still error so
// typos and malformed POSTs don't silently corrupt data.
func (s *Servers) replaceWAFRules(ctx context.Context, policyID string, rules []*store.WAFRule, now time.Time) error {
	for _, r := range rules {
		if r == nil {
			continue
		}
		if r.ID == "" {
			r.ID = uuid.NewString()
		}
		r.PolicyID = policyID
		if r.CreatedAt.IsZero() {
			r.CreatedAt = now
		}
		r.UpdatedAt = now
		if r.Action == "" {
			r.Action = "deny"
		}
		if r.Type == "" {
			r.Type = "ip_cidr"
		}
		if !isValidWAFRuleType(r.Type) {
			return fmt.Errorf("无效的规则类型: %s", r.Type)
		}
		if !isValidWAFAction(r.Action) {
			return fmt.Errorf("无效的规则动作: %s", r.Action)
		}
		if isWAFRegexRuleType(r.Type) {
			if strings.TrimSpace(r.Value) == "" {
				return fmt.Errorf("规则 %s 需要填写匹配模式", r.Type)
			}
			if _, err := regexp.Compile(r.Value); err != nil {
				return fmt.Errorf("无效的正则表达式: %s", r.Value)
			}
		}
		if r.Type == "method_block" {
			if len(r.Methods) == 0 && strings.TrimSpace(r.Value) != "" {
				r.Methods = []string{strings.ToUpper(strings.TrimSpace(r.Value))}
			}
			if len(r.Methods) == 0 {
				return fmt.Errorf("method_block 规则需要指定 HTTP 方法")
			}
		}
		if r.Type == "ip_cidr" && strings.TrimSpace(r.Value) != "" {
			if _, _, err := net.ParseCIDR(r.Value); err != nil {
				if ip := net.ParseIP(strings.TrimSpace(r.Value)); ip == nil {
					return fmt.Errorf("无效的 IP/CIDR 格式: %s", r.Value)
				}
			}
		}
		if r.Type == "rate_limit" {
			// Repair instead of reject: legacy rows with 0 threshold/window
			// should not block the user from toggling/saving the policy.
			if r.Threshold <= 0 {
				r.Threshold = 60
			}
			if r.WindowSeconds <= 0 {
				r.WindowSeconds = 60
			}
			if r.WindowSeconds > 86400 {
				r.WindowSeconds = 86400
			}
		}
		if r.Type == "challenge_captcha" && r.CaptchaType != "" {
			if !isValidCaptchaType(r.CaptchaType) {
				// Unknown captcha types are coerced to empty so the node
				// falls back to its default; previously this returned 400
				// and orphaned the whole policy.
				r.CaptchaType = ""
			}
		}
		if r.BanMode != "" && !isValidBanMode(r.BanMode) {
			r.BanMode = "ipset"
		}
		if r.BanSeconds < 0 {
			r.BanSeconds = 0
		}
		if r.AutoChallengeQPS < 0 {
			r.AutoChallengeQPS = 0
		}
		if r.ShieldSeconds == 0 {
			r.ShieldSeconds = 5
		}
		if r.BanSeconds == 0 {
			r.BanSeconds = 300
		}
	}
	return s.store.ReplaceWAFRules(ctx, policyID, rules)
}

func isValidWAFRuleType(t string) bool {
	switch t {
	case "ip_cidr", "rate_limit", "challenge_captcha", "shield_5s",
		"geo_block", "block_transparent_proxy",
		"sql_injection", "xss", "path_traversal", "ua_block", "method_block":
		return true
	default:
		return false
	}
}

func isWAFRegexRuleType(t string) bool {
	switch t {
	case "sql_injection", "xss", "path_traversal", "ua_block":
		return true
	default:
		return false
	}
}

func isValidWAFAction(a string) bool {
	switch a {
	case "allow", "deny":
		return true
	default:
		return false
	}
}

func isValidCaptchaType(t string) bool {
	switch t {
	case "slide", "click", "rotate", "slide_region", "js_challenge":
		return true
	default:
		return false
	}
}

func isValidBanMode(m string) bool {
	switch m {
	case "ipset", "drop", "page":
		return true
	default:
		return false
	}
}

func isValidWAFScope(scope string) bool {
	switch strings.ToLower(scope) {
	case "global", "domain", "line_group":
		return true
	default:
		return false
	}
}

// requireAdminWAF rejects non-admin callers for fleet-wide WAF resources.
func requireAdminWAF(w http.ResponseWriter, ctx context.Context) bool {
	if isAdmin(ctx) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]any{"error": "仅管理员可操作"})
	return false
}

// requireTenantWAFMutation checks that a non-admin user may create/update a
// domain-scoped WAF policy. Admins always pass. Empty userID is rejected.
func (s *Servers) requireTenantWAFMutation(w http.ResponseWriter, ctx context.Context, scope, scopeID string) bool {
	if isAdmin(ctx) {
		return true
	}
	if !s.requireUserPermission(w, ctx, PermWAFEdit) {
		return false
	}
	userID := getUserID(ctx)
	if userID == "" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权操作防护策略"})
		return false
	}
	product, _ := s.getUserActiveProduct(ctx, userID, "")
	if product == nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无有效套餐，请先购买套餐"})
		return false
	}
	if !product.CustomCCRules {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "当前套餐不支持自定义 CC 规则"})
		return false
	}
	if scope != "domain" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "非管理员只能管理域名级别的防护策略"})
		return false
	}
	if strings.TrimSpace(scopeID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "域名作用域需要scope_id"})
		return false
	}
	scopeDomain, err := s.store.GetDomain(ctx, scopeID)
	if err != nil {
		writeInternalError(w, "get domain for scope check", err)
		return false
	}
	if scopeDomain == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "scope_id 对应的域名不存在"})
		return false
	}
	if scopeDomain.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权管理此域名的防护策略"})
		return false
	}
	return true
}

func (s *Servers) canReadWAFPolicy(ctx context.Context, pol *store.WAFPolicy) bool {
	if pol == nil {
		return false
	}
	if isAdmin(ctx) {
		return true
	}
	userID := getUserID(ctx)
	if userID == "" {
		return false
	}
	if pol.Scope != "domain" || strings.TrimSpace(pol.ScopeID) == "" {
		return false
	}
	scopeDomain, _ := s.store.GetDomain(ctx, pol.ScopeID)
	return scopeDomain != nil && scopeDomain.UserID == userID
}

func (s *Servers) filterWAFPoliciesForUser(ctx context.Context, policies []*store.WAFPolicy) ([]*store.WAFPolicy, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, nil
	}
	filtered := make([]*store.WAFPolicy, 0)
	for _, pol := range policies {
		if pol == nil {
			continue
		}
		if pol.Scope != "domain" || strings.TrimSpace(pol.ScopeID) == "" {
			continue
		}
		scopeDomain, err := s.store.GetDomain(ctx, pol.ScopeID)
		if err != nil {
			return nil, err
		}
		if scopeDomain != nil && scopeDomain.UserID == userID {
			filtered = append(filtered, pol)
		}
	}
	return filtered, nil
}

// WAF bans CRUD (simple)
func (s *Servers) handleWAFBans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !requireAdminWAF(w, ctx) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		bans, err := s.store.ListWAFBans(ctx, 200)
		if err != nil {
			writeInternalError(w, "list WAF bans", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"bans": bans})
	case http.MethodPost:
		var req store.WAFBan
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if strings.TrimSpace(req.IP) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "IP不能为空"})
			return
		}
		if req.Strikes <= 0 {
			req.Strikes = 1
		}
		if req.ExpiresAt.IsZero() {
			req.ExpiresAt = time.Now().Add(10 * time.Minute)
		}
		if req.CreatedAt.IsZero() {
			req.CreatedAt = time.Now()
		}
		req.UpdatedAt = time.Now()
		if err := s.store.CreateOrUpdateWAFBan(ctx, &req); err != nil {
			writeInternalError(w, "create WAF ban", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "", "waf.ban.upsert:"+strings.TrimSpace(req.IP), "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		ip := r.URL.Query().Get("ip")
		if strings.TrimSpace(ip) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "IP不能为空"})
			return
		}
		if err := s.store.DeleteWAFBan(ctx, ip); err != nil {
			writeInternalError(w, "delete WAF ban", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "", "waf.ban.delete:"+strings.TrimSpace(ip), "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// WAF whitelist CRUD
func (s *Servers) handleWAFWhitelist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !requireAdminWAF(w, ctx) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListWAFWhitelist(ctx)
		if err != nil {
			writeInternalError(w, "list WAF whitelist", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"whitelist": list})
	case http.MethodPost:
		var req store.WAFWhitelist
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if strings.TrimSpace(req.IP) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "IP不能为空"})
			return
		}
		if req.ID == "" {
			req.ID = uuid.NewString()
		}
		req.CreatedAt = time.Now()
		req.UpdatedAt = time.Now()
		if err := s.store.CreateWAFWhitelist(ctx, &req); err != nil {
			writeInternalError(w, "create WAF whitelist", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "", "waf.whitelist.create:"+strings.TrimSpace(req.IP), "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": req.ID})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if strings.TrimSpace(id) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
			return
		}
		if err := s.store.DeleteWAFWhitelist(ctx, id); err != nil {
			writeInternalError(w, "delete WAF whitelist", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "", "waf.whitelist.delete:"+strings.TrimSpace(id), "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

type ccPolicyPayload struct {
	Level           string `json:"level"`             // low|medium|high
	Action          string `json:"action"`            // challenge|shield
	CaptchaType     string `json:"captcha_type"`      // slide|click|rotate|slide_region|js_challenge
	BanSeconds      int64  `json:"ban_seconds"`       // base ban seconds
	FailLimit       int64  `json:"fail_limit"`        // challenge fail limit
	TemplateHTML    string `json:"template_html"`     // optional challenge HTML
	BanTemplateHTML string `json:"ban_template_html"` // optional ban HTML
	RedirectURL     string `json:"redirect_url"`      // optional challenge redirect
	BanMode         string `json:"ban_mode"`          // ipset | drop | page
}

// handleCCPolicy manages global CC policy.
func (s *Servers) handleCCPolicy(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	const policyID = "global-cc"
	switch r.Method {
	case http.MethodGet:
		pol, _ := s.store.GetWAFPolicy(ctx, policyID)
		resp := ccPolicyPayload{
			Level:      "medium",
			Action:     "challenge",
			BanSeconds: 300,
			FailLimit:  3,
			BanMode:    "ipset",
		}
		if pol != nil && len(pol.Rules) > 0 {
			rule := pol.Rules[0]
			qps := rule.AutoChallengeQPS
			switch qps {
			case 20:
				resp.Level = "low"
			case 200:
				resp.Level = "high"
			default:
				resp.Level = "medium"
			}
			if rule.Type == "shield_5s" {
				resp.Action = "shield"
			} else {
				resp.Action = "challenge"
			}
			if rule.BanSeconds > 0 {
				resp.BanSeconds = rule.BanSeconds
			}
			if rule.Threshold > 0 {
				resp.FailLimit = rule.Threshold
			}
			resp.TemplateHTML = rule.TemplateHTML
			resp.BanTemplateHTML = rule.BanTemplateHTML
			if strings.TrimSpace(resp.TemplateHTML) == "" {
				if tpl, err := s.defaultWAFChallengeTemplate(ctx); err == nil {
					resp.TemplateHTML = tpl
				}
			}
			if strings.TrimSpace(resp.BanTemplateHTML) == "" && strings.ToLower(resp.BanMode) == "page" {
				if tpl, err := s.defaultWAFBanTemplate(ctx); err == nil {
					resp.BanTemplateHTML = tpl
				}
			}
			resp.RedirectURL = rule.RedirectURL
			if rule.BanMode != "" {
				resp.BanMode = rule.BanMode
			}
			if rule.CaptchaType != "" {
				resp.CaptchaType = rule.CaptchaType
			}
		}
		if !isAdmin(ctx) {
			resp.TemplateHTML = ""
			resp.BanTemplateHTML = ""
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		if !requireAdminWAF(w, ctx) {
			return
		}
		var payload ccPolicyPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		levelMap := map[string]int64{
			"low":    20,
			"medium": 50,
			"high":   200,
		}
		qps, ok := levelMap[strings.ToLower(payload.Level)]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的级别"})
			return
		}
		action := strings.ToLower(payload.Action)
		if action != "challenge" && action != "shield" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的操作"})
			return
		}
		banMode := strings.ToLower(payload.BanMode)
		if banMode == "" {
			banMode = "ipset"
		}
		if banMode != "ipset" && banMode != "drop" && banMode != "page" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的封禁模式"})
			return
		}
		if payload.BanSeconds <= 0 {
			payload.BanSeconds = 300
		}
		if payload.FailLimit <= 0 {
			payload.FailLimit = 3
		}
		now := time.Now()
		pol := &store.WAFPolicy{
			ID:          policyID,
			Name:        "Global CC",
			Scope:       "global",
			ScopeID:     "",
			Description: "Global CC policy",
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_ = s.store.CreateWAFPolicy(ctx, pol)

		rule := &store.WAFRule{
			ID:               uuid.NewString(),
			PolicyID:         policyID,
			Type:             "challenge_captcha",
			Action:           "deny",
			Value:            "",
			Threshold:        payload.FailLimit,
			WindowSeconds:    1,
			ShieldSeconds:    5,
			AutoChallengeQPS: qps,
			BanSeconds:       payload.BanSeconds,
			CaptchaType:      strings.TrimSpace(payload.CaptchaType),
			TemplateHTML:     payload.TemplateHTML,
			BanTemplateHTML:  payload.BanTemplateHTML,
			RedirectURL:      payload.RedirectURL,
			BanMode:          banMode,
			Priority:         1,
			Enabled:          true,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if action == "shield" {
			rule.Type = "shield_5s"
			rule.Threshold = 0
			rule.TemplateHTML = ""
			rule.BanTemplateHTML = ""
			rule.RedirectURL = ""
		}

		if err := s.replaceWAFRules(ctx, policyID, []*store.WAFRule{rule}, now); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		task := s.startPublishTask(r.Context(), "auto", "", "waf.cc", "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "qps": qps, "action": action, "publish_task_id": task.ID})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// handleWAFPolicyEnabled flips ONLY the enabled flag on a policy. We split
// this out from the generic PUT handler because the list page's per-row
// switch does not have the policy's full rule set in hand, and forcing a
// round-trip through replaceWAFRules would (a) require the UI to preserve
// every rule field verbatim and (b) fail for any pre-existing rule that no
// longer matches the current validation schema. This endpoint is authorized
// identically to PUT but touches only the policy row.
func (s *Servers) handleWAFPolicyEnabled(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx := r.Context()
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "策略ID不能为空"})
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	existing, err := s.store.GetWAFPolicy(ctx, id)
	if err != nil {
		writeInternalError(w, "get WAF policy", err)
		return
	}
	if existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "策略不存在"})
		return
	}

	// Same ownership rules as PUT: non-admins may only toggle domain-scoped
	// policies they own, and their product must allow custom CC rules.
	if !s.canReadWAFPolicy(ctx, existing) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权管理此防护策略"})
		return
	}
	if !isAdmin(ctx) {
		userID := getUserID(ctx)
		if userID == "" {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权管理此防护策略"})
			return
		}
		product, _ := s.getUserActiveProduct(ctx, userID, "")
		if product == nil {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "无有效套餐，请先购买套餐"})
			return
		}
		if !product.CustomCCRules {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "当前套餐不支持自定义 CC 规则"})
			return
		}
	}

	existing.Enabled = body.Enabled
	existing.UpdatedAt = time.Now()
	if err := s.store.UpdateWAFPolicy(ctx, existing); err != nil {
		writeInternalError(w, "update WAF policy enabled", err)
		return
	}
	_ = s.startPublishTask(ctx, "auto", "", "waf.policy.enabled:"+id, "", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": existing.Enabled})
}

func (s *Servers) handleWAFRulesetApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/waf/rulesets/")
	name = strings.Trim(strings.TrimSuffix(name, "/apply"), "/")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "规则集名称不能为空"})
		return
	}

	rs, ok := wafrules.GetRuleset(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "规则集不存在"})
		return
	}

	ctx := r.Context()
	var body struct {
		PolicyID string `json:"policy_id"`
		Scope    string `json:"scope"`
		ScopeID  string `json:"scope_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	policyID := strings.TrimSpace(body.PolicyID)
	now := time.Now()
	var policy *store.WAFPolicy

	if policyID != "" {
		policy, _ = s.store.GetWAFPolicy(ctx, policyID)
	}
	if policy == nil {
		policy = &store.WAFPolicy{
			ID:          uuid.NewString(),
			Name:        "ruleset:" + name,
			Scope:       strings.TrimSpace(body.Scope),
			ScopeID:     strings.TrimSpace(body.ScopeID),
			Description: rs.Description,
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if policy.Scope == "" {
			policy.Scope = "global"
		}
		if err := s.store.CreateWAFPolicy(ctx, policy); err != nil {
			writeInternalError(w, "create WAF policy for ruleset", err)
			return
		}
	} else {
		policy.UpdatedAt = now
		if err := s.store.UpdateWAFPolicy(ctx, policy); err != nil {
			writeInternalError(w, "update WAF policy for ruleset", err)
			return
		}
	}

	rules := make([]*store.WAFRule, 0, len(rs.Rules))
	for _, rule := range rs.Rules {
		if rule == nil {
			continue
		}
		cp := *rule
		if cp.ID == "" {
			cp.ID = uuid.NewString()
		}
		cp.CreatedAt = now
		cp.UpdatedAt = now
		rules = append(rules, &cp)
	}
	if err := s.replaceWAFRules(ctx, policy.ID, rules, now); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	_ = s.startPublishTask(ctx, "auto", "", "waf.ruleset.apply:"+name, "", nil)
	pol, _ := s.store.GetWAFPolicy(ctx, policy.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ruleset": name, "policy": pol})
}
