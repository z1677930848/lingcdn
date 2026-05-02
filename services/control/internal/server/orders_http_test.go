package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/lingcdn/control/internal/store"
)

func TestAdminOrdersListFilters(t *testing.T) {
	srv, ts, adminToken := newControlTestServer(t, "")

	now := time.Now()
	_ = srv.store.CreateProduct(context.Background(), &store.Product{
		ID:          "p1",
		Name:        "Starter",
		Slug:        "starter",
		Description: "d",
		PriceCents:  1000,
		Currency:    "CNY",
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	_ = srv.store.CreateOrder(context.Background(), &store.Order{
		ID:          "o1",
		UserID:      "u1",
		ProductID:   "p1",
		ProductName: "Starter",
		AmountCents: 1000,
		Currency:    "CNY",
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	_ = srv.store.CreateOrder(context.Background(), &store.Order{
		ID:          "o2",
		UserID:      "u2",
		ProductID:   "p1",
		ProductName: "Starter",
		AmountCents: 1000,
		Currency:    "CNY",
		Status:      "paid",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	qs := url.Values{}
	qs.Set("status", "pending")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/orders?"+qs.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list admin orders: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(b))
	}
	var out struct {
		Orders []store.Order `json:"orders"`
		Total  int           `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 1 || len(out.Orders) != 1 || out.Orders[0].ID != "o1" {
		t.Fatalf("unexpected result: total=%d len=%d", out.Total, len(out.Orders))
	}

	qs2 := url.Values{}
	qs2.Set("user_id", "u2")
	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/orders?"+qs2.Encode(), nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("list admin orders user filter: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("status=%d body=%s", resp2.StatusCode, string(b))
	}
	var out2 struct {
		Orders []store.Order `json:"orders"`
		Total  int           `json:"total"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&out2); err != nil {
		t.Fatalf("decode2: %v", err)
	}
	if out2.Total != 1 || len(out2.Orders) != 1 || out2.Orders[0].ID != "o2" {
		t.Fatalf("unexpected user filtered result: total=%d len=%d", out2.Total, len(out2.Orders))
	}
}

func TestAdminOrdersCreateAndPatch(t *testing.T) {
	srv, ts, adminToken := newControlTestServer(t, "")

	now := time.Now()
	_ = srv.store.CreateProduct(context.Background(), &store.Product{
		ID:               "p1",
		Name:             "Starter",
		Slug:             "starter",
		Description:      "d",
		PriceMonthCents:  1000,
		PriceQuarterCents: 2500,
		PriceYearCents:   9000,
		Currency:         "CNY",
		Enabled:          true,
		CreatedAt:        now,
		UpdatedAt:        now,
	})

	createBody := []byte(`{"user_id":"u1","product_id":"p1","period":"month","quantity":2,"status":"paid","note":"n"}`)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/orders", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create status=%d body=%s", createResp.StatusCode, string(b))
	}
	var created struct {
		Order store.Order `json:"order"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Order.ID == "" || created.Order.AmountCents != 2000 {
		t.Fatalf("unexpected created order: id=%s amount=%d", created.Order.ID, created.Order.AmountCents)
	}

	patchBody := []byte(`{"status":"cancelled"}`)
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/admin/orders/"+created.Order.ID, bytes.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer "+adminToken)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch order: %v", err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(patchResp.Body)
		t.Fatalf("patch status=%d body=%s", patchResp.StatusCode, string(b))
	}
}
