package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Postgres implements Store using pgxpool.
type Postgres struct {
	pool *pgxpool.Pool
}

var _ Store = (*Postgres)(nil)

// NewPostgres connects to PostgreSQL and returns a Store.
func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

// Ping ensures the database is reachable.
func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// Migrate runs database migrations.
func (p *Postgres) Migrate(ctx context.Context) error {
	log.Info().Msg("running database migrations")

	migrations := []string{
		// Nodes table
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			hostname TEXT NOT NULL UNIQUE,
			public_ip TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			region TEXT NOT NULL DEFAULT '',
			cluster TEXT NOT NULL DEFAULT '',
			capabilities TEXT[] NOT NULL DEFAULT '{}',
			config_version TEXT NOT NULL DEFAULT '',
			token TEXT NOT NULL,
			last_heartbeat TIMESTAMPTZ,
			monitor_enabled BOOLEAN NOT NULL DEFAULT false,
			monitor_protocol TEXT NOT NULL DEFAULT 'http',
			monitor_timeout_seconds INT NOT NULL DEFAULT 5,
			monitor_port INT NOT NULL DEFAULT 80,
			monitor_fail_threshold INT NOT NULL DEFAULT 3,
			monitor_fail_count INT NOT NULL DEFAULT 0,
			monitor_last_ok BOOLEAN NOT NULL DEFAULT false,
			monitor_last_error TEXT NOT NULL DEFAULT '',
			monitor_last_at TIMESTAMPTZ,
			monitor_last_latency_ms INT NOT NULL DEFAULT 0,
			last_metrics_at TIMESTAMPTZ,
			cpu_usage DOUBLE PRECISION NOT NULL DEFAULT 0,
			mem_usage DOUBLE PRECISION NOT NULL DEFAULT 0,
			disk_usage DOUBLE PRECISION NOT NULL DEFAULT 0,
			cpu_count INT NOT NULL DEFAULT 0,
			mem_total BIGINT NOT NULL DEFAULT 0,
			disk_total BIGINT NOT NULL DEFAULT 0,
			bytes_sent BIGINT NOT NULL DEFAULT 0,
			bytes_received BIGINT NOT NULL DEFAULT 0,
			bandwidth_up_bps DOUBLE PRECISION NOT NULL DEFAULT 0,
			bandwidth_down_bps DOUBLE PRECISION NOT NULL DEFAULT 0,
			tcp_established INT NOT NULL DEFAULT 0,
			tcp_syn_recv INT NOT NULL DEFAULT 0,
			tcp_time_wait INT NOT NULL DEFAULT 0,
			nginx_running BOOLEAN NOT NULL DEFAULT false,
			month_bytes_sent BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS public_ip TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS region TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS cluster TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS last_metrics_at TIMESTAMPTZ`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_enabled BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_protocol TEXT NOT NULL DEFAULT 'http'`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_timeout_seconds INT NOT NULL DEFAULT 5`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_port INT NOT NULL DEFAULT 80`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_fail_threshold INT NOT NULL DEFAULT 3`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_fail_count INT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_last_ok BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_last_error TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_last_at TIMESTAMPTZ`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS monitor_last_latency_ms INT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS cpu_usage DOUBLE PRECISION NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS mem_usage DOUBLE PRECISION NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS disk_usage DOUBLE PRECISION NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS cpu_count INT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS mem_total BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS disk_total BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS bytes_sent BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS bytes_received BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS bandwidth_up_bps DOUBLE PRECISION NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS bandwidth_down_bps DOUBLE PRECISION NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS tcp_established INT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS tcp_syn_recv INT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS tcp_time_wait INT NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS nginx_running BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS month_bytes_sent BIGINT NOT NULL DEFAULT 0`,

		// Origins table
		`CREATE TABLE IF NOT EXISTS origins (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			addresses TEXT[] NOT NULL DEFAULT '{}',
			timeout_ms BIGINT NOT NULL DEFAULT 30000,
			max_retries INT NOT NULL DEFAULT 3,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Certificates table
		`CREATE TABLE IF NOT EXISTS certificates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			domain TEXT NOT NULL,
			cert_pem BYTEA NOT NULL,
			key_pem BYTEA NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_certificates_domain ON certificates(domain)`,
		`ALTER TABLE certificates ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_certificates_user_id ON certificates(user_id)`,

		// Domains table
		`CREATE TABLE IF NOT EXISTS domains (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			cname TEXT NOT NULL DEFAULT '',
			line_group_id TEXT,
			origin_id TEXT REFERENCES origins(id),
			cert_id TEXT REFERENCES certificates(id),
			origin_scheme TEXT NOT NULL DEFAULT 'http',
			origin_port INT NOT NULL DEFAULT 80,
			origin_host_mode TEXT NOT NULL DEFAULT 'request_host',
			origin_host TEXT NOT NULL DEFAULT '',
			origin_timeout_ms BIGINT NOT NULL DEFAULT 60000,
			origin_connect_timeout_ms BIGINT NOT NULL DEFAULT 10000,
			error_pages JSONB NOT NULL DEFAULT '[]'::jsonb,
			cache_enabled BOOLEAN NOT NULL DEFAULT true,
			http2_enabled BOOLEAN NOT NULL DEFAULT true,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_domains_name ON domains(name)`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS cname TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS line_group_id TEXT`,
		`ALTER TABLE domains ALTER COLUMN origin_id DROP NOT NULL`,
		`ALTER TABLE domains DROP CONSTRAINT IF EXISTS domains_origin_id_fkey`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS origin_scheme TEXT NOT NULL DEFAULT 'http'`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS origin_port INT NOT NULL DEFAULT 80`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS origin_host_mode TEXT NOT NULL DEFAULT 'request_host'`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS origin_host TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS origin_timeout_ms BIGINT NOT NULL DEFAULT 60000`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS origin_connect_timeout_ms BIGINT NOT NULL DEFAULT 10000`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS error_pages JSONB NOT NULL DEFAULT '[]'::jsonb`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_domains_user_id ON domains(user_id)`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS cache_enabled BOOLEAN NOT NULL DEFAULT true`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS http2_enabled BOOLEAN NOT NULL DEFAULT true`,
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS websocket_enabled BOOLEAN NOT NULL DEFAULT false`,
		// Dedicated HTTPS master switch. Previously derived from `cert_id IS
		// NOT NULL`, which made the UI toggle impossible to reason about:
		// disabling HTTPS required unbinding the cert, and new ACME
		// issuances implicitly turned 443 on. Defaults to TRUE when a cert
		// is bound (preserve legacy behavior) and FALSE otherwise.
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS https_enabled BOOLEAN NOT NULL DEFAULT false`,
		`UPDATE domains SET https_enabled = true WHERE cert_id IS NOT NULL AND cert_id <> '' AND https_enabled = false`,
		// Per-domain security settings (CC protection + IP black/white
		// lists + custom rules). Stored as opaque JSONB because the
		// shape evolves faster than the DB schema — readers decode it
		// into store.DomainSecurity.
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS security_json JSONB NOT NULL DEFAULT '{}'::jsonb`,

		// Per-domain origin authentication settings. Stored as opaque JSONB
		// so the auth shape can evolve (new modes, etc.) without schema churn.
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS origin_auth_json JSONB NOT NULL DEFAULT '{}'::jsonb`,

		// Origin load-balance method: "round_robin" (default, weighted random)
		// or "ip_hash" (consistent hashing on client IP for sticky sessions).
		// Empty string is treated as round_robin to keep backward compat.
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS load_balance_method TEXT NOT NULL DEFAULT 'round_robin'`,

		// Per-domain origin health-check config. JSONB so we can extend
		// the probe shape (header expectations, body match, etc.) without
		// schema churn. Empty object means "disabled".
		`ALTER TABLE domains ADD COLUMN IF NOT EXISTS origin_health_check_json JSONB NOT NULL DEFAULT '{}'::jsonb`,

		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'domains_origin_id_fkey' AND table_name = 'domains'
			) THEN
				ALTER TABLE domains ADD CONSTRAINT domains_origin_id_fkey FOREIGN KEY (origin_id) REFERENCES origins(id) ON DELETE SET NULL;
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'domains' AND indexname = 'idx_domains_cname_lower') THEN
				CREATE UNIQUE INDEX idx_domains_cname_lower ON domains (LOWER(cname)) WHERE cname <> '';
			END IF;
		END $$`,

		// domain_origins: authoritative per-domain origin addresses.
		// Replaces the "global origin pool + domain.origin_id" model.
		// Each row = one upstream address owned by one domain, carrying
		// its own weight and enabled flag. The node does weighted random
		// failover across enabled rows. Deleting a domain cascades.
		`CREATE TABLE IF NOT EXISTS domain_origins (
			id TEXT PRIMARY KEY,
			domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			address TEXT NOT NULL,
			weight INT NOT NULL DEFAULT 1,
			enabled BOOLEAN NOT NULL DEFAULT true,
			sort_order INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_domain_origins_domain_id ON domain_origins(domain_id)`,
		// One-shot backfill from legacy origins / domains.origin_id.
		// Fires only when the new table is empty but legacy data exists,
		// so subsequent startups are no-ops. Expands origins.addresses
		// (one address per row) with weight=1 enabled=true.
		`DO $$
		DECLARE
			already_backfilled BOOLEAN;
		BEGIN
			SELECT EXISTS(SELECT 1 FROM domain_origins) INTO already_backfilled;
			IF already_backfilled THEN RETURN; END IF;
			INSERT INTO domain_origins (id, domain_id, address, weight, enabled, sort_order, created_at, updated_at)
			SELECT
				d.id || ':' || (row_number() OVER (PARTITION BY d.id ORDER BY ord))::TEXT,
				d.id,
				addr,
				1,
				true,
				(row_number() OVER (PARTITION BY d.id ORDER BY ord))::INT - 1,
				NOW(),
				NOW()
			FROM domains d
			JOIN origins o ON o.id = d.origin_id
			CROSS JOIN LATERAL unnest(o.addresses) WITH ORDINALITY AS t(addr, ord)
			WHERE d.origin_id IS NOT NULL AND array_length(o.addresses, 1) > 0;
		EXCEPTION WHEN others THEN
			-- Legacy origins table may not exist in fresh installs, or
			-- migration race; either way skip silently.
			RETURN;
		END $$`,

		// Line groups table
		`CREATE TABLE IF NOT EXISTS line_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			domain TEXT NOT NULL DEFAULT '',
			cname TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			sort INT NOT NULL DEFAULT 100,
			node_ids TEXT[] NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE line_groups ADD COLUMN IF NOT EXISTS domain TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE line_groups ADD COLUMN IF NOT EXISTS cname TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE line_groups ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE line_groups ADD COLUMN IF NOT EXISTS sort INT NOT NULL DEFAULT 100`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'line_groups' AND indexname = 'idx_line_groups_name_lower') THEN
				CREATE UNIQUE INDEX idx_line_groups_name_lower ON line_groups (LOWER(name)) WHERE name <> '';
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'line_groups' AND indexname = 'idx_line_groups_domain_lower') THEN
				CREATE UNIQUE INDEX idx_line_groups_domain_lower ON line_groups (LOWER(domain)) WHERE domain <> '';
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'line_groups' AND indexname = 'idx_line_groups_cname_lower') THEN
				CREATE UNIQUE INDEX idx_line_groups_cname_lower ON line_groups (LOWER(cname)) WHERE cname <> '';
			END IF;
		END $$`,
		`CREATE TABLE IF NOT EXISTS line_group_nodes (
			line_group_id TEXT NOT NULL REFERENCES line_groups(id) ON DELETE CASCADE,
			line TEXT NOT NULL DEFAULT '默认',
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			enabled BOOLEAN NOT NULL DEFAULT true,
			weight INT NOT NULL DEFAULT 1,
			backup BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (line_group_id, line, node_id)
		)`,
		`ALTER TABLE line_group_nodes ADD COLUMN IF NOT EXISTS line TEXT NOT NULL DEFAULT '默认'`,
		`ALTER TABLE line_group_nodes ALTER COLUMN line SET DEFAULT '默认'`,
		`UPDATE line_group_nodes SET line = '默认' WHERE line IS NULL OR line = '' OR line = 'default'`,
		`ALTER TABLE line_group_nodes DROP CONSTRAINT IF EXISTS line_group_nodes_pkey`,
		`ALTER TABLE line_group_nodes ADD PRIMARY KEY (line_group_id, line, node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_line_group_nodes_group ON line_group_nodes(line_group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_line_group_nodes_group_line ON line_group_nodes(line_group_id, line)`,
		`CREATE INDEX IF NOT EXISTS idx_line_group_nodes_node ON line_group_nodes(node_id)`,

		// Cache rules table
		`CREATE TABLE IF NOT EXISTS cache_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			host_pattern TEXT NOT NULL DEFAULT '*',
			path_pattern TEXT NOT NULL DEFAULT '*',
			methods TEXT[] NOT NULL DEFAULT '{GET}',
			ttl_seconds BIGINT NOT NULL DEFAULT 3600,
			cache_query_params BOOLEAN NOT NULL DEFAULT false,
			priority INT NOT NULL DEFAULT 0,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS ban_seconds BIGINT NOT NULL DEFAULT 300`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS template_html TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS ban_template_html TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS redirect_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS ban_mode TEXT NOT NULL DEFAULT 'ipset'`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS path_prefix TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS methods TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS ua_contains TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE waf_rules ADD COLUMN IF NOT EXISTS log_only BOOLEAN NOT NULL DEFAULT false`,

		// Config versions table
		`CREATE TABLE IF NOT EXISTS config_versions (
			version TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			payload BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by TEXT NOT NULL DEFAULT 'system'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_config_versions_created_at ON config_versions(created_at DESC)`,

		// License state table (single row)
		`CREATE TABLE IF NOT EXISTS license_state (
			id INT PRIMARY KEY DEFAULT 1,
			payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Settings table (single row)
		`CREATE TABLE IF NOT EXISTS settings (
			id TEXT PRIMARY KEY,
			system_name TEXT NOT NULL DEFAULT '',
			footer_links TEXT NOT NULL DEFAULT '',
			footer_copyright TEXT NOT NULL DEFAULT '',
			favicon TEXT NOT NULL DEFAULT '',
			logo TEXT NOT NULL DEFAULT '',
			smtp_host TEXT NOT NULL DEFAULT '',
			smtp_port INT NOT NULL DEFAULT 587,
			smtp_username TEXT NOT NULL DEFAULT '',
			smtp_password TEXT NOT NULL DEFAULT '',
			smtp_from TEXT NOT NULL DEFAULT '',
			smtp_from_name TEXT NOT NULL DEFAULT '',
			elasticsearch_url TEXT NOT NULL DEFAULT '',
			elasticsearch_user TEXT NOT NULL DEFAULT '',
			elasticsearch_pass TEXT NOT NULL DEFAULT '',
			elasticsearch_index TEXT NOT NULL DEFAULT '',
			elasticsearch_ts_field TEXT NOT NULL DEFAULT '',
			elasticsearch_domain_field TEXT NOT NULL DEFAULT '',
			elasticsearch_bytes_field TEXT NOT NULL DEFAULT '',
			sales_email TEXT NOT NULL DEFAULT '',
			support_email TEXT NOT NULL DEFAULT '',
			register_enabled BOOLEAN NOT NULL DEFAULT true,
			upgrade_channel TEXT NOT NULL DEFAULT 'stable',
			notify_new_build BOOLEAN NOT NULL DEFAULT true,
			register_email_verification BOOLEAN NOT NULL DEFAULT false,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS system_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS footer_links TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS footer_copyright TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS favicon TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS logo TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS smtp_host TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS smtp_port INT NOT NULL DEFAULT 587`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS smtp_username TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS smtp_password TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS smtp_from TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS smtp_from_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS elasticsearch_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS elasticsearch_user TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS elasticsearch_pass TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS elasticsearch_index TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS elasticsearch_ts_field TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS elasticsearch_domain_field TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS elasticsearch_bytes_field TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS sales_email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS support_email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS register_enabled BOOLEAN NOT NULL DEFAULT true`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS upgrade_channel TEXT NOT NULL DEFAULT 'stable'`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS notify_new_build BOOLEAN NOT NULL DEFAULT true`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS register_email_verification BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS email_enabled BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS dingtalk_enabled BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS dingtalk_webhook TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS wechat_enabled BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS wechat_webhook TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS feishu_enabled BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS feishu_webhook TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS notify_node_resource BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS notify_node_monitor BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS notify_ticket_reply BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS notify_interval INT NOT NULL DEFAULT 5`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS threshold_cpu INT NOT NULL DEFAULT 0`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS threshold_memory INT NOT NULL DEFAULT 0`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS threshold_disk INT NOT NULL DEFAULT 0`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS threshold_bandwidth_up INT NOT NULL DEFAULT 0`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS threshold_bandwidth_down INT NOT NULL DEFAULT 0`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS retention_system_logs INT NOT NULL DEFAULT 90`,
		`ALTER TABLE settings DROP COLUMN IF EXISTS retention_node_logs`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS retention_es_logs INT NOT NULL DEFAULT 7`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS retention_waf_bans INT NOT NULL DEFAULT 7`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS retention_upgrade_logs INT NOT NULL DEFAULT 30`,
		// Log shipper columns existed pre-Filebeat-migration; drop them now
		// that node-side log delivery happens via Filebeat directly to ES.
		`ALTER TABLE settings DROP COLUMN IF EXISTS log_shipper_enabled`,
		`ALTER TABLE settings DROP COLUMN IF EXISTS log_shipper_batch_size`,
		`ALTER TABLE settings DROP COLUMN IF EXISTS log_shipper_flush_ms`,
		`ALTER TABLE settings DROP COLUMN IF EXISTS log_shipper_queue_max`,
		`ALTER TABLE settings DROP COLUMN IF EXISTS log_shipper_endpoint`,

		`CREATE TABLE IF NOT EXISTS balance_accounts (
			user_id TEXT PRIMARY KEY,
			balance_cents BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'CNY',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS balance_transactions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			amount_cents BIGINT NOT NULL DEFAULT 0,
			balance_cents BIGINT NOT NULL DEFAULT 0,
			note TEXT NOT NULL DEFAULT '',
			ref_type TEXT NOT NULL DEFAULT '',
			ref_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_balance_tx_user_time ON balance_transactions(user_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS balance_recharges (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			out_trade_no TEXT NOT NULL UNIQUE,
			amount_cents BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'CNY',
			payment_method TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			trade_no TEXT NOT NULL DEFAULT '',
			paid_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_balance_recharges_user ON balance_recharges(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_balance_recharges_status ON balance_recharges(status)`,
		`CREATE INDEX IF NOT EXISTS idx_balance_recharges_created ON balance_recharges(created_at)`,
		`ALTER TABLE balance_recharges ADD COLUMN IF NOT EXISTS payment_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE balance_recharges ADD COLUMN IF NOT EXISTS payment_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE balance_recharges ADD COLUMN IF NOT EXISTS qr_code TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE balance_recharges ADD COLUMN IF NOT EXISTS notify_raw TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE balance_recharges ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ`,
		`ALTER TABLE balance_recharges ADD COLUMN IF NOT EXISTS closed_at TIMESTAMPTZ`,
		`CREATE TABLE IF NOT EXISTS balance_withdrawals (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			amount_cents BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'CNY',
			method TEXT NOT NULL DEFAULT '',
			account_name TEXT NOT NULL DEFAULT '',
			account_no TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			note TEXT NOT NULL DEFAULT '',
			reviewed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_balance_withdrawals_user ON balance_withdrawals(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_balance_withdrawals_status ON balance_withdrawals(status)`,
		`CREATE INDEX IF NOT EXISTS idx_balance_withdrawals_created ON balance_withdrawals(created_at)`,
		`CREATE TABLE IF NOT EXISTS announcements (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			pinned BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_announcements_status ON announcements(status)`,
		`CREATE INDEX IF NOT EXISTS idx_announcements_pinned ON announcements(pinned)`,
		`CREATE TABLE IF NOT EXISTS system_logs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			location TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_system_logs_type_time ON system_logs(type, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_system_logs_status_time ON system_logs(status, created_at DESC)`,
		// DNS config table (single row)
		`CREATE TABLE IF NOT EXISTS dns_config (
			id INT PRIMARY KEY DEFAULT 1,
			provider TEXT NOT NULL DEFAULT '',
			account_id TEXT NOT NULL DEFAULT '',
			token TEXT NOT NULL DEFAULT '',
			secret TEXT NOT NULL DEFAULT '',
			ttl BIGINT NOT NULL DEFAULT 600,
			enable_ip_weight BOOLEAN NOT NULL DEFAULT false,
			last_error TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Users table
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'admin',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_login_at TIMESTAMPTZ
		)`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS username TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_ip TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_location TEXT NOT NULL DEFAULT ''`,
		`UPDATE users SET username = email WHERE (username = '' OR username IS NULL)`,
		`UPDATE users SET status = 'active' WHERE status IS NULL OR status = ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users (LOWER(username))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (LOWER(email))`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS numeric_id SERIAL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_numeric_id ON users (numeric_id)`,

		// Email verifications
		`CREATE TABLE IF NOT EXISTS email_verifications (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			used_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_verifications_email ON email_verifications (LOWER(email))`,
		`CREATE INDEX IF NOT EXISTS idx_email_verifications_created_at ON email_verifications (created_at DESC)`,

		// Tokens table
		`CREATE TABLE IF NOT EXISTS tokens (
			id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL UNIQUE,
			token_type TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_type ON tokens(token_type)`,

		// Domain blacklist
		`CREATE TABLE IF NOT EXISTS domain_blacklist (
			id TEXT PRIMARY KEY,
			domain TEXT NOT NULL UNIQUE,
			reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_domain_blacklist_domain ON domain_blacklist(LOWER(domain))`,

		`CREATE TABLE IF NOT EXISTS global_templates (
			key TEXT PRIMARY KEY,
			content TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Upgrade tasks
		`CREATE TABLE IF NOT EXISTS upgrade_tasks (
			id TEXT PRIMARY KEY,
			target_version TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT 'stable',
			node_ids TEXT[] NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'pending',
			type TEXT NOT NULL DEFAULT 'node',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS upgrade_logs (
			id SERIAL PRIMARY KEY,
			task_id TEXT NOT NULL REFERENCES upgrade_tasks(id) ON DELETE CASCADE,
			node_id TEXT NOT NULL DEFAULT '',
			level TEXT NOT NULL DEFAULT 'INFO',
			message TEXT NOT NULL DEFAULT '',
			ts TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_upgrade_logs_task ON upgrade_logs(task_id)`,

		// WAF policies/rules
		`CREATE TABLE IF NOT EXISTS waf_policies (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT 'global',
			scope_id TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_waf_policies_scope ON waf_policies(scope, scope_id)`,
		`CREATE TABLE IF NOT EXISTS waf_rules (
			id TEXT PRIMARY KEY,
			policy_id TEXT NOT NULL REFERENCES waf_policies(id) ON DELETE CASCADE,
			type TEXT NOT NULL,
			action TEXT NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			threshold BIGINT NOT NULL DEFAULT 0,
			window_seconds BIGINT NOT NULL DEFAULT 0,
			shield_seconds INT NOT NULL DEFAULT 5,
			auto_challenge_qps BIGINT NOT NULL DEFAULT 0,
			ban_seconds BIGINT NOT NULL DEFAULT 300,
			template_html TEXT NOT NULL DEFAULT '',
			ban_template_html TEXT NOT NULL DEFAULT '',
			redirect_url TEXT NOT NULL DEFAULT '',
			ban_mode TEXT NOT NULL DEFAULT 'ipset',
			expires_at TIMESTAMPTZ,
			path_prefix TEXT NOT NULL DEFAULT '',
			methods TEXT[] NOT NULL DEFAULT '{}',
			ua_contains TEXT NOT NULL DEFAULT '',
			log_only BOOLEAN NOT NULL DEFAULT false,
			note TEXT NOT NULL DEFAULT '',
			priority INT NOT NULL DEFAULT 0,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_waf_rules_policy ON waf_rules(policy_id)`,

		// WAF bans
		`CREATE TABLE IF NOT EXISTS waf_bans (
			ip TEXT PRIMARY KEY,
			reason TEXT NOT NULL DEFAULT '',
			strikes INT NOT NULL DEFAULT 1,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_waf_bans_expires ON waf_bans(expires_at)`,

		// WAF whitelist
		`CREATE TABLE IF NOT EXISTS waf_whitelist (
			id TEXT PRIMARY KEY,
			ip TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_waf_whitelist_ip ON waf_whitelist(ip)`,
	}

	// Upgrade tasks/logs
	migrations = append(migrations,
		`CREATE TABLE IF NOT EXISTS upgrade_tasks (
			id TEXT PRIMARY KEY,
			target_version TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT 'stable',
			node_ids TEXT[] NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE upgrade_tasks ADD COLUMN IF NOT EXISTS channel TEXT NOT NULL DEFAULT 'stable'`,
		`CREATE TABLE IF NOT EXISTS upgrade_logs (
			id SERIAL PRIMARY KEY,
			task_id TEXT NOT NULL REFERENCES upgrade_tasks(id) ON DELETE CASCADE,
			node_id TEXT NOT NULL DEFAULT '',
			level TEXT NOT NULL DEFAULT 'INFO',
			message TEXT NOT NULL DEFAULT '',
			ts TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_upgrade_logs_task ON upgrade_logs(task_id)`,
	)

	migrations = append(migrations,
		`CREATE TABLE IF NOT EXISTS product_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			sort INT NOT NULL DEFAULT 100,
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'product_groups' AND indexname = 'idx_product_groups_name_lower') THEN
				CREATE UNIQUE INDEX idx_product_groups_name_lower ON product_groups (LOWER(name));
			END IF;
		END $$`,
		`CREATE TABLE IF NOT EXISTS products (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			group_id TEXT NOT NULL DEFAULT '',
			sort INT NOT NULL DEFAULT 100,
			region TEXT NOT NULL DEFAULT '',
			line_group_id TEXT NOT NULL DEFAULT '',
			monthly_traffic_bytes BIGINT,
			bandwidth_bps BIGINT,
			conn_limit BIGINT,
			domain_limit INT,
			primary_domain_limit INT,
			http_port_limit INT,
			stream_port_limit INT,
			non_std_port_limit INT,
			websocket BOOLEAN NOT NULL DEFAULT true,
			custom_cc_rules BOOLEAN NOT NULL DEFAULT true,
			http3 BOOLEAN NOT NULL DEFAULT false,
			l2_origin BOOLEAN NOT NULL DEFAULT false,
			cc_protection TEXT NOT NULL DEFAULT '',
			ddos_protection TEXT NOT NULL DEFAULT '',
			price_cents BIGINT NOT NULL DEFAULT 0,
			price_month_cents BIGINT NOT NULL DEFAULT 0,
			price_quarter_cents BIGINT NOT NULL DEFAULT 0,
			price_year_cents BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'CNY',
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS group_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS sort INT NOT NULL DEFAULT 100`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS region TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS line_group_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS monthly_traffic_bytes BIGINT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS bandwidth_bps BIGINT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS conn_limit BIGINT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS domain_limit INT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS primary_domain_limit INT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS http_port_limit INT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS stream_port_limit INT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS non_std_port_limit INT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS websocket BOOLEAN NOT NULL DEFAULT true`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS custom_cc_rules BOOLEAN NOT NULL DEFAULT true`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS http3 BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS l2_origin BOOLEAN NOT NULL DEFAULT false`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS cc_protection TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS ddos_protection TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS price_month_cents BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS price_quarter_cents BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS price_year_cents BIGINT NOT NULL DEFAULT 0`,
		`UPDATE products SET price_month_cents = price_cents WHERE price_month_cents = 0 AND price_cents <> 0`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'products' AND indexname = 'idx_products_slug_lower') THEN
				CREATE UNIQUE INDEX idx_products_slug_lower ON products (LOWER(slug)) WHERE slug <> '';
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'products' AND indexname = 'idx_products_group_id') THEN
				CREATE INDEX idx_products_group_id ON products (group_id);
			END IF;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'products' AND indexname = 'idx_products_line_group_id') THEN
				CREATE INDEX idx_products_line_group_id ON products (line_group_id);
			END IF;
		END $$`,
		`CREATE TABLE IF NOT EXISTS orders (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			product_id TEXT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
			product_name TEXT NOT NULL DEFAULT '',
			amount_cents BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'CNY',
			status TEXT NOT NULL DEFAULT 'pending',
			period TEXT NOT NULL DEFAULT 'month',
			quantity INT NOT NULL DEFAULT 1,
			starts_at TIMESTAMPTZ,
			ends_at TIMESTAMPTZ,
			paid_at TIMESTAMPTZ,
			note TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS period TEXT NOT NULL DEFAULT 'month'`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS quantity INT NOT NULL DEFAULT 1`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS starts_at TIMESTAMPTZ`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS ends_at TIMESTAMPTZ`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS note TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_product_id ON orders(product_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status)`,
	)

	// Clusters table
	migrations = append(migrations,
		`CREATE TABLE IF NOT EXISTS clusters (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			dns_zone TEXT NOT NULL DEFAULT '',
			dns_mode TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'clusters' AND indexname = 'idx_clusters_name_lower') THEN
				CREATE UNIQUE INDEX idx_clusters_name_lower ON clusters (LOWER(name));
			END IF;
		END $$`,
		`ALTER TABLE clusters ADD COLUMN IF NOT EXISTS cname TEXT NOT NULL DEFAULT ''`,
		// Cluster nodes table (replaces line_groups + line_group_nodes)
		`CREATE TABLE IF NOT EXISTS cluster_nodes (
			cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
			line TEXT NOT NULL DEFAULT '默认',
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			enabled BOOLEAN NOT NULL DEFAULT true,
			weight INT NOT NULL DEFAULT 1,
			backup BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (cluster_id, line, node_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cluster_nodes_cluster ON cluster_nodes(cluster_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cluster_nodes_node ON cluster_nodes(node_id)`,
		// Drop legacy line group tables
		`DROP TABLE IF EXISTS line_group_nodes`,
		`DROP TABLE IF EXISTS line_groups`,
	)

	// Node logs are now shipped node→Filebeat→ES; the local PG table is
	// retired. DROP it during migration on existing installs.
	migrations = append(migrations,
		`DROP TABLE IF EXISTS node_logs`,
		// User traffic tracking
		`CREATE TABLE IF NOT EXISTS user_traffic (
			user_id TEXT NOT NULL,
			month TEXT NOT NULL,
			bytes_total BIGINT NOT NULL DEFAULT 0,
			last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, month)
		)`,
	)

	// Certificate table v2: migrate from TEXT id to BIGSERIAL auto-increment,
	// add type/auto_renew/status/fail_reason columns. This is a destructive
	// migration — existing certificate data is dropped because the id type
	// change (TEXT → BIGINT) is incompatible with in-place ALTER on PG, and
	// domains.cert_id foreign key must match. Acceptable in dev/early-prod
	// because ACME certs can be re-issued and uploads re-submitted.
	migrations = append(migrations,
		// 1) Drop the domains→certificates FK so we can drop+recreate the table.
		`ALTER TABLE domains DROP CONSTRAINT IF EXISTS domains_cert_id_fkey`,
		// 2) Clear stale cert_id references in domains.
		`UPDATE domains SET cert_id = NULL WHERE cert_id IS NOT NULL`,
		// 3) Change domains.cert_id column type to BIGINT (from TEXT).
		//    Safe because we just NULLed all values above.
		`ALTER TABLE domains ALTER COLUMN cert_id TYPE BIGINT USING NULL`,
		// 4) Drop old certificates table entirely.
		`DROP TABLE IF EXISTS certificates CASCADE`,
		// 5) Recreate with BIGSERIAL id + new columns.
		`CREATE TABLE IF NOT EXISTS certificates (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			domain TEXT NOT NULL,
			user_id TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT 'upload',
			auto_renew BOOLEAN NOT NULL DEFAULT false,
			status TEXT NOT NULL DEFAULT 'active',
			fail_reason TEXT NOT NULL DEFAULT '',
			cert_pem BYTEA,
			key_pem BYTEA,
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (user_id, domain)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_certificates_domain ON certificates(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_certificates_user_id ON certificates(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_certificates_status ON certificates(status)`,
		// 6) Re-add FK from domains.cert_id → certificates.id with ON DELETE SET NULL
		//    so deleting a cert doesn't require manually unbinding every domain first.
		`ALTER TABLE domains ADD CONSTRAINT domains_cert_id_fkey FOREIGN KEY (cert_id) REFERENCES certificates(id) ON DELETE SET NULL`,
	)

	for _, m := range migrations {
		if _, err := p.pool.Exec(ctx, m); err != nil {
			// Use IF NOT EXISTS / ADD COLUMN IF NOT EXISTS for idempotency,
			// so partial failures are safe to re-run.
			log.Warn().Err(err).Str("migration", truncate(m, 50)).Msg("migration statement failed (may be safe to ignore)")
		}
	}

	defaults := DefaultSettings()
	if _, err := p.pool.Exec(ctx, `
		INSERT INTO settings (
			id, system_name, footer_links, footer_copyright, favicon, logo,
			smtp_host, smtp_port, smtp_username, smtp_password, smtp_from, smtp_from_name,
			elasticsearch_url, elasticsearch_user, elasticsearch_pass, elasticsearch_index,
			elasticsearch_ts_field, elasticsearch_domain_field, elasticsearch_bytes_field,
			sales_email, support_email, register_enabled, upgrade_channel, notify_new_build,
			register_email_verification, email_enabled, dingtalk_enabled, dingtalk_webhook,
			wechat_enabled, wechat_webhook, feishu_enabled, feishu_webhook,
			notify_node_resource, notify_node_monitor,
			notify_ticket_reply, notify_interval, threshold_cpu, threshold_memory, threshold_disk,
			threshold_bandwidth_up, threshold_bandwidth_down, updated_at
		)
		VALUES (
			$1,$2,$3,$4,$5,$6,
			$7,$8,$9,$10,$11,$12,
			$13,$14,$15,$16,
			$17,$18,$19,
			$20,$21,$22,$23,$24,
			$25,$26,$27,$28,
			$29,$30,$31,$32,
			$33,$34,
			$35,$36,$37,$38,
			$39,$40,$41,$42
		)
		ON CONFLICT (id) DO NOTHING
	`,
		defaults.ID,
		defaults.SystemName,
		defaults.FooterLinks,
		defaults.FooterCopyright,
		defaults.Favicon,
		defaults.Logo,
		defaults.SMTPHost,
		defaults.SMTPPort,
		defaults.SMTPUsername,
		defaults.SMTPPassword,
		defaults.SMTPFrom,
		defaults.SMTPFromName,
		defaults.ElasticsearchURL,
		defaults.ElasticsearchUser,
		defaults.ElasticsearchPass,
		defaults.ElasticsearchIndex,
		defaults.ElasticsearchTSField,
		defaults.ElasticsearchDomainField,
		defaults.ElasticsearchBytesField,
		defaults.SalesEmail,
		defaults.SupportEmail,
		defaults.RegisterEnabled,
		defaults.UpgradeChannel,
		defaults.NotifyNewBuild,
		defaults.RegisterEmailVerification,
		defaults.EmailEnabled,
		defaults.DingtalkEnabled,
		defaults.DingtalkWebhook,
		defaults.WechatEnabled,
		defaults.WechatWebhook,
		defaults.FeishuEnabled,
		defaults.FeishuWebhook,
		defaults.NotifyNodeResource,
		defaults.NotifyNodeMonitor,
		defaults.NotifyTicketReply,
		defaults.NotifyInterval,
		defaults.ThresholdCPU,
		defaults.ThresholdMemory,
		defaults.ThresholdDisk,
		defaults.ThresholdBandwidthUp,
		defaults.ThresholdBandwidthDown,
		defaults.UpdatedAt,
	); err != nil {
		return err
	}

	log.Info().Msg("migrations completed successfully")
	return nil
}

// Seed inserts initial data.
func (p *Postgres) Seed(ctx context.Context) error {
	log.Info().Msg("seeding database")

	// Ensure usernames backfilled if empty
	_, _ = p.pool.Exec(ctx, `UPDATE users SET username = email WHERE (username = '' OR username IS NULL)`)

	// Generate bootstrap token if not exists
	var count int
	err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tokens WHERE token_type = 'bootstrap'`).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		token := generateToken()
		tokenHash := hashToken(token)
		_, err = p.pool.Exec(ctx,
			`INSERT INTO tokens (id, token_hash, token_type, description) VALUES ($1, $2, 'bootstrap', 'Node bootstrap token')`,
			generateID(), tokenHash)
		if err != nil {
			return err
		}
		logSeededToken("bootstrap", token)
	}

	// Generate service token if not exists
	err = p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tokens WHERE token_type = 'service'`).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		token := generateToken()
		tokenHash := hashToken(token)
		_, err = p.pool.Exec(ctx,
			`INSERT INTO tokens (id, token_hash, token_type, description) VALUES ($1, $2, 'service', 'Service-to-service token')`,
			generateID(), tokenHash)
		if err != nil {
			return err
		}
		logSeededToken("service", token)
	}

	log.Info().Msg("seeding completed")
	return nil
}

// CreateUser inserts a new user.
func (p *Postgres) CreateUser(ctx context.Context, user *User) error {
	if user.Status == "" {
		user.Status = "active"
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, role, status, created_at, updated_at, last_login_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		user.ID, strings.ToLower(user.Username), strings.ToLower(user.Email), user.PasswordHash, user.Role, user.Status, user.CreatedAt, user.UpdatedAt, nullTimePtr(user.LastLoginAt))
	return err
}

// GetUserByID returns a user by id.
func (p *Postgres) GetUserByID(ctx context.Context, id string) (*User, error) {
	var u User
	var lastLogin sql.NullTime
	err := p.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, role, status, created_at, updated_at, last_login_at, last_login_ip, last_login_location, numeric_id FROM users WHERE id = $1`,
		id).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt, &lastLogin, &u.LastLoginIP, &u.LastLoginLocation, &u.NumericID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return &u, nil
}

// GetUserByEmail returns a user by email.
func (p *Postgres) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	var lastLogin sql.NullTime
	err := p.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, role, status, created_at, updated_at, last_login_at, last_login_ip, last_login_location, numeric_id FROM users WHERE LOWER(email) = LOWER($1)`,
		email).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt, &lastLogin, &u.LastLoginIP, &u.LastLoginLocation, &u.NumericID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return &u, nil
}

// GetUserByUsername returns a user by username.
func (p *Postgres) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	var lastLogin sql.NullTime
	err := p.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, role, status, created_at, updated_at, last_login_at, last_login_ip, last_login_location, numeric_id FROM users WHERE LOWER(username) = LOWER($1)`,
		username).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt, &lastLogin, &u.LastLoginIP, &u.LastLoginLocation, &u.NumericID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return &u, nil
}

// GetUserByLogin returns a user by username or email.
func (p *Postgres) GetUserByLogin(ctx context.Context, login string) (*User, error) {
	if strings.Contains(login, "@") {
		return p.GetUserByEmail(ctx, login)
	}
	return p.GetUserByUsername(ctx, login)
}

// ListUsers returns users limited by "limit" (0 = all).
func (p *Postgres) ListUsers(ctx context.Context, limit int) ([]*User, error) {
	query := `SELECT id, username, email, password_hash, role, status, created_at, updated_at, last_login_at, last_login_ip, last_login_location, numeric_id FROM users ORDER BY created_at DESC`
	if limit > 0 {
		query += ` LIMIT $1`
	}
	rows, err := func() (pgx.Rows, error) {
		if limit > 0 {
			return p.pool.Query(ctx, query, limit)
		}
		return p.pool.Query(ctx, query)
	}()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		var u User
		var lastLogin sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt, &lastLogin, &u.LastLoginIP, &u.LastLoginLocation, &u.NumericID); err != nil {
			return nil, err
		}
		if lastLogin.Valid {
			u.LastLoginAt = &lastLogin.Time
		}
		users = append(users, &u)
	}
	return users, nil
}

func (p *Postgres) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// UpdateUserLastLogin updates a user's last login time, IP and location.
func (p *Postgres) UpdateUserLastLogin(ctx context.Context, id string, lastLoginAt time.Time, ip string, location string) error {
	_, err := p.pool.Exec(ctx, `UPDATE users SET last_login_at = $1, last_login_ip = $2, last_login_location = $3, updated_at = NOW() WHERE id = $4`, lastLoginAt, ip, location, id)
	return err
}

func (p *Postgres) UpdateUserStatus(ctx context.Context, id string, status string) error {
	_, err := p.pool.Exec(ctx, `UPDATE users SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	return err
}

func (p *Postgres) UpdateUserRole(ctx context.Context, id string, role string) error {
	_, err := p.pool.Exec(ctx, `UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2`, role, id)
	return err
}

func (p *Postgres) UpdateUserPasswordHash(ctx context.Context, id string, passwordHash string) error {
	_, err := p.pool.Exec(ctx, `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`, passwordHash, id)
	return err
}

// DeleteUser removes a user by ID.
func (p *Postgres) DeleteUser(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

// Close releases the underlying pool.
func (p *Postgres) Close() {
	p.pool.Close()
}

// Pool exposes the underlying pgx pool for callers that need it.
func (p *Postgres) Pool() *pgxpool.Pool {
	return p.pool
}

// WithTimeout derives a context with a short timeout for DB ops.
func WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 5*time.Second)
}

// --- Node operations ---

func (p *Postgres) CreateNode(ctx context.Context, node *Node) error {
	if strings.TrimSpace(node.MonitorProtocol) == "" {
		node.MonitorProtocol = "http"
	}
	if node.MonitorTimeout == 0 {
		node.MonitorTimeout = 5
	}
	if node.MonitorPort == 0 {
		node.MonitorPort = 80
	}
	if node.MonitorFailThreshold == 0 {
		node.MonitorFailThreshold = 3
	}
	if node.Capabilities == nil {
		node.Capabilities = []string{}
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO nodes (id, hostname, public_ip, version, status, region, cluster, capabilities, config_version, token, last_heartbeat,
		                  monitor_enabled, monitor_protocol, monitor_timeout_seconds, monitor_port, monitor_fail_threshold,
		                  monitor_fail_count, monitor_last_ok, monitor_last_error, monitor_last_at, monitor_last_latency_ms,
		                  created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
		         $12, $13, $14, $15, $16,
		         $17, $18, $19, $20, $21,
		         $22, $23)`,
		node.ID, node.Hostname, node.PublicIP, node.Version, node.Status, node.Region, node.Cluster, node.Capabilities,
		node.ConfigVersion, node.Token, nullTime(node.LastHeartbeat),
		node.MonitorEnabled, node.MonitorProtocol, node.MonitorTimeout, node.MonitorPort, node.MonitorFailThreshold,
		node.MonitorFailCount, node.MonitorLastOK, node.MonitorLastError, nullTime(node.MonitorLastAt), node.MonitorLastLatencyMs,
		node.CreatedAt, node.UpdatedAt)
	return err
}

func (p *Postgres) GetNode(ctx context.Context, id string) (*Node, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, hostname, public_ip, version, status, region, cluster, capabilities, config_version, token, last_heartbeat,
		        monitor_enabled, monitor_protocol, monitor_timeout_seconds, monitor_port, monitor_fail_threshold,
		        monitor_fail_count, monitor_last_ok, monitor_last_error, monitor_last_at, monitor_last_latency_ms,
		        cpu_usage, mem_usage, disk_usage, cpu_count, mem_total, disk_total, last_metrics_at,
		        bytes_sent, bytes_received, bandwidth_up_bps, bandwidth_down_bps,
		        tcp_established, tcp_syn_recv, tcp_time_wait, nginx_running, month_bytes_sent,
		        created_at, updated_at
		 FROM nodes WHERE id = $1`, id)
	return scanNode(row)
}

func (p *Postgres) GetNodeByHostname(ctx context.Context, hostname string) (*Node, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, hostname, public_ip, version, status, region, cluster, capabilities, config_version, token, last_heartbeat,
		        monitor_enabled, monitor_protocol, monitor_timeout_seconds, monitor_port, monitor_fail_threshold,
		        monitor_fail_count, monitor_last_ok, monitor_last_error, monitor_last_at, monitor_last_latency_ms,
		        cpu_usage, mem_usage, disk_usage, cpu_count, mem_total, disk_total, last_metrics_at,
		        bytes_sent, bytes_received, bandwidth_up_bps, bandwidth_down_bps,
		        tcp_established, tcp_syn_recv, tcp_time_wait, nginx_running, month_bytes_sent,
		        created_at, updated_at
		 FROM nodes WHERE hostname = $1`, hostname)
	return scanNode(row)
}

func (p *Postgres) ListNodes(ctx context.Context) ([]*Node, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, hostname, public_ip, version, status, region, cluster, capabilities, config_version, token, last_heartbeat,
		        monitor_enabled, monitor_protocol, monitor_timeout_seconds, monitor_port, monitor_fail_threshold,
		        monitor_fail_count, monitor_last_ok, monitor_last_error, monitor_last_at, monitor_last_latency_ms,
		        cpu_usage, mem_usage, disk_usage, cpu_count, mem_total, disk_total, last_metrics_at,
		        bytes_sent, bytes_received, bandwidth_up_bps, bandwidth_down_bps,
		        tcp_established, tcp_syn_recv, tcp_time_wait, nginx_running, month_bytes_sent,
		        created_at, updated_at
		 FROM nodes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		node, err := scanNodeRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (p *Postgres) CountNodes(ctx context.Context) (int, error) {
	var n int
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (p *Postgres) UpdateNodeStatus(ctx context.Context, id, status, configVersion string) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE nodes SET status = $2, config_version = $3, last_heartbeat = NOW(), updated_at = NOW() WHERE id = $1`,
		id, status, configVersion)
	return err
}

func (p *Postgres) UpdateNodeToken(ctx context.Context, id, tokenHash string) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE nodes SET token = $2, updated_at = NOW() WHERE id = $1`,
		id, tokenHash)
	return err
}

// UpdateNodeHeartbeatInfo updates only the fields that change on every heartbeat.
// Empty string values are ignored so heartbeats never clobber admin-managed columns.
func (p *Postgres) UpdateNodeHeartbeatInfo(ctx context.Context, id, publicIP, version, region string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("node id required")
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE nodes
		 SET public_ip = COALESCE(NULLIF($2, ''), public_ip),
		     version   = COALESCE(NULLIF($3, ''), version),
		     region    = COALESCE(NULLIF($4, ''), region),
		     last_heartbeat = NOW(),
		     updated_at = NOW()
		 WHERE id = $1`,
		id, publicIP, version, region)
	return err
}

// UpdateNode updates multiple fields for a node.
func (p *Postgres) UpdateNode(ctx context.Context, node *Node) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE nodes
		 SET hostname = COALESCE(NULLIF($2, ''), hostname),
		     public_ip = COALESCE(NULLIF($3, ''), public_ip),
		     version = COALESCE(NULLIF($4, ''), version),
		     status = COALESCE(NULLIF($5, ''), status),
		     region = COALESCE(NULLIF($6, ''), region),
		     cluster = COALESCE(NULLIF($7, ''), cluster),
		     capabilities = COALESCE($8::text[], capabilities),
		     config_version = COALESCE(NULLIF($9, ''), config_version),
		     token = CASE WHEN $10 = '' THEN token ELSE $10 END,
		     updated_at = NOW()
		 WHERE id = $1`,
		node.ID,
		node.Hostname,
		node.PublicIP,
		node.Version,
		node.Status,
		node.Region,
		node.Cluster,
		node.Capabilities,
		node.ConfigVersion,
		node.Token,
	)
	return err
}

// RegisterOrRefreshNode atomically inserts a new node or refreshes an existing
// row keyed by hostname. This replaces the historical sequence of
// GetNodeByHostname → UpdateNodeToken → UpdateNodeStatus → UpdateNode which
// had a TOCTOU window between the existence check and the update, and could
// leave the row in an inconsistent state if any middle write failed.
//
// On conflict (hostname already exists) we keep the existing id and refresh
// the volatile fields. If the existing row is disabled, the UPSERT's WHERE
// clause suppresses the update, RETURNING yields zero rows, and we surface
// ErrNodeDisabled. Callers that still need the old id-typed access path
// (e.g. session bookkeeping) should use the returned id.
func (p *Postgres) RegisterOrRefreshNode(ctx context.Context, node *Node) (string, error) {
	if node == nil {
		return "", errors.New("node is nil")
	}
	// Default monitor columns for first-time INSERT. Existing rows retain
	// their admin-configured values because we don't list these columns
	// in the DO UPDATE SET clause.
	if strings.TrimSpace(node.MonitorProtocol) == "" {
		node.MonitorProtocol = "http"
	}
	if node.MonitorTimeout == 0 {
		node.MonitorTimeout = 5
	}
	if node.MonitorPort == 0 {
		node.MonitorPort = 80
	}
	if node.MonitorFailThreshold == 0 {
		node.MonitorFailThreshold = 3
	}
	if node.CreatedAt.IsZero() {
		node.CreatedAt = time.Now()
	}
	if node.UpdatedAt.IsZero() {
		node.UpdatedAt = node.CreatedAt
	}
	if node.Capabilities == nil {
		node.Capabilities = []string{}
	}

	var outID string
	err := p.pool.QueryRow(ctx,
		`INSERT INTO nodes (id, hostname, public_ip, version, status, region, cluster, capabilities, config_version, token,
		                    monitor_enabled, monitor_protocol, monitor_timeout_seconds, monitor_port, monitor_fail_threshold,
		                    created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		         $11, $12, $13, $14, $15,
		         $16, $17)
		 ON CONFLICT (hostname) DO UPDATE SET
		     public_ip      = EXCLUDED.public_ip,
		     version        = EXCLUDED.version,
		     status         = 'online',
		     region         = EXCLUDED.region,
		     capabilities   = EXCLUDED.capabilities,
		     token          = EXCLUDED.token,
		     updated_at     = NOW()
		 WHERE nodes.status <> 'disabled'
		 RETURNING id`,
		node.ID, node.Hostname, node.PublicIP, node.Version, node.Status, node.Region, node.Cluster, node.Capabilities,
		node.ConfigVersion, node.Token,
		node.MonitorEnabled, node.MonitorProtocol, node.MonitorTimeout, node.MonitorPort, node.MonitorFailThreshold,
		node.CreatedAt, node.UpdatedAt,
	).Scan(&outID)
	if err != nil {
		// pgx returns ErrNoRows when ON CONFLICT ... WHERE filtered the update out.
		// Convert that to our sentinel so callers can distinguish a disabled node
		// from a storage error.
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNodeDisabled
		}
		return "", err
	}
	return outID, nil
}

func (p *Postgres) UpdateNodeMonitorConfig(ctx context.Context, id string, cfg NodeMonitorConfig) error {
	proto := strings.TrimSpace(cfg.Protocol)
	if proto == "" {
		proto = "http"
	}
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 5
	}
	port := cfg.Port
	if port <= 0 {
		port = 80
	}
	threshold := cfg.FailThreshold
	if threshold <= 0 {
		threshold = 3
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE nodes
		 SET monitor_enabled = $2,
		     monitor_protocol = $3,
		     monitor_timeout_seconds = $4,
		     monitor_port = $5,
		     monitor_fail_threshold = $6,
		     monitor_fail_count = 0,
		     monitor_last_ok = false,
		     monitor_last_error = '',
		     monitor_last_at = NULL,
		     monitor_last_latency_ms = 0,
		     updated_at = NOW()
		 WHERE id = $1`,
		id, cfg.Enabled, proto, timeout, port, threshold,
	)
	return err
}

func (p *Postgres) UpdateNodeMonitorResult(ctx context.Context, id string, res NodeMonitorResult) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE nodes
		 SET monitor_last_ok = $2,
		     monitor_last_error = $3,
		     monitor_last_at = $4,
		     monitor_last_latency_ms = $5,
		     monitor_fail_count = $6,
		     updated_at = NOW()
		 WHERE id = $1`,
		id, res.LastOK, res.LastError, nullTime(res.LastAt), res.LastLatencyMs, res.FailCount,
	)
	return err
}

func (p *Postgres) UpdateNodeTelemetry(ctx context.Context, id string, t NodeTelemetry) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE nodes
		 SET cpu_usage = $2,
		     mem_usage = $3,
		     disk_usage = $4,
		     cpu_count = $5,
		     mem_total = $6,
		     disk_total = $7,
		     tcp_established = $8,
		     tcp_syn_recv = $9,
		     tcp_time_wait = $10,
		     nginx_running = $11,
		     bandwidth_up_bps = CASE
		       WHEN last_metrics_at IS NULL THEN bandwidth_up_bps
		       WHEN EXTRACT(EPOCH FROM (NOW() - last_metrics_at)) <= 0 THEN bandwidth_up_bps
		       WHEN $12 < bytes_sent THEN $12::float8 / EXTRACT(EPOCH FROM (NOW() - last_metrics_at))
		       ELSE ($12 - bytes_sent)::float8 / EXTRACT(EPOCH FROM (NOW() - last_metrics_at))
		     END,
		     bandwidth_down_bps = CASE
		       WHEN last_metrics_at IS NULL THEN bandwidth_down_bps
		       WHEN EXTRACT(EPOCH FROM (NOW() - last_metrics_at)) <= 0 THEN bandwidth_down_bps
		       WHEN $13 < bytes_received THEN $13::float8 / EXTRACT(EPOCH FROM (NOW() - last_metrics_at))
		       ELSE ($13 - bytes_received)::float8 / EXTRACT(EPOCH FROM (NOW() - last_metrics_at))
		     END,
		     month_bytes_sent = CASE
		       WHEN last_metrics_at IS NULL THEN GREATEST($12 - bytes_sent, 0)
		       WHEN date_trunc('month', last_metrics_at) = date_trunc('month', NOW()) THEN month_bytes_sent + GREATEST($12 - bytes_sent, 0)
		       ELSE GREATEST($12 - bytes_sent, 0)
		     END,
		     bytes_sent = $12,
		     bytes_received = $13,
		     last_metrics_at = NOW(),
		     updated_at = NOW()
		 WHERE id = $1`,
		id,
		t.CPUUsage,
		t.MemUsage,
		t.DiskUsage,
		t.CPUCount,
		t.MemTotal,
		t.DiskTotal,
		t.TCPEstablished,
		t.TCPSynRecv,
		t.TCPTimeWait,
		t.NginxRunning,
		t.BytesSent,
		t.BytesReceived,
	)
	return err
}

func (p *Postgres) DeleteNode(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, id)
	return err
}

// --- Cluster node operations ---

func (p *Postgres) ListClusterNodes(ctx context.Context, clusterID, line string) ([]*ClusterNode, error) {
	line = strings.TrimSpace(line)
	if line == "default" {
		line = "默认"
	}
	var rows pgx.Rows
	var err error
	if line == "" || line == "all" {
		rows, err = p.pool.Query(ctx,
			`SELECT cluster_id, line, node_id, enabled, weight, backup, created_at, updated_at
			 FROM cluster_nodes WHERE cluster_id = $1 ORDER BY created_at ASC`, clusterID)
	} else {
		rows, err = p.pool.Query(ctx,
			`SELECT cluster_id, line, node_id, enabled, weight, backup, created_at, updated_at
			 FROM cluster_nodes WHERE cluster_id = $1 AND line = $2 ORDER BY created_at ASC`, clusterID, line)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*ClusterNode
	for rows.Next() {
		var n ClusterNode
		if err := rows.Scan(&n.ClusterID, &n.Line, &n.NodeID, &n.Enabled, &n.Weight, &n.Backup, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &n)
	}
	return list, rows.Err()
}

func (p *Postgres) UpsertClusterNode(ctx context.Context, n *ClusterNode) error {
	if n == nil {
		return nil
	}
	now := time.Now()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	n.UpdatedAt = now
	if n.Weight <= 0 {
		n.Weight = 1
	}
	n.Line = strings.TrimSpace(n.Line)
	if n.Line == "" || n.Line == "default" {
		n.Line = "默认"
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO cluster_nodes (cluster_id, line, node_id, enabled, weight, backup, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (cluster_id, line, node_id) DO UPDATE SET enabled=EXCLUDED.enabled, weight=EXCLUDED.weight, backup=EXCLUDED.backup, updated_at=EXCLUDED.updated_at`,
		n.ClusterID, n.Line, n.NodeID, n.Enabled, n.Weight, n.Backup, n.CreatedAt, n.UpdatedAt)
	return err
}

func (p *Postgres) DeleteClusterNode(ctx context.Context, clusterID, line, nodeID string) error {
	line = strings.TrimSpace(line)
	if line == "" || line == "default" {
		line = "默认"
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM cluster_nodes WHERE cluster_id=$1 AND line=$2 AND node_id=$3`, clusterID, line, nodeID)
	return err
}

// --- Domain operations ---

func (p *Postgres) CreateDomain(ctx context.Context, domain *Domain) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO domains (id, name, cname, user_id, line_group_id, origin_id, cert_id, origin_scheme, origin_port, origin_host_mode, origin_host, origin_timeout_ms, origin_connect_timeout_ms, error_pages, cache_enabled, http2_enabled, websocket_enabled, https_enabled, enabled, security_json, origin_auth_json, load_balance_method, origin_health_check_json, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)`,
		domain.ID, domain.Name, domain.CNAME, domain.UserID, nullString(domain.LineGroupID), nullString(domain.OriginID), nullString(domain.CertID),
		defaultOriginScheme(domain.OriginScheme), defaultOriginPort(domain.OriginPort), defaultOriginHostMode(domain.OriginHostMode),
		domain.OriginHost, defaultOriginTimeout(domain.OriginTimeoutMs), defaultOriginConnectTimeout(domain.OriginConnectTimeoutMs),
		marshalErrorPages(domain.ErrorPages), domain.CacheEnabled, domain.HTTP2Enabled, domain.WebsocketEnabled, domain.HTTPSEnabled, domain.Enabled,
		marshalDomainSecurity(domain.Security), marshalOriginAuth(domain.OriginAuth),
		defaultLoadBalanceMethod(domain.LoadBalanceMethod), marshalOriginHealthCheck(domain.OriginHealthCheck),
		domain.CreatedAt, domain.UpdatedAt)
	return err
}

func (p *Postgres) GetDomain(ctx context.Context, id string) (*Domain, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, name, cname, user_id, line_group_id, origin_id, cert_id, origin_scheme, origin_port, origin_host_mode, origin_host, origin_timeout_ms, origin_connect_timeout_ms, error_pages, cache_enabled, http2_enabled, websocket_enabled, https_enabled, enabled, security_json, origin_auth_json, load_balance_method, origin_health_check_json, created_at, updated_at
		 FROM domains WHERE id = $1`, id)
	return scanDomain(row)
}

func (p *Postgres) GetDomainByName(ctx context.Context, name string) (*Domain, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, name, cname, user_id, line_group_id, origin_id, cert_id, origin_scheme, origin_port, origin_host_mode, origin_host, origin_timeout_ms, origin_connect_timeout_ms, error_pages, cache_enabled, http2_enabled, websocket_enabled, https_enabled, enabled, security_json, origin_auth_json, load_balance_method, origin_health_check_json, created_at, updated_at
		 FROM domains WHERE name = $1`, name)
	return scanDomain(row)
}

func (p *Postgres) ListDomains(ctx context.Context) ([]*Domain, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, cname, user_id, line_group_id, origin_id, cert_id, origin_scheme, origin_port, origin_host_mode, origin_host, origin_timeout_ms, origin_connect_timeout_ms, error_pages, cache_enabled, http2_enabled, websocket_enabled, https_enabled, enabled, security_json, origin_auth_json, load_balance_method, origin_health_check_json, created_at, updated_at
		 FROM domains ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []*Domain
	for rows.Next() {
		domain, err := scanDomainRows(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, domain)
	}
	return domains, rows.Err()
}

func (p *Postgres) CountDomains(ctx context.Context) (int, error) {
	var n int
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM domains`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (p *Postgres) ListDomainsByUser(ctx context.Context, userID string) ([]*Domain, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, cname, user_id, line_group_id, origin_id, cert_id, origin_scheme, origin_port, origin_host_mode, origin_host, origin_timeout_ms, origin_connect_timeout_ms, error_pages, cache_enabled, http2_enabled, websocket_enabled, https_enabled, enabled, security_json, origin_auth_json, load_balance_method, origin_health_check_json, created_at, updated_at
		 FROM domains WHERE user_id = $1 ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []*Domain
	for rows.Next() {
		domain, err := scanDomainRows(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, domain)
	}
	return domains, rows.Err()
}

func (p *Postgres) CountDomainsByUser(ctx context.Context, userID string) (int, error) {
	var n int
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM domains WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (p *Postgres) UpdateDomain(ctx context.Context, domain *Domain) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE domains SET name = $2, cname = $3, user_id = $4, line_group_id = $5, origin_id = $6, cert_id = $7,
		    origin_scheme = $8, origin_port = $9, origin_host_mode = $10, origin_host = $11,
		    origin_timeout_ms = $12, origin_connect_timeout_ms = $13, error_pages = $14, cache_enabled = $15, http2_enabled = $16, websocket_enabled = $17, https_enabled = $18, enabled = $19, security_json = $20, origin_auth_json = $21,
		    load_balance_method = $22, origin_health_check_json = $23, updated_at = NOW()
		 WHERE id = $1`,
		domain.ID, domain.Name, domain.CNAME, domain.UserID, nullString(domain.LineGroupID), nullString(domain.OriginID), nullString(domain.CertID),
		defaultOriginScheme(domain.OriginScheme), defaultOriginPort(domain.OriginPort), defaultOriginHostMode(domain.OriginHostMode),
		domain.OriginHost, defaultOriginTimeout(domain.OriginTimeoutMs), defaultOriginConnectTimeout(domain.OriginConnectTimeoutMs),
		marshalErrorPages(domain.ErrorPages), domain.CacheEnabled, domain.HTTP2Enabled, domain.WebsocketEnabled, domain.HTTPSEnabled, domain.Enabled,
		marshalDomainSecurity(domain.Security), marshalOriginAuth(domain.OriginAuth),
		defaultLoadBalanceMethod(domain.LoadBalanceMethod), marshalOriginHealthCheck(domain.OriginHealthCheck))
	return err
}

func (p *Postgres) DeleteDomain(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM domains WHERE id = $1`, id)
	return err
}

// --- Origin operations ---

func (p *Postgres) CreateOrigin(ctx context.Context, origin *Origin) error {
	if origin.Addresses == nil {
		origin.Addresses = []string{}
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO origins (id, name, addresses, timeout_ms, max_retries, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		origin.ID, origin.Name, origin.Addresses, origin.TimeoutMs, origin.MaxRetries,
		origin.CreatedAt, origin.UpdatedAt)
	return err
}

func (p *Postgres) GetOrigin(ctx context.Context, id string) (*Origin, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, name, addresses, timeout_ms, max_retries, created_at, updated_at
		 FROM origins WHERE id = $1`, id)
	return scanOrigin(row)
}

func (p *Postgres) ListOrigins(ctx context.Context) ([]*Origin, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, addresses, timeout_ms, max_retries, created_at, updated_at
		 FROM origins ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var origins []*Origin
	for rows.Next() {
		origin, err := scanOriginRows(rows)
		if err != nil {
			return nil, err
		}
		origins = append(origins, origin)
	}
	return origins, rows.Err()
}

func (p *Postgres) UpdateOrigin(ctx context.Context, origin *Origin) error {
	if origin.Addresses == nil {
		origin.Addresses = []string{}
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE origins SET name = $2, addresses = $3, timeout_ms = $4, max_retries = $5, updated_at = NOW()
		 WHERE id = $1`,
		origin.ID, origin.Name, origin.Addresses, origin.TimeoutMs, origin.MaxRetries)
	return err
}

func (p *Postgres) DeleteOrigin(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM origins WHERE id = $1`, id)
	return err
}

// --- DomainOrigin operations ---

func (p *Postgres) ListDomainOrigins(ctx context.Context, domainID string) ([]*DomainOrigin, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, domain_id, address, weight, enabled, sort_order, created_at, updated_at
		 FROM domain_origins WHERE domain_id = $1 ORDER BY sort_order, id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DomainOrigin
	for rows.Next() {
		o := &DomainOrigin{}
		if err := rows.Scan(&o.ID, &o.DomainID, &o.Address, &o.Weight, &o.Enabled, &o.SortOrder, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (p *Postgres) ListAllDomainOrigins(ctx context.Context) ([]*DomainOrigin, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, domain_id, address, weight, enabled, sort_order, created_at, updated_at
		 FROM domain_origins ORDER BY domain_id, sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DomainOrigin
	for rows.Next() {
		o := &DomainOrigin{}
		if err := rows.Scan(&o.ID, &o.DomainID, &o.Address, &o.Weight, &o.Enabled, &o.SortOrder, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ReplaceDomainOrigins atomically swaps all origin rows for a domain.
// A full-replace API is used rather than per-row PUT/POST because the
// UI presents the origin list as a single editable set — the user adds,
// edits, reorders, deletes and presses "save" once. Transactional
// delete+insert keeps the set consistent (no partial state where a node
// pulls config mid-edit and sees a half-updated list).
func (p *Postgres) ReplaceDomainOrigins(ctx context.Context, domainID string, entries []*DomainOrigin) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM domain_origins WHERE domain_id = $1`, domainID); err != nil {
		return err
	}
	for i, e := range entries {
		if e.ID == "" {
			e.ID = uuid.NewString()
		}
		weight := e.Weight
		if weight <= 0 {
			weight = 1
		}
		if weight > 100 {
			weight = 100
		}
		sortOrder := e.SortOrder
		if sortOrder == 0 {
			sortOrder = int32(i)
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO domain_origins (id, domain_id, address, weight, enabled, sort_order, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
			e.ID, domainID, e.Address, weight, e.Enabled, sortOrder,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) DeleteDomainOrigins(ctx context.Context, domainID string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM domain_origins WHERE domain_id = $1`, domainID)
	return err
}

// --- Certificate operations ---

const certColumns = `id, name, domain, user_id, type, auto_renew, status, fail_reason, cert_pem, key_pem, expires_at, created_at, updated_at`

func (p *Postgres) CreateCertificate(ctx context.Context, cert *Certificate) error {
	return p.pool.QueryRow(ctx,
		`INSERT INTO certificates (name, domain, user_id, type, auto_renew, status, fail_reason, cert_pem, key_pem, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id`,
		cert.Name, cert.Domain, cert.UserID, cert.Type, cert.AutoRenew,
		cert.Status, cert.FailReason, cert.CertPEM, cert.KeyPEM,
		cert.ExpiresAt, cert.CreatedAt, cert.UpdatedAt,
	).Scan(&cert.ID)
}

func (p *Postgres) GetCertificate(ctx context.Context, id int64) (*Certificate, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT `+certColumns+` FROM certificates WHERE id = $1`, id)
	return scanCertificate(row)
}

func (p *Postgres) GetCertificateByDomain(ctx context.Context, domain string) (*Certificate, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT `+certColumns+` FROM certificates WHERE domain = $1 ORDER BY expires_at DESC LIMIT 1`, domain)
	return scanCertificate(row)
}

func (p *Postgres) ListCertificates(ctx context.Context) ([]*Certificate, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT `+certColumns+` FROM certificates ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []*Certificate
	for rows.Next() {
		cert, err := scanCertificateRows(rows)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	return certs, rows.Err()
}

func (p *Postgres) ListCertificatesByUser(ctx context.Context, userID string) ([]*Certificate, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT `+certColumns+` FROM certificates WHERE user_id = $1 ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []*Certificate
	for rows.Next() {
		cert, err := scanCertificateRows(rows)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	return certs, rows.Err()
}

func (p *Postgres) UpdateCertificate(ctx context.Context, cert *Certificate) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE certificates SET name=$2, domain=$3, user_id=$4, type=$5, auto_renew=$6,
		 status=$7, fail_reason=$8, cert_pem=$9, key_pem=$10, expires_at=$11, updated_at=NOW()
		 WHERE id = $1`,
		cert.ID, cert.Name, cert.Domain, cert.UserID, cert.Type, cert.AutoRenew,
		cert.Status, cert.FailReason, cert.CertPEM, cert.KeyPEM, cert.ExpiresAt)
	return err
}

func (p *Postgres) DeleteCertificate(ctx context.Context, id int64) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM certificates WHERE id = $1`, id)
	return err
}

// --- Config version operations ---

func (p *Postgres) CreateConfigVersion(ctx context.Context, cv *ConfigVersion) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO config_versions (version, checksum, payload, created_at, created_by)
		 VALUES ($1, $2, $3, $4, $5)`,
		cv.Version, cv.Checksum, cv.Payload, cv.CreatedAt, cv.CreatedBy)
	return err
}

func (p *Postgres) GetConfigVersion(ctx context.Context, version string) (*ConfigVersion, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT version, checksum, payload, created_at, created_by
		 FROM config_versions WHERE version = $1`, version)
	return scanConfigVersion(row)
}

func (p *Postgres) GetLatestConfigVersion(ctx context.Context) (*ConfigVersion, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT version, checksum, payload, created_at, created_by
		 FROM config_versions ORDER BY created_at DESC LIMIT 1`)
	return scanConfigVersion(row)
}

func (p *Postgres) ListConfigVersions(ctx context.Context, limit int) ([]*ConfigVersion, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT version, checksum, payload, created_at, created_by
		 FROM config_versions ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*ConfigVersion
	for rows.Next() {
		cv, err := scanConfigVersionRows(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, cv)
	}
	return versions, rows.Err()
}

// --- Cache rule operations ---

func (p *Postgres) CreateCacheRule(ctx context.Context, rule *CacheRule) error {
	if rule.Methods == nil {
		rule.Methods = []string{}
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO cache_rules (id, name, host_pattern, path_pattern, methods, ttl_seconds, cache_query_params, priority, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		rule.ID, rule.Name, rule.HostPattern, rule.PathPattern, rule.Methods,
		rule.TTLSeconds, rule.CacheQueryParams, rule.Priority, rule.Enabled,
		rule.CreatedAt, rule.UpdatedAt)
	return err
}

func (p *Postgres) GetCacheRule(ctx context.Context, id string) (*CacheRule, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, name, host_pattern, path_pattern, methods, ttl_seconds, cache_query_params, priority, enabled, created_at, updated_at
		 FROM cache_rules WHERE id = $1`, id)
	return scanCacheRule(row)
}

func (p *Postgres) ListCacheRules(ctx context.Context) ([]*CacheRule, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, host_pattern, path_pattern, methods, ttl_seconds, cache_query_params, priority, enabled, created_at, updated_at
		 FROM cache_rules ORDER BY priority DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*CacheRule
	for rows.Next() {
		rule, err := scanCacheRuleRows(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (p *Postgres) UpdateCacheRule(ctx context.Context, rule *CacheRule) error {
	if rule.Methods == nil {
		rule.Methods = []string{}
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE cache_rules SET name = $2, host_pattern = $3, path_pattern = $4, methods = $5, ttl_seconds = $6, cache_query_params = $7, priority = $8, enabled = $9, updated_at = NOW()
		 WHERE id = $1`,
		rule.ID, rule.Name, rule.HostPattern, rule.PathPattern, rule.Methods,
		rule.TTLSeconds, rule.CacheQueryParams, rule.Priority, rule.Enabled)
	return err
}

func (p *Postgres) DeleteCacheRule(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM cache_rules WHERE id = $1`, id)
	return err
}

// --- Token operations ---

func (p *Postgres) ValidateServiceToken(ctx context.Context, token string) (bool, error) {
	tokenHash := hashToken(token)
	var count int
	err := p.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tokens WHERE token_hash = $1 AND token_type = 'service' AND (expires_at IS NULL OR expires_at > NOW())`,
		tokenHash).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (p *Postgres) ValidateBootstrapToken(ctx context.Context, token string) (bool, error) {
	tokenHash := hashToken(token)
	var count int
	err := p.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tokens WHERE token_hash = $1 AND token_type = 'bootstrap' AND (expires_at IS NULL OR expires_at > NOW())`,
		tokenHash).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (p *Postgres) CreateBootstrapToken(ctx context.Context, description string, ttl time.Duration) (string, time.Time, error) {
	token := generateToken()
	exp := time.Time{}
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO tokens (id, token_hash, token_type, description, expires_at) VALUES ($1,$2,'bootstrap',$3,$4)`,
		generateID(), hashToken(token), description, nullTime(exp))
	if err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

// License state
func (p *Postgres) SetLicenseState(ctx context.Context, st *LicenseState) error {
	if st == nil {
		return nil
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `
		INSERT INTO license_state (id, payload, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload, updated_at = EXCLUDED.updated_at
	`, b)
	return err
}

func (p *Postgres) GetLicenseState(ctx context.Context) (*LicenseState, error) {
	row := p.pool.QueryRow(ctx, `SELECT payload FROM license_state WHERE id=1 LIMIT 1`)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var st LicenseState
	if err := json.Unmarshal(payload, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// Settings
func (p *Postgres) GetSettings(ctx context.Context) (*Settings, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, system_name, footer_links, footer_copyright, favicon, logo, smtp_host, smtp_port, smtp_username, smtp_password, smtp_from, smtp_from_name, elasticsearch_url, elasticsearch_user, elasticsearch_pass, elasticsearch_index, elasticsearch_ts_field, elasticsearch_domain_field, elasticsearch_bytes_field, sales_email, support_email, register_enabled, upgrade_channel, notify_new_build, register_email_verification, email_enabled, dingtalk_enabled, dingtalk_webhook, wechat_enabled, wechat_webhook, feishu_enabled, feishu_webhook, notify_node_resource, notify_node_monitor, notify_ticket_reply, notify_interval, threshold_cpu, threshold_memory, threshold_disk, threshold_bandwidth_up, threshold_bandwidth_down, retention_system_logs, retention_es_logs, retention_waf_bans, retention_upgrade_logs, updated_at FROM settings WHERE id='default'`)
	var s Settings
	if err := row.Scan(
		&s.ID,
		&s.SystemName,
		&s.FooterLinks,
		&s.FooterCopyright,
		&s.Favicon,
		&s.Logo,
		&s.SMTPHost,
		&s.SMTPPort,
		&s.SMTPUsername,
		&s.SMTPPassword,
		&s.SMTPFrom,
		&s.SMTPFromName,
		&s.ElasticsearchURL,
		&s.ElasticsearchUser,
		&s.ElasticsearchPass,
		&s.ElasticsearchIndex,
		&s.ElasticsearchTSField,
		&s.ElasticsearchDomainField,
		&s.ElasticsearchBytesField,
		&s.SalesEmail,
		&s.SupportEmail,
		&s.RegisterEnabled,
		&s.UpgradeChannel,
		&s.NotifyNewBuild,
		&s.RegisterEmailVerification,
		&s.EmailEnabled,
		&s.DingtalkEnabled,
		&s.DingtalkWebhook,
		&s.WechatEnabled,
		&s.WechatWebhook,
		&s.FeishuEnabled,
		&s.FeishuWebhook,
		&s.NotifyNodeResource,
		&s.NotifyNodeMonitor,
		&s.NotifyTicketReply,
		&s.NotifyInterval,
		&s.ThresholdCPU,
		&s.ThresholdMemory,
		&s.ThresholdDisk,
		&s.ThresholdBandwidthUp,
		&s.ThresholdBandwidthDown,
		&s.RetentionSystemLogs,
		&s.RetentionESLogs,
		&s.RetentionWafBans,
		&s.RetentionUpgradeLogs,
		&s.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DefaultSettings(), nil
		}
		return nil, err
	}
	return &s, nil
}

func (p *Postgres) UpdateSettings(ctx context.Context, s *Settings) error {
	if s == nil {
		return nil
	}
	if s.ID == "" {
		s.ID = "default"
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = time.Now()
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO settings (id, system_name, footer_links, footer_copyright, favicon, logo, smtp_host, smtp_port, smtp_username, smtp_password, smtp_from, smtp_from_name, elasticsearch_url, elasticsearch_user, elasticsearch_pass, elasticsearch_index, elasticsearch_ts_field, elasticsearch_domain_field, elasticsearch_bytes_field, sales_email, support_email, register_enabled, upgrade_channel, notify_new_build, register_email_verification, email_enabled, dingtalk_enabled, dingtalk_webhook, wechat_enabled, wechat_webhook, feishu_enabled, feishu_webhook, notify_node_resource, notify_node_monitor, notify_ticket_reply, notify_interval, threshold_cpu, threshold_memory, threshold_disk, threshold_bandwidth_up, threshold_bandwidth_down, retention_system_logs, retention_es_logs, retention_waf_bans, retention_upgrade_logs, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44,$45,$46)
		 ON CONFLICT (id) DO UPDATE SET
		   system_name = EXCLUDED.system_name,
		   footer_links = EXCLUDED.footer_links,
		   footer_copyright = EXCLUDED.footer_copyright,
		   favicon = EXCLUDED.favicon,
		   logo = EXCLUDED.logo,
		   smtp_host = EXCLUDED.smtp_host,
		   smtp_port = EXCLUDED.smtp_port,
		   smtp_username = EXCLUDED.smtp_username,
		   smtp_password = EXCLUDED.smtp_password,
		   smtp_from = EXCLUDED.smtp_from,
		   smtp_from_name = EXCLUDED.smtp_from_name,
		   elasticsearch_url = EXCLUDED.elasticsearch_url,
		   elasticsearch_user = EXCLUDED.elasticsearch_user,
		   elasticsearch_pass = EXCLUDED.elasticsearch_pass,
		   elasticsearch_index = EXCLUDED.elasticsearch_index,
		   elasticsearch_ts_field = EXCLUDED.elasticsearch_ts_field,
		   elasticsearch_domain_field = EXCLUDED.elasticsearch_domain_field,
		   elasticsearch_bytes_field = EXCLUDED.elasticsearch_bytes_field,
		   sales_email = EXCLUDED.sales_email,
		   support_email = EXCLUDED.support_email,
		   register_enabled = EXCLUDED.register_enabled,
		   upgrade_channel = EXCLUDED.upgrade_channel,
		   notify_new_build = EXCLUDED.notify_new_build,
		   register_email_verification = EXCLUDED.register_email_verification,
		   email_enabled = EXCLUDED.email_enabled,
		   dingtalk_enabled = EXCLUDED.dingtalk_enabled,
		   dingtalk_webhook = EXCLUDED.dingtalk_webhook,
		   wechat_enabled = EXCLUDED.wechat_enabled,
		   wechat_webhook = EXCLUDED.wechat_webhook,
		   feishu_enabled = EXCLUDED.feishu_enabled,
		   feishu_webhook = EXCLUDED.feishu_webhook,
		   notify_node_resource = EXCLUDED.notify_node_resource,
		   notify_node_monitor = EXCLUDED.notify_node_monitor,
		   notify_ticket_reply = EXCLUDED.notify_ticket_reply,
		   notify_interval = EXCLUDED.notify_interval,
		   threshold_cpu = EXCLUDED.threshold_cpu,
		   threshold_memory = EXCLUDED.threshold_memory,
		   threshold_disk = EXCLUDED.threshold_disk,
		   threshold_bandwidth_up = EXCLUDED.threshold_bandwidth_up,
		   threshold_bandwidth_down = EXCLUDED.threshold_bandwidth_down,
		   retention_system_logs = EXCLUDED.retention_system_logs,
		   retention_es_logs = EXCLUDED.retention_es_logs,
		   retention_waf_bans = EXCLUDED.retention_waf_bans,
		   retention_upgrade_logs = EXCLUDED.retention_upgrade_logs,
		   updated_at = EXCLUDED.updated_at`,
		s.ID,
		s.SystemName,
		s.FooterLinks,
		s.FooterCopyright,
		s.Favicon,
		s.Logo,
		s.SMTPHost,
		s.SMTPPort,
		s.SMTPUsername,
		s.SMTPPassword,
		s.SMTPFrom,
		s.SMTPFromName,
		s.ElasticsearchURL,
		s.ElasticsearchUser,
		s.ElasticsearchPass,
		s.ElasticsearchIndex,
		s.ElasticsearchTSField,
		s.ElasticsearchDomainField,
		s.ElasticsearchBytesField,
		s.SalesEmail,
		s.SupportEmail,
		s.RegisterEnabled,
		s.UpgradeChannel,
		s.NotifyNewBuild,
		s.RegisterEmailVerification,
		s.EmailEnabled,
		s.DingtalkEnabled,
		s.DingtalkWebhook,
		s.WechatEnabled,
		s.WechatWebhook,
		s.FeishuEnabled,
		s.FeishuWebhook,
		s.NotifyNodeResource,
		s.NotifyNodeMonitor,
		s.NotifyTicketReply,
		s.NotifyInterval,
		s.ThresholdCPU,
		s.ThresholdMemory,
		s.ThresholdDisk,
		s.ThresholdBandwidthUp,
		s.ThresholdBandwidthDown,
		s.RetentionSystemLogs,
		s.RetentionESLogs,
		s.RetentionWafBans,
		s.RetentionUpgradeLogs,
		s.UpdatedAt,
	)
	return err
}

func (p *Postgres) GetBalanceAccount(ctx context.Context, userID string) (*BalanceAccount, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	row := p.pool.QueryRow(ctx, `SELECT user_id, balance_cents, currency, updated_at FROM balance_accounts WHERE user_id=$1 LIMIT 1`, userID)
	var a BalanceAccount
	if err := row.Scan(&a.UserID, &a.BalanceCents, &a.Currency, &a.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &BalanceAccount{UserID: userID, BalanceCents: 0, Currency: "CNY"}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(a.Currency) == "" {
		a.Currency = "CNY"
	}
	return &a, nil
}

func (p *Postgres) ListBalanceTransactions(ctx context.Context, userID string, page, pageSize int) ([]*BalanceTransaction, int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return []*BalanceTransaction{}, 0, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	var total int64
	_ = p.pool.QueryRow(ctx, `SELECT COUNT(1) FROM balance_transactions WHERE user_id=$1`, userID).Scan(&total)
	rows, err := p.pool.Query(ctx,
		`SELECT id, user_id, type, amount_cents, balance_cents, note, ref_type, ref_id, created_at
		 FROM balance_transactions WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []*BalanceTransaction
	for rows.Next() {
		var t BalanceTransaction
		if err := rows.Scan(&t.ID, &t.UserID, &t.Type, &t.AmountCents, &t.BalanceCents, &t.Note, &t.RefType, &t.RefID, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		res = append(res, &t)
	}
	return res, total, rows.Err()
}

func (p *Postgres) AdminListBalanceAccounts(ctx context.Context, userID string, page, pageSize int) ([]*BalanceAccount, int64, error) {
	userID = strings.TrimSpace(userID)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	qCount := `SELECT COUNT(1) FROM balance_accounts WHERE 1=1`
	q := `SELECT user_id, balance_cents, currency, updated_at FROM balance_accounts WHERE 1=1`
	args := make([]any, 0, 1)
	if userID != "" {
		args = append(args, userID)
		qCount += fmt.Sprintf(" AND user_id=$%d", len(args))
		q += fmt.Sprintf(" AND user_id=$%d", len(args))
	}
	var total int64
	_ = p.pool.QueryRow(ctx, qCount, args...).Scan(&total)
	args = append(args, pageSize, offset)
	q += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []*BalanceAccount
	for rows.Next() {
		var a BalanceAccount
		if err := rows.Scan(&a.UserID, &a.BalanceCents, &a.Currency, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if strings.TrimSpace(a.Currency) == "" {
			a.Currency = "CNY"
		}
		res = append(res, &a)
	}
	return res, total, rows.Err()
}

func (p *Postgres) CreateBalanceRecharge(ctx context.Context, r *BalanceRecharge) error {
	if r == nil || r.ID == "" || r.UserID == "" || r.OutTradeNo == "" {
		return fmt.Errorf("invalid recharge")
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO balance_recharges (id, user_id, out_trade_no, amount_cents, currency, payment_method, payment_provider, payment_url, qr_code, notify_raw, expires_at, closed_at, status, trade_no, paid_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		r.ID, r.UserID, r.OutTradeNo, r.AmountCents, r.Currency, r.PaymentMethod, r.PaymentProvider, r.PaymentURL, r.QRCode, r.NotifyRaw, nullTime(r.ExpiresAt), nullTime(r.ClosedAt), r.Status, r.TradeNo, nullTime(r.PaidAt), r.CreatedAt, r.UpdatedAt)
	return err
}

func (p *Postgres) GetBalanceRechargeByOutTradeNo(ctx context.Context, outTradeNo string) (*BalanceRecharge, error) {
	outTradeNo = strings.TrimSpace(outTradeNo)
	if outTradeNo == "" {
		return nil, nil
	}
	row := p.pool.QueryRow(ctx,
		`SELECT id, user_id, out_trade_no, amount_cents, currency, payment_method, payment_provider, payment_url, qr_code, notify_raw, COALESCE(expires_at, '0001-01-01'::timestamptz), COALESCE(closed_at, '0001-01-01'::timestamptz), status, trade_no, COALESCE(paid_at, '0001-01-01'::timestamptz), created_at, updated_at
		 FROM balance_recharges WHERE out_trade_no=$1 LIMIT 1`, outTradeNo)
	var r BalanceRecharge
	if err := row.Scan(&r.ID, &r.UserID, &r.OutTradeNo, &r.AmountCents, &r.Currency, &r.PaymentMethod, &r.PaymentProvider, &r.PaymentURL, &r.QRCode, &r.NotifyRaw, &r.ExpiresAt, &r.ClosedAt, &r.Status, &r.TradeNo, &r.PaidAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (p *Postgres) AdminListBalanceRecharges(ctx context.Context, userID, status string, page, pageSize int) ([]*BalanceRecharge, int64, error) {
	userID = strings.TrimSpace(userID)
	status = strings.TrimSpace(status)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	qCount := `SELECT COUNT(1) FROM balance_recharges WHERE 1=1`
	q := `SELECT id, user_id, out_trade_no, amount_cents, currency, payment_method, payment_provider, payment_url, qr_code, notify_raw, COALESCE(expires_at, '0001-01-01'::timestamptz), COALESCE(closed_at, '0001-01-01'::timestamptz), status, trade_no, COALESCE(paid_at, '0001-01-01'::timestamptz), created_at, updated_at FROM balance_recharges WHERE 1=1`
	args := make([]any, 0, 2)
	if userID != "" {
		args = append(args, userID)
		qCount += fmt.Sprintf(" AND user_id=$%d", len(args))
		q += fmt.Sprintf(" AND user_id=$%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		qCount += fmt.Sprintf(" AND status=$%d", len(args))
		q += fmt.Sprintf(" AND status=$%d", len(args))
	}
	var total int64
	_ = p.pool.QueryRow(ctx, qCount, args...).Scan(&total)
	args = append(args, pageSize, offset)
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []*BalanceRecharge
	for rows.Next() {
		var r BalanceRecharge
		if err := rows.Scan(&r.ID, &r.UserID, &r.OutTradeNo, &r.AmountCents, &r.Currency, &r.PaymentMethod, &r.PaymentProvider, &r.PaymentURL, &r.QRCode, &r.NotifyRaw, &r.ExpiresAt, &r.ClosedAt, &r.Status, &r.TradeNo, &r.PaidAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if strings.TrimSpace(r.Currency) == "" {
			r.Currency = "CNY"
		}
		res = append(res, &r)
	}
	return res, total, rows.Err()
}

func (p *Postgres) AdminUpdateBalanceRecharge(ctx context.Context, id, status, tradeNo, notifyRaw string, paidAt time.Time) error {
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" || status == "" {
		return nil
	}
	if status == "paid" && paidAt.IsZero() {
		paidAt = time.Now()
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var r BalanceRecharge
	if err := tx.QueryRow(ctx,
		`SELECT id, user_id, out_trade_no, amount_cents, currency, payment_method, status
		 FROM balance_recharges WHERE id=$1 FOR UPDATE`, id).
		Scan(&r.ID, &r.UserID, &r.OutTradeNo, &r.AmountCents, &r.Currency, &r.PaymentMethod, &r.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	if r.Status == "pending" && status == "paid" {
		var bal int64
		var cur string
		err := tx.QueryRow(ctx, `SELECT balance_cents, currency FROM balance_accounts WHERE user_id=$1 FOR UPDATE`, r.UserID).Scan(&bal, &cur)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				cur = "CNY"
				if _, err := tx.Exec(ctx, `INSERT INTO balance_accounts (user_id, balance_cents, currency, updated_at) VALUES ($1,$2,$3,NOW())`, r.UserID, 0, cur); err != nil {
					return err
				}
				bal = 0
			} else {
				return err
			}
		}
		next := bal + r.AmountCents
		if _, err := tx.Exec(ctx, `UPDATE balance_accounts SET balance_cents=$1, currency=$2, updated_at=NOW() WHERE user_id=$3`, next, cur, r.UserID); err != nil {
			return err
		}
		tid := generateID()
		if _, err := tx.Exec(ctx,
			`INSERT INTO balance_transactions (id, user_id, type, amount_cents, balance_cents, note, ref_type, ref_id, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			tid, r.UserID, "recharge", r.AmountCents, next, "recharge paid", "recharge", r.ID, paidAt); err != nil {
			return err
		}
	}
	closedAt := sql.NullTime{}
	if status == "closed" || status == "cancelled" {
		closedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	paidAtArg := sql.NullTime{}
	if status == "paid" {
		paidAtArg = nullTime(paidAt)
	}
	if _, err := tx.Exec(ctx, `UPDATE balance_recharges SET status=$1, trade_no=$2, paid_at=COALESCE($3, paid_at), notify_raw=COALESCE(NULLIF($4, ''), notify_raw), closed_at=COALESCE($5, closed_at), updated_at=NOW() WHERE id=$6`, status, tradeNo, paidAtArg, notifyRaw, closedAt, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) AdminListBalanceWithdrawals(ctx context.Context, userID, status string, page, pageSize int) ([]*BalanceWithdrawal, int64, error) {
	userID = strings.TrimSpace(userID)
	status = strings.TrimSpace(status)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	qCount := `SELECT COUNT(1) FROM balance_withdrawals WHERE 1=1`
	q := `SELECT id, user_id, amount_cents, currency, method, account_name, account_no, status, note, COALESCE(reviewed_at, '0001-01-01'::timestamptz), created_at, updated_at FROM balance_withdrawals WHERE 1=1`
	args := make([]any, 0, 2)
	if userID != "" {
		args = append(args, userID)
		qCount += fmt.Sprintf(" AND user_id=$%d", len(args))
		q += fmt.Sprintf(" AND user_id=$%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		qCount += fmt.Sprintf(" AND status=$%d", len(args))
		q += fmt.Sprintf(" AND status=$%d", len(args))
	}
	var total int64
	_ = p.pool.QueryRow(ctx, qCount, args...).Scan(&total)
	args = append(args, pageSize, offset)
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []*BalanceWithdrawal
	for rows.Next() {
		var w BalanceWithdrawal
		if err := rows.Scan(&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method, &w.AccountName, &w.AccountNo, &w.Status, &w.Note, &w.ReviewedAt, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if strings.TrimSpace(w.Currency) == "" {
			w.Currency = "CNY"
		}
		res = append(res, &w)
	}
	return res, total, rows.Err()
}

func (p *Postgres) AdminUpdateBalanceWithdrawal(ctx context.Context, id, status, note string, reviewedAt time.Time) error {
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" || status == "" {
		return nil
	}
	if reviewedAt.IsZero() {
		reviewedAt = time.Now()
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var w BalanceWithdrawal
	var curStatus string
	if err := tx.QueryRow(ctx, `SELECT id, user_id, amount_cents, currency, status FROM balance_withdrawals WHERE id=$1 FOR UPDATE`, id).
		Scan(&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &curStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if curStatus == status {
		if _, err := tx.Exec(ctx, `UPDATE balance_withdrawals SET note=$1, updated_at=NOW() WHERE id=$2`, note, id); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if curStatus == "pending" && (status == "approved" || status == "paid") {
		var bal int64
		err := tx.QueryRow(ctx, `SELECT balance_cents FROM balance_accounts WHERE user_id=$1 FOR UPDATE`, w.UserID).Scan(&bal)
		if err != nil {
			return errors.New("insufficient balance")
		}
		if bal < w.AmountCents {
			return errors.New("insufficient balance")
		}
		next := bal - w.AmountCents
		if _, err := tx.Exec(ctx, `UPDATE balance_accounts SET balance_cents=$1, updated_at=NOW() WHERE user_id=$2`, next, w.UserID); err != nil {
			return err
		}
		tid := generateID()
		if _, err := tx.Exec(ctx,
			`INSERT INTO balance_transactions (id, user_id, type, amount_cents, balance_cents, note, ref_type, ref_id, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			tid, w.UserID, "withdraw", -w.AmountCents, next, note, "withdrawal", w.ID, reviewedAt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE balance_withdrawals SET status=$1, note=$2, reviewed_at=$3, updated_at=NOW() WHERE id=$4`, status, note, nullTime(reviewedAt), id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) AdminAdjustBalance(ctx context.Context, userID string, amountCents int64, note string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || amountCents == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var bal int64
	var cur string
	err = tx.QueryRow(ctx, `SELECT balance_cents, currency FROM balance_accounts WHERE user_id=$1 FOR UPDATE`, userID).Scan(&bal, &cur)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			cur = "CNY"
			if _, err := tx.Exec(ctx, `INSERT INTO balance_accounts (user_id, balance_cents, currency, updated_at) VALUES ($1,$2,$3,NOW())`, userID, 0, cur); err != nil {
				return err
			}
			bal = 0
		} else {
			return err
		}
	}
	next := bal + amountCents
	if next < 0 {
		return errors.New("insufficient balance")
	}
	if _, err := tx.Exec(ctx, `UPDATE balance_accounts SET balance_cents=$1, currency=$2, updated_at=NOW() WHERE user_id=$3`, next, cur, userID); err != nil {
		return err
	}
	tid := generateID()
	if _, err := tx.Exec(ctx,
		`INSERT INTO balance_transactions (id, user_id, type, amount_cents, balance_cents, note, ref_type, ref_id, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())`,
		tid, userID, "adjust", amountCents, next, note, "adjust", ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) AdminRechargeStats(ctx context.Context, from, to time.Time) ([]*BalanceRechargeStats, error) {
	if from.IsZero() || to.IsZero() {
		return []*BalanceRechargeStats{}, nil
	}
	to = to.Add(24 * time.Hour)
	rows, err := p.pool.Query(ctx, `
		WITH recharge_stats AS (
			SELECT to_char(paid_at, 'YYYY-MM-DD') AS day,
			       SUM(amount_cents) AS recharge_cents,
			       COUNT(1) AS recharge_count
			FROM balance_recharges
			WHERE status='paid'
			  AND paid_at IS NOT NULL
			  AND paid_at >= $1
			  AND paid_at < $2
			GROUP BY day
		), adjust_stats AS (
			SELECT to_char(created_at, 'YYYY-MM-DD') AS day,
			       SUM(amount_cents) AS adjust_cents,
			       COUNT(1) AS adjust_count
			FROM balance_transactions
			WHERE type='adjust'
			  AND created_at >= $1
			  AND created_at < $2
			GROUP BY day
		)
		SELECT
			COALESCE(r.day, a.day) AS day,
			COALESCE(r.recharge_cents, 0) AS recharge_cents,
			COALESCE(r.recharge_count, 0) AS recharge_count,
			COALESCE(a.adjust_cents, 0) AS adjust_cents,
			COALESCE(a.adjust_count, 0) AS adjust_count,
			(COALESCE(r.recharge_cents, 0) + COALESCE(a.adjust_cents, 0)) AS total_cents,
			(COALESCE(r.recharge_count, 0) + COALESCE(a.adjust_count, 0)) AS total_count,
			COALESCE(r.recharge_cents, 0) AS paid_cents,
			COALESCE(r.recharge_count, 0) AS paid_count,
			0 AS pending_count
		FROM recharge_stats r
		FULL JOIN adjust_stats a ON a.day = r.day
		ORDER BY COALESCE(r.day, a.day)
	`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []*BalanceRechargeStats
	for rows.Next() {
		var s BalanceRechargeStats
		if err := rows.Scan(
			&s.Day,
			&s.RechargeCents,
			&s.RechargeCount,
			&s.AdjustCents,
			&s.AdjustCount,
			&s.TotalCents,
			&s.TotalCount,
			&s.PaidCents,
			&s.PaidCount,
			&s.PendingCount,
		); err != nil {
			return nil, err
		}
		res = append(res, &s)
	}
	return res, rows.Err()
}

func (p *Postgres) ListAnnouncements(ctx context.Context, status, q string, page, pageSize int) ([]*Announcement, int64, error) {
	status = strings.TrimSpace(status)
	q = strings.TrimSpace(q)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	qCount := `SELECT COUNT(1) FROM announcements WHERE 1=1`
	query := `SELECT id, title, content, status, pinned, created_at, updated_at FROM announcements WHERE 1=1`
	args := make([]any, 0, 3)
	if status != "" {
		args = append(args, status)
		qCount += fmt.Sprintf(" AND status=$%d", len(args))
		query += fmt.Sprintf(" AND status=$%d", len(args))
	}
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like)
		qCount += fmt.Sprintf(" AND (LOWER(title) LIKE $%d OR LOWER(content) LIKE $%d)", len(args)-1, len(args))
		query += fmt.Sprintf(" AND (LOWER(title) LIKE $%d OR LOWER(content) LIKE $%d)", len(args)-1, len(args))
	}
	var total int64
	_ = p.pool.QueryRow(ctx, qCount, args...).Scan(&total)
	args = append(args, pageSize, offset)
	query += fmt.Sprintf(" ORDER BY pinned DESC, updated_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []*Announcement
	for rows.Next() {
		var a Announcement
		if err := rows.Scan(&a.ID, &a.Title, &a.Content, &a.Status, &a.Pinned, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		res = append(res, &a)
	}
	return res, total, rows.Err()
}

func (p *Postgres) GetAnnouncement(ctx context.Context, id string) (*Announcement, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	row := p.pool.QueryRow(ctx,
		`SELECT id, title, content, status, pinned, created_at, updated_at FROM announcements WHERE id=$1`, id)
	var a Announcement
	if err := row.Scan(&a.ID, &a.Title, &a.Content, &a.Status, &a.Pinned, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (p *Postgres) CreateAnnouncement(ctx context.Context, a *Announcement) error {
	if a == nil {
		return nil
	}
	if a.ID == "" {
		a.ID = generateID()
	}
	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	if strings.TrimSpace(a.Status) == "" {
		a.Status = "draft"
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO announcements (id, title, content, status, pinned, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		a.ID, a.Title, a.Content, a.Status, a.Pinned, a.CreatedAt, a.UpdatedAt)
	return err
}

func (p *Postgres) UpdateAnnouncement(ctx context.Context, a *Announcement) error {
	if a == nil || strings.TrimSpace(a.ID) == "" {
		return nil
	}
	a.UpdatedAt = time.Now()
	_, err := p.pool.Exec(ctx,
		`UPDATE announcements SET title=$1, content=$2, status=$3, pinned=$4, updated_at=$5 WHERE id=$6`,
		a.Title, a.Content, a.Status, a.Pinned, a.UpdatedAt, a.ID)
	return err
}

func (p *Postgres) DeleteAnnouncement(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM announcements WHERE id=$1`, id)
	return err
}

func (p *Postgres) CreateSystemLog(ctx context.Context, log *SystemLog) error {
	if log == nil {
		return nil
	}
	if log.ID == "" {
		log.ID = generateID()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO system_logs (id, type, status, message, user_id, username, ip, location, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		log.ID, log.Type, log.Status, log.Message, log.UserID, log.Username, log.IP, log.Location, log.CreatedAt)
	return err
}

func (p *Postgres) ListSystemLogs(ctx context.Context, logType, status, q string, page, pageSize int) ([]*SystemLog, int64, error) {
	logType = strings.TrimSpace(logType)
	status = strings.TrimSpace(status)
	q = strings.TrimSpace(q)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	qCount := `SELECT COUNT(1) FROM system_logs WHERE 1=1`
	query := `SELECT id, type, status, message, user_id, username, ip, location, created_at FROM system_logs WHERE 1=1`
	args := make([]any, 0, 4)
	if logType != "" {
		args = append(args, logType)
		qCount += fmt.Sprintf(" AND type=$%d", len(args))
		query += fmt.Sprintf(" AND type=$%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		qCount += fmt.Sprintf(" AND status=$%d", len(args))
		query += fmt.Sprintf(" AND status=$%d", len(args))
	}
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like)
		qCount += fmt.Sprintf(" AND (LOWER(username) LIKE $%d OR LOWER(message) LIKE $%d OR LOWER(ip) LIKE $%d)", len(args)-2, len(args)-1, len(args))
		query += fmt.Sprintf(" AND (LOWER(username) LIKE $%d OR LOWER(message) LIKE $%d OR LOWER(ip) LIKE $%d)", len(args)-2, len(args)-1, len(args))
	}
	var total int64
	_ = p.pool.QueryRow(ctx, qCount, args...).Scan(&total)
	args = append(args, pageSize, offset)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []*SystemLog
	for rows.Next() {
		var l SystemLog
		if err := rows.Scan(&l.ID, &l.Type, &l.Status, &l.Message, &l.UserID, &l.Username, &l.IP, &l.Location, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		res = append(res, &l)
	}
	return res, total, rows.Err()
}

func (p *Postgres) DeleteSystemLogsOlderThan(ctx context.Context, before time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx, `DELETE FROM system_logs WHERE created_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("delete old system_logs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (p *Postgres) DeleteExpiredWafBansOlderThan(ctx context.Context, before time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx, `DELETE FROM waf_bans WHERE expires_at IS NOT NULL AND expires_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("delete expired waf_bans: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (p *Postgres) DeleteUpgradeTasksOlderThan(ctx context.Context, before time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx, `DELETE FROM upgrade_tasks WHERE created_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("delete old upgrade_tasks: %w", err)
	}
	return tag.RowsAffected(), nil
}

// --- DNS config ---

func (p *Postgres) GetDNSConfig(ctx context.Context) (*DNSConfig, error) {
	var cfg DNSConfig
	err := p.pool.QueryRow(ctx,
		`SELECT provider, account_id, token, secret, ttl, enable_ip_weight, last_error, updated_at FROM dns_config WHERE id = 1`).
		Scan(&cfg.Provider, &cfg.AccountID, &cfg.Token, &cfg.Secret, &cfg.TTL, &cfg.EnableIPWeight, &cfg.LastError, &cfg.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

func (p *Postgres) SaveDNSConfig(ctx context.Context, cfg *DNSConfig) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO dns_config (id, provider, account_id, token, secret, ttl, enable_ip_weight, last_error, updated_at)
		 VALUES (1, $1, $2, $3, $4, $5, $6, $7, NOW())
		 ON CONFLICT (id) DO UPDATE SET provider = EXCLUDED.provider, account_id = EXCLUDED.account_id, token = EXCLUDED.token, secret = EXCLUDED.secret,
			ttl = EXCLUDED.ttl, enable_ip_weight = EXCLUDED.enable_ip_weight, last_error = EXCLUDED.last_error, updated_at = NOW()`,
		cfg.Provider, cfg.AccountID, cfg.Token, cfg.Secret, cfg.TTL, cfg.EnableIPWeight, cfg.LastError)
	return err
}

// --- Helper functions ---

func scanNode(row pgx.Row) (*Node, error) {
	var n Node
	var lastHeartbeat sql.NullTime
	var lastMetricsAt sql.NullTime
	var monitorLastAt sql.NullTime
	err := row.Scan(
		&n.ID, &n.Hostname, &n.PublicIP, &n.Version, &n.Status, &n.Region, &n.Cluster, &n.Capabilities,
		&n.ConfigVersion, &n.Token, &lastHeartbeat,
		&n.MonitorEnabled, &n.MonitorProtocol, &n.MonitorTimeout, &n.MonitorPort, &n.MonitorFailThreshold,
		&n.MonitorFailCount, &n.MonitorLastOK, &n.MonitorLastError, &monitorLastAt, &n.MonitorLastLatencyMs,
		&n.CPUUsage, &n.MemUsage, &n.DiskUsage, &n.CPUCount, &n.MemTotal, &n.DiskTotal, &lastMetricsAt,
		&n.BytesSent, &n.BytesReceived, &n.BandwidthUpBps, &n.BandwidthDownBps,
		&n.TCPEstablished, &n.TCPSynRecv, &n.TCPTimeWait, &n.NginxRunning, &n.MonthBytesSent,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lastHeartbeat.Valid {
		n.LastHeartbeat = lastHeartbeat.Time
	}
	if monitorLastAt.Valid {
		n.MonitorLastAt = monitorLastAt.Time
	}
	if lastMetricsAt.Valid {
		n.LastMetricsAt = lastMetricsAt.Time
	}
	return &n, nil
}

func scanNodeRows(rows pgx.Rows) (*Node, error) {
	var n Node
	var lastHeartbeat sql.NullTime
	var lastMetricsAt sql.NullTime
	var monitorLastAt sql.NullTime
	err := rows.Scan(
		&n.ID, &n.Hostname, &n.PublicIP, &n.Version, &n.Status, &n.Region, &n.Cluster, &n.Capabilities,
		&n.ConfigVersion, &n.Token, &lastHeartbeat,
		&n.MonitorEnabled, &n.MonitorProtocol, &n.MonitorTimeout, &n.MonitorPort, &n.MonitorFailThreshold,
		&n.MonitorFailCount, &n.MonitorLastOK, &n.MonitorLastError, &monitorLastAt, &n.MonitorLastLatencyMs,
		&n.CPUUsage, &n.MemUsage, &n.DiskUsage, &n.CPUCount, &n.MemTotal, &n.DiskTotal, &lastMetricsAt,
		&n.BytesSent, &n.BytesReceived, &n.BandwidthUpBps, &n.BandwidthDownBps,
		&n.TCPEstablished, &n.TCPSynRecv, &n.TCPTimeWait, &n.NginxRunning, &n.MonthBytesSent,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if lastHeartbeat.Valid {
		n.LastHeartbeat = lastHeartbeat.Time
	}
	if monitorLastAt.Valid {
		n.MonitorLastAt = monitorLastAt.Time
	}
	if lastMetricsAt.Valid {
		n.LastMetricsAt = lastMetricsAt.Time
	}
	return &n, nil
}

func scanDomain(row pgx.Row) (*Domain, error) {
	var d Domain
	var certID sql.NullString
	var lineGroupID sql.NullString
	var originID sql.NullString
	var errorPagesBytes []byte
	var securityBytes []byte
	var originAuthBytes []byte
	var loadBalanceMethod string
	var healthCheckBytes []byte
	err := row.Scan(&d.ID, &d.Name, &d.CNAME, &d.UserID, &lineGroupID, &originID, &certID,
		&d.OriginScheme, &d.OriginPort, &d.OriginHostMode, &d.OriginHost, &d.OriginTimeoutMs, &d.OriginConnectTimeoutMs, &errorPagesBytes,
		&d.CacheEnabled, &d.HTTP2Enabled, &d.WebsocketEnabled, &d.HTTPSEnabled, &d.Enabled, &securityBytes, &originAuthBytes,
		&loadBalanceMethod, &healthCheckBytes, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lineGroupID.Valid {
		d.LineGroupID = lineGroupID.String
	}
	if originID.Valid {
		d.OriginID = originID.String
	}
	if certID.Valid {
		d.CertID = certID.String
	}
	if len(errorPagesBytes) > 0 {
		_ = json.Unmarshal(errorPagesBytes, &d.ErrorPages)
	}
	if len(securityBytes) > 0 && string(securityBytes) != "{}" {
		var sec DomainSecurity
		if err := json.Unmarshal(securityBytes, &sec); err == nil {
			d.Security = &sec
		}
	}
	if len(originAuthBytes) > 0 && string(originAuthBytes) != "{}" {
		var auth OriginAuth
		if err := json.Unmarshal(originAuthBytes, &auth); err == nil {
			d.OriginAuth = &auth
		}
	}
	d.LoadBalanceMethod = defaultLoadBalanceMethod(loadBalanceMethod)
	if len(healthCheckBytes) > 0 && string(healthCheckBytes) != "{}" {
		var hc OriginHealthCheck
		if err := json.Unmarshal(healthCheckBytes, &hc); err == nil {
			d.OriginHealthCheck = &hc
		}
	}
	return &d, nil
}

func scanDomainRows(rows pgx.Rows) (*Domain, error) {
	var d Domain
	var certID sql.NullString
	var lineGroupID sql.NullString
	var originID sql.NullString
	var errorPagesBytes []byte
	var securityBytes []byte
	var originAuthBytes []byte
	var loadBalanceMethod string
	var healthCheckBytes []byte
	err := rows.Scan(&d.ID, &d.Name, &d.CNAME, &d.UserID, &lineGroupID, &originID, &certID,
		&d.OriginScheme, &d.OriginPort, &d.OriginHostMode, &d.OriginHost, &d.OriginTimeoutMs, &d.OriginConnectTimeoutMs, &errorPagesBytes,
		&d.CacheEnabled, &d.HTTP2Enabled, &d.WebsocketEnabled, &d.HTTPSEnabled, &d.Enabled, &securityBytes, &originAuthBytes,
		&loadBalanceMethod, &healthCheckBytes, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if lineGroupID.Valid {
		d.LineGroupID = lineGroupID.String
	}
	if originID.Valid {
		d.OriginID = originID.String
	}
	if certID.Valid {
		d.CertID = certID.String
	}
	if len(errorPagesBytes) > 0 {
		_ = json.Unmarshal(errorPagesBytes, &d.ErrorPages)
	}
	if len(securityBytes) > 0 && string(securityBytes) != "{}" {
		var sec DomainSecurity
		if err := json.Unmarshal(securityBytes, &sec); err == nil {
			d.Security = &sec
		}
	}
	if len(originAuthBytes) > 0 && string(originAuthBytes) != "{}" {
		var auth OriginAuth
		if err := json.Unmarshal(originAuthBytes, &auth); err == nil {
			d.OriginAuth = &auth
		}
	}
	d.LoadBalanceMethod = defaultLoadBalanceMethod(loadBalanceMethod)
	if len(healthCheckBytes) > 0 && string(healthCheckBytes) != "{}" {
		var hc OriginHealthCheck
		if err := json.Unmarshal(healthCheckBytes, &hc); err == nil {
			d.OriginHealthCheck = &hc
		}
	}
	return &d, nil
}

func scanOrigin(row pgx.Row) (*Origin, error) {
	var o Origin
	err := row.Scan(&o.ID, &o.Name, &o.Addresses, &o.TimeoutMs, &o.MaxRetries, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &o, nil
}

func scanOriginRows(rows pgx.Rows) (*Origin, error) {
	var o Origin
	err := rows.Scan(&o.ID, &o.Name, &o.Addresses, &o.TimeoutMs, &o.MaxRetries, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func scanCertificate(row pgx.Row) (*Certificate, error) {
	var c Certificate
	err := row.Scan(&c.ID, &c.Name, &c.Domain, &c.UserID, &c.Type, &c.AutoRenew,
		&c.Status, &c.FailReason, &c.CertPEM, &c.KeyPEM, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func scanCertificateRows(rows pgx.Rows) (*Certificate, error) {
	var c Certificate
	err := rows.Scan(&c.ID, &c.Name, &c.Domain, &c.UserID, &c.Type, &c.AutoRenew,
		&c.Status, &c.FailReason, &c.CertPEM, &c.KeyPEM, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanConfigVersion(row pgx.Row) (*ConfigVersion, error) {
	var cv ConfigVersion
	err := row.Scan(&cv.Version, &cv.Checksum, &cv.Payload, &cv.CreatedAt, &cv.CreatedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &cv, nil
}

func scanConfigVersionRows(rows pgx.Rows) (*ConfigVersion, error) {
	var cv ConfigVersion
	err := rows.Scan(&cv.Version, &cv.Checksum, &cv.Payload, &cv.CreatedAt, &cv.CreatedBy)
	if err != nil {
		return nil, err
	}
	return &cv, nil
}

func scanCacheRule(row pgx.Row) (*CacheRule, error) {
	var r CacheRule
	err := row.Scan(&r.ID, &r.Name, &r.HostPattern, &r.PathPattern, &r.Methods,
		&r.TTLSeconds, &r.CacheQueryParams, &r.Priority, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func scanCacheRuleRows(rows pgx.Rows) (*CacheRule, error) {
	var r CacheRule
	err := rows.Scan(&r.ID, &r.Name, &r.HostPattern, &r.PathPattern, &r.Methods,
		&r.TTLSeconds, &r.CacheQueryParams, &r.Priority, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func marshalErrorPages(pages []ErrorPage) []byte {
	if len(pages) == 0 {
		return []byte("[]")
	}
	data, err := json.Marshal(pages)
	if err != nil {
		return []byte("[]")
	}
	return data
}

// marshalDomainSecurity serialises DomainSecurity for the security_json
// JSONB column. nil becomes "{}" so the NOT NULL constraint holds.
func marshalDomainSecurity(sec *DomainSecurity) []byte {
	if sec == nil {
		return []byte("{}")
	}
	data, err := json.Marshal(sec)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func marshalOriginAuth(auth *OriginAuth) []byte {
	if auth == nil {
		return []byte("{}")
	}
	data, err := json.Marshal(auth)
	if err != nil {
		return []byte("{}")
	}
	return data
}

// marshalOriginHealthCheck serialises OriginHealthCheck for the
// origin_health_check_json JSONB column. nil becomes "{}" so the
// NOT NULL constraint holds.
func marshalOriginHealthCheck(hc *OriginHealthCheck) []byte {
	if hc == nil {
		return []byte("{}")
	}
	data, err := json.Marshal(hc)
	if err != nil {
		return []byte("{}")
	}
	return data
}

// defaultLoadBalanceMethod normalises the load-balance method string.
// Empty / unknown values fall back to "round_robin" so legacy rows
// keep their previous behaviour.
func defaultLoadBalanceMethod(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ip_hash":
		return "ip_hash"
	default:
		return "round_robin"
	}
}

func defaultOriginScheme(s string) string {
	if strings.TrimSpace(s) == "" {
		return "http"
	}
	return s
}

func defaultOriginPort(p int32) int32 {
	if p <= 0 {
		return 80
	}
	return p
}

func defaultOriginHostMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return "request_host"
	}
	return mode
}

func defaultOriginTimeout(v int64) int64 {
	if v <= 0 {
		return 60000
	}
	return v
}

func defaultOriginConnectTimeout(v int64) int64 {
	if v <= 0 {
		return 10000
	}
	return v
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func logSeededToken(tokenType, token string) {
	masked := maskToken(token)
	log.Info().
		Str("token_type", tokenType).
		Str("token_masked", masked).
		Msg("generated token")

	if shouldPrintSeedTokens() {
		log.Warn().
			Str("token_type", tokenType).
			Msgf("generated %s token (save and rotate if needed): %s", tokenType, token)
	}
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func shouldPrintSeedTokens() bool {
	v := strings.ToLower(os.Getenv("SEED_PRINT_TOKENS"))
	return v == "1" || v == "true" || v == "yes"
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// --- Upgrade tasks/logs ---

func (p *Postgres) CreateUpgradeTask(ctx context.Context, t *UpgradeTask) error {
	if t.ID == "" {
		t.ID = generateID()
	}
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.Type == "" {
		t.Type = "node"
	}
	if t.Channel == "" {
		t.Channel = "stable"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.NodeIDs == nil {
		t.NodeIDs = []string{}
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO upgrade_tasks (id, target_version, channel, node_ids, status, type, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (id) DO UPDATE SET
			target_version = EXCLUDED.target_version,
			channel = EXCLUDED.channel,
			node_ids = EXCLUDED.node_ids,
			status = EXCLUDED.status,
			type = EXCLUDED.type`,
		t.ID, t.TargetVersion, t.Channel, t.NodeIDs, t.Status, t.Type, t.CreatedAt)
	return err
}

// UpdateUpgradeTaskStatus updates only the status column of an upgrade task,
// preserving all other fields (target_version, node_ids, channel, type, ...).
func (p *Postgres) UpdateUpgradeTaskStatus(ctx context.Context, id, status string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("upgrade task id required")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return errors.New("upgrade task status required")
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE upgrade_tasks SET status = $2 WHERE id = $1`,
		id, status)
	return err
}

func (p *Postgres) ListUpgradeTasks(ctx context.Context, limit int) ([]*UpgradeTask, error) {
	if limit == 0 {
		limit = 50
	}
	rows, err := p.pool.Query(ctx,
		`SELECT id, target_version, channel, node_ids, status, type, created_at FROM upgrade_tasks ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []*UpgradeTask
	for rows.Next() {
		var t UpgradeTask
		if err := rows.Scan(&t.ID, &t.TargetVersion, &t.Channel, &t.NodeIDs, &t.Status, &t.Type, &t.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, &t)
	}
	return res, rows.Err()
}

func (p *Postgres) GetUpgradeTask(ctx context.Context, id string) (*UpgradeTask, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, target_version, channel, node_ids, status, type, created_at FROM upgrade_tasks WHERE id=$1`, id)
	var t UpgradeTask
	if err := row.Scan(&t.ID, &t.TargetVersion, &t.Channel, &t.NodeIDs, &t.Status, &t.Type, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (p *Postgres) AppendUpgradeLog(ctx context.Context, id string, log UpgradeLog) error {
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO upgrade_logs (task_id, node_id, level, message, ts) VALUES ($1,$2,$3,$4,$5)`,
		id, log.NodeID, strings.ToUpper(log.Level), log.Message, log.Timestamp)
	return err
}

func (p *Postgres) ListUpgradeLogs(ctx context.Context, id, nodeID string, limit int) ([]UpgradeLog, error) {
	if limit == 0 {
		limit = 200
	}
	var rows pgx.Rows
	var err error
	if nodeID != "" {
		rows, err = p.pool.Query(ctx,
			`SELECT task_id, node_id, level, message, ts FROM upgrade_logs WHERE task_id=$1 AND (node_id=$2 OR node_id='') ORDER BY ts ASC LIMIT $3`,
			id, nodeID, limit)
	} else {
		rows, err = p.pool.Query(ctx,
			`SELECT task_id, node_id, level, message, ts FROM upgrade_logs WHERE task_id=$1 ORDER BY ts ASC LIMIT $2`,
			id, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []UpgradeLog
	for rows.Next() {
		var l UpgradeLog
		if err := rows.Scan(&l.TaskID, &l.NodeID, &l.Level, &l.Message, &l.Timestamp); err != nil {
			return nil, err
		}
		res = append(res, l)
	}
	return res, rows.Err()
}

// --- WAF ---

func (p *Postgres) ListWAFPolicies(ctx context.Context) ([]*WAFPolicy, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, scope, scope_id, description, enabled, created_at, updated_at FROM waf_policies ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*WAFPolicy
	for rows.Next() {
		var pcy WAFPolicy
		if err := rows.Scan(&pcy.ID, &pcy.Name, &pcy.Scope, &pcy.ScopeID, &pcy.Description, &pcy.Enabled, &pcy.CreatedAt, &pcy.UpdatedAt); err != nil {
			return nil, err
		}
		if rules, err := p.ListWAFRules(ctx, pcy.ID); err == nil {
			pcy.Rules = rules
		}
		res = append(res, &pcy)
	}
	return res, rows.Err()
}

func (p *Postgres) GetWAFPolicy(ctx context.Context, id string) (*WAFPolicy, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, name, scope, scope_id, description, enabled, created_at, updated_at FROM waf_policies WHERE id=$1`, id)
	var pcy WAFPolicy
	if err := row.Scan(&pcy.ID, &pcy.Name, &pcy.Scope, &pcy.ScopeID, &pcy.Description, &pcy.Enabled, &pcy.CreatedAt, &pcy.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if rules, err := p.ListWAFRules(ctx, id); err == nil {
		pcy.Rules = rules
	}
	return &pcy, nil
}

func (p *Postgres) CreateWAFPolicy(ctx context.Context, pol *WAFPolicy) error {
	if pol.ID == "" {
		pol.ID = generateID()
	}
	if pol.Scope == "" {
		pol.Scope = "global"
	}
	now := time.Now()
	if pol.CreatedAt.IsZero() {
		pol.CreatedAt = now
	}
	pol.UpdatedAt = now
	_, err := p.pool.Exec(ctx,
		`INSERT INTO waf_policies (id, name, scope, scope_id, description, enabled, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, scope=EXCLUDED.scope, scope_id=EXCLUDED.scope_id,
		 description=EXCLUDED.description, enabled=EXCLUDED.enabled, updated_at=EXCLUDED.updated_at`,
		pol.ID, pol.Name, pol.Scope, pol.ScopeID, pol.Description, pol.Enabled, pol.CreatedAt, pol.UpdatedAt)
	return err
}

func (p *Postgres) UpdateWAFPolicy(ctx context.Context, pol *WAFPolicy) error {
	pol.UpdatedAt = time.Now()
	_, err := p.pool.Exec(ctx,
		`UPDATE waf_policies SET name=$2, scope=$3, scope_id=$4, description=$5, enabled=$6, updated_at=$7 WHERE id=$1`,
		pol.ID, pol.Name, pol.Scope, pol.ScopeID, pol.Description, pol.Enabled, pol.UpdatedAt)
	return err
}

func (p *Postgres) DeleteWAFPolicy(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM waf_policies WHERE id=$1`, id)
	return err
}

func (p *Postgres) ListWAFRules(ctx context.Context, policyID string) ([]*WAFRule, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, policy_id, type, action, value, threshold, window_seconds, shield_seconds, auto_challenge_qps, ban_seconds, template_html, ban_template_html, redirect_url, ban_mode, expires_at, path_prefix, methods, ua_contains, log_only, note, priority, enabled, created_at, updated_at
		 FROM waf_rules WHERE policy_id=$1 ORDER BY priority ASC, created_at ASC`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []*WAFRule
	for rows.Next() {
		var r WAFRule
		if err := rows.Scan(&r.ID, &r.PolicyID, &r.Type, &r.Action, &r.Value, &r.Threshold, &r.WindowSeconds, &r.ShieldSeconds, &r.AutoChallengeQPS, &r.BanSeconds, &r.TemplateHTML, &r.BanTemplateHTML, &r.RedirectURL, &r.BanMode, &r.ExpiresAt, &r.PathPrefix, &r.Methods, &r.UAContains, &r.LogOnly, &r.Note, &r.Priority, &r.Enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if r.BanMode == "" {
			r.BanMode = "ipset"
		}
		res = append(res, &r)
	}
	return res, rows.Err()
}

func (p *Postgres) ReplaceWAFRules(ctx context.Context, policyID string, rules []*WAFRule) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM waf_rules WHERE policy_id=$1`, policyID); err != nil {
		return err
	}
	now := time.Now()
	for _, r := range rules {
		if r == nil {
			continue
		}
		if r.ID == "" {
			r.ID = generateID()
		}
		r.PolicyID = policyID
		if r.CreatedAt.IsZero() {
			r.CreatedAt = now
		}
		r.UpdatedAt = now
		if r.BanMode == "" {
			r.BanMode = "ipset"
		}
		if r.Methods == nil {
			r.Methods = []string{}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO waf_rules (id, policy_id, type, action, value, threshold, window_seconds, shield_seconds, auto_challenge_qps, ban_seconds, template_html, ban_template_html, redirect_url, ban_mode, expires_at, path_prefix, methods, ua_contains, log_only, note, priority, enabled, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
			r.ID, r.PolicyID, r.Type, r.Action, r.Value, r.Threshold, r.WindowSeconds, r.ShieldSeconds, r.AutoChallengeQPS, r.BanSeconds, r.TemplateHTML, r.BanTemplateHTML, r.RedirectURL, r.BanMode, nullTime(r.ExpiresAt), r.PathPrefix, r.Methods, r.UAContains, r.LogOnly, r.Note, r.Priority, r.Enabled, r.CreatedAt, r.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// WAF bans
func (p *Postgres) ListWAFBans(ctx context.Context, limit int) ([]*WAFBan, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := p.pool.Query(ctx, `SELECT ip, reason, strikes, expires_at, created_at, updated_at FROM waf_bans ORDER BY expires_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []*WAFBan
	for rows.Next() {
		var b WAFBan
		if err := rows.Scan(&b.IP, &b.Reason, &b.Strikes, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, &b)
	}
	return res, rows.Err()
}

func (p *Postgres) CreateOrUpdateWAFBan(ctx context.Context, ban *WAFBan) error {
	if ban.CreatedAt.IsZero() {
		ban.CreatedAt = time.Now()
	}
	ban.UpdatedAt = time.Now()
	_, err := p.pool.Exec(ctx,
		`INSERT INTO waf_bans (ip, reason, strikes, expires_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (ip) DO UPDATE SET reason=EXCLUDED.reason, strikes=EXCLUDED.strikes, expires_at=EXCLUDED.expires_at, updated_at=EXCLUDED.updated_at`,
		ban.IP, ban.Reason, ban.Strikes, ban.ExpiresAt, ban.CreatedAt, ban.UpdatedAt)
	return err
}

func (p *Postgres) DeleteWAFBan(ctx context.Context, ip string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM waf_bans WHERE ip=$1`, ip)
	return err
}

func (p *Postgres) GetWAFBan(ctx context.Context, ip string) (*WAFBan, error) {
	row := p.pool.QueryRow(ctx, `SELECT ip, reason, strikes, expires_at, created_at, updated_at FROM waf_bans WHERE ip=$1`, ip)
	var b WAFBan
	if err := row.Scan(&b.IP, &b.Reason, &b.Strikes, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// WAF whitelist
func (p *Postgres) ListWAFWhitelist(ctx context.Context) ([]*WAFWhitelist, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, ip, note, created_by, created_at, updated_at FROM waf_whitelist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*WAFWhitelist
	for rows.Next() {
		var w WAFWhitelist
		if err := rows.Scan(&w.ID, &w.IP, &w.Note, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, &w)
	}
	return res, rows.Err()
}

func (p *Postgres) CreateWAFWhitelist(ctx context.Context, w *WAFWhitelist) error {
	if w == nil {
		return nil
	}
	now := time.Now()
	if w.ID == "" {
		w.ID = generateID()
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	_, err := p.pool.Exec(ctx,
		`INSERT INTO waf_whitelist (id, ip, note, created_by, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (id) DO UPDATE SET ip=EXCLUDED.ip, note=EXCLUDED.note, created_by=EXCLUDED.created_by, updated_at=EXCLUDED.updated_at`,
		w.ID, w.IP, w.Note, w.CreatedBy, w.CreatedAt, w.UpdatedAt)
	return err
}

func (p *Postgres) DeleteWAFWhitelist(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM waf_whitelist WHERE id=$1`, id)
	return err
}

func (p *Postgres) IsIPWhitelisted(ctx context.Context, ip string) (bool, error) {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false, nil
	}

	rows, err := p.pool.Query(ctx, `SELECT ip FROM waf_whitelist`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var entry string
		if err := rows.Scan(&entry); err != nil {
			return false, err
		}
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err == nil && cidr.Contains(parsedIP) {
				return true, nil
			}
			continue
		}
		if entry == ip {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Email verifications
func (p *Postgres) CreateEmailVerification(ctx context.Context, v *EmailVerification) error {
	if v == nil {
		return nil
	}
	if v.ID == "" {
		v.ID = generateID()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	v.Email = strings.ToLower(strings.TrimSpace(v.Email))
	_, err := p.pool.Exec(ctx,
		`INSERT INTO email_verifications (id, email, token_hash, expires_at, used_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		v.ID,
		v.Email,
		v.TokenHash,
		v.ExpiresAt,
		nullTimePtr(v.UsedAt),
		v.CreatedAt,
	)
	return err
}

func (p *Postgres) GetLatestEmailVerificationByEmail(ctx context.Context, email string) (*EmailVerification, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, email, token_hash, expires_at, used_at, created_at
		 FROM email_verifications
		 WHERE LOWER(email) = LOWER($1)
		 ORDER BY created_at DESC
		 LIMIT 1`,
		email,
	)
	var v EmailVerification
	var usedAt sql.NullTime
	if err := row.Scan(&v.ID, &v.Email, &v.TokenHash, &v.ExpiresAt, &usedAt, &v.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if usedAt.Valid {
		v.UsedAt = &usedAt.Time
	}
	return &v, nil
}

func (p *Postgres) MarkEmailVerificationUsed(ctx context.Context, id string, usedAt time.Time) (bool, error) {
	res, err := p.pool.Exec(ctx, `UPDATE email_verifications SET used_at = $1 WHERE id = $2 AND used_at IS NULL`, usedAt, id)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- API Tokens ---

func (p *Postgres) CreateAPIToken(ctx context.Context, description string, ttl time.Duration) (string, *APIToken, error) {
	token := generateToken()
	tokenHash := hashToken(token)
	id := generateID()
	now := time.Now()

	var expiresAt *time.Time
	if ttl > 0 {
		exp := now.Add(ttl)
		expiresAt = &exp
	}

	// Store only first 8 chars as prefix for display
	tokenPrefix := token[:8]

	_, err := p.pool.Exec(ctx,
		`INSERT INTO tokens (id, token_hash, token_type, description, expires_at, created_at)
		 VALUES ($1, $2, 'api', $3, $4, $5)`,
		id, tokenHash, description, nullTimePtr(expiresAt), now)
	if err != nil {
		return "", nil, err
	}

	return token, &APIToken{
		ID:          id,
		Description: description,
		TokenPrefix: tokenPrefix,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}, nil
}

func (p *Postgres) ListAPITokens(ctx context.Context) ([]*APIToken, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, description, LEFT(token_hash, 8), expires_at, created_at
		 FROM tokens WHERE token_type = 'api' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		var t APIToken
		var expiresAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.Description, &t.TokenPrefix, &expiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			t.ExpiresAt = &expiresAt.Time
		}
		tokens = append(tokens, &t)
	}
	return tokens, rows.Err()
}

func (p *Postgres) DeleteAPIToken(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM tokens WHERE id = $1 AND token_type = 'api'`, id)
	return err
}

func (p *Postgres) ValidateAPIToken(ctx context.Context, token string) (bool, error) {
	tokenHash := hashToken(token)
	var count int
	err := p.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tokens WHERE token_hash = $1 AND token_type = 'api' AND (expires_at IS NULL OR expires_at > NOW())`,
		tokenHash).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// --- Domain Blacklist ---

func (p *Postgres) CreateDomainBlacklist(ctx context.Context, b *DomainBlacklist) error {
	if b.ID == "" {
		b.ID = generateID()
	}
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now
	b.Domain = strings.ToLower(strings.TrimSpace(b.Domain))

	_, err := p.pool.Exec(ctx,
		`INSERT INTO domain_blacklist (id, domain, reason, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		b.ID, b.Domain, b.Reason, b.CreatedAt, b.UpdatedAt)
	return err
}

func (p *Postgres) ListDomainBlacklist(ctx context.Context) ([]*DomainBlacklist, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, domain, reason, created_at, updated_at
		 FROM domain_blacklist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*DomainBlacklist
	for rows.Next() {
		var b DomainBlacklist
		if err := rows.Scan(&b.ID, &b.Domain, &b.Reason, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &b)
	}
	return list, rows.Err()
}

func (p *Postgres) DeleteDomainBlacklist(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM domain_blacklist WHERE id = $1`, id)
	return err
}

func (p *Postgres) IsDomainBlacklisted(ctx context.Context, domain string) (bool, string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Check exact match first
	var reason string
	err := p.pool.QueryRow(ctx,
		`SELECT reason FROM domain_blacklist WHERE LOWER(domain) = $1`,
		domain).Scan(&reason)
	if err == nil {
		return true, reason, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, "", err
	}

	// Check wildcard patterns (e.g., *.example.com)
	rows, err := p.pool.Query(ctx,
		`SELECT domain, reason FROM domain_blacklist WHERE domain LIKE '*.%'`)
	if err != nil {
		return false, "", err
	}
	defer rows.Close()

	for rows.Next() {
		var pattern, r string
		if err := rows.Scan(&pattern, &r); err != nil {
			return false, "", err
		}
		// Convert *.example.com to .example.com suffix check
		suffix := strings.TrimPrefix(pattern, "*")
		if strings.HasSuffix(domain, suffix) || domain == strings.TrimPrefix(suffix, ".") {
			return true, r, nil
		}
	}

	return false, "", rows.Err()
}

func (p *Postgres) ListGlobalTemplateOverrides(ctx context.Context) ([]*GlobalTemplateOverride, error) {
	rows, err := p.pool.Query(ctx, `SELECT key, content, updated_at FROM global_templates ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*GlobalTemplateOverride
	for rows.Next() {
		var t GlobalTemplateOverride
		if err := rows.Scan(&t.Key, &t.Content, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &t)
	}
	return list, rows.Err()
}

func (p *Postgres) GetGlobalTemplateOverride(ctx context.Context, key string) (*GlobalTemplateOverride, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	var t GlobalTemplateOverride
	err := p.pool.QueryRow(ctx, `SELECT key, content, updated_at FROM global_templates WHERE key = $1`, key).
		Scan(&t.Key, &t.Content, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (p *Postgres) UpsertGlobalTemplateOverride(ctx context.Context, t *GlobalTemplateOverride) error {
	if t == nil {
		return nil
	}
	key := strings.TrimSpace(t.Key)
	if key == "" {
		return nil
	}
	now := time.Now()
	_, err := p.pool.Exec(ctx,
		`INSERT INTO global_templates (key, content, updated_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT(key) DO UPDATE SET content = EXCLUDED.content, updated_at = EXCLUDED.updated_at`,
		key, t.Content, now)
	return err
}

func (p *Postgres) DeleteGlobalTemplateOverride(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM global_templates WHERE key = $1`, key)
	return err
}

func (p *Postgres) ListProductGroups(ctx context.Context) ([]*ProductGroup, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, name, sort, description, created_at, updated_at FROM product_groups ORDER BY sort ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*ProductGroup
	for rows.Next() {
		var g ProductGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Sort, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &g)
	}
	return list, rows.Err()
}

func (p *Postgres) GetProductGroup(ctx context.Context, id string) (*ProductGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var g ProductGroup
	err := p.pool.QueryRow(ctx, `SELECT id, name, sort, description, created_at, updated_at FROM product_groups WHERE id = $1`, id).
		Scan(&g.ID, &g.Name, &g.Sort, &g.Description, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (p *Postgres) CreateProductGroup(ctx context.Context, g *ProductGroup) error {
	if g == nil {
		return nil
	}
	now := time.Now()
	cp := *g
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = generateID()
	}
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Description = strings.TrimSpace(cp.Description)
	if cp.Sort == 0 {
		cp.Sort = 100
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	_, err := p.pool.Exec(ctx,
		`INSERT INTO product_groups (id, name, sort, description, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		cp.ID, cp.Name, cp.Sort, cp.Description, cp.CreatedAt, cp.UpdatedAt)
	if err == nil {
		*g = cp
	}
	return err
}

func (p *Postgres) UpdateProductGroup(ctx context.Context, g *ProductGroup) error {
	if g == nil {
		return nil
	}
	now := time.Now()
	cp := *g
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Description = strings.TrimSpace(cp.Description)
	if cp.Sort == 0 {
		cp.Sort = 100
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	ct, err := p.pool.Exec(ctx,
		`UPDATE product_groups SET name = $2, sort = $3, description = $4, updated_at = $5 WHERE id = $1`,
		cp.ID, cp.Name, cp.Sort, cp.Description, cp.UpdatedAt)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	*g = cp
	return nil
}

func (p *Postgres) DeleteProductGroup(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM product_groups WHERE id = $1`, id)
	return err
}

func (p *Postgres) ListProducts(ctx context.Context) ([]*Product, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, name, slug, description, group_id, sort, region, line_group_id,
		       monthly_traffic_bytes, bandwidth_bps, conn_limit,
		       domain_limit, primary_domain_limit, http_port_limit, stream_port_limit, non_std_port_limit,
		       websocket, custom_cc_rules, http3, l2_origin, cc_protection, ddos_protection,
		       price_cents, price_month_cents, price_quarter_cents, price_year_cents,
		       currency, enabled, created_at, updated_at
		FROM products ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Product
	for rows.Next() {
		var pr Product
		var monthly, bandwidth, conn sql.NullInt64
		var domain, primaryDomain, httpPort, streamPort, nonStdPort sql.NullInt32
		if err := rows.Scan(
			&pr.ID, &pr.Name, &pr.Slug, &pr.Description, &pr.GroupID, &pr.Sort, &pr.Region, &pr.LineGroupID,
			&monthly, &bandwidth, &conn,
			&domain, &primaryDomain, &httpPort, &streamPort, &nonStdPort,
			&pr.Websocket, &pr.CustomCCRules, &pr.HTTP3, &pr.L2Origin, &pr.CCProtection, &pr.DDoSProtection,
			&pr.PriceCents, &pr.PriceMonthCents, &pr.PriceQuarterCents, &pr.PriceYearCents,
			&pr.Currency, &pr.Enabled, &pr.CreatedAt, &pr.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if monthly.Valid {
			v := monthly.Int64
			pr.MonthlyTrafficBytes = &v
		}
		if bandwidth.Valid {
			v := bandwidth.Int64
			pr.BandwidthBps = &v
		}
		if conn.Valid {
			v := conn.Int64
			pr.ConnLimit = &v
		}
		if domain.Valid {
			v := int32(domain.Int32)
			pr.DomainLimit = &v
		}
		if primaryDomain.Valid {
			v := int32(primaryDomain.Int32)
			pr.PrimaryDomainLimit = &v
		}
		if httpPort.Valid {
			v := int32(httpPort.Int32)
			pr.HTTPPortLimit = &v
		}
		if streamPort.Valid {
			v := int32(streamPort.Int32)
			pr.StreamPortLimit = &v
		}
		if nonStdPort.Valid {
			v := int32(nonStdPort.Int32)
			pr.NonStdPortLimit = &v
		}
		list = append(list, &pr)
	}
	return list, rows.Err()
}

func (p *Postgres) GetProduct(ctx context.Context, id string) (*Product, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var pr Product
	var monthly, bandwidth, conn sql.NullInt64
	var domain, primaryDomain, httpPort, streamPort, nonStdPort sql.NullInt32
	err := p.pool.QueryRow(ctx, `
		SELECT id, name, slug, description, group_id, sort, region, line_group_id,
		       monthly_traffic_bytes, bandwidth_bps, conn_limit,
		       domain_limit, primary_domain_limit, http_port_limit, stream_port_limit, non_std_port_limit,
		       websocket, custom_cc_rules, http3, l2_origin, cc_protection, ddos_protection,
		       price_cents, price_month_cents, price_quarter_cents, price_year_cents,
		       currency, enabled, created_at, updated_at
		FROM products WHERE id = $1`, id).
		Scan(
			&pr.ID, &pr.Name, &pr.Slug, &pr.Description, &pr.GroupID, &pr.Sort, &pr.Region, &pr.LineGroupID,
			&monthly, &bandwidth, &conn,
			&domain, &primaryDomain, &httpPort, &streamPort, &nonStdPort,
			&pr.Websocket, &pr.CustomCCRules, &pr.HTTP3, &pr.L2Origin, &pr.CCProtection, &pr.DDoSProtection,
			&pr.PriceCents, &pr.PriceMonthCents, &pr.PriceQuarterCents, &pr.PriceYearCents,
			&pr.Currency, &pr.Enabled, &pr.CreatedAt, &pr.UpdatedAt,
		)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if monthly.Valid {
		v := monthly.Int64
		pr.MonthlyTrafficBytes = &v
	}
	if bandwidth.Valid {
		v := bandwidth.Int64
		pr.BandwidthBps = &v
	}
	if conn.Valid {
		v := conn.Int64
		pr.ConnLimit = &v
	}
	if domain.Valid {
		v := int32(domain.Int32)
		pr.DomainLimit = &v
	}
	if primaryDomain.Valid {
		v := int32(primaryDomain.Int32)
		pr.PrimaryDomainLimit = &v
	}
	if httpPort.Valid {
		v := int32(httpPort.Int32)
		pr.HTTPPortLimit = &v
	}
	if streamPort.Valid {
		v := int32(streamPort.Int32)
		pr.StreamPortLimit = &v
	}
	if nonStdPort.Valid {
		v := int32(nonStdPort.Int32)
		pr.NonStdPortLimit = &v
	}
	return &pr, nil
}

func (p *Postgres) CreateProduct(ctx context.Context, pr *Product) error {
	if pr == nil {
		return nil
	}
	now := time.Now()
	cp := *pr
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = generateID()
	}
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Slug = strings.ToLower(strings.TrimSpace(cp.Slug))
	cp.Description = strings.TrimSpace(cp.Description)
	cp.GroupID = strings.TrimSpace(cp.GroupID)
	cp.Region = strings.TrimSpace(cp.Region)
	cp.LineGroupID = strings.TrimSpace(cp.LineGroupID)
	if cp.Sort == 0 {
		cp.Sort = 100
	}
	cp.Currency = strings.ToUpper(strings.TrimSpace(cp.Currency))
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	if cp.PriceMonthCents == 0 && cp.PriceCents != 0 {
		cp.PriceMonthCents = cp.PriceCents
	}
	if cp.PriceCents == 0 && cp.PriceMonthCents != 0 {
		cp.PriceCents = cp.PriceMonthCents
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	_, err := p.pool.Exec(ctx,
		`INSERT INTO products (
			id, name, slug, description, group_id, sort, region, line_group_id,
			monthly_traffic_bytes, bandwidth_bps, conn_limit,
			domain_limit, primary_domain_limit, http_port_limit, stream_port_limit, non_std_port_limit,
			websocket, custom_cc_rules, http3, l2_origin, cc_protection, ddos_protection,
			price_cents, price_month_cents, price_quarter_cents, price_year_cents,
			currency, enabled, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,
			$9,$10,$11,
			$12,$13,$14,$15,$16,
			$17,$18,$19,$20,$21,$22,
			$23,$24,$25,$26,
			$27,$28,$29,$30
		)`,
		cp.ID, cp.Name, cp.Slug, cp.Description, cp.GroupID, cp.Sort, cp.Region, cp.LineGroupID,
		cp.MonthlyTrafficBytes, cp.BandwidthBps, cp.ConnLimit,
		cp.DomainLimit, cp.PrimaryDomainLimit, cp.HTTPPortLimit, cp.StreamPortLimit, cp.NonStdPortLimit,
		cp.Websocket, cp.CustomCCRules, cp.HTTP3, cp.L2Origin, cp.CCProtection, cp.DDoSProtection,
		cp.PriceCents, cp.PriceMonthCents, cp.PriceQuarterCents, cp.PriceYearCents,
		cp.Currency, cp.Enabled, cp.CreatedAt, cp.UpdatedAt,
	)
	if err == nil {
		*pr = cp
	}
	return err
}

func (p *Postgres) UpdateProduct(ctx context.Context, pr *Product) error {
	if pr == nil {
		return nil
	}
	now := time.Now()
	cp := *pr
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Slug = strings.ToLower(strings.TrimSpace(cp.Slug))
	cp.Description = strings.TrimSpace(cp.Description)
	cp.GroupID = strings.TrimSpace(cp.GroupID)
	cp.Region = strings.TrimSpace(cp.Region)
	cp.LineGroupID = strings.TrimSpace(cp.LineGroupID)
	if cp.Sort == 0 {
		cp.Sort = 100
	}
	cp.Currency = strings.ToUpper(strings.TrimSpace(cp.Currency))
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	if cp.PriceMonthCents == 0 && cp.PriceCents != 0 {
		cp.PriceMonthCents = cp.PriceCents
	}
	if cp.PriceCents == 0 && cp.PriceMonthCents != 0 {
		cp.PriceCents = cp.PriceMonthCents
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	ct, err := p.pool.Exec(ctx,
		`UPDATE products SET
			name = $2, slug = $3, description = $4, group_id = $5, sort = $6, region = $7, line_group_id = $8,
			monthly_traffic_bytes = $9, bandwidth_bps = $10, conn_limit = $11,
			domain_limit = $12, primary_domain_limit = $13, http_port_limit = $14, stream_port_limit = $15, non_std_port_limit = $16,
			websocket = $17, custom_cc_rules = $18, http3 = $19, l2_origin = $20, cc_protection = $21, ddos_protection = $22,
			price_cents = $23, price_month_cents = $24, price_quarter_cents = $25, price_year_cents = $26,
			currency = $27, enabled = $28, updated_at = $29
		 WHERE id = $1`,
		cp.ID, cp.Name, cp.Slug, cp.Description, cp.GroupID, cp.Sort, cp.Region, cp.LineGroupID,
		cp.MonthlyTrafficBytes, cp.BandwidthBps, cp.ConnLimit,
		cp.DomainLimit, cp.PrimaryDomainLimit, cp.HTTPPortLimit, cp.StreamPortLimit, cp.NonStdPortLimit,
		cp.Websocket, cp.CustomCCRules, cp.HTTP3, cp.L2Origin, cp.CCProtection, cp.DDoSProtection,
		cp.PriceCents, cp.PriceMonthCents, cp.PriceQuarterCents, cp.PriceYearCents,
		cp.Currency, cp.Enabled, cp.UpdatedAt,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	*pr = cp
	return nil
}

func (p *Postgres) DeleteProduct(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM orders WHERE product_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM products WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ListOrders(ctx context.Context, userID string) ([]*Order, error) {
	userID = strings.TrimSpace(userID)
	rows, err := p.pool.Query(ctx, `
		SELECT id, user_id, product_id, product_name, amount_cents, currency, status,
		       period, quantity, starts_at, ends_at, paid_at, note,
		       created_at, updated_at
		FROM orders WHERE ($1 = '' OR user_id = $1) ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Order
	for rows.Next() {
		var o Order
		var startsAt, endsAt, paidAt sql.NullTime
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.ProductID, &o.ProductName, &o.AmountCents, &o.Currency, &o.Status,
			&o.Period, &o.Quantity, &startsAt, &endsAt, &paidAt, &o.Note,
			&o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if startsAt.Valid {
			v := startsAt.Time
			o.StartsAt = &v
		}
		if endsAt.Valid {
			v := endsAt.Time
			o.EndsAt = &v
		}
		if paidAt.Valid {
			v := paidAt.Time
			o.PaidAt = &v
		}
		list = append(list, &o)
	}
	return list, rows.Err()
}

func (p *Postgres) GetOrder(ctx context.Context, id string) (*Order, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var o Order
	var startsAt, endsAt, paidAt sql.NullTime
	err := p.pool.QueryRow(ctx, `
		SELECT id, user_id, product_id, product_name, amount_cents, currency, status,
		       period, quantity, starts_at, ends_at, paid_at, note,
		       created_at, updated_at
		FROM orders WHERE id = $1`, id).
		Scan(
			&o.ID, &o.UserID, &o.ProductID, &o.ProductName, &o.AmountCents, &o.Currency, &o.Status,
			&o.Period, &o.Quantity, &startsAt, &endsAt, &paidAt, &o.Note,
			&o.CreatedAt, &o.UpdatedAt,
		)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if startsAt.Valid {
		v := startsAt.Time
		o.StartsAt = &v
	}
	if endsAt.Valid {
		v := endsAt.Time
		o.EndsAt = &v
	}
	if paidAt.Valid {
		v := paidAt.Time
		o.PaidAt = &v
	}
	return &o, nil
}

func (p *Postgres) CreateOrder(ctx context.Context, o *Order) error {
	if o == nil {
		return nil
	}
	now := time.Now()
	cp := *o
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = generateID()
	}
	cp.UserID = strings.TrimSpace(cp.UserID)
	cp.ProductID = strings.TrimSpace(cp.ProductID)
	cp.ProductName = strings.TrimSpace(cp.ProductName)
	cp.Status = strings.ToLower(strings.TrimSpace(cp.Status))
	if cp.Status == "" {
		cp.Status = "pending"
	}
	cp.Period = strings.ToLower(strings.TrimSpace(cp.Period))
	if cp.Period == "" {
		cp.Period = "month"
	}
	if cp.Quantity <= 0 {
		cp.Quantity = 1
	}
	cp.Note = strings.TrimSpace(cp.Note)
	cp.Currency = strings.ToUpper(strings.TrimSpace(cp.Currency))
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	_, err := p.pool.Exec(ctx,
		`INSERT INTO orders (
			id, user_id, product_id, product_name, amount_cents, currency, status,
			period, quantity, starts_at, ends_at, paid_at, note,
			created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,
			$8,$9,$10,$11,$12,$13,
			$14,$15
		)`,
		cp.ID, cp.UserID, cp.ProductID, cp.ProductName, cp.AmountCents, cp.Currency, cp.Status,
		cp.Period, cp.Quantity, cp.StartsAt, cp.EndsAt, cp.PaidAt, cp.Note,
		cp.CreatedAt, cp.UpdatedAt,
	)
	if err == nil {
		*o = cp
	}
	return err
}

func (p *Postgres) UpdateOrder(ctx context.Context, o *Order) error {
	if o == nil {
		return nil
	}
	now := time.Now()
	cp := *o
	cp.UserID = strings.TrimSpace(cp.UserID)
	cp.ProductID = strings.TrimSpace(cp.ProductID)
	cp.ProductName = strings.TrimSpace(cp.ProductName)
	cp.Status = strings.ToLower(strings.TrimSpace(cp.Status))
	if cp.Status == "" {
		cp.Status = "pending"
	}
	cp.Period = strings.ToLower(strings.TrimSpace(cp.Period))
	if cp.Period == "" {
		cp.Period = "month"
	}
	if cp.Quantity <= 0 {
		cp.Quantity = 1
	}
	cp.Note = strings.TrimSpace(cp.Note)
	cp.Currency = strings.ToUpper(strings.TrimSpace(cp.Currency))
	if cp.Currency == "" {
		cp.Currency = "CNY"
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	ct, err := p.pool.Exec(ctx,
		`UPDATE orders SET
			user_id = $2, product_id = $3, product_name = $4,
			amount_cents = $5, currency = $6, status = $7,
			period = $8, quantity = $9, starts_at = $10, ends_at = $11, paid_at = $12, note = $13,
			updated_at = $14
		 WHERE id = $1`,
		cp.ID, cp.UserID, cp.ProductID, cp.ProductName,
		cp.AmountCents, cp.Currency, cp.Status,
		cp.Period, cp.Quantity, cp.StartsAt, cp.EndsAt, cp.PaidAt, cp.Note,
		cp.UpdatedAt)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	*o = cp
	return nil
}

func (p *Postgres) DeleteOrder(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, id)
	return err
}

// --- User traffic tracking ---

func (p *Postgres) GetUserTraffic(ctx context.Context, userID, month string) (int64, error) {
	var total int64
	err := p.pool.QueryRow(ctx, `SELECT bytes_total FROM user_traffic WHERE user_id = $1 AND month = $2`, userID, month).Scan(&total)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return total, nil
}

func (p *Postgres) IncrementUserTraffic(ctx context.Context, userID, month string, bytes int64) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO user_traffic (user_id, month, bytes_total, last_updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (user_id, month) DO UPDATE SET bytes_total = user_traffic.bytes_total + $3, last_updated_at = NOW()`,
		userID, month, bytes)
	return err
}

// --- Cluster operations ---

func (p *Postgres) CreateCluster(ctx context.Context, c *Cluster) error {
	if c == nil {
		return nil
	}
	now := time.Now()
	cp := *c
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = generateID()
	}
	cp.Name = strings.TrimSpace(cp.Name)
	cp.DNSZone = strings.TrimSpace(cp.DNSZone)
	cp.DNSMode = strings.TrimSpace(cp.DNSMode)
	cp.CNAME = strings.TrimSpace(cp.CNAME)
	cp.Description = strings.TrimSpace(cp.Description)
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	_, err := p.pool.Exec(ctx,
		`INSERT INTO clusters (id, name, dns_zone, dns_mode, cname, description, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		cp.ID, cp.Name, cp.DNSZone, cp.DNSMode, cp.CNAME, cp.Description, cp.Enabled, cp.CreatedAt, cp.UpdatedAt)
	if err == nil {
		*c = cp
	}
	return err
}

func (p *Postgres) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var c Cluster
	err := p.pool.QueryRow(ctx,
		`SELECT id, name, dns_zone, dns_mode, cname, description, enabled, created_at, updated_at FROM clusters WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.DNSZone, &c.DNSMode, &c.CNAME, &c.Description, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *Postgres) ListClusters(ctx context.Context) ([]*Cluster, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, dns_zone, dns_mode, cname, description, enabled, created_at, updated_at FROM clusters ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Cluster
	for rows.Next() {
		var c Cluster
		if err := rows.Scan(&c.ID, &c.Name, &c.DNSZone, &c.DNSMode, &c.CNAME, &c.Description, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &c)
	}
	return list, rows.Err()
}

func (p *Postgres) UpdateCluster(ctx context.Context, c *Cluster) error {
	if c == nil {
		return nil
	}
	now := time.Now()
	cp := *c
	cp.Name = strings.TrimSpace(cp.Name)
	cp.DNSZone = strings.TrimSpace(cp.DNSZone)
	cp.DNSMode = strings.TrimSpace(cp.DNSMode)
	cp.CNAME = strings.TrimSpace(cp.CNAME)
	cp.Description = strings.TrimSpace(cp.Description)
	cp.UpdatedAt = now
	ct, err := p.pool.Exec(ctx,
		`UPDATE clusters SET name = $2, dns_zone = $3, dns_mode = $4, cname = $5, description = $6, enabled = $7, updated_at = $8 WHERE id = $1`,
		cp.ID, cp.Name, cp.DNSZone, cp.DNSMode, cp.CNAME, cp.Description, cp.Enabled, cp.UpdatedAt)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	*c = cp
	return nil
}

func (p *Postgres) DeleteCluster(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, id)
	return err
}
