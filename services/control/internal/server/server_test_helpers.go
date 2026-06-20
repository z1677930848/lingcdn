package server

import (
	"testing"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/store"
)

func newTestServers(t *testing.T, mem store.Store) *Servers {
	t.Helper()
	if mem == nil {
		mem = store.NewMemory("test-token", "admin-token")
	}
	return New(config.Config{AuthSecret: "test-secret"}, nil, nil, nil, nil, nil, nil, mem, nil)
}
