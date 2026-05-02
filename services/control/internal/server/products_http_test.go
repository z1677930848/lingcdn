package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/lingcdn/control/internal/store"
)

// TestProductCreateAcceptsClusterIDAlias asserts the server accepts the
// `cluster_id` alias the Admin UI submits when binding a plan to a cluster.
// Before this fix the handler only read `line_group_id`, silently dropping
// the cluster binding and leaving plans "unlinked" after save.
func TestProductCreateAcceptsClusterIDAlias(t *testing.T) {
	_, ts, adminToken := newControlTestServer(t, "")

	body := []byte(`{"name":"ClusterAlias","slug":"cluster-alias","description":"x","price_cents":100,"currency":"CNY","enabled":true,"cluster_id":"cluster-xyz"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/products", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, string(raw))
	}

	var created struct {
		Product struct {
			ID          string `json:"id"`
			LineGroupID string `json:"line_group_id"`
		} `json:"product"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode: %v body=%s", err, string(raw))
	}
	if created.Product.LineGroupID != "cluster-xyz" {
		t.Fatalf("cluster_id alias not persisted: got LineGroupID=%q body=%s", created.Product.LineGroupID, string(raw))
	}
}

// TestProductPatchAcceptsClusterIDAlias verifies the same alias works for
// the PATCH path used by the Edit dialog in the Admin UI.
func TestProductPatchAcceptsClusterIDAlias(t *testing.T) {
	_, ts, adminToken := newControlTestServer(t, "")

	create := []byte(`{"name":"PatchAlias","slug":"patch-alias","description":"x","price_cents":100,"currency":"CNY","enabled":true}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/products", bytes.NewReader(create))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, string(raw))
	}
	var created struct {
		Product struct {
			ID string `json:"id"`
		} `json:"product"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode create: %v body=%s", err, string(raw))
	}

	patch := []byte(`{"cluster_id":"cluster-after-patch"}`)
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/products/"+created.Product.ID, bytes.NewReader(patch))
	patchReq.Header.Set("Authorization", "Bearer "+adminToken)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	defer patchResp.Body.Close()
	patchRaw, _ := io.ReadAll(patchResp.Body)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patchResp.StatusCode, string(patchRaw))
	}

	getReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/products/"+created.Product.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer getResp.Body.Close()
	getRaw, _ := io.ReadAll(getResp.Body)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getResp.StatusCode, string(getRaw))
	}
	var got struct {
		Product struct {
			LineGroupID string `json:"line_group_id"`
		} `json:"product"`
	}
	if err := json.Unmarshal(getRaw, &got); err != nil {
		t.Fatalf("decode get: %v body=%s", err, string(getRaw))
	}
	if got.Product.LineGroupID != "cluster-after-patch" {
		t.Fatalf("patch cluster_id alias not persisted: LineGroupID=%q body=%s", got.Product.LineGroupID, string(getRaw))
	}
}

func loginToken(t *testing.T, base, identifier, password string) string {
	t.Helper()
	reqBody := bytes.NewBufferString(`{"identifier":"` + identifier + `","password":"` + password + `"}`)
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if out.Token == "" {
		t.Fatalf("missing token")
	}
	return out.Token
}

func TestProductsCRUDAndVisibility(t *testing.T) {
	srv, ts, adminToken := newControlTestServer(t, "")

	hash, err := bcrypt.GenerateFromPassword([]byte("user12345"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	_ = srv.store.CreateUser(context.Background(), &store.User{
		ID:           "u-user",
		Username:     "user",
		Email:        "user@example.com",
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	})
	userToken := loginToken(t, ts.URL, "user", "user12345")

	createBody := []byte(`{"name":"Starter","slug":"starter","description":"d","price_cents":1000,"currency":"CNY","enabled":false}`)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/products", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create status=%d body=%s", createResp.StatusCode, string(b))
	}
	var created struct {
		Product struct {
			ID string `json:"id"`
		} `json:"product"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Product.ID == "" {
		t.Fatalf("missing product id")
	}

	userListReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/products", nil)
	userListReq.Header.Set("Authorization", "Bearer "+userToken)
	userListResp, err := http.DefaultClient.Do(userListReq)
	if err != nil {
		t.Fatalf("user list: %v", err)
	}
	defer userListResp.Body.Close()
	if userListResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(userListResp.Body)
		t.Fatalf("user list status=%d body=%s", userListResp.StatusCode, string(b))
	}
	var userList struct {
		Products []struct {
			ID string `json:"id"`
		} `json:"products"`
	}
	_ = json.NewDecoder(userListResp.Body).Decode(&userList)
	for _, p := range userList.Products {
		if p.ID == created.Product.ID {
			t.Fatalf("expected disabled product hidden from user")
		}
	}

	adminListReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/products", nil)
	adminListReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminListResp, err := http.DefaultClient.Do(adminListReq)
	if err != nil {
		t.Fatalf("admin list: %v", err)
	}
	defer adminListResp.Body.Close()
	if adminListResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(adminListResp.Body)
		t.Fatalf("admin list status=%d body=%s", adminListResp.StatusCode, string(b))
	}
	var adminList struct {
		Products []struct {
			ID string `json:"id"`
		} `json:"products"`
	}
	_ = json.NewDecoder(adminListResp.Body).Decode(&adminList)
	found := false
	for _, p := range adminList.Products {
		if p.ID == created.Product.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected product in admin list")
	}

	patchBody := []byte(`{"enabled":true,"price_cents":2000}`)
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/products/"+created.Product.ID, bytes.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer "+adminToken)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch product: %v", err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(patchResp.Body)
		t.Fatalf("patch status=%d body=%s", patchResp.StatusCode, string(b))
	}

	userListReq2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/products", nil)
	userListReq2.Header.Set("Authorization", "Bearer "+userToken)
	userListResp2, err := http.DefaultClient.Do(userListReq2)
	if err != nil {
		t.Fatalf("user list2: %v", err)
	}
	defer userListResp2.Body.Close()
	if userListResp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(userListResp2.Body)
		t.Fatalf("user list2 status=%d body=%s", userListResp2.StatusCode, string(b))
	}
	var userList2 struct {
		Products []struct {
			ID string `json:"id"`
		} `json:"products"`
	}
	_ = json.NewDecoder(userListResp2.Body).Decode(&userList2)
	found = false
	for _, p := range userList2.Products {
		if p.ID == created.Product.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected enabled product visible to user")
	}

	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/products/"+created.Product.ID, nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete product: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(delResp.Body)
		t.Fatalf("delete status=%d body=%s", delResp.StatusCode, string(b))
	}
}

