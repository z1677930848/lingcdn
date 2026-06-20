package server

import (
	"testing"

	"github.com/lingcdn/control/internal/store"
)

func TestResolveESDomainField(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "domain"},
		{"domain", "domain"},
		{"domain.keyword", "domain"},
		{"host", "host"},
	}
	for _, tc := range tests {
		got := resolveESDomainField(&store.Settings{ElasticsearchDomainField: tc.in})
		if got != tc.want {
			t.Fatalf("resolveESDomainField(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveESClientIPField(t *testing.T) {
	if got := resolveESClientIPField(); got != "client_ip" {
		t.Fatalf("resolveESClientIPField() = %q", got)
	}
}
