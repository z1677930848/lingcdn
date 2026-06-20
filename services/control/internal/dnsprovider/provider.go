package dnsprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// Client defines minimal DNS provider operations used by cleanup/recover.
type Client interface {
	Ping(ctx context.Context) error
	// Recover should ensure critical records exist (stub for now).
	Recover(ctx context.Context) (string, error)
	// Cleanup should remove irrelevant records (stub for now).
	Cleanup(ctx context.Context) (string, error)
	// SupportsLine reports whether provider supports线路解析(例如电信/联通).
	SupportsLine() bool
	// EnsureRecords makes DNS records for (name, type) match the desired values.
	// - zone: DNS zone (e.g. cdn.example.com)
	// - name: relative record name within zone (e.g. "www" or "a.b"); use "@" for apex
	// - values: for A/AAAA -> IP list; for CNAME -> single target (len 0/1)
	// - line: 线路名称（默认线路可传空或"默认"）
	// - weights: optional value->weight map for A/AAAA (provider dependent)
	EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, line string, weights map[string]int32) (string, error)
	// ListRecords returns records in a zone (optionally filtered by type). Used for pruning.
	ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error)
	// ListProviderDomains returns all domain zones configured at the DNS provider account.
	ListProviderDomains(ctx context.Context) ([]ProviderDomain, error)
}

// ProviderDomain represents a domain zone from the DNS provider.
type ProviderDomain struct {
	Name        string `json:"name"`
	RecordCount int    `json:"record_count"`
}

// SyncReady reports whether automatic DNS record sync is implemented for a provider.
func SyncReady(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "alidns", "aliyun", "dnspod", "dnspod-global", "dnspod_global", "dnspod-intl", "dnspod-int", "cloudflare",
		"route53", "aws", "huawei", "huaweicloud", "google", "gcp", "googlecloud", "51dns", "dns51", "dnsla", "dns.la":
		return true
	default:
		return false
	}
}

// NewClient constructs a provider client based on cfg.Provider.
func NewClient(cfg *store.DNSConfig) (Client, error) {
	if cfg == nil {
		return nil, errors.New("dns config is nil")
	}
	switch cfg.Provider {
	case "alidns", "aliyun":
		return NewAliDNSClient(cfg.AccountID, cfg.Token, cfg.Secret), nil
	case "dnspod":
		return NewDNSPodClient(cfg.AccountID, cfg.Token), nil
	case "dnspod-global", "dnspod_global", "dnspod-intl", "dnspod-int":
		return NewDNSPodClient(cfg.AccountID, cfg.Token), nil
	case "cloudflare":
		return NewCloudflareClient(cfg.Token, cfg.Secret, cfg.AccountID), nil
	case "route53", "aws":
		ak, sk := strings.TrimSpace(cfg.AccountID), strings.TrimSpace(cfg.Secret)
		if ak == "" || sk == "" {
			return nil, errors.New("route53: access key / secret required")
		}
		return NewRoute53Client(ak, sk), nil
	case "huawei", "huaweicloud":
		ak, sk := strings.TrimSpace(cfg.AccountID), strings.TrimSpace(cfg.Secret)
		if ak == "" || sk == "" {
			return nil, errors.New("huawei dns: access key / secret required")
		}
		return NewHuaweiDNSClient(ak, sk), nil
	case "google", "gcp", "googlecloud":
		project := strings.TrimSpace(cfg.AccountID)
		token, secret := strings.TrimSpace(cfg.Token), strings.TrimSpace(cfg.Secret)
		if project == "" {
			return nil, errors.New("gcp dns: project id required (account_id)")
		}
		if token == "" && secret == "" {
			return nil, errors.New("gcp dns: token or service account json required")
		}
		return NewGoogleDNSClient(project, token, secret), nil
	case "51dns", "dns51":
		token, secret := strings.TrimSpace(cfg.Token), strings.TrimSpace(cfg.Secret)
		if token == "" && secret == "" {
			return nil, errors.New("51dns: token or secret required")
		}
		return NewDNS51Client(token, secret), nil
	case "dnsla", "dns.la":
		token, secret := strings.TrimSpace(cfg.Token), strings.TrimSpace(cfg.Secret)
		if token == "" && secret == "" {
			return nil, errors.New("dns.la: token or secret required")
		}
		return NewDNSLAClient(token, secret), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

// newHTTPClient returns a shared HTTP client with sane timeouts.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
	}
}
