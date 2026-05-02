package server

// Admin-only product catalog: product groups (pricing tiers) and individual
// products. Products carry quotas (bandwidth/traffic/domain-count) and
// capability flags (websocket, custom_cc_rules, http3, l2_origin). GET
// /api/products is available to non-admins but returns only enabled
// products; all mutations are admin-only.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleProductGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	ctx2, cancel := store.WithTimeout(ctx)
	defer cancel()
	switch r.Method {
	case http.MethodGet:
		groups, err := s.store.ListProductGroups(ctx2)
		if err != nil {
			writeInternalError(w, "list product groups", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
		return
	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Sort        int    `json:"sort"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Description = strings.TrimSpace(req.Description)
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		if req.Sort <= 0 {
			req.Sort = 100
		}
		now := time.Now()
		g := &store.ProductGroup{
			ID:          uuid.NewString(),
			Name:        req.Name,
			Sort:        req.Sort,
			Description: req.Description,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.store.CreateProductGroup(ctx2, g); err != nil {
			writeInternalError(w, "create product group", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"group": g})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
}

func (s *Servers) handleProductGroupByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/product-groups/")
	id = strings.TrimSpace(strings.Trim(id, "/"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}
	ctx2, cancel := store.WithTimeout(ctx)
	defer cancel()
	switch r.Method {
	case http.MethodPatch:
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		existing, err := s.store.GetProductGroup(ctx2, id)
		if err != nil {
			writeInternalError(w, "get product group", err)
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
			return
		}
		if raw, ok := payload["name"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的名称"})
					return
				}
				v = strings.TrimSpace(v)
				if v != "" {
					existing.Name = v
				}
			}
		}
		if raw, ok := payload["description"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.Description = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的描述"})
					return
				}
				existing.Description = strings.TrimSpace(v)
			}
		}
		if raw, ok := payload["sort"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v int
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的排序值"})
					return
				}
				if v <= 0 {
					v = 100
				}
				existing.Sort = v
			}
		}
		existing.UpdatedAt = time.Now()
		if err := s.store.UpdateProductGroup(ctx2, existing); err != nil {
			writeInternalError(w, "update product group", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"group": existing})
		return
	case http.MethodDelete:
		if err := s.store.DeleteProductGroup(ctx2, id); err != nil {
			writeInternalError(w, "delete product group", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
}

func (s *Servers) handleProducts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		ctx2, cancel := store.WithTimeout(ctx)
		defer cancel()
		products, err := s.store.ListProducts(ctx2)
		if err != nil {
			writeInternalError(w, "list products", err)
			return
		}
		if role := getUserRole(ctx); role != "admin" {
			var filtered []*store.Product
			for _, p := range products {
				if p != nil && p.Enabled {
					filtered = append(filtered, p)
				}
			}
			products = filtered
		}
		writeJSON(w, http.StatusOK, map[string]any{"products": products})
		return
	case http.MethodPost:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		var req struct {
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Description string `json:"description"`
			GroupID     string `json:"group_id"`
			Sort        int    `json:"sort"`
			Region      string `json:"region"`
			LineGroupID string `json:"line_group_id"`
			// ClusterID is the name the Admin UI uses for the same concept;
			// accept it so the cluster dropdown actually persists even when
			// the frontend submits the `cluster_id` key instead of
			// `line_group_id`. One of the two is assigned to the product's
			// LineGroupID column below.
			ClusterID string `json:"cluster_id"`

			MonthlyTrafficBytes *int64 `json:"monthly_traffic_bytes"`
			BandwidthBps        *int64 `json:"bandwidth_bps"`
			ConnLimit           *int64 `json:"conn_limit"`

			DomainLimit        *int32 `json:"domain_limit"`
			PrimaryDomainLimit *int32 `json:"primary_domain_limit"`
			HTTPPortLimit      *int32 `json:"http_port_limit"`
			StreamPortLimit    *int32 `json:"stream_port_limit"`
			NonStdPortLimit    *int32 `json:"non_std_port_limit"`

			Websocket      *bool  `json:"websocket"`
			CustomCCRules  *bool  `json:"custom_cc_rules"`
			HTTP3          *bool  `json:"http3"`
			L2Origin       *bool  `json:"l2_origin"`
			CCProtection   string `json:"cc_protection"`
			DDoSProtection string `json:"ddos_protection"`

			PriceCents        *int64 `json:"price_cents"`
			PriceMonthCents   *int64 `json:"price_month_cents"`
			PriceQuarterCents *int64 `json:"price_quarter_cents"`
			PriceYearCents    *int64 `json:"price_year_cents"`
			Currency          string `json:"currency"`
			Enabled           *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
		req.Description = strings.TrimSpace(req.Description)
		req.GroupID = strings.TrimSpace(req.GroupID)
		req.Region = strings.TrimSpace(req.Region)
		req.LineGroupID = strings.TrimSpace(req.LineGroupID)
		req.ClusterID = strings.TrimSpace(req.ClusterID)
		// Merge: explicit line_group_id wins, cluster_id is used only when
		// the explicit field is missing (how the current Admin UI submits).
		if req.LineGroupID == "" {
			req.LineGroupID = req.ClusterID
		}
		req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		if req.Currency == "" {
			req.Currency = "CNY"
		}
		if req.Sort <= 0 {
			req.Sort = 100
		}
		if req.MonthlyTrafficBytes != nil && *req.MonthlyTrafficBytes < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的月流量值"})
			return
		}
		if req.BandwidthBps != nil && *req.BandwidthBps < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的带宽值"})
			return
		}
		if req.ConnLimit != nil && *req.ConnLimit < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的连接限制"})
			return
		}

		priceMonth := int64(0)
		if req.PriceMonthCents != nil {
			priceMonth = *req.PriceMonthCents
		} else if req.PriceCents != nil {
			priceMonth = *req.PriceCents
		}
		if priceMonth < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的月价格"})
			return
		}
		priceQuarter := int64(0)
		if req.PriceQuarterCents != nil {
			priceQuarter = *req.PriceQuarterCents
		}
		if priceQuarter < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的季度价格"})
			return
		}
		priceYear := int64(0)
		if req.PriceYearCents != nil {
			priceYear = *req.PriceYearCents
		}
		if priceYear < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的年价格"})
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		websocket := true
		if req.Websocket != nil {
			websocket = *req.Websocket
		}
		customCC := true
		if req.CustomCCRules != nil {
			customCC = *req.CustomCCRules
		}
		http3 := false
		if req.HTTP3 != nil {
			http3 = *req.HTTP3
		}
		l2Origin := false
		if req.L2Origin != nil {
			l2Origin = *req.L2Origin
		}

		now := time.Now()
		p := &store.Product{
			ID:          uuid.NewString(),
			Name:        req.Name,
			Slug:        req.Slug,
			Description: req.Description,
			GroupID:     req.GroupID,
			Sort:        req.Sort,
			Region:      req.Region,
			LineGroupID: req.LineGroupID,

			MonthlyTrafficBytes: req.MonthlyTrafficBytes,
			BandwidthBps:        req.BandwidthBps,
			ConnLimit:           req.ConnLimit,
			DomainLimit:         req.DomainLimit,
			PrimaryDomainLimit:  req.PrimaryDomainLimit,
			HTTPPortLimit:       req.HTTPPortLimit,
			StreamPortLimit:     req.StreamPortLimit,
			NonStdPortLimit:     req.NonStdPortLimit,
			Websocket:           websocket,
			CustomCCRules:       customCC,
			HTTP3:               http3,
			L2Origin:            l2Origin,
			CCProtection:        strings.TrimSpace(req.CCProtection),
			DDoSProtection:      strings.TrimSpace(req.DDoSProtection),

			PriceCents:        priceMonth,
			PriceMonthCents:   priceMonth,
			PriceQuarterCents: priceQuarter,
			PriceYearCents:    priceYear,
			Currency:          req.Currency,
			Enabled:           enabled,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		ctx2, cancel := store.WithTimeout(ctx)
		defer cancel()
		if err := s.store.CreateProduct(ctx2, p); err != nil {
			writeInternalError(w, "create product", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"product": p})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
}

func (s *Servers) handleProductByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/products/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		ctx2, cancel := store.WithTimeout(ctx)
		defer cancel()
		p, err := s.store.GetProduct(ctx2, id)
		if err != nil {
			writeInternalError(w, "get product", err)
			return
		}
		if p == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
			return
		}
		if role := getUserRole(ctx); role != "admin" && !p.Enabled {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"product": p})
		return
	case http.MethodPatch:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		ctx2, cancel := store.WithTimeout(ctx)
		defer cancel()
		existing, err := s.store.GetProduct(ctx2, id)
		if err != nil {
			writeInternalError(w, "get product", err)
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
			return
		}
		if raw, ok := payload["name"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的名称"})
					return
				}
				v = strings.TrimSpace(v)
				if v != "" {
					existing.Name = v
				}
			}
		}
		if raw, ok := payload["slug"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的别名"})
					return
				}
				v = strings.ToLower(strings.TrimSpace(v))
				if v != "" {
					existing.Slug = v
				}
			}
		}
		if raw, ok := payload["description"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.Description = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的描述"})
					return
				}
				existing.Description = strings.TrimSpace(v)
			}
		}
		if raw, ok := payload["group_id"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.GroupID = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的分组ID"})
					return
				}
				existing.GroupID = strings.TrimSpace(v)
			}
		}
		if raw, ok := payload["sort"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v int
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的排序值"})
					return
				}
				if v <= 0 {
					v = 100
				}
				existing.Sort = v
			}
		}
		if raw, ok := payload["region"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.Region = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的区域"})
					return
				}
				existing.Region = strings.TrimSpace(v)
			}
		}
		// Accept either `line_group_id` or `cluster_id` — the Admin UI submits
		// the latter when the user picks a cluster from the dropdown, while
		// older tooling and the API contract use the former. Explicit
		// `line_group_id` wins if both are present in the same patch.
		if raw, ok := payload["line_group_id"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.LineGroupID = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的线路组ID"})
					return
				}
				existing.LineGroupID = strings.TrimSpace(v)
			}
		} else if raw, ok := payload["cluster_id"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.LineGroupID = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的集群ID"})
					return
				}
				existing.LineGroupID = strings.TrimSpace(v)
			}
		}

		if present, v, err := parseOptionalInt64Field(payload, "monthly_traffic_bytes"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的月流量值"})
			return
		} else if present {
			if v != nil && *v < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的月流量值"})
				return
			}
			existing.MonthlyTrafficBytes = v
		}
		if present, v, err := parseOptionalInt64Field(payload, "bandwidth_bps"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的带宽值"})
			return
		} else if present {
			if v != nil && *v < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的带宽值"})
				return
			}
			existing.BandwidthBps = v
		}
		if present, v, err := parseOptionalInt64Field(payload, "conn_limit"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的连接限制"})
			return
		} else if present {
			if v != nil && *v < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的连接限制"})
				return
			}
			existing.ConnLimit = v
		}
		if present, v, err := parseOptionalInt32Field(payload, "domain_limit"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的域名限制"})
			return
		} else if present {
			existing.DomainLimit = v
		}
		if present, v, err := parseOptionalInt32Field(payload, "primary_domain_limit"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的主域名限制"})
			return
		} else if present {
			existing.PrimaryDomainLimit = v
		}
		if present, v, err := parseOptionalInt32Field(payload, "http_port_limit"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的HTTP端口限制"})
			return
		} else if present {
			existing.HTTPPortLimit = v
		}
		if present, v, err := parseOptionalInt32Field(payload, "stream_port_limit"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的流端口限制"})
			return
		} else if present {
			existing.StreamPortLimit = v
		}
		if present, v, err := parseOptionalInt32Field(payload, "non_std_port_limit"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的非标端口限制"})
			return
		} else if present {
			existing.NonStdPortLimit = v
		}

		if ok, v, err := parseOptionalBoolField(payload, "websocket"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的WebSocket值"})
			return
		} else if ok {
			existing.Websocket = v
		}
		if ok, v, err := parseOptionalBoolField(payload, "custom_cc_rules"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的自定义CC规则值"})
			return
		} else if ok {
			existing.CustomCCRules = v
		}
		if ok, v, err := parseOptionalBoolField(payload, "http3"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的HTTP3值"})
			return
		} else if ok {
			existing.HTTP3 = v
		}
		if ok, v, err := parseOptionalBoolField(payload, "l2_origin"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的L2回源值"})
			return
		} else if ok {
			existing.L2Origin = v
		}
		if raw, ok := payload["cc_protection"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.CCProtection = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的CC防护值"})
					return
				}
				existing.CCProtection = strings.TrimSpace(v)
			}
		}
		if raw, ok := payload["ddos_protection"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
				existing.DDoSProtection = ""
			} else {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的DDoS防护值"})
					return
				}
				existing.DDoSProtection = strings.TrimSpace(v)
			}
		}

		if present, v, err := parseOptionalInt64Field(payload, "price_month_cents"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的月价格"})
			return
		} else if present {
			if v != nil && *v < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的月价格"})
				return
			}
			existing.PriceMonthCents = 0
			if v != nil {
				existing.PriceMonthCents = *v
			}
			existing.PriceCents = existing.PriceMonthCents
		}
		if present, v, err := parseOptionalInt64Field(payload, "price_quarter_cents"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的季度价格"})
			return
		} else if present {
			if v != nil && *v < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的季度价格"})
				return
			}
			existing.PriceQuarterCents = 0
			if v != nil {
				existing.PriceQuarterCents = *v
			}
		}
		if present, v, err := parseOptionalInt64Field(payload, "price_year_cents"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的年价格"})
			return
		} else if present {
			if v != nil && *v < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的年价格"})
				return
			}
			existing.PriceYearCents = 0
			if v != nil {
				existing.PriceYearCents = *v
			}
		}
		if present, v, err := parseOptionalInt64Field(payload, "price_cents"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的价格"})
			return
		} else if present {
			if v != nil && *v < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的价格"})
				return
			}
			if v != nil {
				existing.PriceCents = *v
				if existing.PriceMonthCents == 0 {
					existing.PriceMonthCents = *v
				}
			}
		}

		if raw, ok := payload["currency"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
				var v string
				if err := json.Unmarshal(trimmed, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的货币类型"})
					return
				}
				v = strings.ToUpper(strings.TrimSpace(v))
				if v != "" {
					existing.Currency = v
				}
			}
		}
		if ok, v, err := parseOptionalBoolField(payload, "enabled"); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的启用状态"})
			return
		} else if ok {
			existing.Enabled = v
		}
		existing.UpdatedAt = time.Now()
		if err := s.store.UpdateProduct(ctx2, existing); err != nil {
			writeInternalError(w, "update product", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"product": existing})
		return
	case http.MethodDelete:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		ctx2, cancel := store.WithTimeout(ctx)
		defer cancel()
		if err := s.store.DeleteProduct(ctx2, id); err != nil {
			writeInternalError(w, "delete product", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
}
