package server

import (
	"net/http"
	"strings"

	"github.com/lingcdn/control/internal/store"
)

// Middleware is the canonical HTTP middleware signature used across this
// package. Composing these with `chain` removes the method-guard,
// admin-check, and store-timeout boilerplate that otherwise leaks into
// every handler (around 150+ duplicate blocks today).
type Middleware func(http.HandlerFunc) http.HandlerFunc

// chain composes middlewares so the first argument runs outermost and the
// innermost one is the last to run before `h`. That way writing
//
//	chain(handler, methodOnly(http.MethodGet), adminOnly, withStoreTimeout)
//
// reads top-to-bottom as "reject wrong methods, then reject non-admins,
// then apply DB timeout, then call handler" — which matches how we want
// to short-circuit failures cheaply.
func chain(h http.HandlerFunc, mws ...Middleware) http.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		if mw := mws[i]; mw != nil {
			h = mw(h)
		}
	}
	return h
}

// methodOnly rejects any request whose method is not in the whitelist with a
// 405 + JSON error, matching the shape every handler currently produces by
// hand. Passing no methods makes this a no-op (useful for conditional wiring).
func methodOnly(methods ...string) Middleware {
	if len(methods) == 0 {
		return func(next http.HandlerFunc) http.HandlerFunc { return next }
	}
	allowed := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		allowed[strings.ToUpper(strings.TrimSpace(m))] = struct{}{}
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if _, ok := allowed[strings.ToUpper(r.Method)]; !ok {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
				return
			}
			next(w, r)
		}
	}
}

// withStoreTimeout replaces r.Context() with the store's default timeout
// so handlers stop having to write `ctx, cancel := store.WithTimeout(r.Context()); defer cancel()`
// at the top of every function. The cancel is scheduled when the handler
// returns, so a long-running handler still gets cut off at the configured
// timeout — preserving the original safety invariant.
//
// Admin/auth concerns are intentionally handled elsewhere: see the existing
// s.withAuth / s.withAdmin methods, both of which satisfy the Middleware
// signature and can be composed here via chain().
func withStoreTimeout(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := store.WithTimeout(r.Context())
		defer cancel()
		next(w, r.WithContext(ctx))
	}
}

