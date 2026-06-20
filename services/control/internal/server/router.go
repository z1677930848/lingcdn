package server

// HTTP route registration for the admin/UI API. All routes land on a
// single net/http ServeMux behind withMetrics + withBodyLimit; per-route
// auth/admin/license gating is applied by wrapping the handler in the
// appropriate middleware here. Keep this as the single source of truth
// for URL → handler wiring.

import (
	"net/http"
	"strings"
)

func (s *Servers) adminMux() http.Handler {
	mux := http.NewServeMux()

	// Health checks
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)

	// Auth. All sensitive endpoints (credential acceptance, email-triggered
	// operations) are wrapped in per-IP rate limiters to curb brute-force and
	// spam. The confirm endpoint shares the login limiter since both gate
	// credential-adjacent secrets.
	s.initRateLimiters()
	mux.HandleFunc("/api/auth/login", rateLimit(s.authLimiter, "too many login attempts, try again later", s.handleLogin))
	mux.HandleFunc("/api/auth/register", rateLimit(s.registerLimiter, "too many registration attempts, try again later", s.handleRegister))
	mux.HandleFunc("/api/auth/register/email/request", rateLimit(s.emailLimiter, "too many email code requests, try again later", s.handleRegisterEmailRequest))
	mux.HandleFunc("/api/auth/password/reset/request", rateLimit(s.emailLimiter, "too many email code requests, try again later", s.handlePasswordResetRequest))
	mux.HandleFunc("/api/auth/password/reset/confirm", rateLimit(s.authLimiter, "too many reset attempts, try again later", s.handlePasswordResetConfirm))
	mux.HandleFunc("/api/auth/captcha", s.handleCaptcha)
	mux.HandleFunc("/api/auth/me", s.withAuth(s.handleMe))
	mux.HandleFunc("/api/auth/logout", s.withAuth(s.handleLogout))
	mux.HandleFunc("/api/auth/password/change", s.withAuth(s.handleChangePassword))
	mux.HandleFunc("/api/public/settings", s.handlePublicSettings)
	mux.HandleFunc("/api/public/announcements", s.handlePublicAnnouncements)
	mux.HandleFunc("/api/public/license/status", s.handlePublicLicenseStatus)
	// Offline-only local bundle distribution for node install (LICENSE_MODE=offline).
	mux.HandleFunc("/local/node_install.sh", s.handleLocalNodeInstallScript)
	mux.HandleFunc("/api/local/upgrade/latest", s.handleLocalUpgradeLatest)
	mux.HandleFunc("/api/local/bundles/", s.handleLocalBundleDownload)
	mux.HandleFunc("/api/admin/local-bundles", s.withAdmin(s.handleAdminLocalBundles))
	mux.HandleFunc("/api/admin/local-bundles/", s.withAdmin(s.handleAdminLocalBundleDelete))
	mux.HandleFunc("/api/admin/control-domain", s.withAdmin(s.handleControlDomainInfo))
	mux.HandleFunc("/api/admin/control-domain/verify", s.withAdmin(s.handleControlDomainVerify))
	mux.HandleFunc("/api/settings", s.withAdmin(s.handleSettings))

	// API Tokens
	mux.HandleFunc("/api/api-tokens", s.withAdmin(s.handleAPITokens))
	mux.HandleFunc("/api/api-tokens/", s.withAdmin(s.handleAPITokenByID))

	// Domain Blacklist
	mux.HandleFunc("/api/domain-blacklist", s.withAdmin(s.handleDomainBlacklist))
	mux.HandleFunc("/api/domain-blacklist/", s.withAdmin(s.handleDomainBlacklistByID))

	// Users
	mux.HandleFunc("/api/users", s.withAdmin(s.handleUsers))
	mux.HandleFunc("/api/users/bulk", s.withAdmin(s.handleUsersBulk))
	mux.HandleFunc("/api/users/", s.withAdmin(s.handleUserByID))
	mux.HandleFunc("/api/admin/user-groups", s.withAdmin(s.handleAdminUserGroups))
	mux.HandleFunc("/api/admin/user-groups/", s.withAdmin(s.handleAdminUserGroupByID))

	// Products (plans)
	mux.HandleFunc("/api/product-groups", s.withAuth(s.handleProductGroups))
	mux.HandleFunc("/api/product-groups/", s.withAuth(s.handleProductGroupByID))
	mux.HandleFunc("/api/products", s.withAuth(s.handleProducts))
	mux.HandleFunc("/api/products/", s.withAuth(s.handleProductByID))

	// Overview — full platform stats; admin-only (see handleOverview).
	mux.HandleFunc("/api/overview", s.withAdmin(s.handleOverview))

	// User balance API
	mux.HandleFunc("/api/balance/account", s.withAuth(s.handleBalanceAccount))
	mux.HandleFunc("/api/balance/transactions", s.withAuth(s.handleBalanceTransactions))
	mux.HandleFunc("/api/balance/recharges", s.withAuth(s.handleBalanceRecharges)) // GET list / POST create
	mux.HandleFunc("/api/balance/withdrawals", s.withAuth(s.handleBalanceWithdrawals))

	// Payment callbacks (no auth, signature verified by provider)
	mux.HandleFunc("/api/payments/notify/", s.handlePaymentNotify)
	mux.HandleFunc("/api/payments/mock/", s.withAuth(s.handlePaymentMock))

	// User orders API
	mux.HandleFunc("/api/user/orders", s.withAuth(s.handleUserOrders))

	// Support tickets
	mux.HandleFunc("/api/tickets", s.withAuth(s.handleTickets))
	mux.HandleFunc("/api/tickets/", s.withAuth(s.handleTicketByID))

	// Admin API. Every /api/admin/* endpoint is gated by withAdmin (which
	// layers on withAuth) as the single source of truth. Handlers that also
	// perform their own `if role != "admin"` check retain it as defence in
	// depth — cheap, and protects against a future refactor accidentally
	// re-routing a handler through withAuth alone. handleAdminStats in
	// particular was previously only behind withAuth, which let any
	// authenticated user see aggregate counts (users/domains/licenses); that
	// leak is closed here.
	mux.HandleFunc("/api/admin/ping", s.withAdmin(s.handlePing))
	mux.HandleFunc("/api/admin/stats", s.withAdmin(s.handleAdminStats))
	mux.HandleFunc("/api/admin/logs", s.withAdmin(s.handleAdminSystemLogs))
	mux.HandleFunc("/api/system/info", s.withAuth(s.handleSystemInfo))
	mux.HandleFunc("/api/admin/announcements", s.withAdmin(s.handleAdminAnnouncements))
	mux.HandleFunc("/api/admin/announcements/", s.withAdmin(s.handleAdminAnnouncementByID))
	mux.HandleFunc("/api/admin/orders", s.withAdmin(s.handleAdminOrders))
	mux.HandleFunc("/api/admin/orders/", s.withAdmin(s.handleAdminOrderByID))
	mux.HandleFunc("/api/admin/tickets", s.withAdmin(s.handleAdminTickets))
	mux.HandleFunc("/api/admin/tickets/", s.withAdmin(s.handleAdminTicketByID))
	mux.HandleFunc("/api/admin/balance/accounts", s.withAdmin(s.handleAdminBalanceAccounts))
	mux.HandleFunc("/api/admin/balance/recharges", s.withAdmin(s.handleAdminBalanceRecharges))
	mux.HandleFunc("/api/admin/balance/recharges/", s.withAdmin(s.handleAdminBalanceRechargeByID))
	mux.HandleFunc("/api/admin/balance/withdrawals", s.withAdmin(s.handleAdminBalanceWithdrawals))
	mux.HandleFunc("/api/admin/balance/withdrawals/", s.withAdmin(s.handleAdminBalanceWithdrawalByID))
	mux.HandleFunc("/api/admin/balance/adjust", s.withAdmin(s.handleAdminBalanceAdjust))
	mux.HandleFunc("/api/admin/balance/stats", s.withAdmin(s.handleAdminBalanceStats))

	// Nodes API
	mux.HandleFunc("/api/nodes", s.withAuth(s.handleNodes))
	mux.HandleFunc("/api/nodes/", s.withAuth(s.handleNodeByID))
	mux.HandleFunc("/api/nodes/overview", s.withAdmin(s.handleNodesOverview))
	mux.HandleFunc("/api/nodes/install-command", s.withAdmin(s.handleNodeInstallCommand))
	mux.HandleFunc("/api/nodes/install-ssh", s.withAdmin(s.handleNodeInstallSSH))
	mux.HandleFunc("/api/nodes/bootstrap-token", s.withAdmin(s.handleNodeBootstrapToken))
	mux.HandleFunc("/api/nodes/monitor/rank", s.withAdmin(s.handleNodeMonitorRank))
	mux.HandleFunc("/api/nodes/monitor/series", s.withAdmin(s.handleNodeMonitorSeries))
	mux.HandleFunc("/api/nodes/monitor/stream", s.withAdmin(s.handleNodeMonitorStream))

	// Domains API
	mux.HandleFunc("/api/domains", s.withAuth(s.handleDomains))
	mux.HandleFunc("/api/domains/", s.withAuth(s.handleDomainByID))
	mux.HandleFunc("/api/domains/health/metrics", s.withAdmin(s.handleDomainHealthMetrics))
	mux.HandleFunc("/api/domains/health/rank", s.withAdmin(s.handleDomainHealthRank))

	// L4 stream forwarding API
	mux.HandleFunc("/api/stream-forwards", s.withAuth(s.handleStreamForwards))
	mux.HandleFunc("/api/stream-forwards/", s.withAuth(s.handleStreamForwardByID))
	mux.HandleFunc("/api/admin/stream-forwards", s.withAdmin(s.handleAdminStreamForwards))

	// Origins API
	mux.HandleFunc("/api/origins", s.withAuth(s.handleOrigins))
	mux.HandleFunc("/api/origins/", s.withAuth(s.handleOriginByID))

	// Certificates API
	mux.HandleFunc("/api/certificates", s.withAuth(s.handleCertificates))
	mux.HandleFunc("/api/certificates/", s.withAuth(s.handleCertificateByID))
	mux.HandleFunc("/api/certificates/acme", s.withAuth(s.handleACMECertificate))

	// Cache Rules API
	// Cache rules, publish, purge, version history are fleet-wide admin
	// operations. They must not be exposed to tenant users: previously
	// gated only by withAuth, which let any logged-in user mutate global
	// cache behaviour or trigger a fleet publish/purge. Promoted to
	// withAdmin so the middleware rejects non-admins before the handler.
	mux.HandleFunc("/api/cache-rules", s.withAdmin(s.handleCacheRules))
	mux.HandleFunc("/api/cache-rules/", s.withAdmin(s.handleCacheRuleByID))

	// Config API
	mux.HandleFunc("/api/config/publish", s.withAdmin(s.handlePublish))
	mux.HandleFunc("/api/config/versions", s.withAdmin(s.handleConfigVersions))

	// Purge API
	mux.HandleFunc("/api/purge", s.withAdmin(s.handlePurge))
	mux.HandleFunc("/api/purge/", s.withAdmin(s.handlePurgeByID))
	mux.HandleFunc("/api/preload", s.withAdmin(s.handlePreload))
	mux.HandleFunc("/api/preload/", s.withAdmin(s.handlePreloadByID))

	// DNS API — every endpoint carries provider credentials or triggers
	// destructive provider-side operations (recover/cleanup delete real
	// DNS records). Must be admin-only. The previous withAuth gating let
	// any authenticated tenant read DNS provider tokens and nuke records.
	mux.HandleFunc("/api/dns/config", s.withAdmin(s.handleDNSConfig))
	mux.HandleFunc("/api/dns/providers", s.withAdmin(s.handleDNSProviders))
	mux.HandleFunc("/api/dns/config/recover", s.withAdmin(s.handleDNSRecover))
	mux.HandleFunc("/api/dns/config/cleanup", s.withAdmin(s.handleDNSCleanup))
	mux.HandleFunc("/api/dns/sync", s.withAdmin(s.handleDNSSync))
	mux.HandleFunc("/api/dns/provider-options", s.withAdmin(s.handleDNSProviderOptions))
	mux.HandleFunc("/api/dns/domain-options", s.withAdmin(s.handleDNSDomainOptions))
	mux.HandleFunc("/api/dns/provider-domains", s.withAdmin(s.handleDNSProviderDomains))
	mux.HandleFunc("/api/dns/lines", s.withAdmin(s.handleDNSLines))
	mux.HandleFunc("/api/dns/tasks", s.withAdmin(s.handleDNSTasks))

	mux.HandleFunc("/api/alerts", s.withAdmin(s.handleAlertRules))
	mux.HandleFunc("/api/alerts/", s.withAdmin(s.handleAlertRuleByID))

	// Cluster API
	mux.HandleFunc("/api/clusters", s.withAuth(s.handleClusters))
	mux.HandleFunc("/api/clusters/", s.withAuth(s.handleClusterByID))

	mux.HandleFunc("/api/waf/policies", s.withAuth(s.handleWAFPolicies))
	mux.HandleFunc("/api/waf/policies/", s.withAuth(s.handleWAFPolicyByID))
	mux.HandleFunc("/api/waf/cc", s.withAuth(s.handleCCPolicy))
	mux.HandleFunc("/api/waf/bans", s.withAuth(s.handleWAFBans))
	mux.HandleFunc("/api/waf/whitelist", s.withAuth(s.handleWAFWhitelist))
	mux.HandleFunc("/api/waf/rulesets/", s.withAdmin(s.handleWAFRulesetApply))
	mux.HandleFunc("/api/ddos/xdp/stats", s.withAdmin(s.handleDdosXdpStats))
	// ES log search: admins see all tenants; users are scoped to owned domains.
	mux.HandleFunc("/api/logs/es/health", s.withAdmin(s.handleESHealth))
	mux.HandleFunc("/api/logs/status", s.withAuth(s.handleLogsStatus))
	mux.HandleFunc("/api/logs/search", s.withAuth(s.handleLogsSearch))
	// License API. Method and store-timeout concerns are handled by the
	// middleware chain so the handlers can focus purely on business logic.
	mux.HandleFunc("/api/license/status", s.withAuthNoLicense(chain(s.handleLicenseStatus, methodOnly(http.MethodGet), withStoreTimeout)))
	mux.HandleFunc("/api/license/activate", s.withAuthNoLicense(chain(s.handleLicenseActivate, methodOnly(http.MethodPost))))
	mux.HandleFunc("/api/license/import", s.withAuthNoLicense(chain(s.handleLicenseImport, methodOnly(http.MethodPost))))
	// System upgrade API (admin-only; queries auth.lingcdn.cloud portal)
	// System-level operations are admin-only (upgrade, template edits,
	// SMTP diagnostics). Handlers still perform internal checks, but the
	// middleware shortcut keeps the intent obvious in the router.
	mux.HandleFunc("/api/system/upgrade", s.withAdmin(s.handleUpgradeInfo))
	mux.HandleFunc("/api/system/upgrade/precheck", s.withAdmin(s.handleUpgradePrecheck))
	mux.HandleFunc("/api/system/upgrade/control", s.withAdmin(s.handleUpgradeControl))
	mux.HandleFunc("/api/system/upgrade/nodes", s.withAdmin(s.handleUpgradeNodes))
	mux.HandleFunc("/api/system/upgrade/tasks", s.withAdmin(s.handleUpgradeTasks))
	mux.HandleFunc("/api/system/upgrade/tasks/", s.withAdmin(s.handleUpgradeTaskLogs))
	mux.HandleFunc("/api/system/email/test", s.withAdmin(s.handleEmailTest))
	mux.HandleFunc("/api/system/templates", s.withAdmin(s.handleGlobalTemplates))
	mux.HandleFunc("/api/system/templates/", s.withAdmin(s.handleGlobalTemplateByKey))
	mux.HandleFunc("/api/system/tasks", s.withAdmin(s.handleSystemTasks))
	mux.HandleFunc("/api/system/tasks/", s.withAdmin(s.handleSystemTaskAction))
	mux.HandleFunc("/api/system/publish/tasks", s.withAdmin(s.handlePublishTasks))
	mux.HandleFunc("/api/system/publish/tasks/", s.withAdmin(s.handlePublishTaskByID))
	mux.HandleFunc("/api/sync/active", s.withAuth(s.handleSyncActive))
	mux.HandleFunc("/api/sync/stream", s.withAuth(s.handleSyncStream))

	mux.HandleFunc("/api/geoip/latest", s.withAuthNoLicense(s.handleGeoIPLatest))
	mux.HandleFunc("/api/geoip/file", s.withAuthNoLicense(s.handleGeoIPFile))
	mux.HandleFunc("/api/system/geoip/status", s.withAuth(s.handleGeoIPStatus))
	mux.HandleFunc("/api/system/geoip/refresh", s.withAuth(s.handleGeoIPRefresh))

	// Webhook API (no auth, uses signature verification)
	mux.HandleFunc("/api/webhook/upgrade", s.HandleUpgradeWebhook)
	mux.HandleFunc("/api/webhook/tasks", s.HandleTaskWebhook)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
			return
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if s.tryServeControlUI(w, r) {
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "未找到"})
	})

	// Wrap with metrics and body-size-limit middleware
	return s.withControlDomainRedirect(s.withMetrics(s.withBodyLimit(mux)))
}
