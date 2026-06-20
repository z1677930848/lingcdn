package dnsprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

const dnslaAPIBase = "https://api.dns.la/api"

// DNSLAClient manages records on DNS.LA (帝恩爱斯).
type DNSLAClient struct {
	apiID      string
	apiSecret  string
	authHeader string
	httpClient *http.Client

	mu        sync.Mutex
	domainIDs map[string]string
}

func NewDNSLAClient(apiID, apiSecret string) *DNSLAClient {
	id := strings.TrimSpace(apiID)
	secret := strings.TrimSpace(apiSecret)
	if secret == "" {
		secret = id
	}
	token := base64.StdEncoding.EncodeToString([]byte(id + ":" + secret))
	return &DNSLAClient{
		apiID:      id,
		apiSecret:  secret,
		authHeader: "Basic " + token,
		httpClient: newHTTPClient(),
		domainIDs:  make(map[string]string),
	}
}

func (c *DNSLAClient) Ping(ctx context.Context) error {
	_, err := c.get(ctx, "domainList?pageIndex=1&pageSize=1")
	return err
}

func (c *DNSLAClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return syncVerifiedMessage("DNS.LA"), nil
}

func (c *DNSLAClient) Cleanup(ctx context.Context) (string, error) {
	return c.Recover(ctx)
}

func (c *DNSLAClient) SupportsLine() bool { return false }

func (c *DNSLAClient) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	var all []ProviderDomain
	page := 1
	for {
		raw, err := c.get(ctx, fmt.Sprintf("domainList?pageIndex=%d&pageSize=100", page))
		if err != nil {
			return nil, err
		}
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Data []struct {
					Domain string `json:"domain"`
				} `json:"data"`
				TotalPage int `json:"totalPage"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, err
		}
		for _, d := range resp.Data.Data {
			all = append(all, ProviderDomain{Name: strings.TrimSpace(d.Domain)})
		}
		if page >= resp.Data.TotalPage || len(resp.Data.Data) == 0 {
			break
		}
		page++
	}
	return all, nil
}

func (c *DNSLAClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, _ string, _ map[string]int32) (string, error) {
	zone = NormalizeZone(zone)
	rr := NormalizeName(name)
	if zone == "" {
		return "", fmt.Errorf("dns.la: zone is required")
	}
	if err := validateRecordValues(recordType, values); err != nil {
		return "", err
	}

	domainID, err := c.getDomainID(ctx, zone)
	if err != nil {
		return "", err
	}
	host := rr
	if host == "@" {
		host = "@"
	}

	desired := normalizeDesiredValues(values)
	existing, err := c.listManagedRecords(ctx, domainID, host, recordType)
	if err != nil {
		return "", err
	}

	targetTTL := normalizeDNSLATTL(ttl)
	res, err := EnsureManagedRecords(existing, desired, targetTTL,
		func(id string) error { return c.deleteRecord(ctx, id) },
		func(id, val string) error { return c.updateRecord(ctx, domainID, id, host, recordType, val, targetTTL) },
		func(val string) error { return c.createRecord(ctx, domainID, host, recordType, val, targetTTL) },
	)
	if err != nil {
		return "", err
	}
	return formatEnsureMessage("dns.la", recordType, JoinFQDN(rr, zone), res), nil
}

func (c *DNSLAClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	zone = NormalizeZone(zone)
	if zone == "" {
		return nil, fmt.Errorf("dns.la: zone is required")
	}
	domainID, err := c.getDomainID(ctx, zone)
	if err != nil {
		return nil, err
	}

	page := 1
	var out []DNSRecord
	for {
		q := fmt.Sprintf("recordList?pageIndex=%d&pageSize=100&domainId=%s", page, url.QueryEscape(domainID))
		if recordType != "" {
			q += "&type=" + url.QueryEscape(strconv.Itoa(dnslaRecordTypeEnum(recordType)))
		}
		raw, err := c.get(ctx, q)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Data []struct {
					Host string `json:"host"`
					Type int    `json:"type"`
					Data string `json:"data"`
					TTL  int64  `json:"ttl"`
				} `json:"data"`
				TotalPage int `json:"totalPage"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Data.Data {
			host := strings.TrimSpace(r.Host)
			display := zone
			if host != "" && host != "@" {
				display = host + "." + zone
			}
			out = append(out, DNSRecord{
				Name:  display,
				Type:  dnslaEnumToRecordType(r.Type),
				Value: strings.Trim(strings.TrimSpace(r.Data), "."),
				TTL:   r.TTL,
			})
		}
		if page >= resp.Data.TotalPage || len(resp.Data.Data) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (c *DNSLAClient) getDomainID(ctx context.Context, zone string) (string, error) {
	c.mu.Lock()
	if id, ok := c.domainIDs[zone]; ok && id != "" {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	raw, err := c.get(ctx, "domain?domain="+url.QueryEscape(zone))
	if err != nil {
		return "", err
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if resp.Data.ID == "" {
		return "", fmt.Errorf("dns.la: domain not found: %s", zone)
	}
	c.mu.Lock()
	c.domainIDs[zone] = resp.Data.ID
	c.mu.Unlock()
	return resp.Data.ID, nil
}

func (c *DNSLAClient) listManagedRecords(ctx context.Context, domainID, host string, recordType RecordType) ([]ManagedRecord, error) {
	q := fmt.Sprintf("recordList?pageIndex=1&pageSize=100&domainId=%s&host=%s&type=%d",
		url.QueryEscape(domainID), url.QueryEscape(host), dnslaRecordTypeEnum(recordType))
	raw, err := c.get(ctx, q)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Data []struct {
				ID   string `json:"id"`
				Data string `json:"data"`
				TTL  int64  `json:"ttl"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	out := make([]ManagedRecord, 0, len(resp.Data.Data))
	for _, r := range resp.Data.Data {
		out = append(out, ManagedRecord{
			ID:    r.ID,
			Value: strings.Trim(strings.TrimSpace(r.Data), "."),
			TTL:   r.TTL,
		})
	}
	return out, nil
}

func (c *DNSLAClient) createRecord(ctx context.Context, domainID, host string, recordType RecordType, value string, ttl int64) error {
	payload := map[string]any{
		"domainId": domainID,
		"type":     dnslaRecordTypeEnum(recordType),
		"host":     host,
		"data":     value,
		"ttl":      ttl,
	}
	_, err := c.post(ctx, "record", payload, http.MethodPost)
	return err
}

func (c *DNSLAClient) updateRecord(ctx context.Context, domainID, recordID, host string, recordType RecordType, value string, ttl int64) error {
	payload := map[string]any{
		"id":       recordID,
		"domainId": domainID,
		"type":     dnslaRecordTypeEnum(recordType),
		"host":     host,
		"data":     value,
		"ttl":      ttl,
	}
	_, err := c.post(ctx, "record", payload, http.MethodPut)
	return err
}

func (c *DNSLAClient) deleteRecord(ctx context.Context, recordID string) error {
	_, err := c.post(ctx, "record?id="+url.QueryEscape(recordID), nil, http.MethodDelete)
	return err
}

func (c *DNSLAClient) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dnslaAPIBase+"/"+strings.TrimPrefix(path, "/"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return c.parseResponse(body)
}

func (c *DNSLAClient) post(ctx context.Context, path string, payload map[string]any, method string) ([]byte, error) {
	if method == "" {
		method = http.MethodPost
	}
	var body io.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, dnslaAPIBase+"/"+strings.TrimPrefix(path, "/"), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return c.parseResponse(raw)
}

func (c *DNSLAClient) parseResponse(body []byte) ([]byte, error) {
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("dns.la: invalid response: %w", err)
	}
	if resp.Code != 200 {
		msg := strings.TrimSpace(resp.Msg)
		if msg == "" {
			msg = fmt.Sprintf("code %d", resp.Code)
		}
		return nil, fmt.Errorf("dns.la: %s", msg)
	}
	return body, nil
}

func dnslaRecordTypeEnum(recordType RecordType) int {
	switch recordType {
	case RecordTypeA:
		return 1
	case RecordTypeCNAME:
		return 5
	case RecordTypeAAAA:
		return 28
	default:
		return 1
	}
}

func dnslaEnumToRecordType(v int) RecordType {
	switch v {
	case 1:
		return RecordTypeA
	case 5:
		return RecordTypeCNAME
	case 28:
		return RecordTypeAAAA
	default:
		return RecordType(fmt.Sprintf("TYPE%d", v))
	}
}

func normalizeDNSLATTL(ttl int64) int64 {
	if ttl <= 0 {
		return 600
	}
	if ttl < 60 {
		return 60
	}
	if ttl > 86400 {
		return 86400
	}
	return ttl
}
