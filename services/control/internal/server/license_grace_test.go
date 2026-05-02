package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lingcdn/control/internal/config"
)

func TestOnLicenseVerifyFailed_UsesGraceForActive(t *testing.T) {
	srv := &Servers{
		cfg: config.Config{LicenseGraceHours: 24, LicenseMode: "online"},
	}
	srv.setLicenseState(licenseState{
		Status:      "active",
		ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
		LastChecked: time.Now().Add(-5 * time.Minute),
	})

	srv.onLicenseVerifyFailed("signature verify failed")
	st := srv.currentLicenseStatus()
	if st.Status != "active" {
		t.Fatalf("status=%q want=active", st.Status)
	}
	if st.GraceUntil.IsZero() {
		t.Fatalf("expected grace_until to be set")
	}
}

func TestVerifyLicenseOnce_PausedStatusSkipsGrace(t *testing.T) {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/licenses/verify" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error":"license paused","status":"paused"}`))
	}))
	t.Cleanup(portal.Close)

	old := hardcodedUpgradePortalBase
	hardcodedUpgradePortalBase = portal.URL
	t.Cleanup(func() { hardcodedUpgradePortalBase = old })

	srv := &Servers{
		cfg: config.Config{LicenseGraceHours: 24, LicenseMode: "online"},
	}
	srv.setLicenseState(licenseState{
		Status:     "active",
		LicenseKey: "LIC-TEST",
		ExpiresAt:  time.Now().Add(7 * 24 * time.Hour),
	})

	if _, err := srv.verifyLicenseOnce(context.Background()); err == nil {
		t.Fatalf("expected verify failure for paused license")
	}
	st := srv.currentLicenseStatus()
	if st.Status != "paused" {
		t.Fatalf("status=%q want=paused", st.Status)
	}
	if !st.GraceUntil.IsZero() {
		t.Fatalf("expected no grace window for paused status")
	}
	if st.Reason == "" {
		t.Fatalf("expected paused reason to be set")
	}
}

func TestVerifyLicenseOnce_LegacyNotActiveMapsPaused(t *testing.T) {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/licenses/verify" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error":"license not active"}`))
	}))
	t.Cleanup(portal.Close)

	old := hardcodedUpgradePortalBase
	hardcodedUpgradePortalBase = portal.URL
	t.Cleanup(func() { hardcodedUpgradePortalBase = old })

	srv := &Servers{
		cfg: config.Config{LicenseGraceHours: 24, LicenseMode: "online"},
	}
	srv.setLicenseState(licenseState{
		Status:     "active",
		LicenseKey: "LIC-TEST",
		ExpiresAt:  time.Now().Add(7 * 24 * time.Hour),
	})

	if _, err := srv.verifyLicenseOnce(context.Background()); err == nil {
		t.Fatalf("expected verify failure for inactive license")
	}
	st := srv.currentLicenseStatus()
	if st.Status != "paused" {
		t.Fatalf("status=%q want=paused", st.Status)
	}
	if !st.GraceUntil.IsZero() {
		t.Fatalf("expected no grace window for legacy inactive response")
	}
}
