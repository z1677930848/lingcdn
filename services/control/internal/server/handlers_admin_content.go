package server

// Admin content management: announcements (list/create + item PATCH/DELETE)
// and orders (admin list/create/get/PATCH/DELETE + a user-facing read-only
// list of their own orders enriched with product details). These are kept in
// one file because their status normalizers are shared and the UI surfaces
// them together under "Operations".

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

// orderCreateLocks serialises concurrent /api/user/orders POSTs for the
// same (userID, productID). Without this lock two overlapping clicks
// race through the "latestUserProductOrderEnd" read → balance deduction →
// CreateOrder path and produce two orders with identical startsAt=now,
// endsAt=now+1period — i.e. the user is billed twice for a single period
// instead of having their subscription extended. Single-process mutex only.
var orderCreateLocks sync.Map // key: "userID|productID", value: *sync.Mutex

func orderCreateLock(userID, productID string) *sync.Mutex {
	key := strings.TrimSpace(userID) + "|" + strings.TrimSpace(productID)
	if mu, ok := orderCreateLocks.Load(key); ok {
		return mu.(*sync.Mutex)
	}
	mu := &sync.Mutex{}
	existing, _ := orderCreateLocks.LoadOrStore(key, mu)
	return existing.(*sync.Mutex)
}

// normalizeAnnouncementStatus returns the canonical value for a status query
// parameter plus an ok bit. "" (no filter) and "all" both normalize to "",
// which the store interprets as "don't filter by status".
func normalizeAnnouncementStatus(status string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "published":
		return "published", true
	case "draft":
		return "draft", true
	case "", "all":
		return "", true
	default:
		return "", false
	}
}

// normalizeOrderStatus returns the canonical order status, accepting both
// "cancelled" and "canceled" for UI compatibility.
func normalizeOrderStatus(status string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return "pending", true
	case "paid":
		return "paid", true
	case "expired":
		return "expired", true
	case "cancelled", "canceled":
		return "cancelled", true
	case "", "all":
		return "", true
	default:
		return "", false
	}
}

func (s *Servers) handleAdminAnnouncements(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, ok := normalizeAnnouncementStatus(r.URL.Query().Get("status"))
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		page := parseIntQuery(r, "page", 1)
		pageSize := parseIntQuery(r, "pageSize", 10)
		items, total, err := s.store.ListAnnouncements(ctx, status, q, page, pageSize)
		if err != nil {
			writeInternalError(w, "list announcements", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"announcements": items, "total": total})
	case http.MethodPost:
		var req struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Status  string `json:"status"`
			Pinned  bool   `json:"pinned"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "标题不能为空"})
			return
		}
		status, ok := normalizeAnnouncementStatus(req.Status)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
			return
		}
		if status == "" {
			status = "draft"
		}
		a := &store.Announcement{
			Title:   req.Title,
			Content: strings.TrimSpace(req.Content),
			Status:  status,
			Pinned:  req.Pinned,
		}
		if err := s.store.CreateAnnouncement(ctx, a); err != nil {
			writeInternalError(w, "create announcement", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"announcement": a})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleAdminAnnouncementByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/announcements/")
	id = strings.TrimSpace(strings.Trim(id, "/"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Title   *string `json:"title"`
			Content *string `json:"content"`
			Status  *string `json:"status"`
			Pinned  *bool   `json:"pinned"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		existing, err := s.store.GetAnnouncement(ctx, id)
		if err != nil {
			writeInternalError(w, "get announcement", err)
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "公告不存在"})
			return
		}
		// Skip the DB round-trip if nothing actually changed.
		changed := false
		if req.Title != nil {
			title := strings.TrimSpace(*req.Title)
			if title == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "标题不能为空"})
				return
			}
			if title != existing.Title {
				existing.Title = title
				changed = true
			}
		}
		if req.Content != nil && *req.Content != existing.Content {
			existing.Content = strings.TrimSpace(*req.Content)
			changed = true
		}
		if req.Status != nil {
			status, ok := normalizeAnnouncementStatus(*req.Status)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
				return
			}
			if status == "" {
				status = "draft"
			}
			if status != existing.Status {
				existing.Status = status
				changed = true
			}
		}
		if req.Pinned != nil && *req.Pinned != existing.Pinned {
			existing.Pinned = *req.Pinned
			changed = true
		}
		if !changed {
			writeJSON(w, http.StatusOK, map[string]any{"announcement": existing})
			return
		}
		if err := s.store.UpdateAnnouncement(ctx, existing); err != nil {
			writeInternalError(w, "update announcement", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"announcement": existing})
	case http.MethodDelete:
		if err := s.store.DeleteAnnouncement(ctx, id); err != nil {
			writeInternalError(w, "delete announcement", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// handleAdminOrders lists orders (optionally filtered by user/status) and
// creates new orders for users. Pricing defaults to the product's period
// price when amount_cents is omitted; amount is multiplied by quantity.
func (s *Servers) handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
		status, ok := normalizeOrderStatus(r.URL.Query().Get("status"))
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
			return
		}
		items, err := s.store.ListOrders(ctx, userID)
		if err != nil {
			writeInternalError(w, "list orders", err)
			return
		}
		if status != "" {
			filtered := make([]*store.Order, 0, len(items))
			for _, o := range items {
				if o != nil && strings.EqualFold(strings.TrimSpace(o.Status), status) {
					filtered = append(filtered, o)
				}
			}
			items = filtered
		}

		// Pagination is applied in-memory because the underlying ListOrders
		// returns the full slice. If performance matters, push this into the
		// store.
		total := len(items)
		page := parseIntQuery(r, "page", 1)
		pageSize := parseIntQuery(r, "page_size", parseIntQuery(r, "pageSize", 10))
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 10
		}
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"orders":    items[start:end],
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
		return
	case http.MethodPost:
		var req struct {
			UserID      string  `json:"user_id"`
			ProductID   string  `json:"product_id"`
			Period      string  `json:"period"`
			Quantity    int32   `json:"quantity"`
			Status      string  `json:"status"`
			Note        string  `json:"note"`
			AmountCents *int64  `json:"amount_cents"`
			Currency    string  `json:"currency"`
			StartsAt    *string `json:"starts_at"`
			EndsAt      *string `json:"ends_at"`
			PaidAt      *string `json:"paid_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.UserID = strings.TrimSpace(req.UserID)
		req.ProductID = strings.TrimSpace(req.ProductID)
		req.Period = strings.ToLower(strings.TrimSpace(req.Period))
		req.Status = strings.ToLower(strings.TrimSpace(req.Status))
		req.Note = strings.TrimSpace(req.Note)
		req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
		if req.UserID == "" || req.ProductID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "用户ID和产品ID不能为空"})
			return
		}
		if req.Period == "" {
			req.Period = "month"
		}
		if req.Quantity <= 0 {
			req.Quantity = 1
		}
		if req.Status == "" {
			req.Status = "paid"
		}
		if _, ok := normalizeOrderStatus(req.Status); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
			return
		}

		orderMu := orderCreateLock(req.UserID, req.ProductID)
		orderMu.Lock()
		defer orderMu.Unlock()

		p, err := s.store.GetProduct(ctx, req.ProductID)
		if err != nil {
			writeInternalError(w, "get product", err)
			return
		}
		if p == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "产品不存在"})
			return
		}
		amount := int64(0)
		if req.AmountCents != nil {
			amount = *req.AmountCents
		} else {
			switch req.Period {
			case "month":
				amount = p.PriceMonthCents
				if amount == 0 {
					amount = p.PriceCents
				}
			case "quarter":
				amount = p.PriceQuarterCents
			case "year":
				amount = p.PriceYearCents
			default:
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的周期"})
				return
			}
			amount = amount * int64(req.Quantity)
		}
		if amount < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的金额"})
			return
		}
		currency := req.Currency
		if currency == "" {
			currency = p.Currency
		}
		if currency == "" {
			currency = "CNY"
		}

		parseTime := func(s *string) (*time.Time, error) {
			if s == nil {
				return nil, nil
			}
			v := strings.TrimSpace(*s)
			if v == "" {
				return nil, nil
			}
			tm, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, err
			}
			return &tm, nil
		}
		startsAt, err := parseTime(req.StartsAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的开始时间"})
			return
		}
		endsAt, err := parseTime(req.EndsAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的结束时间"})
			return
		}
		paidAt, err := parseTime(req.PaidAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的支付时间"})
			return
		}
		if paidAt == nil && req.Status == "paid" {
			now := time.Now().UTC()
			paidAt = &now
		}
		now := time.Now().UTC()

		// When admin omits explicit dates for a paid order, derive the same
		// renewal window + endsAt stacking rules as user self-service checkout.
		if req.Status == "paid" && startsAt == nil && endsAt == nil {
			var renewalBase time.Time
			latestEnd, err := s.latestUserProductOrderEnd(ctx, req.UserID, p.ID)
			if err != nil {
				writeInternalError(w, "list user orders", err)
				return
			}
			if latestEnd != nil {
				settings := s.resolveSettings(ctx)
				windowDays := normalizeRenewalBeforeExpiryDays(settings.RenewalBeforeExpiryDays)
				latestEndUTC := latestEnd.UTC()
				if latestEndUTC.After(now.AddDate(0, 0, windowDays)) {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error": fmt.Sprintf("当前套餐尚未进入续费期，仅允许到期前 %d 天内续费（到期时间：%s）", windowDays, latestEndUTC.Format("2006-01-02 15:04")),
					})
					return
				}
				if latestEndUTC.After(now) {
					renewalBase = latestEndUTC
				}
			}
			startsAtVal := now
			startsAt = &startsAtVal
			if renewalBase.IsZero() {
				renewalBase = startsAtVal
			}
			var endsAtVal time.Time
			switch req.Period {
			case "month":
				endsAtVal = renewalBase.AddDate(0, 1*int(req.Quantity), 0)
			case "quarter":
				endsAtVal = renewalBase.AddDate(0, 3*int(req.Quantity), 0)
			case "year":
				endsAtVal = renewalBase.AddDate(1*int(req.Quantity), 0, 0)
			default:
				endsAtVal = renewalBase.AddDate(0, 1, 0)
			}
			endsAt = &endsAtVal
		}

		// Admin-created orders share the same UTC-only convention as
		// self-service orders (see handleUserOrderCreate). Without
		// `.UTC()` here CreatedAt/UpdatedAt ended up in local time
		// while PaidAt was UTC — which produced audit logs that looked
		// like the order was "paid before it existed" on hosts with
		// non-UTC zones.
		o := &store.Order{
			ID:          uuid.NewString(),
			UserID:      req.UserID,
			ProductID:   p.ID,
			ProductName: p.Name,
			AmountCents: amount,
			Currency:    currency,
			Status:      req.Status,
			Period:      req.Period,
			Quantity:    req.Quantity,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
			PaidAt:      paidAt,
			Note:        req.Note,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.store.CreateOrder(ctx, o); err != nil {
			writeInternalError(w, "create order", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"order": o})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
}

// handleAdminOrderByID handles GET/PATCH/DELETE for a single order. PATCH
// uses a raw map so we can distinguish "field omitted" from "field: null"
// for optional time fields.
func (s *Servers) handleAdminOrderByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/orders/")
	id = strings.TrimSpace(strings.Trim(id, "/"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		o, err := s.store.GetOrder(ctx, id)
		if err != nil {
			writeInternalError(w, "get order", err)
			return
		}
		if o == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"order": o})
		return
	case http.MethodPatch:
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		existing, err := s.store.GetOrder(ctx, id)
		if err != nil {
			writeInternalError(w, "get order", err)
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
			return
		}
		if raw, ok := payload["status"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
					return
				}
				v = strings.ToLower(strings.TrimSpace(v))
				if _, ok := normalizeOrderStatus(v); !ok || v == "" {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
					return
				}
				existing.Status = v
				if v == "paid" && existing.PaidAt == nil {
					now := time.Now().UTC()
					existing.PaidAt = &now
				}
			}
		}
		if raw, ok := payload["note"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.Note = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的备注"})
					return
				}
				existing.Note = strings.TrimSpace(v)
			}
		}
		if raw, ok := payload["amount_cents"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v int64
				if err := json.Unmarshal(trimmed, &v); err != nil || v < 0 {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的金额"})
					return
				}
				existing.AmountCents = v
			}
		}
		if raw, ok := payload["period"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的周期"})
					return
				}
				v = strings.ToLower(strings.TrimSpace(v))
				if v != "month" && v != "quarter" && v != "year" {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的周期"})
					return
				}
				existing.Period = v
			}
		}
		if raw, ok := payload["quantity"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v int32
				if err := json.Unmarshal(trimmed, &v); err != nil || v <= 0 {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的数量"})
					return
				}
				existing.Quantity = v
			}
		}
		// parseTimeField returns (value, present, err). "present" is true whenever
		// the key appears in the payload (including null or empty string), so
		// callers can clear the field explicitly.
		parseTimeField := func(key string) (*time.Time, bool, error) {
			raw, ok := payload[key]
			if !ok {
				return nil, false, nil
			}
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				return nil, true, nil
			}
			var v string
			if err := json.Unmarshal(trimmed, &v); err != nil {
				return nil, true, err
			}
			v = strings.TrimSpace(v)
			if v == "" {
				return nil, true, nil
			}
			tm, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, true, err
			}
			return &tm, true, nil
		}
		if v, present, err := parseTimeField("starts_at"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的开始时间"})
			return
		} else if present {
			existing.StartsAt = v
		}
		if v, present, err := parseTimeField("ends_at"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的结束时间"})
			return
		} else if present {
			existing.EndsAt = v
		}
		if v, present, err := parseTimeField("paid_at"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的支付时间"})
			return
		} else if present {
			existing.PaidAt = v
		}
		// Keep UpdatedAt aligned with the UTC convention used by every
		// other write path on this table; mixing zones would show up as
		// bogus clock skew in the order audit trail.
		existing.UpdatedAt = time.Now().UTC()
		if err := s.store.UpdateOrder(ctx, existing); err != nil {
			writeInternalError(w, "update order", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"order": existing})
		return
	case http.MethodDelete:
		if err := s.store.DeleteOrder(ctx, id); err != nil {
			writeInternalError(w, "delete order", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
}

// handleUserOrders returns orders for the currently authenticated user,
// enriched with product details and the user's aggregate domain counts.
// POST creates a new order (user self-purchase), deducting balance.
func (s *Servers) handleUserOrders(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}

	if r.Method == http.MethodPost {
		s.handleUserOrderCreate(w, r, ctx, userID)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	items, err := s.store.ListOrders(ctx, userID)
	if err != nil {
		writeInternalError(w, "list user orders", err)
		return
	}
	// Enrich each order with product-level fields the UI needs to render
	// quotas and capabilities without a second round-trip.
	type orderView struct {
		*store.Order
		LineGroupID         string `json:"line_group_id,omitempty"`
		DomainLimit         int32  `json:"domain_limit,omitempty"`
		PrimaryDomainLimit  int32  `json:"primary_domain_limit,omitempty"`
		DomainCount         int    `json:"domain_count,omitempty"`
		PrimaryDomainCount  int    `json:"primary_domain_count,omitempty"`
		Websocket           bool   `json:"websocket"`
		CustomCCRules       bool   `json:"custom_cc_rules"`
		MonthlyTrafficBytes *int64 `json:"monthly_traffic_bytes,omitempty"`
		BandwidthBps        *int64 `json:"bandwidth_bps,omitempty"`
		ConnLimit           *int64 `json:"conn_limit,omitempty"`
	}
	var views []orderView
	for _, o := range items {
		if o == nil {
			continue
		}
		v := orderView{Order: o}
		p, err := s.store.GetProduct(ctx, o.ProductID)
		if err == nil && p != nil {
			v.LineGroupID = p.LineGroupID
			if v.LineGroupID == "" {
				v.LineGroupID = p.ClusterID
			}
			if p.DomainLimit != nil {
				v.DomainLimit = *p.DomainLimit
			}
			if p.PrimaryDomainLimit != nil {
				v.PrimaryDomainLimit = *p.PrimaryDomainLimit
			}
			v.Websocket = p.Websocket
			v.CustomCCRules = p.CustomCCRules
			v.MonthlyTrafficBytes = p.MonthlyTrafficBytes
			v.BandwidthBps = p.BandwidthBps
			v.ConnLimit = p.ConnLimit
		}
		views = append(views, v)
	}
	// Count user's domains for the quota badge. Kept here rather than in
	// ListOrders because ListOrders is reused by the admin path which doesn't
	// need the per-request domain enumeration.
	userDomains, _ := s.store.ListDomainsByUser(ctx, userID)
	domainCount := len(userDomains)
	primaryDomainCount := 0
	for _, ud := range userDomains {
		if isPrimaryDomain(ud.Name) {
			primaryDomainCount++
		}
	}
	for i := range views {
		views[i].DomainCount = domainCount
		views[i].PrimaryDomainCount = primaryDomainCount
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": views, "total": len(views)})
}

// handleUserOrderCreate processes a user self-purchase: resolve product,
// calculate price for the chosen period, deduct balance (0-cost orders skip
// the deduction), create a "paid" order with auto-calculated dates.
func (s *Servers) handleUserOrderCreate(w http.ResponseWriter, r *http.Request, ctx context.Context, userID string) {
	if !s.requireUserPermission(w, ctx, PermOrdersPurchase) {
		return
	}
	var req struct {
		ProductID string `json:"product_id"`
		Period    string `json:"period"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}
	req.ProductID = strings.TrimSpace(req.ProductID)
	req.Period = strings.ToLower(strings.TrimSpace(req.Period))
	if req.ProductID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "产品ID不能为空"})
		return
	}
	if req.Period == "" {
		req.Period = "month"
	}

	// Take the per (user, product) lock BEFORE reading existing orders and balance.
	orderMu := orderCreateLock(userID, req.ProductID)
	orderMu.Lock()
	defer orderMu.Unlock()
	_ = s.finishUserOrderCreate(w, ctx, userID, req.ProductID, req.Period)
}

func (s *Servers) finishUserOrderCreate(w http.ResponseWriter, ctx context.Context, userID, productID, period string) error {
	p, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		writeInternalError(w, "get product", err)
		return err
	}
	if p == nil || !p.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "产品不存在或已下架"})
		return nil
	}

	var amount int64
	switch period {
	case "month":
		amount = p.PriceMonthCents
		if amount == 0 {
			amount = p.PriceCents
		}
	case "quarter":
		amount = p.PriceQuarterCents
	case "year":
		amount = p.PriceYearCents
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的周期，必须为 month/quarter/year"})
		return nil
	}
	if amount < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "产品价格无效"})
		return nil
	}

	now := time.Now().UTC()
	var renewalBase time.Time
	latestEnd, err := s.latestUserProductOrderEnd(ctx, userID, p.ID)
	if err != nil {
		writeInternalError(w, "list user orders", err)
		return err
	}
	if latestEnd != nil {
		settings := s.resolveSettings(ctx)
		windowDays := normalizeRenewalBeforeExpiryDays(settings.RenewalBeforeExpiryDays)
		latestEndUTC := latestEnd.UTC()
		if latestEndUTC.After(now.AddDate(0, 0, windowDays)) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": fmt.Sprintf("当前套餐尚未进入续费期，仅允许到期前 %d 天内续费（到期时间：%s）", windowDays, latestEndUTC.Format("2006-01-02 15:04")),
			})
			return nil
		}
		if latestEndUTC.After(now) {
			renewalBase = latestEndUTC
		}
	}

	balanceDeducted := int64(0)
	if amount > 0 {
		if err := s.store.AdminAdjustBalance(ctx, userID, -amount, "购买产品: "+p.Name+" ("+period+")"); err != nil {
			if strings.Contains(err.Error(), "insufficient") {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "余额不足，请先充值"})
				return nil
			}
			writeInternalError(w, "deduct balance", err)
			return err
		}
		balanceDeducted = amount
	}

	paidAt := now
	startsAt := now
	if renewalBase.IsZero() {
		renewalBase = startsAt
	}
	var endsAt time.Time
	switch period {
	case "month":
		endsAt = renewalBase.AddDate(0, 1, 0)
	case "quarter":
		endsAt = renewalBase.AddDate(0, 3, 0)
	case "year":
		endsAt = renewalBase.AddDate(1, 0, 0)
	}

	currency := p.Currency
	if currency == "" {
		currency = "CNY"
	}

	o := &store.Order{
		ID:          uuid.NewString(),
		UserID:      userID,
		ProductID:   p.ID,
		ProductName: p.Name,
		AmountCents: amount,
		Currency:    currency,
		Status:      "paid",
		Period:      period,
		Quantity:    1,
		StartsAt:    &startsAt,
		EndsAt:      &endsAt,
		PaidAt:      &paidAt,
		Note:        "用户自助购买",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.CreateOrder(ctx, o); err != nil {
		if balanceDeducted > 0 {
			_ = s.store.AdminAdjustBalance(ctx, userID, balanceDeducted, "购买失败退款: "+p.Name+" ("+period+")")
		}
		writeInternalError(w, "create order", err)
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"order": o})
	return nil
}

func normalizeRenewalBeforeExpiryDays(days int) int {
	if days < 0 {
		return 0
	}
	if days > 3650 {
		return 3650
	}
	return days
}

func (s *Servers) latestUserProductOrderEnd(ctx context.Context, userID, productID string) (*time.Time, error) {
	orders, err := s.store.ListOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	var latest *time.Time
	for _, o := range orders {
		if o == nil || o.Status != "paid" || o.ProductID != productID || o.EndsAt == nil {
			continue
		}
		if latest == nil || o.EndsAt.After(*latest) {
			cp := *o.EndsAt
			latest = &cp
		}
	}
	return latest, nil
}
