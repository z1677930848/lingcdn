package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/lingcdn/control/internal/cert"
	"github.com/lingcdn/control/internal/compiler"
	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/ddos"
	"github.com/lingcdn/control/internal/metrics"
	"github.com/lingcdn/control/internal/nodehub"
	"github.com/lingcdn/control/internal/publisher"
	"github.com/lingcdn/control/internal/purge"
	"github.com/lingcdn/control/internal/store"
	controlpb "github.com/lingcdn/control/proto/gen"
)

// Servers groups control plane listeners.
type Servers struct {
	cfg         config.Config
	hub         *nodehub.Hub
	compiler    *compiler.Compiler
	publisher   *publisher.Publisher
	purge       *purge.Service
	cert        *cert.Manager
	store       store.Store
	metrics     *metrics.Metrics
	xdpStore    *ddos.XdpStore
	nodeMonitor *nodeMonitorRecorder
	sseBroker   *sseBroker
	grpcServer  *grpc.Server
	acmeMgr     *autocert.Manager
	// acmeIssuer is a separate autocert.Manager used only for on-demand
	// certificate issuance via the /api/certificates/acme endpoint. It is
	// lazy-initialised (see ensureACMEIssuer) so users can issue Let's
	// Encrypt certs for their managed domains regardless of whether the
	// control plane itself runs under ACME (s.acmeMgr / ACMEEnable).
	acmeIssuer   *autocert.Manager
	acmeIssuerMu sync.Mutex
	dnsSyncMu    sync.Mutex
	dnsSyncAt    time.Time
	startedAt    time.Time
	// Upgrade versions cache.
	upgradeVersions   map[string]*UpgradeVersion
	upgradeVersionsMu sync.RWMutex

	// license state (in memory)
	licenseMu sync.RWMutex
	license   licenseState

	licenseFile string

	// cached public IP for license display
	cachedPublicIP   string
	cachedPublicIPAt time.Time

	geoip *GeoIPManager

	// IP-based rate limiters for sensitive auth endpoints. Protects against
	// credential brute-forcing and verification-code spam. Initialized in
	// initRateLimiters(), which is invoked lazily on first request.
	authLimiter     *ipRateLimiter // login
	registerLimiter *ipRateLimiter // register
	emailLimiter    *ipRateLimiter // register-email-request, password-reset-request
	rateLimitOnce   sync.Once

	// Async writer for WAF bans reported by nodes via heartbeat. This keeps the
	// heartbeat path free of synchronous per-ban database round-trips and caps
	// the blast radius when a single node floods us with ban reports.
	wafBanQ *wafBanQueue
}

// initRateLimiters lazily constructs per-IP limiters the first time they are
// needed. Kept lazy so tests that construct a zero-value Servers don't need to
// call a separate init step.
func (s *Servers) initRateLimiters() {
	s.rateLimitOnce.Do(func() {
		// login: 20 attempts / minute per IP; bursts of 20 are fine for humans
		// retrying, but blocks credential-stuffing and brute-force runs.
		s.authLimiter = newIPRateLimiter(20, 3*time.Second)
		// register: 5 attempts / minute per IP to curb account spam.
		s.registerLimiter = newIPRateLimiter(5, 12*time.Second)
		// email code requests: 3 per minute per IP; codes cost email traffic.
		s.emailLimiter = newIPRateLimiter(3, 20*time.Second)
	})
}

// --- DNS handlers --- extracted to handlers_dns.go

// --- Auth/Captcha --- handlers extracted to handlers_auth.go

// --- systemReportLoop/reportSystemOnce --- extracted to background_jobs.go

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func resolveBuildMetadata() (string, string) {
	buildTime := strings.TrimSpace(os.Getenv("BUILD_TIME"))
	buildHash := firstNonEmpty(
		os.Getenv("BUILD_HASH"),
		os.Getenv("GIT_COMMIT"),
		os.Getenv("BUILD_COMMIT"),
		os.Getenv("COMMIT_SHA"),
	)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.time":
				if buildTime == "" {
					buildTime = strings.TrimSpace(setting.Value)
				}
			case "vcs.revision":
				if buildHash == "" {
					buildHash = strings.TrimSpace(setting.Value)
				}
			}
		}
	}
	return buildTime, buildHash
}

// --- Overview --- handlers/types extracted to handlers_overview.go

// domainView is the enriched payload for website/domain management.
type domainView struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	UserID                 string            `json:"user_id,omitempty"`
	LineGroupID            string            `json:"line_group_id,omitempty"`
	LineGroupName          string            `json:"line_group_name,omitempty"`
	OriginID               string            `json:"origin_id,omitempty"`
	OriginName             string            `json:"origin_name,omitempty"`
	OriginAddresses        []string          `json:"origin_addresses,omitempty"`
	OriginScheme           string            `json:"origin_scheme,omitempty"`
	OriginPort             int32             `json:"origin_port,omitempty"`
	OriginHostMode         string            `json:"origin_host_mode,omitempty"`
	OriginHost             string            `json:"origin_host,omitempty"`
	OriginTimeoutMs        int64             `json:"origin_timeout_ms,omitempty"`
	OriginConnectTimeoutMs int64             `json:"origin_connect_timeout_ms,omitempty"`
	CertID                 string            `json:"cert_id,omitempty"`
	CertName               string            `json:"cert_name,omitempty"`
	CertDomain             string            `json:"cert_domain,omitempty"`
	HTTPSEnabled           bool              `json:"https_enabled"`
	HTTP2Enabled           bool              `json:"http2_enabled"`
	ListenPort             int               `json:"listen_port,omitempty"`
	CNAME                  string            `json:"cname,omitempty"`
	ErrorPages             []store.ErrorPage `json:"error_pages,omitempty"`
	Enabled                bool              `json:"enabled"`
	CacheEnabled           bool              `json:"cache_enabled"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
}

type upgradeInfo struct {
	CurrentVersion  string        `json:"current_version"`
	LatestVersion   string        `json:"latest_version"`
	Channel         string        `json:"channel"`
	Mode            string        `json:"mode"` // manual | auto
	Checksum        string        `json:"checksum,omitempty"`
	DownloadURL     string        `json:"download_url,omitempty"`
	Changelog       string        `json:"changelog,omitempty"`
	Signature       string        `json:"signature,omitempty"`
	SigAlg          string        `json:"sig_alg,omitempty"`
	SigTarget       string        `json:"sig_target,omitempty"`
	PubKey          string        `json:"pubkey,omitempty"`
	NodeLatest      string        `json:"node_latest_version,omitempty"`
	NodeLatestAMD64 string        `json:"node_latest_amd64,omitempty"`
	NodeLatestARM64 string        `json:"node_latest_arm64,omitempty"`
	Nodes           []upgradeNode `json:"nodes"`
	Notes           []string      `json:"notes,omitempty"`
	CheckedAt       time.Time     `json:"checked_at"`
	Source          string        `json:"source,omitempty"`
}

type upgradeNode struct {
	ID             string `json:"id"`
	Hostname       string `json:"hostname"`
	CurrentVersion string `json:"current_version"`
	TargetVersion  string `json:"target_version"`
	Status         string `json:"status"` // up_to_date | upgrade_needed | unknown
}

type upgradeTask struct {
	ID            string       `json:"id"`
	TargetVersion string       `json:"target_version"`
	Channel       string       `json:"channel,omitempty"`
	NodeIDs       []string     `json:"node_ids"`
	Status        string       `json:"status"` // pending | running | success | failed
	Type          string       `json:"type"`   // control | node
	CreatedAt     time.Time    `json:"created_at"`
	Logs          []upgradeLog `json:"logs"`
}

type upgradeLog struct {
	Timestamp time.Time `json:"ts"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	NodeID    string    `json:"node_id,omitempty"`
}

var (
	upgradeTaskMu   sync.Mutex
	upgradeTaskList = make(map[string]*upgradeTask)
)

// New builds the server bundle with all dependencies injected.
func New(cfg config.Config, hub *nodehub.Hub, compiler *compiler.Compiler, publisher *publisher.Publisher, purge *purge.Service, cert *cert.Manager, store store.Store, m *metrics.Metrics) *Servers {
	recorder := newNodeMonitorRecorder()
	return &Servers{
		cfg:             cfg,
		hub:             hub,
		compiler:        compiler,
		publisher:       publisher,
		purge:           purge,
		cert:            cert,
		store:           store,
		metrics:         m,
		xdpStore:        ddos.NewXdpStore(),
		nodeMonitor:     recorder,
		sseBroker:       newSSEBroker(recorder, store),
		acmeMgr:         nil,
		startedAt:       time.Now(),
		upgradeVersions: make(map[string]*UpgradeVersion),
		license: licenseState{
			Status:    "unlicensed",
			UpdatedAt: time.Now(),
			Reason:    "system unlicensed",
		},
		licenseFile: cfg.LicenseFile,
		geoip:       NewGeoIPManager(cfg),
	}
}

// --- ccPolicyPayload/handleCCPolicy --- moved to handlers_waf.go

// Serve starts gRPC and HTTP servers and blocks until context is canceled or an error occurs.
func (s *Servers) Serve(ctx context.Context) error {
	// Async writer for WAF bans reported via heartbeat. Buffers up to 4096 bans
	// and flushes in batches of 128 every 250ms; on shutdown it drains best-effort.
	s.wafBanQ = newWAFBanQueue(s.store, 4096, 128, 250*time.Millisecond)
	s.wafBanQ.start(ctx)
	defer s.wafBanQ.shutdown()

	nodeCtl := newNodeControlServer(s.hub, s.compiler, s.publisher, s.purge, s.store, s.xdpStore, s.cfg, s.triggerDNSSync, s.notifyNodeOffline, s.licenseAllowsNewNode, s.nodeMonitor, s.sseBroker.notify, s)
	nodeCtl.wafBanQ = s.wafBanQ

	s.grpcServer = grpc.NewServer()
	controlpb.RegisterNodeControlServer(s.grpcServer, nodeCtl)
	controlpb.RegisterAdminControlServer(s.grpcServer, newAdminControlServer(s.cfg, s.store, s.publisher, s.purge))
	reflection.Register(s.grpcServer)

	lis, err := net.Listen("tcp", s.cfg.GRPCAddr)
	if err != nil {
		return err
	}

	// load license from file (best effort)
	_ = s.loadLicenseFromStore()
	_ = s.loadLicenseFromFile()
	if _, err := s.verifyLicenseOnce(context.Background()); err != nil {
		log.Warn().Err(err).Msg("initial license verify failed")
	}
	// start license verify loop
	go s.licenseVerifyLoop(ctx)
	go s.systemReportLoop(ctx)
	go s.nodeActiveMonitorLoop(ctx)
	go s.sseBroker.run()
	go s.dataRetentionLoop(ctx)
	go s.upgradeTaskSweeper(ctx)
	// Evicts nodehub sessions for hosts that are genuinely gone. Paired
	// with nodehub.ClearConfigStream, which preserves sessions across
	// transient stream drops so reconnects don't lose their hub binding.
	go s.hubSessionSweeper(ctx)
	// Daily sweep to auto-renew ACME certs nearing expiry. Without this,
	// Let's Encrypt certs issued through the UI silently expire at day 90
	// and users find out when TLS starts failing in production.
	go s.certificateRenewalLoop(ctx)
	// Rehydrate any pending node-upgrade commands from the store. Without
	// this, a control-plane restart between "operator clicks upgrade" and
	// "node heartbeats the next time" silently drops the command, stranding
	// the task in "running" until it times out 30 minutes later.
	rehydrateNodeUpgradeCommands(ctx, s.store)
	if s.geoip != nil {
		s.geoip.Start(ctx)
	}

	adminMux := s.adminMux()
	adminSrv := &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           adminMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	var httpsSrv *http.Server
	metricsSrv := &http.Server{
		Addr:              s.cfg.MetricsAddr,
		Handler:           s.metricsMux(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	errCh := make(chan error, 3)

	go func() {
		log.Info().Str("addr", s.cfg.GRPCAddr).Msg("starting gRPC server")
		if err := s.grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
		}
	}()

	if s.cfg.HTTPAddr != "" && !s.cfg.ACMEEnable {
		go func() {
			log.Info().Str("addr", s.cfg.HTTPAddr).Msg("starting HTTP server")
			if err := serveHTTP(adminSrv); err != nil {
				errCh <- err
			}
		}()
	}

	if s.cfg.ACMEEnable {
		s.acmeMgr = &autocert.Manager{
			Cache:      autocert.DirCache(s.cfg.ACMECacheDir),
			Prompt:     autocert.AcceptTOS,
			HostPolicy: s.acmeHostPolicy(),
			Email:      s.cfg.ACMEEmail,
		}
		if s.cfg.ACMEStaging {
			s.acmeMgr.Client = &acme.Client{DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory"}
		}

		// HTTP-01 challenge listener on :80
		go func() {
			addr := ":80"
			log.Info().Str("addr", addr).Msg("starting ACME HTTP challenge listener")
			challengeSrv := &http.Server{
				Addr: addr,
				Handler: s.acmeMgr.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
				})),
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       10 * time.Second,
				WriteTimeout:      10 * time.Second,
			}
			if err := challengeSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("acme http server: %w", err)
			}
		}()

		httpsSrv = &http.Server{
			Addr:              s.cfg.HTTPSAddr,
			Handler:           adminMux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			TLSConfig: &tls.Config{
				GetCertificate: s.acmeMgr.GetCertificate,
			},
		}
		go func() {
			log.Info().Str("addr", s.cfg.HTTPSAddr).Msg("starting HTTPS server (ACME)")
			if err := httpsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}

	if s.cfg.MetricsAddr != "" {
		go func() {
			log.Info().Str("addr", s.cfg.MetricsAddr).Msg("starting metrics server")
			if err := serveHTTP(metricsSrv); err != nil {
				errCh <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		log.Info().Msg("shutdown requested")
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.grpcServer.GracefulStop()
	_ = adminSrv.Shutdown(shutdownCtx)
	_ = metricsSrv.Shutdown(shutdownCtx)

	return nil
}

func serveHTTP(srv *http.Server) error {
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// --- adminMux --- extracted to router.go

// --- tryServeControlUI --- extracted to webui.go

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Servers) withMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.metrics == nil {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		s.metrics.RecordHTTPRequest(r.Method, path, strconv.Itoa(rw.statusCode), duration)
	})
}

// withBodyLimit wraps an http.Handler with a 10MB request body size limit.
func (s *Servers) withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
		next.ServeHTTP(w, r)
	})
}

func normalizePath(path string) string {
	// Normalize paths with IDs to reduce cardinality
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if isUUID(part) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (s *Servers) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := authenticateRequest(s.cfg, s.store, r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
			return
		}
		if allowed, reason := s.checkLicenseForHTTP(r.URL.Path); !allowed {
			writeJSON(w, http.StatusPaymentRequired, map[string]any{"error": reason, "status": s.currentLicenseStatus().Status})
			return
		}
		req := r.WithContext(ctx)
		if shouldAuditRequest(ctx, req) {
			rec := &statusRecorder{ResponseWriter: w}
			next(rec, req)
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			logStatus := "success"
			if status >= 400 {
				logStatus = "failed"
			}
			userID, username := s.resolveLogUser(ctx)
			s.writeSystemLog(ctx, inferAuditLogType(req), logStatus, buildAuditMessage(req), userID, username, getRequestIP(req))
			return
		}
		next(w, req)
	}
}

// withAdmin wraps withAuth and additionally requires the caller to have the
// "admin" role. Handlers protected by this middleware do NOT need to repeat
// the `if role != "admin"` check; the middleware rejects non-admin requests
// before invoking the handler.
func (s *Servers) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r.Context()) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "仅管理员可操作"})
			return
		}
		next(w, r)
	})
}

// withAuthNoLicense skips license gating for selected endpoints.
func (s *Servers) withAuthNoLicense(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := authenticateRequest(s.cfg, s.store, r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
			return
		}
		req := r.WithContext(ctx)
		if shouldAuditRequest(ctx, req) {
			rec := &statusRecorder{ResponseWriter: w}
			next(rec, req)
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			logStatus := "success"
			if status >= 400 {
				logStatus = "failed"
			}
			userID, username := s.resolveLogUser(ctx)
			s.writeSystemLog(ctx, inferAuditLogType(req), logStatus, buildAuditMessage(req), userID, username, getRequestIP(req))
			return
		}
		next(w, req)
	}
}

// checkLicenseForHTTP checks whether endpoint can bypass license gating.
func (s *Servers) checkLicenseForHTTP(path string) (bool, string) {
	whitelist := []string{
		"/api/auth/",
		"/api/license/",
		"/api/geoip/",
	}
	for _, p := range whitelist {
		if strings.HasPrefix(path, p) {
			return true, ""
		}
	}
	// Development bypass: only when BOTH the explicit opt-in env is set AND
	// the store backend is the ephemeral in-memory one (so a prod Postgres
	// deployment can never hit this path even if the env leaks). Enforces a
	// narrow scope that keeps the online license gate intact in production.
	if os.Getenv("LINGCDN_DEV_BYPASS_LICENSE") == "1" && strings.EqualFold(s.cfg.StoreBackend, "memory") {
		return true, ""
	}
	st := s.ensureLicenseStatus()
	status := strings.ToLower(strings.TrimSpace(st.Status))
	if status == "" {
		status = "unlicensed"
	}
	switch status {
	case "active":
		if !st.GraceUntil.IsZero() && time.Now().After(st.GraceUntil) && st.Reason != "" {
			return false, st.Reason
		}
		return true, ""
	case "expired", "limited":
		return true, ""
	case "paused", "suspended":
		return false, stReason(st, "system license is no longer valid, please reactivate via auth.lingcdn.cloud")
	case "revoked", "unlicensed":
		return false, stReason(st, "system is not licensed")
	default:
		return false, stReason(st, "system is not licensed")
	}
}
func stReason(st licenseState, fallback string) string {
	if st.Reason != "" {
		return st.Reason
	}
	return fallback
}

// licenseAllowsNewNode checks whether a node operation is allowed by license.
// 已存在的节点在任何非 active 状态（expired / limited / unlicensed 等）均允许重注册，
// 只有真正的"新"节点需要 active 许可证。
func (s *Servers) licenseAllowsNewNode(ctx context.Context, existing bool) (licenseState, bool, error) {
	st := s.ensureLicenseStatus()
	now := time.Now()
	if st.Status != "active" {
		// 已存在的节点无论许可证处于何种非 active 状态，都允许重注册/重连。
		// 这避免了控制面重启或许可证暂时失效时，已有节点被永久锁死。
		if existing {
			return st, true, nil
		}
		return st, false, nil
	}
	if !st.ExpiresAt.IsZero() && now.After(st.ExpiresAt) {
		st.Status = "expired"
		st.Reason = "license expired"
		s.setLicenseState(st)
		if existing {
			return st, true, nil
		}
		return st, false, nil
	}
	if existing {
		return st, true, nil
	}
	if st.MaxNodes > 0 {
		nodes, err := s.store.ListNodes(ctx)
		if err != nil {
			return st, false, err
		}
		if len(nodes) >= st.MaxNodes {
			st.Reason = fmt.Sprintf("node limit reached (%d/%d)", len(nodes), st.MaxNodes)
			return st, false, nil
		}
	}
	return st, true, nil
}

// --- License status/activate/import --- handlers extracted to handlers_license.go

// fetchESOverview pulls overview metrics from Elasticsearch. Best-effort: returns nil on missing config.

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// --- ES / Logs --- handlers extracted to handlers_logs.go

// --- Health/Stats/SystemInfo --- handlers extracted to handlers_health.go

// --- Admin content (orders/announcements) --- handlers extracted to handlers_admin_content.go

// --- Balance --- handlers extracted to handlers_balance.go

// --- Public/Public-license --- handlers extracted to handlers_public.go

// --- Settings --- handlers extracted to handlers_settings.go

// --- API tokens/Domain blacklist --- handlers extracted to handlers_admin_misc.go

// --- Users/Email test --- handlers extracted to handlers_users.go

// --- Products/ProductGroups --- handlers extracted to handlers_products.go

func (s *Servers) resolveUpgradeChannel(ctx context.Context, raw string) (string, error) {
	channel := strings.TrimSpace(raw)
	if channel == "" && s.store != nil {
		if settings, err := s.store.GetSettings(ctx); err == nil && settings != nil {
			channel = strings.TrimSpace(settings.UpgradeChannel)
		}
	}
	if channel == "" {
		return "stable", nil
	}
	normalized, err := normalizeUpgradeChannel(channel)
	if err != nil {
		if strings.TrimSpace(raw) != "" {
			return "", err
		}
		return "stable", nil
	}
	return normalized, nil
}

func compactStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, raw := range items {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// ensureDomainUnique validates domain name and cname uniqueness.
func (s *Servers) ensureDomainUnique(ctx context.Context, d store.Domain, excludeID string) error {
	domains, err := s.store.ListDomains(ctx)
	if err != nil {
		return err
	}
	for _, item := range domains {
		if item == nil || item.ID == excludeID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(d.Name)) {
			return fmt.Errorf("domain exists")
		}
		if d.CNAME != "" && strings.EqualFold(strings.TrimSpace(item.CNAME), strings.TrimSpace(d.CNAME)) {
			return fmt.Errorf("cname already used by another domain")
		}
	}
	return nil
}

// getUserActiveProduct finds the active product for a user, optionally matching a specific line group.
// Returns nil product (without error) if no active product is found.
func (s *Servers) getUserActiveProduct(ctx context.Context, userID, lineGroupID string) (*store.Product, error) {
	orders, err := s.store.ListOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, o := range orders {
		if o == nil || o.Status != "paid" {
			continue
		}
		if o.EndsAt != nil && o.EndsAt.Before(time.Now()) {
			continue
		}
		p, err := s.store.GetProduct(ctx, o.ProductID)
		if err != nil || p == nil {
			continue
		}
		// 兼容 cluster_id：如果 line_group_id 为空，使用 cluster_id 作为集群标识
		productLineGroupID := p.LineGroupID
		if productLineGroupID == "" {
			productLineGroupID = p.ClusterID
		}
		if lineGroupID != "" {
			if productLineGroupID == lineGroupID {
				return p, nil
			}
		} else {
			return p, nil
		}
	}
	return nil, nil
}

// isPrimaryDomain returns true if the domain name is a primary (top-level) domain.
// e.g., "example.com" is primary; "www.example.com", "*.example.com" are not.
func isPrimaryDomain(name string) bool {
	n := strings.TrimPrefix(name, "*.")
	return strings.Count(n, ".") == 1
}

func normalizeDomainOrigin(d *store.Domain) error {
	d.OriginScheme = strings.TrimSpace(d.OriginScheme)
	if d.OriginScheme == "" {
		d.OriginScheme = "http"
	}
	switch d.OriginScheme {
	case "http", "https", "follow_protocol", "follow_port", "follow_both":
	default:
		return fmt.Errorf("invalid origin_scheme")
	}
	if d.OriginPort <= 0 {
		if d.OriginScheme == "https" {
			d.OriginPort = 443
		} else {
			d.OriginPort = 80
		}
	}
	d.OriginHostMode = strings.TrimSpace(d.OriginHostMode)
	if d.OriginHostMode == "" {
		d.OriginHostMode = "request_host"
	}
	switch d.OriginHostMode {
	case "request_host", "request_host_port", "custom":
	default:
		return fmt.Errorf("invalid origin_host_mode")
	}
	d.OriginHost = strings.TrimSpace(d.OriginHost)
	if d.OriginHostMode == "custom" && d.OriginHost == "" {
		return fmt.Errorf("origin_host required when origin_host_mode=custom")
	}
	if d.OriginTimeoutMs <= 0 {
		d.OriginTimeoutMs = 60000
	}
	if d.OriginConnectTimeoutMs <= 0 {
		d.OriginConnectTimeoutMs = 10000
	}
	return nil
}

func normalizeErrorPages(pages []store.ErrorPage) ([]store.ErrorPage, error) {
	if len(pages) == 0 {
		return nil, nil
	}
	allowedStatus := map[int]bool{401: true, 403: true, 404: true, 429: true, 500: true, 502: true, 504: true}
	allowedMode := map[string]bool{"html": true, "json": true, "redirect": true}
	seen := make(map[int]bool)
	out := make([]store.ErrorPage, 0, len(pages))
	for _, p := range pages {
		if !allowedStatus[p.Status] {
			return nil, fmt.Errorf("unsupported error_page status %d", p.Status)
		}
		mode := strings.ToLower(strings.TrimSpace(p.Mode))
		if mode == "" {
			mode = "html"
		}
		if !allowedMode[mode] {
			return nil, fmt.Errorf("invalid error_page mode %s", mode)
		}
		content := strings.TrimSpace(p.Content)
		if content == "" {
			return nil, fmt.Errorf("content required for error_page %d", p.Status)
		}
		if mode == "redirect" {
			if !(strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://") || strings.HasPrefix(content, "/")) {
				return nil, fmt.Errorf("redirect content must be URL for status %d", p.Status)
			}
		}
		if seen[p.Status] {
			continue
		}
		seen[p.Status] = true
		out = append(out, store.ErrorPage{
			Status:  p.Status,
			Mode:    mode,
			Content: content,
		})
	}
	return out, nil
}

func parseOptionalBoolField(payload map[string]json.RawMessage, key string) (bool, bool, error) {
	raw, ok := payload[key]
	if !ok {
		return false, false, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false, false, nil
	}
	var v bool
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return false, false, err
	}
	return true, v, nil
}

func parseOptionalInt64Field(payload map[string]json.RawMessage, key string) (bool, *int64, error) {
	raw, ok := payload[key]
	if !ok {
		return false, nil, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return true, nil, nil
	}
	var v int64
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return false, nil, err
	}
	return true, &v, nil
}

func parseOptionalInt32Field(payload map[string]json.RawMessage, key string) (bool, *int32, error) {
	raw, ok := payload[key]
	if !ok {
		return false, nil, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return true, nil, nil
	}
	var v int32
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return false, nil, err
	}
	return true, &v, nil
}

func parseIntQuery(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return v
}

func defaultOriginScheme(val string) string {
	if strings.TrimSpace(val) == "" {
		return "http"
	}
	return val
}

func defaultOriginPort(port int32) int32 {
	if port <= 0 {
		return 80
	}
	return port
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

func sendEmail(cfg config.Config, to, subject, body string) error {
	if cfg.SMTPHost == "" {
		return fmt.Errorf("smtp host not configured")
	}
	from := strings.TrimSpace(cfg.SMTPFrom)
	if from == "" {
		from = cfg.SMTPUser
	}
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	msg := []byte("To: " + to + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" + body + "\r\n")

	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)

	if cfg.SMTPTLSEnable {
		tlsCfg := &tls.Config{
			ServerName:         cfg.SMTPHost,
			InsecureSkipVerify: cfg.SMTPInsecureSkipVerify,
		}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		c, err := smtp.NewClient(conn, cfg.SMTPHost)
		if err != nil {
			return err
		}
		defer c.Quit()
		if cfg.SMTPUser != "" {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
		if err := c.Mail(from); err != nil {
			return err
		}
		if err := c.Rcpt(to); err != nil {
			return err
		}
		w, err := c.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write(msg); err != nil {
			return err
		}
		return w.Close()
	}

	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

func (s *Servers) notifyAdmin(subject, body string) {
	if s == nil {
		return
	}
	if strings.TrimSpace(s.cfg.SMTPHost) == "" || strings.TrimSpace(s.cfg.AdminEmail) == "" {
		return
	}
	go func() {
		if err := sendEmail(s.cfg, s.cfg.AdminEmail, subject, body); err != nil {
			log.Warn().Err(err).Msg("failed to send admin email")
		}
	}()
}

func (s *Servers) notifyNodeOffline(hostname string) {
	if hostname == "" {
		hostname = "unknown-node"
	}

	// Check if node monitor notification is enabled
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	settings, err := s.store.GetSettings(ctx)
	if err != nil || settings == nil || !settings.NotifyNodeMonitor {
		return
	}

	title := "节点离线告警"
	content := fmt.Sprintf("节点 %s 已离线或停止，请检查节点状态和网络连接。", hostname)

	// Send email to admin
	s.notifyAdmin("Node offline alert", fmt.Sprintf("Node %s is offline or stopped; please check node and network.", hostname))

	// Send webhook notifications
	s.sendWebhookNotification(ctx, title, content)
}

func sanitizeCNAMEBase(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "*", "wildcard")
	var b strings.Builder
	prevDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if r == '.' {
			if !prevDash {
				b.WriteRune('-')
			}
			prevDash = true
			continue
		}
		if r == '-' {
			if !prevDash {
				b.WriteRune('-')
			}
			prevDash = true
			continue
		}
		if !prevDash {
			b.WriteRune('-')
		}
		prevDash = true
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		out = "site"
	}
	return out
}

// defaultCnameSuffix is used when neither CNAME_SUFFIX nor DNS_ZONE is set.
// Returning the bare sanitized domain here is dangerous: it would produce a
// CNAME identical to the user's own hostname (e.g. "111.com" -> "111-com"),
// which isn't usable as an alias target and confuses end-users. A stable,
// obviously-CDN suffix keeps the generated value meaningful even on fresh
// installs where the operator hasn't configured DNS yet.
const defaultCnameSuffix = "cdn.lingcdn.net"

// resolveCnameSuffix returns the effective CNAME suffix. Priority:
// 1. Config file / env var CNAME_SUFFIX (via cfg.CnameSuffix)
// 2. DNS_ZONE env (common shared config)
// 3. Hard-coded fallback (defaultCnameSuffix)
// The result is stripped of leading/trailing dots.
func (s *Servers) resolveCnameSuffix() string {
	if v := strings.Trim(strings.TrimSpace(s.cfg.CnameSuffix), "."); v != "" {
		return v
	}
	if v := strings.Trim(strings.TrimSpace(os.Getenv("DNS_ZONE")), "."); v != "" {
		return v
	}
	return defaultCnameSuffix
}

func (s *Servers) generateDomainCNAME(name string) string {
	return s.generateDomainCNAMEForZone(name, "")
}

// generateDomainCNAMEForZone produces a per-domain CNAME. If zoneHint is
// non-empty the CNAME is placed inside that zone (so it lands inside the
// cluster's own DNSZone and survives SplitByZone in dns_sync). Otherwise
// the global suffix is used.
//
// The label is a random 8-letter lowercase prefix (not derived from the
// domain name): this guarantees uniqueness across sites sharing the same
// zone and hides the origin domain from the generated alias.
func (s *Servers) generateDomainCNAMEForZone(name, zoneHint string) string {
	_ = name
	base := randomCNAMEPrefix(8)
	zone := strings.Trim(strings.TrimSpace(zoneHint), ".")
	if zone != "" {
		return fmt.Sprintf("%s.%s", base, zone)
	}
	cnameSuffix := s.resolveCnameSuffix()
	if cnameSuffix == "" {
		return base
	}
	return fmt.Sprintf("%s.%s", base, cnameSuffix)
}

// randomCNAMEPrefix returns n lowercase ASCII letters suitable for a DNS
// label. Uses crypto/rand so collisions across sites are negligible; falls
// back to a deterministic suffix if the system RNG fails (extremely rare).
func randomCNAMEPrefix(n int) string {
	if n <= 0 {
		n = 8
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// Fall back to a timestamp-derived value; still DNS-safe.
		ts := fmt.Sprintf("%x", time.Now().UnixNano())
		if len(ts) < n {
			ts = strings.Repeat(ts, (n/len(ts))+1)
		}
		return ts[:n]
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}

// lookupClusterZone returns the DNSZone of the cluster referenced by
// lineGroupID, or "" if not found. Errors are swallowed because callers
// treat the zone as an optional hint.
func (s *Servers) lookupClusterZone(ctx context.Context, lineGroupID string) string {
	id := strings.TrimSpace(lineGroupID)
	if id == "" || s.store == nil {
		return ""
	}
	cl, err := s.store.GetCluster(ctx, id)
	if err != nil || cl == nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(cl.DNSZone), ".")
}

func (s *Servers) generateLineGroupCNAME(name, domain string) string {
	base := sanitizeCNAMEBase(name)
	if base == "" {
		base = sanitizeCNAMEBase(domain)
	}
	if base == "" {
		return ""
	}
	cnameSuffix := s.resolveCnameSuffix()
	if cnameSuffix != "" {
		return fmt.Sprintf("%s.%s", base, cnameSuffix)
	}
	return base
}

// generateClusterCNAME builds a cluster-level FQDN that lives inside the
// cluster's own DNSZone. This keeps cluster A/AAAA records inside the same
// managed zone that syncDNSRecords iterates over.
func (s *Servers) generateClusterCNAME(name, dnsZone string) string {
	zone := strings.Trim(strings.TrimSpace(dnsZone), ".")
	base := sanitizeCNAMEBase(name)
	if base == "" {
		return ""
	}
	if zone == "" {
		// Fall back to the global suffix so at least we produce a valid FQDN;
		// syncDNSRecords will silently skip it if it doesn't land inside any
		// managed zone, which is the correct behavior.
		suffix := s.resolveCnameSuffix()
		if suffix == "" {
			return base
		}
		return fmt.Sprintf("%s.%s", base, suffix)
	}
	return fmt.Sprintf("%s.%s", base, zone)
}

// --- Nodes --- handlers extracted to handlers_nodes.go

// --- Domains --- handlers extracted to handlers_domains.go

// --- Origins/CacheRules/Publish/Purge --- handlers extracted to handlers_content_delivery.go

// --- Upgrade control/node --- handlers extracted to handlers_upgrade.go

func (s *Servers) metricsMux() http.Handler {
	mux := http.NewServeMux()

	// Update node metrics before serving
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(s.cfg.MetricsToken) != "" {
			authz := strings.TrimSpace(r.Header.Get("Authorization"))
			const bearer = "Bearer "
			if !strings.HasPrefix(authz, bearer) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			got := strings.TrimSpace(strings.TrimPrefix(authz, bearer))
			if subtle.ConstantTimeCompare([]byte(got), []byte(strings.TrimSpace(s.cfg.MetricsToken))) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		if s.metrics != nil {
			s.metrics.SetNodesConnected(s.hub.Count())
		}
		promhttp.Handler().ServeHTTP(w, r)
	})

	return mux
}

// Helper function
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeInternalError logs the raw error with context and returns a descriptive but safe
// error message to the client. The operation name (e.g. "list users", "create domain")
// is included in both the log and the client response to aid debugging.
func writeInternalError(w http.ResponseWriter, operation string, err error) {
	log.Error().Err(err).Str("operation", operation).Msg("internal error")
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error":     fmt.Sprintf("内部错误(%s): %v", operation, err),
		"operation": operation,
	})
}

// dataRetentionLoop periodically cleans up old data based on retention settings.
// --- dataRetentionLoop/runDataRetention/cleanupESIndices --- extracted to background_jobs.go

// acmeHostPolicy allows ACME only for managed domains.
func (s *Servers) acmeHostPolicy() autocert.HostPolicy {
	return func(ctx context.Context, host string) error {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			return fmt.Errorf("empty host")
		}
		d, err := s.store.GetDomainByName(ctx, host)
		if err != nil {
			return err
		}
		if d == nil {
			return fmt.Errorf("domain not managed: %s", host)
		}
		return nil
	}
}

// ensureACMEIssuer returns an autocert.Manager dedicated to on-demand
// certificate issuance for /api/certificates/acme. If the control plane
// is already running under ACME (s.acmeMgr != nil) we reuse that manager
// so the HTTP-01 challenge handler on :80 services the new orders too.
// Otherwise we lazily build a standalone manager and start a dedicated
// HTTP-01 challenge listener on :80. This means ACME cert issuance works
// even when the control plane itself doesn't run HTTPS (ACMEEnable=false).
func (s *Servers) ensureACMEIssuer() *autocert.Manager {
	if s.acmeMgr != nil {
		return s.acmeMgr
	}
	s.acmeIssuerMu.Lock()
	defer s.acmeIssuerMu.Unlock()
	if s.acmeIssuer != nil {
		return s.acmeIssuer
	}
	cacheDir := s.cfg.ACMECacheDir
	if strings.TrimSpace(cacheDir) == "" {
		cacheDir = "/etc/lingcdn/acme"
	}
	mgr := &autocert.Manager{
		Cache:      autocert.DirCache(cacheDir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: s.acmeHostPolicy(),
		Email:      s.cfg.ACMEEmail,
	}
	if s.cfg.ACMEStaging {
		mgr.Client = &acme.Client{DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory"}
	}
	s.acmeIssuer = mgr

	// Start a dedicated HTTP-01 challenge listener so Let's Encrypt can
	// validate domain ownership. Without this the ACME handshake fails
	// because there is nothing serving /.well-known/acme-challenge/*.
	go func() {
		addr := ":80"
		log.Info().Str("addr", addr).Msg("starting standalone ACME HTTP-01 challenge listener")
		challengeSrv := &http.Server{
			Addr: addr,
			Handler: mgr.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Non-challenge requests get a redirect to HTTPS.
				http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
			})),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
		}
		if err := challengeSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("standalone ACME challenge listener failed — certificate issuance may not work")
		}
	}()

	return mgr
}
