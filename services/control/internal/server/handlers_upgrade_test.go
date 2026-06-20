package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lingcdn/control/internal/store"
)

func TestHandleUpgradeInfoRequiresAdmin(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	s := newTestServers(t, mem)

	req := httptest.NewRequest(http.MethodGet, "/api/system/upgrade", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyRole, "user"))
	w := httptest.NewRecorder()
	s.handleUpgradeInfo(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-admin, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpgradeInfoAdminOK(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	s := newTestServers(t, mem)

	req := httptest.NewRequest(http.MethodGet, "/api/system/upgrade", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyRole, "admin"))
	w := httptest.NewRecorder()
	s.handleUpgradeInfo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}
