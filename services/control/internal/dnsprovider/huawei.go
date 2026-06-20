package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// HuaweiDNSClient manages records on Huawei Cloud DNS.
type HuaweiDNSClient struct {
	signer     *huaweiSigner
	httpClient *http.Client

	mu      sync.Mutex
	zoneIDs map[string]string
}

func NewHuaweiDNSClient(accessKey, secretKey string) *HuaweiDNSClient {
	return &HuaweiDNSClient{
		signer:     &huaweiSigner{ak: strings.TrimSpace(accessKey), sk: strings.TrimSpace(secretKey)},
		httpClient: newHTTPClient(),
		zoneIDs:    make(map[string]string),
	}
}

func (c *HuaweiDNSClient) Ping(ctx context.Context) error {
	_, err := c.doJSON(ctx, http.MethodGet, "/v2/zones", url.Values{"limit": {"1"}}, nil)
	return err
}

func (c *HuaweiDNSClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return syncVerifiedMessage("华为云 DNS"), nil
}

func (c *HuaweiDNSClient) Cleanup(ctx context.Context) (string, error) {
	return c.Recover(ctx)
}

func (c *HuaweiDNSClient) SupportsLine() bool { return false }

func (c *HuaweiDNSClient) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	var all []ProviderDomain
	marker := ""
	for {
		q := url.Values{"limit": {"100"}}
		if marker != "" {
			q.Set("marker", marker)
		}
		raw, err := c.doJSON(ctx, http.MethodGet, "/v2/zones", q, nil)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Zones []struct {
				Name string `json:"name"`
			} `json:"zones"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, err
		}
		for _, z := range resp.Zones {
			all = append(all, ProviderDomain{Name: strings.Trim(strings.TrimSpace(z.Name), ".")})
		}
		if resp.Links.Next == "" || len(resp.Zones) == 0 {
			break
		}
		if u, err := url.Parse(resp.Links.Next); err == nil {
			marker = u.Query().Get("marker")
		}
		if marker == "" {
			break
		}
	}
	return all, nil
}

func (c *HuaweiDNSClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, _ string, _ map[string]int32) (string, error) {
	zone = NormalizeZone(zone)
	rr := NormalizeName(name)
	if zone == "" {
		return "", fmt.Errorf("huawei dns: zone is required")
	}
	if err := validateRecordValues(recordType, values); err != nil {
		return "", err
	}

	zoneID, err := c.getZoneID(ctx, zone)
	if err != nil {
		return "", err
	}

	recordName := huaweiRecordName(rr, zone)
	desired := normalizeDesiredValues(values)
	targetTTL := normalizeHuaweiTTL(ttl)
	existing, err := c.listManagedRecords(ctx, zoneID, recordName, recordType)
	if err != nil {
		return "", err
	}

	existingSet := desiredValues(existing)
	sameValues := len(existingSet) == len(desired)
	if sameValues {
		for val := range desired {
			if _, ok := existingSet[val]; !ok {
				sameValues = false
				break
			}
		}
	}
	sameTTL := true
	if targetTTL > 0 && len(existing) > 0 {
		for _, rec := range existing {
			if rec.TTL != targetTTL {
				sameTTL = false
				break
			}
		}
	}
	if sameValues && sameTTL {
		return formatEnsureMessage("huawei", recordType, recordName, EnsureResult{}), nil
	}

	res := EnsureResult{}
	for val := range existingSet {
		if _, ok := desired[val]; !ok {
			res.Deleted++
		}
	}
	for val := range desired {
		if _, ok := existingSet[val]; !ok {
			res.Created++
		}
	}
	if !sameTTL && sameValues {
		res.Updated = len(desired)
	}

	recordsetID := ""
	if len(existing) > 0 {
		recordsetID = existing[0].ID
	}
	if len(desired) == 0 {
		if recordsetID == "" {
			return formatEnsureMessage("huawei", recordType, recordName, EnsureResult{}), nil
		}
		if err := c.deleteRecordSet(ctx, zoneID, recordsetID); err != nil {
			return "", err
		}
		return formatEnsureMessage("huawei", recordType, recordName, res), nil
	}

	records := valuesFromDesired(desired)
	if recordsetID == "" {
		if err := c.createRecordSet(ctx, zoneID, recordName, recordType, records, targetTTL); err != nil {
			return "", err
		}
	} else {
		if err := c.updateRecordSet(ctx, zoneID, recordsetID, recordName, recordType, records, targetTTL); err != nil {
			return "", err
		}
	}
	return formatEnsureMessage("huawei", recordType, recordName, res), nil
}

func (c *HuaweiDNSClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	zone = NormalizeZone(zone)
	if zone == "" {
		return nil, fmt.Errorf("huawei dns: zone is required")
	}
	zoneID, err := c.getZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	q := url.Values{"limit": {"500"}}
	if recordType != "" {
		q.Set("type", string(recordType))
	}
	raw, err := c.doJSON(ctx, http.MethodGet, "/v2/zones/"+zoneID+"/recordsets", q, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Recordsets []struct {
			Name    string   `json:"name"`
			Type    string   `json:"type"`
			TTL     int64    `json:"ttl"`
			Records []string `json:"records"`
		} `json:"recordsets"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}

	out := make([]DNSRecord, 0)
	for _, set := range resp.Recordsets {
		fqdn := strings.Trim(strings.TrimSpace(set.Name), ".")
		rr, ok := SplitByZone(fqdn, zone)
		if !ok {
			continue
		}
		display := zone
		if rr != "@" {
			display = rr + "." + zone
		}
		for _, val := range set.Records {
			out = append(out, DNSRecord{
				Name:  display,
				Type:  RecordType(strings.TrimSpace(set.Type)),
				Value: strings.Trim(strings.TrimSpace(val), "."),
				TTL:   set.TTL,
			})
		}
	}
	return out, nil
}

func (c *HuaweiDNSClient) getZoneID(ctx context.Context, zone string) (string, error) {
	c.mu.Lock()
	if id, ok := c.zoneIDs[zone]; ok && id != "" {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	q := url.Values{"name": {zone + "."}, "type": {"public"}}
	raw, err := c.doJSON(ctx, http.MethodGet, "/v2/zones", q, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"zones"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	for _, z := range resp.Zones {
		name := strings.Trim(strings.TrimSpace(z.Name), ".")
		if strings.EqualFold(name, zone) {
			c.mu.Lock()
			c.zoneIDs[zone] = z.ID
			c.mu.Unlock()
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("huawei dns: zone not found: %s", zone)
}

func (c *HuaweiDNSClient) listManagedRecords(ctx context.Context, zoneID, recordName string, recordType RecordType) ([]ManagedRecord, error) {
	q := url.Values{
		"name":  {recordName},
		"type":  {string(recordType)},
		"limit": {"100"},
	}
	raw, err := c.doJSON(ctx, http.MethodGet, "/v2/zones/"+zoneID+"/recordsets", q, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Recordsets []struct {
			ID      string   `json:"id"`
			TTL     int64    `json:"ttl"`
			Records []string `json:"records"`
		} `json:"recordsets"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}

	var out []ManagedRecord
	for _, set := range resp.Recordsets {
		for _, val := range set.Records {
			out = append(out, ManagedRecord{
				ID:    set.ID,
				Value: strings.Trim(strings.TrimSpace(val), "."),
				TTL:   set.TTL,
			})
		}
	}
	return out, nil
}

func (c *HuaweiDNSClient) createRecordSet(ctx context.Context, zoneID, recordName string, recordType RecordType, records []string, ttl int64) error {
	body := map[string]any{
		"name":        recordName,
		"type":        string(recordType),
		"ttl":         ttl,
		"records":     records,
		"description": "managed by lingcdn",
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/v2.1/zones/"+zoneID+"/recordsets", nil, body)
	return err
}

func (c *HuaweiDNSClient) updateRecordSet(ctx context.Context, zoneID, recordsetID, recordName string, recordType RecordType, records []string, ttl int64) error {
	body := map[string]any{
		"name":    recordName,
		"type":    string(recordType),
		"ttl":     ttl,
		"records": records,
	}
	_, err := c.doJSON(ctx, http.MethodPut, "/v2.1/zones/"+zoneID+"/recordsets/"+recordsetID, nil, body)
	return err
}

func (c *HuaweiDNSClient) deleteRecordSet(ctx context.Context, zoneID, recordsetID string) error {
	_, err := c.doJSON(ctx, http.MethodDelete, "/v2.1/zones/"+zoneID+"/recordsets/"+recordsetID, nil, nil)
	return err
}

func (c *HuaweiDNSClient) doJSON(ctx context.Context, method, path string, query url.Values, payload map[string]any) ([]byte, error) {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	raw, status, err := c.signer.do(ctx, c.httpClient, method, path, query, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		var apiErr struct {
			ErrorCode string `json:"error_code"`
			Message   string `json:"message"`
		}
		_ = json.Unmarshal(raw, &apiErr)
		if apiErr.Message != "" {
			return nil, fmt.Errorf("huawei dns: %s: %s", apiErr.ErrorCode, apiErr.Message)
		}
		return nil, fmt.Errorf("huawei dns: HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func huaweiRecordName(rr, zone string) string {
	if rr == "@" {
		return zone + "."
	}
	return rr + "." + zone + "."
}

func normalizeHuaweiTTL(ttl int64) int64 {
	if ttl <= 0 {
		return 300
	}
	if ttl < 60 {
		return 60
	}
	if ttl > 86400 {
		return 86400
	}
	return ttl
}

func valuesFromDesired(desired map[string]struct{}) []string {
	out := make([]string, 0, len(desired))
	for val := range desired {
		out = append(out, val)
	}
	return out
}
