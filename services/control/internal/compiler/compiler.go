package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
	"github.com/lingcdn/control/internal/templates"
)

// defaultErrorStatuses enumerates HTTP statuses for which a default
// error page template (error.<status>.html) may be injected into every
// domain's error_pages when the domain has no explicit override.
var defaultErrorStatuses = []int{400, 403, 404, 500, 502, 503, 504}

// NodeConfig is the configuration sent to edge nodes.
type NodeConfig struct {
	Version      string                     `json:"version"`
	Checksum     string                     `json:"checksum"`
	GeneratedAt  string                     `json:"generated_at"`
	Domains      []DomainConfig             `json:"domains"`
	Origins      map[string]OriginConfig    `json:"origins"`
	Certificates map[string]CertConfig      `json:"certificates"`
	CacheRules   []CacheRuleConfig          `json:"cache_rules"`
	WAFPolicies  []WAFPolicyConfig          `json:"waf_policies,omitempty"`
	WAFBans      []WAFBanConfig             `json:"waf_bans,omitempty"`
	WAFWhitelist []WAFWhitelistConfig       `json:"waf_whitelist,omitempty"`
	License      *LicenseConfig             `json:"license,omitempty"`
	UserLimits   map[string]UserLimitConfig `json:"user_limits,omitempty"`
	Templates    *GlobalTemplatesConfig     `json:"templates,omitempty"`
}

// GlobalTemplatesConfig carries admin-editable HTML/JSON templates that
// the node renders for error pages, WAF shield pages, default ban and
// challenge fallbacks. Populated from the global template store each
// time the node config is compiled.
type GlobalTemplatesConfig struct {
	ErrorPages              map[int]string `json:"error_pages,omitempty"`
	ErrorDefault            string         `json:"error_default,omitempty"`
	WAFShieldPage           string         `json:"waf_shield_page,omitempty"`
	WAFBanDefault           string         `json:"waf_ban_default,omitempty"`
	WAFChallengeDefaultJSON string         `json:"waf_challenge_default_json,omitempty"`
	CnameNotFound           string         `json:"cname_not_found,omitempty"`
	DirectIP                string         `json:"direct_ip,omitempty"`
}

// DomainConfig represents a domain in the node config.
type DomainConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	UserID   string `json:"user_id,omitempty"`
	OriginID string `json:"origin_id"`
	CertID   string `json:"cert_id,omitempty"`
	// HTTPSEnabled tells the node whether to terminate TLS on 443 for this
	// domain. Distinct from CertID: a domain may have a cert on file but
	// want HTTPS temporarily off, or have HTTPS explicitly on while ACME
	// provisions the cert asynchronously.
	HTTPSEnabled           bool   `json:"https_enabled"`
	CacheEnabled           bool   `json:"cache_enabled"`
	HTTP2Enabled           bool   `json:"http2_enabled"`
	WebsocketEnabled       bool   `json:"websocket_enabled"`
	OriginScheme           string `json:"origin_scheme,omitempty"`
	OriginPort             int32  `json:"origin_port,omitempty"`
	OriginHostMode         string `json:"origin_host_mode,omitempty"`
	OriginHost             string `json:"origin_host,omitempty"`
	OriginTimeoutMs        int64  `json:"origin_timeout_ms,omitempty"`
	OriginConnectTimeoutMs int64  `json:"origin_connect_timeout_ms,omitempty"`
	// OriginAuth tells the node to inject authentication headers/credentials
	// when connecting to the origin. nil/zero means no auth.
	OriginAuth *OriginAuthConfig `json:"origin_auth,omitempty"`
	// Origins: per-domain upstream addresses. When non-empty, these
	// fully replace the legacy OriginID -> Origins[OriginID].Addresses
	// lookup at the node. Kept inline on the domain so the node does
	// not need a separate lookup table and so per-domain weight/enable
	// are always tied to the domain they belong to.
	Origins    []DomainOriginConfig `json:"origins,omitempty"`
	ErrorPages []ErrorPageConfig    `json:"error_pages,omitempty"`
	// LoadBalanceMethod selects the per-domain origin distribution
	// policy on the node. "round_robin" (default) is a weighted random
	// pick across enabled origins; "ip_hash" makes the node hash the
	// client IP onto a stable origin index for sticky sessions.
	LoadBalanceMethod string `json:"load_balance_method,omitempty"`
	// OriginHealthCheck, when Enabled, makes the node periodically
	// probe each origin address with an HTTP GET. Origins that fail
	// the probe consecutively `FailThreshold` times are removed from
	// the candidate pool for new requests until they pass `PassThreshold`
	// consecutive probes again. nil/zero means health check disabled.
	OriginHealthCheck *OriginHealthCheckConfig `json:"origin_health_check,omitempty"`
}

// OriginHealthCheckConfig is the wire-format mirror of
// store.OriginHealthCheck delivered to nodes.
type OriginHealthCheckConfig struct {
	Enabled        bool   `json:"enabled"`
	IntervalSec    int32  `json:"interval_sec,omitempty"`
	TimeoutMs      int64  `json:"timeout_ms,omitempty"`
	Path           string `json:"path,omitempty"`
	ExpectedStatus int32  `json:"expected_status,omitempty"`
	FailThreshold  int32  `json:"fail_threshold,omitempty"`
	PassThreshold  int32  `json:"pass_threshold,omitempty"`
}

// DomainOriginConfig is a single upstream bound to a domain. Weight is
// 1..100 and enabled=false means "skip entirely" (not even attempted on
// failover). The node performs weighted random selection across
// enabled entries.
type DomainOriginConfig struct {
	Address string `json:"address"`
	Weight  int32  `json:"weight"`
	Enabled bool   `json:"enabled"`
}

// OriginAuthConfig is the origin authentication block delivered to nodes.
// The node inspects Mode to decide how to inject credentials into the
// upstream request. When Enabled is false or the struct is nil, no auth
// is applied.
type OriginAuthConfig struct {
	Enabled   bool                     `json:"enabled"`
	Mode      string                   `json:"mode,omitempty"` // "header" or "basic"
	Headers   []OriginAuthHeaderConfig `json:"headers,omitempty"`
	BasicUser string                   `json:"basic_user,omitempty"`
	BasicPass string                   `json:"basic_pass,omitempty"`
}

// OriginAuthHeaderConfig is a single custom header injected on origin fetch.
type OriginAuthHeaderConfig struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// UserLimitConfig represents per-user resource limits for edge enforcement.
type UserLimitConfig struct {
	BandwidthBps        int64 `json:"bandwidth_bps,omitempty"`
	ConnLimit           int64 `json:"conn_limit,omitempty"`
	MonthlyTrafficBytes int64 `json:"monthly_traffic_bytes,omitempty"`
}

// OriginConfig represents an origin server in the node config.
type OriginConfig struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Addresses  []string `json:"addresses"`
	TimeoutMs  int64    `json:"timeout_ms"`
	MaxRetries int32    `json:"max_retries"`
}

// CertConfig represents a certificate in the node config.
type CertConfig struct {
	ID      string `json:"id"`
	Domain  string `json:"domain"`
	CertPEM []byte `json:"cert_pem,omitempty"`
	KeyPEM  []byte `json:"key_pem,omitempty"`
}

// ErrorPageConfig represents custom error pages.
type ErrorPageConfig struct {
	Status  int    `json:"status"`
	Mode    string `json:"mode"`
	Content string `json:"content"`
}

// LicenseConfig represents system license status for nodes.
type LicenseConfig struct {
	Status        string `json:"status"`
	ExpiresAtUnix int64  `json:"expires_at_unix,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// CacheRuleConfig represents a cache rule in the node config.
type CacheRuleConfig struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	HostPattern      string   `json:"host_pattern"`
	PathPattern      string   `json:"path_pattern"`
	Methods          []string `json:"methods"`
	TTLSeconds       int64    `json:"ttl_seconds"`
	CacheQueryParams bool     `json:"cache_query_params"`
	Priority         int32    `json:"priority"`
}

// WAFPolicyConfig represents a WAF policy delivered to nodes.
type WAFPolicyConfig struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Scope       string          `json:"scope"` // global | domain | line_group
	ScopeID     string          `json:"scope_id,omitempty"`
	Description string          `json:"description,omitempty"`
	Enabled     bool            `json:"enabled"`
	Rules       []WAFRuleConfig `json:"rules"`
}

// WAFRuleConfig is a single rule in a policy.
type WAFRuleConfig struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`   // ip_cidr | rate_limit | geo_block
	Action           string   `json:"action"` // allow | deny
	Value            string   `json:"value"`
	Threshold        int64    `json:"threshold,omitempty"`
	WindowSeconds    int64    `json:"window_seconds,omitempty"`
	Note             string   `json:"note,omitempty"`
	Priority         int32    `json:"priority,omitempty"`
	ShieldSeconds    int64    `json:"shield_seconds,omitempty"`
	AutoChallengeQPS int64    `json:"auto_challenge_qps,omitempty"`
	ExpiresAtUnix    int64    `json:"expires_at,omitempty"`
	BanSeconds       int64    `json:"ban_seconds,omitempty"`
	TemplateHTML     string   `json:"template_html,omitempty"`
	BanTemplateHTML  string   `json:"ban_template_html,omitempty"`
	RedirectURL      string   `json:"redirect_url,omitempty"`
	BanMode          string   `json:"ban_mode,omitempty"`
	CaptchaType      string   `json:"captcha_type,omitempty"`
	PathPrefix       string   `json:"path_prefix,omitempty"`
	Methods          []string `json:"methods,omitempty"`
	UAContains       string   `json:"ua_contains,omitempty"`
	LogOnly          bool     `json:"log_only,omitempty"`
	GeoCountries     []string `json:"geo_countries,omitempty"` // ISO 3166-1 alpha-2 codes for geo_block
}

// WAFBanConfig represents distributed ban entries.
type WAFBanConfig struct {
	IP        string `json:"ip"`
	Reason    string `json:"reason,omitempty"`
	Strikes   int    `json:"strikes,omitempty"`
	ExpiresAt int64  `json:"expires_at"`
}

// WAFWhitelistConfig represents distributed whitelist entries.
type WAFWhitelistConfig struct {
	IP   string `json:"ip"` // IP or CIDR
	Note string `json:"note,omitempty"`
}

// Compiler turns management configs into executable node configs.
type Compiler struct {
	store store.Store
}

// New creates a new Compiler.
func New(s store.Store) *Compiler {
	return &Compiler{store: s}
}

// Compile reads all configuration from the store and compiles it into a NodeConfig.
// Returns the version string, JSON payload, and any error.
func (c *Compiler) Compile(ctx context.Context) (version string, payload []byte, err error) {
	log.Ctx(ctx).Debug().Msg("compiling node configuration")

	defaultTemplate := make(map[string]templates.GlobalTemplateDef)
	for _, d := range templates.DefaultGlobalTemplateDefs() {
		defaultTemplate[d.Key] = d
	}
	overrideTemplate := make(map[string]string)
	if c.store != nil {
		if list, err := c.store.ListGlobalTemplateOverrides(ctx); err == nil {
			for _, t := range list {
				if t == nil || strings.TrimSpace(t.Key) == "" {
					continue
				}
				overrideTemplate[t.Key] = t.Content
			}
		}
	}
	// Resolve system name from settings so default + admin-overridden
	// templates can substitute `{{SYSTEM_NAME}}` at compile time. Failure
	// to read settings falls back to "LingCDN" so we never block a build.
	systemName := "LingCDN"
	if c.store != nil {
		if s, err := c.store.GetSettings(ctx); err == nil && s != nil {
			if name := strings.TrimSpace(s.SystemName); name != "" {
				systemName = name
			}
		}
	}
	effectiveTemplate := func(key string) string {
		var raw string
		if v, ok := overrideTemplate[key]; ok {
			raw = v
		} else if d, ok := defaultTemplate[key]; ok {
			raw = d.DefaultContent
		} else {
			return ""
		}
		return strings.ReplaceAll(raw, "{{SYSTEM_NAME}}", systemName)
	}

	// Fetch all domains
	domains, err := c.store.ListDomains(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list domains: %w", err)
	}

	// Fetch all origins
	origins, err := c.store.ListOrigins(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list origins: %w", err)
	}

	// Fetch all per-domain origin entries (new authoritative source).
	// Indexed by domain_id so the per-domain loop below can inline them
	// into each DomainConfig.Origins without N+1 DB round trips.
	domainOrigins, err := c.store.ListAllDomainOrigins(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list domain origins: %w", err)
	}
	domainOriginsByDomain := make(map[string][]*store.DomainOrigin, len(domains))
	for _, e := range domainOrigins {
		domainOriginsByDomain[e.DomainID] = append(domainOriginsByDomain[e.DomainID], e)
	}

	embedCerts := true
	if v := strings.TrimSpace(os.Getenv("CONTROL_EMBED_CERTS")); v != "" {
		switch strings.ToLower(v) {
		case "0", "false", "no", "off":
			embedCerts = false
		case "1", "true", "yes", "on":
			embedCerts = true
		}
	}

	// Fetch all certificates (optional).
	var certs []*store.Certificate
	if embedCerts {
		certs, err = c.store.ListCertificates(ctx)
		if err != nil {
			return "", nil, fmt.Errorf("list certificates: %w", err)
		}
	}

	// Fetch all cache rules
	rules, err := c.store.ListCacheRules(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list cache rules: %w", err)
	}

	var licenseCfg *LicenseConfig
	if c.store != nil {
		if st, err := c.store.GetLicenseState(ctx); err == nil && st != nil {
			status := strings.ToLower(strings.TrimSpace(st.Status))
			if status == "" {
				status = "unlicensed"
			}
			lc := LicenseConfig{Status: status}
			if !st.ExpiresAt.IsZero() {
				lc.ExpiresAtUnix = st.ExpiresAt.Unix()
			}
			if strings.TrimSpace(st.Reason) != "" {
				lc.Reason = st.Reason
			}
			licenseCfg = &lc
		}
	}

	// Build node config
	nodeConfig := NodeConfig{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Domains:      make([]DomainConfig, 0, len(domains)),
		Origins:      make(map[string]OriginConfig, len(origins)),
		Certificates: make(map[string]CertConfig),
		CacheRules:   make([]CacheRuleConfig, 0, len(rules)),
		License:      licenseCfg,
	}

	neededCertIDs := make(map[string]struct{})

	// Convert domains (only enabled ones)
	for _, d := range domains {
		if !d.Enabled {
			continue
		}
		errorPages := convertErrorPages(d.ErrorPages)
		have := make(map[int]struct{}, len(errorPages))
		for _, p := range errorPages {
			have[p.Status] = struct{}{}
		}
		for _, status := range defaultErrorStatuses {
			if _, ok := have[status]; ok {
				continue
			}
			key := fmt.Sprintf("error.%d.html", status)
			if tpl := strings.TrimSpace(effectiveTemplate(key)); tpl != "" {
				errorPages = append(errorPages, ErrorPageConfig{Status: status, Mode: "html", Content: tpl})
			}
		}

		// Track needed cert IDs so we can avoid embedding unrelated certs.
		// Previously this had a fallback: `if certID == "" { certID = d.Name }`
		// which silently broke whenever a user had bound a different cert.
		// Now we strictly require CertID to be explicitly set (the upload
		// and ACME handlers auto-bind on success).
		certID := strings.TrimSpace(d.CertID)
		if certID != "" {
			neededCertIDs[certID] = struct{}{}
		}

		// Per-domain origins (new model). Serialised inline on the
		// DomainConfig so nodes no longer need to look up a shared
		// origin pool keyed by OriginID.
		var perDomainOrigins []DomainOriginConfig
		if list := domainOriginsByDomain[d.ID]; len(list) > 0 {
			perDomainOrigins = make([]DomainOriginConfig, 0, len(list))
			for _, e := range list {
				perDomainOrigins = append(perDomainOrigins, DomainOriginConfig{
					Address: e.Address,
					Weight:  e.Weight,
					Enabled: e.Enabled,
				})
			}
		}

		nodeConfig.Domains = append(nodeConfig.Domains, DomainConfig{
			ID:                     d.ID,
			Name:                   d.Name,
			UserID:                 d.UserID,
			OriginID:               d.OriginID,
			CertID:                 d.CertID,
			HTTPSEnabled:           d.HTTPSEnabled,
			CacheEnabled:           d.CacheEnabled,
			HTTP2Enabled:           d.HTTP2Enabled,
			WebsocketEnabled:       d.WebsocketEnabled,
			OriginScheme:           defaultOriginScheme(d.OriginScheme),
			OriginPort:             defaultOriginPort(d.OriginPort),
			OriginHostMode:         defaultOriginHostMode(d.OriginHostMode),
			OriginHost:             d.OriginHost,
			OriginTimeoutMs:        defaultOriginTimeout(d.OriginTimeoutMs),
			OriginConnectTimeoutMs: defaultOriginConnectTimeout(d.OriginConnectTimeoutMs),
			OriginAuth:             compileOriginAuth(d.OriginAuth),
			Origins:                perDomainOrigins,
			ErrorPages:             errorPages,
			LoadBalanceMethod:      compileLoadBalanceMethod(d.LoadBalanceMethod),
			OriginHealthCheck:      compileOriginHealthCheck(d.OriginHealthCheck),
		})
	}

	// Convert origins
	for _, o := range origins {
		nodeConfig.Origins[o.ID] = OriginConfig{
			ID:         o.ID,
			Name:       o.Name,
			Addresses:  o.Addresses,
			TimeoutMs:  o.TimeoutMs,
			MaxRetries: o.MaxRetries,
		}
	}

	// Convert certificates
	if embedCerts {
		// Only embed certs that are actually referenced by enabled domains.
		for _, cert := range certs {
			if cert == nil {
				continue
			}
			certIDStr := strconv.FormatInt(cert.ID, 10)
			if _, ok := neededCertIDs[certIDStr]; !ok {
				continue
			}
			nodeConfig.Certificates[certIDStr] = CertConfig{
				ID:      certIDStr,
				Domain:  cert.Domain,
				CertPEM: cert.CertPEM,
				KeyPEM:  cert.KeyPEM,
			}
		}
	}

	// Convert cache rules (only enabled ones)
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		nodeConfig.CacheRules = append(nodeConfig.CacheRules, CacheRuleConfig{
			ID:               r.ID,
			Name:             r.Name,
			HostPattern:      r.HostPattern,
			PathPattern:      r.PathPattern,
			Methods:          r.Methods,
			TTLSeconds:       r.TTLSeconds,
			CacheQueryParams: r.CacheQueryParams,
			Priority:         r.Priority,
		})
	}

	// Provide a safe baseline when no cache rules are configured.
	//
	// This improves latency for public-origin scenarios out of the box, while keeping risk low by
	// only caching common static assets (GET/HEAD). User-configured rules (if any) always win.
	if len(nodeConfig.CacheRules) == 0 && defaultCacheRulesEnabled() {
		nodeConfig.CacheRules = append(nodeConfig.CacheRules, defaultCacheRules()...)
	}

	// WAF policies (enabled only)
	if wafPolicies, err := c.store.ListWAFPolicies(ctx); err == nil {
		for _, p := range wafPolicies {
			if p == nil || !p.Enabled {
				continue
			}
			// attach rules
			if p.Rules == nil {
				if rules, err := c.store.ListWAFRules(ctx, p.ID); err == nil {
					p.Rules = rules
				}
			}
			nodePol := WAFPolicyConfig{
				ID:          p.ID,
				Name:        p.Name,
				Scope:       p.Scope,
				ScopeID:     p.ScopeID,
				Description: p.Description,
				Enabled:     p.Enabled,
				Rules:       []WAFRuleConfig{},
			}
			for _, r := range p.Rules {
				if r == nil || !r.Enabled {
					continue
				}
				banMode := r.BanMode
				if banMode == "" {
					banMode = "ipset"
				}
				nodePol.Rules = append(nodePol.Rules, WAFRuleConfig{
					ID:               r.ID,
					Type:             r.Type,
					Action:           r.Action,
					Value:            r.Value,
					Threshold:        r.Threshold,
					WindowSeconds:    r.WindowSeconds,
					Note:             r.Note,
					Priority:         r.Priority,
					ShieldSeconds:    r.ShieldSeconds,
					AutoChallengeQPS: r.AutoChallengeQPS,
					BanSeconds:       r.BanSeconds,
					CaptchaType:      r.CaptchaType,
					TemplateHTML: func() string {
						if strings.TrimSpace(r.TemplateHTML) != "" {
							return r.TemplateHTML
						}
						if r.Type == "challenge_captcha" {
							// 根据 captcha_type 选择对应模板
							switch r.CaptchaType {
							case "slide":
								return effectiveTemplate("waf.challenge.slide")
							case "click":
								return effectiveTemplate("waf.challenge.click")
							case "rotate":
								return effectiveTemplate("waf.challenge.rotate")
							case "slide_region":
								return effectiveTemplate("waf.challenge.slide_region")
							case "js_challenge":
								return effectiveTemplate("waf.challenge.js")
							default:
								return effectiveTemplate("waf.challenge.page")
							}
						}
						return ""
					}(),
					BanTemplateHTML: func() string {
						if strings.TrimSpace(r.BanTemplateHTML) != "" {
							return r.BanTemplateHTML
						}
						if banMode == "page" {
							return effectiveTemplate("waf.ban.page")
						}
						return ""
					}(),
					RedirectURL:   r.RedirectURL,
					BanMode:       banMode,
					PathPrefix:    r.PathPrefix,
					Methods:       r.Methods,
					UAContains:    r.UAContains,
					LogOnly:       r.LogOnly,
					ExpiresAtUnix: r.ExpiresAt.Unix(),
				})
			}
			nodeConfig.WAFPolicies = append(nodeConfig.WAFPolicies, nodePol)
		}
	}

	// Per-domain security settings (Domain.Security) → synthesise a
	// WAFPolicyConfig{Scope:"domain"} so the rich cdnfly-style CC panel on
	// the domain-detail page actually lands at the edge without requiring
	// operators to build raw WAF rules.
	//
	// Rule ordering (priority, higher first on the node):
	//   1000 whitelist IPs  → allow
	//    900 blacklist IPs  → deny
	//    800 search-bot UA  → allow/deny per SearchBot
	//    700 custom rules   → per-row action (mapped from mode)
	//    100 default mode   → catches everything else
	for _, d := range domains {
		if d == nil || !d.Enabled || d.Security == nil {
			continue
		}
		sec := d.Security
		// No master enabled toggle anymore: each sub-field is its own
		// switch (DefaultMode, IPBlacklist length, per-rule Enabled,
		// BlockedRegions length). A DomainSecurity with all sub-fields
		// at their zero values produces an empty policy below, which is
		// equivalent to the old "disabled" state at the edge.
		pol := WAFPolicyConfig{
			ID:      "domain-sec-" + d.ID,
			Name:    "domain:" + d.Name,
			Scope:   "domain",
			ScopeID: d.ID,
			Enabled: true,
			Rules:   []WAFRuleConfig{},
		}
		banSeconds := sec.BanSeconds
		if banSeconds <= 0 {
			banSeconds = 300
		}

		// IP whitelist — evaluated first, short-circuits everything.
		for _, ip := range sec.IPWhitelist {
			ip = strings.TrimSpace(ip)
			if ip == "" || strings.HasPrefix(ip, "#") {
				continue
			}
			pol.Rules = append(pol.Rules, WAFRuleConfig{
				ID:       fmt.Sprintf("%s-wl-%x", d.ID, hashString(ip)),
				Type:     "ip_cidr",
				Action:   "allow",
				Value:    ip,
				Priority: 1000,
				Note:     "domain whitelist",
			})
		}
		// IP blacklist — deny with configured ban duration.
		for _, ip := range sec.IPBlacklist {
			ip = strings.TrimSpace(ip)
			if ip == "" || strings.HasPrefix(ip, "#") {
				continue
			}
			pol.Rules = append(pol.Rules, WAFRuleConfig{
				ID:         fmt.Sprintf("%s-bl-%x", d.ID, hashString(ip)),
				Type:       "ip_cidr",
				Action:     "deny",
				Value:      ip,
				Priority:   900,
				BanSeconds: banSeconds,
				BanMode:    "ipset",
				Note:       "domain blacklist",
			})
		}
		// Search-engine bot handling.
		switch strings.ToLower(strings.TrimSpace(sec.SearchBot)) {
		case "allow":
			pol.Rules = append(pol.Rules, WAFRuleConfig{
				ID:         d.ID + "-bot-allow",
				Type:       "ua_match",
				Action:     "allow",
				UAContains: "bot|spider|slurp|bingpreview|googlebot|baiduspider|yandex",
				Priority:   800,
				Note:       "search-engine bot allow",
			})
		case "deny":
			pol.Rules = append(pol.Rules, WAFRuleConfig{
				ID:         d.ID + "-bot-deny",
				Type:       "ua_match",
				Action:     "deny",
				UAContains: "bot|spider|slurp|bingpreview|googlebot|baiduspider|yandex",
				Priority:   800,
				BanSeconds: banSeconds,
				BanMode:    "ipset",
				Note:       "search-engine bot deny",
			})
		}
		// Region (geo) blocking — deny visitors from selected countries.
		// Priority 850 sits between bot handling (800) and blacklist (900),
		// so IP whitelist still overrides it. Each domain gets a single
		// geo_block rule carrying a list of country codes; the node's
		// GeoIP resolver does the lookup once per request.
		if len(sec.BlockedRegions) > 0 {
			codes := make([]string, 0, len(sec.BlockedRegions))
			for _, c := range sec.BlockedRegions {
				c = strings.TrimSpace(strings.ToUpper(c))
				if c != "" {
					codes = append(codes, c)
				}
			}
			if len(codes) > 0 {
				pol.Rules = append(pol.Rules, WAFRuleConfig{
					ID:           d.ID + "-geo-block",
					Type:         "geo_block",
					Action:       "deny",
					Value:        strings.Join(codes, ","),
					GeoCountries: codes,
					Priority:     850,
					Note:         "region block",
				})
			}
		}
		// Block transparent proxies — deny requests carrying
		// x-forwarded-for headers. Priority 860 so IP whitelist
		// still overrides but it sits above geo and bot rules.
		if sec.BlockTransparentProxy {
			pol.Rules = append(pol.Rules, WAFRuleConfig{
				ID:       d.ID + "-block-proxy",
				Type:     "block_transparent_proxy",
				Action:   "deny",
				Value:    "x-forwarded-for",
				Priority: 860,
				Note:     "block transparent proxy",
			})
		}
		// Custom rules (each row).
		for i, r := range sec.CustomRules {
			if !r.Enabled {
				continue
			}
			action, captcha := secModeToAction(r.Mode)
			rid := r.ID
			if rid == "" {
				rid = fmt.Sprintf("%s-cr-%d", d.ID, i)
			}
			ruleType := "path_match"
			if strings.HasPrefix(r.Match, "header:") {
				ruleType = "header_match"
			} else if strings.HasPrefix(r.Match, "ua:") {
				ruleType = "ua_match"
			}
			pol.Rules = append(pol.Rules, WAFRuleConfig{
				ID:          rid,
				Type:        ruleType,
				Action:      action,
				Value:       r.Match,
				CaptchaType: captcha,
				Priority:    700,
				Note:        r.Note,
				BanSeconds:  banSeconds,
				BanMode:     "ipset",
			})
		}
		// Default catch-all mode.
		if action, captcha := secModeToAction(sec.DefaultMode); action != "" {
			pol.Rules = append(pol.Rules, WAFRuleConfig{
				ID:            d.ID + "-default",
				Type:          "default",
				Action:        action,
				CaptchaType:   captcha,
				Priority:      100,
				BanSeconds:    banSeconds,
				BanMode:       "ipset",
				ShieldSeconds: shieldSecondsForMode(sec.DefaultMode),
				Note:          "domain default mode: " + sec.DefaultMode,
			})
		}
		if len(pol.Rules) > 0 {
			nodeConfig.WAFPolicies = append(nodeConfig.WAFPolicies, pol)
		}
	}

	// WAF bans (global)
	if bans, err := c.store.ListWAFBans(ctx, 500); err == nil {
		for _, b := range bans {
			if b == nil {
				continue
			}
			if !b.ExpiresAt.IsZero() && b.ExpiresAt.Before(time.Now()) {
				continue
			}
			nodeConfig.WAFBans = append(nodeConfig.WAFBans, WAFBanConfig{
				IP:        b.IP,
				Reason:    b.Reason,
				Strikes:   b.Strikes,
				ExpiresAt: b.ExpiresAt.Unix(),
			})
		}
	}

	// WAF whitelist (global)
	if whitelist, err := c.store.ListWAFWhitelist(ctx); err == nil {
		for _, w := range whitelist {
			if w == nil {
				continue
			}
			nodeConfig.WAFWhitelist = append(nodeConfig.WAFWhitelist, WAFWhitelistConfig{
				IP:   w.IP,
				Note: w.Note,
			})
		}
	}

	// Build per-user limits from active products
	userIDs := make(map[string]struct{})
	for _, d := range domains {
		if d.Enabled && strings.TrimSpace(d.UserID) != "" {
			userIDs[d.UserID] = struct{}{}
		}
	}
	if len(userIDs) > 0 {
		userLimits := make(map[string]UserLimitConfig)
		for uid := range userIDs {
			orders, err := c.store.ListOrders(ctx, uid)
			if err != nil {
				continue
			}
			for _, o := range orders {
				if o == nil || o.Status != "paid" {
					continue
				}
				if o.EndsAt != nil && o.EndsAt.Before(time.Now()) {
					continue
				}
				p, err := c.store.GetProduct(ctx, o.ProductID)
				if err != nil || p == nil {
					continue
				}
				var lc UserLimitConfig
				if p.BandwidthBps != nil && *p.BandwidthBps > 0 {
					lc.BandwidthBps = *p.BandwidthBps
				}
				if p.ConnLimit != nil && *p.ConnLimit > 0 {
					lc.ConnLimit = *p.ConnLimit
				}
				if p.MonthlyTrafficBytes != nil && *p.MonthlyTrafficBytes > 0 {
					lc.MonthlyTrafficBytes = *p.MonthlyTrafficBytes
				}
				if lc.BandwidthBps > 0 || lc.ConnLimit > 0 || lc.MonthlyTrafficBytes > 0 {
					userLimits[uid] = lc
				}
				break // use first active product
			}
		}
		if len(userLimits) > 0 {
			nodeConfig.UserLimits = userLimits
		}
	}

	// Pack admin-editable global templates so the node can render error
	// pages, WAF shield/ban/challenge fallbacks without any hardcoded
	// content. Values are sourced from the same `effectiveTemplate`
	// helper used for per-domain error pages / WAF rule templates.
	gt := &GlobalTemplatesConfig{
		ErrorPages:              make(map[int]string, len(defaultErrorStatuses)),
		ErrorDefault:            strings.TrimSpace(effectiveTemplate("error.default.html")),
		WAFShieldPage:           strings.TrimSpace(effectiveTemplate("waf.shield.page")),
		WAFBanDefault:           strings.TrimSpace(effectiveTemplate("waf.ban.default")),
		WAFChallengeDefaultJSON: strings.TrimSpace(effectiveTemplate("waf.challenge.default_json")),
		CnameNotFound:           strings.TrimSpace(effectiveTemplate("node.cname_not_found")),
		DirectIP:                strings.TrimSpace(effectiveTemplate("node.direct_ip")),
	}
	for _, status := range defaultErrorStatuses {
		if tpl := strings.TrimSpace(effectiveTemplate(fmt.Sprintf("error.%d.html", status))); tpl != "" {
			gt.ErrorPages[status] = tpl
		}
	}
	if len(gt.ErrorPages) == 0 {
		gt.ErrorPages = nil
	}
	nodeConfig.Templates = gt

	// Serialize to JSON
	payload, err = json.Marshal(nodeConfig)
	if err != nil {
		return "", nil, fmt.Errorf("marshal config: %w", err)
	}

	// Calculate checksum
	checksum := sha256.Sum256(payload)
	checksumHex := hex.EncodeToString(checksum[:])

	// Generate version (timestamp + short checksum)
	version = fmt.Sprintf("v%d-%s", time.Now().Unix(), checksumHex[:8])

	// Update config with version and checksum
	nodeConfig.Version = version
	nodeConfig.Checksum = checksumHex

	// Re-serialize with version and checksum
	payload, err = json.Marshal(nodeConfig)
	if err != nil {
		return "", nil, fmt.Errorf("marshal config with version: %w", err)
	}

	// Recalculate checksum with version included
	checksum = sha256.Sum256(payload)
	checksumHex = hex.EncodeToString(checksum[:])

	log.Ctx(ctx).Info().
		Str("version", version).
		Str("checksum", checksumHex[:16]).
		Int("domains", len(nodeConfig.Domains)).
		Int("origins", len(nodeConfig.Origins)).
		Int("certificates", len(nodeConfig.Certificates)).
		Int("cache_rules", len(nodeConfig.CacheRules)).
		Msg("configuration compiled")

	return version, payload, nil
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

// compileOriginAuth converts the store OriginAuth into the node-facing
// OriginAuthConfig. Returns nil when auth is disabled or empty, so nodes
// don't receive a meaningless zero struct.
func compileOriginAuth(src *store.OriginAuth) *OriginAuthConfig {
	if src == nil || !src.Enabled {
		return nil
	}
	cfg := &OriginAuthConfig{
		Enabled:   true,
		Mode:      src.Mode,
		BasicUser: src.BasicUser,
		BasicPass: src.BasicPass,
	}
	for _, h := range src.Headers {
		name := strings.TrimSpace(h.Name)
		if name == "" {
			continue
		}
		cfg.Headers = append(cfg.Headers, OriginAuthHeaderConfig{
			Name:  name,
			Value: h.Value,
		})
	}
	return cfg
}

// compileLoadBalanceMethod normalises the per-domain load-balance method
// for the wire format. Empty/unknown values fall back to "round_robin"
// to keep the legacy weighted-random behaviour for migrated rows.
func compileLoadBalanceMethod(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ip_hash":
		return "ip_hash"
	default:
		return "round_robin"
	}
}

// compileOriginHealthCheck converts the store OriginHealthCheck into the
// node-facing config. Returns nil when health-check is disabled or the
// struct is missing, so nodes don't waste a checker task on a no-op.
// Defaults are filled in here so the node always receives a complete
// snapshot — easier to audit than trying to keep two layers in sync.
func compileOriginHealthCheck(src *store.OriginHealthCheck) *OriginHealthCheckConfig {
	if src == nil || !src.Enabled {
		return nil
	}
	cfg := &OriginHealthCheckConfig{
		Enabled:        true,
		IntervalSec:    src.IntervalSec,
		TimeoutMs:      src.TimeoutMs,
		Path:           strings.TrimSpace(src.Path),
		ExpectedStatus: src.ExpectedStatus,
		FailThreshold:  src.FailThreshold,
		PassThreshold:  src.PassThreshold,
	}
	if cfg.IntervalSec < 5 {
		cfg.IntervalSec = 30
	}
	if cfg.TimeoutMs <= 0 {
		cfg.TimeoutMs = 5000
	}
	if cfg.Path == "" {
		cfg.Path = "/"
	}
	if cfg.FailThreshold <= 0 {
		cfg.FailThreshold = 3
	}
	if cfg.PassThreshold <= 0 {
		cfg.PassThreshold = 2
	}
	return cfg
}

func convertErrorPages(pages []store.ErrorPage) []ErrorPageConfig {
	if len(pages) == 0 {
		return nil
	}
	out := make([]ErrorPageConfig, 0, len(pages))
	for _, p := range pages {
		out = append(out, ErrorPageConfig{
			Status:  p.Status,
			Mode:    p.Mode,
			Content: p.Content,
		})
	}
	return out
}

func defaultCacheRulesEnabled() bool {
	v := strings.TrimSpace(os.Getenv("CONTROL_DEFAULT_CACHE_RULES"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func envInt64(name string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	out, err := strconv.ParseInt(v, 10, 64)
	if err != nil || out <= 0 {
		return fallback
	}
	return out
}

func defaultCacheRules() []CacheRuleConfig {
	ttlStatic := envInt64("CONTROL_DEFAULT_CACHE_TTL_STATIC_SECONDS", 3600)       // 1h
	ttlFonts := envInt64("CONTROL_DEFAULT_CACHE_TTL_FONTS_SECONDS", 86400)        // 1d
	ttlImages := envInt64("CONTROL_DEFAULT_CACHE_TTL_IMAGES_SECONDS", 86400)      // 1d
	ttlDownloads := envInt64("CONTROL_DEFAULT_CACHE_TTL_DOWNLOADS_SECONDS", 3600) // 1h
	ttlMisc := envInt64("CONTROL_DEFAULT_CACHE_TTL_MISC_SECONDS", 600)            // 10m

	// Note: patterns use '*' wildcard; nodes perform a simple glob->regex conversion.
	return []CacheRuleConfig{
		{
			ID:               "default-fonts-woff2",
			Name:             "Default: fonts (woff2)",
			HostPattern:      "*",
			PathPattern:      "*.woff2",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlFonts,
			CacheQueryParams: true,
			Priority:         -100,
		},
		{
			ID:               "default-fonts-woff",
			Name:             "Default: fonts (woff)",
			HostPattern:      "*",
			PathPattern:      "*.woff",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlFonts,
			CacheQueryParams: true,
			Priority:         -101,
		},
		{
			ID:               "default-static-js",
			Name:             "Default: js",
			HostPattern:      "*",
			PathPattern:      "*.js",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlStatic,
			CacheQueryParams: true,
			Priority:         -110,
		},
		{
			ID:               "default-static-css",
			Name:             "Default: css",
			HostPattern:      "*",
			PathPattern:      "*.css",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlStatic,
			CacheQueryParams: true,
			Priority:         -111,
		},
		{
			ID:               "default-static-map",
			Name:             "Default: sourcemaps",
			HostPattern:      "*",
			PathPattern:      "*.map",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlStatic,
			CacheQueryParams: true,
			Priority:         -112,
		},
		{
			ID:               "default-images-png",
			Name:             "Default: images (png)",
			HostPattern:      "*",
			PathPattern:      "*.png",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlImages,
			CacheQueryParams: true,
			Priority:         -120,
		},
		{
			ID:               "default-images-jpg",
			Name:             "Default: images (jpg)",
			HostPattern:      "*",
			PathPattern:      "*.jpg",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlImages,
			CacheQueryParams: true,
			Priority:         -121,
		},
		{
			ID:               "default-images-jpeg",
			Name:             "Default: images (jpeg)",
			HostPattern:      "*",
			PathPattern:      "*.jpeg",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlImages,
			CacheQueryParams: true,
			Priority:         -122,
		},
		{
			ID:               "default-images-webp",
			Name:             "Default: images (webp)",
			HostPattern:      "*",
			PathPattern:      "*.webp",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlImages,
			CacheQueryParams: true,
			Priority:         -123,
		},
		{
			ID:               "default-images-gif",
			Name:             "Default: images (gif)",
			HostPattern:      "*",
			PathPattern:      "*.gif",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlImages,
			CacheQueryParams: true,
			Priority:         -124,
		},
		{
			ID:               "default-images-svg",
			Name:             "Default: images (svg)",
			HostPattern:      "*",
			PathPattern:      "*.svg*",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlImages,
			CacheQueryParams: true,
			Priority:         -125,
		},
		{
			ID:               "default-images-ico",
			Name:             "Default: images (ico)",
			HostPattern:      "*",
			PathPattern:      "*.ico",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlImages,
			CacheQueryParams: true,
			Priority:         -126,
		},
		{
			ID:               "default-downloads-zip",
			Name:             "Default: downloads (zip)",
			HostPattern:      "*",
			PathPattern:      "*.zip",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlDownloads,
			CacheQueryParams: true,
			Priority:         -130,
		},
		{
			ID:               "default-downloads-tar",
			Name:             "Default: downloads (tar/gz)",
			HostPattern:      "*",
			PathPattern:      "*.tar*",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlDownloads,
			CacheQueryParams: true,
			Priority:         -131,
		},
		{
			ID:               "default-misc-robots",
			Name:             "Default: misc (robots)",
			HostPattern:      "*",
			PathPattern:      "*/robots.txt",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlMisc,
			CacheQueryParams: true,
			Priority:         -140,
		},
		{
			ID:               "default-misc-favicon",
			Name:             "Default: misc (favicon)",
			HostPattern:      "*",
			PathPattern:      "*/favicon.ico",
			Methods:          []string{"GET", "HEAD"},
			TTLSeconds:       ttlMisc,
			CacheQueryParams: true,
			Priority:         -141,
		},
	}
}

// CompileAndStore compiles the configuration and stores it as a new version.
func (c *Compiler) CompileAndStore(ctx context.Context, createdBy string) (*store.ConfigVersion, error) {
	version, payload, err := c.Compile(ctx)
	if err != nil {
		return nil, err
	}

	// Calculate checksum
	checksum := sha256.Sum256(payload)
	checksumHex := hex.EncodeToString(checksum[:])

	cv := &store.ConfigVersion{
		Version:   version,
		Checksum:  checksumHex,
		Payload:   payload,
		CreatedAt: time.Now(),
		CreatedBy: createdBy,
	}

	if err := c.store.CreateConfigVersion(ctx, cv); err != nil {
		return nil, fmt.Errorf("store config version: %w", err)
	}

	log.Ctx(ctx).Info().
		Str("version", version).
		Str("created_by", createdBy).
		Msg("configuration version stored")

	return cv, nil
}

// GetLatestConfig returns the latest compiled configuration.
func (c *Compiler) GetLatestConfig(ctx context.Context) (*store.ConfigVersion, error) {
	return c.store.GetLatestConfigVersion(ctx)
}

// GetConfig returns a specific configuration version.
func (c *Compiler) GetConfig(ctx context.Context, version string) (*store.ConfigVersion, error) {
	return c.store.GetConfigVersion(ctx, version)
}

// hashString returns a short hex digest of s, used to derive stable
// per-entry rule IDs for IP blacklist/whitelist entries.
func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:4])
}

// secModeToAction maps a DomainSecurity preset key onto (action, captcha_type)
// consumable by the node WAF engine. "" action means "no rule needed" (off).
func secModeToAction(mode string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "off":
		return "", ""
	case "loose":
		// Loose → cheap JS challenge only.
		return "challenge", "js_challenge"
	case "js":
		return "challenge", "js_challenge"
	case "shield5s":
		return "shield", ""
	case "click":
		return "challenge", "click"
	case "click_easy":
		return "challenge", "click"
	case "slide":
		return "challenge", "slide"
	case "slide_easy":
		return "challenge", "slide"
	case "captcha":
		return "challenge", "slide_region"
	case "rotate":
		return "challenge", "rotate"
	case "custom":
		// Custom rules carry their own actions; no catch-all here.
		return "", ""
	default:
		return "challenge", "js_challenge"
	}
}

// shieldSecondsForMode returns the shield window for modes that use the
// node's shield action. 0 means "use node default".
func shieldSecondsForMode(mode string) int64 {
	if strings.EqualFold(strings.TrimSpace(mode), "shield5s") {
		return 5
	}
	return 0
}
