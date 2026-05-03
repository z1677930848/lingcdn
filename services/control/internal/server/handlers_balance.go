package server

// Balance account handlers: user-facing read endpoints for their own account,
// transactions, recharges, withdrawals, and the admin counterparts for
// approving recharges / withdrawals and adjusting balances manually.
// Keeping user and admin variants side-by-side because they share request
// shapes and serialization; the admin variants all gate on the "admin" role.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lingcdn/control/internal/store"
	"github.com/rs/zerolog/log"
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
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}

	switch r.Method {
	case http.MethodGet:
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

	case http.MethodPost:
		if !s.cfg.PaymentEnabled {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "充值功能未启用"})
			return
		}
		var req struct {
			AmountCents   int64  `json:"amount_cents"`
			PaymentMethod string `json:"payment_method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		if req.AmountCents <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "充值金额必须大于0"})
			return
		}
		if req.AmountCents < s.cfg.PaymentMinRechargeCents {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("最低充值金额 %d 分", s.cfg.PaymentMinRechargeCents)})
			return
		}
		if !s.payProvider.SupportsMethod(req.PaymentMethod) {
			req.PaymentMethod = "alipay" // default
		}

		id := uuid.NewString()
		outTradeNo := strings.ReplaceAll(uuid.NewString(), "-", "")
		payResult, err := s.payProvider.CreateRecharge(ctx, outTradeNo, userID, req.AmountCents, req.PaymentMethod, "余额充值")
		if err != nil {
			writeInternalError(w, "create payment", err)
			return
		}
		recharge := &store.BalanceRecharge{
			ID:              id,
			UserID:          userID,
			OutTradeNo:      outTradeNo,
			AmountCents:     req.AmountCents,
			Currency:        "CNY",
			PaymentMethod:   req.PaymentMethod,
			PaymentProvider: s.payProvider.Name(),
			PaymentURL:      payResult.PayURL,
			QRCode:          payResult.QRCode,
			ExpiresAt:       time.Now().Add(30 * time.Minute),
			Status:          "pending",
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		if err := s.store.CreateBalanceRecharge(ctx, recharge); err != nil {
			writeInternalError(w, "create recharge", err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"recharge":  recharge,
			"pay_url":   payResult.PayURL,
			"qr_code":   payResult.QRCode,
			"form_html": payResult.FormHTML,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
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
	if err := s.store.AdminUpdateBalanceRecharge(ctx, id, strings.TrimSpace(req.Status), strings.TrimSpace(req.TradeNo), "", paidAt); err != nil {
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

// handlePaymentNotify receives async payment provider callbacks and confirms recharges.
func (s *Servers) handlePaymentNotify(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	cb, err := s.payProvider.VerifyCallback(r)
	if err != nil {
		log.Warn().Err(err).Str("provider", s.payProvider.Name()).Msg("payment callback verify failed")
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "sign verify failed"})
		return
	}

	recharge, err := s.store.GetBalanceRechargeByOutTradeNo(ctx, cb.OutTradeNo)
	if err != nil {
		writeInternalError(w, "get recharge by trade no", err)
		return
	}
	if recharge == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "recharge not found"})
		return
	}
	if cb.AmountCents > 0 && cb.AmountCents != recharge.AmountCents {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "支付金额不匹配"})
		return
	}

	if cb.Status == "paid" {
		if err := s.store.AdminUpdateBalanceRecharge(ctx, recharge.ID, "paid", cb.TradeNo, cb.RawBody, cb.PaidAt); err != nil {
			writeInternalError(w, "update recharge paid", err)
			return
		}
	} else {
		_ = s.store.AdminUpdateBalanceRecharge(ctx, recharge.ID, cb.Status, cb.TradeNo, cb.RawBody, cb.PaidAt)
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Servers) handlePaymentMock(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if s.payProvider.Name() != "mock" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
		return
	}
	outTradeNo := strings.TrimPrefix(r.URL.Path, "/api/payments/mock/")
	outTradeNo = strings.Trim(strings.TrimSpace(outTradeNo), "/")
	if outTradeNo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少充值单号"})
		return
	}
	recharge, err := s.store.GetBalanceRechargeByOutTradeNo(ctx, outTradeNo)
	if err != nil {
		writeInternalError(w, "get mock recharge", err)
		return
	}
	if recharge == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "充值单不存在"})
		return
	}
	if r.Method == http.MethodPost || strings.TrimSpace(r.URL.Query().Get("confirm")) == "1" {
		if err := s.store.AdminUpdateBalanceRecharge(ctx, recharge.ID, "paid", "mock-"+outTradeNo, "mock", time.Now()); err != nil {
			writeInternalError(w, "confirm mock recharge", err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>支付成功</title></head><body><h2>支付成功，余额已到账</h2><p>请返回控制台刷新余额。</p></body></html>"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(fmt.Sprintf("<!doctype html><html><head><meta charset=\"utf-8\"><title>模拟支付</title></head><body><h2>模拟支付</h2><p>订单号：%s</p><p>金额：%.2f CNY</p><form method=\"post\"><button type=\"submit\">确认支付</button></form></body></html>", recharge.OutTradeNo, float64(recharge.AmountCents)/100)))
}
