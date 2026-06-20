package dnsprovider

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const huaweiDNSEndpoint = "https://dns.myhuaweicloud.com"

type huaweiSigner struct {
	ak string
	sk string
}

func (s *huaweiSigner) do(ctx context.Context, client *http.Client, method, path string, query url.Values, body []byte) ([]byte, int, error) {
	if client == nil {
		client = newHTTPClient()
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	reqURL := huaweiDNSEndpoint + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
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
		req.Header.Set("Content-Type", "application/json")
	}

	now := time.Now().UTC()
	date := now.Format("20060102T150405Z")
	req.Header.Set("Host", "dns.myhuaweicloud.com")
	req.Header.Set("X-Sdk-Date", date)

	auth, err := s.sign(method, path, query, req.Header, body, date)
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
	return raw, resp.StatusCode, err
}

func (s *huaweiSigner) sign(method, path string, query url.Values, headers http.Header, body []byte, date string) (string, error) {
	payloadHash := sha256Hex(body)
	canonicalQuery := huaweiCanonicalQuery(query)
	canonicalHeaders, signedHeaders := huaweiCanonicalHeaders(headers)
	canonicalRequest := strings.Join([]string{
		method,
		huaweiCanonicalURI(path),
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	crHash := sha256Hex([]byte(canonicalRequest))
	stringToSign := "SDK-HMAC-SHA256\n" + date + "\n" + crHash

	sig := hex.EncodeToString(hmacSHA256([]byte(s.sk), []byte(stringToSign)))
	return fmt.Sprintf("SDK-HMAC-SHA256 Access=%s, SignedHeaders=%s, Signature=%s", s.ak, signedHeaders, sig), nil
}

func huaweiCanonicalURI(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = huaweiEscape(p)
	}
	out := strings.Join(parts, "/")
	if out == "" || !strings.HasSuffix(out, "/") {
		out += "/"
	}
	return out
}

func huaweiCanonicalQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	var keys []string
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := query[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, huaweiEscape(k)+"="+huaweiEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func huaweiCanonicalHeaders(headers http.Header) (string, string) {
	type pair struct{ k, v string }
	var pairs []pair
	for k, vals := range headers {
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk != "host" && lk != "x-sdk-date" && lk != "content-type" {
			continue
		}
		for _, v := range vals {
			pairs = append(pairs, pair{lk, strings.TrimSpace(v)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k == pairs[j].k {
			return pairs[i].v < pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	var signed []string
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(p.k)
		b.WriteByte(':')
		b.WriteString(p.v)
		if len(signed) == 0 || signed[len(signed)-1] != p.k {
			signed = append(signed, p.k)
		}
	}
	b.WriteByte('\n')
	return b.String(), strings.Join(signed, ";")
}

func huaweiEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '~' || c == '.' {
			b.WriteByte(c)
			continue
		}
		b.WriteString(fmt.Sprintf("%%%02X", c))
	}
	return b.String()
}
