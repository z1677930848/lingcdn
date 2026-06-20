package server

import (
	"net"
	"net/http"
	"testing"
)

func TestGetRequestIPLoopbackProxyUsesXFF(t *testing.T) {
	setTrustedProxies("")
	r, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	r.RemoteAddr = "127.0.0.1:54321"
	r.Header.Set("X-Forwarded-For", "203.0.113.50, 127.0.0.1")
	if got := getRequestIP(r); got != "203.0.113.50" {
		t.Fatalf("expected real client IP from XFF via loopback proxy, got %q", got)
	}
}

func TestGetRequestIPPublicPeerIgnoresSpoofedXFF(t *testing.T) {
	setTrustedProxies("")
	r, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	r.RemoteAddr = "198.51.100.10:54321"
	r.Header.Set("X-Forwarded-For", "203.0.113.50")
	if got := getRequestIP(r); got != "198.51.100.10" {
		t.Fatalf("public peer should ignore spoofed XFF, got %q", got)
	}
}

func TestGetRequestIPConfiguredTrustedProxy(t *testing.T) {
	setTrustedProxies("10.0.0.0/8")
	r, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	r.RemoteAddr = "10.1.2.3:8080"
	r.Header.Set("X-Real-IP", "203.0.113.77")
	if got := getRequestIP(r); got != "203.0.113.77" {
		t.Fatalf("expected X-Real-IP from trusted subnet peer, got %q", got)
	}
}

func TestIsTrustedProxyIPLoopbackAlwaysTrusted(t *testing.T) {
	setTrustedProxies("")
	if !isTrustedProxyIP(net.ParseIP("127.0.0.1")) {
		t.Fatal("127.0.0.1 should always be trusted")
	}
}
