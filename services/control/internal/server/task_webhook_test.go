package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestTaskWebhook_IngestAndList(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	event := TaskWebhookEvent{
		ID:        "task-1",
		RelID:     "83",
		Source:    "node",
		Type:      "node.sync.data",
		Message:   "rpc error: unavailable",
		Status:    "failed",
		SubTasks:  3,
		Retryable: true,
		DetailURL: "/admin/dashboard/nodes/83",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(event)
	mac := hmac.New(sha256.New, []byte("test-webhook-secret"))
	_, _ = mac.Write(raw)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/webhook/tasks", bytes.NewReader(raw))
	req.Header.Set("X-Webhook-Signature", sig)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook status=%d body=%s", resp.StatusCode, string(b))
	}

	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/system/tasks?source=node", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lb, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResp.StatusCode, string(lb))
	}
	var out struct {
		Tasks []struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(lb, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, tsk := range out.Tasks {
		if tsk.ID == "task-1" {
			t.Fatalf("reported tasks should not be listed, body=%s", string(lb))
		}
	}
}
