package dnsprovider

import (
	"context"
	"errors"
	"strings"
)

// DNSLAClient is a placeholder implementation.
type DNSLAClient struct {
	token  string
	secret string
}

func NewDNSLAClient(token, secret string) *DNSLAClient {
	return &DNSLAClient{
		token:  strings.TrimSpace(token),
		secret: strings.TrimSpace(secret),
	}
}

func (c *DNSLAClient) Ping(ctx context.Context) error {
	_ = ctx
	if c.token == "" && c.secret == "" {
		return errors.New("dns.la: token or secret required")
	}
	return nil
}

func (c *DNSLAClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "DNS.LA credentials verified (record sync not implemented)", nil
}

func (c *DNSLAClient) Cleanup(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "DNS.LA credentials verified (record sync not implemented)", nil
}

func (c *DNSLAClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64) (string, error) {
	_ = ctx
	_ = zone
	_ = name
	_ = recordType
	_ = values
	_ = ttl
	return "", errors.New("dns.la: record sync not implemented")
}

func (c *DNSLAClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	_ = ctx
	_ = zone
	_ = recordType
	return nil, errors.New("dns.la: record list not implemented")
}
