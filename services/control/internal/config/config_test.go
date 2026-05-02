package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadWithOptionsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
database_url: "postgres://file-user:file-pass@127.0.0.1:5432/filedb?sslmode=disable"
control_ui_dir: "/srv/lingcdn/ui"
smtp_port: 2525
license_verify_interval: "15m"
portal_base: "https://portal.example.com"
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadWithOptions(LoadOptions{File: path})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}

	if cfg.DatabaseURL != "postgres://file-user:file-pass@127.0.0.1:5432/filedb?sslmode=disable" {
		t.Fatalf("unexpected database url: %q", cfg.DatabaseURL)
	}
	if cfg.ControlUIDir != "/srv/lingcdn/ui" {
		t.Fatalf("unexpected control ui dir: %q", cfg.ControlUIDir)
	}
	if cfg.SMTPPort != 2525 {
		t.Fatalf("unexpected smtp port: %d", cfg.SMTPPort)
	}
	if cfg.LicenseVerifyInterval != 15*time.Minute {
		t.Fatalf("unexpected license verify interval: %s", cfg.LicenseVerifyInterval)
	}
	if cfg.PortalBase != DefaultPortalBase {
		t.Fatalf("unexpected portal base: %q", cfg.PortalBase)
	}
}

func TestLoadWithOptionsEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("http_addr: ':8080'\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("HTTP_ADDR", ":9090")

	cfg, err := LoadWithOptions(LoadOptions{File: path})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("expected env override, got %q", cfg.HTTPAddr)
	}
}

func TestLoadWithOptionsAutoDetectsConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("control_ui_dir: '/auto/ui'\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)

	cfg, err := LoadWithOptions(LoadOptions{})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}

	if cfg.ControlUIDir != "/auto/ui" {
		t.Fatalf("unexpected control ui dir: %q", cfg.ControlUIDir)
	}
}

func TestLoadWithOptionsRejectsMissingFile(t *testing.T) {
	_, err := LoadWithOptions(LoadOptions{File: filepath.Join(t.TempDir(), "missing.yaml")})
	if err == nil {
		t.Fatalf("expected missing file error")
	}
}

func TestLoadWithOptionsRejectsNestedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("server:\n  http_addr: ':8080'\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadWithOptions(LoadOptions{File: path})
	if err == nil {
		t.Fatalf("expected nested config error")
	}
}

func TestLoadDetailedAutoCreatesConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	result, err := LoadDetailed(LoadOptions{AutoCreate: true})
	if err != nil {
		t.Fatalf("LoadDetailed: %v", err)
	}
	if !result.Generated {
		t.Fatalf("expected generated config file")
	}
	if filepath.Base(result.File) != "config.yaml" {
		t.Fatalf("unexpected config file path: %q", result.File)
	}
	if _, err := os.Stat(result.File); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
	if result.Config.AuthSecret == "" || result.Config.AuthSecret == "dev-secret-change-me" {
		t.Fatalf("expected generated auth secret")
	}
	if result.Config.AdminPassword == "" || result.Config.AdminPassword == "lingcdn123" {
		t.Fatalf("expected generated admin password")
	}
	if result.Config.ControlUIDir != "ui" {
		t.Fatalf("expected default ui dir, got %q", result.Config.ControlUIDir)
	}
}

func TestResolveControlUIDirPrefersAutoDetectedUIDir(t *testing.T) {
	dir := t.TempDir()
	uiDir := filepath.Join(dir, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatalf("mkdir ui: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "index.html"), []byte("INDEX"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	t.Chdir(dir)

	resolved := ResolveControlUIDir("")
	if resolved != uiDir {
		t.Fatalf("expected resolved ui dir %q, got %q", uiDir, resolved)
	}
}

func TestLoadIgnoresCustomPortalBaseAndLicenseMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("portal_base: 'https://portal.example.com'\nlicense_mode: 'open'\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("PORTAL_BASE", "https://evil.example.com")
	t.Setenv("LICENSE_MODE", "offline")

	cfg, err := LoadWithOptions(LoadOptions{File: path})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}

	if cfg.PortalBase != DefaultPortalBase {
		t.Fatalf("portal_base=%q want %q", cfg.PortalBase, DefaultPortalBase)
	}
	if cfg.LicenseMode != "online" {
		t.Fatalf("license_mode=%q want online", cfg.LicenseMode)
	}
}
