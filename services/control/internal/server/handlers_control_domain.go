package server

import (
	"net/http"
	"strings"
)

func (s *Servers) handleControlDomainVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" {
		domain = s.effectiveControlDomain()
	}
	result, err := s.verifyControlDomainDNS(domain)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Servers) handleControlDomainInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	writeJSON(w, http.StatusOK, s.controlIdentitySnapshot())
}
