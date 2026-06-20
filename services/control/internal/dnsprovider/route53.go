package dnsprovider

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Route53Client manages DNS records via Amazon Route53.
type Route53Client struct {
	signer     *awsSigner
	httpClient *http.Client

	mu      sync.Mutex
	zoneIDs map[string]string
}

type route53HostedZones struct {
	XMLName     xml.Name `xml:"ListHostedZonesResponse"`
	HostedZones []struct {
		ID   string `xml:"Id"`
		Name string `xml:"Name"`
	} `xml:"HostedZones>HostedZone"`
	IsTruncated  bool   `xml:"IsTruncated"`
	NextMarker   string `xml:"NextMarker"`
}

type route53RecordSets struct {
	XMLName            xml.Name `xml:"ListResourceRecordSetsResponse"`
	ResourceRecordSets []struct {
		Name            string `xml:"Name"`
		Type            string `xml:"Type"`
		TTL             int64  `xml:"TTL"`
		ResourceRecords []struct {
			Value string `xml:"Value"`
		} `xml:"ResourceRecords>ResourceRecord"`
	} `xml:"ResourceRecordSets>ResourceRecordSet"`
}

type route53ChangeResponse struct {
	XMLName xml.Name `xml:"ErrorResponse"`
	Error   struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Error"`
}

func NewRoute53Client(accessKey, secretKey string) *Route53Client {
	return &Route53Client{
		signer:     newRoute53Signer(accessKey, secretKey),
		httpClient: newHTTPClient(),
		zoneIDs:    make(map[string]string),
	}
}

func (c *Route53Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "2013-04-01/hostedzone", "maxitems=1", nil)
	return err
}

func (c *Route53Client) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return syncVerifiedMessage("Route53"), nil
}

func (c *Route53Client) Cleanup(ctx context.Context) (string, error) {
	return c.Recover(ctx)
}

func (c *Route53Client) SupportsLine() bool { return false }

func (c *Route53Client) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	var all []ProviderDomain
	marker := ""
	for {
		q := "maxitems=100"
		if marker != "" {
			q += "&marker=" + url.QueryEscape(marker)
		}
		body, err := c.do(ctx, http.MethodGet, "2013-04-01/hostedzone", q, nil)
		if err != nil {
			return nil, err
		}
		var parsed route53HostedZones
		if err := xml.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		for _, z := range parsed.HostedZones {
			name := strings.Trim(strings.TrimSpace(z.Name), ".")
			all = append(all, ProviderDomain{Name: name})
		}
		if !parsed.IsTruncated || parsed.NextMarker == "" {
			break
		}
		marker = parsed.NextMarker
	}
	return all, nil
}

func (c *Route53Client) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, _ string, _ map[string]int32) (string, error) {
	zone = NormalizeZone(zone)
	rr := NormalizeName(name)
	if zone == "" {
		return "", fmt.Errorf("route53: zone is required")
	}
	if err := validateRecordValues(recordType, values); err != nil {
		return "", err
	}

	zoneID, err := c.getZoneID(ctx, zone)
	if err != nil {
		return "", err
	}

	fqdn := route53FQDN(JoinFQDN(rr, zone))
	desired := normalizeDesiredValues(values)
	targetTTL := normalizeRoute53TTL(ttl)
	existing, err := c.listRecordValues(ctx, zoneID, fqdn, recordType)
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
		return formatEnsureMessage("route53", recordType, fqdn, EnsureResult{}), nil
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

	action := "UPSERT"
	if len(desired) == 0 {
		action = "DELETE"
		if len(existingSet) == 0 {
			return formatEnsureMessage("route53", recordType, fqdn, EnsureResult{}), nil
		}
		desired = existingSet
	}
	if err := c.changeRecords(ctx, zoneID, action, fqdn, recordType, desired, targetTTL); err != nil {
		return "", err
	}
	return formatEnsureMessage("route53", recordType, fqdn, res), nil
}

func (c *Route53Client) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	zone = NormalizeZone(zone)
	if zone == "" {
		return nil, fmt.Errorf("route53: zone is required")
	}
	zoneID, err := c.getZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	body, err := c.do(ctx, http.MethodGet, "2013-04-01/hostedzone/"+zoneID+"/rrset", "maxitems=300", nil)
	if err != nil {
		return nil, err
	}
	var parsed route53RecordSets
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	out := make([]DNSRecord, 0)
	for _, set := range parsed.ResourceRecordSets {
		rt := RecordType(strings.TrimSpace(set.Type))
		if recordType != "" && rt != recordType {
			continue
		}
		fqdn := strings.Trim(strings.TrimSpace(set.Name), ".")
		rr, ok := SplitByZone(fqdn, zone)
		if !ok {
			continue
		}
		displayName := rr
		if rr != "@" {
			displayName = rr + "." + zone
		} else {
			displayName = zone
		}
		for _, rec := range set.ResourceRecords {
			out = append(out, DNSRecord{
				Name:  displayName,
				Type:  rt,
				Value: strings.Trim(strings.TrimSpace(rec.Value), "."),
				TTL:   set.TTL,
			})
		}
	}
	return out, nil
}

func (c *Route53Client) getZoneID(ctx context.Context, zone string) (string, error) {
	c.mu.Lock()
	if id, ok := c.zoneIDs[zone]; ok && id != "" {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	marker := ""
	target := strings.ToLower(zone)
	for {
		q := "maxitems=100"
		if marker != "" {
			q += "&marker=" + url.QueryEscape(marker)
		}
		body, err := c.do(ctx, http.MethodGet, "2013-04-01/hostedzone", q, nil)
		if err != nil {
			return "", err
		}
		var parsed route53HostedZones
		if err := xml.Unmarshal(body, &parsed); err != nil {
			return "", err
		}
		for _, z := range parsed.HostedZones {
			name := strings.Trim(strings.TrimSpace(z.Name), ".")
			if strings.EqualFold(name, target) {
				id := strings.TrimPrefix(z.ID, "/hostedzone/")
				c.mu.Lock()
				c.zoneIDs[zone] = id
				c.mu.Unlock()
				return id, nil
			}
		}
		if !parsed.IsTruncated || parsed.NextMarker == "" {
			break
		}
		marker = parsed.NextMarker
	}
	return "", fmt.Errorf("route53: zone not found: %s", zone)
}

func (c *Route53Client) listRecordValues(ctx context.Context, zoneID, fqdn string, recordType RecordType) ([]ManagedRecord, error) {
	q := "name=" + url.QueryEscape(fqdn) + "&type=" + url.QueryEscape(string(recordType)) + "&maxitems=100"
	body, err := c.do(ctx, http.MethodGet, "2013-04-01/hostedzone/"+zoneID+"/rrset", q, nil)
	if err != nil {
		return nil, err
	}
	var parsed route53RecordSets
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var out []ManagedRecord
	for _, set := range parsed.ResourceRecordSets {
		if !strings.EqualFold(strings.TrimSpace(set.Name), fqdn) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(set.Type), string(recordType)) {
			continue
		}
		for _, rec := range set.ResourceRecords {
			out = append(out, ManagedRecord{
				ID:    fqdn,
				Value: strings.Trim(strings.TrimSpace(rec.Value), "."),
				TTL:   set.TTL,
			})
		}
	}
	return out, nil
}

func (c *Route53Client) changeRecords(ctx context.Context, zoneID, action, fqdn string, recordType RecordType, values map[string]struct{}, ttl int64) error {
	var recordsXML strings.Builder
	for val := range values {
		content := val
		if recordType == RecordTypeCNAME {
			content = route53FQDN(val)
		}
		recordsXML.WriteString("<ResourceRecord><Value>")
		recordsXML.WriteString(xmlEscape(content))
		recordsXML.WriteString("</Value></ResourceRecord>")
	}
	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/"><ChangeBatch><Changes><Change><Action>%s</Action><ResourceRecordSet><Name>%s</Name><Type>%s</Type><TTL>%d</TTL><ResourceRecords>%s</ResourceRecords></ResourceRecordSet></Change></Changes></ChangeBatch></ChangeResourceRecordSetsRequest>`,
		action, xmlEscape(fqdn), recordType, ttl, recordsXML.String())

	body, err := c.do(ctx, http.MethodPost, "2013-04-01/hostedzone/"+zoneID+"/rrset/", "", []byte(xmlBody))
	if err != nil {
		return err
	}
	if strings.Contains(string(body), "<Error>") {
		var parsed route53ChangeResponse
		_ = xml.Unmarshal(body, &parsed)
		if parsed.Error.Message != "" {
			return fmt.Errorf("route53: %s: %s", parsed.Error.Code, parsed.Error.Message)
		}
		return errors.New("route53: change failed")
	}
	return nil
}

func (c *Route53Client) do(ctx context.Context, method, path, query string, payload []byte) ([]byte, error) {
	raw, status, err := c.signer.do(ctx, c.httpClient, method, path, query, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		if strings.Contains(string(raw), "<Error>") {
			var parsed route53ChangeResponse
			_ = xml.Unmarshal(raw, &parsed)
			if parsed.Error.Message != "" {
				return nil, fmt.Errorf("route53: %s: %s", parsed.Error.Code, parsed.Error.Message)
			}
		}
		return nil, fmt.Errorf("route53: HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func route53FQDN(name string) string {
	name = strings.Trim(strings.TrimSpace(name), ".")
	return name + "."
}

func normalizeRoute53TTL(ttl int64) int64 {
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

func desiredValues(records []ManagedRecord) map[string]struct{} {
	out := make(map[string]struct{}, len(records))
	for _, rec := range records {
		out[strings.Trim(rec.Value, ".")] = struct{}{}
	}
	return out
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
