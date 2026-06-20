package dnsprovider

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const googleDNSBase = "https://dns.googleapis.com/dns/v1"

// GoogleDNSClient manages records on Google Cloud DNS.
type GoogleDNSClient struct {
	projectID  string
	token      string
	saJSON     string
	httpClient *http.Client

	mu         sync.Mutex
	zoneNames  map[string]string
	tokenCache struct {
		value     string
		expiresAt time.Time
	}
}

func NewGoogleDNSClient(projectID, token, serviceAccountJSON string) *GoogleDNSClient {
	return &GoogleDNSClient{
		projectID:  strings.TrimSpace(projectID),
		token:      strings.TrimSpace(token),
		saJSON:     strings.TrimSpace(serviceAccountJSON),
		httpClient: newHTTPClient(),
		zoneNames:  make(map[string]string),
	}
}

func (c *GoogleDNSClient) Ping(ctx context.Context) error {
	if c.projectID == "" {
		return fmt.Errorf("gcp dns: project id required (account_id)")
	}
	accessToken, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/projects/%s/managedZones?pageSize=1", googleDNSBase, url.PathEscape(c.projectID))
	_, err = c.doGET(ctx, u, accessToken)
	return err
}

func (c *GoogleDNSClient) Recover(ctx context.Context) (string, error) {
	if err := c.Ping(ctx); err != nil {
		return "", err
	}
	return syncVerifiedMessage("Google Cloud DNS"), nil
}

func (c *GoogleDNSClient) Cleanup(ctx context.Context) (string, error) {
	return c.Recover(ctx)
}

func (c *GoogleDNSClient) SupportsLine() bool { return false }

func (c *GoogleDNSClient) ListProviderDomains(ctx context.Context) ([]ProviderDomain, error) {
	accessToken, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var all []ProviderDomain
	pageToken := ""
	for {
		u := fmt.Sprintf("%s/projects/%s/managedZones?pageSize=100", googleDNSBase, url.PathEscape(c.projectID))
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		raw, err := c.doGET(ctx, u, accessToken)
		if err != nil {
			return nil, err
		}
		var resp struct {
			ManagedZones []struct {
				DNSName string `json:"dnsName"`
			} `json:"managedZones"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, err
		}
		for _, z := range resp.ManagedZones {
			all = append(all, ProviderDomain{Name: strings.Trim(strings.TrimSpace(z.DNSName), ".")})
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return all, nil
}

func (c *GoogleDNSClient) EnsureRecords(ctx context.Context, zone, name string, recordType RecordType, values []string, ttl int64, _ string, _ map[string]int32) (string, error) {
	zone = NormalizeZone(zone)
	rr := NormalizeName(name)
	if zone == "" {
		return "", fmt.Errorf("gcp dns: zone is required")
	}
	if err := validateRecordValues(recordType, values); err != nil {
		return "", err
	}

	zoneName, err := c.getManagedZoneName(ctx, zone)
	if err != nil {
		return "", err
	}

	rrsetName := googleRRSetName(rr, zone)
	desired := normalizeDesiredValues(values)
	targetTTL := normalizeGoogleTTL(ttl)
	existing, err := c.listManagedRecords(ctx, zoneName, rrsetName, recordType)
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
		return formatEnsureMessage("gcp", recordType, rrsetName, EnsureResult{}), nil
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

	if len(desired) == 0 {
		if len(existingSet) == 0 {
			return formatEnsureMessage("gcp", recordType, rrsetName, EnsureResult{}), nil
		}
		if err := c.deleteRRSet(ctx, zoneName, rrsetName, recordType); err != nil {
			return "", err
		}
		return formatEnsureMessage("gcp", recordType, rrsetName, res), nil
	}

	rrdatas := googleRRDatas(recordType, valuesFromDesired(desired))
	if err := c.patchRRSet(ctx, zoneName, rrsetName, recordType, rrdatas, targetTTL); err != nil {
		return "", err
	}
	return formatEnsureMessage("gcp", recordType, rrsetName, res), nil
}

func (c *GoogleDNSClient) ListRecords(ctx context.Context, zone string, recordType RecordType) ([]DNSRecord, error) {
	zone = NormalizeZone(zone)
	if zone == "" {
		return nil, fmt.Errorf("gcp dns: zone is required")
	}
	zoneName, err := c.getManagedZoneName(ctx, zone)
	if err != nil {
		return nil, err
	}

	accessToken, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/projects/%s/managedZones/%s/rrsets?pageSize=500", googleDNSBase, url.PathEscape(c.projectID), url.PathEscape(zoneName))
	raw, err := c.doGET(ctx, u, accessToken)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Rrsets []struct {
			Name    string   `json:"name"`
			Type    string   `json:"type"`
			TTL     int64    `json:"ttl"`
			Rrdatas []string `json:"rrdatas"`
		} `json:"rrsets"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}

	out := make([]DNSRecord, 0)
	for _, set := range resp.Rrsets {
		rt := RecordType(strings.TrimSpace(set.Type))
		if recordType != "" && rt != recordType {
			continue
		}
		fqdn := strings.Trim(strings.TrimSpace(set.Name), ".")
		rr, ok := SplitByZone(fqdn, zone)
		if !ok {
			continue
		}
		display := zone
		if rr != "@" {
			display = rr + "." + zone
		}
		for _, val := range set.Rrdatas {
			out = append(out, DNSRecord{
				Name:  display,
				Type:  rt,
				Value: strings.Trim(strings.TrimSpace(val), "."),
				TTL:   set.TTL,
			})
		}
	}
	return out, nil
}

func (c *GoogleDNSClient) getManagedZoneName(ctx context.Context, zone string) (string, error) {
	c.mu.Lock()
	if name, ok := c.zoneNames[zone]; ok && name != "" {
		c.mu.Unlock()
		return name, nil
	}
	c.mu.Unlock()

	accessToken, err := c.accessToken(ctx)
	if err != nil {
		return "", err
	}
	u := fmt.Sprintf("%s/projects/%s/managedZones?pageSize=100", googleDNSBase, url.PathEscape(c.projectID))
	raw, err := c.doGET(ctx, u, accessToken)
	if err != nil {
		return "", err
	}
	var resp struct {
		ManagedZones []struct {
			Name    string `json:"name"`
			DNSName string `json:"dnsName"`
		} `json:"managedZones"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	target := strings.ToLower(zone)
	for _, z := range resp.ManagedZones {
		dnsName := strings.Trim(strings.TrimSpace(z.DNSName), ".")
		if strings.EqualFold(dnsName, target) {
			c.mu.Lock()
			c.zoneNames[zone] = z.Name
			c.mu.Unlock()
			return z.Name, nil
		}
	}
	return "", fmt.Errorf("gcp dns: zone not found: %s", zone)
}

func (c *GoogleDNSClient) listManagedRecords(ctx context.Context, zoneName, rrsetName string, recordType RecordType) ([]ManagedRecord, error) {
	accessToken, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/projects/%s/managedZones/%s/rrsets/%s/%s",
		googleDNSBase, url.PathEscape(c.projectID), url.PathEscape(zoneName),
		url.PathEscape(rrsetName), url.PathEscape(string(recordType)))
	raw, err := c.doGET(ctx, u, accessToken)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return nil, nil
		}
		return nil, err
	}
	var resp struct {
		TTL     int64    `json:"ttl"`
		Rrdatas []string `json:"rrdatas"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	out := make([]ManagedRecord, 0, len(resp.Rrdatas))
	for _, val := range resp.Rrdatas {
		out = append(out, ManagedRecord{
			ID:    rrsetName,
			Value: strings.Trim(strings.TrimSpace(val), "."),
			TTL:   resp.TTL,
		})
	}
	return out, nil
}

func (c *GoogleDNSClient) patchRRSet(ctx context.Context, zoneName, rrsetName string, recordType RecordType, rrdatas []string, ttl int64) error {
	accessToken, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"name":    rrsetName,
		"type":    string(recordType),
		"ttl":     ttl,
		"rrdatas": rrdatas,
	}
	changeBody := map[string]any{"additions": []any{payload}}
	existing, _ := c.listManagedRecords(ctx, zoneName, rrsetName, recordType)
	if len(existing) > 0 {
		old := map[string]any{
			"name":    rrsetName,
			"type":    string(recordType),
			"ttl":     existing[0].TTL,
			"rrdatas": googleRRDatas(recordType, valuesFromDesired(desiredValues(existing))),
		}
		changeBody["deletions"] = []any{old}
	}
	body, _ := json.Marshal(changeBody)
	u := fmt.Sprintf("%s/projects/%s/managedZones/%s/changes", googleDNSBase, url.PathEscape(c.projectID), url.PathEscape(zoneName))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := ioReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gcp dns: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (c *GoogleDNSClient) deleteRRSet(ctx context.Context, zoneName, rrsetName string, recordType RecordType) error {
	existing, err := c.listManagedRecords(ctx, zoneName, rrsetName, recordType)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}
	accessToken, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	deletion := map[string]any{
		"name":    rrsetName,
		"type":    string(recordType),
		"ttl":     existing[0].TTL,
		"rrdatas": googleRRDatas(recordType, valuesFromDesired(desiredValues(existing))),
	}
	body, _ := json.Marshal(map[string]any{"deletions": []any{deletion}})
	u := fmt.Sprintf("%s/projects/%s/managedZones/%s/changes", googleDNSBase, url.PathEscape(c.projectID), url.PathEscape(zoneName))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := ioReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gcp dns: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (c *GoogleDNSClient) doGET(ctx context.Context, u, accessToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gcp dns: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func (c *GoogleDNSClient) accessToken(ctx context.Context) (string, error) {
	if c.token != "" && !strings.HasPrefix(c.token, "{") {
		return c.token, nil
	}
	c.mu.Lock()
	if c.tokenCache.value != "" && time.Now().Before(c.tokenCache.expiresAt.Add(-30*time.Second)) {
		token := c.tokenCache.value
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	saJSON := c.saJSON
	if saJSON == "" && strings.HasPrefix(c.token, "{") {
		saJSON = c.token
	}
	if saJSON == "" {
		return "", fmt.Errorf("gcp dns: access token or service account json required")
	}
	token, expiresIn, err := googleServiceAccountToken(ctx, c.httpClient, saJSON)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.tokenCache.value = token
	c.tokenCache.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	c.mu.Unlock()
	return token, nil
}

func googleServiceAccountToken(ctx context.Context, client *http.Client, saJSON string) (string, int64, error) {
	var sa struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
		return "", 0, fmt.Errorf("gcp dns: invalid service account json: %w", err)
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	now := time.Now().UTC()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iss":   sa.ClientEmail,
		"scope": "https://www.googleapis.com/auth/ndev.clouddns.readwrite",
		"aud":   sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	payload := base64.RawURLEncoding.EncodeToString(claims)
	signingInput := header + "." + payload

	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", 0, fmt.Errorf("gcp dns: invalid private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return "", 0, fmt.Errorf("gcp dns: parse private key: %w", err)
		}
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", 0, fmt.Errorf("gcp dns: private key is not RSA")
	}
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", 0, err
	}
	assertion := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sa.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if client == nil {
		client = newHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, _ := ioReadAll(resp.Body)
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", 0, err
	}
	if parsed.AccessToken == "" {
		return "", 0, fmt.Errorf("gcp dns: token exchange failed: %s", strings.TrimSpace(string(raw)))
	}
	return parsed.AccessToken, parsed.ExpiresIn, nil
}

func googleRRSetName(rr, zone string) string {
	if rr == "@" {
		return zone + "."
	}
	return rr + "." + zone + "."
}

func googleRRDatas(recordType RecordType, values []string) []string {
	out := make([]string, 0, len(values))
	for _, val := range values {
		val = strings.Trim(strings.TrimSpace(val), ".")
		if recordType == RecordTypeCNAME {
			val = val + "."
		}
		out = append(out, val)
	}
	return out
}

func normalizeGoogleTTL(ttl int64) int64 {
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
