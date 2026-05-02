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
	"time"
)

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

