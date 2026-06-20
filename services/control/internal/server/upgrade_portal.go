package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	portalLatestCacheTTL      = 5 * time.Minute
	portalLatestErrorCacheTTL = 30 * time.Second
)

type portalLatestCacheEntry struct {
	latest *portalLatest
	err    error
	at     time.Time
}

func portalLatestCacheKey(baseURL, product, channel, platform, arch, version string) string {
	return strings.Join([]string{baseURL, product, channel, platform, arch, version}, "|")
}

func (s *Servers) fetchPortalLatestCached(ctx context.Context, baseURL, product, channel, platform, arch, version string) (*portalLatest, error) {
	if s == nil {
		return fetchPortalLatest(ctx, baseURL, product, channel, platform, arch, version)
	}
	key := portalLatestCacheKey(baseURL, product, channel, platform, arch, version)

	s.portalLatestCacheMu.RLock()
	if e, ok := s.portalLatestCache[key]; ok {
		ttl := portalLatestCacheTTL
		if e.err != nil {
			ttl = portalLatestErrorCacheTTL
		}
		if time.Since(e.at) < ttl {
			s.portalLatestCacheMu.RUnlock()
			return e.latest, e.err
		}
	}
	s.portalLatestCacheMu.RUnlock()

	latest, err := fetchPortalLatest(ctx, baseURL, product, channel, platform, arch, version)

	s.portalLatestCacheMu.Lock()
	if s.portalLatestCache == nil {
		s.portalLatestCache = make(map[string]portalLatestCacheEntry)
	}
	s.portalLatestCache[key] = portalLatestCacheEntry{latest: latest, err: err, at: time.Now()}
	s.portalLatestCacheMu.Unlock()

	return latest, err
}

// fetchPortalLatestAll queries control + node (amd64/arm64) latest versions in parallel.
func (s *Servers) fetchPortalLatestAll(ctx context.Context, portal, channel, controlArch string) (control, nodeAMD64, nodeARM64 *portalLatest, notes []string) {
	type slot struct {
		idx    int
		latest *portalLatest
		err    error
	}
	queries := []struct {
		idx              int
		product, platform, arch string
	}{
		{0, "control", "linux", controlArch},
		{1, "node", "linux", "amd64"},
		{2, "node", "linux", "arm64"},
	}
	out := make([]slot, len(queries))
	var wg sync.WaitGroup
	for _, q := range queries {
		wg.Add(1)
		go func(q struct {
			idx              int
			product, platform, arch string
		}) {
			defer wg.Done()
			latest, err := s.fetchPortalLatestCached(ctx, portal, q.product, channel, q.platform, q.arch, "latest")
			out[q.idx] = slot{idx: q.idx, latest: latest, err: err}
		}(q)
	}
	wg.Wait()

	if out[0].err == nil {
		control = out[0].latest
	} else {
		notes = append(notes, fmt.Sprintf("failed to query control latest version: %v", out[0].err))
	}
	if out[1].err == nil {
		nodeAMD64 = out[1].latest
	}
	if out[2].err == nil {
		nodeARM64 = out[2].latest
	}
	return control, nodeAMD64, nodeARM64, notes
}

type portalLatest struct {
	Product     string `json:"product"`
	Version     string `json:"version"`
	Channel     string `json:"channel"`
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
	DownloadURL string `json:"download_url"`
	Checksum    string `json:"checksum"`
	Signature   string `json:"signature"`
	SigAlg      string `json:"sig_alg"`
	SigTarget   string `json:"sig_target"`
	PubKey      string `json:"pubkey"`
	SizeBytes   int64  `json:"size_bytes"`
	Changelog   string `json:"changelog"`
	BuildID     string `json:"build_id"`
}

func fetchPortalLatest(ctx context.Context, baseURL, product, channel, platform, arch, version string) (*portalLatest, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, errors.New("missing portal base url")
	}
	u, err := url.JoinPath(strings.TrimRight(baseURL, "/"), "/api/upgrade/latest")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("product", product)
	if channel != "" {
		q.Set("channel", channel)
	}
	if platform != "" {
		q.Set("platform", platform)
	}
	if arch != "" {
		q.Set("arch", arch)
	}
	if strings.TrimSpace(version) != "" {
		q.Set("version", strings.TrimSpace(version))
	}
	reqURL := u + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("portal latest http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out portalLatest
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.Version) == "" {
		return nil, errors.New("portal response missing version")
	}
	return &out, nil
}

