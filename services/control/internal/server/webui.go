package server

// Embedded Control UI static-file server. Serves the built SPA under the root
// path when the UI dir resolver (see config.ResolveControlUIDir) finds a
// valid index.html; falls through to API routing otherwise so that deployments
// without a bundled UI still work headless.

import (
	"net/http"
	"os"
	"strings"

	"github.com/lingcdn/control/internal/config"
)

func (s *Servers) tryServeControlUI(w http.ResponseWriter, r *http.Request) bool {
	uiDir := config.ResolveControlUIDir(s.cfg.ControlUIDir)
	if uiDir == "" {
		return false
	}
	st, err := os.Stat(uiDir)
	if err != nil || !st.IsDir() {
		return false
	}

	// Serve an exact file match if present; otherwise fall back to index.html
	// for SPA deep links (e.g. /dashboard or /users/123).
	rel := strings.TrimPrefix(r.URL.Path, "/")
	if rel == "" {
		rel = "index.html"
	}
	target, err := safeJoin(uiDir, rel)
	if err == nil {
		if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, target)
			return true
		}
	}

	index, err := safeJoin(uiDir, "index.html")
	if err != nil {
		return false
	}
	if fi, err := os.Stat(index); err != nil || fi.IsDir() {
		return false
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
	return true
}
