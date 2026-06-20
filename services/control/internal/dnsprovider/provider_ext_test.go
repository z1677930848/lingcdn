package dnsprovider

import (
	"net/url"
	"testing"

	"github.com/lingcdn/control/internal/store"
)

func TestDNS51Sign(t *testing.T) {
	params := url.Values{}
	params.Set("apiKey", "test-key")
	params.Set("domain", "example.com")
	params.Set("timestamp", "1521005892")
	got := dns51Sign(params, "test-secret")
	if got == "" {
		t.Fatal("expected non-empty hash")
	}
	if len(got) != 32 {
		t.Fatalf("expected md5 hex length 32, got %d", len(got))
	}
}

func TestSyncReadyExtendedProviders(t *testing.T) {
	for _, p := range []string{"route53", "huawei", "google", "51dns", "dnsla"} {
		if !SyncReady(p) {
			t.Fatalf("expected sync ready for %s", p)
		}
	}
}

func TestNewClientExtendedProviders(t *testing.T) {
	cases := []struct {
		provider string
		cfg      func() *store.DNSConfig
	}{
		{"route53", func() *store.DNSConfig {
			return &store.DNSConfig{Provider: "route53", AccountID: "ak", Secret: "sk"}
		}},
		{"huawei", func() *store.DNSConfig {
			return &store.DNSConfig{Provider: "huawei", AccountID: "ak", Secret: "sk"}
		}},
		{"google", func() *store.DNSConfig {
			return &store.DNSConfig{Provider: "google", AccountID: "proj", Token: "token"}
		}},
		{"51dns", func() *store.DNSConfig {
			return &store.DNSConfig{Provider: "51dns", Token: "key", Secret: "secret"}
		}},
		{"dnsla", func() *store.DNSConfig {
			return &store.DNSConfig{Provider: "dnsla", Token: "id", Secret: "secret"}
		}},
	}
	for _, tc := range cases {
		client, err := NewClient(tc.cfg())
		if err != nil {
			t.Fatalf("%s: %v", tc.provider, err)
		}
		if client == nil {
			t.Fatalf("%s: nil client", tc.provider)
		}
	}
}
