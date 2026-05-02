package server

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// handleAdminSystemLogs lists system logs for admin.
func (s *Servers) handleAdminSystemLogs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	logType := strings.TrimSpace(r.URL.Query().Get("type"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "pageSize", 10)
	if pageSize == 0 {
		pageSize = parseIntQuery(r, "page_size", 10)
	}
	logs, total, err := s.store.ListSystemLogs(ctx, logType, status, q, page, pageSize)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "total": total})
}

// shouldAuditRequest returns true for admin non-read requests.
func shouldAuditRequest(ctx context.Context, r *http.Request) bool {
	if r == nil {
		return false
	}
	if getUserRole(ctx) != "admin" {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	return true
}

// buildAuditMessage builds a concise message from method and path.
func buildAuditMessage(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	msg := strings.ToUpper(r.Method) + " " + r.URL.Path
	if q := strings.TrimSpace(r.URL.RawQuery); q != "" {
		msg += "?" + q
	}
	return msg
}

// inferAuditLogType classifies audit logs by path.
func inferAuditLogType(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "action"
	}
	path := strings.ToLower(strings.TrimSpace(r.URL.Path))
	switch {
	case strings.HasPrefix(path, "/api/system/email/"),
		strings.HasPrefix(path, "/api/auth/register/email/"),
		strings.HasPrefix(path, "/api/auth/password/reset/"):
		return "email"
	case strings.Contains(path, "/backup"):
		return "backup"
	default:
		return "action"
	}
}

// writeSystemLog writes a system log entry. Failures are ignored.
func (s *Servers) writeSystemLog(ctx context.Context, logType, status, message, userID, username, ip string) {
	if s == nil || s.store == nil {
		return
	}
	logType = strings.TrimSpace(logType)
	if logType == "" {
		logType = "action"
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "success"
	}
	message = strings.TrimSpace(message)
	if ctx == nil {
		ctx = context.Background()
	}
	location := s.resolveIPLocation(ip)
	entry := &store.SystemLog{
		Type:      logType,
		Status:    status,
		Message:   message,
		UserID:    strings.TrimSpace(userID),
		Username:  strings.TrimSpace(username),
		IP:        strings.TrimSpace(ip),
		Location:  location,
		CreatedAt: time.Now(),
	}
	ctxLog, cancel := store.WithTimeout(context.Background())
	defer cancel()
	_ = s.store.CreateSystemLog(ctxLog, entry)
}

// resolveLogUser resolves user id + display name for logs.
func (s *Servers) resolveLogUser(ctx context.Context) (string, string) {
	var userID string
	var email string
	if ctx != nil {
		if v, ok := ctx.Value(ctxKeyUserID).(string); ok {
			userID = strings.TrimSpace(v)
		}
		if v, ok := ctx.Value(ctxKeyEmail).(string); ok {
			email = strings.TrimSpace(v)
		}
	}
	if userID == "" {
		return "", email
	}
	if s == nil || s.store == nil {
		return userID, email
	}
	ctxStore, cancel := store.WithTimeout(context.Background())
	defer cancel()
	u, err := s.store.GetUserByID(ctxStore, userID)
	if err == nil && u != nil && strings.TrimSpace(u.Username) != "" {
		return u.ID, u.Username
	}
	return userID, email
}

// resolveIPLocation maps IP to human-readable location using geoip.
func (s *Servers) resolveIPLocation(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || s == nil || s.geoip == nil {
		return ""
	}
	geo := s.geoip.Lookup(ip)
	if geo == nil {
		return ""
	}
	if name := strings.TrimSpace(geo.Name); name != "" {
		return name
	}
	parts := make([]string, 0, 3)
	if strings.TrimSpace(geo.Country) != "" {
		parts = append(parts, strings.TrimSpace(geo.Country))
	}
	if strings.TrimSpace(geo.Region) != "" {
		parts = append(parts, strings.TrimSpace(geo.Region))
	}
	if strings.TrimSpace(geo.City) != "" {
		parts = append(parts, strings.TrimSpace(geo.City))
	}
	return strings.TrimSpace(strings.Join(parts, " / "))
}

// getRequestIP extracts client IP from common proxy headers.
func getRequestIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// statusRecorder captures HTTP status code for audit logs.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}
