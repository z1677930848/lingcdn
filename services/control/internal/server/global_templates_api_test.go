package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestGlobalTemplates_CRUD(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	do := func(method, path string, body any) (*http.Response, []byte) {
		t.Helper()
		var r io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			r = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, ts.URL+path, r)
		if err != nil {
			t.Fatalf("new req: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, b
	}

	resp, b := do(http.MethodGet, "/api/system/templates", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.StatusCode, string(b))
	}
	var list struct {
		Templates []struct {
			Key            string `json:"key"`
			DefaultContent string `json:"default_content"`
			Content        string `json:"content"`
			Customized     bool   `json:"customized"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(b, &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list.Templates) == 0 {
		t.Fatalf("expected templates")
	}
	key := "email.register_code.text"
	var before *struct {
		Key            string `json:"key"`
		DefaultContent string `json:"default_content"`
		Content        string `json:"content"`
		Customized     bool   `json:"customized"`
	}
	for i := range list.Templates {
		if list.Templates[i].Key == key {
			before = &list.Templates[i]
			break
		}
	}
	if before == nil {
		t.Fatalf("missing key %s", key)
	}
	if before.Customized {
		t.Fatalf("expected default customized=false")
	}
	if before.Content != before.DefaultContent {
		t.Fatalf("expected content=default_content")
	}

	newContent := "hello {{system_name}} code={{code}}"
	resp2, b2 := do(http.MethodPut, "/api/system/templates/"+key, map[string]any{"content": newContent})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("put status=%d body=%s", resp2.StatusCode, string(b2))
	}

	resp3, b3 := do(http.MethodGet, "/api/system/templates/"+key, nil)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("get status=%d body=%s", resp3.StatusCode, string(b3))
	}
	var one struct {
		Template struct {
			Key        string `json:"key"`
			Content    string `json:"content"`
			Customized bool   `json:"customized"`
		} `json:"template"`
	}
	if err := json.Unmarshal(b3, &one); err != nil {
		t.Fatalf("unmarshal one: %v", err)
	}
	if one.Template.Key != key {
		t.Fatalf("key=%s", one.Template.Key)
	}
	if one.Template.Content != newContent {
		t.Fatalf("content mismatch")
	}
	if !one.Template.Customized {
		t.Fatalf("expected customized=true")
	}

	resp4, b4 := do(http.MethodPost, "/api/system/templates/"+key+"/reset", nil)
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", resp4.StatusCode, string(b4))
	}

	resp5, b5 := do(http.MethodGet, "/api/system/templates/"+key, nil)
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("get after reset status=%d body=%s", resp5.StatusCode, string(b5))
	}
	if err := json.Unmarshal(b5, &one); err != nil {
		t.Fatalf("unmarshal one after reset: %v", err)
	}
	if one.Template.Customized {
		t.Fatalf("expected customized=false after reset")
	}
	if one.Template.Content == newContent {
		t.Fatalf("expected reset to default")
	}
}

