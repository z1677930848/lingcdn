package server

// Admin settings: the single /api/settings endpoint (admin-only) plus the
// helpers that layer default values / env-sourced config over whatever is
// stored in the DB. Write paths treat empty SMTP/ES secret strings as
// "preserve existing" so the UI can round-trip without exposing plaintext
// secrets.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) handleSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		withTimeout, cancel := store.WithTimeout(ctx)
		defer cancel()
		settings, err := s.store.GetSettings(withTimeout)
		if err != nil {
			writeInternalError(w, "get settings", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"settings": s.applySettingsDefaults(settings)})
	case http.MethodPut:
		type settingsPatch struct {
			SystemName                *string `json:"system_name"`
			FooterLinks               *string `json:"footer_links"`
			FooterCopyright           *string `json:"footer_copyright"`
			Favicon                   *string `json:"favicon"`
			Logo                      *string `json:"logo"`
			SMTPHost                  *string `json:"smtp_host"`
			SMTPPort                  *int    `json:"smtp_port"`
			SMTPUsername              *string `json:"smtp_username"`
			SMTPPassword              *string `json:"smtp_password"`
			SMTPFrom                  *string `json:"smtp_from"`
			SMTPFromName              *string `json:"smtp_from_name"`
			ElasticsearchURL          *string `json:"elasticsearch_url"`
			ElasticsearchUser         *string `json:"elasticsearch_user"`
			ElasticsearchPass         *string `json:"elasticsearch_pass"`
			ElasticsearchIndex        *string `json:"elasticsearch_index"`
			ElasticsearchTSField      *string `json:"elasticsearch_ts_field"`
			ElasticsearchDomainField  *string `json:"elasticsearch_domain_field"`
			ElasticsearchBytesField   *string `json:"elasticsearch_bytes_field"`
			SalesEmail                *string `json:"sales_email"`
			SupportEmail              *string `json:"support_email"`
			RegisterEnabled           *bool   `json:"register_enabled"`
			UpgradeChannel            *string `json:"upgrade_channel"`
			NotifyNewBuild            *bool   `json:"notify_new_build"`
			RegisterEmailVerification *bool   `json:"register_email_verification"`
			EmailEnabled              *bool   `json:"email_enabled"`
			DingtalkEnabled           *bool   `json:"dingtalk_enabled"`
			DingtalkWebhook           *string `json:"dingtalk_webhook"`
			WechatEnabled             *bool   `json:"wechat_enabled"`
			WechatWebhook             *string `json:"wechat_webhook"`
			FeishuEnabled             *bool   `json:"feishu_enabled"`
			FeishuWebhook             *string `json:"feishu_webhook"`
			NotifyNodeResource        *bool   `json:"notify_node_resource"`
			NotifyNodeMonitor         *bool   `json:"notify_node_monitor"`
			NotifyTicketReply         *bool   `json:"notify_ticket_reply"`
			NotifyInterval            *int    `json:"notify_interval"`
			ThresholdCPU              *int    `json:"threshold_cpu"`
			ThresholdMemory           *int    `json:"threshold_memory"`
			ThresholdDisk             *int    `json:"threshold_disk"`
			ThresholdBandwidthUp      *int    `json:"threshold_bandwidth_up"`
			ThresholdBandwidthDown    *int    `json:"threshold_bandwidth_down"`
			RetentionSystemLogs       *int    `json:"retention_system_logs"`
			RetentionESLogs           *int    `json:"retention_es_logs"`
			RetentionWafBans          *int    `json:"retention_waf_bans"`
			RetentionUpgradeLogs      *int    `json:"retention_upgrade_logs"`
		}
		var req settingsPatch
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		withTimeout, cancel := store.WithTimeout(ctx)
		defer cancel()
		existing, _ := s.store.GetSettings(withTimeout)
		if existing == nil {
			existing = &store.Settings{ID: "default"}
		}

		next := *existing
		if strings.TrimSpace(next.ID) == "" {
			next.ID = "default"
		}
		if req.SystemName != nil {
			next.SystemName = strings.TrimSpace(*req.SystemName)
		}
		if req.FooterLinks != nil {
			next.FooterLinks = strings.TrimSpace(*req.FooterLinks)
		}
		if req.FooterCopyright != nil {
			next.FooterCopyright = strings.TrimSpace(*req.FooterCopyright)
		}
		if req.Favicon != nil {
			next.Favicon = strings.TrimSpace(*req.Favicon)
		}
		if req.Logo != nil {
			next.Logo = strings.TrimSpace(*req.Logo)
		}
		if req.SMTPHost != nil {
			next.SMTPHost = strings.TrimSpace(*req.SMTPHost)
		}
		if req.SMTPPort != nil {
			next.SMTPPort = *req.SMTPPort
		}
		if req.SMTPUsername != nil {
			next.SMTPUsername = strings.TrimSpace(*req.SMTPUsername)
		}
		if req.SMTPPassword != nil {
			// Empty SMTP password from UI means "don't change" so admins can edit
			// other fields without having to re-enter the password.
			if strings.TrimSpace(*req.SMTPPassword) == "" && existing != nil {
				next.SMTPPassword = existing.SMTPPassword
			} else {
				next.SMTPPassword = *req.SMTPPassword
			}
		}
		if req.SMTPFrom != nil {
			next.SMTPFrom = strings.TrimSpace(*req.SMTPFrom)
		}
		if req.SMTPFromName != nil {
			next.SMTPFromName = strings.TrimSpace(*req.SMTPFromName)
		}
		if req.ElasticsearchURL != nil {
			trimmed := strings.TrimSpace(*req.ElasticsearchURL)
			// Empty string disables ES integration; only validate non-empty values.
			// Reject early when the URL is syntactically malformed or points at
			// an unspecified address (0.0.0.0 / ::), because those values cause
			// the node-install Filebeat tail to ship a useless `--es_host
			// 0.0.0.0` flag, which silently breaks log delivery on every node
			// installed from that point on. We intentionally allow loopback
			// (127.0.0.1 / localhost) here so single-host installs keep working
			// for control-plane → ES queries; buildNodeInstallFilebeatTail
			// separately refuses to forward loopback to remote nodes.
			if trimmed != "" {
				if _, _, _, perr := parseESEndpoint(trimmed); perr != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error": "无效的 Elasticsearch URL：" + perr.Error() + "（请填写如 http://es.example.com:9200 或 https://1.2.3.4:9200 的真实可拨号地址；请勿使用 0.0.0.0/:: 这类绑定占位符）",
					})
					return
				}
			}
			next.ElasticsearchURL = trimmed
		}
		if req.ElasticsearchUser != nil {
			next.ElasticsearchUser = strings.TrimSpace(*req.ElasticsearchUser)
		}
		if req.ElasticsearchPass != nil {
			if strings.TrimSpace(*req.ElasticsearchPass) == "" && existing != nil {
				next.ElasticsearchPass = existing.ElasticsearchPass
			} else {
				next.ElasticsearchPass = *req.ElasticsearchPass
			}
		}
		if req.ElasticsearchIndex != nil {
			next.ElasticsearchIndex = strings.TrimSpace(*req.ElasticsearchIndex)
		}
		if req.ElasticsearchTSField != nil {
			next.ElasticsearchTSField = strings.TrimSpace(*req.ElasticsearchTSField)
		}
		if req.ElasticsearchDomainField != nil {
			next.ElasticsearchDomainField = strings.TrimSpace(*req.ElasticsearchDomainField)
		}
		if req.ElasticsearchBytesField != nil {
			next.ElasticsearchBytesField = strings.TrimSpace(*req.ElasticsearchBytesField)
		}
		if req.SalesEmail != nil {
			next.SalesEmail = strings.TrimSpace(*req.SalesEmail)
		}
		if req.SupportEmail != nil {
			next.SupportEmail = strings.TrimSpace(*req.SupportEmail)
		}
		if req.RegisterEnabled != nil {
			next.RegisterEnabled = *req.RegisterEnabled
		}
		if req.NotifyNewBuild != nil {
			next.NotifyNewBuild = *req.NotifyNewBuild
		}
		if req.RegisterEmailVerification != nil {
			next.RegisterEmailVerification = *req.RegisterEmailVerification
		}
		if req.EmailEnabled != nil {
			next.EmailEnabled = *req.EmailEnabled
		}
		if req.DingtalkEnabled != nil {
			next.DingtalkEnabled = *req.DingtalkEnabled
		}
		if req.DingtalkWebhook != nil {
			next.DingtalkWebhook = strings.TrimSpace(*req.DingtalkWebhook)
		}
		if req.WechatEnabled != nil {
			next.WechatEnabled = *req.WechatEnabled
		}
		if req.WechatWebhook != nil {
			next.WechatWebhook = strings.TrimSpace(*req.WechatWebhook)
		}
		if req.FeishuEnabled != nil {
			next.FeishuEnabled = *req.FeishuEnabled
		}
		if req.FeishuWebhook != nil {
			next.FeishuWebhook = strings.TrimSpace(*req.FeishuWebhook)
		}
		if req.NotifyNodeResource != nil {
			next.NotifyNodeResource = *req.NotifyNodeResource
		}
		if req.NotifyNodeMonitor != nil {
			next.NotifyNodeMonitor = *req.NotifyNodeMonitor
		}
		if req.NotifyTicketReply != nil {
			next.NotifyTicketReply = *req.NotifyTicketReply
		}
		if req.NotifyInterval != nil {
			next.NotifyInterval = *req.NotifyInterval
		}
		if req.ThresholdCPU != nil {
			next.ThresholdCPU = *req.ThresholdCPU
		}
		if req.ThresholdMemory != nil {
			next.ThresholdMemory = *req.ThresholdMemory
		}
		if req.ThresholdDisk != nil {
			next.ThresholdDisk = *req.ThresholdDisk
		}
		if req.ThresholdBandwidthUp != nil {
			next.ThresholdBandwidthUp = *req.ThresholdBandwidthUp
		}
		if req.ThresholdBandwidthDown != nil {
			next.ThresholdBandwidthDown = *req.ThresholdBandwidthDown
		}
		if req.RetentionSystemLogs != nil {
			next.RetentionSystemLogs = *req.RetentionSystemLogs
		}
		if req.RetentionESLogs != nil {
			next.RetentionESLogs = *req.RetentionESLogs
		}
		if req.RetentionWafBans != nil {
			next.RetentionWafBans = *req.RetentionWafBans
		}
		if req.RetentionUpgradeLogs != nil {
			next.RetentionUpgradeLogs = *req.RetentionUpgradeLogs
		}

		if req.UpgradeChannel != nil {
			channel := strings.TrimSpace(*req.UpgradeChannel)
			if channel != "" {
				normalized, err := normalizeUpgradeChannel(channel)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
					return
				}
				next.UpgradeChannel = normalized
			}
		}

		if next.SMTPPort <= 0 {
			next.SMTPPort = 587
		}

		next.UpdatedAt = time.Now()
		settings := &next
		if err := s.store.UpdateSettings(withTimeout, settings); err != nil {
			writeInternalError(w, "update settings", err)
			return
		}
		// system_name is baked into edge-node templates (error pages, WAF
		// pages, captcha hints) at compile time. When it changes we have
		// to trigger a fresh compile + push so connected nodes pick it up
		// immediately rather than waiting for the next unrelated config
		// mutation. Other settings (SMTP, ES, retention) do not affect the
		// node payload so we skip the push to avoid unnecessary churn.
		if s.publisher != nil && strings.TrimSpace(existing.SystemName) != strings.TrimSpace(settings.SystemName) {
			go func() {
				pubCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := s.publisher.Publish(pubCtx, "", nil); err != nil {
					log.Warn().Err(err).Msg("publish after system_name change failed")
				}
			}()
		}
		// Best-effort push of cdn-access / cdn-error index templates so
		// auto-created daily indices inherit our mappings. Failures are
		// logged inside pushESIndexTemplates and never block the save.
		applied := pushESIndexTemplates(ctx, settings)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "templates_applied": applied})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// applySettingsDefaults layers defaults → env-sourced cfg values → DB settings
// (in order of increasing precedence). Empty strings in DB settings fall back
// to defaults so callers never observe "" in required fields. Non-string fields
// like RegisterEnabled / NotifyNewBuild are passed through as-is.
func (s *Servers) applySettingsDefaults(settings *store.Settings) *store.Settings {
	defaults := store.DefaultSettings()
	if strings.TrimSpace(s.cfg.SMTPHost) != "" {
		defaults.SMTPHost = s.cfg.SMTPHost
	}
	if s.cfg.SMTPPort > 0 {
		defaults.SMTPPort = s.cfg.SMTPPort
	}
	if strings.TrimSpace(s.cfg.SMTPUser) != "" {
		defaults.SMTPUsername = s.cfg.SMTPUser
	}
	if strings.TrimSpace(s.cfg.SMTPPass) != "" {
		defaults.SMTPPassword = s.cfg.SMTPPass
	}
	if strings.TrimSpace(s.cfg.SMTPFrom) != "" {
		defaults.SMTPFrom = s.cfg.SMTPFrom
	}
	if strings.TrimSpace(s.cfg.ElasticsearchURL) != "" {
		defaults.ElasticsearchURL = s.cfg.ElasticsearchURL
	}
	if strings.TrimSpace(s.cfg.ElasticsearchUser) != "" {
		defaults.ElasticsearchUser = s.cfg.ElasticsearchUser
	}
	if strings.TrimSpace(s.cfg.ElasticsearchPass) != "" {
		defaults.ElasticsearchPass = s.cfg.ElasticsearchPass
	}
	if strings.TrimSpace(s.cfg.ElasticsearchIndex) != "" {
		defaults.ElasticsearchIndex = s.cfg.ElasticsearchIndex
	}
	if strings.TrimSpace(s.cfg.ElasticsearchTSField) != "" {
		defaults.ElasticsearchTSField = s.cfg.ElasticsearchTSField
	}
	if strings.TrimSpace(s.cfg.ElasticsearchDomainField) != "" {
		defaults.ElasticsearchDomainField = s.cfg.ElasticsearchDomainField
	}
	if strings.TrimSpace(s.cfg.ElasticsearchBytesField) != "" {
		defaults.ElasticsearchBytesField = s.cfg.ElasticsearchBytesField
	}
	if settings == nil {
		return defaults
	}
	out := *defaults
	if strings.TrimSpace(settings.ID) != "" {
		out.ID = settings.ID
	}
	if strings.TrimSpace(settings.SystemName) != "" {
		out.SystemName = settings.SystemName
	}
	out.FooterLinks = settings.FooterLinks
	if strings.TrimSpace(settings.FooterCopyright) != "" {
		out.FooterCopyright = settings.FooterCopyright
	}
	out.Favicon = settings.Favicon
	out.Logo = settings.Logo
	if strings.TrimSpace(settings.SMTPHost) != "" {
		out.SMTPHost = settings.SMTPHost
	}
	if settings.SMTPPort > 0 {
		out.SMTPPort = settings.SMTPPort
	}
	if strings.TrimSpace(settings.SMTPUsername) != "" {
		out.SMTPUsername = settings.SMTPUsername
	}
	if strings.TrimSpace(settings.SMTPPassword) != "" {
		out.SMTPPassword = settings.SMTPPassword
	}
	if strings.TrimSpace(settings.SMTPFrom) != "" {
		out.SMTPFrom = settings.SMTPFrom
	}
	if strings.TrimSpace(settings.SMTPFromName) != "" {
		out.SMTPFromName = settings.SMTPFromName
	}
	if strings.TrimSpace(settings.ElasticsearchURL) != "" {
		out.ElasticsearchURL = settings.ElasticsearchURL
	}
	if strings.TrimSpace(settings.ElasticsearchUser) != "" {
		out.ElasticsearchUser = settings.ElasticsearchUser
	}
	if strings.TrimSpace(settings.ElasticsearchPass) != "" {
		out.ElasticsearchPass = settings.ElasticsearchPass
	}
	if strings.TrimSpace(settings.ElasticsearchIndex) != "" {
		out.ElasticsearchIndex = settings.ElasticsearchIndex
	}
	if strings.TrimSpace(settings.ElasticsearchTSField) != "" {
		out.ElasticsearchTSField = settings.ElasticsearchTSField
	}
	if strings.TrimSpace(settings.ElasticsearchDomainField) != "" {
		out.ElasticsearchDomainField = settings.ElasticsearchDomainField
	}
	if strings.TrimSpace(settings.ElasticsearchBytesField) != "" {
		out.ElasticsearchBytesField = settings.ElasticsearchBytesField
	}
	if strings.TrimSpace(settings.SalesEmail) != "" {
		out.SalesEmail = settings.SalesEmail
	}
	if strings.TrimSpace(settings.SupportEmail) != "" {
		out.SupportEmail = settings.SupportEmail
	}
	out.RegisterEnabled = settings.RegisterEnabled
	if strings.TrimSpace(settings.UpgradeChannel) != "" {
		if normalized, err := normalizeUpgradeChannel(settings.UpgradeChannel); err == nil {
			out.UpgradeChannel = normalized
		}
	}
	out.NotifyNewBuild = settings.NotifyNewBuild
	out.RegisterEmailVerification = settings.RegisterEmailVerification
	if !settings.UpdatedAt.IsZero() {
		out.UpdatedAt = settings.UpdatedAt
	}
	return &out
}

// resolveSettings returns the effective settings (defaults + cfg + DB)
// or falls back to defaults on any DB error. Callers treat failure to
// load settings as non-fatal because a running control plane that
// briefly loses its DB connection should still be able to log, send
// emails with env-configured SMTP, etc.
func (s *Servers) resolveSettings(ctx context.Context) *store.Settings {
	if s.store == nil {
		return s.applySettingsDefaults(nil)
	}
	withTimeout, cancel := store.WithTimeout(ctx)
	defer cancel()
	settings, err := s.store.GetSettings(withTimeout)
	if err != nil {
		return s.applySettingsDefaults(nil)
	}
	return s.applySettingsDefaults(settings)
}

// smtpConfigFromSettings returns a config.Config with SMTP fields overridden
// from the supplied settings. Non-SMTP fields are preserved from s.cfg.
func (s *Servers) smtpConfigFromSettings(settings *store.Settings) config.Config {
	cfg := s.cfg
	if settings == nil {
		return cfg
	}
	if strings.TrimSpace(settings.SMTPHost) != "" {
		cfg.SMTPHost = strings.TrimSpace(settings.SMTPHost)
	}
	if settings.SMTPPort > 0 {
		cfg.SMTPPort = settings.SMTPPort
	}
	if strings.TrimSpace(settings.SMTPUsername) != "" {
		cfg.SMTPUser = strings.TrimSpace(settings.SMTPUsername)
	}
	if strings.TrimSpace(settings.SMTPPassword) != "" {
		cfg.SMTPPass = settings.SMTPPassword
	}
	if strings.TrimSpace(settings.SMTPFrom) != "" {
		cfg.SMTPFrom = strings.TrimSpace(settings.SMTPFrom)
	}
	return cfg
}
