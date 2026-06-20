package server

// Admin-only user management: list/create users, bulk operations
// (disable/enable/delete/reset_password), per-user PATCH/DELETE, and an
// adjacent /api/email/test endpoint that uses the same admin surface to
// smoke-test the configured SMTP server.
//
// Self-protection is consistent across bulk and item endpoints: the currently
// authenticated admin cannot delete themselves, demote their own role, or
// disable their own account. This prevents accidental lockouts of a
// single-admin control plane.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/lingcdn/control/internal/store"
)

// emailRequest is the input payload for the SMTP test endpoint.
type emailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *Servers) handleUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		users, err := s.store.ListUsers(ctx, 0)
		if err != nil {
			writeInternalError(w, "list users", err)
			return
		}
		groups, _ := s.store.ListUserGroups(ctx)
		groupNames := make(map[string]string, len(groups))
		for _, g := range groups {
			if g != nil {
				groupNames[g.ID] = g.Name
			}
		}
		// Strip password hashes and other sensitive fields before returning.
		safe := make([]map[string]any, 0, len(users))
		for _, u := range users {
			item := map[string]any{
				"id":                  u.ID,
				"numeric_id":          u.NumericID,
				"username":            u.Username,
				"email":               u.Email,
				"role":                u.Role,
				"status":              u.Status,
				"group_id":            u.GroupID,
				"created_at":          u.CreatedAt,
				"last_login_at":       u.LastLoginAt,
				"last_login_ip":       u.LastLoginIP,
				"last_login_location": u.LastLoginLocation,
			}
			if u.GroupID != "" {
				item["group_name"] = groupNames[u.GroupID]
			}
			safe = append(safe, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": safe})

	case http.MethodPost:
		if role := getUserRole(ctx); role != "admin" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
			return
		}
		var req struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
			Status   string `json:"status"`
			GroupID  string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		req.Email = strings.TrimSpace(req.Email)
		if req.Username == "" || req.Email == "" || req.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "用户名、邮箱和密码不能为空"})
			return
		}
		if msg := passwordPolicyError(req.Password); msg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
			return
		}
		role := req.Role
		if role == "" {
			role = "user"
		}
		if role != "admin" && role != "user" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的角色"})
			return
		}
		status := strings.TrimSpace(req.Status)
		if status == "" {
			status = "active"
		}
		if status != "active" && status != "disabled" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
			return
		}
		if existing, _ := s.store.GetUserByUsername(ctx, req.Username); existing != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "用户名已存在"})
			return
		}
		if existing, _ := s.store.GetUserByEmail(ctx, req.Email); existing != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "邮箱已被注册"})
			return
		}
		req.GroupID = strings.TrimSpace(req.GroupID)
		if err := s.validateUserGroupRef(ctx, req.GroupID); err != nil {
			if err == errUserGroupNotFound {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			writeInternalError(w, "validate user group", err)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeInternalError(w, "hash password", err)
			return
		}
		u := &store.User{
			ID:           uuid.NewString(),
			Username:     strings.ToLower(req.Username),
			Email:        strings.ToLower(req.Email),
			PasswordHash: string(hash),
			Role:         role,
			Status:       status,
			GroupID:      req.GroupID,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := s.store.CreateUser(ctx, u); err != nil {
			writeInternalError(w, "create user", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         u.ID,
			"numeric_id": u.NumericID,
			"username":   u.Username,
			"email":      u.Email,
			"role":       u.Role,
			"status":     u.Status,
			"group_id":   u.GroupID,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// handleUsersBulk applies a single admin action (disable/enable/delete/
// reset_password) to a list of user IDs. Partial failures are reported per-ID
// in the `failed` list; the response is always 200 OK.
func (s *Servers) handleUsersBulk(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r.Context()) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	var req struct {
		IDs      []string `json:"ids"`
		Action   string   `json:"action"`
		Password string   `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}
	if len(req.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID列表不能为空"})
		return
	}
	action := strings.TrimSpace(req.Action)
	if action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "操作类型不能为空"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	var hash string
	if action == "reset_password" {
		password := strings.TrimSpace(req.Password)
		if msg := passwordPolicyError(password); msg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
			return
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			writeInternalError(w, "hash password", err)
			return
		}
		hash = string(hashed)
	}

	// Self-protection: never allow destructive bulk actions against the current admin.
	// Otherwise, a lone administrator could accidentally delete/disable themselves and
	// lock the whole control plane out.
	callerID := getUserID(r.Context())
	destructive := action == "disable" || action == "delete"

	failed := make([]map[string]any, 0)
	success := 0
	total := 0
	for _, rawID := range req.IDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		total++
		if destructive && id == callerID {
			failed = append(failed, map[string]any{
				"id":    id,
				"error": "不能对自己执行 " + action + " 操作",
			})
			continue
		}
		var err error
		switch action {
		case "disable":
			err = s.store.UpdateUserStatus(ctx, id, "disabled")
		case "enable":
			err = s.store.UpdateUserStatus(ctx, id, "active")
		case "delete":
			err = s.store.DeleteUser(ctx, id)
		case "reset_password":
			err = s.store.UpdateUserPasswordHash(ctx, id, hash)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的操作"})
			return
		}
		if err != nil {
			failed = append(failed, map[string]any{
				"id":    id,
				"error": err.Error(),
			})
			continue
		}
		success++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"total":   total,
		"success": success,
		"failed":  failed,
	})
}

// handleEmailTest sends a probe email using the currently-configured SMTP
// settings. Used by the Settings UI to verify SMTP config after editing.
func (s *Servers) handleEmailTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	var req emailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}
	req.To = strings.TrimSpace(req.To)
	if req.To == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目标不能为空"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		writeInternalError(w, "get settings", err)
		return
	}
	smtpCfg := s.smtpConfigFromSettings(s.applySettingsDefaults(settings))
	if smtpCfg.SMTPHost == "" || smtpCfg.SMTPFrom == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SMTP主机和发件地址不能为空"})
		return
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		subject = "LingCDN test email"
	}
	body := req.Body
	if body == "" {
		body = "This is a test email from LingCDN."
	}
	if err := sendEmail(smtpCfg, req.To, subject, body); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleUserByID handles PATCH (status/role/password) and DELETE for an
// individual user. Same self-protection rules as handleUsersBulk.
func (s *Servers) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r.Context()) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID不能为空"})
		return
	}
	// Self-protection: forbid irreversible operations against the current admin.
	callerID := getUserID(r.Context())
	isSelf := callerID != "" && callerID == id
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	switch r.Method {
	case http.MethodDelete:
		if isSelf {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "不能删除自己"})
			return
		}
		if err := s.store.DeleteUser(ctx, id); err != nil {
			writeInternalError(w, "delete user", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case http.MethodPatch:
		var req struct {
			Status   *string `json:"status"`
			Role     *string `json:"role"`
			Password *string `json:"password"`
			GroupID  *string `json:"group_id"`
			Email    *string `json:"email"`
			Username *string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if req.Status == nil && req.Role == nil && req.Password == nil && req.GroupID == nil && req.Email == nil && req.Username == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "没有需要更新的字段"})
			return
		}
		if req.Status != nil {
			status := strings.TrimSpace(*req.Status)
			if status != "active" && status != "disabled" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态值"})
				return
			}
			if isSelf && status == "disabled" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "不能禁用自己"})
				return
			}
			if err := s.store.UpdateUserStatus(ctx, id, status); err != nil {
				writeInternalError(w, "update user status", err)
				return
			}
		}
		if req.Role != nil {
			role := strings.TrimSpace(*req.Role)
			if role != "admin" && role != "user" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的角色"})
				return
			}
			if isSelf && role != "admin" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "不能降级自己"})
				return
			}
			if err := s.store.UpdateUserRole(ctx, id, role); err != nil {
				writeInternalError(w, "update user role", err)
				return
			}
		}
		if req.Password != nil {
			password := strings.TrimSpace(*req.Password)
			if msg := passwordPolicyError(password); msg != "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				writeInternalError(w, "hash password", err)
				return
			}
			if err := s.store.UpdateUserPasswordHash(ctx, id, string(hash)); err != nil {
				writeInternalError(w, "update user password", err)
				return
			}
		}
		if req.GroupID != nil {
			groupID := strings.TrimSpace(*req.GroupID)
			if err := s.validateUserGroupRef(ctx, groupID); err != nil {
				if err == errUserGroupNotFound {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
					return
				}
				writeInternalError(w, "validate user group", err)
				return
			}
			if err := s.store.UpdateUserGroupID(ctx, id, groupID); err != nil {
				writeInternalError(w, "update user group", err)
				return
			}
		}
		if req.Email != nil {
			email := strings.ToLower(strings.TrimSpace(*req.Email))
			if email == "" || !strings.Contains(email, "@") {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱格式无效"})
				return
			}
			if existing, _ := s.store.GetUserByEmail(ctx, email); existing != nil && existing.ID != id {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "邮箱已被注册"})
				return
			}
			if err := s.store.UpdateUserEmail(ctx, id, email); err != nil {
				writeInternalError(w, "update user email", err)
				return
			}
		}
		if req.Username != nil {
			username := strings.ToLower(strings.TrimSpace(*req.Username))
			if username == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "用户名不能为空"})
				return
			}
			if existing, _ := s.store.GetUserByUsername(ctx, username); existing != nil && existing.ID != id {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "用户名已存在"})
				return
			}
			if err := s.store.UpdateUserUsername(ctx, id, username); err != nil {
				writeInternalError(w, "update user username", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
}
