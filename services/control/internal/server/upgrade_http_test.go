package server

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crypto/hmac"
	"crypto/sha256"

	"golang.org/x/crypto/bcrypt"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/store"
)

func newControlTestServer(t *testing.T, portalBase string) (*Servers, *httptest.Server, string) {
	t.Helper()

	old := hardcodedUpgradePortalBase
	hardcodedUpgradePortalBase = portalBase
	t.Cleanup(func() { hardcodedUpgradePortalBase = old })

	cfg := config.Config{
		AuthSecret:     "test-auth-secret",
		WebhookSecret:  "test-webhook-secret",
		PortalBase:     portalBase,
		ControlService: "lingcdn-control",
	}

	st := store.NewMemory("", "")
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	_ = st.CreateUser(context.Background(), &store.User{
		ID:           "u-admin",
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordHash: string(hash),
		Role:         "admin",
	})

	s := New(cfg, nil, nil, nil, nil, nil, st, nil)
	s.setLicenseState(licenseState{
		Status:    "active",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		UpdatedAt: time.Now(),
	})

	ts := httptest.NewServer(s.adminMux())
	t.Cleanup(ts.Close)

	token := loginAdminToken(t, ts.URL)
	return s, ts, token
}

func loginAdminToken(t *testing.T, base string) string {
	t.Helper()
	reqBody := bytes.NewBufferString(`{"identifier":"admin","password":"admin123"}`)
	req, err := http.NewRequest(http.MethodPost, base+"/api/auth/login", reqBody)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login status=%d body=%s", resp.StatusCode, string(b))
	}
	var out struct {
		Token string `json:"token"`
		User  struct {
			Role string `json:"role"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if strings.TrimSpace(out.Token) == "" || out.User.Role != "admin" {
		t.Fatalf("login token/role invalid")
	}
	return out.Token
}

func TestControl_WebhookSignatureAndTimestamp(t *testing.T) {
	_, ts, _ := newControlTestServer(t, "")

	payload := map[string]any{
		"event":        "build.created",
		"product":      "node",
		"version":      "1.0.0-beta.1",
		"channel":      "stable",
		"platform":     "linux",
		"arch":         "amd64",
		"download_url": "http://example.com/api/upgrade/latest",
		"checksum":     strings.Repeat("a", 64),
		"signature":    "sig",
		"changelog":    "ok",
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, []byte("test-webhook-secret"))
	_, _ = mac.Write(raw)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/webhook/upgrade", bytes.NewReader(raw))
	req.Header.Set("X-Webhook-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook status=%d", resp.StatusCode)
	}

	reqBad, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/webhook/upgrade", bytes.NewReader(raw))
	reqBad.Header.Set("X-Webhook-Signature", "sha256=deadbeef")
	respBad, err := http.DefaultClient.Do(reqBad)
	if err != nil {
		t.Fatalf("webhook bad: %v", err)
	}
	respBad.Body.Close()
	if respBad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", respBad.StatusCode)
	}

	payload["timestamp"] = time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	rawStale, _ := json.Marshal(payload)
	mac2 := hmac.New(sha256.New, []byte("test-webhook-secret"))
	_, _ = mac2.Write(rawStale)
	sig2 := "sha256=" + hex.EncodeToString(mac2.Sum(nil))
	reqStale, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/webhook/upgrade", bytes.NewReader(rawStale))
	reqStale.Header.Set("X-Webhook-Signature", sig2)
	respStale, err := http.DefaultClient.Do(reqStale)
	if err != nil {
		t.Fatalf("webhook stale: %v", err)
	}
	respStale.Body.Close()
	if respStale.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 stale, got %d", respStale.StatusCode)
	}
}

func TestControl_UpgradeInfoAndTaskLogs(t *testing.T) {
	var portalURL string
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/upgrade/latest") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product":      r.URL.Query().Get("product"),
			"version":      "9.9.9",
			"channel":      r.URL.Query().Get("channel"),
			"platform":     r.URL.Query().Get("platform"),
			"arch":         r.URL.Query().Get("arch"),
			"download_url": portalURL + "/api/upgrade/file/xxx?tk=1",
			"checksum":     strings.Repeat("b", 64),
			"signature":    "sig",
			"sig_alg":      "ed25519",
			"sig_target":   "sha256",
			"pubkey":       "pub",
			"size_bytes":   123,
			"changelog":    "notes",
			"build_id":     "bid",
		})
	}))
	t.Cleanup(portal.Close)
	portalURL = portal.URL

	_, ts, token := newControlTestServer(t, portal.URL)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/system/upgrade?channel=stable", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upgrade info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upgrade info status=%d body=%s", resp.StatusCode, string(b))
	}
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if info["latest_version"] != "9.9.9" {
		t.Fatalf("unexpected latest_version=%v", info["latest_version"])
	}

	upgradeReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/system/upgrade/control", bytes.NewBufferString(`{"channel":"stable"}`))
	upgradeReq.Header.Set("Authorization", "Bearer "+token)
	upgradeReq.Header.Set("Content-Type", "application/json")
	upgradeResp, err := http.DefaultClient.Do(upgradeReq)
	if err != nil {
		t.Fatalf("upgrade control: %v", err)
	}
	defer upgradeResp.Body.Close()
	if upgradeResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(upgradeResp.Body)
		t.Fatalf("upgrade control status=%d body=%s", upgradeResp.StatusCode, string(b))
	}
	var out struct {
		TaskID string `json:"task_id"`
		Ok     bool   `json:"ok"`
	}
	if err := json.NewDecoder(upgradeResp.Body).Decode(&out); err != nil {
		t.Fatalf("decode upgrade resp: %v", err)
	}
	if !out.Ok || strings.TrimSpace(out.TaskID) == "" {
		t.Fatalf("task id missing")
	}

	time.Sleep(200 * time.Millisecond)
	logReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/system/upgrade/tasks/"+out.TaskID, nil)
	logReq.Header.Set("Authorization", "Bearer "+token)
	logResp, err := http.DefaultClient.Do(logReq)
	if err != nil {
		t.Fatalf("task logs: %v", err)
	}
	defer logResp.Body.Close()
	if logResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(logResp.Body)
		t.Fatalf("task logs status=%d body=%s", logResp.StatusCode, string(b))
	}
	var logsOut struct {
		Logs []map[string]any `json:"logs"`
	}
	if err := json.NewDecoder(logResp.Body).Decode(&logsOut); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	if len(logsOut.Logs) == 0 {
		t.Fatalf("expected logs")
	}
}

func TestControl_UpgradeNodes_LatestKeepsLatestWhenArchMismatch(t *testing.T) {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/upgrade/latest") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		version := "1.0.0-beta.1"
		if r.URL.Query().Get("arch") == "arm64" {
			version = "2.0.0"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product":      r.URL.Query().Get("product"),
			"version":      version,
			"channel":      r.URL.Query().Get("channel"),
			"platform":     r.URL.Query().Get("platform"),
			"arch":         r.URL.Query().Get("arch"),
			"download_url": "http://example.com",
			"checksum":     strings.Repeat("b", 64),
			"signature":    "sig",
			"sig_alg":      "ed25519",
			"sig_target":   "sha256",
			"pubkey":       "pub",
			"size_bytes":   123,
			"changelog":    "notes",
			"build_id":     "bid",
		})
	}))
	t.Cleanup(portal.Close)

	_, ts, token := newControlTestServer(t, portal.URL)
	raw := []byte(`{"node_ids":["n1"],"target_version":"latest","channel":"stable","force":false}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/system/upgrade/nodes", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upgrade nodes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upgrade nodes status=%d body=%s", resp.StatusCode, string(b))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["target"] != "latest" {
		t.Fatalf("expected target=latest, got %v", out["target"])
	}
}
