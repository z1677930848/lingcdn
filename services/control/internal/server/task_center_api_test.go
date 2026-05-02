package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestTaskCenter_ListAndRetry(t *testing.T) {
	s, ts, token := newControlTestServer(t, "")

	dns := s.runDNSTask("sync", "", "auto", func() (string, error) { return "ok", nil })
	s.saveUpgradeTask(context.Background(), &upgradeTask{
		ID:            "up-1",
		Type:          "control",
		TargetVersion: "1.0.0-beta.1",
		Channel:       "stable",
		Status:        "failed",
		CreatedAt:     time.Now(),
	})

	time.Sleep(30 * time.Millisecond)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/system/tasks?limit=50", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.StatusCode, string(b))
	}
	var out struct {
		Summary map[string]any `json:"summary"`
		Tasks   []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Tasks) == 0 {
		t.Fatalf("expected tasks")
	}

	retryReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/system/tasks/"+dns.ID+"/retry", bytes.NewReader([]byte(`{}`)))
	retryReq.Header.Set("Authorization", "Bearer "+token)
	retryReq.Header.Set("Content-Type", "application/json")
	retryResp, err := http.DefaultClient.Do(retryReq)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	rb, _ := io.ReadAll(retryResp.Body)
	retryResp.Body.Close()
	if retryResp.StatusCode != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", retryResp.StatusCode, string(rb))
	}
	var retryOut struct {
		OK     bool   `json:"ok"`
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(rb, &retryOut); err != nil {
		t.Fatalf("retry unmarshal: %v", err)
	}
	if !retryOut.OK || retryOut.TaskID == "" {
		t.Fatalf("retry response invalid: %s", string(rb))
	}
}
