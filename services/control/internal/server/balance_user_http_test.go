package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/lingcdn/control/internal/store"
)

func loginTokenWithPassword(t *testing.T, base, identifier, password string) string {
	t.Helper()
	reqBody, _ := json.Marshal(map[string]any{
		"identifier": identifier,
		"password":   password,
	})
	req, err := http.NewRequest(http.MethodPost, base+"/api/auth/login", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new login request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("login status=%d body=%s", resp.StatusCode, string(body))
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode login response failed: %v", err)
	}
	if out.Token == "" {
		t.Fatalf("empty token returned")
	}
	return out.Token
}

func TestUserBalanceEndpoints_ReturnOwnData(t *testing.T) {
	s, ts, adminToken := newControlTestServer(t, "")

	hash, err := bcrypt.GenerateFromPassword([]byte("user123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if err := s.store.CreateUser(context.Background(), &store.User{
		ID:           "u-user-1",
		Username:     "user1",
		Email:        "user1@example.com",
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
	}); err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	userToken := loginTokenWithPassword(t, ts.URL, "user1", "user123")

	adjustRaw, _ := json.Marshal(map[string]any{
		"user_id":      "u-user-1",
		"amount_cents": 12345,
		"note":         "seed balance",
	})
	adjustReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/balance/adjust", bytes.NewReader(adjustRaw))
	adjustReq.Header.Set("Authorization", "Bearer "+adminToken)
	adjustReq.Header.Set("Content-Type", "application/json")
	adjustResp, err := http.DefaultClient.Do(adjustReq)
	if err != nil {
		t.Fatalf("adjust request failed: %v", err)
	}
	defer adjustResp.Body.Close()
	if adjustResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(adjustResp.Body)
		t.Fatalf("adjust status=%d body=%s", adjustResp.StatusCode, string(body))
	}

	accountReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/balance/account", nil)
	accountReq.Header.Set("Authorization", "Bearer "+userToken)
	accountResp, err := http.DefaultClient.Do(accountReq)
	if err != nil {
		t.Fatalf("account request failed: %v", err)
	}
	defer accountResp.Body.Close()
	if accountResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(accountResp.Body)
		t.Fatalf("account status=%d body=%s", accountResp.StatusCode, string(body))
	}
	var accountOut struct {
		Account struct {
			UserID       string `json:"user_id"`
			BalanceCents int64  `json:"balance_cents"`
			Currency     string `json:"currency"`
		} `json:"account"`
	}
	if err := json.NewDecoder(accountResp.Body).Decode(&accountOut); err != nil {
		t.Fatalf("decode account failed: %v", err)
	}
	if accountOut.Account.UserID != "u-user-1" {
		t.Fatalf("unexpected account user_id: %s", accountOut.Account.UserID)
	}
	if accountOut.Account.BalanceCents != 12345 {
		t.Fatalf("unexpected account balance: %d", accountOut.Account.BalanceCents)
	}
	if accountOut.Account.Currency == "" {
		t.Fatalf("expected currency to be non-empty")
	}

	txReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/balance/transactions?page=1&page_size=20", nil)
	txReq.Header.Set("Authorization", "Bearer "+userToken)
	txResp, err := http.DefaultClient.Do(txReq)
	if err != nil {
		t.Fatalf("transactions request failed: %v", err)
	}
	defer txResp.Body.Close()
	if txResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(txResp.Body)
		t.Fatalf("transactions status=%d body=%s", txResp.StatusCode, string(body))
	}
	var txOut struct {
		Transactions []struct {
			UserID      string `json:"user_id"`
			AmountCents int64  `json:"amount_cents"`
		} `json:"transactions"`
		Total int64 `json:"total"`
	}
	if err := json.NewDecoder(txResp.Body).Decode(&txOut); err != nil {
		t.Fatalf("decode transactions failed: %v", err)
	}
	if len(txOut.Transactions) == 0 || txOut.Total == 0 {
		t.Fatalf("expected non-empty transactions")
	}
	if txOut.Transactions[0].UserID != "u-user-1" {
		t.Fatalf("unexpected tx user_id: %s", txOut.Transactions[0].UserID)
	}
	if txOut.Transactions[0].AmountCents != 12345 {
		t.Fatalf("unexpected tx amount: %d", txOut.Transactions[0].AmountCents)
	}

	rechargeReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/balance/recharges?page=1&page_size=20", nil)
	rechargeReq.Header.Set("Authorization", "Bearer "+userToken)
	rechargeResp, err := http.DefaultClient.Do(rechargeReq)
	if err != nil {
		t.Fatalf("recharges request failed: %v", err)
	}
	defer rechargeResp.Body.Close()
	if rechargeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(rechargeResp.Body)
		t.Fatalf("recharges status=%d body=%s", rechargeResp.StatusCode, string(body))
	}

	withdrawReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/balance/withdrawals?page=1&page_size=20", nil)
	withdrawReq.Header.Set("Authorization", "Bearer "+userToken)
	withdrawResp, err := http.DefaultClient.Do(withdrawReq)
	if err != nil {
		t.Fatalf("withdrawals request failed: %v", err)
	}
	defer withdrawResp.Body.Close()
	if withdrawResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(withdrawResp.Body)
		t.Fatalf("withdrawals status=%d body=%s", withdrawResp.StatusCode, string(body))
	}
}
