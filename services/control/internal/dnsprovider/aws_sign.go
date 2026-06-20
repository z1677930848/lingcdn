package dnsprovider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const route53Host = "route53.amazonaws.com"

type awsSigner struct {
	accessKey string
	secretKey string
	session   string
	region    string
	service   string
}

func newRoute53Signer(accessKey, secretKey string) *awsSigner {
	return &awsSigner{
		accessKey: strings.TrimSpace(accessKey),
		secretKey: strings.TrimSpace(secretKey),
		region:    "us-east-1",
		service:   "route53",
	}
}

func (s *awsSigner) do(ctx context.Context, client *http.Client, method, path, query string, body []byte) ([]byte, int, error) {
	if client == nil {
		client = newHTTPClient()
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	reqURL := "https://" + route53Host + path
	if query != "" {
		reqURL += "?" + query
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/xml")
	}

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	req.Header.Set("Host", route53Host)
	req.Header.Set("X-Amz-Date", amzDate)
	if s.session != "" {
		req.Header.Set("X-Amz-Security-Token", s.session)
	}

	auth, err := s.authorization(method, path, query, req.Header, body, amzDate)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", auth)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

func (s *awsSigner) authorization(method, path, query string, headers http.Header, body []byte, amzDate string) (string, error) {
	payloadHash := sha256Hex(body)
	canonicalHeaders, signedHeaders := awsCanonicalHeaders(headers)
	canonicalRequest := strings.Join([]string{
		method,
		path,
		awsCanonicalQueryString(query),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	dateStamp := amzDate[:8]
	credentialScope := dateStamp + "/" + s.region + "/" + s.service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := awsSigningKey(s.secretKey, dateStamp, s.region, s.service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	return fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey, credentialScope, signedHeaders, signature,
	), nil
}

func awsCanonicalHeaders(headers http.Header) (string, string) {
	type pair struct {
		key, val string
	}
	var pairs []pair
	for k, vals := range headers {
		lk := strings.ToLower(strings.TrimSpace(k))
		switch lk {
		case "host", "x-amz-date", "x-amz-security-token":
		default:
			continue
		}
		for _, v := range vals {
			pairs = append(pairs, pair{lk, strings.TrimSpace(v)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].val < pairs[j].val
		}
		return pairs[i].key < pairs[j].key
	})

	var signed []string
	var canonical strings.Builder
	for i, p := range pairs {
		if i > 0 {
			canonical.WriteByte('\n')
		}
		canonical.WriteString(p.key)
		canonical.WriteByte(':')
		canonical.WriteString(p.val)
		if signed == nil || signed[len(signed)-1] != p.key {
			signed = append(signed, p.key)
		}
	}
	canonical.WriteByte('\n')
	return canonical.String(), strings.Join(signed, ";")
}

func awsCanonicalQueryString(query string) string {
	if query == "" {
		return ""
	}
	vals, err := url.ParseQuery(query)
	if err != nil {
		return query
	}
	var keys []string
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := vals[k]
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, awsEscape(k)+"="+awsEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func awsEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteString(fmt.Sprintf("%%%02X", c))
	}
	return b.String()
}

func awsSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
