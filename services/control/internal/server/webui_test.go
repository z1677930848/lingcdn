package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTryServeControlUI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("INDEX"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("JS"), 0o644); err != nil {
		t.Fatalf("write js: %v", err)
	}

	s := &Servers{}
	s.cfg.ControlUIDir = dir

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.URL.Path = "/"
	if !s.tryServeControlUI(rr, req) {
		t.Fatalf("expected served")
	}
	if rr.Body.String() != "INDEX" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "http://example.com/assets/app.js", nil)
	req.URL.Path = "/assets/app.js"
	if !s.tryServeControlUI(rr, req) {
		t.Fatalf("expected served")
	}
	if rr.Body.String() != "JS" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "http://example.com/any/route", nil)
	req.URL.Path = "/any/route"
	if !s.tryServeControlUI(rr, req) {
		t.Fatalf("expected served")
	}
	if rr.Body.String() != "INDEX" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestTryServeControlUINotConfigured(t *testing.T) {
	s := &Servers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.URL.Path = "/"
	if s.tryServeControlUI(rr, req) {
		t.Fatalf("expected not served")
	}
}

func TestTryServeControlUIAutoResolvesUIDir(t *testing.T) {
	dir := t.TempDir()
	uiDir := filepath.Join(dir, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatalf("mkdir ui: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "index.html"), []byte("AUTO"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	t.Chdir(dir)

	s := &Servers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.URL.Path = "/"
	if !s.tryServeControlUI(rr, req) {
		t.Fatalf("expected served from auto-resolved ui dir")
	}
	if rr.Body.String() != "AUTO" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}
