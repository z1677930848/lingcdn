package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lingcdn/control/internal/store"
)

func TestHandleLogoutRevokesToken(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	user := &store.User{ID: "user-logout", Username: "logout-user", Email: "logout@example.com", Role: "user"}
	if err := mem.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	s := newTestServers(t, mem)
	token, err := issueJWT(s.cfg.AuthSecret, user, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.handleLogout(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logout status: %d body=%s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	s.withAuth(s.handleMe)(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked token to fail auth, got %d", w2.Code)
	}
}
