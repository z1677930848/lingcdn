package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Servers) handleGeoIPLatest(w http.ResponseWriter, r *http.Request) {
	role := getUserRole(r.Context())
	if role != "service" && role != "bootstrap" && role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权限操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if s.geoip == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "GeoIP未配置"})
		return
	}
	st := s.geoip.Snapshot()
	if st.SHA256 == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "GeoIP数据库未就绪", "last_error": st.LastError})
		return
	}
	edition := strings.TrimSpace(s.cfg.GeoIPEdition)
	if edition == "" {
		edition = "GeoLite2-City"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sha256":       st.SHA256,
		"size_bytes":   st.SizeBytes,
		"updated_at":   st.UpdatedAt.Unix(),
		"edition":      edition,
		"filename":     edition + ".mmdb",
		"download_url": "/api/geoip/file",
	})
}

func (s *Servers) handleGeoIPFile(w http.ResponseWriter, r *http.Request) {
	role := getUserRole(r.Context())
	if role != "service" && role != "bootstrap" && role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权限操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if s.geoip == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "GeoIP未配置"})
		return
	}
	path := s.geoip.targetPath()
	fi, err := os.Stat(path)
	if err != nil || fi == nil || fi.IsDir() {
		st := s.geoip.Snapshot()
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "GeoIP数据库未就绪", "last_error": st.LastError})
		return
	}
	st := s.geoip.Snapshot()
	edition := strings.TrimSpace(s.cfg.GeoIPEdition)
	if edition == "" {
		edition = "GeoLite2-City"
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(edition+".mmdb"))
	w.Header().Set("Cache-Control", "no-store")
	if st.SHA256 != "" {
		w.Header().Set("ETag", `"`+st.SHA256+`"`)
	}
	http.ServeFile(w, r, path)
}

func (s *Servers) handleGeoIPStatus(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权限操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if s.geoip == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	st := s.geoip.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":     true,
		"sha256":      st.SHA256,
		"size_bytes":  st.SizeBytes,
		"updated_at":  st.UpdatedAt.Unix(),
		"updated_at_h": st.UpdatedAt.Format(time.RFC3339),
		"last_error":  st.LastError,
		"storage_dir": s.cfg.GeoIPStorageDir,
		"edition":     s.cfg.GeoIPEdition,
		"interval_s":  int64(s.cfg.GeoIPUpdateInterval.Seconds()),
	})
}

func (s *Servers) handleGeoIPRefresh(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权限操作"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if s.geoip == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "GeoIP未配置"})
		return
	}
	if err := s.geoip.Refresh(r.Context()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	st := s.geoip.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sha256": st.SHA256, "updated_at": st.UpdatedAt.Unix()})
}

