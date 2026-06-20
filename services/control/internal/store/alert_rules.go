package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func scanAlertRule(row interface {
	Scan(dest ...any) error
}) (*AlertRule, error) {
	r := &AlertRule{}
	if err := row.Scan(
		&r.ID, &r.Name, &r.Metric, &r.Threshold, &r.WindowSeconds,
		&r.Severity, &r.Enabled, &r.NotifyChannels, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return r, nil
}

const alertRuleColumns = `id, name, metric, threshold, window_seconds, severity, enabled, notify_channels, created_at, updated_at`

func (p *Postgres) ListAlertRules(ctx context.Context) ([]*AlertRule, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+alertRuleColumns+` FROM alert_rules ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) GetAlertRule(ctx context.Context, id string) (*AlertRule, error) {
	row := p.pool.QueryRow(ctx, `SELECT `+alertRuleColumns+` FROM alert_rules WHERE id=$1`, id)
	r, err := scanAlertRule(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}

func (p *Postgres) CreateAlertRule(ctx context.Context, r *AlertRule) error {
	if r == nil || strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("invalid alert rule")
	}
	now := time.Now()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}
	if r.NotifyChannels == nil {
		r.NotifyChannels = []string{}
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO alert_rules (id, name, metric, threshold, window_seconds, severity, enabled, notify_channels, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		r.ID, r.Name, r.Metric, r.Threshold, r.WindowSeconds, r.Severity, r.Enabled, r.NotifyChannels, r.CreatedAt, r.UpdatedAt,
	)
	return err
}

func (p *Postgres) UpdateAlertRule(ctx context.Context, r *AlertRule) error {
	if r == nil || strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("invalid alert rule")
	}
	if r.NotifyChannels == nil {
		r.NotifyChannels = []string{}
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE alert_rules SET name=$2, metric=$3, threshold=$4, window_seconds=$5, severity=$6, enabled=$7, notify_channels=$8, updated_at=NOW()
		 WHERE id=$1`,
		r.ID, r.Name, r.Metric, r.Threshold, r.WindowSeconds, r.Severity, r.Enabled, r.NotifyChannels,
	)
	return err
}

func (p *Postgres) DeleteAlertRule(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id=$1`, id)
	return err
}

func (m *Memory) ListAlertRules(ctx context.Context) ([]*AlertRule, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*AlertRule, 0, len(m.alertRules))
	for _, r := range m.alertRules {
		if r != nil {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *Memory) GetAlertRule(ctx context.Context, id string) (*AlertRule, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()
	r := m.alertRules[id]
	if r == nil {
		return nil, nil
	}
	cp := *r
	return &cp, nil
}

func (m *Memory) CreateAlertRule(ctx context.Context, r *AlertRule) error {
	_ = ctx
	if r == nil || r.ID == "" {
		return fmt.Errorf("invalid alert rule")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *r
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	cp.UpdatedAt = time.Now()
	m.alertRules[cp.ID] = &cp
	return nil
}

func (m *Memory) UpdateAlertRule(ctx context.Context, r *AlertRule) error {
	_ = ctx
	if r == nil || r.ID == "" {
		return fmt.Errorf("invalid alert rule")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.alertRules[r.ID]; !ok {
		return fmt.Errorf("alert rule not found")
	}
	cp := *r
	cp.UpdatedAt = time.Now()
	m.alertRules[cp.ID] = &cp
	return nil
}

func (m *Memory) DeleteAlertRule(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.alertRules, id)
	return nil
}
