package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lingcdn/control/internal/store"
)

func TestHandleBalanceWithdrawalsCreate(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	ctx := context.Background()

	user := &store.User{ID: "user-1", Username: "alice", Email: "a@example.com", Role: "user"}
	if err := mem.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := mem.AdminAdjustBalance(ctx, user.ID, 10000, "seed"); err != nil {
		t.Fatalf("seed balance: %v", err)
	}

	s := newTestServers(t, mem)

	body, _ := json.Marshal(map[string]any{
		"amount_cents":  5000,
		"method":        "alipay",
		"account_name":  "Alice",
		"account_no":    "13800138000",
		"note":          "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/balance/withdrawals", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyUserID, user.ID))
	w := httptest.NewRecorder()
	s.handleBalanceWithdrawals(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Withdrawal store.BalanceWithdrawal `json:"withdrawal"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Withdrawal.Status != "pending" {
		t.Fatalf("expected pending status, got %q", resp.Withdrawal.Status)
	}
	if resp.Withdrawal.AmountCents != 5000 {
		t.Fatalf("expected 5000 cents, got %d", resp.Withdrawal.AmountCents)
	}

	// Second pending withdrawal should be rejected.
	req2 := httptest.NewRequest(http.MethodPost, "/api/balance/withdrawals", bytes.NewReader(body))
	req2 = req2.WithContext(context.WithValue(req2.Context(), ctxKeyUserID, user.ID))
	w2 := httptest.NewRecorder()
	s.handleBalanceWithdrawals(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate pending, got %d body=%s", w2.Code, w2.Body.String())
	}
}

func TestHandleBalanceWithdrawalsInsufficientBalance(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	ctx := context.Background()

	user := &store.User{ID: "user-2", Username: "bob", Email: "b@example.com", Role: "user"}
	if err := mem.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	s := newTestServers(t, mem)

	body, _ := json.Marshal(map[string]any{
		"amount_cents": 100,
		"method":       "alipay",
		"account_name": "Bob",
		"account_no":   "123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/balance/withdrawals", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyUserID, user.ID))
	w := httptest.NewRecorder()
	s.handleBalanceWithdrawals(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}
