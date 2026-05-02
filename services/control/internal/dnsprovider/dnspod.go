package dnsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	dnspodDomainListEndpoint   = "https://dnsapi.cn/Domain.List"
	dnspodRecordListEndpoint   = "https://dnsapi.cn/Record.List"
	dnspodRecordCreateEndpoint = "https://dnsapi.cn/Record.Create"
	dnspodRecordModifyEndpoint = "https://dnsapi.cn/Record.Modify"
	dnspodRecordRemoveEndpoint = "https://dnsapi.cn/Record.Remove"
)

// DNSPodClient uses DNSPod token (id,token) to validate and manage records.
type DNSPodClient struct {
	loginToken string
	httpClient *http.Client
}

type dnspodStatus struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type dnspodResp struct {
	Status dnspodStatus `json:"status"`
}

type dnspodRecord struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	TTL    int64  `json:"ttl,string"`
	Weight int32  `json:"weight,string"`
	Line   string `json:"line"`
}

type dnspodRecordListResp struct {
	Status  dnspodStatus   `json:"status"`
	Records []dnspodRecord `json:"records"`
}

type dnspodRecordMutationResp struct {
	Status dnspodStatus `json:"status"`
	Record struct {
		ID string `json:"id"`
	} `json:"record"`
}

// NewDNSPodClient creates a DNSPod client.
// accountID + token or combined token string "id,token".
func NewDNSPodClient(accountID, token string) *DNSPodClient {
	loginToken := strings.TrimSpace(token)
	if strings.TrimSpace(accountID) != "" {
		loginToken = strings.TrimSpace(accountID) + "," + strings.TrimSpace(token)
	}
	return &DNSPodClient{
		loginToken: loginToken,
		httpClient: newHTTPClient(),
	}
}

func (c *DNSPodClient) Ping(ctx context.Context) error {
	if c.loginToken == "" {
		return errors.New("dnspod token missing")
	}
	values := url.Values{}
	values.Set("login_token", c.loginToken)
	values.Set("format", "json")
	values.Set("length", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dnspodDomainListEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := ioReadAll(resp.Body)

	var parsed dnspodResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if parsed.Status.Code != "1" {
		return &ProviderError{Code: parsed.Status.Code, Message: parsed.Status.Message}
	}
	return nil
}

func (c *DNSPodClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "DNSPod credentials verified", nil
}

func (c *DNSPodClient) Cleanup(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "DNSPod credentials verified", nil
}

// ListProviderDomains returns all domain zones from the DNSPod account.
func (c *DNSPodClient) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	values := url.Values{}
	values.Set("login_token", c.loginToken)
	values.Set("format", "json")
	values.Set("length", "3000")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dnspodDomainListEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := ioReadAll(resp.Body)

	var parsed struct {
		Status  dnspodStatus `json:"status"`
		Domains []struct {
			Name    string `json:"name"`
			Records string `json:"records"`
		} `json:"domains"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Status.Code != "1" {
		return nil, &ProviderError{Code: parsed.Status.Code, Message: parsed.Status.Message}
	}
	var result []ProviderDomain
	for _, d := range parsed.Domains {
		cnt, _ := strconv.Atoi(d.Records)
		result = append(result, ProviderDomain{Name: d.Name, RecordCount: cnt})
	}
	return result, nil
}

func (c *DNSPodClient) SupportsLine() bool {
	return true
}

func (c *DNSPodClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, line string, weights map[string]int32) (string, error) {
	zone = NormalizeZone(zone)
	sub := NormalizeName(name)
	line = normalizeDNSPodLine(line)
	if zone == "" {
		return "", fmt.Errorf("dnspod: zone is required")
	}
	if err := validateRecordValues(recordType, values); err != nil {
		return "", err
	}

	existing, err := c.listRecords(ctx, zone, sub, recordType, line)
	if err != nil {
		return "", err
	}

	desired := make(map[string]struct{}, len(values))
	desiredWeight := make(map[string]int32, len(values))
	for _, v := range values {
		v = strings.Trim(strings.TrimSpace(v), ".")
		if v == "" {
			continue
		}
		desired[v] = struct{}{}
		w := int32(1)
		if weights != nil {
			if val := weights[v]; val > 0 {
				w = val
			}
		}
		desiredWeight[v] = w
	}

	created := 0
	updated := 0
	deleted := 0
	present := make(map[string]bool, len(desired))

	for _, rec := range existing {
		val := strings.Trim(strings.TrimSpace(rec.Value), ".")
		if _, ok := desired[val]; !ok {
			if err := c.removeRecord(ctx, zone, rec.ID); err != nil {
				return "", err
			}
			deleted++
			continue
		}
		present[val] = true
		targetWeight := desiredWeight[val]
		currentWeight := rec.Weight
		if currentWeight <= 0 {
			currentWeight = 1
		}
		needWeight := weights != nil && targetWeight > 0 && currentWeight != targetWeight
		if (ttl > 0 && rec.TTL != ttl) || needWeight {
			if err := c.modifyRecord(ctx, zone, rec.ID, sub, recordType, val, ttl, line, targetWeight); err != nil {
				return "", err
			}
			updated++
		}
	}

	for val := range desired {
		if present[val] {
			continue
		}
		if _, err := c.createRecord(ctx, zone, sub, recordType, val, ttl, line, desiredWeight[val]); err != nil {
			return "", err
		}
		created++
	}

	return fmt.Sprintf("dnspod ensured %s %s.%s (create=%d update=%d delete=%d)", recordType, sub, zone, created, updated, deleted), nil
}

func (c *DNSPodClient) doForm(ctx context.Context, endpoint string, values url.Values, out any) error {
	if c.loginToken == "" {
		return errors.New("dnspod token missing")
	}
	values.Set("login_token", c.loginToken)
	values.Set("format", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := ioReadAll(resp.Body)
	return json.Unmarshal(body, out)
}

func (c *DNSPodClient) listRecords(ctx context.Context, zone, sub string, recordType RecordType, line string) ([]dnspodRecord, error) {
	values := url.Values{}
	values.Set("domain", zone)
	values.Set("sub_domain", sub)
	values.Set("record_type", string(recordType))
	if strings.TrimSpace(line) != "" {
		values.Set("record_line", line)
	}
	values.Set("length", "3000")
	values.Set("offset", "0")
	var parsed dnspodRecordListResp
	if err := c.doForm(ctx, dnspodRecordListEndpoint, values, &parsed); err != nil {
		return nil, err
	}
	if parsed.Status.Code != "1" {
		return nil, &ProviderError{Code: parsed.Status.Code, Message: parsed.Status.Message}
	}
	return parsed.Records, nil
}

func (c *DNSPodClient) createRecord(ctx context.Context, zone, sub string, recordType RecordType, value string, ttl int64, line string, weight int32) (string, error) {
	values := url.Values{}
	values.Set("domain", zone)
	values.Set("sub_domain", sub)
	values.Set("record_type", string(recordType))
	values.Set("record_line", line)
	values.Set("value", value)
	if ttl > 0 {
		values.Set("ttl", strconv.FormatInt(ttl, 10))
	}
	if weight > 0 {
		values.Set("weight", strconv.FormatInt(int64(weight), 10))
	}
	var parsed dnspodRecordMutationResp
	if err := c.doForm(ctx, dnspodRecordCreateEndpoint, values, &parsed); err != nil {
		return "", err
	}
	if parsed.Status.Code != "1" {
		return "", &ProviderError{Code: parsed.Status.Code, Message: parsed.Status.Message}
	}
	return parsed.Record.ID, nil
}

func (c *DNSPodClient) modifyRecord(ctx context.Context, zone, recordID, sub string, recordType RecordType, value string, ttl int64, line string, weight int32) error {
	values := url.Values{}
	values.Set("domain", zone)
	values.Set("record_id", recordID)
	values.Set("sub_domain", sub)
	values.Set("record_type", string(recordType))
	values.Set("record_line", line)
	values.Set("value", value)
	if ttl > 0 {
		values.Set("ttl", strconv.FormatInt(ttl, 10))
	}
	if weight > 0 {
		values.Set("weight", strconv.FormatInt(int64(weight), 10))
	}
	var parsed dnspodRecordMutationResp
	if err := c.doForm(ctx, dnspodRecordModifyEndpoint, values, &parsed); err != nil {
		return err
	}
	if parsed.Status.Code != "1" {
		return &ProviderError{Code: parsed.Status.Code, Message: parsed.Status.Message}
	}
	return nil
}

func (c *DNSPodClient) removeRecord(ctx context.Context, zone, recordID string) error {
	values := url.Values{}
	values.Set("domain", zone)
	values.Set("record_id", recordID)
	var parsed dnspodRecordMutationResp
	if err := c.doForm(ctx, dnspodRecordRemoveEndpoint, values, &parsed); err != nil {
		return err
	}
	if parsed.Status.Code != "1" {
		return &ProviderError{Code: parsed.Status.Code, Message: parsed.Status.Message}
	}
	return nil
}

func (c *DNSPodClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	zone = NormalizeZone(zone)
	if zone == "" {
		return nil, fmt.Errorf("dnspod: zone is required")
	}
	values := url.Values{}
	values.Set("domain", zone)
	if recordType != "" {
		values.Set("record_type", string(recordType))
	}
	values.Set("length", "3000")
	values.Set("offset", "0")
	var parsed dnspodRecordListResp
	if err := c.doForm(ctx, dnspodRecordListEndpoint, values, &parsed); err != nil {
		return nil, err
	}
	if parsed.Status.Code != "1" {
		return nil, &ProviderError{Code: parsed.Status.Code, Message: parsed.Status.Message}
	}
	out := make([]DNSRecord, 0, len(parsed.Records))
	for _, r := range parsed.Records {
		out = append(out, DNSRecord{
			Name:  strings.TrimSpace(r.Name) + "." + zone,
			Type:  RecordType(strings.TrimSpace(r.Type)),
			Value: strings.Trim(strings.TrimSpace(r.Value), "."),
			TTL:   r.TTL,
			Line:  normalizeDNSPodLine(r.Line),
		})
	}
	return out, nil
}

func normalizeDNSPodLine(raw string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return "默认"
	}
	switch strings.ToLower(line) {
	case "default":
		return "默认"
	}
	switch line {
	case "全网", "全球":
		return "默认"
	case "国内":
		return "境内"
	case "海外":
		return "境外"
	default:
		return line
	}
}
