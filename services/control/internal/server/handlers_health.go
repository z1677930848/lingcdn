package server

// Health/readiness/ping probes + admin-only overview stats and system info.
// These are the small-stakes endpoints the UI dashboard polls on an
// interval, so they must stay fast and never touch slow backends (ES, etc).

import (
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/buildinfo"
	"github.com/lingcdn/control/internal/store"
)

// Health check handlers
func (s *Servers) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Servers) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if s.store != nil {
		if err := s.store.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("db not ready"))
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Servers) handlePing(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "lingcdn-control",
		"nodes":   s.hub.Count(),
	})
}

// handleAdminStats returns admin overview metrics.
func (s *Servers) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	totalUsers := 0
	totalProducts := 0
	totalLicenses := 0

	if s.store != nil {
		if users, err := s.store.ListUsers(ctx, 1000); err == nil {
			totalUsers = len(users)
		}
		if domains, err := s.store.ListDomains(ctx); err == nil {
			totalProducts = len(domains)
		}
		if lic, err := s.store.GetLicenseState(ctx); err == nil && lic != nil {
			if lic.MaxNodes > 0 {
				totalLicenses = lic.MaxNodes
			} else {
				totalLicenses = 1
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"totalUsers":    totalUsers,
		"totalProducts": totalProducts,
		"totalLicenses": totalLicenses,
		"totalRevenue":  0,
	})
}

func (s *Servers) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	startedAt := s.startedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	uptime := time.Since(startedAt)
	buildTime, buildHash := resolveBuildMetadata()
	// Authoritative version: compiled-in via buildinfo. Never fall back to
	// placeholder strings like "1.0.0-beta.1" — the old behaviour was that
	// /api/system/info and /api/control/report disagreed about what version
	// this control plane was running, which made upgrade bugs very hard to
	// diagnose.
	currentVersion := buildinfo.Version()

	writeJSON(w, http.StatusOK, map[string]any{
		"info": map[string]any{
			"started_at":     startedAt.Format(time.RFC3339),
			"uptime_seconds": int64(uptime.Seconds()),
			"build_time":     buildTime,
			"build_hash":     buildHash,
			"debug_mode":     strings.EqualFold(s.cfg.LogLevel, "debug"),
			"db_debug_mode":  strings.EqualFold(s.cfg.StoreBackend, "memory"),
			"app_version":    currentVersion,
			"go_version":     runtime.Version(),
		},
	})
}
