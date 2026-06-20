package compiler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lingcdn/control/internal/store"
)

func TestCompileIncludesStreamForwards(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	ctx := context.Background()

	sf := &store.StreamForward{
		ID:                 "sf-1",
		UserID:             "user-1",
		Name:               "test-tcp",
		Protocol:           "tcp",
		ListenPort:         9500,
		OriginHost:         "127.0.0.1",
		OriginPort:         8080,
		Enabled:            true,
		HealthCheckEnabled: true,
	}
	if err := mem.CreateStreamForward(ctx, sf); err != nil {
		t.Fatalf("create stream forward: %v", err)
	}

	c := New(mem)
	_, payload, err := c.Compile(ctx)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	var cfg NodeConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		t.Fatalf("unmarshal node config: %v", err)
	}
	if len(cfg.StreamForwards) != 1 {
		t.Fatalf("expected 1 stream forward, got %d", len(cfg.StreamForwards))
	}
	got := cfg.StreamForwards[0]
	if got.ID != "sf-1" || got.ListenPort != 9500 || !got.HealthCheckEnabled {
		t.Fatalf("unexpected stream forward: %+v", got)
	}
}
