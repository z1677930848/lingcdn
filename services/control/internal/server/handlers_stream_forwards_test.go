package server

import (
	"context"
	"testing"

	"github.com/lingcdn/control/internal/store"
)

func testServers(t *testing.T) *Servers {
	t.Helper()
	return newTestServers(t, store.NewMemory("test-token", "admin-token"))
}

func TestNormalizeStreamForward(t *testing.T) {
	s := testServers(t)
	ctx := context.Background()

	sf := &store.StreamForward{
		UserID:     "user-1",
		Protocol:   "tcp",
		ListenPort: 9000,
		OriginHost: "10.0.0.1",
		OriginPort: 8080,
		Enabled:    true,
	}
	if err := s.normalizeStreamForward(ctx, sf, ""); err != nil {
		t.Fatalf("normalize valid rule: %v", err)
	}
	if sf.Protocol != "tcp" {
		t.Fatalf("expected tcp, got %q", sf.Protocol)
	}

	bad := &store.StreamForward{
		UserID:     "user-1",
		Protocol:   "icmp",
		ListenPort: 9000,
		OriginHost: "10.0.0.1",
		OriginPort: 8080,
	}
	if err := s.normalizeStreamForward(ctx, bad, ""); err == nil {
		t.Fatal("expected error for invalid protocol")
	}
}

func TestEnforceStreamForwardQuotaNoProduct(t *testing.T) {
	s := testServers(t)
	ctx := context.Background()
	if err := s.enforceStreamForwardQuota(ctx, "unknown-user", ""); err == nil {
		t.Fatal("expected quota error without active product")
	}
}
