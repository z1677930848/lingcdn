package dnsprovider

import (
	"context"
	"errors"
	"strings"
)

// DNS51Client is a placeholder implementation.
type DNS51Client struct {
	token  string
	secret string
}

func NewDNS51Client(token, secret string) *DNS51Client {
	return &DNS51Client{
		token:  strings.TrimSpace(token),
		secret: strings.TrimSpace(secret),
	}
}

func (c *DNS51Client) Ping(ctx context.Context) error {
	_ = ctx
	if c.token == "" && c.secret == "" {
		return errors.New("51dns: token or secret required")
	}
	return nil
}

func (c *DNS51Client) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "51DNS credentials verified (record sync not implemented)", nil
}

func (c *DNS51Client) Cleanup(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "51DNS credentials verified (record sync not implemented)", nil
}

func (c *DNS51Client) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64) (string, error) {
	_ = ctx
	_ = zone
	_ = name
	_ = recordType
	_ = values
	_ = ttl
	return "", errors.New("51dns: record sync not implemented")
}

func (c *DNS51Client) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	_ = ctx
	_ = zone
	_ = recordType
	return nil, errors.New("51dns: record list not implemented")
}
