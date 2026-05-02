package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestAdminBalanceStats_WithDateAliasAndSplitFields(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	today := time.Now()
	day := today.Format("2006-01-02")

	adjustReqBody := map[string]any{
		"user_id":      "u-admin",
		"amount_cents": 1200,
		"note":         "",
	}
	rawAdjust, _ := json.Marshal(adjustReqBody)
	reqAdjust, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/balance/adjust", bytes.NewReader(rawAdjust))
	reqAdjust.Header.Set("Authorization", "Bearer "+token)
	reqAdjust.Header.Set("Content-Type", "application/json")
	respAdjust, err := http.DefaultClient.Do(reqAdjust)
	if err != nil {
		t.Fatalf("adjust request failed: %v", err)
	}
	defer respAdjust.Body.Close()
	if respAdjust.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(respAdjust.Body)
		t.Fatalf("adjust status=%d body=%s", respAdjust.StatusCode, string(b))
	}

	reqStats, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/balance/stats?start_date="+day+"&end_date="+day, nil)
	reqStats.Header.Set("Authorization", "Bearer "+token)
	respStats, err := http.DefaultClient.Do(reqStats)
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer respStats.Body.Close()
	if respStats.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(respStats.Body)
		t.Fatalf("stats status=%d body=%s", respStats.StatusCode, string(b))
	}

	var out struct {
		Stats []struct {
			Day           string `json:"day"`
			RechargeCents int64  `json:"recharge_cents"`
			RechargeCount int64  `json:"recharge_count"`
			AdjustCents   int64  `json:"adjust_cents"`
			AdjustCount   int64  `json:"adjust_count"`
			TotalCents    int64  `json:"total_cents"`
			TotalCount    int64  `json:"total_count"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(respStats.Body).Decode(&out); err != nil {
		t.Fatalf("decode stats failed: %v", err)
	}
	if len(out.Stats) == 0 {
		t.Fatalf("expected non-empty stats")
	}

	matched := false
	for _, s := range out.Stats {
		if s.Day != day {
			continue
		}
		matched = true
		if s.AdjustCount <= 0 {
			t.Fatalf("expected adjust_count > 0 for day %s", day)
		}
		if s.AdjustCents == 0 {
			t.Fatalf("expected adjust_cents != 0 for day %s", day)
		}
		if s.TotalCents != s.RechargeCents+s.AdjustCents {
			t.Fatalf("total_cents mismatch: got %d expect %d", s.TotalCents, s.RechargeCents+s.AdjustCents)
		}
		if s.TotalCount != s.RechargeCount+s.AdjustCount {
			t.Fatalf("total_count mismatch: got %d expect %d", s.TotalCount, s.RechargeCount+s.AdjustCount)
		}
	}
	if !matched {
		t.Fatalf("expected stats item for day %s", day)
	}

	reqStatsAlias, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/balance/stats?from="+day+"&to="+day, nil)
	reqStatsAlias.Header.Set("Authorization", "Bearer "+token)
	respStatsAlias, err := http.DefaultClient.Do(reqStatsAlias)
	if err != nil {
		t.Fatalf("stats alias request failed: %v", err)
	}
	defer respStatsAlias.Body.Close()
	if respStatsAlias.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(respStatsAlias.Body)
		t.Fatalf("stats alias status=%d body=%s", respStatsAlias.StatusCode, string(b))
	}
}

func TestAdminBalanceStats_InvalidDateRange(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/balance/stats?from=2026-02-08&to=2026-02-07", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stats invalid range request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d, body=%s", resp.StatusCode, string(b))
	}
}
