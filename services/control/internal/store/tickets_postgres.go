package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func normalizeTicketCategory(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "billing", "technical":
		return strings.ToLower(strings.TrimSpace(c))
	default:
		return "general"
	}
}

func normalizeTicketPriority(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "low", "high":
		return strings.ToLower(strings.TrimSpace(p))
	default:
		return "normal"
	}
}

func normalizeTicketStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "open", "replied", "closed":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return ""
	}
}

func (p *Postgres) CreateTicket(ctx context.Context, t *Ticket, initialBody string) error {
	if t == nil || strings.TrimSpace(t.UserID) == "" {
		return fmt.Errorf("ticket user_id required")
	}
	subject := strings.TrimSpace(t.Subject)
	body := strings.TrimSpace(initialBody)
	if subject == "" || body == "" {
		return fmt.Errorf("subject and body required")
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	t.Subject = subject
	t.Category = normalizeTicketCategory(t.Category)
	t.Priority = normalizeTicketPriority(t.Priority)
	if t.Status == "" {
		t.Status = "open"
	}
	t.LastReplyBy = "user"
	t.ReplyCount = 1
	t.CreatedAt = now
	t.UpdatedAt = now

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO tickets (id, user_id, subject, category, status, priority, last_reply_by, reply_count, created_at, updated_at, closed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		t.ID, t.UserID, t.Subject, t.Category, t.Status, t.Priority, t.LastReplyBy, t.ReplyCount, t.CreatedAt, t.UpdatedAt, t.ClosedAt,
	); err != nil {
		return err
	}

	reply := &TicketReply{
		ID:         uuid.NewString(),
		TicketID:   t.ID,
		AuthorID:   t.UserID,
		AuthorRole: "user",
		AuthorName: strings.TrimSpace(t.Username),
		Body:       body,
		CreatedAt:  now,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ticket_replies (id, ticket_id, author_id, author_role, author_name, body, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		reply.ID, reply.TicketID, reply.AuthorID, reply.AuthorRole, reply.AuthorName, reply.Body, reply.CreatedAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) GetTicket(ctx context.Context, id string) (*Ticket, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	row := p.pool.QueryRow(ctx, `
		SELECT t.id, t.user_id, t.subject, t.category, t.status, t.priority, t.last_reply_by, t.reply_count,
		       t.created_at, t.updated_at, t.closed_at, COALESCE(u.username,''), COALESCE(u.email,'')
		FROM tickets t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE t.id = $1`, id)
	var t Ticket
	if err := row.Scan(&t.ID, &t.UserID, &t.Subject, &t.Category, &t.Status, &t.Priority, &t.LastReplyBy, &t.ReplyCount,
		&t.CreatedAt, &t.UpdatedAt, &t.ClosedAt, &t.Username, &t.Email); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (p *Postgres) listTickets(ctx context.Context, userID, status string, page, pageSize int, admin bool) ([]*Ticket, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	args := []any{}
	where := []string{"1=1"}
	n := 1
	if !admin {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			return nil, 0, fmt.Errorf("user_id required")
		}
		where = append(where, fmt.Sprintf("t.user_id = $%d", n))
		args = append(args, userID)
		n++
	} else if uid := strings.TrimSpace(userID); uid != "" {
		where = append(where, fmt.Sprintf("t.user_id = $%d", n))
		args = append(args, uid)
		n++
	}
	if st := normalizeTicketStatus(status); st != "" {
		where = append(where, fmt.Sprintf("t.status = $%d", n))
		args = append(args, st)
		n++
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM tickets t WHERE %s`, whereSQL)
	if err := p.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	listQ := fmt.Sprintf(`
		SELECT t.id, t.user_id, t.subject, t.category, t.status, t.priority, t.last_reply_by, t.reply_count,
		       t.created_at, t.updated_at, t.closed_at, COALESCE(u.username,''), COALESCE(u.email,'')
		FROM tickets t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE %s
		ORDER BY t.updated_at DESC
		LIMIT $%d OFFSET $%d`, whereSQL, n, n+1)
	args = append(args, pageSize, offset)
	rows, err := p.pool.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*Ticket, 0)
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.UserID, &t.Subject, &t.Category, &t.Status, &t.Priority, &t.LastReplyBy, &t.ReplyCount,
			&t.CreatedAt, &t.UpdatedAt, &t.ClosedAt, &t.Username, &t.Email); err != nil {
			return nil, 0, err
		}
		out = append(out, &t)
	}
	return out, total, rows.Err()
}

func (p *Postgres) ListTicketsByUser(ctx context.Context, userID, status string, page, pageSize int) ([]*Ticket, int64, error) {
	return p.listTickets(ctx, userID, status, page, pageSize, false)
}

func (p *Postgres) ListTicketsAdmin(ctx context.Context, userID, status string, page, pageSize int) ([]*Ticket, int64, error) {
	return p.listTickets(ctx, userID, status, page, pageSize, true)
}

func (p *Postgres) UpdateTicket(ctx context.Context, t *Ticket) error {
	if t == nil || strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf("ticket id required")
	}
	t.UpdatedAt = time.Now().UTC()
	if t.Status == "closed" && t.ClosedAt == nil {
		now := t.UpdatedAt
		t.ClosedAt = &now
	}
	if t.Status != "closed" {
		t.ClosedAt = nil
	}
	_, err := p.pool.Exec(ctx, `
		UPDATE tickets SET subject=$2, category=$3, status=$4, priority=$5, last_reply_by=$6, reply_count=$7, updated_at=$8, closed_at=$9
		WHERE id=$1`,
		t.ID, t.Subject, normalizeTicketCategory(t.Category), t.Status, normalizeTicketPriority(t.Priority),
		t.LastReplyBy, t.ReplyCount, t.UpdatedAt, t.ClosedAt,
	)
	return err
}

func (p *Postgres) AddTicketReply(ctx context.Context, reply *TicketReply) error {
	if reply == nil || strings.TrimSpace(reply.TicketID) == "" {
		return fmt.Errorf("ticket_id required")
	}
	body := strings.TrimSpace(reply.Body)
	if body == "" {
		return fmt.Errorf("body required")
	}
	if reply.ID == "" {
		reply.ID = uuid.NewString()
	}
	reply.Body = body
	reply.CreatedAt = time.Now().UTC()
	if reply.AuthorRole == "" {
		reply.AuthorRole = "user"
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO ticket_replies (id, ticket_id, author_id, author_role, author_name, body, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		reply.ID, reply.TicketID, reply.AuthorID, reply.AuthorRole, reply.AuthorName, reply.Body, reply.CreatedAt,
	); err != nil {
		return err
	}

	status := "replied"
	if reply.AuthorRole == "user" {
		status = "open"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tickets SET last_reply_by=$2, reply_count=reply_count+1, status=$3, updated_at=$4, closed_at=NULL
		WHERE id=$1`,
		reply.TicketID, reply.AuthorRole, status, reply.CreatedAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ListTicketReplies(ctx context.Context, ticketID string) ([]*TicketReply, error) {
	ticketID = strings.TrimSpace(ticketID)
	if ticketID == "" {
		return nil, nil
	}
	rows, err := p.pool.Query(ctx, `
		SELECT id, ticket_id, author_id, author_role, author_name, body, created_at
		FROM ticket_replies WHERE ticket_id=$1 ORDER BY created_at ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*TicketReply, 0)
	for rows.Next() {
		var r TicketReply
		if err := rows.Scan(&r.ID, &r.TicketID, &r.AuthorID, &r.AuthorRole, &r.AuthorName, &r.Body, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}
