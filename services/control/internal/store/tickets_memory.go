package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (m *Memory) CreateTicket(ctx context.Context, t *Ticket, initialBody string) error {
	_ = ctx
	if t == nil || strings.TrimSpace(t.UserID) == "" {
		return fmt.Errorf("ticket user_id required")
	}
	subject := strings.TrimSpace(t.Subject)
	body := strings.TrimSpace(initialBody)
	if subject == "" || body == "" {
		return fmt.Errorf("subject and body required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tickets == nil {
		m.tickets = make(map[string]*Ticket)
	}
	if m.ticketReplies == nil {
		m.ticketReplies = make(map[string][]*TicketReply)
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	cp := *t
	cp.Subject = subject
	cp.Category = normalizeTicketCategory(cp.Category)
	cp.Priority = normalizeTicketPriority(cp.Priority)
	cp.Status = "open"
	cp.LastReplyBy = "user"
	cp.ReplyCount = 1
	cp.CreatedAt = now
	cp.UpdatedAt = now
	m.tickets[cp.ID] = &cp
	reply := &TicketReply{
		ID:         uuid.NewString(),
		TicketID:   cp.ID,
		AuthorID:   cp.UserID,
		AuthorRole: "user",
		AuthorName: strings.TrimSpace(cp.Username),
		Body:       body,
		CreatedAt:  now,
	}
	m.ticketReplies[cp.ID] = []*TicketReply{reply}
	*t = cp
	return nil
}

func (m *Memory) GetTicket(ctx context.Context, id string) (*Ticket, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tickets[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	if u := m.usersByIDLocked(cp.UserID); u != nil {
		cp.Username = u.Username
		cp.Email = u.Email
	}
	return &cp, nil
}

func (m *Memory) usersByIDLocked(id string) *User {
	for _, u := range m.users {
		if u != nil && u.ID == id {
			return u
		}
	}
	return nil
}

func (m *Memory) memoryListTickets(userID, status string, page, pageSize int, admin bool) ([]*Ticket, int64) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	all := make([]*Ticket, 0, len(m.tickets))
	for _, t := range m.tickets {
		if t == nil {
			continue
		}
		if !admin && t.UserID != strings.TrimSpace(userID) {
			continue
		}
		if admin && strings.TrimSpace(userID) != "" && t.UserID != strings.TrimSpace(userID) {
			continue
		}
		if st := normalizeTicketStatus(status); st != "" && t.Status != st {
			continue
		}
		cp := *t
		if u := m.usersByIDLocked(cp.UserID); u != nil {
			cp.Username = u.Username
			cp.Email = u.Email
		}
		all = append(all, &cp)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	total := int64(len(all))
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []*Ticket{}, total
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total
}

func (m *Memory) ListTicketsByUser(ctx context.Context, userID, status string, page, pageSize int) ([]*Ticket, int64, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	items, total := m.memoryListTickets(userID, status, page, pageSize, false)
	return items, total, nil
}

func (m *Memory) ListTicketsAdmin(ctx context.Context, userID, status string, page, pageSize int) ([]*Ticket, int64, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	items, total := m.memoryListTickets(userID, status, page, pageSize, true)
	return items, total, nil
}

func (m *Memory) UpdateTicket(ctx context.Context, t *Ticket) error {
	_ = ctx
	if t == nil || strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf("ticket id required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.tickets[t.ID]
	if !ok {
		return fmt.Errorf("ticket not found")
	}
	existing.Subject = strings.TrimSpace(t.Subject)
	if existing.Subject == "" {
		existing.Subject = t.Subject
	}
	if cat := normalizeTicketCategory(t.Category); cat != "" {
		existing.Category = cat
	}
	if st := normalizeTicketStatus(t.Status); st != "" {
		existing.Status = st
	}
	if pr := normalizeTicketPriority(t.Priority); pr != "" {
		existing.Priority = pr
	}
	existing.LastReplyBy = t.LastReplyBy
	existing.ReplyCount = t.ReplyCount
	existing.UpdatedAt = time.Now().UTC()
	if existing.Status == "closed" {
		if t.ClosedAt != nil {
			existing.ClosedAt = t.ClosedAt
		} else if existing.ClosedAt == nil {
			now := existing.UpdatedAt
			existing.ClosedAt = &now
		}
	} else {
		existing.ClosedAt = nil
	}
	return nil
}

func (m *Memory) AddTicketReply(ctx context.Context, reply *TicketReply) error {
	_ = ctx
	if reply == nil || strings.TrimSpace(reply.TicketID) == "" {
		return fmt.Errorf("ticket_id required")
	}
	body := strings.TrimSpace(reply.Body)
	if body == "" {
		return fmt.Errorf("body required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[reply.TicketID]
	if !ok {
		return fmt.Errorf("ticket not found")
	}
	if t.Status == "closed" {
		return fmt.Errorf("ticket closed")
	}
	if reply.ID == "" {
		reply.ID = uuid.NewString()
	}
	reply.Body = body
	reply.CreatedAt = time.Now().UTC()
	if reply.AuthorRole == "" {
		reply.AuthorRole = "user"
	}
	cp := *reply
	m.ticketReplies[reply.TicketID] = append(m.ticketReplies[reply.TicketID], &cp)
	t.ReplyCount++
	t.LastReplyBy = reply.AuthorRole
	t.UpdatedAt = reply.CreatedAt
	t.ClosedAt = nil
	if reply.AuthorRole == "admin" {
		t.Status = "replied"
	} else {
		t.Status = "open"
	}
	return nil
}

func (m *Memory) ListTicketReplies(ctx context.Context, ticketID string) ([]*TicketReply, error) {
	_ = ctx
	ticketID = strings.TrimSpace(ticketID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	replies := m.ticketReplies[ticketID]
	out := make([]*TicketReply, 0, len(replies))
	for _, r := range replies {
		if r == nil {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
