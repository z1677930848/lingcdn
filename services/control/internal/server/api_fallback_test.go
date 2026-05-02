package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lingcdn/control/internal/config"
)

func TestAdminMux_NotFoundAPIReturnsJSON404(t *testing.T) {
	s := New(config.Config{}, nil, nil, nil, nil, nil, nil, nil)
	h := s.adminMux()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/not-exist", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json, got %q", got)
	}
	if body := rr.Body.String(); body == "" || body == "lingcdn-control API" {
		t.Fatalf("unexpected body: %q", body)
	}
}
