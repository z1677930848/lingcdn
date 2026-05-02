package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/store"
	"github.com/lingcdn/control/internal/templates"
)

type globalTemplateDTO struct {
	Key            string     `json:"key"`
	Name           string     `json:"name"`
	Group          string     `json:"group"`
	Mode           string     `json:"mode"`
	DefaultContent string     `json:"default_content"`
	Content        string     `json:"content"`
	Customized     bool       `json:"customized"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	Placeholders   []string   `json:"placeholders,omitempty"`
}

func (s *Servers) handleGlobalTemplates(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	overrides, err := s.store.ListGlobalTemplateOverrides(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "模板查找失败"})
		return
	}
	overrideMap := make(map[string]*store.GlobalTemplateOverride, len(overrides))
	for _, t := range overrides {
		if t == nil || strings.TrimSpace(t.Key) == "" {
			continue
		}
		overrideMap[t.Key] = t
	}
	defs := defaultGlobalTemplateDefs()
	out := make([]globalTemplateDTO, 0, len(defs))
	for _, d := range defs {
		dto := globalTemplateDTO{
			Key:            d.Key,
			Name:           d.Name,
			Group:          d.Group,
			Mode:           d.Mode,
			DefaultContent: d.DefaultContent,
			Content:        d.DefaultContent,
			Customized:     false,
			Placeholders:   d.Placeholders,
		}
		if ov := overrideMap[d.Key]; ov != nil {
			dto.Content = ov.Content
			dto.Customized = true
			tm := ov.UpdatedAt
			dto.UpdatedAt = &tm
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

func (s *Servers) handleGlobalTemplateByKey(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/system/templates/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "密钥不能为空"})
		return
	}
	if strings.HasSuffix(path, "/reset") {
		key := strings.TrimSuffix(path, "/reset")
		key = strings.Trim(key, "/")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "密钥不能为空"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
			return
		}
		if !hasGlobalTemplateDef(key) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "模板不存在"})
			return
		}
		ctx, cancel := store.WithTimeout(r.Context())
		defer cancel()
		if err := s.store.DeleteGlobalTemplateOverride(ctx, key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "重置失败"})
			return
		}
		invalidateTemplateOverrideCache(key)
		task := s.startPublishTask(r.Context(), "auto", "", "template.reset:"+key, "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "publish_task_id": task.ID})
		return
	}

	key := path
	def, ok := getGlobalTemplateDef(key)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "模板不存在"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := store.WithTimeout(r.Context())
		defer cancel()
		ov, err := s.store.GetGlobalTemplateOverride(ctx, key)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "模板查找失败"})
			return
		}
		dto := globalTemplateDTO{
			Key:            def.Key,
			Name:           def.Name,
			Group:          def.Group,
			Mode:           def.Mode,
			DefaultContent: def.DefaultContent,
			Content:        def.DefaultContent,
			Customized:     false,
			Placeholders:   def.Placeholders,
		}
		if ov != nil {
			dto.Content = ov.Content
			dto.Customized = true
			tm := ov.UpdatedAt
			dto.UpdatedAt = &tm
		}
		writeJSON(w, http.StatusOK, map[string]any{"template": dto})
	case http.MethodPut:
		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		ctx, cancel := store.WithTimeout(r.Context())
		defer cancel()
		if err := s.store.UpsertGlobalTemplateOverride(ctx, &store.GlobalTemplateOverride{
			Key:     key,
			Content: req.Content,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "保存失败"})
			return
		}
		invalidateTemplateOverrideCache(key)
		task := s.startPublishTask(r.Context(), "auto", "", "template.save:"+key, "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "publish_task_id": task.ID})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func hasGlobalTemplateDef(key string) bool {
	_, ok := getGlobalTemplateDef(key)
	return ok
}

func getGlobalTemplateDef(key string) (globalTemplateDef, bool) {
	for _, d := range templates.DefaultGlobalTemplateDefs() {
		if d.Key == key {
			return globalTemplateDef(d), true
		}
	}
	return globalTemplateDef{}, false
}

type globalTemplateDef = templates.GlobalTemplateDef

func defaultGlobalTemplateDefs() []globalTemplateDef {
	return templates.DefaultGlobalTemplateDefs()
}
