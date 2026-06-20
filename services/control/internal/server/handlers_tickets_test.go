package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lingcdn/control/internal/store"
)

func seedTicketUser(t *testing.T, mem *store.Memory, id, username string) *store.User {
	t.Helper()
	ctx := context.Background()
	user := &store.User{ID: id, Username: username, Email: username + "@example.com", Role: "user"}
	if err := mem.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func TestHandleTicketsCreateAndList(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	user := seedTicketUser(t, mem, "user-1", "alice")
	other := seedTicketUser(t, mem, "user-2", "bob")
	s := newTestServers(t, mem)

	createBody, _ := json.Marshal(map[string]any{
		"subject":  "无法访问域名",
		"category": "technical",
		"body":     "example.com 返回 502",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tickets", bytes.NewReader(createBody))
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyUserID, user.ID))
	w := httptest.NewRecorder()
	s.handleTickets(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d body=%s", w.Code, w.Body.String())
	}

	var created struct {
		Ticket store.Ticket `json:"ticket"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Ticket.Subject != "无法访问域名" || created.Ticket.Status != "open" {
		t.Fatalf("unexpected ticket: %+v", created.Ticket)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/tickets", nil)
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), ctxKeyUserID, user.ID))
	listW := httptest.NewRecorder()
	s.handleTickets(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d body=%s", listW.Code, listW.Body.String())
	}
	var listResp struct {
		Tickets []store.Ticket `json:"tickets"`
		Total   int64          `json:"total"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listResp.Total != 1 || len(listResp.Tickets) != 1 {
		t.Fatalf("expected 1 ticket, got total=%d len=%d", listResp.Total, len(listResp.Tickets))
	}

	otherList := httptest.NewRequest(http.MethodGet, "/api/tickets", nil)
	otherList = otherList.WithContext(context.WithValue(otherList.Context(), ctxKeyUserID, other.ID))
	otherW := httptest.NewRecorder()
	s.handleTickets(otherW, otherList)
	var otherResp struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(otherW.Body.Bytes(), &otherResp)
	if otherResp.Total != 0 {
		t.Fatalf("other user should see 0 tickets, got %d", otherResp.Total)
	}
}

func TestHandleTicketReplyAndClose(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	user := seedTicketUser(t, mem, "user-3", "carol")
	s := newTestServers(t, mem)

	createBody, _ := json.Marshal(map[string]any{
		"subject":  "账单疑问",
		"category": "billing",
		"body":     "充值未到账",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/tickets", bytes.NewReader(createBody))
	createReq = createReq.WithContext(context.WithValue(createReq.Context(), ctxKeyUserID, user.ID))
	createW := httptest.NewRecorder()
	s.handleTickets(createW, createReq)
	var created struct {
		Ticket store.Ticket `json:"ticket"`
	}
	_ = json.Unmarshal(createW.Body.Bytes(), &created)
	id := created.Ticket.ID

	replyBody, _ := json.Marshal(map[string]any{"body": "补充订单号 12345"})
	replyReq := httptest.NewRequest(http.MethodPost, "/api/tickets/"+id+"/replies", bytes.NewReader(replyBody))
	replyReq = replyReq.WithContext(context.WithValue(replyReq.Context(), ctxKeyUserID, user.ID))
	replyW := httptest.NewRecorder()
	s.handleTicketByID(replyW, replyReq)
	if replyW.Code != http.StatusOK {
		t.Fatalf("reply: expected 200, got %d body=%s", replyW.Code, replyW.Body.String())
	}

	closeReq := httptest.NewRequest(http.MethodPost, "/api/tickets/"+id+"/close", nil)
	closeReq = closeReq.WithContext(context.WithValue(closeReq.Context(), ctxKeyUserID, user.ID))
	closeW := httptest.NewRecorder()
	s.handleTicketByID(closeW, closeReq)
	if closeW.Code != http.StatusOK {
		t.Fatalf("close: expected 200, got %d body=%s", closeW.Code, closeW.Body.String())
	}

	blockedBody, _ := json.Marshal(map[string]any{"body": "还能回复吗"})
	blockedReq := httptest.NewRequest(http.MethodPost, "/api/tickets/"+id+"/replies", bytes.NewReader(blockedBody))
	blockedReq = blockedReq.WithContext(context.WithValue(blockedReq.Context(), ctxKeyUserID, user.ID))
	blockedW := httptest.NewRecorder()
	s.handleTicketByID(blockedW, blockedReq)
	if blockedW.Code != http.StatusBadRequest {
		t.Fatalf("reply on closed: expected 400, got %d body=%s", blockedW.Code, blockedW.Body.String())
	}
}

func TestHandleAdminTicketReplyReopensClosed(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	user := seedTicketUser(t, mem, "user-4", "dave")
	admin := seedTicketUser(t, mem, "admin-1", "admin")
	admin.Role = "admin"
	s := newTestServers(t, mem)

	createBody, _ := json.Marshal(map[string]any{
		"subject":  "SSL 证书",
		"category": "technical",
		"body":     "证书申请失败",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/tickets", bytes.NewReader(createBody))
	createReq = createReq.WithContext(context.WithValue(createReq.Context(), ctxKeyUserID, user.ID))
	createW := httptest.NewRecorder()
	s.handleTickets(createW, createReq)
	var created struct {
		Ticket store.Ticket `json:"ticket"`
	}
	_ = json.Unmarshal(createW.Body.Bytes(), &created)
	id := created.Ticket.ID

	closeReq := httptest.NewRequest(http.MethodPost, "/api/tickets/"+id+"/close", nil)
	closeReq = closeReq.WithContext(context.WithValue(closeReq.Context(), ctxKeyUserID, user.ID))
	closeW := httptest.NewRecorder()
	s.handleTicketByID(closeW, closeReq)

	adminReply, _ := json.Marshal(map[string]any{"body": "已重新签发，请刷新页面"})
	adminReq := httptest.NewRequest(http.MethodPost, "/api/admin/tickets/"+id+"/replies", bytes.NewReader(adminReply))
	adminReq = adminReq.WithContext(context.WithValue(adminReq.Context(), ctxKeyUserID, admin.ID))
	adminW := httptest.NewRecorder()
	s.handleAdminTicketByID(adminW, adminReq)
	if adminW.Code != http.StatusOK {
		t.Fatalf("admin reply: expected 200, got %d body=%s", adminW.Code, adminW.Body.String())
	}
	var adminResp struct {
		Ticket store.Ticket `json:"ticket"`
	}
	if err := json.Unmarshal(adminW.Body.Bytes(), &adminResp); err != nil {
		t.Fatalf("decode admin reply: %v", err)
	}
	if adminResp.Ticket.Status != "replied" {
		t.Fatalf("expected reopened status replied, got %q", adminResp.Ticket.Status)
	}
}

func TestHandleAdminTicketsListAndPatch(t *testing.T) {
	mem := store.NewMemory("test-token", "admin-token")
	user := seedTicketUser(t, mem, "user-5", "eve")
	s := newTestServers(t, mem)

	createBody, _ := json.Marshal(map[string]any{
		"subject":  "功能建议",
		"category": "general",
		"body":     "希望增加批量导入",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/tickets", bytes.NewReader(createBody))
	createReq = createReq.WithContext(context.WithValue(createReq.Context(), ctxKeyUserID, user.ID))
	createW := httptest.NewRecorder()
	s.handleTickets(createW, createReq)
	var created struct {
		Ticket store.Ticket `json:"ticket"`
	}
	_ = json.Unmarshal(createW.Body.Bytes(), &created)
	id := created.Ticket.ID

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/tickets?status=open", nil)
	listW := httptest.NewRecorder()
	s.handleAdminTickets(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("admin list: expected 200, got %d body=%s", listW.Code, listW.Body.String())
	}

	patchBody, _ := json.Marshal(map[string]any{"priority": "high", "status": "replied"})
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/admin/tickets/"+id, bytes.NewReader(patchBody))
	patchW := httptest.NewRecorder()
	s.handleAdminTicketByID(patchW, patchReq)
	if patchW.Code != http.StatusOK {
		t.Fatalf("admin patch: expected 200, got %d body=%s", patchW.Code, patchW.Body.String())
	}
	var patched struct {
		Ticket store.Ticket `json:"ticket"`
	}
	if err := json.Unmarshal(patchW.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if patched.Ticket.Priority != "high" || patched.Ticket.Status != "replied" {
		t.Fatalf("unexpected patched ticket: %+v", patched.Ticket)
	}
}
