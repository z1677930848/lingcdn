package server

// Background maintenance goroutines launched from Servers.Serve: periodic
// system stats report to the portal (online mode) and retention cleanup
// across node/system logs, WAF bans, upgrade tasks, and ES indices.
// Both are best-effort: failures are logged, never escalated.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/buildinfo"
	"github.com/lingcdn/control/internal/store"
)

func (s *Servers) systemReportLoop(ctx context.Context) {
	if s.licenseMode() != "online" {
		return
	}
	interval := s.cfg.SystemReportInterval
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runBGTask("system.report", func() (string, error) {
		return s.reportSystemOnce(context.Background())
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runBGTask("system.report", func() (string, error) {
				return s.reportSystemOnce(context.Background())
			})
		}
	}
}

func (s *Servers) reportSystemOnce(ctx context.Context) (string, error) {
	if s.licenseMode() != "online" {
		return "skip: license mode " + s.licenseMode(), nil
	}
	st := s.currentLicenseStatus()
	if s.staticLicenseIndexURL() != "" {
		return "skip: static license index mode", nil
	}
	if s.store == nil {
		return "skip: store missing", nil
	}

	// Version is the single source of truth for "what binary am I running".
	// Until this refactor, reportSystemOnce read APP_VERSION from the process
	// environment. That meant a stale APP_VERSION=... line in
	// /etc/lingcdn/lingcdn.env could shadow the real binary version forever
	// after an upgrade — the portal would keep seeing the old number. Now we
	// read directly from buildinfo, which is populated at link time via
	// -ldflags; environment has no voice here.
	current := buildinfo.Version()

	ctxStore, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	userCount := 0
	if n, err := s.store.CountUsers(ctxStore); err == nil {
		userCount = n
	} else if users, err := s.store.ListUsers(ctxStore, 0); err == nil {
		userCount = len(users)
	}
	nodesTotal := 0
	if n, err := s.store.CountNodes(ctxStore); err == nil {
		nodesTotal = n
	} else if nodes, err := s.store.ListNodes(ctxStore); err == nil {
		nodesTotal = len(nodes)
	}
	nodesInstalled := 0
	if nodes, err := s.store.ListNodes(ctxStore); err == nil {
		for _, n := range nodes {
			if n == nil {
				continue
			}
			if strings.TrimSpace(n.Status) != "" || !n.LastHeartbeat.IsZero() {
				nodesInstalled++
			}
		}
	}
	totalSites := 0
	if n, err := s.store.CountDomains(ctxStore); err == nil {
		totalSites = n
	} else if domains, err := s.store.ListDomains(ctxStore); err == nil {
		totalSites = len(domains)
	}

	licenseAt := st.UpdatedAt
	if !st.LastChecked.IsZero() {
		licenseAt = st.LastChecked
	}
	if licenseAt.IsZero() {
		licenseAt = time.Now().UTC()
	}

	payload, _ := json.Marshal(struct {
		ControlID      string `json:"control_id"`
		SitesTotal     int    `json:"sites_total"`
		NodesTotal     int    `json:"nodes_total"`
		NodesInstalled int    `json:"nodes_installed"`
		UsersTotal     int    `json:"users_total"`
		Version        string `json:"version"`
		LicenseKey     string `json:"license_key"`
		LicenseIP      string `json:"license_ip"`
		LicenseAt      string `json:"license_at"`
	}{
		ControlID:      strings.TrimSpace(s.cfg.ControlID),
		SitesTotal:     totalSites,
		NodesTotal:     nodesTotal,
		NodesInstalled: nodesInstalled,
		UsersTotal:     userCount,
		Version:        current,
		LicenseKey:     strings.TrimSpace(st.LicenseKey),
		LicenseIP:      strings.TrimSpace(s.cfg.PublicIP),
		LicenseAt:      licenseAt.Format(time.RFC3339),
	})

	endpoint := s.portalBase() + "/api/control/report"

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	// Prefer the per-license HMAC secret the portal delivers via license
	// verify (auto-synced, no operator key sharing required). Fall back to
	// the legacy operator-managed PORTAL_REPORT_SECRET in cfg, which is
	// what older portals — those that do not yet embed report_secret in
	// their license verify response — still expect.
	secret := strings.TrimSpace(st.ReportSecret)
	if secret == "" {
		secret = strings.TrimSpace(s.cfg.PortalReportSecret)
	}
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(bytes.TrimSpace(payload))
		req.Header.Set("X-Report-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("system report request failed")
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		log.Warn().Int("status", resp.StatusCode).Str("body", string(b)).Msg("system report rejected")
		return "", fmt.Errorf("report rejected: status %d", resp.StatusCode)
	}
	return "ok", nil
}

// dataRetentionLoop periodically cleans up old data based on retention settings.
func (s *Servers) dataRetentionLoop(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runBGTask("cleanup.retention", func() (string, error) {
				return s.runDataRetention()
			})
		}
	}
}

func (s *Servers) runDataRetention() (string, error) {
	settingsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	settings, err := s.store.GetSettings(settingsCtx)
	cancel()
	if err != nil {
		log.Warn().Err(err).Msg("data retention: failed to load settings")
		settings = store.DefaultSettings()
	}

	cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cleanCancel()

	var parts []string
	var lastErr error

	// System logs
	if days := settings.RetentionSystemLogs; days > 0 {
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		deleted, err := s.store.DeleteSystemLogsOlderThan(cleanCtx, cutoff)
		if err != nil {
			log.Warn().Err(err).Msg("data retention: failed to clean system_logs")
			lastErr = err
		} else if deleted > 0 {
			parts = append(parts, fmt.Sprintf("操作日志 %d 条", deleted))
		}
	}

	// Expired WAF bans
	if days := settings.RetentionWafBans; days > 0 {
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		deleted, err := s.store.DeleteExpiredWafBansOlderThan(cleanCtx, cutoff)
		if err != nil {
			log.Warn().Err(err).Msg("data retention: failed to clean waf_bans")
			lastErr = err
		} else if deleted > 0 {
			parts = append(parts, fmt.Sprintf("过期封禁 %d 条", deleted))
		}
	}

	// Upgrade tasks + logs (CASCADE)
	if days := settings.RetentionUpgradeLogs; days > 0 {
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		deleted, err := s.store.DeleteUpgradeTasksOlderThan(cleanCtx, cutoff)
		if err != nil {
			log.Warn().Err(err).Msg("data retention: failed to clean upgrade_tasks")
			lastErr = err
		} else if deleted > 0 {
			parts = append(parts, fmt.Sprintf("升级任务 %d 条", deleted))
		}
	}

	// ES access log indices
	esDeleted := 0
	if days := settings.RetentionESLogs; days > 0 && settings.ElasticsearchURL != "" {
		esDeleted = s.cleanupESIndices(settings, days)
		if esDeleted > 0 {
			parts = append(parts, fmt.Sprintf("ES索引 %d 个", esDeleted))
		}
	}

	if len(parts) == 0 {
		msg := "无需清理"
		if lastErr != nil {
			return msg, fmt.Errorf("部分清理失败: %w", lastErr)
		}
		return msg, nil
	}
	msg := "已清理: " + strings.Join(parts, ", ")
	log.Info().Str("summary", msg).Msg("data retention completed")
	if lastErr != nil {
		return msg, fmt.Errorf("部分清理失败: %w", lastErr)
	}
	return msg, nil
}

// cleanupESIndices deletes Elasticsearch indices older than the retention period.
func (s *Servers) cleanupESIndices(settings *store.Settings, retentionDays int) int {
	prefix := settings.ElasticsearchIndex
	if prefix == "" {
		prefix = "cdn-access"
	}
	esURL := strings.TrimRight(settings.ElasticsearchURL, "/")

	// List indices matching the prefix
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", esURL+"/_cat/indices/"+prefix+"-*?format=json&h=index", nil)
	if err != nil {
		log.Warn().Err(err).Msg("data retention: failed to build ES index list request")
		return 0
	}
	if settings.ElasticsearchUser != "" {
		req.SetBasicAuth(settings.ElasticsearchUser, settings.ElasticsearchPass)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("data retention: failed to list ES indices")
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Warn().Int("status", resp.StatusCode).Msg("data retention: ES index list returned non-200")
		return 0
	}

	var indices []struct {
		Index string `json:"index"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&indices); err != nil {
		log.Warn().Err(err).Msg("data retention: failed to decode ES index list")
		return 0
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	deleted := 0

	// Date formats commonly used in index names
	dateFormats := []string{"2006.01.02", "2006-01-02", "20060102"}

	for _, idx := range indices {
		name := idx.Index
		// Extract date suffix after the prefix
		suffix := strings.TrimPrefix(name, prefix+"-")
		if suffix == name {
			continue // no prefix match
		}

		var indexDate time.Time
		var parsed bool
		for _, fmt := range dateFormats {
			if t, err := time.Parse(fmt, suffix); err == nil {
				indexDate = t
				parsed = true
				break
			}
		}
		if !parsed {
			continue
		}

		if indexDate.Before(cutoff) {
			delReq, err := http.NewRequest("DELETE", esURL+"/"+name, nil)
			if err != nil {
				continue
			}
			if settings.ElasticsearchUser != "" {
				delReq.SetBasicAuth(settings.ElasticsearchUser, settings.ElasticsearchPass)
			}
			delResp, err := client.Do(delReq)
			if err != nil {
				log.Warn().Err(err).Str("index", name).Msg("data retention: failed to delete ES index")
				continue
			}
			delResp.Body.Close()
			if delResp.StatusCode == http.StatusOK {
				deleted++
			} else {
				log.Warn().Int("status", delResp.StatusCode).Str("index", name).Msg("data retention: ES index delete returned non-200")
			}
		}
	}
	if deleted > 0 {
		log.Info().Int("deleted", deleted).Int("retention_days", retentionDays).Msg("data retention: cleaned ES indices")
	}
	return deleted
}

// certificateRenewalLoop scans once a day for ACME-issued certificates that
// expire within the renewal window and reissues them in place. Without
// this loop Let's Encrypt certs just silently expire at day 90 and the
// user has to hunt down the "申请免费证书" button to re-trigger —
// typically after the phone starts ringing.
//
// We identify renewable certs by: Type == "acme" AND AutoRenew == true.
// Manually uploaded records have Type == "upload" and are skipped —
// operators keep ownership of those.
//
// On success the loop reuses persistACMECertificate so the domain binding
// and publish steps match the interactive path. On failure we keep the
// old cert in place (still valid until NotAfter) and retry on the next
// tick; Let's Encrypt rate-limits aggressively so we don't retry on a
// tight schedule.
func (s *Servers) certificateRenewalLoop(ctx context.Context) {
	// Daily cadence. Renewing within a 15-day window gives us ~15 retry
	// opportunities before hard expiry, which easily absorbs transient
	// LE / DNS / challenge outages.
	const (
		tickEvery     = 24 * time.Hour
		renewalWindow = 15 * 24 * time.Hour
	)
	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()

	// Initial run shortly after startup so operators don't have to wait a
	// full day to see renewals kick in. 5 min lets the rest of the control
	// plane settle (store, publisher, etc.) before we start issuing.
	initialDelay := time.NewTimer(5 * time.Minute)
	defer initialDelay.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-initialDelay.C:
			runBGTask("cert.renew", func() (string, error) {
				return s.runCertificateRenewal(renewalWindow)
			})
		case <-ticker.C:
			runBGTask("cert.renew", func() (string, error) {
				return s.runCertificateRenewal(renewalWindow)
			})
		}
	}
}

// runCertificateRenewal lists all certificates and re-issues any ACME-
// issued cert whose expiry falls inside the renewal window. Errors are
// logged per-cert and do not abort the sweep.
func (s *Servers) runCertificateRenewal(window time.Duration) (string, error) {
	listCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	certs, err := s.store.ListCertificates(listCtx)
	cancel()
	if err != nil {
		return "", fmt.Errorf("list certificates: %w", err)
	}

	deadline := time.Now().Add(window)
	var scanned, renewed, skipped, failed int

	for _, c := range certs {
		if c == nil {
			continue
		}
		scanned++
		// Only renew active ACME certs with auto_renew enabled.
		if c.Type != "acme" || !c.AutoRenew {
			skipped++
			continue
		}
		switch strings.ToLower(strings.TrimSpace(c.Status)) {
		case "failed", "pending":
			skipped++
			continue
		}
		host := strings.ToLower(strings.TrimSpace(c.Domain))
		if host == "" {
			skipped++
			continue
		}
		// Not due for renewal yet.
		if !c.ExpiresAt.IsZero() && c.ExpiresAt.After(deadline) {
			skipped++
			continue
		}
		if err := s.renewSingleCertificate(c); err != nil {
			failed++
			log.Warn().Err(err).Int64("cert_id", c.ID).Str("domain", host).Msg("cert.renew: renewal failed")
			continue
		}
		renewed++
		log.Info().Int64("cert_id", c.ID).Str("domain", host).Msg("cert.renew: renewed")
	}
	return fmt.Sprintf("scanned=%d renewed=%d skipped=%d failed=%d", scanned, renewed, skipped, failed), nil
}

// renewSingleCertificate drives one ACME renewal with a bounded ctx.
func (s *Servers) renewSingleCertificate(cert *store.Certificate) error {
	if cert == nil {
		return fmt.Errorf("certificate missing")
	}
	host := strings.ToLower(strings.TrimSpace(cert.Domain))
	if host == "" {
		return fmt.Errorf("certificate domain empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	domainCfg, err := s.store.GetDomainByName(ctx, host)
	if err != nil {
		return fmt.Errorf("get domain: %w", err)
	}
	if domainCfg == nil {
		return fmt.Errorf("domain no longer managed")
	}
	mgr := s.ensureACMEIssuer()
	if mgr == nil {
		return fmt.Errorf("acme issuer not available")
	}
	if mgr.Email == "" {
		mgr.Email = "admin@" + host
	}
	certPEM, keyPEM, notAfter, issueErr := s.issueCertViaACME(mgr, host)
	if issueErr != nil {
		return fmt.Errorf("acquire certificate: %w", issueErr)
	}
	return s.persistACMECertificate(ctx, cert.ID, host, certPEM, keyPEM, notAfter, domainCfg)
}
