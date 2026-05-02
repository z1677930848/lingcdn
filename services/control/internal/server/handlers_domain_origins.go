package server

// Per-domain origin-address management.
//
// GET  /api/domains/{id}/origins       -> list rows
// PUT  /api/domains/{id}/origins       -> replace whole set (body: { origins: [...] })
//
// Ownership was already verified in handleDomainByID before it routed
// here, so this handler trusts the {id} and only validates payload
// shape. Every mutation triggers an auto-publish so edge nodes pick up
// the change without a separate "publish" click.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/lingcdn/control/internal/store"
)

type domainOriginInput struct {
	ID        string `json:"id,omitempty"`
	Address   string `json:"address"`
	Weight    int32  `json:"weight,omitempty"`
	Enabled   *bool  `json:"enabled,omitempty"`
	SortOrder int32  `json:"sort_order,omitempty"`
}

func (s *Servers) handleDomainOrigins(w http.ResponseWriter, r *http.Request, domainID string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListDomainOrigins(ctx, domainID)
		if err != nil {
			writeInternalError(w, "list domain origins", err)
			return
		}
		if list == nil {
			list = []*store.DomainOrigin{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"origins": list})

	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求内容格式错误"})
			return
		}
		var payload struct {
			Origins []domainOriginInput `json:"origins"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}

		entries := make([]*store.DomainOrigin, 0, len(payload.Origins))
		for i, in := range payload.Origins {
			addr := strings.TrimSpace(in.Address)
			if addr == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "地址不能为空"})
				return
			}
			weight := in.Weight
			if weight <= 0 {
				weight = 1
			}
			if weight > 100 {
				weight = 100
			}
			enabled := true
			if in.Enabled != nil {
				enabled = *in.Enabled
			}
			sortOrder := in.SortOrder
			if sortOrder == 0 {
				sortOrder = int32(i)
			}
			entries = append(entries, &store.DomainOrigin{
				ID:        strings.TrimSpace(in.ID),
				DomainID:  domainID,
				Address:   addr,
				Weight:    weight,
				Enabled:   enabled,
				SortOrder: sortOrder,
			})
		}

		if err := s.store.ReplaceDomainOrigins(ctx, domainID, entries); err != nil {
			writeInternalError(w, "replace domain origins", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "domain:origins:"+domainID, "", "", nil)

		// Return the canonical list so the UI can refresh form state.
		list, err := s.store.ListDomainOrigins(ctx, domainID)
		if err != nil {
			writeInternalError(w, "list domain origins", err)
			return
		}
		if list == nil {
			list = []*store.DomainOrigin{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"origins": list})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}
