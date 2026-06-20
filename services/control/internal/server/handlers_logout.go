package server

import (
	"net/http"
	"strings"
)

func extractBearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.Header.Get("X-Service-Token")
	}
	if token == "" {
		if c, err := r.Cookie("lingcdn_token"); err == nil {
			token = c.Value
		}
	}
	token = strings.TrimPrefix(token, "Bearer ")
	return strings.TrimSpace(token)
}
