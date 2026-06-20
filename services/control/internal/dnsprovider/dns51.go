package dnsprovider

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const dns51APIBase = "https://www.51dns.com/api"

// DNS51Client manages records on 51DNS (帝恩思).
type DNS51Client struct {
	apiKey     string
	apiSecret  string
	httpClient *http.Client

	mu        sync.Mutex
	domainIDs map[string]int
}

type dns51Resp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func NewDNS51Client(apiKey, apiSecret string) *DNS51Client {
	if apiSecret == "" {
		apiSecret = apiKey
	}
	return &DNS51Client{
		apiKey:     strings.TrimSpace(apiKey),
		apiSecret:  strings.TrimSpace(apiSecret),
		httpClient: newHTTPClient(),
		domainIDs:  make(map[string]int),
	}
}

func (c *DNS51Client) Ping(ctx context.Context) error {
	_, err := c.call(ctx, "domain/list/", url.Values{"page": {"1"}, "pageSize": {"1"}})
	return err
}

func (c *DNS51Client) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return syncVerifiedMessage("51DNS"), nil
}

func (c *DNS51Client) Cleanup(ctx context.Context) (string, error) {
	return c.Recover(ctx)
}

func (c *DNS51Client) SupportsLine() bool { return false }

func (c *DNS51Client) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	var all []ProviderDomain
	page := 1
	for {
		raw, err := c.call(ctx, "domain/list/", url.Values{
			"page":     {strconv.Itoa(page)},
			"pageSize": {"100"},
		})
		if err != nil {
			return nil, err
		}
		var data struct {
			Data []struct {
				DomainID   int    `json:"domainID"`
				DomainName string `json:"domain"`
			} `json:"data"`
			PageCount int `json:"pageCount"`
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		for _, d := range data.Data {
			all = append(all, ProviderDomain{Name: strings.TrimSpace(d.DomainName)})
		}
		if page >= data.PageCount || len(data.Data) == 0 {
			break
		}
		page++
	}
	return all, nil
}

func (c *DNS51Client) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, _ string, _ map[string]int32) (string, error) {
	zone = NormalizeZone(zone)
	rr := NormalizeName(name)
	if zone == "" {
		return "", fmt.Errorf("51dns: zone is required")
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

	targetTTL := normalizeDNS51TTL(ttl)
	res, err := EnsureManagedRecords(existing, desired, targetTTL,
		func(id string) error { return c.deleteRecord(ctx, domainID, id) },
		func(id, val string) error { return c.editRecord(ctx, domainID, id, host, recordType, val, targetTTL) },
		func(val string) error { return c.createRecord(ctx, domainID, host, recordType, val, targetTTL) },
	)
	if err != nil {
		return "", err
	}
	return formatEnsureMessage("51dns", recordType, JoinFQDN(rr, zone), res), nil
}

func (c *DNS51Client) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	zone = NormalizeZone(zone)
	if zone == "" {
		return nil, fmt.Errorf("51dns: zone is required")
	}
	domainID, err := c.getDomainID(ctx, zone)
	if err != nil {
		return nil, err
	}

	page := 1
	var out []DNSRecord
	for {
		params := url.Values{
			"domainID": {strconv.Itoa(domainID)},
			"page":     {strconv.Itoa(page)},
			"pageSize": {"100"},
		}
		if recordType != "" {
			params.Set("type", string(recordType))
		}
		raw, err := c.call(ctx, "record/search/", params)
		if err != nil {
			return nil, err
		}
		var data struct {
			Data struct {
				Data []struct {
					Record   string `json:"record"`
					Type     string `json:"type"`
					Value    string `json:"value"`
					TTL      int64  `json:"TTL"`
				} `json:"data"`
			} `json:"data"`
			PageCount int `json:"pageCount"`
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		for _, r := range data.Data.Data {
			host := strings.TrimSpace(r.Record)
			display := zone
			if host != "" && host != "@" {
				display = host + "." + zone
			}
			out = append(out, DNSRecord{
				Name:  display,
				Type:  RecordType(strings.TrimSpace(r.Type)),
				Value: strings.Trim(strings.TrimSpace(r.Value), "."),
				TTL:   r.TTL,
			})
		}
		if page >= data.PageCount || len(data.Data.Data) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (c *DNS51Client) getDomainID(ctx context.Context, zone string) (int, error) {
	c.mu.Lock()
	if id, ok := c.domainIDs[zone]; ok && id > 0 {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	page := 1
	target := strings.ToLower(zone)
	for {
		raw, err := c.call(ctx, "domain/list/", url.Values{
			"page":     {strconv.Itoa(page)},
			"pageSize": {"100"},
		})
		if err != nil {
			return 0, err
		}
		var data struct {
			Data []struct {
				DomainID   int    `json:"domainID"`
				DomainName string `json:"domain"`
			} `json:"data"`
			PageCount int `json:"pageCount"`
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return 0, err
		}
		for _, d := range data.Data {
			if strings.EqualFold(strings.TrimSpace(d.DomainName), target) {
				c.mu.Lock()
				c.domainIDs[zone] = d.DomainID
				c.mu.Unlock()
				return d.DomainID, nil
			}
		}
		if page >= data.PageCount || len(data.Data) == 0 {
			break
		}
		page++
	}
	return 0, fmt.Errorf("51dns: domain not found: %s", zone)
}

type dns51Record struct {
	RecordID int    `json:"recordID"`
	Record   string `json:"record"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int64  `json:"TTL"`
}

func (c *DNS51Client) listManagedRecords(ctx context.Context, domainID int, host string, recordType RecordType) ([]ManagedRecord, error) {
	params := url.Values{
		"domainID": {strconv.Itoa(domainID)},
		"host":     {host},
		"type":     {string(recordType)},
		"page":     {"1"},
		"pageSize": {"100"},
	}
	raw, err := c.call(ctx, "record/search/", params)
	if err != nil {
		return nil, err
	}
	var data struct {
		Data struct {
			Data []dns51Record `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	out := make([]ManagedRecord, 0, len(data.Data.Data))
	for _, r := range data.Data.Data {
		out = append(out, ManagedRecord{
			ID:    strconv.Itoa(r.RecordID),
			Value: strings.Trim(strings.TrimSpace(r.Value), "."),
			TTL:   r.TTL,
		})
	}
	return out, nil
}

func (c *DNS51Client) createRecord(ctx context.Context, domainID int, host string, recordType RecordType, value string, ttl int64) error {
	params := url.Values{
		"domainID": {strconv.Itoa(domainID)},
		"host":     {host},
		"type":     {string(recordType)},
		"value":    {value},
		"TTL":      {strconv.FormatInt(ttl, 10)},
	}
	_, err := c.call(ctx, "record/create/", params)
	return err
}

func (c *DNS51Client) editRecord(ctx context.Context, domainID int, recordID, host string, recordType RecordType, value string, ttl int64) error {
	params := url.Values{
		"domainID": {strconv.Itoa(domainID)},
		"recordID": {recordID},
		"host":     {host},
		"type":     {string(recordType)},
		"value":    {value},
		"TTL":      {strconv.FormatInt(ttl, 10)},
	}
	_, err := c.call(ctx, "record/edit/", params)
	return err
}

func (c *DNS51Client) deleteRecord(ctx context.Context, domainID int, recordID string) error {
	params := url.Values{
		"domainID": {strconv.Itoa(domainID)},
		"recordID": {recordID},
	}
	_, err := c.call(ctx, "record/remove/", params)
	return err
}

func (c *DNS51Client) call(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("apiKey", c.apiKey)
	params.Set("timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	params.Set("hash", dns51Sign(params, c.apiSecret))

	endpoint := dns51APIBase + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed dns51Resp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("51dns: invalid response: %w", err)
	}
	if parsed.Code != 0 {
		msg := strings.TrimSpace(parsed.Message)
		if msg == "" {
			msg = fmt.Sprintf("code %d", parsed.Code)
		}
		return nil, fmt.Errorf("51dns: %s", msg)
	}
	return body, nil
}

func dns51Sign(params url.Values, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+params.Get(k))
	}
	h := md5.Sum([]byte(strings.Join(parts, "&") + secret))
	return hex.EncodeToString(h[:])
}

func normalizeDNS51TTL(ttl int64) int64 {
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
