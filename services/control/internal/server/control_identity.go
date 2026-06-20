package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
)

var controlHostnameRE = regexp.MustCompile(`(?i)^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// normalizeControlDomain lowercases and strips port/scheme from a hostname.
func normalizeControlDomain(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	if h, _, err := net.SplitHostPort(raw); err == nil {
		raw = h
	}
	raw = strings.TrimSuffix(raw, ".")
	if !isValidControlHostname(raw) {
		return ""
	}
	return raw
}

func isValidControlHostname(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" || len(host) > 253 {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	return controlHostnameRE.MatchString(host)
}

func (s *Servers) loadSettingsForControlIdentity() *store.Settings {
	if s.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil
	}
	return settings
}

func (s *Servers) effectiveControlDomain() string {
	domain := normalizeControlDomain(s.cfg.ControlDomain)
	if settings := s.loadSettingsForControlIdentity(); settings != nil {
		if d := normalizeControlDomain(settings.ControlDomain); d != "" {
			domain = d
		}
	}
	return domain
}

func (s *Servers) effectiveControlPublicHTTPS() bool {
	if s.cfg.ACMEEnable {
		return true
	}
	if settings := s.loadSettingsForControlIdentity(); settings != nil {
		return settings.ControlPublicHTTPS
	}
	return s.cfg.ControlPublicHTTPS
}

func (s *Servers) effectiveControlRedirectIP() bool {
	if settings := s.loadSettingsForControlIdentity(); settings != nil {
		return settings.ControlRedirectIP
	}
	return s.cfg.ControlRedirectToDomain
}

func (s *Servers) controlHTTPPort() string {
	port := "9080"
	if _, p, err := net.SplitHostPort(s.cfg.HTTPAddr); err == nil && strings.TrimSpace(p) != "" {
		port = strings.TrimSpace(p)
	}
	if s.effectiveControlPublicHTTPS() {
		if _, p, err := net.SplitHostPort(s.cfg.HTTPSAddr); err == nil && strings.TrimSpace(p) != "" {
			port = strings.TrimSpace(p)
		} else {
			port = "443"
		}
	}
	return port
}

func buildControlPublicURL(scheme, host, port string) string {
	if host == "" {
		return ""
	}
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		return scheme + "://" + host
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

func (s *Servers) resolveControlHTTPBase(requestHost string) string {
	scheme := "http"
	if s.effectiveControlPublicHTTPS() {
		scheme = "https"
	}
	port := s.controlHTTPPort()
	if domain := s.effectiveControlDomain(); domain != "" {
		return buildControlPublicURL(scheme, domain, port)
	}
	if ip := strings.TrimSpace(s.cfg.PublicIP); ip != "" && net.ParseIP(ip) != nil {
		return buildControlPublicURL(scheme, ip, port)
	}
	hostOnly := strings.TrimSpace(requestHost)
	if h, _, err := net.SplitHostPort(hostOnly); err == nil {
		hostOnly = h
	}
	if hostOnly != "" {
		return buildControlPublicURL(scheme, hostOnly, port)
	}
	return buildControlPublicURL(scheme, "127.0.0.1", port)
}

func (s *Servers) resolveControlGRPCEndpoint() string {
	_, grpcPort, err := net.SplitHostPort(s.cfg.GRPCAddr)
	if err != nil || strings.TrimSpace(grpcPort) == "" {
		grpcPort = "9443"
	}
	if ep := strings.TrimSpace(s.cfg.PublicGRPCEndpoint); ep != "" {
		if host, port, ok := parseControlHostPort(ep, grpcPort); ok {
			return net.JoinHostPort(host, port)
		}
	}
	if domain := s.effectiveControlDomain(); domain != "" {
		return net.JoinHostPort(domain, grpcPort)
	}
	if ip := strings.TrimSpace(s.cfg.PublicIP); ip != "" && net.ParseIP(ip) != nil {
		return net.JoinHostPort(ip, grpcPort)
	}
	return ""
}

func parseControlHostPort(hostPort, defaultPort string) (host, port string, ok bool) {
	host = strings.TrimSpace(hostPort)
	port = strings.TrimSpace(defaultPort)
	if h, p, err := net.SplitHostPort(hostPort); err == nil {
		host = strings.TrimSpace(h)
		port = strings.TrimSpace(p)
	}
	if host == "" || !isValidControlHostname(host) {
		return "", "", false
	}
	if port == "" {
		port = defaultPort
	}
	return host, port, true
}

func (s *Servers) controlIdentitySnapshot() map[string]any {
	domain := s.effectiveControlDomain()
	publicURL := s.resolveControlHTTPBase("")
	grpcEndpoint := s.resolveControlGRPCEndpoint()
	return map[string]any{
		"control_domain":         domain,
		"control_public_url":     publicURL,
		"control_grpc_endpoint":  grpcEndpoint,
		"control_public_https":   s.effectiveControlPublicHTTPS(),
		"control_redirect_ip":    s.effectiveControlRedirectIP(),
		"public_ip":              strings.TrimSpace(s.cfg.PublicIP),
		"configured_grpc":        strings.TrimSpace(s.cfg.PublicGRPCEndpoint),
	}
}

func (s *Servers) withControlDomainRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.effectiveControlRedirectIP() {
			next.ServeHTTP(w, r)
			return
		}
		domain := s.effectiveControlDomain()
		if domain == "" {
			next.ServeHTTP(w, r)
			return
		}
		host := strings.TrimSpace(r.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.ToLower(host)
		if host == domain || host == "localhost" || host == "127.0.0.1" {
			next.ServeHTTP(w, r)
			return
		}
		if ip := net.ParseIP(host); ip != nil {
			target := buildControlPublicURL(
				func() string {
					if s.effectiveControlPublicHTTPS() {
						return "https"
					}
					return "http"
				}(),
				domain,
				s.controlHTTPPort(),
			)
			http.Redirect(w, r, target+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Servers) verifyControlDomainDNS(domain string) (map[string]any, error) {
	domain = normalizeControlDomain(domain)
	if domain == "" {
		return nil, fmt.Errorf("invalid domain")
	}
	addrs, err := net.LookupHost(domain)
	if err != nil {
		return map[string]any{
			"domain":  domain,
			"ok":      false,
			"error":   err.Error(),
			"records": []string{},
		}, nil
	}
	expected := strings.TrimSpace(s.cfg.PublicIP)
	matched := expected == ""
	for _, addr := range addrs {
		if addr == expected {
			matched = true
			break
		}
	}
	return map[string]any{
		"domain":     domain,
		"ok":         matched,
		"records":    addrs,
		"expected":   expected,
		"public_url": s.resolveControlHTTPBase(""),
	}, nil
}

func (s *Servers) isControlDomainHost(host string) bool {
	domain := s.effectiveControlDomain()
	if domain == "" {
		return false
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host == domain
}
