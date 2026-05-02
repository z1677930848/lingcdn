package server

// License HTTP handlers: admin-only status/activate/import endpoints.
// resolvePublicIP lives here because it is only invoked via the license
// portal activation flow (the IP is bundled into the activation request for
// server-side geo/ownership checks).

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// License APIs. Method dispatch and store-timeout are applied by middleware
// at registration (see /api/license/* wiring). Handlers below see a context
// that already has the store timeout attached via withStoreTimeout where
// relevant.
func (s *Servers) handleLicenseStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodes, _ := s.store.ListNodes(ctx)
	st := s.ensureLicenseStatus()
	if st.MaxNodes > 0 && len(nodes) > st.MaxNodes {
		if st.Status == "active" {
			st.Status = "limited"
		}
		st.Reason = fmt.Sprintf("node limit exceeded (%d/%d)", len(nodes), st.MaxNodes)
	}
	licenseIP := strings.TrimSpace(s.cfg.PublicIP)
	if licenseIP == "" {
		licenseIP = s.resolvePublicIP()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"license":      st,
		"active_nodes": len(nodes),
		"mode":         s.licenseMode(),
		"now":          time.Now(),
		"control_id":   strings.TrimSpace(s.cfg.ControlID),
		"license_ip":   licenseIP,
	})
}

// resolvePublicIP detects the server's public IP via external services when PUBLIC_IP is not configured.
func (s *Servers) resolvePublicIP() string {
	s.licenseMu.RLock()
	cached := s.cachedPublicIP
	ts := s.cachedPublicIPAt
	s.licenseMu.RUnlock()
	if cached != "" && time.Since(ts) < 10*time.Minute {
		return cached
	}

	endpoints := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, ep := range endpoints {
		resp, err := client.Get(ep)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		ip := strings.TrimSpace(string(body))
		if ip != "" && net.ParseIP(ip) != nil {
			s.licenseMu.Lock()
			s.cachedPublicIP = ip
			s.cachedPublicIPAt = time.Now()
			s.licenseMu.Unlock()
			return ip
		}
	}
	return ""
}

func (s *Servers) handleLicenseActivate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LicenseKey string `json:"license_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.LicenseKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON或缺少license_key"})
		return
	}
	key := strings.TrimSpace(req.LicenseKey)
	statusCode, verifyResp, err := s.requestPortalLicenseVerify(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "官方授权验证失败: " + err.Error()})
		return
	}
	if !verifyResp.OK || statusCode != http.StatusOK {
		reason := strings.TrimSpace(verifyResp.Error)
		if reason == "" {
			reason = fmt.Sprintf("官方授权验证失败: 状态码 %d", statusCode)
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": reason})
		return
	}
	st := s.currentLicenseStatus()
	st.LicenseKey = key
	st, err = s.buildVerifiedLicenseState(st, verifyResp)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	st.LastChecked = time.Now()
	st.UpdatedAt = time.Now()
	st.GraceUntil = time.Time{}
	s.setLicenseState(st)
	writeJSON(w, http.StatusOK, map[string]any{"license": st})
}

func (s *Servers) handleLicenseImport(w http.ResponseWriter, r *http.Request) {
	_ = r
	writeJSON(w, http.StatusForbidden, map[string]any{"error": "手动导入授权已禁用，请使用 auth.lingcdn.cloud"})
}
