package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var DefaultPortalBase = "https://auth.lingcdn.cloud"
var DefaultLicensePubKey = ""
var BuildConfigLocked = "false"

func buildConfigLocked() bool {
	v := strings.ToLower(strings.TrimSpace(BuildConfigLocked))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// Config holds runtime settings for the control plane.
type Config struct {
	GRPCAddr                   string
	HTTPAddr                   string
	MetricsAddr                string
	MetricsToken               string
	DatabaseURL                string
	RedisURL                   string
	ServiceToken               string
	AdminBootstrapToken        string
	AuthSecret                 string
	AdminUsername              string
	AdminEmail                 string
	AdminPassword              string
	StoreBackend               string
	UpgradeAPI                 string
	ControlService             string
	ControlID                  string
	ControlUIDir               string
	TLSCertFile                string
	TLSKeyFile                 string
	TLSClientCA                string
	LogLevel                   string
	LogFormat                  string
	LogFile                    string
	WebhookSecret              string
	PortalReportSecret         string
	ElasticsearchURL           string
	ElasticsearchUser          string
	ElasticsearchPass          string
	ElasticsearchIndex         string
	PortalBase                 string
	UpgradePubKey              string
	PublicGRPCEndpoint         string
	PublicIP                   string
	ElasticsearchTSField       string
	ElasticsearchDomainField   string
	ElasticsearchBytesField    string
	CACertFile                 string
	CAKeyFile                  string
	ACMEEnable                 bool
	ACMEEmail                  string
	ACMECacheDir               string
	ACMEStaging                bool
	HTTPSAddr                  string
	ACMECertFile               string
	ACMEKeyFile                string
	LicenseFile                string
	LicenseMode                string
	LicenseStaticIndexURL      string
	LicenseGraceHours          int
	LicenseVerifyInterval      time.Duration
	SystemReportInterval       time.Duration
	LicensePubKey              string
	AllowInsecureLicensePubKey bool
	SMTPHost                   string
	SMTPPort                   int
	SMTPUser                   string
	SMTPPass                   string
	SMTPFrom                   string
	SMTPTLSEnable              bool
	SMTPInsecureSkipVerify     bool
	MaxMindLicenseKey          string
	GeoIPStorageDir            string
	GeoIPEdition               string
	GeoIPUpdateInterval        time.Duration
	CnameSuffix                string

	PaymentEnabled          bool   `json:"payment_enabled"`
	PaymentProvider         string `json:"payment_provider"`
	PaymentEPayURL          string `json:"payment_epay_url"`
	PaymentEPayPID          string `json:"payment_epay_pid"`
	PaymentEPayKey          string `json:"payment_epay_key"`
	PaymentEPayNotifyURL    string `json:"payment_epay_notify_url"`
	PaymentEPayReturnURL    string `json:"payment_epay_return_url"`
	PaymentMinRechargeCents int64  `json:"payment_min_recharge_cents"`
}

// LoadOptions controls how runtime config is loaded.
type LoadOptions struct {
	File       string
	AutoCreate bool
}

// LoadResult describes the resolved runtime config and how it was sourced.
type LoadResult struct {
	Config    Config
	File      string
	Generated bool
}

type valueSource interface {
	Lookup(key string) (string, bool)
}

type envSource struct{}

func (envSource) Lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

type mapSource map[string]string

func (m mapSource) Lookup(key string) (string, bool) {
	v, ok := m[normalizeConfigKey(key)]
	return v, ok
}

type layeredSource struct {
	sources []valueSource
}

func (l layeredSource) Lookup(key string) (string, bool) {
	for _, src := range l.sources {
		if src == nil {
			continue
		}
		if v, ok := src.Lookup(key); ok {
			return v, true
		}
	}
	return "", false
}

// Load reads configuration from environment variables with sane defaults.
func Load() Config {
	cfg := defaultConfig()
	applyOverrides(&cfg, envSource{}, buildConfigLocked())
	return cfg
}

// LoadWithOptions reads configuration from an optional YAML/JSON file and then
// applies environment variable overrides on top.
func LoadWithOptions(opts LoadOptions) (Config, error) {
	result, err := LoadDetailed(opts)
	if err != nil {
		return Config{}, err
	}
	return result.Config, nil
}

// LoadDetailed reads runtime config and also reports whether a config file was
// materialized automatically.
func LoadDetailed(opts LoadOptions) (LoadResult, error) {
	cfg := defaultConfig()
	locked := buildConfigLocked()
	baseForFile := cfg
	applyOverrides(&baseForFile, envSource{}, locked)

	var sources []valueSource
	sources = append(sources, envSource{})

	configPath, generated, err := resolveConfigFile(opts, baseForFile, locked)
	if err != nil {
		return LoadResult{}, err
	}
	if configPath != "" {
		fileValues, err := loadConfigFileValues(configPath)
		if err != nil {
			return LoadResult{}, err
		}
		sources = append(sources, mapSource(fileValues))
	}

	applyOverrides(&cfg, layeredSource{sources: sources}, locked)
	return LoadResult{
		Config:    cfg,
		File:      configPath,
		Generated: generated,
	}, nil
}

func defaultConfig() Config {
	return Config{
		GRPCAddr:                   ":9443",
		HTTPAddr:                   ":8080",
		MetricsAddr:                ":9100",
		MetricsToken:               "",
		DatabaseURL:                "postgres://lingcdn:strongpass@127.0.0.1:5432/lingcdn?sslmode=disable",
		RedisURL:                   "redis://127.0.0.1:6379/0",
		ServiceToken:               "",
		AdminBootstrapToken:        "",
		AuthSecret:                 "dev-secret-change-me",
		AdminUsername:              "admin",
		AdminEmail:                 "admin@lingcdn.cloud",
		AdminPassword:              "lingcdn123",
		StoreBackend:               "postgres",
		UpgradeAPI:                 "",
		ControlService:             "lingcdn-control",
		ControlID:                  "control",
		ControlUIDir:               "ui",
		TLSCertFile:                "",
		TLSKeyFile:                 "",
		TLSClientCA:                "",
		LogLevel:                   "info",
		LogFormat:                  "json",
		LogFile:                    "",
		WebhookSecret:              "lingcdn-webhook-secret-2024",
		PortalReportSecret:         "",
		ElasticsearchURL:           "",
		ElasticsearchUser:          "",
		ElasticsearchPass:          "",
		ElasticsearchIndex:         "cdn-access",
		PortalBase:                 DefaultPortalBase,
		UpgradePubKey:              "",
		PublicGRPCEndpoint:         "",
		PublicIP:                   "",
		ElasticsearchTSField:       "@timestamp",
		ElasticsearchDomainField:   "domain.keyword",
		ElasticsearchBytesField:    "bytes",
		CACertFile:                 "",
		CAKeyFile:                  "",
		ACMEEnable:                 false,
		ACMEEmail:                  "",
		ACMECacheDir:               "/etc/lingcdn/acme",
		ACMEStaging:                false,
		HTTPSAddr:                  ":443",
		ACMECertFile:               "",
		ACMEKeyFile:                "",
		LicenseFile:                "data/license.json",
		LicenseMode:                NormalizeLicenseMode("online"),
		LicenseStaticIndexURL:      "",
		LicenseGraceHours:          24,
		LicenseVerifyInterval:      5 * time.Minute,
		SystemReportInterval:       10 * time.Minute,
		LicensePubKey:              DefaultLicensePubKey,
		AllowInsecureLicensePubKey: false,
		SMTPHost:                   "",
		SMTPPort:                   587,
		SMTPUser:                   "",
		SMTPPass:                   "",
		SMTPFrom:                   "",
		SMTPTLSEnable:              true,
		SMTPInsecureSkipVerify:     false,
		MaxMindLicenseKey:          "",
		GeoIPStorageDir:            "data/geoip",
		GeoIPEdition:               "GeoLite2-City",
		GeoIPUpdateInterval:        168 * time.Hour,

		PaymentEnabled:          false,
		PaymentProvider:         "mock",
		PaymentEPayURL:          "",
		PaymentEPayPID:          "",
		PaymentEPayKey:          "",
		PaymentEPayNotifyURL:    "",
		PaymentEPayReturnURL:    "",
		PaymentMinRechargeCents: 100,
	}
}

func applyOverrides(cfg *Config, src valueSource, locked bool) {
	cfg.GRPCAddr = lookupString(src, "GRPC_ADDR", cfg.GRPCAddr)
	cfg.HTTPAddr = lookupString(src, "HTTP_ADDR", cfg.HTTPAddr)
	cfg.MetricsAddr = lookupString(src, "METRICS_ADDR", cfg.MetricsAddr)
	cfg.MetricsToken = lookupString(src, "METRICS_TOKEN", cfg.MetricsToken)
	cfg.DatabaseURL = lookupString(src, "DATABASE_URL", cfg.DatabaseURL)
	cfg.RedisURL = lookupString(src, "REDIS_URL", cfg.RedisURL)
	cfg.ServiceToken = lookupString(src, "SERVICE_TOKEN", cfg.ServiceToken)
	cfg.AdminBootstrapToken = lookupString(src, "ADMIN_BOOTSTRAP_TOKEN", cfg.AdminBootstrapToken)
	cfg.AuthSecret = lookupString(src, "AUTH_SECRET", cfg.AuthSecret)
	cfg.AdminUsername = lookupString(src, "ADMIN_USERNAME", cfg.AdminUsername)
	cfg.AdminEmail = lookupString(src, "ADMIN_EMAIL", cfg.AdminEmail)
	cfg.AdminPassword = lookupString(src, "ADMIN_PASSWORD", cfg.AdminPassword)
	cfg.StoreBackend = lookupString(src, "STORE_BACKEND", cfg.StoreBackend)
	cfg.UpgradeAPI = lookupString(src, "UPGRADE_API", cfg.UpgradeAPI)
	cfg.ControlService = lookupString(src, "CONTROL_SERVICE", cfg.ControlService)
	cfg.ControlID = lookupString(src, "CONTROL_ID", cfg.ControlID)
	cfg.ControlUIDir = lookupString(src, "CONTROL_UI_DIR", cfg.ControlUIDir)
	cfg.TLSCertFile = lookupString(src, "TLS_CERT_FILE", cfg.TLSCertFile)
	cfg.TLSKeyFile = lookupString(src, "TLS_KEY_FILE", cfg.TLSKeyFile)
	cfg.TLSClientCA = lookupString(src, "TLS_CLIENT_CA", cfg.TLSClientCA)
	cfg.LogLevel = lookupString(src, "LOG_LEVEL", cfg.LogLevel)
	cfg.LogFormat = lookupString(src, "LOG_FORMAT", cfg.LogFormat)
	cfg.LogFile = lookupString(src, "LOG_FILE", cfg.LogFile)
	cfg.WebhookSecret = lookupString(src, "WEBHOOK_SECRET", cfg.WebhookSecret)
	cfg.ElasticsearchURL = lookupString(src, "ES_URL", cfg.ElasticsearchURL)
	cfg.ElasticsearchUser = lookupString(src, "ES_USER", cfg.ElasticsearchUser)
	cfg.ElasticsearchPass = lookupString(src, "ES_PASS", cfg.ElasticsearchPass)
	cfg.ElasticsearchIndex = lookupString(src, "ES_INDEX_PREFIX", cfg.ElasticsearchIndex)
	cfg.UpgradePubKey = lookupString(src, "UPGRADE_PUBKEY", cfg.UpgradePubKey)
	cfg.PublicGRPCEndpoint = lookupString(src, "PUBLIC_GRPC_ENDPOINT", cfg.PublicGRPCEndpoint)
	cfg.PublicIP = lookupString(src, "PUBLIC_IP", cfg.PublicIP)
	cfg.ElasticsearchTSField = lookupString(src, "ES_FIELD_TIMESTAMP", cfg.ElasticsearchTSField)
	cfg.ElasticsearchDomainField = lookupString(src, "ES_FIELD_DOMAIN", cfg.ElasticsearchDomainField)
	cfg.ElasticsearchBytesField = lookupString(src, "ES_FIELD_BYTES", cfg.ElasticsearchBytesField)
	cfg.CACertFile = lookupString(src, "CA_CERT_FILE", cfg.CACertFile)
	cfg.CAKeyFile = lookupString(src, "CA_KEY_FILE", cfg.CAKeyFile)
	cfg.ACMEEnable = lookupBool(src, "ACME_ENABLE", cfg.ACMEEnable)
	cfg.ACMEEmail = lookupString(src, "ACME_EMAIL", cfg.ACMEEmail)
	cfg.ACMECacheDir = lookupString(src, "ACME_CACHE_DIR", cfg.ACMECacheDir)
	cfg.ACMEStaging = lookupBool(src, "ACME_STAGING", cfg.ACMEStaging)
	cfg.HTTPSAddr = lookupString(src, "HTTPS_ADDR", cfg.HTTPSAddr)
	cfg.ACMECertFile = lookupString(src, "ACME_CERT_FILE", cfg.ACMECertFile)
	cfg.ACMEKeyFile = lookupString(src, "ACME_KEY_FILE", cfg.ACMEKeyFile)
	cfg.LicenseFile = lookupString(src, "LICENSE_FILE", cfg.LicenseFile)
	cfg.LicenseMode = NormalizeLicenseMode(cfg.LicenseMode)
	cfg.LicenseStaticIndexURL = ""
	cfg.LicenseGraceHours = lookupInt(src, "LICENSE_GRACE_HOURS", cfg.LicenseGraceHours)
	cfg.LicenseVerifyInterval = lookupDuration(src, "LICENSE_VERIFY_INTERVAL", cfg.LicenseVerifyInterval)
	cfg.SystemReportInterval = lookupDuration(src, "SYSTEM_REPORT_INTERVAL", cfg.SystemReportInterval)
	cfg.AllowInsecureLicensePubKey = false
	cfg.SMTPHost = lookupString(src, "SMTP_HOST", cfg.SMTPHost)
	cfg.SMTPPort = lookupInt(src, "SMTP_PORT", cfg.SMTPPort)
	cfg.SMTPUser = lookupString(src, "SMTP_USER", cfg.SMTPUser)
	cfg.SMTPPass = lookupString(src, "SMTP_PASS", cfg.SMTPPass)
	cfg.SMTPFrom = lookupString(src, "SMTP_FROM", cfg.SMTPFrom)
	cfg.SMTPTLSEnable = lookupBool(src, "SMTP_TLS", cfg.SMTPTLSEnable)
	cfg.SMTPInsecureSkipVerify = lookupBool(src, "SMTP_INSECURE_SKIP_VERIFY", cfg.SMTPInsecureSkipVerify)
	cfg.MaxMindLicenseKey = lookupString(src, "MAXMIND_LICENSE_KEY", cfg.MaxMindLicenseKey)
	cfg.GeoIPStorageDir = lookupString(src, "GEOIP_STORAGE_DIR", cfg.GeoIPStorageDir)
	cfg.GeoIPEdition = lookupString(src, "GEOIP_EDITION", cfg.GeoIPEdition)
	cfg.GeoIPUpdateInterval = lookupDuration(src, "GEOIP_UPDATE_INTERVAL", cfg.GeoIPUpdateInterval)
	cfg.CnameSuffix = lookupString(src, "CNAME_SUFFIX", cfg.CnameSuffix)

	cfg.PaymentEnabled = lookupBool(src, "PAYMENT_ENABLED", cfg.PaymentEnabled)
	cfg.PaymentProvider = lookupString(src, "PAYMENT_PROVIDER", cfg.PaymentProvider)
	cfg.PaymentEPayURL = lookupString(src, "PAYMENT_EPAY_URL", cfg.PaymentEPayURL)
	cfg.PaymentEPayPID = lookupString(src, "PAYMENT_EPAY_PID", cfg.PaymentEPayPID)
	cfg.PaymentEPayKey = lookupString(src, "PAYMENT_EPAY_KEY", cfg.PaymentEPayKey)
	cfg.PaymentEPayNotifyURL = lookupString(src, "PAYMENT_EPAY_NOTIFY_URL", cfg.PaymentEPayNotifyURL)
	cfg.PaymentEPayReturnURL = lookupString(src, "PAYMENT_EPAY_RETURN_URL", cfg.PaymentEPayReturnURL)
	cfg.PaymentMinRechargeCents = lookupInt64(src, "PAYMENT_MIN_RECHARGE_CENTS", cfg.PaymentMinRechargeCents)

	cfg.PortalReportSecret = lookupString(src, "PORTAL_REPORT_SECRET", cfg.WebhookSecret)
	cfg.PortalBase = DefaultPortalBase
	if !locked {
		cfg.LicensePubKey = lookupString(src, "LICENSE_PUBKEY", cfg.LicensePubKey)
	}

	// 当 LICENSE_PUBKEY 未配置时，回退使用 UPGRADE_PUBKEY（两者共用同一密钥对）
	if strings.TrimSpace(cfg.LicensePubKey) == "" && strings.TrimSpace(cfg.UpgradePubKey) != "" {
		cfg.LicensePubKey = cfg.UpgradePubKey
	}
}

func resolveConfigFile(opts LoadOptions, base Config, locked bool) (string, bool, error) {
	candidate := strings.TrimSpace(opts.File)
	if candidate == "" {
		candidate = strings.TrimSpace(os.Getenv("CONTROL_CONFIG_FILE"))
	}
	if candidate == "" {
		candidate = strings.TrimSpace(os.Getenv("CONFIG_FILE"))
	}
	if candidate != "" {
		if isRegularFile(candidate) {
			return candidate, false, nil
		}
		if opts.AutoCreate {
			path, err := writeGeneratedConfig(candidate, base, locked)
			return path, err == nil, err
		}
		return "", false, fmt.Errorf("config file not found: %s", candidate)
	}

	for _, autoPath := range autoConfigCandidates() {
		if isRegularFile(autoPath) {
			return autoPath, false, nil
		}
	}
	if !opts.AutoCreate {
		return "", false, nil
	}

	target, err := defaultGeneratedConfigPath()
	if err != nil {
		return "", false, err
	}
	path, err := writeGeneratedConfig(target, base, locked)
	return path, err == nil, err
}

func autoConfigCandidates() []string {
	names := []string{"config.yaml", "config.yml", "config.json"}
	var dirs []string

	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if exeDir != "" && !containsPath(dirs, exeDir) {
			dirs = append(dirs, exeDir)
		}
	}

	var candidates []string
	for _, dir := range dirs {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}
	return candidates
}

func loadConfigFileValues(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}

	values := make(map[string]string, len(raw))
	for key, value := range raw {
		text, err := stringifyConfigValue(value)
		if err != nil {
			return nil, fmt.Errorf("invalid value for %s in %s: %w", key, path, err)
		}
		values[normalizeConfigKey(key)] = text
	}
	return values, nil
}

func stringifyConfigValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int8, int16, int32, int64:
		return fmt.Sprint(v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v), nil
	case float32, float64:
		return fmt.Sprint(v), nil
	case []any:
		return "", errors.New("array values are not supported; use a scalar")
	case map[string]any:
		return "", errors.New("nested objects are not supported; use flat KEY: value pairs")
	case map[any]any:
		return "", errors.New("nested objects are not supported; use flat KEY: value pairs")
	default:
		return fmt.Sprint(v), nil
	}
}

func isRegularFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func defaultGeneratedConfigPath() (string, error) {
	if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		return filepath.Join(wd, "config.yaml"), nil
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if strings.TrimSpace(exeDir) != "" {
			return filepath.Join(exeDir, "config.yaml"), nil
		}
	}
	return "", errors.New("unable to determine config file path")
}

func writeGeneratedConfig(path string, base Config, locked bool) (string, error) {
	cfg := prepareGeneratedConfig(base, locked)
	content, err := renderConfigYAML(cfg, locked)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func prepareGeneratedConfig(base Config, locked bool) Config {
	cfg := base
	cfg.ServiceToken = ensureGeneratedSecret(cfg.ServiceToken, 32, "")
	cfg.AdminBootstrapToken = ensureGeneratedSecret(cfg.AdminBootstrapToken, 32, "")
	cfg.AuthSecret = ensureGeneratedSecret(cfg.AuthSecret, 32, "dev-secret-change-me")
	cfg.WebhookSecret = ensureGeneratedSecret(cfg.WebhookSecret, 32, "lingcdn-webhook-secret-2024")
	if strings.TrimSpace(cfg.PortalReportSecret) == "" || strings.TrimSpace(cfg.PortalReportSecret) == strings.TrimSpace(cfg.WebhookSecret) {
		cfg.PortalReportSecret = cfg.WebhookSecret
	}
	cfg.AdminPassword = ensureGeneratedSecret(cfg.AdminPassword, 18, "lingcdn123")
	if !locked {
		cfg.LicensePubKey = strings.TrimSpace(cfg.LicensePubKey)
	}
	return cfg
}

func ensureGeneratedSecret(current string, size int, unsafeValues ...string) string {
	value := strings.TrimSpace(current)
	if value != "" && !matchesAny(value, unsafeValues) {
		return value
	}
	token, err := randomSecret(size)
	if err != nil {
		return value
	}
	return token
}

func matchesAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func randomSecret(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func renderConfigYAML(cfg Config, locked bool) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# LingCDN control plane config generated automatically.\n")
	b.WriteString("# Edit this file and restart the service when needed.\n\n")
	writeGroup(&b, "Database and cache", []configEntry{
		{Key: "DATABASE_URL", Value: cfg.DatabaseURL},
		{Key: "REDIS_URL", Value: cfg.RedisURL},
	})
	writeGroup(&b, "Listeners", []configEntry{
		{Key: "HTTP_ADDR", Value: cfg.HTTPAddr},
		{Key: "GRPC_ADDR", Value: cfg.GRPCAddr},
		{Key: "METRICS_ADDR", Value: cfg.MetricsAddr},
		{Key: "METRICS_TOKEN", Value: cfg.MetricsToken},
		{Key: "HTTPS_ADDR", Value: cfg.HTTPSAddr},
	})
	writeGroup(&b, "Control plane", []configEntry{
		{Key: "STORE_BACKEND", Value: cfg.StoreBackend},
		{Key: "CONTROL_SERVICE", Value: cfg.ControlService},
		{Key: "CONTROL_ID", Value: cfg.ControlID},
		{Key: "CONTROL_UI_DIR", Value: cfg.ControlUIDir},
		{Key: "SERVICE_TOKEN", Value: cfg.ServiceToken},
		{Key: "ADMIN_BOOTSTRAP_TOKEN", Value: cfg.AdminBootstrapToken},
		{Key: "AUTH_SECRET", Value: cfg.AuthSecret},
		{Key: "WEBHOOK_SECRET", Value: cfg.WebhookSecret},
		{Key: "PORTAL_REPORT_SECRET", Value: cfg.PortalReportSecret},
	})
	writeGroup(&b, "Admin bootstrap", []configEntry{
		{Key: "ADMIN_USERNAME", Value: cfg.AdminUsername},
		{Key: "ADMIN_EMAIL", Value: cfg.AdminEmail},
		{Key: "ADMIN_PASSWORD", Value: cfg.AdminPassword},
	})
	writeGroup(&b, "Network identity", []configEntry{
		{Key: "PUBLIC_GRPC_ENDPOINT", Value: cfg.PublicGRPCEndpoint},
		{Key: "PUBLIC_IP", Value: cfg.PublicIP},
		{Key: "PORTAL_BASE", Value: cfg.PortalBase, Omit: locked},
		{Key: "UPGRADE_API", Value: cfg.UpgradeAPI},
		{Key: "UPGRADE_PUBKEY", Value: cfg.UpgradePubKey},
	})
	writeGroup(&b, "Logging", []configEntry{
		{Key: "LOG_LEVEL", Value: cfg.LogLevel},
		{Key: "LOG_FORMAT", Value: cfg.LogFormat},
		{Key: "LOG_FILE", Value: cfg.LogFile},
	})
	writeGroup(&b, "TLS and ACME", []configEntry{
		{Key: "TLS_CERT_FILE", Value: cfg.TLSCertFile},
		{Key: "TLS_KEY_FILE", Value: cfg.TLSKeyFile},
		{Key: "TLS_CLIENT_CA", Value: cfg.TLSClientCA},
		{Key: "CA_CERT_FILE", Value: cfg.CACertFile},
		{Key: "CA_KEY_FILE", Value: cfg.CAKeyFile},
		{Key: "ACME_ENABLE", Value: cfg.ACMEEnable},
		{Key: "ACME_EMAIL", Value: cfg.ACMEEmail},
		{Key: "ACME_CACHE_DIR", Value: cfg.ACMECacheDir},
		{Key: "ACME_STAGING", Value: cfg.ACMEStaging},
		{Key: "ACME_CERT_FILE", Value: cfg.ACMECertFile},
		{Key: "ACME_KEY_FILE", Value: cfg.ACMEKeyFile},
	})
	writeGroup(&b, "License and auth", []configEntry{
		{Key: "LICENSE_FILE", Value: cfg.LicenseFile},
		{Key: "LICENSE_MODE", Value: cfg.LicenseMode},
		{Key: "LICENSE_STATIC_INDEX_URL", Value: cfg.LicenseStaticIndexURL},
		{Key: "LICENSE_GRACE_HOURS", Value: cfg.LicenseGraceHours},
		{Key: "LICENSE_VERIFY_INTERVAL", Value: cfg.LicenseVerifyInterval.String()},
		{Key: "LICENSE_PUBKEY", Value: cfg.LicensePubKey, Omit: locked},
		{Key: "ALLOW_INSECURE_LICENSE_PUBKEY", Value: cfg.AllowInsecureLicensePubKey},
	})
	writeGroup(&b, "Mail and GeoIP", []configEntry{
		{Key: "SMTP_HOST", Value: cfg.SMTPHost},
		{Key: "SMTP_PORT", Value: cfg.SMTPPort},
		{Key: "SMTP_USER", Value: cfg.SMTPUser},
		{Key: "SMTP_PASS", Value: cfg.SMTPPass},
		{Key: "SMTP_FROM", Value: cfg.SMTPFrom},
		{Key: "SMTP_TLS", Value: cfg.SMTPTLSEnable},
		{Key: "SMTP_INSECURE_SKIP_VERIFY", Value: cfg.SMTPInsecureSkipVerify},
		{Key: "MAXMIND_LICENSE_KEY", Value: cfg.MaxMindLicenseKey},
		{Key: "GEOIP_STORAGE_DIR", Value: cfg.GeoIPStorageDir},
		{Key: "GEOIP_EDITION", Value: cfg.GeoIPEdition},
		{Key: "GEOIP_UPDATE_INTERVAL", Value: cfg.GeoIPUpdateInterval.String()},
		{Key: "SYSTEM_REPORT_INTERVAL", Value: cfg.SystemReportInterval.String()},
	})
	writeGroup(&b, "Elasticsearch", []configEntry{
		{Key: "ES_URL", Value: cfg.ElasticsearchURL},
		{Key: "ES_USER", Value: cfg.ElasticsearchUser},
		{Key: "ES_PASS", Value: cfg.ElasticsearchPass},
		{Key: "ES_INDEX_PREFIX", Value: cfg.ElasticsearchIndex},
		{Key: "ES_FIELD_TIMESTAMP", Value: cfg.ElasticsearchTSField},
		{Key: "ES_FIELD_DOMAIN", Value: cfg.ElasticsearchDomainField},
		{Key: "ES_FIELD_BYTES", Value: cfg.ElasticsearchBytesField},
	})
	return []byte(b.String()), nil
}

type configEntry struct {
	Key   string
	Value any
	Omit  bool
}

func writeGroup(b *strings.Builder, title string, entries []configEntry) {
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n")
	for _, entry := range entries {
		if entry.Omit {
			continue
		}
		b.WriteString(entry.Key)
		b.WriteString(": ")
		b.WriteString(mustYAMLScalar(entry.Value))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func mustYAMLScalar(value any) string {
	data, err := yaml.Marshal(value)
	if err != nil {
		return "\"\""
	}
	return strings.TrimSpace(string(data))
}

func containsPath(paths []string, target string) bool {
	for _, path := range paths {
		if filepath.Clean(path) == filepath.Clean(target) {
			return true
		}
	}
	return false
}

// ResolveControlUIDir returns the first usable UI directory.
// Priority:
// 1. Explicit configured path
// 2. Current working directory / ui
// 3. Executable directory / ui
// 4. Current working directory / data/control-ui
// 5. Executable directory / data/control-ui
func ResolveControlUIDir(configured string) string {
	var candidates []string

	if configured = strings.TrimSpace(configured); configured != "" {
		candidates = append(candidates, configured)
	}

	if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		candidates = append(candidates,
			filepath.Join(wd, "ui"),
			filepath.Join(wd, "data", "control-ui"),
		)
	}
	if exePath, err := os.Executable(); err == nil {
		if exeDir := strings.TrimSpace(filepath.Dir(exePath)); exeDir != "" {
			candidates = append(candidates,
				filepath.Join(exeDir, "ui"),
				filepath.Join(exeDir, "data", "control-ui"),
			)
		}
	}

	for _, candidate := range candidates {
		if isValidControlUIDir(candidate) {
			return candidate
		}
	}
	return strings.TrimSpace(configured)
}

func isValidControlUIDir(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return false
	}
	indexPath := filepath.Join(path, "index.html")
	indexInfo, err := os.Stat(indexPath)
	return err == nil && !indexInfo.IsDir()
}

func lookupString(src valueSource, key, def string) string {
	if v, ok := src.Lookup(key); ok {
		return v
	}
	return def
}

func lookupBool(src valueSource, key string, def bool) bool {
	v, ok := src.Lookup(key)
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func lookupInt64(src valueSource, key string, def int64) int64 {
	v, ok := src.Lookup(key)
	if !ok {
		return def
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
		return n
	}
	return def
}

func lookupInt(src valueSource, key string, def int) int {
	v, ok := src.Lookup(key)
	if !ok {
		return def
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return def
}

func lookupDuration(src valueSource, key string, def time.Duration) time.Duration {
	v, ok := src.Lookup(key)
	if !ok {
		return def
	}
	if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil {
		return d
	}
	return def
}

func normalizeConfigKey(key string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_", " ", "_")
	return strings.ToUpper(strings.TrimSpace(replacer.Replace(key)))
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func getEnvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return def
}

func NormalizeLicenseMode(mode string) string {
	return "online"
}
