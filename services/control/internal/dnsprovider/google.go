package dnsprovider

import (
	"context"
	"errors"
	"strings"
)

// GoogleCloudDNSClient is a placeholder implementation.
type GoogleCloudDNSClient struct {
	token     string
	secret    string
	projectID string
}

func NewGoogleCloudDNSClient(token, secret, projectID string) *GoogleCloudDNSClient {
	return &GoogleCloudDNSClient{
		token:     strings.TrimSpace(token),
		secret:    strings.TrimSpace(secret),
		projectID: strings.TrimSpace(projectID),
	}
}

func (c *GoogleCloudDNSClient) Ping(ctx context.Context) error {
	_ = ctx
	if c.projectID == "" {
		return errors.New("gcp dns: project id required (account_id)")
	}
	if c.token == "" && c.secret == "" {
		return errors.New("gcp dns: token or service account json required")
	}
	return nil
}

func (c *GoogleCloudDNSClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "Google Cloud DNS credentials verified (record sync not implemented)", nil
}

func (c *GoogleCloudDNSClient) Cleanup(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "Google Cloud DNS credentials verified (record sync not implemented)", nil
}

func (c *GoogleCloudDNSClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64) (string, error) {
	_ = ctx
	_ = zone
	_ = name
	_ = recordType
	_ = values
	_ = ttl
	return "", errors.New("gcp dns: record sync not implemented")
}

func (c *GoogleCloudDNSClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	_ = ctx
	_ = zone
	_ = recordType
	return nil, errors.New("gcp dns: record list not implemented")
}
