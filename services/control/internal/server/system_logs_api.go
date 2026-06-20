package server

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// trustedProxyNets is an atomic snapshot of the parsed TrustedProxies config.
// It's consulted by getRequestIP to decide whether X-Forwarded-For /
// X-Real-IP from the current peer may be trusted. Kept as an atomic value
// so we don't need a mutex on every request.
var trustedProxyNets atomic.Pointer[[]*net.IPNet]

// setTrustedProxies replaces the trusted-proxy CIDR list used by
// getRequestIP. The string is the same format accepted by cfg.TrustedProxies:
// comma-separated CIDR or bare IP entries. Invalid entries are dropped with
// no error to keep startup resilient; callers that want to surface parse
// errors should call parseTrustedProxies directly.
func setTrustedProxies(list string) {
	nets := parseTrustedProxies(list)
	// Store a pointer to the slice (may be empty); nil means "no trusted
	// proxies configured, ignore XFF entirely".
	if len(nets) == 0 {
		var empty []*net.IPNet
		trustedProxyNets.Store(&empty)
		return
	}
	trustedProxyNets.Store(&nets)
}

// parseTrustedProxies parses a comma-separated list of CIDR blocks or bare
// IPs into *net.IPNet entries. A bare IP "a.b.c.d" becomes "a.b.c.d/32" and
// "::1" becomes "::1/128". Invalid tokens are skipped.
func parseTrustedProxies(list string) []*net.IPNet {
	var out []*net.IPNet
	for _, raw := range strings.Split(list, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "/") {
			if ip := net.ParseIP(entry); ip != nil {
				if ip4 := ip.To4(); ip4 != nil {
					entry = ip4.String() + "/32"
				} else {
					entry = ip.String() + "/128"
				}
			}
		}
		_, ipnet, err := net.ParseCIDR(entry)
		if err != nil || ipnet == nil {
			continue
		}
		out = append(out, ipnet)
	}
	return out
}

// isTrustedProxyIP reports whether ip belongs to any of the configured
// trusted-proxy CIDRs. Loopback addresses (127.0.0.1 / ::1) are always
// treated as trusted so a co-located Nginx reverse proxy works out of the
// box without TRUSTED_PROXIES — external clients still cannot spoof XFF
// because their TCP peer is never loopback.
func isTrustedProxyIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	ptr := trustedProxyNets.Load()
	if ptr == nil {
		return false
	}
	for _, n := range *ptr {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

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

// getRequestIP extracts the client IP honoring the TrustedProxies config.
//
// Security note: when the immediate TCP peer is not a trusted proxy we MUST
// NOT look at X-Forwarded-For / X-Real-IP, because a client can send
// "X-Forwarded-For: <random>" to rotate through rate-limiter buckets.
// Loopback peers (same-host Nginx) are always trusted. Additional CIDRs
// (CDN / upstream LB) can be listed in TRUSTED_PROXIES.
func getRequestIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	peer := remoteAddrIP(r)
	// When no trusted proxies are configured we simply use the peer IP.
	// This is the correct default for a control plane reachable directly,
	// and it also guarantees that a misconfigured deployment fails-closed
	// rather than fails-open for the rate limiters.
	if !isTrustedProxyIP(net.ParseIP(peer)) {
		return peer
	}

	// Peer is a trusted proxy — honor X-Forwarded-For, walking the chain
	// from right (closest hop) to left (originating client). The first
	// entry that's NOT itself a trusted proxy is the real client IP.
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			if candidate == "" {
				continue
			}
			ip := net.ParseIP(candidate)
			if ip == nil {
				continue
			}
			if !isTrustedProxyIP(ip) {
				return candidate
			}
		}
		// All entries were trusted proxies (unusual) — fall through to
		// X-Real-IP / RemoteAddr rather than returning a proxy address.
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		return xr
	}
	return peer
}

// remoteAddrIP returns the host portion of r.RemoteAddr, trimmed of any
// port suffix and surrounding whitespace. It's the IP of the immediate
// TCP peer and is never influenced by headers.
func remoteAddrIP(r *http.Request) string {
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
