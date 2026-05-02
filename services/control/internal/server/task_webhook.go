package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type TaskWebhookEvent struct {
	ID        string `json:"id"`
	RelID     string `json:"rel_id"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Status    string `json:"status"`
	SubTasks  int    `json:"sub_tasks"`
	Retryable bool   `json:"retryable"`
	DetailURL string `json:"detail_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Timestamp string `json:"timestamp"`
}

func (s *Servers) HandleTaskWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	signature := r.Header.Get("X-Webhook-Signature")
	if !s.verifyWebhookSignature(body, signature) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var event TaskWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" {
		http.Error(w, "id/type required", http.StatusBadRequest)
		return
	}

	if event.Timestamp != "" {
		ts, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			http.Error(w, "invalid timestamp", http.StatusUnauthorized)
			return
		}
		if time.Since(ts).Abs() > 5*time.Minute {
			http.Error(w, "stale request", http.StatusUnauthorized)
			return
		}
	}

	createdAt := time.Now()
	updatedAt := time.Now()
	if strings.TrimSpace(event.CreatedAt) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(event.CreatedAt)); err == nil {
			createdAt = t
		}
	}
	if strings.TrimSpace(event.UpdatedAt) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(event.UpdatedAt)); err == nil {
			updatedAt = t
		}
	}

	src := strings.TrimSpace(event.Source)
	if src == "" {
		src = "external"
	}
	st := strings.ToLower(strings.TrimSpace(event.Status))
	if st == "" {
		st = "unknown"
	}

	upsertExternalTask(externalTask{
		ID:        strings.TrimSpace(event.ID),
		RelID:     strings.TrimSpace(event.RelID),
		Source:    src,
		Type:      strings.TrimSpace(event.Type),
		Message:   strings.TrimSpace(event.Message),
		Status:    st,
		SubTasks:  event.SubTasks,
		Retryable: event.Retryable,
		DetailURL: strings.TrimSpace(event.DetailURL),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

