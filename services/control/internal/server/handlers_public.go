package server

// Public (unauthenticated) settings, announcements, and license snapshot.
// Used by the login/landing UI before a JWT exists — only safe-to-expose
// branding and feature flags get surfaced. No PII, no admin tunables.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handlePublicSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		writeInternalError(w, "get settings", err)
		return
	}
	normalized := s.applySettingsDefaults(settings)
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": map[string]any{
			"system_name":                 normalized.SystemName,
			"footer_links":                normalized.FooterLinks,
			"footer_copyright":            normalized.FooterCopyright,
			"favicon":                     normalized.Favicon,
			"logo":                        normalized.Logo,
			"sidebar_brand_mode":          normalized.SidebarBrandMode,
			"register_enabled":            normalized.RegisterEnabled,
			"register_email_verification": normalized.RegisterEmailVerification,
			"renewal_before_expiry_days":  normalized.RenewalBeforeExpiryDays,
		},
	})
}

func (s *Servers) handlePublicAnnouncements(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "page_size", 0)
	if pageSize <= 0 {
		pageSize = parseIntQuery(r, "pageSize", 10)
	}
	items, total, err := s.store.ListAnnouncements(ctx, "published", q, page, pageSize)
	if err != nil {
		writeInternalError(w, "list announcements", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"announcements": items, "total": total})
}

func (s *Servers) handlePublicLicenseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	st := s.ensureLicenseStatus()
	if s.store != nil && st.MaxNodes > 0 {
		ctx, cancel := store.WithTimeout(r.Context())
		nodes, _ := s.store.ListNodes(ctx)
		cancel()
		if len(nodes) > st.MaxNodes && st.Status == "active" {
			st.Status = "limited"
			st.Reason = fmt.Sprintf("node limit exceeded (%d/%d)", len(nodes), st.MaxNodes)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": st.Status,
		"reason": st.Reason,
		"mode":   s.licenseMode(),
	})
}
