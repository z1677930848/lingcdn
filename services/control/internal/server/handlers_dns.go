package server

// DNS admin handlers: provider config, one-shot recover/cleanup actions,
// provider metadata (domains, routing lines), and the in-memory task log
// that backs the "DNS Tasks" panel in the UI. The sync task plumbing
// ('runDNSSyncNow', 'runDNSSyncTask') lives in dns_sync.go but shares the
// same dnsTask type and global task list defined here.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/dnsprovider"
	"github.com/lingcdn/control/internal/store"
)

// dnsTask is the in-memory record for a single DNS operation (recover,
// cleanup, sync). The list is bounded by runDNSTask to the last 200 entries.
type dnsTask struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // recover | cleanup
	Subject   string    `json:"subject,omitempty"`
	Provider  string    `json:"provider"`
	Status    string    `json:"status"` // pending | running | success | failed
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Package-level task log. Shared with dns_sync.go and task_center_api.go,
// which is why the state lives here rather than on *Servers.
var (
	dnsTaskMu   sync.Mutex
	dnsTaskList []*dnsTask
)

// handleDNSConfig handles GET/PUT for the provider credentials used by DNS
// sync. Credentials are stored as-is in the DB; transport security is the
// caller's responsibility.
func (s *Servers) handleDNSConfig(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		cfg, err := s.store.GetDNSConfig(ctx)
		if err != nil {
			writeInternalError(w, "get DNS config", err)
			return
		}
		if cfg == nil {
			cfg = &store.DNSConfig{
				TTL: 600,
			}
		}
		if cfg.TTL == 0 {
			cfg.TTL = 600
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"config":    cfg,
			"providers": dnsProviders(),
		})

	case http.MethodPut:
		var req struct {
			Provider       string `json:"provider"`
			AccountID      string `json:"account_id"`
			Token          string `json:"token"`
			Secret         string `json:"secret"`
			TTL            int64  `json:"ttl"`
			EnableIPWeight bool   `json:"enable_ip_weight"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if strings.TrimSpace(req.Provider) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "提供商不能为空"})
			return
		}
		if req.TTL <= 0 {
			req.TTL = 600
		}

		cfg := &store.DNSConfig{
			Provider:       strings.TrimSpace(req.Provider),
			AccountID:      strings.TrimSpace(req.AccountID),
			Token:          strings.TrimSpace(req.Token),
			Secret:         strings.TrimSpace(req.Secret),
			TTL:            req.TTL,
			EnableIPWeight: req.EnableIPWeight,
			LastError:      "",
			UpdatedAt:      time.Now(),
		}
		if err := s.store.SaveDNSConfig(ctx, cfg); err != nil {
			writeInternalError(w, "save DNS config", err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"config":    cfg,
			"providers": dnsProviders(),
			"message":   "saved",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleDNSProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": dnsProviders(),
	})
}

func (s *Servers) handleDNSRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	task := s.runDNSSyncNow("recover", "manual recover")
	if task == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "DNS 恢复任务启动失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": task.Status != "failed", "message": task.Message, "task_id": task.ID})
}

func (s *Servers) handleDNSCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	task := s.runDNSCleanupNow("cleanup", "manual cleanup")
	if task == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "DNS 清理任务启动失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": task.Status != "failed", "message": task.Message, "task_id": task.ID})
}

func (s *Servers) handleDNSProviderOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": dnsProviders()})
}

func (s *Servers) handleDNSDomainOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	domains, err := s.store.ListDomains(ctx)
	if err != nil {
		writeInternalError(w, "list domains", err)
		return
	}
	opts := make([]map[string]any, 0, len(domains))
	for _, d := range domains {
		opts = append(opts, map[string]any{
			"id":   d.ID,
			"name": d.Name,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": opts})
}

// handleDNSProviderDomains returns domains directly from the DNS provider account.
func (s *Servers) handleDNSProviderDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	cfg, err := s.store.GetDNSConfig(ctx)
	if err != nil {
		writeInternalError(w, "get DNS config", err)
		return
	}
	if cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"domains": []any{}, "message": "DNS 未配置"})
		return
	}

	client, err := dnsprovider.NewClient(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	domains, err := client.ListProviderDomains(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": domains})
}

// handleDNSLines returns available DNS routing lines based on the current DNS provider.
func (s *Servers) handleDNSLines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	cfg, err := s.store.GetDNSConfig(ctx)
	if err != nil {
		writeInternalError(w, "get DNS config", err)
		return
	}

	type lineOption struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}

	provider := ""
	if cfg != nil {
		provider = strings.TrimSpace(cfg.Provider)
	}

	var lines []lineOption
	switch provider {
	case "dnspod", "dnspod-global":
		lines = []lineOption{
			{"默认", "默认"},
			{"境内", "境内"},
			{"境外", "境外"},
			{"电信", "电信"},
			{"联通", "联通"},
			{"移动", "移动"},
			{"教育网", "教育网"},
			{"搜索引擎", "搜索引擎"},
		}
	case "alidns":
		lines = []lineOption{
			{"默认", "默认"},
			{"电信", "电信"},
			{"联通", "联通"},
			{"移动", "移动"},
			{"教育网", "教育网"},
			{"境外", "境外"},
			{"搜索引擎", "搜索引擎"},
		}
	default:
		lines = []lineOption{
			{"默认", "默认"},
		}
	}

	supportsLine := provider == "dnspod" || provider == "dnspod-global" || provider == "alidns"
	writeJSON(w, http.StatusOK, map[string]any{
		"lines":         lines,
		"provider":      provider,
		"supports_line": supportsLine,
	})
}

// handleDNSTasks serves the in-memory DNS task log (latest 50).
func (s *Servers) handleDNSTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	dnsTaskMu.Lock()
	defer dnsTaskMu.Unlock()

	// return latest 50 tasks
	n := len(dnsTaskList)
	start := 0
	if n > 50 {
		start = n - 50
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": dnsTaskList[start:]})
}

// runDNSTask launches fn in a goroutine, records its progress on a new
// dnsTask, and appends it to the bounded in-memory task list. The list is
// trimmed to the last 200 entries so long-running control planes do not
// accumulate unbounded memory.
func (s *Servers) runDNSTask(kind, subject, provider string, fn func() (string, error)) *dnsTask {
	t := &dnsTask{
		ID:        uuid.NewString(),
		Type:      kind,
		Subject:   strings.TrimSpace(subject),
		Provider:  provider,
		Status:    "running",
		Message:   "running",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	go func(task *dnsTask) {
		msg, err := fn()
		dnsTaskMu.Lock()
		if err != nil {
			task.Status = "failed"
			task.Message = err.Error()
		} else {
			task.Status = "success"
			task.Message = msg
		}
		task.UpdatedAt = time.Now()
		dnsTaskMu.Unlock()
		s.emitDNSTaskEvent(task)
	}(t)

	dnsTaskMu.Lock()
	dnsTaskList = append(dnsTaskList, t)
	// Trim list to keep only the last 200 entries
	if len(dnsTaskList) > 200 {
		dnsTaskList = dnsTaskList[len(dnsTaskList)-200:]
	}
	dnsTaskMu.Unlock()
	s.emitDNSTaskEvent(t)
	return t
}

// emitDNSTaskEvent pushes a DNS task-state snapshot through the SSE broker.
func (s *Servers) emitDNSTaskEvent(t *dnsTask) {
	if s == nil || s.sseBroker == nil || t == nil {
		return
	}
	completed := time.Time{}
	if t.Status == "success" || t.Status == "failed" {
		completed = t.UpdatedAt
	}
	s.sseBroker.notifyTask(syncTaskEvent{
		Kind:        "dns",
		ID:          t.ID,
		Subject:     t.Subject,
		Status:      t.Status,
		Message:     t.Message,
		StartedAt:   t.CreatedAt,
		CompletedAt: completed,
	})
}

func dnsSyncNotReadyMessage(provider string) string {
	return fmt.Sprintf("DNS 提供商 %q 尚未支持自动解析同步", strings.TrimSpace(provider))
}

func ensureDNSSyncReady(cfg *store.DNSConfig) error {
	if cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
		return fmt.Errorf("DNS 提供商未配置")
	}
	if !dnsprovider.SyncReady(cfg.Provider) {
		return fmt.Errorf("%s", dnsSyncNotReadyMessage(cfg.Provider))
	}
	return nil
}

func dnsProviders() []map[string]any {
	type entry struct {
		value, label string
		syncReady    bool
	}
	list := []entry{
		{"dnspod", "DNSPod (dnspod.cn)", true},
		{"dnspod-global", "DNSPod Global (dnspod.com)", true},
		{"alidns", "AliDNS (aliyun.com)", true},
		{"cloudflare", "Cloudflare", true},
		{"route53", "Amazon Route53", true},
		{"huawei", "华为云 DNS", true},
		{"google", "Google Cloud DNS", true},
		{"51dns", "51DNS", true},
		{"dnsla", "DNS.LA", true},
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, map[string]any{
			"value":      p.value,
			"label":      p.label,
			"sync_ready": p.syncReady,
		})
	}
	return out
}
