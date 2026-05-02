package server

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/dnsprovider"
	"github.com/lingcdn/control/internal/store"
)

type systemTask struct {
	ID        string    `json:"id"`
	RelID     string    `json:"rel_id,omitempty"`
	Source    string    `json:"source"` // dns | upgrade
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Status    string    `json:"status"` // pending | running | success | failed | unknown
	SubTasks  int       `json:"sub_tasks"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Retryable bool   `json:"retryable"`
	DetailURL string `json:"detail_url,omitempty"`
}

type systemTaskSummary struct {
	Tasks    int `json:"tasks"`
	SubTasks int `json:"sub_tasks"`
	Pending  int `json:"pending"`
	Running  int `json:"running"`
	Success  int `json:"success"`
	Failed   int `json:"failed"`
	Unknown  int `json:"unknown"`
}

func (s *Servers) handleSystemTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	q := r.URL.Query()
	sourceFilter := strings.TrimSpace(q.Get("source"))
	typeFilter := strings.TrimSpace(q.Get("type"))
	statusFilter := strings.TrimSpace(q.Get("status"))
	relIDFilter := strings.TrimSpace(q.Get("rel_id"))
	idFilter := strings.TrimSpace(q.Get("id"))
	limit := 200
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}

	tasks := s.collectSystemTasks(r.Context(), limit)
	if sourceFilter != "" {
		tasks = filterTasks(tasks, func(t systemTask) bool { return t.Source == sourceFilter })
	}
	if typeFilter != "" {
		tasks = filterTasks(tasks, func(t systemTask) bool { return t.Type == typeFilter })
	}
	if statusFilter != "" {
		tasks = filterTasks(tasks, func(t systemTask) bool { return t.Status == statusFilter })
	}
	if relIDFilter != "" {
		tasks = filterTasks(tasks, func(t systemTask) bool { return t.RelID == relIDFilter })
	}
	if idFilter != "" {
		tasks = filterTasks(tasks, func(t systemTask) bool { return strings.Contains(t.ID, idFilter) })
	}

	summary := summarizeSystemTasks(tasks)
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": summary,
		"tasks":   tasks,
	})
}

func (s *Servers) handleSystemTaskAction(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/system/tasks/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}
	if !strings.HasSuffix(path, "/retry") {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	id := strings.TrimSuffix(path, "/retry")
	id = strings.Trim(id, "/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}

	if pt, ok := getPublishTask(id); ok && pt != nil {
		newPT := s.startPublishTask(r.Context(), pt.Trigger, pt.Subject, pt.Reason+":retry", pt.Version, pt.NodeIDs)
		if newPT == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "发布重试入队失败"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task_id": newPT.ID})
		return
	}

	task, ok := findDNSTask(id)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "仅支持DNS或发布任务重试"})
		return
	}

	var newTask *dnsTask
	switch strings.ToLower(strings.TrimSpace(task.Type)) {
	case "sync":
		newTask = s.runDNSSyncNow(task.Subject, "retry")
	case "recover":
		newTask = s.runDNSProviderTaskNow(r.Context(), "recover", task.Subject)
	case "cleanup":
		newTask = s.runDNSProviderTaskNow(r.Context(), "cleanup", task.Subject)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "该任务类型不支持重试"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task_id": newTask.ID})
}

func (s *Servers) runDNSProviderTaskNow(reqCtx context.Context, kind, subject string) *dnsTask {
	ctx, cancel := store.WithTimeout(reqCtx)
	defer cancel()

	cfg, _ := s.store.GetDNSConfig(ctx)
	if cfg == nil {
		return s.runDNSTask(kind, subject, "manual", func() (string, error) { return "dns config missing", nil })
	}
	client, err := dnsprovider.NewClient(cfg)
	if err != nil {
		return s.runDNSTask(kind, subject, cfg.Provider, func() (string, error) { return "", err })
	}

	switch kind {
	case "recover":
		return s.runDNSTask(kind, subject, cfg.Provider, func() (string, error) {
			taskCtx, taskCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer taskCancel()
			return client.Recover(taskCtx)
		})
	case "cleanup":
		return s.runDNSTask(kind, subject, cfg.Provider, func() (string, error) {
			taskCtx, taskCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer taskCancel()
			return client.Cleanup(taskCtx)
		})
	default:
		return s.runDNSTask(kind, subject, cfg.Provider, func() (string, error) { return "unsupported task", nil })
	}
}

func (s *Servers) collectSystemTasks(ctx context.Context, limit int) []systemTask {
	var out []systemTask

	dnsTaskMu.Lock()
	localDNS := make([]*dnsTask, len(dnsTaskList))
	copy(localDNS, dnsTaskList)
	dnsTaskMu.Unlock()

	for _, t := range localDNS {
		if t == nil {
			continue
		}
		out = append(out, systemTask{
			ID:        t.ID,
			RelID:     t.Provider,
			Source:    "dns",
			Type:      "dns." + strings.ToLower(strings.TrimSpace(t.Type)),
			Message:   t.Message,
			Status:    normalizeTaskStatus(t.Status),
			SubTasks:  0,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
			Retryable: normalizeTaskStatus(t.Status) == "failed",
		})
	}

	up := s.listUpgradeTasks(ctx, 200)
	for _, t := range up {
		sub := 1
		if len(t.NodeIDs) > 0 {
			sub = len(t.NodeIDs)
		}
		msg := strings.TrimSpace(t.TargetVersion)
		if msg == "" {
			msg = "-"
		}
		message := "target=" + msg + " channel=" + strings.TrimSpace(t.Channel)
		status := strings.ToLower(strings.TrimSpace(t.Status))
		if status == "completed" {
			status = "success"
		}
		if status == "" {
			status = "unknown"
		}

		out = append(out, systemTask{
			ID:        t.ID,
			RelID:     strconv.Itoa(sub),
			Source:    "upgrade",
			Type:      "upgrade." + strings.ToLower(strings.TrimSpace(t.Type)),
			Message:   message,
			Status:    normalizeTaskStatus(status),
			SubTasks:  sub,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.CreatedAt,
			Retryable: false,
			DetailURL: "/admin/dashboard/upgrade/" + t.ID,
		})
	}

	for _, t := range listBGTasks(200) {
		if t == nil {
			continue
		}
		out = append(out, systemTask{
			ID:        t.ID,
			Source:    "system",
			Type:      strings.ToLower(strings.TrimSpace(t.Type)),
			Message:   t.Message,
			Status:    normalizeTaskStatus(t.Status),
			SubTasks:  0,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
			Retryable: false,
		})
	}

	for _, t := range listPublishTasks(200) {
		if t == nil {
			continue
		}
		msg := strings.TrimSpace(t.Message)
		if msg == "" {
			msg = "-"
		}
		message := "version=" + strings.TrimSpace(t.Version) +
			" trigger=" + strings.TrimSpace(t.Trigger) +
			" success=" + strconv.Itoa(t.SuccessNodes) +
			" failed=" + strconv.Itoa(t.FailedNodes)
		if t.Reason != "" {
			message = message + " reason=" + t.Reason
		}
		out = append(out, systemTask{
			ID:        t.ID,
			Source:    "publish",
			Type:      "publish.config",
			Message:   firstNonEmpty(message, msg),
			Status:    normalizeTaskStatus(t.Status),
			SubTasks:  t.TotalNodes,
			CreatedAt: t.StartedAt,
			UpdatedAt: t.CompletedAt,
			Retryable: false,
			DetailURL: "/admin/dashboard/tasks/publish/" + t.ID,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func findDNSTask(id string) (*dnsTask, bool) {
	dnsTaskMu.Lock()
	defer dnsTaskMu.Unlock()
	for _, t := range dnsTaskList {
		if t != nil && t.ID == id {
			return t, true
		}
	}
	return nil, false
}

func normalizeTaskStatus(st string) string {
	s := strings.ToLower(strings.TrimSpace(st))
	switch s {
	case "pending":
		return "pending"
	case "running":
		return "running"
	case "success", "completed":
		return "success"
	case "failed", "error":
		return "failed"
	default:
		if s == "" {
			return "unknown"
		}
		return s
	}
}

func filterTasks(in []systemTask, fn func(systemTask) bool) []systemTask {
	out := make([]systemTask, 0, len(in))
	for _, t := range in {
		if fn(t) {
			out = append(out, t)
		}
	}
	return out
}

func summarizeSystemTasks(tasks []systemTask) systemTaskSummary {
	var s systemTaskSummary
	s.Tasks = len(tasks)
	for _, t := range tasks {
		s.SubTasks += t.SubTasks
		switch normalizeTaskStatus(t.Status) {
		case "pending":
			s.Pending++
		case "running":
			s.Running++
		case "success":
			s.Success++
		case "failed":
			s.Failed++
		default:
			s.Unknown++
		}
	}
	return s
}
