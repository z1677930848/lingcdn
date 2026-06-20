package server

import (
	"net/http"
	"strings"
	"time"
)

func (s *Servers) revokeToken(jti string, expiresAt time.Time) {
	jti = strings.TrimSpace(jti)
	if s == nil || jti == "" {
		return
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(24 * time.Hour)
	}
	s.tokenRevokeMu.Lock()
	defer s.tokenRevokeMu.Unlock()
	if s.tokenRevoke == nil {
		s.tokenRevoke = make(map[string]time.Time)
	}
	s.tokenRevoke[jti] = expiresAt
	if len(s.tokenRevoke) > 2048 {
		now := time.Now()
		for id, exp := range s.tokenRevoke {
			if now.After(exp) {
				delete(s.tokenRevoke, id)
			}
		}
	}
}

func (s *Servers) isTokenRevoked(jti string) bool {
	jti = strings.TrimSpace(jti)
	if s == nil || jti == "" {
		return false
	}
	s.tokenRevokeMu.RLock()
	exp, ok := s.tokenRevoke[jti]
	s.tokenRevokeMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		s.tokenRevokeMu.Lock()
		delete(s.tokenRevoke, jti)
		s.tokenRevokeMu.Unlock()
		return false
	}
	return true
}

func (s *Servers) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	token := extractBearerToken(r)
	if token != "" {
		if claims, err := parseJWT(s.cfg.AuthSecret, token); err == nil && strings.TrimSpace(claims.ID) != "" {
			exp := time.Now().Add(24 * time.Hour)
			if claims.ExpiresAt != nil {
				exp = claims.ExpiresAt.Time
			}
			s.revokeToken(claims.ID, exp)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
