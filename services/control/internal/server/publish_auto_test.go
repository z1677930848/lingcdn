package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestAutoPublish_CreatesPublishTaskInTaskCenter(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	key := "email.register_code.text"
	body := bytes.NewBufferString(`{"content":"hello {{code}}"}`)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/system/templates/"+key, body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put status=%d body=%s", resp.StatusCode, string(b))
	}
	var out struct {
		OK            bool   `json:"ok"`
		PublishTaskID string `json:"publish_task_id"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.OK || out.PublishTaskID == "" {
		t.Fatalf("expected publish_task_id, body=%s", string(b))
	}

	time.Sleep(30 * time.Millisecond)

	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/system/tasks?source=publish", nil)
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
	var listOut struct {
		Tasks []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			Type   string `json:"type"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(lb, &listOut); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	found := false
	for _, tsk := range listOut.Tasks {
		if tsk.ID == out.PublishTaskID && tsk.Source == "publish" && tsk.Type == "publish.config" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("publish task not found in task center, body=%s", string(lb))
	}

	detailReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/system/publish/tasks/"+out.PublishTaskID, nil)
	detailReq.Header.Set("Authorization", "Bearer "+token)
	detailResp, err := http.DefaultClient.Do(detailReq)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	db, _ := io.ReadAll(detailResp.Body)
	detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailResp.StatusCode, string(db))
	}
}

