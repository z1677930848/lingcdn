package server

// L4 TCP/UDP stream forwarding rules.
//
// GET    /api/stream-forwards          -> list user's rules
// POST   /api/stream-forwards          -> create rule
// GET    /api/stream-forwards/{id}     -> get rule
// PUT    /api/stream-forwards/{id}     -> update rule
// DELETE /api/stream-forwards/{id}     -> delete rule
// GET    /api/admin/stream-forwards    -> admin list all rules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleStreamForwards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := getUserRole(ctx)
	userID, _ := ctx.Value(ctxKeyUserID).(string)

	switch r.Method {
	case http.MethodGet:
		var list []*store.StreamForward
		var err error
		if role == "admin" && r.URL.Query().Get("all") == "1" {
			list, err = s.store.ListAllStreamForwards(ctx)
		} else {
			list, err = s.store.ListStreamForwards(ctx, userID)
		}
		if err != nil {
			writeInternalError(w, "list stream forwards", err)
			return
		}
		if list == nil {
			list = []*store.StreamForward{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"stream_forwards": list})

	case http.MethodPost:
		if role != "admin" && !s.requireUserPermission(w, ctx, PermStreamForwardsWrite) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求内容格式错误"})
			return
		}
		var sf store.StreamForward
		if err := json.Unmarshal(body, &sf); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if !strings.Contains(string(body), "health_check_enabled") {
			sf.HealthCheckEnabled = true
		}
		if role != "admin" {
			sf.UserID = userID
		} else if strings.TrimSpace(sf.UserID) == "" {
			sf.UserID = userID
		}
		if err := s.normalizeStreamForward(ctx, &sf, ""); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := s.enforceStreamForwardQuota(ctx, sf.UserID, ""); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
			return
		}
		if sf.ID == "" {
			sf.ID = uuid.NewString()
		}
		if err := s.store.CreateStreamForward(ctx, &sf); err != nil {
			writeInternalError(w, "create stream forward", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "stream:"+sf.ID, "stream:create:"+sf.ID, "", nil)
		writeJSON(w, http.StatusCreated, sf)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleStreamForwardByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := getUserRole(ctx)
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	id := strings.TrimPrefix(r.URL.Path, "/api/stream-forwards/")

	existing, err := s.store.GetStreamForward(ctx, id)
	if err != nil {
		writeInternalError(w, "get stream forward", err)
		return
	}
	if existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "转发规则不存在"})
		return
	}
	if role != "admin" && existing.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权操作此规则"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, existing)

	case http.MethodPut:
		if role != "admin" && !s.requireUserPermission(w, ctx, PermStreamForwardsWrite) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求内容格式错误"})
			return
		}
		var sf store.StreamForward
		if err := json.Unmarshal(body, &sf); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		sf.ID = id
		if role != "admin" {
			sf.UserID = existing.UserID
		} else if strings.TrimSpace(sf.UserID) == "" {
			sf.UserID = existing.UserID
		}
		if !strings.Contains(string(body), "health_check_enabled") {
			sf.HealthCheckEnabled = existing.HealthCheckEnabled
		}
		if err := s.normalizeStreamForward(ctx, &sf, id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if sf.Enabled && !existing.Enabled {
			if err := s.enforceStreamForwardQuota(ctx, sf.UserID, id); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
				return
			}
		}
		if err := s.store.UpdateStreamForward(ctx, &sf); err != nil {
			writeInternalError(w, "update stream forward", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "stream:"+sf.ID, "stream:update:"+sf.ID, "", nil)
		writeJSON(w, http.StatusOK, sf)

	case http.MethodDelete:
		if role != "admin" && !s.requireUserPermission(w, ctx, PermStreamForwardsWrite) {
			return
		}
		if err := s.store.DeleteStreamForward(ctx, id); err != nil {
			writeInternalError(w, "delete stream forward", err)
			return
		}
		_ = s.startPublishTask(ctx, "auto", "stream:"+id, "stream:delete:"+id, "", nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleAdminStreamForwards(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "需要管理员权限"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	list, err := s.store.ListAllStreamForwards(r.Context())
	if err != nil {
		writeInternalError(w, "list stream forwards", err)
		return
	}
	if list == nil {
		list = []*store.StreamForward{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"stream_forwards": list})
}

func (s *Servers) normalizeStreamForward(ctx context.Context, sf *store.StreamForward, excludeID string) error {
	sf.Name = strings.TrimSpace(sf.Name)
	sf.OriginHost = strings.TrimSpace(sf.OriginHost)
	sf.Protocol = strings.ToLower(strings.TrimSpace(sf.Protocol))
	if sf.Protocol == "" {
		sf.Protocol = "tcp"
	}
	if sf.Protocol != "tcp" && sf.Protocol != "udp" {
		return fmt.Errorf("协议必须是 tcp 或 udp")
	}
	if sf.ListenPort <= 0 || sf.ListenPort > 65535 {
		return fmt.Errorf("监听端口无效")
	}
	if sf.OriginHost == "" {
		return fmt.Errorf("回源地址不能为空")
	}
	if sf.OriginPort <= 0 || sf.OriginPort > 65535 {
		return fmt.Errorf("回源端口无效")
	}
	if strings.TrimSpace(sf.DomainID) != "" {
		d, err := s.store.GetDomain(ctx, sf.DomainID)
		if err != nil {
			return fmt.Errorf("查询域名失败")
		}
		if d == nil {
			return fmt.Errorf("关联域名不存在")
		}
		if d.UserID != sf.UserID {
			return fmt.Errorf("域名不属于当前用户")
		}
	}
	// Port conflict: same listen port may not be reused within the same line group
	// (or globally when no domain/cluster binding exists).
	resolveScope := func(domainID string) string {
		if strings.TrimSpace(domainID) == "" {
			return ""
		}
		if d, _ := s.store.GetDomain(ctx, domainID); d != nil {
			return strings.TrimSpace(d.LineGroupID)
		}
		return ""
	}
	sfScope := resolveScope(sf.DomainID)
	all, err := s.store.ListAllStreamForwards(ctx)
	if err != nil {
		return err
	}
	for _, other := range all {
		if other == nil || !other.Enabled || other.ID == excludeID || !sf.Enabled {
			continue
		}
		if other.ListenPort != sf.ListenPort {
			continue
		}
		otherScope := resolveScope(other.DomainID)
		if sfScope != "" && otherScope != "" && sfScope != otherScope {
			continue
		}
		if sfScope == "" && otherScope != "" {
			continue
		}
		if sfScope != "" && otherScope == "" {
			continue
		}
		return fmt.Errorf("监听端口 %d 已被占用", sf.ListenPort)
	}
	return nil
}

func (s *Servers) enforceStreamForwardQuota(ctx context.Context, userID, excludeID string) error {
	product, err := s.getUserActiveProduct(ctx, userID, "")
	if err != nil || product == nil || product.StreamPortLimit == nil || *product.StreamPortLimit <= 0 {
		return fmt.Errorf("当前套餐不支持四层转发或未配置端口配额")
	}
	count, err := s.store.CountEnabledStreamForwardsByUser(ctx, userID)
	if err != nil {
		return err
	}
	if excludeID != "" {
		if existing, _ := s.store.GetStreamForward(ctx, excludeID); existing != nil && existing.Enabled {
			count--
		}
	}
	if int32(count) >= *product.StreamPortLimit {
		return fmt.Errorf("已达到套餐四层转发端口上限 (%d)", *product.StreamPortLimit)
	}
	return nil
}
