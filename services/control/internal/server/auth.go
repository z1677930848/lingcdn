package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/store"
)

type ctxKey string

const (
	ctxKeyUserID ctxKey = "user_id"
	ctxKeyEmail  ctxKey = "email"
	ctxKeyRole   ctxKey = "role"
)

type authClaims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func issueJWT(secret string, user *store.User, ttl time.Duration) (string, error) {
	claims := authClaims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func parseJWT(secret, token string) (*authClaims, error) {
	var claims authClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return &claims, nil
}

func withUserContext(ctx context.Context, claims *authClaims) context.Context {
	ctx = context.WithValue(ctx, ctxKeyUserID, claims.UserID)
	ctx = context.WithValue(ctx, ctxKeyEmail, claims.Email)
	ctx = context.WithValue(ctx, ctxKeyRole, claims.Role)
	return ctx
}

func getUserRole(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRole).(string); ok {
		return v
	}
	return ""
}

func getUserID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyUserID).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func getUserEmail(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyEmail).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// isAdmin returns true when the caller has the admin role on the current context.
func isAdmin(ctx context.Context) bool {
	return getUserRole(ctx) == "admin"
}

// sameOriginRequest returns true when the request's Origin or Referer header
// matches the Host header (scheme-agnostic). This is the baseline CSRF check
// we apply before trusting a cookie-based token on state-changing methods.
// When neither Origin nor Referer is present we fall back to the restrictive
// answer (false) so that cross-origin scripted POSTs cannot ride cookie auth.
func sameOriginRequest(r *http.Request) bool {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return false
	}
	hostOnly := host
	if i := strings.IndexByte(hostOnly, ':'); i >= 0 {
		hostOnly = hostOnly[:i]
	}
	check := func(raw string) bool {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return false
		}
		// Allow both absolute URLs (https://host[:port]/...) and bare hosts.
		if i := strings.Index(raw, "://"); i >= 0 {
			raw = raw[i+3:]
		}
		if i := strings.IndexByte(raw, '/'); i >= 0 {
			raw = raw[:i]
		}
		// Strip port for comparison.
		if i := strings.IndexByte(raw, ':'); i >= 0 {
			raw = raw[:i]
		}
		return strings.EqualFold(raw, hostOnly)
	}
	if o := r.Header.Get("Origin"); o != "" {
		return check(o)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		return check(ref)
	}
	return false
}

// isSafeMethod returns true for HTTP methods that the HTTP spec classifies as
// safe (no state change). These are allowed to authenticate via cookie without
// a same-origin check.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func authenticateRequest(cfg config.Config, st store.Store, r *http.Request) (context.Context, bool) {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.Header.Get("X-Service-Token")
	}
	if token == "" {
		// Fallback: try cookie. Cookie-based auth is only trusted for safe
		// methods, or for state-changing methods when the request is same-origin
		// (basic CSRF defense). Otherwise we drop the cookie token and let the
		// request fail auth so the client must send an explicit header token.
		if c, err := r.Cookie("lingcdn_token"); err == nil {
			if isSafeMethod(r.Method) || sameOriginRequest(r) {
				token = c.Value
			}
		}
	}
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	// Service token path (for nodes or trusted callers)
	if token != "" && cfg.ServiceToken != "" && token == cfg.ServiceToken {
		return context.WithValue(r.Context(), ctxKeyRole, "service"), true
	}

	// JWT path
	if token != "" {
		if claims, err := parseJWT(cfg.AuthSecret, token); err == nil {
			return withUserContext(r.Context(), claims), true
		}
	}

	// Bootstrap token path for node registration, if configured
	if token != "" && st != nil {
		if ok, _ := st.ValidateBootstrapToken(r.Context(), token); ok {
			return context.WithValue(r.Context(), ctxKeyRole, "bootstrap"), true
		}
	}

	return r.Context(), false
}

func validatePassword(hash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
