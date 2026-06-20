package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleAdminUserGroups(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		groups, err := s.store.ListUserGroups(ctx)
		if err != nil {
			writeInternalError(w, "list user groups", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"groups":               groups,
			"available_permissions": userGroupPermissionCatalog,
		})
	case http.MethodPost:
		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Permissions []string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Description = strings.TrimSpace(req.Description)
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
			return
		}
		now := time.Now()
		g := &store.UserGroup{
			ID:          uuid.NewString(),
			Name:        req.Name,
			Description: req.Description,
			Permissions: normalizeUserGroupPermissions(req.Permissions),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.store.CreateUserGroup(ctx, g); err != nil {
			writeInternalError(w, "create user group", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"group": g})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleAdminUserGroupByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	if role := getUserRole(ctx); role != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/user-groups/")
	id = strings.TrimSpace(strings.Trim(id, "/"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		existing, err := s.store.GetUserGroup(ctx, id)
		if err != nil {
			writeInternalError(w, "get user group", err)
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "分组不存在"})
			return
		}
		var req struct {
			Name        *string   `json:"name"`
			Description *string   `json:"description"`
			Permissions *[]string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "名称不能为空"})
				return
			}
			existing.Name = name
		}
		if req.Description != nil {
			existing.Description = strings.TrimSpace(*req.Description)
		}
		if req.Permissions != nil {
			existing.Permissions = normalizeUserGroupPermissions(*req.Permissions)
		}
		if err := s.store.UpdateUserGroup(ctx, existing); err != nil {
			writeInternalError(w, "update user group", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"group": existing})
	case http.MethodDelete:
		if err := s.store.DeleteUserGroup(ctx, id); err != nil {
			writeInternalError(w, "delete user group", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) validateUserGroupRef(ctx context.Context, groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil
	}
	g, err := s.store.GetUserGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if g == nil {
		return errUserGroupNotFound
	}
	return nil
}

var errUserGroupNotFound = storeNotFound("用户分组不存在")

type storeNotFound string

func (e storeNotFound) Error() string { return string(e) }
