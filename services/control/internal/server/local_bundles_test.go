package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lingcdn/control/internal/config"
)

func TestParseBundleFilename(t *testing.T) {
	product, version, platform, arch, ok := parseBundleFilename("lingcdn-node-1.0.9-linux-amd64.tar.gz")
	if !ok || product != "node" || version != "1.0.9" || platform != "linux" || arch != "amd64" {
		t.Fatalf("unexpected parse: ok=%v product=%q version=%q platform=%q arch=%q", ok, product, version, platform, arch)
	}
	if _, _, _, _, ok := parseBundleFilename("evil.bin"); ok {
		t.Fatal("expected invalid filename to fail")
	}
}

func TestCompareVersion(t *testing.T) {
	if compareVersion("1.0.10", "1.0.9") <= 0 {
		t.Fatal("1.0.10 should be greater than 1.0.9")
	}
	if compareVersion("1.0.9", "1.0.10") >= 0 {
		t.Fatal("1.0.9 should be less than 1.0.10")
	}
}

func TestOfflineLocalBundleEnabled(t *testing.T) {
	s := &Servers{cfg: config.Config{LicenseMode: "online"}}
	if s.offlineLocalBundleEnabled() {
		t.Fatal("online mode should not enable local bundles")
	}
	s.cfg.LicenseMode = "offline"
	if !s.offlineLocalBundleEnabled() {
		t.Fatal("offline mode should enable local bundles")
	}
}

func TestResolveLocalBundleLatest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"lingcdn-node-1.0.8-linux-amd64.tar.gz",
		"lingcdn-node-1.0.9-linux-amd64.tar.gz",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("stub"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &Servers{cfg: config.Config{LicenseMode: "offline", LocalBundleDir: dir}}

	rec, err := s.resolveLocalBundle("node", "stable", "linux", "amd64", "latest", "http://127.0.0.1:9080")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Version != "1.0.9" {
		t.Fatalf("expected latest 1.0.9, got %s", rec.Version)
	}
}
