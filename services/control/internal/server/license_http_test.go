package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestCheckLicenseForHTTPAllowsExpiredAndLimited(t *testing.T) {
	srv := &Servers{}

	srv.setLicenseState(licenseState{Status: "active", ExpiresAt: time.Now().Add(-time.Hour)})
	if ok, _ := srv.checkLicenseForHTTP("/api/domains"); !ok {
		t.Fatalf("expected expired license to allow business API")
	}

	srv.setLicenseState(licenseState{Status: "limited"})
	if ok, _ := srv.checkLicenseForHTTP("/api/domains"); !ok {
		t.Fatalf("expected limited license to allow business API")
	}
}

func TestCheckLicenseForHTTPBlocksUnlicensedAndRevoked(t *testing.T) {
	srv := &Servers{}

	srv.setLicenseState(licenseState{Status: "unlicensed"})
	if ok, _ := srv.checkLicenseForHTTP("/api/domains"); ok {
		t.Fatalf("expected unlicensed to block business API")
	}

	srv.setLicenseState(licenseState{Status: "revoked", Reason: "revoked"})
	if ok, _ := srv.checkLicenseForHTTP("/api/domains"); ok {
		t.Fatalf("expected revoked to block business API")
	}

	srv.setLicenseState(licenseState{Status: "paused"})
	if ok, _ := srv.checkLicenseForHTTP("/api/domains"); ok {
		t.Fatalf("expected paused to block business API")
	}
}

func TestLicenseImportDisabled(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/license/import", bytes.NewBufferString(`{"payload":"anything"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("license import: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !stringsContains(out["error"], "auth.lingcdn.cloud") {
		t.Fatalf("unexpected error response: %#v", out)
	}
}

func stringsContains(v any, part string) bool {
	s, _ := v.(string)
	return s != "" && bytes.Contains([]byte(s), []byte(part))
}
