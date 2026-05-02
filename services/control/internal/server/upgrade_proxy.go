package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// fetchUpgradeUpstream fetches upgrade info from an upstream API (e.g., portal).
func fetchUpgradeUpstream(endpoint string) (*upgradeInfo, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("upgrade endpoint must use HTTPS: %s", u.Scheme)
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upgrade upstream http %d", resp.StatusCode)
	}
	var info upgradeInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}
