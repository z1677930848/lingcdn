package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleTickets(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	userID := strings.TrimSpace(getUserID(ctx))
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		page := parseIntQuery(r, "page", 1)
		pageSize := parseIntQuery(r, "page_size", parseIntQuery(r, "pageSize", 20))
		tickets, total, err := s.store.ListTicketsByUser(ctx, userID, status, page, pageSize)
		if err != nil {
			writeInternalError(w, "list tickets", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets, "total": total, "page": page, "page_size": pageSize})
	case http.MethodPost:
		var req struct {
			Subject  string `json:"subject"`
			Category string `json:"category"`
			Body     string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		user, _ := s.store.GetUserByID(ctx, userID)
		username := ""
		if user != nil {
			username = user.Username
		}
		t := &store.Ticket{
			ID:       uuid.NewString(),
			UserID:   userID,
			Subject:  strings.TrimSpace(req.Subject),
			Category: req.Category,
			Username: username,
		}
		if err := s.store.CreateTicket(ctx, t, req.Body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.notifyTicketUserActivity(ctx, t, "提交了工单")
		writeJSON(w, http.StatusCreated, map[string]any{"ticket": t})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleTicketByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	userID := strings.TrimSpace(getUserID(ctx))
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/tickets/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "工单ID不能为空"})
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	ticket, err := s.store.GetTicket(ctx, id)
	if err != nil {
		writeInternalError(w, "get ticket", err)
		return
	}
	if ticket == nil || ticket.UserID != userID {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "工单不存在"})
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		replies, err := s.store.ListTicketReplies(ctx, id)
		if err != nil {
			writeInternalError(w, "list ticket replies", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ticket": ticket, "replies": replies})

	case action == "replies" && r.Method == http.MethodPost:
		if ticket.Status == "closed" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "工单已关闭，无法回复"})
			return
		}
		var req struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		user, _ := s.store.GetUserByID(ctx, userID)
		name := userID
		if user != nil {
			name = user.Username
		}
		reply := &store.TicketReply{
			TicketID:   id,
			AuthorID:   userID,
			AuthorRole: "user",
			AuthorName: name,
			Body:       req.Body,
		}
		if err := s.store.AddTicketReply(ctx, reply); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		ticket, _ = s.store.GetTicket(ctx, id)
		s.notifyTicketUserActivity(ctx, ticket, "回复了工单")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "reply": reply, "ticket": ticket})

	case action == "close" && r.Method == http.MethodPost:
		if ticket.Status == "closed" {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ticket": ticket})
			return
		}
		ticket.Status = "closed"
		if err := s.store.UpdateTicket(ctx, ticket); err != nil {
			writeInternalError(w, "close ticket", err)
			return
		}
		ticket, _ = s.store.GetTicket(ctx, id)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ticket": ticket})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleAdminTickets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "page_size", parseIntQuery(r, "pageSize", 20))
	tickets, total, err := s.store.ListTicketsAdmin(ctx, userID, status, page, pageSize)
	if err != nil {
		writeInternalError(w, "list tickets", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets, "total": total, "page": page, "page_size": pageSize})
}

func (s *Servers) handleAdminTicketByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/tickets/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "工单ID不能为空"})
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	ticket, err := s.store.GetTicket(ctx, id)
	if err != nil {
		writeInternalError(w, "get ticket", err)
		return
	}
	if ticket == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "工单不存在"})
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		replies, err := s.store.ListTicketReplies(ctx, id)
		if err != nil {
			writeInternalError(w, "list ticket replies", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ticket": ticket, "replies": replies})

	case len(parts) == 1 && r.Method == http.MethodPatch:
		var req struct {
			Status   *string `json:"status"`
			Priority *string `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		if req.Status != nil {
			st := strings.ToLower(strings.TrimSpace(*req.Status))
			if st != "open" && st != "replied" && st != "closed" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的状态"})
				return
			}
			ticket.Status = st
		}
		if req.Priority != nil {
			ticket.Priority = *req.Priority
		}
		if err := s.store.UpdateTicket(ctx, ticket); err != nil {
			writeInternalError(w, "update ticket", err)
			return
		}
		ticket, _ = s.store.GetTicket(ctx, id)
		writeJSON(w, http.StatusOK, map[string]any{"ticket": ticket})

	case action == "replies" && r.Method == http.MethodPost:
		var req struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		adminID := getUserID(ctx)
		adminName := "管理员"
		if u, _ := s.store.GetUserByID(ctx, adminID); u != nil && u.Username != "" {
			adminName = u.Username
		}
		reply := &store.TicketReply{
			TicketID:   id,
			AuthorID:   adminID,
			AuthorRole: "admin",
			AuthorName: adminName,
			Body:       req.Body,
		}
		if ticket.Status == "closed" {
			ticket.Status = "replied"
			ticket.ClosedAt = nil
			_ = s.store.UpdateTicket(ctx, ticket)
		}
		if err := s.store.AddTicketReply(ctx, reply); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		ticket, _ = s.store.GetTicket(ctx, id)
		s.notifyTicketAdminReply(ctx, ticket, reply.Body)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "reply": reply, "ticket": ticket})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) notifyTicketUserActivity(ctx context.Context, ticket *store.Ticket, action string) {
	if s == nil || ticket == nil {
		return
	}
	settings := s.resolveSettings(ctx)
	if !settings.NotifyTicketReply {
		return
	}
	name := ticket.Username
	if name == "" {
		name = ticket.UserID
	}
	title := "工单用户动态"
	content := fmt.Sprintf("用户 **%s** %s\n\n- 主题：%s\n- 分类：%s\n- 状态：%s", name, action, ticket.Subject, ticket.Category, ticket.Status)
	s.sendWebhookNotification(ctx, title, content)
}

func (s *Servers) notifyTicketAdminReply(ctx context.Context, ticket *store.Ticket, body string) {
	if s == nil || ticket == nil {
		return
	}
	user, err := s.store.GetUserByID(ctx, ticket.UserID)
	if err != nil || user == nil || strings.TrimSpace(user.Email) == "" {
		return
	}
	smtpCfg := s.smtpConfigFromSettings(s.applySettingsDefaults(s.resolveSettings(ctx)))
	if strings.TrimSpace(smtpCfg.SMTPHost) == "" || strings.TrimSpace(smtpCfg.SMTPFrom) == "" {
		return
	}
	subject := fmt.Sprintf("[LingCDN] 工单回复：%s", ticket.Subject)
	preview := body
	if len(preview) > 200 {
		preview = preview[:200] + "…"
	}
	msg := fmt.Sprintf("您好 %s，\n\n您的工单「%s」已有管理员回复：\n\n%s\n\n请登录控制台查看详情。", user.Username, ticket.Subject, preview)
	go func() {
		if err := sendEmail(smtpCfg, user.Email, subject, msg); err != nil {
			// best-effort
		}
	}()
}
