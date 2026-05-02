package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// TestAdminCreateDomainWithLineGroupID reproduces the end-to-end path the UI
// uses when an admin creates a website: POST /api/domains with
// {name, line_group_id, origin_id} in the payload and Authorization: Bearer
// <admin token>. Regression guard against the "line_group_id required" 400
// reported after refactoring handlers into handlers_domains.go — the server
// must accept a payload that explicitly carries the cluster id even when the
// user_id is absent.
func TestAdminCreateDomainWithLineGroupID(t *testing.T) {
	srv, ts, adminToken := newControlTestServer(t, "")
	ctx := context.Background()

	if err := srv.store.CreateCluster(ctx, &store.Cluster{
		ID:        "cluster-test",
		Name:      "test",
		DNSZone:   "test.example.com",
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := srv.store.CreateOrigin(ctx, &store.Origin{
		ID:        "origin-test",
		Name:      "origin-test",
		Addresses: []string{"10.0.0.1"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create origin: %v", err)
	}

	body := []byte(`{"id":"","name":"aaa.com","line_group_id":"cluster-test","origin_id":"origin-test","cache_enabled":true,"http2_enabled":true}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/domains", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("create domain status=%d body=%s", resp.StatusCode, string(raw))
	}

	var out store.Domain
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode domain: %v body=%s", err, string(raw))
	}
	if out.LineGroupID != "cluster-test" {
		t.Fatalf("expected LineGroupID=cluster-test got %q", out.LineGroupID)
	}
	if out.Name != "aaa.com" {
		t.Fatalf("expected Name=aaa.com got %q", out.Name)
	}
}

// TestAdminCreateDomainMissingLineGroupID asserts that the diagnostic
// 400 is raised only when the payload omits the field AND the store has
// no clusters at all (so the admin fallback can't pick one). The error
// body merges hint text into `error` so both new and old UIs surface it.
func TestAdminCreateDomainMissingLineGroupID(t *testing.T) {
	_, ts, adminToken := newControlTestServer(t, "")

	body := []byte(`{"name":"bbb.com","origin_id":"does-not-matter"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/domains", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, string(raw))
	}
	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &errResp); err != nil {
		t.Fatalf("decode err: %v body=%s", err, string(raw))
	}
	if !strings.Contains(errResp.Error, "line_group_id required") {
		t.Fatalf("expected error to contain 'line_group_id required', got %q", errResp.Error)
	}
	if !strings.Contains(errResp.Error, "集群管理") {
		t.Fatalf("expected error to contain operator hint '集群管理', got %q", errResp.Error)
	}
}

// TestAdminCreateDomainAutoPicksSingleCluster verifies the admin convenience
// fallback: when the payload omits line_group_id but the system has exactly
// one cluster configured, the server binds the new domain to that cluster
// instead of returning a 400. This is the fresh-install recovery path we
// added after the "line_group_id required" reports.
func TestAdminCreateDomainAutoPicksSingleCluster(t *testing.T) {
	srv, ts, adminToken := newControlTestServer(t, "")
	ctx := context.Background()
	if err := srv.store.CreateCluster(ctx, &store.Cluster{
		ID:        "cluster-sole",
		Name:      "sole",
		DNSZone:   "sole.example.com",
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := srv.store.CreateOrigin(ctx, &store.Origin{
		ID:        "origin-sole",
		Name:      "origin-sole",
		Addresses: []string{"10.0.0.1"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create origin: %v", err)
	}

	body := []byte(`{"name":"ccc.com","origin_id":"origin-sole","cache_enabled":true}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/domains", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected success, got %d body=%s", resp.StatusCode, string(raw))
	}
	var out store.Domain
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode domain: %v body=%s", err, string(raw))
	}
	if out.LineGroupID != "cluster-sole" {
		t.Fatalf("expected auto-bound LineGroupID=cluster-sole, got %q", out.LineGroupID)
	}
}

// TestAdminCreateDomainMultiClusterPicksEnabled covers the broadened
// fallback for multi-cluster environments: when several clusters exist and
// the admin omits line_group_id, the server picks an enabled cluster
// instead of 400'ing, skipping disabled ones. The memory store returns
// clusters in map iteration order (unstable), so the assertion only checks
// that the chosen cluster is one of the enabled candidates.
func TestAdminCreateDomainMultiClusterPicksEnabled(t *testing.T) {
	srv, ts, adminToken := newControlTestServer(t, "")
	ctx := context.Background()

	// First: a disabled cluster. The fallback must skip it.
	if err := srv.store.CreateCluster(ctx, &store.Cluster{
		ID:        "cluster-offline",
		Name:      "offline",
		DNSZone:   "offline.example.com",
		Enabled:   false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create offline cluster: %v", err)
	}
	// A pair of enabled clusters — fallback should pick one of them, never
	// the offline one above.
	if err := srv.store.CreateCluster(ctx, &store.Cluster{
		ID:        "cluster-primary",
		Name:      "primary",
		DNSZone:   "primary.example.com",
		Enabled:   true,
		CreatedAt: time.Now().Add(time.Second),
		UpdatedAt: time.Now().Add(time.Second),
	}); err != nil {
		t.Fatalf("create primary cluster: %v", err)
	}
	if err := srv.store.CreateCluster(ctx, &store.Cluster{
		ID:        "cluster-secondary",
		Name:      "secondary",
		DNSZone:   "secondary.example.com",
		Enabled:   true,
		CreatedAt: time.Now().Add(2 * time.Second),
		UpdatedAt: time.Now().Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("create secondary cluster: %v", err)
	}
	if err := srv.store.CreateOrigin(ctx, &store.Origin{
		ID:        "origin-multi",
		Name:      "origin-multi",
		Addresses: []string{"10.0.0.1"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create origin: %v", err)
	}

	body := []byte(`{"name":"ddd.com","origin_id":"origin-multi","cache_enabled":true}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/domains", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected success, got %d body=%s", resp.StatusCode, string(raw))
	}
	var out store.Domain
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode domain: %v body=%s", err, string(raw))
	}
	if out.LineGroupID != "cluster-primary" && out.LineGroupID != "cluster-secondary" {
		t.Fatalf("expected auto-bound LineGroupID to be one of the enabled clusters, got %q", out.LineGroupID)
	}
	if out.LineGroupID == "cluster-offline" {
		t.Fatalf("fallback must not pick a disabled cluster, got %q", out.LineGroupID)
	}
}
