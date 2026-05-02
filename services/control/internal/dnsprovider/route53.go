package dnsprovider

import (
	"context"
	"errors"
	"strings"
)

// Route53Client is a placeholder implementation.
type Route53Client struct {
	ak string
	sk string
}

func NewRoute53Client(ak, sk string) *Route53Client {
	return &Route53Client{ak: strings.TrimSpace(ak), sk: strings.TrimSpace(sk)}
}

func (c *Route53Client) Ping(ctx context.Context) error {
	_ = ctx
	if c.ak == "" || c.sk == "" {
		return errors.New("route53: access key / secret required")
	}
	return nil
}

func (c *Route53Client) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "Route53 credentials verified (record sync not implemented)", nil
}

func (c *Route53Client) Cleanup(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "Route53 credentials verified (record sync not implemented)", nil
}

func (c *Route53Client) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64) (string, error) {
	_ = ctx
	_ = zone
	_ = name
	_ = recordType
	_ = values
	_ = ttl
	return "", errors.New("route53: record sync not implemented")
}

func (c *Route53Client) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	_ = ctx
	_ = zone
	_ = recordType
	return nil, errors.New("route53: record list not implemented")
}
