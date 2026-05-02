package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	cfAPIBase        = "https://api.cloudflare.com/client/v4"
	cfVerifyEndpoint = cfAPIBase + "/user/tokens/verify"
)

// CloudflareClient supports token verify + DNS record management (A/AAAA/CNAME).
type CloudflareClient struct {
	token      string
	apiKey     string
	email      string
	httpClient *http.Client

	mu     sync.Mutex
	zoneID map[string]string // zone name -> id
}

type cfResp[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result T `json:"result"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int64  `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

func NewCloudflareClient(token, apiKey, email string) *CloudflareClient {
	return &CloudflareClient{
		token:      strings.TrimSpace(token),
		apiKey:     strings.TrimSpace(apiKey),
		email:      strings.TrimSpace(email),
		httpClient: newHTTPClient(),
		zoneID:     make(map[string]string),
	}
}

func (c *CloudflareClient) authHeaders(req *http.Request) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
		return nil
	}
	if c.apiKey != "" && c.email != "" {
		req.Header.Set("X-Auth-Key", c.apiKey)
		req.Header.Set("X-Auth-Email", c.email)
		return nil
	}
	return errors.New("cloudflare token or apiKey+email required")
}

func (c *CloudflareClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfVerifyEndpoint, nil)
	if err != nil {
		return err
	}
	if err := c.authHeaders(req); err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := ioReadAll(resp.Body)
	var parsed cfResp[any]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("cloudflare: %d %s", parsed.Errors[0].Code, parsed.Errors[0].Message)
		}
		return errors.New("cloudflare: verify failed")
	}
	return nil
}

func (c *CloudflareClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "Cloudflare credentials verified", nil
}

func (c *CloudflareClient) Cleanup(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "Cloudflare credentials verified", nil
}

// ListProviderDomains returns all zones from the Cloudflare account.
func (c *CloudflareClient) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	var all []ProviderDomain
	page := 1
	for {
		u := fmt.Sprintf("%s/zones?page=%d&per_page=50", cfAPIBase, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		c.authHeaders(req)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var result struct {
			Success    bool `json:"success"`
			ResultInfo struct {
				TotalCount int `json:"total_count"`
			} `json:"result_info"`
			Result []struct {
				Name string `json:"name"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		for _, z := range result.Result {
			all = append(all, ProviderDomain{Name: z.Name})
		}
		if len(all) >= result.ResultInfo.TotalCount || len(result.Result) == 0 {
			break
		}
		page++
	}
	return all, nil
}

func (c *CloudflareClient) SupportsLine() bool {
	return false
}

func (c *CloudflareClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, _ string, _ map[string]int32) (string, error) {
	zone = NormalizeZone(zone)
	rr := NormalizeName(name)
	if zone == "" {
		return "", fmt.Errorf("cloudflare: zone is required")
	}
	if err := validateRecordValues(recordType, values); err != nil {
		return "", err
	}

	zoneID, err := c.getZoneID(ctx, zone)
	if err != nil {
		return "", err
	}

	fqdn := JoinFQDN(rr, zone)
	recordTypeStr := string(recordType)
	existing, err := c.listDNSRecords(ctx, zoneID, recordTypeStr, fqdn)
	if err != nil {
		return "", err
	}

	desired := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.Trim(strings.TrimSpace(v), ".")
		if v == "" {
			continue
		}
		desired[v] = struct{}{}
	}

	created := 0
	updated := 0
	deleted := 0
	present := make(map[string]bool, len(desired))

	cfTTL := normalizeCloudflareTTL(ttl)

	for _, rec := range existing {
		val := strings.Trim(strings.TrimSpace(rec.Content), ".")
		if _, ok := desired[val]; !ok {
			if err := c.deleteDNSRecord(ctx, zoneID, rec.ID); err != nil {
				return "", err
			}
			deleted++
			continue
		}
		present[val] = true
		if cfTTL > 0 && rec.TTL != cfTTL {
			if err := c.updateDNSRecord(ctx, zoneID, rec.ID, recordTypeStr, fqdn, val, cfTTL); err != nil {
				return "", err
			}
			updated++
		}
	}

	for val := range desired {
		if present[val] {
			continue
		}
		if err := c.createDNSRecord(ctx, zoneID, recordTypeStr, fqdn, val, cfTTL); err != nil {
			return "", err
		}
		created++
	}

	return fmt.Sprintf("cloudflare ensured %s %s (create=%d update=%d delete=%d)", recordType, fqdn, created, updated, deleted), nil
}

func normalizeCloudflareTTL(ttl int64) int64 {
	// 1 means "auto"; otherwise 120..86400
	if ttl < 0 {
		return 1
	}
	if ttl == 0 {
		return 1
	}
	if ttl < 120 {
		return 120
	}
	if ttl > 86400 {
		return 86400
	}
	return ttl
}

func (c *CloudflareClient) getZoneID(ctx context.Context, zone string) (string, error) {
	c.mu.Lock()
	if id, ok := c.zoneID[zone]; ok && id != "" {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	u, _ := url.Parse(cfAPIBase + "/zones")
	q := u.Query()
	q.Set("name", zone)
	q.Set("per_page", "50")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	if err := c.authHeaders(req); err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := ioReadAll(resp.Body)

	var parsed cfResp[[]cfZone]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return "", fmt.Errorf("cloudflare: %d %s", parsed.Errors[0].Code, parsed.Errors[0].Message)
		}
		return "", errors.New("cloudflare: list zones failed")
	}
	for _, z := range parsed.Result {
		if strings.EqualFold(z.Name, zone) {
			c.mu.Lock()
			c.zoneID[zone] = z.ID
			c.mu.Unlock()
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("cloudflare: zone not found: %s", zone)
}

func (c *CloudflareClient) listDNSRecords(ctx context.Context, zoneID, recordType, fqdn string) ([]cfDNSRecord, error) {
	u, _ := url.Parse(cfAPIBase + "/zones/" + zoneID + "/dns_records")
	q := u.Query()
	q.Set("type", recordType)
	q.Set("name", fqdn)
	q.Set("per_page", "100")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if err := c.authHeaders(req); err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := ioReadAll(resp.Body)

	var parsed cfResp[[]cfDNSRecord]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return nil, fmt.Errorf("cloudflare: %d %s", parsed.Errors[0].Code, parsed.Errors[0].Message)
		}
		return nil, errors.New("cloudflare: list records failed")
	}
	return parsed.Result, nil
}

func (c *CloudflareClient) createDNSRecord(ctx context.Context, zoneID, recordType, fqdn, content string, ttl int64) error {
	payload := map[string]any{
		"type":    recordType,
		"name":    fqdn,
		"content": content,
		"ttl":     ttl,
		"proxied": false,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfAPIBase+"/zones/"+zoneID+"/dns_records", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.authHeaders(req); err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := ioReadAll(resp.Body)
	var parsed cfResp[cfDNSRecord]
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("cloudflare: %d %s", parsed.Errors[0].Code, parsed.Errors[0].Message)
		}
		return errors.New("cloudflare: create record failed")
	}
	return nil
}

func (c *CloudflareClient) updateDNSRecord(ctx context.Context, zoneID, recordID, recordType, fqdn, content string, ttl int64) error {
	payload := map[string]any{
		"type":    recordType,
		"name":    fqdn,
		"content": content,
		"ttl":     ttl,
		"proxied": false,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, cfAPIBase+"/zones/"+zoneID+"/dns_records/"+recordID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.authHeaders(req); err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := ioReadAll(resp.Body)
	var parsed cfResp[cfDNSRecord]
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("cloudflare: %d %s", parsed.Errors[0].Code, parsed.Errors[0].Message)
		}
		return errors.New("cloudflare: update record failed")
	}
	return nil
}

func (c *CloudflareClient) deleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, cfAPIBase+"/zones/"+zoneID+"/dns_records/"+recordID, nil)
	if err != nil {
		return err
	}
	if err := c.authHeaders(req); err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := ioReadAll(resp.Body)
	var parsed cfResp[any]
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("cloudflare: %d %s", parsed.Errors[0].Code, parsed.Errors[0].Message)
		}
		return errors.New("cloudflare: delete record failed")
	}
	return nil
}

func (c *CloudflareClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	zone = NormalizeZone(zone)
	if zone == "" {
		return nil, fmt.Errorf("cloudflare: zone is required")
	}
	zoneID, err := c.getZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}
	rt := ""
	if recordType != "" {
		rt = string(recordType)
	}
	records, err := c.listDNSRecords(ctx, zoneID, rt, "")
	if err != nil {
		return nil, err
	}
	out := make([]DNSRecord, 0, len(records))
	for _, r := range records {
		out = append(out, DNSRecord{
			Name:  strings.TrimSuffix(strings.TrimSpace(r.Name), "."),
			Type:  RecordType(strings.TrimSpace(r.Type)),
			Value: strings.Trim(strings.TrimSpace(r.Content), "."),
			TTL:   r.TTL,
		})
	}
	return out, nil
}
