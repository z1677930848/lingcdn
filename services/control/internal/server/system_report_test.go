package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/store"
)

func TestSystemReport_ReportOnce(t *testing.T) {
	secret := "test-webhook-secret"
	ch := make(chan struct{}, 1)

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if r.URL.Path != "/api/control/report" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		sig := r.Header.Get("X-Report-Signature")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(bytes.TrimSpace(raw))
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if sig != expected {
			t.Fatalf("signature mismatch: got=%s expected=%s", sig, expected)
		}

		var req struct {
			ControlID      string `json:"control_id"`
			SitesTotal     int    `json:"sites_total"`
			NodesTotal     int    `json:"nodes_total"`
			NodesInstalled int    `json:"nodes_installed"`
			UsersTotal     int    `json:"users_total"`
			Version        string `json:"version"`
			LicenseIP      string `json:"license_ip"`
			LicenseAt      string `json:"license_at"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.ControlID != "control-test" {
			t.Fatalf("control_id=%s", req.ControlID)
		}
		if req.Version != "9.9.9" {
			t.Fatalf("version=%s", req.Version)
		}
		if req.UsersTotal != 2 {
			t.Fatalf("users_total=%d", req.UsersTotal)
		}
		if req.NodesTotal != 3 {
			t.Fatalf("nodes_total=%d", req.NodesTotal)
		}
		if req.NodesInstalled != 2 {
			t.Fatalf("nodes_installed=%d", req.NodesInstalled)
		}
		if req.SitesTotal != 4 {
			t.Fatalf("sites_total=%d", req.SitesTotal)
		}
		if req.LicenseIP != "8.8.8.8" {
			t.Fatalf("license_ip=%s", req.LicenseIP)
		}
		if strings.TrimSpace(req.LicenseAt) == "" {
			t.Fatalf("license_at is empty")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		ch <- struct{}{}
	}))
	t.Cleanup(portal.Close)

	old := hardcodedUpgradePortalBase
	hardcodedUpgradePortalBase = portal.URL
	t.Cleanup(func() { hardcodedUpgradePortalBase = old })

	prevVersion := os.Getenv("APP_VERSION")
	_ = os.Setenv("APP_VERSION", "9.9.9")
	t.Cleanup(func() { _ = os.Setenv("APP_VERSION", prevVersion) })

	st := store.NewMemory("", "")
	ctx := context.Background()
	_ = st.CreateUser(ctx, &store.User{ID: "u1", Username: "u1", Email: "u1@example.com", PasswordHash: "x", Role: "user"})
	_ = st.CreateUser(ctx, &store.User{ID: "u2", Username: "u2", Email: "u2@example.com", PasswordHash: "x", Role: "user"})
	_ = st.CreateNode(ctx, &store.Node{ID: "n1", Hostname: "n1", Token: "t1"})
	_ = st.CreateNode(ctx, &store.Node{ID: "n2", Hostname: "n2", Token: "t2"})
	_ = st.CreateNode(ctx, &store.Node{ID: "n3", Hostname: "n3", Token: "t3"})
	_ = st.UpdateNodeStatus(ctx, "n1", "online", "v1")
	_ = st.UpdateNodeStatus(ctx, "n2", "online", "v1")
	_ = st.CreateDomain(ctx, &store.Domain{ID: "d1", Name: "a.example.com", Enabled: true})
	_ = st.CreateDomain(ctx, &store.Domain{ID: "d2", Name: "b.example.com", Enabled: true})
	_ = st.CreateDomain(ctx, &store.Domain{ID: "d3", Name: "c.example.com", Enabled: true})
	_ = st.CreateDomain(ctx, &store.Domain{ID: "d4", Name: "d.example.com", Enabled: true})

	cfg := config.Config{
		PortalBase:         portal.URL,
		PortalReportSecret: secret,
		ControlID:          "control-test",
		PublicIP:           "8.8.8.8",
	}
	s := New(cfg, nil, nil, nil, nil, nil, st, nil)
	s.license = licenseState{Status: "active", LicenseKey: "LIC-TEST"}

	if _, err := s.reportSystemOnce(context.Background()); err != nil {
		t.Fatalf("report error: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("no report received")
	}
}

// TestSystemReport_PrefersLicenseReportSecret guards the new auto-sync flow:
// when the portal has delivered a per-license report_secret via the license
// verify response (and we cached it in licenseState.ReportSecret), the
// system report HMAC must be signed with THAT secret — not with the legacy
// cfg.PortalReportSecret. This is what makes the operator-free key sync work.
func TestSystemReport_PrefersLicenseReportSecret(t *testing.T) {
	licenseSecret := "delivered-by-portal-via-license-verify"
	staleCfgSecret := "old-out-of-sync-operator-secret"

	ch := make(chan struct{}, 1)
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		// Portal verifies with the license-delivered secret, mirroring the
		// real PHP ReportController behaviour.
		mac := hmac.New(sha256.New, []byte(licenseSecret))
		_, _ = mac.Write(bytes.TrimSpace(raw))
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if got := r.Header.Get("X-Report-Signature"); got != expected {
			t.Fatalf("signature mismatch: got=%s expected=%s (control should have used licenseSecret, not cfg)", got, expected)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		ch <- struct{}{}
	}))
	t.Cleanup(portal.Close)

	old := hardcodedUpgradePortalBase
	hardcodedUpgradePortalBase = portal.URL
	t.Cleanup(func() { hardcodedUpgradePortalBase = old })

	st := store.NewMemory("", "")

	cfg := config.Config{
		PortalBase:         portal.URL,
		PortalReportSecret: staleCfgSecret, // intentionally wrong; license secret should win
		ControlID:          "control-test",
		PublicIP:           "8.8.8.8",
	}
	s := New(cfg, nil, nil, nil, nil, nil, st, nil)
	s.license = licenseState{
		Status:       "active",
		LicenseKey:   "LIC-TEST",
		ReportSecret: licenseSecret,
	}

	if _, err := s.reportSystemOnce(context.Background()); err != nil {
		t.Fatalf("report error: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("no report received (likely signed with the wrong secret and rejected)")
	}
}

// TestBuildVerifiedLicenseState_AdoptsReportSecretFromUnsignedResponse
// exercises the legacy/non-signed verify path: when the portal cannot sign
// (no PORTAL_SIGNING_PRIVKEY) but does deliver report_secret as a top-level
// field, the control plane must adopt it.
func TestBuildVerifiedLicenseState_AdoptsReportSecretFromUnsignedResponse(t *testing.T) {
	s := &Servers{cfg: config.Config{}}
	current := licenseState{Status: "active", LicenseKey: "LIC-X"}
	resp := portalLicenseVerifyResponse{
		OK:           true,
		Status:       "active",
		ReportSecret: "freshly-delivered-secret",
	}
	got, err := s.buildVerifiedLicenseState(current, resp)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got.ReportSecret != "freshly-delivered-secret" {
		t.Fatalf("ReportSecret not adopted: got=%q", got.ReportSecret)
	}
}

// TestBuildVerifiedLicenseState_KeepsCachedReportSecretWhenPortalOmits
// guards against accidentally clearing a cached secret when the portal
// happens not to deliver one in a given verify round-trip (e.g. because IP
// no longer matches bind_ip, or because we are talking to an older portal).
func TestBuildVerifiedLicenseState_KeepsCachedReportSecretWhenPortalOmits(t *testing.T) {
	s := &Servers{cfg: config.Config{}}
	current := licenseState{
		Status:       "active",
		LicenseKey:   "LIC-X",
		ReportSecret: "cached-from-earlier-verify",
	}
	resp := portalLicenseVerifyResponse{OK: true, Status: "active"} // no ReportSecret
	got, err := s.buildVerifiedLicenseState(current, resp)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got.ReportSecret != "cached-from-earlier-verify" {
		t.Fatalf("cached ReportSecret was dropped: got=%q", got.ReportSecret)
	}
}
