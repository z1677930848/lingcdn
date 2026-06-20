package dnsprovider

import (
	"context"
	"fmt"
)

// syncStubClient validates credentials but does not yet implement record sync.
type syncStubClient struct {
	provider string
	ping     func(context.Context) error
}

func (c *syncStubClient) Ping(ctx context.Context) error {
	if c.ping == nil {
		return fmt.Errorf("%s: credentials required", c.provider)
	}
	return c.ping(ctx)
}

func (c *syncStubClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s 凭证校验通过（记录同步尚未实现）", c.provider), nil
}

func (c *syncStubClient) Cleanup(ctx context.Context) (string, error) {
	return c.Recover(ctx)
}

func (c *syncStubClient) SupportsLine() bool { return false }

func (c *syncStubClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, line string, weights map[string]int32) (string, error) {
	_ = ctx
	_ = zone
	_ = name
	_ = recordType
	_ = values
	_ = ttl
	_ = line
	_ = weights
	return "", fmt.Errorf("%s: 记录同步尚未实现", c.provider)
}

func (c *syncStubClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	_ = ctx
	_ = zone
	_ = recordType
	return nil, fmt.Errorf("%s: 记录列表尚未实现", c.provider)
}

func (c *syncStubClient) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	_ = ctx
	return nil, fmt.Errorf("%s: 域名列表尚未实现", c.provider)
}
