package dnsprovider

import (
	"context"
	"errors"
	"strings"
)

// HuaweiDNSClient is a placeholder implementation.
type HuaweiDNSClient struct {
	ak string
	sk string
}

func NewHuaweiDNSClient(ak, sk, _ string) *HuaweiDNSClient {
	return &HuaweiDNSClient{ak: strings.TrimSpace(ak), sk: strings.TrimSpace(sk)}
}

func (c *HuaweiDNSClient) Ping(ctx context.Context) error {
	_ = ctx
	if c.ak == "" || c.sk == "" {
		return errors.New("huawei dns: access key / secret required")
	}
	return nil
}

func (c *HuaweiDNSClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "Huawei Cloud DNS credentials verified (record sync not implemented)", nil
}

func (c *HuaweiDNSClient) Cleanup(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "Huawei Cloud DNS credentials verified (record sync not implemented)", nil
}

func (c *HuaweiDNSClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64) (string, error) {
	_ = ctx
	_ = zone
	_ = name
	_ = recordType
	_ = values
	_ = ttl
	return "", errors.New("huawei dns: record sync not implemented")
}

func (c *HuaweiDNSClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	_ = ctx
	_ = zone
	_ = recordType
	return nil, errors.New("huawei dns: record list not implemented")
}
