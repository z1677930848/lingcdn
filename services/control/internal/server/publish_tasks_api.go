package server

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (s *Servers) handlePublishTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": listPublishTasks(200)})
}

func (s *Servers) handlePublishTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/system/publish/tasks/")
	id = strings.Trim(id, "/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}
	t, ok := getPublishTask(id)
	if !ok || t == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "任务不存在"})
		return
	}
	type nodeStatus struct {
		NodeID  string `json:"node_id"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	nodes := make([]nodeStatus, 0, len(t.NodeIDs))
	for _, nid := range t.NodeIDs {
		st := "success"
		msg := ""
		if t.Errors != nil {
			if errMsg, ok := t.Errors[nid]; ok && strings.TrimSpace(errMsg) != "" {
				st = "failed"
				msg = errMsg
			}
		}
		nodes = append(nodes, nodeStatus{NodeID: nid, Status: st, Message: msg})
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": t, "nodes": nodes})
}

// handleSyncActive returns running tasks plus failed tasks within the last 5
// minutes, used by the UI to bootstrap sync indicators before the SSE stream
// catches up. The optional `subject` query parameter is treated as a prefix
// filter (e.g. `domain:` matches every domain task; `domain:abc` matches one).
//
// Authorization: admins see everything. Non-admin users only see subjects
// belonging to domains they own (subject form `domain:<id>`); other subjects
// are filtered out.
func (s *Servers) handleSyncActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx := r.Context()
	subjectFilter := strings.TrimSpace(r.URL.Query().Get("subject"))

	ownedDomains, adminAll := s.syncOwnedDomainIDs(ctx)
	if !adminAll && len(ownedDomains) == 0 && getUserID(ctx) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"tasks": []any{}})
		return
	}

	allowSubject := func(sub string) bool {
		if subjectFilter != "" && !strings.HasPrefix(sub, subjectFilter) {
			return false
		}
		return s.allowSyncSubject(adminAll, ownedDomains, sub)
	}

	cutoff := time.Now().Add(-5 * time.Minute)
	out := make([]syncTaskEvent, 0, 16)

	for _, t := range listPublishTasks(200) {
		if t == nil || t.Subject == "" {
			continue
		}
		if !allowSubject(t.Subject) {
			continue
		}
		if t.Status != "running" && !(t.Status == "failed" && t.CompletedAt.After(cutoff)) {
			continue
		}
		out = append(out, syncTaskEvent{
			Kind:        "publish",
			ID:          t.ID,
			Subject:     t.Subject,
			Status:      t.Status,
			Message:     t.Message,
			StartedAt:   t.StartedAt,
			CompletedAt: t.CompletedAt,
		})
	}

	dnsTaskMu.Lock()
	dnsCopy := make([]*dnsTask, len(dnsTaskList))
	copy(dnsCopy, dnsTaskList)
	dnsTaskMu.Unlock()
	for _, t := range dnsCopy {
		if t == nil || t.Subject == "" {
			continue
		}
		if !allowSubject(t.Subject) {
			continue
		}
		completed := time.Time{}
		if t.Status == "success" || t.Status == "failed" {
			completed = t.UpdatedAt
		}
		if t.Status != "running" && !(t.Status == "failed" && completed.After(cutoff)) {
			continue
		}
		out = append(out, syncTaskEvent{
			Kind:        "dns",
			ID:          t.ID,
			Subject:     t.Subject,
			Status:      t.Status,
			Message:     t.Message,
			StartedAt:   t.CreatedAt,
			CompletedAt: completed,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"tasks": out})
}

func (s *Servers) syncOwnedDomainIDs(ctx context.Context) (owned map[string]bool, adminAll bool) {
	if isAdmin(ctx) {
		return nil, true
	}
	owned = map[string]bool{}
	userID := getUserID(ctx)
	if userID == "" {
		return owned, false
	}
	domains, err := s.store.ListDomainsByUser(ctx, userID)
	if err != nil {
		return owned, false
	}
	for _, d := range domains {
		if d != nil {
			owned[d.ID] = true
		}
	}
	return owned, false
}

func (s *Servers) allowSyncSubject(adminAll bool, owned map[string]bool, sub string) bool {
	if adminAll {
		return true
	}
	if strings.HasPrefix(sub, "domain:") {
		return owned[strings.TrimPrefix(sub, "domain:")]
	}
	return false
}
