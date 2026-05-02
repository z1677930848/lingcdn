package server

// Balance account handlers: user-facing read endpoints for their own account,
// transactions, recharges, withdrawals, and the admin counterparts for
// approving recharges / withdrawals and adjusting balances manually.
// Keeping user and admin variants side-by-side because they share request
// shapes and serialization; the admin variants all gate on the "admin" role.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleBalanceAccount(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}
	account, err := s.store.GetBalanceAccount(ctx, userID)
	if err != nil {
		writeInternalError(w, "get balance account", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": account})
}

func (s *Servers) handleBalanceTransactions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "page_size", parseIntQuery(r, "pageSize", 20))
	transactions, total, err := s.store.ListBalanceTransactions(ctx, userID, page, pageSize)
	if err != nil {
		writeInternalError(w, "list balance transactions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transactions": transactions,
		"total":        total,
		"page":         page,
		"page_size":    pageSize,
	})
}

func (s *Servers) handleBalanceRecharges(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "page_size", parseIntQuery(r, "pageSize", 20))
	recharges, total, err := s.store.AdminListBalanceRecharges(ctx, userID, status, page, pageSize)
	if err != nil {
		writeInternalError(w, "list balance recharges", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recharges": recharges,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (s *Servers) handleBalanceWithdrawals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "page_size", parseIntQuery(r, "pageSize", 20))
	withdrawals, total, err := s.store.AdminListBalanceWithdrawals(ctx, userID, status, page, pageSize)
	if err != nil {
		writeInternalError(w, "list balance withdrawals", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"withdrawals": withdrawals,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
	})
}

func (s *Servers) handleAdminBalanceAccounts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "page_size", 10)
	accounts, total, err := s.store.AdminListBalanceAccounts(ctx, userID, page, pageSize)
	if err != nil {
		writeInternalError(w, "list balance accounts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts":  accounts,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (s *Servers) handleAdminBalanceRecharges(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "page_size", 10)
	recharges, total, err := s.store.AdminListBalanceRecharges(ctx, userID, status, page, pageSize)
	if err != nil {
		writeInternalError(w, "list balance recharges", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recharges": recharges,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (s *Servers) handleAdminBalanceRechargeByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/balance/recharges/")
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	var req struct {
		Status  string `json:"status"`
		TradeNo string `json:"trade_no"`
		PaidAt  string `json:"paid_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}
	paidAt := time.Time{}
	if strings.TrimSpace(req.PaidAt) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.PaidAt)); err == nil {
			paidAt = t
		}
	}
	if err := s.store.AdminUpdateBalanceRecharge(ctx, id, strings.TrimSpace(req.Status), strings.TrimSpace(req.TradeNo), paidAt); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Servers) handleAdminBalanceWithdrawals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "page_size", 10)
	withdrawals, total, err := s.store.AdminListBalanceWithdrawals(ctx, userID, status, page, pageSize)
	if err != nil {
		writeInternalError(w, "list balance withdrawals", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"withdrawals": withdrawals,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
	})
}

func (s *Servers) handleAdminBalanceWithdrawalByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/balance/withdrawals/")
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	var req struct {
		Status     string `json:"status"`
		Note       string `json:"note"`
		ReviewedAt string `json:"reviewed_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}
	reviewedAt := time.Time{}
	if strings.TrimSpace(req.ReviewedAt) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ReviewedAt)); err == nil {
			reviewedAt = t
		}
	}
	if err := s.store.AdminUpdateBalanceWithdrawal(ctx, id, strings.TrimSpace(req.Status), strings.TrimSpace(req.Note), reviewedAt); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAdminBalanceAdjust applies a signed cents-delta to a user's balance.
// user_id is preferred; if empty, the identifier is resolved as either an ID
// or a login (email/username) via the user store. Zero-amount requests are
// rejected so an accidental empty-form submission doesn't create a no-op
// audit entry.
func (s *Servers) handleAdminBalanceAdjust(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	var req struct {
		UserID     string `json:"user_id"`
		Identifier string `json:"identifier"`
		Amount     int64  `json:"amount_cents"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}
	userID := strings.TrimSpace(req.UserID)
	identifier := strings.TrimSpace(req.Identifier)
	if userID == "" && identifier != "" {
		if u, err := s.store.GetUserByID(ctx, identifier); err != nil {
			writeInternalError(w, "get user by ID", err)
			return
		} else if u != nil {
			userID = u.ID
		} else if u2, err := s.store.GetUserByLogin(ctx, identifier); err != nil {
			writeInternalError(w, "get user by login", err)
			return
		} else if u2 != nil {
			userID = u2.ID
		}
	}
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "用户不存在"})
		return
	}
	if req.Amount == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "金额不能为空"})
		return
	}
	if err := s.store.AdminAdjustBalance(ctx, userID, req.Amount, strings.TrimSpace(req.Note)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user_id": userID})
}

// handleAdminBalanceStats returns aggregate recharge stats for a date range.
// Dates are parsed as calendar days (YYYY-MM-DD) in the server's local time.
func (s *Servers) handleAdminBalanceStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
	toRaw := strings.TrimSpace(r.URL.Query().Get("to"))
	// Alternate query param names accepted for UI convenience.
	if fromRaw == "" {
		fromRaw = strings.TrimSpace(r.URL.Query().Get("start_date"))
	}
	if toRaw == "" {
		toRaw = strings.TrimSpace(r.URL.Query().Get("end_date"))
	}
	if fromRaw == "" || toRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的日期范围"})
		return
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的日期范围"})
		return
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的日期范围"})
		return
	}
	if from.After(to) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的日期范围"})
		return
	}
	stats, err := s.store.AdminRechargeStats(ctx, from, to)
	if err != nil {
		writeInternalError(w, "query recharge stats", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}
