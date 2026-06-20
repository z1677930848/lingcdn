package server

import (
	"testing"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/store"
)

func TestNormalizeControlDomain(t *testing.T) {
	cases := map[string]string{
		"CDUser.EXMC.CN":  "cduser.exmc.cn",
		"https://foo.com": "foo.com",
		"foo.com:9080":    "foo.com",
		"":                "",
		"not a domain!":   "",
	}
	for in, want := range cases {
		if got := normalizeControlDomain(in); got != want {
			t.Fatalf("normalizeControlDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveControlHTTPBaseUsesDomain(t *testing.T) {
	s := &Servers{cfg: config.Config{
		HTTPAddr:      ":9080",
		PublicIP:      "38.76.191.153",
		ControlDomain: "cduser.exmc.cn",
	}}
	got := s.resolveControlHTTPBase("")
	if got != "http://cduser.exmc.cn:9080" {
		t.Fatalf("unexpected base: %s", got)
	}
}

func TestResolveControlHTTPBaseHTTPS(t *testing.T) {
	s := &Servers{cfg: config.Config{
		HTTPSAddr:          ":443",
		ControlDomain:      "cduser.exmc.cn",
		ControlPublicHTTPS: true,
	}}
	got := s.resolveControlHTTPBase("")
	if got != "https://cduser.exmc.cn" {
		t.Fatalf("unexpected base: %s", got)
	}
}

func TestResolveControlGRPCEndpoint(t *testing.T) {
	s := &Servers{cfg: config.Config{
		GRPCAddr:      ":9443",
		ControlDomain: "cduser.exmc.cn",
	}}
	if got := s.resolveControlGRPCEndpoint(); got != "cduser.exmc.cn:9443" {
		t.Fatalf("unexpected grpc endpoint: %s", got)
	}
}

func TestSettingsOverrideConfigDomain(t *testing.T) {
	mem := store.NewMemory("", "")
	ctx := t.Context()
	if err := mem.UpdateSettings(ctx, &store.Settings{
		ID:            "default",
		ControlDomain: "panel.example.com",
	}); err != nil {
		t.Fatal(err)
	}
	s := &Servers{
		cfg:   config.Config{ControlDomain: "cfg.example.com", HTTPAddr: ":9080"},
		store: mem,
	}
	if got := s.effectiveControlDomain(); got != "panel.example.com" {
		t.Fatalf("settings should override config, got %q", got)
	}
}

func TestSettingsOverrideConfigRedirectOff(t *testing.T) {
	mem := store.NewMemory("", "")
	ctx := t.Context()
	if err := mem.UpdateSettings(ctx, &store.Settings{ID: "default", ControlRedirectIP: false}); err != nil {
		t.Fatal(err)
	}
	s := &Servers{
		cfg:   config.Config{ControlRedirectToDomain: true, ControlDomain: "cduser.exmc.cn"},
		store: mem,
	}
	if s.effectiveControlRedirectIP() {
		t.Fatal("settings false should disable redirect even when config true")
	}
}

func TestBuildControlPublicURL(t *testing.T) {
	if got := buildControlPublicURL("https", "foo.com", "443"); got != "https://foo.com" {
		t.Fatalf("unexpected url: %s", got)
	}
	if got := buildControlPublicURL("http", "1.2.3.4", "9080"); got != "http://1.2.3.4:9080" {
		t.Fatalf("unexpected url: %s", got)
	}
}
