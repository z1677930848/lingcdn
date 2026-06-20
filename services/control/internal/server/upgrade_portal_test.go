package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchPortalLatestCachedHitsCache(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"product":"node","version":"1.2.3","channel":"stable","platform":"linux","arch":"amd64"}`))
	}))
	t.Cleanup(srv.Close)

	s := &Servers{portalLatestCache: make(map[string]portalLatestCacheEntry)}
	ctx := context.Background()

	first, err := s.fetchPortalLatestCached(ctx, srv.URL, "node", "stable", "linux", "amd64", "latest")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if first.Version != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %q", first.Version)
	}

	second, err := s.fetchPortalLatestCached(ctx, srv.URL, "node", "stable", "linux", "amd64", "latest")
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if second.Version != "1.2.3" {
		t.Fatalf("expected cached version 1.2.3, got %q", second.Version)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 portal call, got %d", got)
	}
}

func TestFetchPortalLatestCachedExpiresAfterTTL(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{"product":"node","version":"1.0.0","channel":"stable","platform":"linux","arch":"amd64"}`))
			return
		}
		_, _ = w.Write([]byte(`{"product":"node","version":"2.0.0","channel":"stable","platform":"linux","arch":"amd64"}`))
	}))
	t.Cleanup(srv.Close)

	s := &Servers{portalLatestCache: make(map[string]portalLatestCacheEntry)}
	ctx := context.Background()
	key := portalLatestCacheKey(srv.URL, "node", "stable", "linux", "amd64", "latest")

	if _, err := s.fetchPortalLatestCached(ctx, srv.URL, "node", "stable", "linux", "amd64", "latest"); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	s.portalLatestCacheMu.Lock()
	s.portalLatestCache[key] = portalLatestCacheEntry{
		latest: &portalLatest{Version: "1.0.0"},
		at:     time.Now().Add(-portalLatestCacheTTL - time.Second),
	}
	s.portalLatestCacheMu.Unlock()

	latest, err := s.fetchPortalLatestCached(ctx, srv.URL, "node", "stable", "linux", "amd64", "latest")
	if err != nil {
		t.Fatalf("refetch after expiry: %v", err)
	}
	if latest.Version != "2.0.0" {
		t.Fatalf("expected refreshed version 2.0.0, got %q", latest.Version)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 portal calls, got %d", got)
	}
}
