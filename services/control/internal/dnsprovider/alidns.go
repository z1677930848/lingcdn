package dnsprovider

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const aliEndpoint = "https://alidns.aliyuncs.com/"

// AliDNSClient implements minimal calls to AliDNS API.
type AliDNSClient struct {
	accessKeyID     string
	accessKeySecret string
	httpClient      *http.Client
}

type aliCommonResp struct {
	RequestID string `json:"RequestId"`
	Code      string `json:"Code,omitempty"`
	Message   string `json:"Message,omitempty"`
}

type describeDomainsResp struct {
	aliCommonResp
	TotalCount int `json:"TotalCount"`
	Domains    struct {
		Domain []struct {
			DomainName  string `json:"DomainName"`
			RecordCount int    `json:"RecordCount"`
		} `json:"Domain"`
	} `json:"Domains"`
}

// NewAliDNSClient creates a new AliDNS client.
func NewAliDNSClient(id, key, secret string) *AliDNSClient {
	if secret == "" {
		secret = key
	}
	return &AliDNSClient{
		accessKeyID:     id,
		accessKeySecret: secret,
		httpClient:      newHTTPClient(),
	}
}

// Ping validates credentials by calling DescribeDomains (page 1, size 1).
func (c *AliDNSClient) Ping(ctx context.Context) error {
	values := c.commonParams("DescribeDomains")
	values.Set("PageNumber", "1")
	values.Set("PageSize", "1")
	resp, err := c.doRequest(ctx, values)
	if err != nil {
		return err
	}
	var parsed describeDomainsResp
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return err
	}
	if parsed.Code != "" {
		return &ProviderError{Code: parsed.Code, Message: parsed.Message}
	}
	return nil
}

// ListProviderDomains returns all domain zones from the AliDNS account.
func (c *AliDNSClient) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	var all []ProviderDomain
	page := 1
	for {
		values := c.commonParams("DescribeDomains")
		values.Set("PageNumber", strconv.Itoa(page))
		values.Set("PageSize", "100")
		resp, err := c.doRequest(ctx, values)
		if err != nil {
			return nil, err
		}
		var parsed describeDomainsResp
		if err := json.Unmarshal(resp, &parsed); err != nil {
			return nil, err
		}
		if parsed.Code != "" {
			return nil, &ProviderError{Code: parsed.Code, Message: parsed.Message}
		}
		for _, d := range parsed.Domains.Domain {
			all = append(all, ProviderDomain{
				Name:        d.DomainName,
				RecordCount: d.RecordCount,
			})
		}
		if len(all) >= parsed.TotalCount || len(parsed.Domains.Domain) == 0 {
			break
		}
		page++
	}
	return all, nil
}

// Recover is a stub that simply validates credentials.
func (c *AliDNSClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "AliDNS credentials verified，未执行具体记录恢复（需业务规则）", nil
}

// Cleanup is a stub that simply validates credentials.
func (c *AliDNSClient) Cleanup(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return "AliDNS credentials verified，未执行实际删除（需明确删除规则）", nil
}

func (c *AliDNSClient) commonParams(action string) url.Values {
	values := url.Values{}
	values.Set("Format", "JSON")
	values.Set("Version", "2015-01-09")
	values.Set("AccessKeyId", c.accessKeyID)
	values.Set("SignatureMethod", "HMAC-SHA1")
	values.Set("Timestamp", time.Now().UTC().Format(time.RFC3339))
	values.Set("SignatureVersion", "1.0")
	values.Set("SignatureNonce", randomNonce())
	values.Set("Action", action)
	return values
}

func (c *AliDNSClient) doRequest(ctx context.Context, params url.Values) ([]byte, error) {
	sign := signAliRequest(c.accessKeySecret, params)
	params.Set("Signature", sign)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aliEndpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Bytes("body", body).Msg("alidns request failed")
	}
	return body, nil
}

func signAliRequest(secret string, params url.Values) string {
	// Sort parameters by key name
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build canonicalized query string: percentEncode(key)=percentEncode(value)
	var pairs []string
	for _, k := range keys {
		pairs = append(pairs, percentEncode(k)+"="+percentEncode(params.Get(k)))
	}
	canonicalized := strings.Join(pairs, "&")

	// StringToSign = HTTPMethod + "&" + percentEncode("/") + "&" + percentEncode(canonicalized)
	stringToSign := "GET&%2F&" + percentEncode(canonicalized)

	mac := hmac.New(sha1.New, []byte(secret+"&"))
	_, _ = mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func percentEncode(s string) string {
	s = url.QueryEscape(s)
	s = strings.ReplaceAll(s, "+", "%20")
	s = strings.ReplaceAll(s, "*", "%2A")
	return strings.ReplaceAll(s, "%7E", "~")
}

// ProviderError wraps provider error codes.
type ProviderError struct {
	Code    string
	Message string
}

func (e *ProviderError) Error() string {
	return e.Code + ": " + e.Message
}

// randomNonce generates a simple nonce.
func randomNonce() string {
	return strings.ReplaceAll(time.Now().Format("20060102150405.000000000"), ".", "")
}

type aliDescribeRecordsResp struct {
	aliCommonResp
	DomainRecords struct {
		Record []aliRecord `json:"Record"`
	} `json:"DomainRecords"`
}

type aliRecord struct {
	RecordID string `json:"RecordId"`
	RR       string `json:"RR"`
	Type     string `json:"Type"`
	Value    string `json:"Value"`
	TTL      int64  `json:"TTL"`
	Status   string `json:"Status"`
	Weight   int32  `json:"Weight"`
	Line     string `json:"Line"`
}

type aliAddRecordResp struct {
	aliCommonResp
	RecordID string `json:"RecordId"`
}

func (c *AliDNSClient) SupportsLine() bool {
	return true
}

func (c *AliDNSClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, line string, weights map[string]int32) (string, error) {
	zone = NormalizeZone(zone)
	rr := NormalizeName(name)
	line = normalizeAliLine(line)
	if zone == "" {
		return "", fmt.Errorf("alidns: zone is required")
	}
	if err := validateRecordValues(recordType, values); err != nil {
		return "", err
	}

	records, err := c.listRecords(ctx, zone, rr, recordType, line)
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

	// First pass: delete extras, update TTL on matches.
	for _, rec := range records {
		val := strings.Trim(strings.TrimSpace(rec.Value), ".")
		if _, ok := desired[val]; !ok {
			if err := c.deleteRecord(ctx, rec.RecordID); err != nil {
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
			if err := c.updateRecord(ctx, rec.RecordID, rr, recordType, val, ttl, line, targetWeight); err != nil {
				return "", err
			}
			updated++
		}
	}

	// Second pass: create missing.
	if len(desired) > 0 {
		for val := range desired {
			if present[val] {
				continue
			}
			if _, err := c.addRecord(ctx, zone, rr, recordType, val, ttl, line, desiredWeight[val]); err != nil {
				return "", err
			}
			created++
		}
	}

	return fmt.Sprintf("alidns ensured %s %s.%s (create=%d update=%d delete=%d)", recordType, rr, zone, created, updated, deleted), nil
}

func (c *AliDNSClient) listRecords(ctx context.Context, zone, rr string, recordType RecordType, line string) ([]aliRecord, error) {
	values := c.commonParams("DescribeDomainRecords")
	values.Set("DomainName", zone)
	values.Set("PageNumber", "1")
	values.Set("PageSize", "500")
	values.Set("SearchMode", "EXACT")
	if rr != "" && rr != "@" {
		values.Set("RRKeyWord", rr)
	} else {
		values.Set("RRKeyWord", "@")
	}
	values.Set("TypeKeyWord", string(recordType))

	resp, err := c.doRequest(ctx, values)
	if err != nil {
		return nil, err
	}
	var parsed aliDescribeRecordsResp
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, err
	}
	if parsed.Code != "" {
		return nil, &ProviderError{Code: parsed.Code, Message: parsed.Message}
	}

	out := make([]aliRecord, 0, len(parsed.DomainRecords.Record))
	for _, rec := range parsed.DomainRecords.Record {
		if !strings.EqualFold(strings.TrimSpace(rec.RR), rr) && !(rr == "@" && strings.TrimSpace(rec.RR) == "@") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rec.Type), string(recordType)) {
			continue
		}
		out = append(out, rec)
	}
	if strings.TrimSpace(line) != "" {
		target := normalizeAliLine(line)
		filtered := out[:0]
		for _, rec := range out {
			if strings.EqualFold(normalizeAliLine(rec.Line), target) {
				filtered = append(filtered, rec)
			}
		}
		out = filtered
	}
	return out, nil
}

func (c *AliDNSClient) addRecord(ctx context.Context, zone, rr string, recordType RecordType, value string, ttl int64, line string, weight int32) (string, error) {
	values := c.commonParams("AddDomainRecord")
	values.Set("DomainName", zone)
	values.Set("RR", rr)
	values.Set("Type", string(recordType))
	values.Set("Value", value)
	if strings.TrimSpace(line) != "" {
		values.Set("Line", line)
	}
	if ttl > 0 {
		values.Set("TTL", strconv.FormatInt(ttl, 10))
	}
	if weight > 0 {
		values.Set("Weight", strconv.FormatInt(int64(weight), 10))
	}

	resp, err := c.doRequest(ctx, values)
	if err != nil {
		return "", err
	}
	var parsed aliAddRecordResp
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", err
	}
	if parsed.Code != "" {
		return "", &ProviderError{Code: parsed.Code, Message: parsed.Message}
	}
	return parsed.RecordID, nil
}

func (c *AliDNSClient) updateRecord(ctx context.Context, recordID, rr string, recordType RecordType, value string, ttl int64, line string, weight int32) error {
	values := c.commonParams("UpdateDomainRecord")
	values.Set("RecordId", recordID)
	values.Set("RR", rr)
	values.Set("Type", string(recordType))
	values.Set("Value", value)
	if strings.TrimSpace(line) != "" {
		values.Set("Line", line)
	}
	if ttl > 0 {
		values.Set("TTL", strconv.FormatInt(ttl, 10))
	}
	if weight > 0 {
		values.Set("Weight", strconv.FormatInt(int64(weight), 10))
	}

	resp, err := c.doRequest(ctx, values)
	if err != nil {
		return err
	}
	var parsed aliCommonResp
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return err
	}
	if parsed.Code != "" {
		return &ProviderError{Code: parsed.Code, Message: parsed.Message}
	}
	return nil
}

func (c *AliDNSClient) deleteRecord(ctx context.Context, recordID string) error {
	values := c.commonParams("DeleteDomainRecord")
	values.Set("RecordId", recordID)

	resp, err := c.doRequest(ctx, values)
	if err != nil {
		return err
	}
	var parsed aliCommonResp
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return err
	}
	if parsed.Code != "" {
		return &ProviderError{Code: parsed.Code, Message: parsed.Message}
	}
	return nil
}

func (c *AliDNSClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	zone = NormalizeZone(zone)
	if zone == "" {
		return nil, fmt.Errorf("alidns: zone is required")
	}
	values := c.commonParams("DescribeDomainRecords")
	values.Set("DomainName", zone)
	values.Set("PageNumber", "1")
	values.Set("PageSize", "500")
	if recordType != "" {
		values.Set("TypeKeyWord", string(recordType))
	}

	resp, err := c.doRequest(ctx, values)
	if err != nil {
		return nil, err
	}
	var parsed aliDescribeRecordsResp
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, err
	}
	if parsed.Code != "" {
		return nil, &ProviderError{Code: parsed.Code, Message: parsed.Message}
	}
	out := make([]DNSRecord, 0, len(parsed.DomainRecords.Record))
	for _, r := range parsed.DomainRecords.Record {
		out = append(out, DNSRecord{
			Name:  strings.TrimSpace(r.RR) + "." + zone,
			Type:  RecordType(strings.TrimSpace(r.Type)),
			Value: strings.Trim(strings.TrimSpace(r.Value), "."),
			TTL:   r.TTL,
			Line:  aliLineToCanonical(r.Line),
		})
	}
	return out, nil
}

func normalizeAliLine(raw string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return "default"
	}
	switch strings.ToLower(line) {
	case "default", "telecom", "unicom", "mobile", "edu", "oversea", "search":
		return strings.ToLower(line)
	}
	switch line {
	case "默认", "全网", "全球":
		return "default"
	case "电信":
		return "telecom"
	case "联通":
		return "unicom"
	case "移动":
		return "mobile"
	case "教育网":
		return "edu"
	case "搜索引擎":
		return "search"
	case "境外", "海外":
		return "oversea"
	case "境内", "国内":
		return "default"
	default:
		return line
	}
}

func aliLineToCanonical(raw string) string {
	line := strings.TrimSpace(raw)
	switch strings.ToLower(line) {
	case "", "default":
		return "默认"
	case "telecom":
		return "电信"
	case "unicom":
		return "联通"
	case "mobile":
		return "移动"
	case "edu":
		return "教育网"
	case "search":
		return "搜索引擎"
	case "oversea":
		return "境外"
	default:
		return line
	}
}
