package server

// Certificate management handlers: admin + per-user CRUD over TLS certs,
// plus ACME provisioning for managed domains. Private keys are stripped
// from list/get responses. Certificate IDs are auto-increment integers
// starting from 1 (BIGSERIAL in Postgres).

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

// acmeIssueLocks serialises ACME issuance per (owner, host) tuple. Without
// this, two rapid clicks on "申请证书" race through the failed-record
// cleanup → CreateCertificate(pending) → issueCertViaACME path and can
// burn the upstream rate limit (Let's Encrypt: 5 duplicate certs per week).
// Locks are cheap — keyed entries are kept resident for the lifetime of
// the process, which matters only for workloads issuing into millions of
// distinct hostnames.
var acmeIssueLocks sync.Map // key: "owner|host", value: *sync.Mutex

func acmeIssueLock(ownerID, host string) *sync.Mutex {
	key := strings.ToLower(strings.TrimSpace(ownerID)) + "|" + strings.ToLower(strings.TrimSpace(host))
	if mu, ok := acmeIssueLocks.Load(key); ok {
		return mu.(*sync.Mutex)
	}
	mu := &sync.Mutex{}
	existing, _ := acmeIssueLocks.LoadOrStore(key, mu)
	return existing.(*sync.Mutex)
}

// certSafeView returns a map with private keys stripped for API responses.
func certSafeView(c *store.Certificate) map[string]any {
	return map[string]any{
		"id":          c.ID,
		"name":        c.Name,
		"domain":      c.Domain,
		"user_id":     c.UserID,
		"type":        c.Type,
		"auto_renew":  c.AutoRenew,
		"status":      c.Status,
		"fail_reason": c.FailReason,
		"expires_at":  c.ExpiresAt,
		"created_at":  c.CreatedAt,
		"updated_at":  c.UpdatedAt,
	}
}

func (s *Servers) handleCertificates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := getUserRole(ctx)
	userID, _ := ctx.Value(ctxKeyUserID).(string)

	switch r.Method {
	case http.MethodGet:
		var certs []*store.Certificate
		var err error
		if role == "admin" {
			certs, err = s.store.ListCertificates(ctx)
		} else {
			certs, err = s.store.ListCertificatesByUser(ctx, userID)
		}
		if err != nil {
			writeInternalError(w, "list certificates", err)
			return
		}
		safeCerts := make([]map[string]any, len(certs))
		for i, c := range certs {
			safeCerts[i] = certSafeView(c)
		}
		writeJSON(w, http.StatusOK, map[string]any{"certificates": safeCerts})

	case http.MethodPost:
		if role != "admin" && !s.requireUserPermission(w, ctx, PermCertificatesWrite) {
			return
		}
		// Upload a certificate (PEM).
		var body struct {
			Name    string `json:"name"`
			Domain  string `json:"domain"`
			UserID  string `json:"user_id"` // admin can specify; users get overridden
			CertPEM string `json:"cert_pem"`
			KeyPEM  string `json:"key_pem"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
			return
		}
		domain := strings.ToLower(strings.TrimSpace(body.Domain))
		if domain == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "域名不能为空"})
			return
		}
		certPEM := strings.TrimSpace(body.CertPEM)
		keyPEM := strings.TrimSpace(body.KeyPEM)
		if certPEM == "" || keyPEM == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "证书和密钥不能为空"})
			return
		}

		// Verify the cert/key pair actually matches, and that the leaf
		// cert covers the claimed domain. Without these two checks any
		// authenticated user could upload a mismatched blob (triggering
		// TLS handshake failures downstream) or attach an unrelated
		// certificate to a domain, polluting the cert table and ACME
		// renewal paths.
		keyPair, kpErr := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if kpErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "证书与私钥不匹配: " + kpErr.Error()})
			return
		}
		if len(keyPair.Certificate) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "证书链为空"})
			return
		}
		leaf, leafErr := x509.ParseCertificate(keyPair.Certificate[0])
		if leafErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "证书解析失败: " + leafErr.Error()})
			return
		}
		if !certCoversDomain(leaf, domain) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "证书未覆盖该域名，请确认 SAN/CN 包含 " + domain,
			})
			return
		}
		expiresAt := leaf.NotAfter

		ownerID := userID
		if role == "admin" && strings.TrimSpace(body.UserID) != "" {
			ownerID = strings.TrimSpace(body.UserID)
		}

		if role != "admin" {
			product, err := s.getUserActiveProduct(ctx, userID, "")
			if err != nil {
				writeInternalError(w, "get user product", err)
				return
			}
			if product == nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "无有效套餐，请先购买套餐"})
				return
			}
			dom, err := s.store.GetDomainByName(ctx, domain)
			if err != nil {
				writeInternalError(w, "get domain for cert upload", err)
				return
			}
			if dom == nil || dom.UserID != userID {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权为该域名上传证书"})
				return
			}
		}

		// Check uniqueness (user_id + domain)
		if existing, _ := s.store.GetCertificateByDomain(ctx, domain); existing != nil && existing.UserID == ownerID {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "该域名下已存在证书，请先删除后重新上传"})
			return
		}

		status := "active"
		if expiresAt.Before(time.Now()) {
			status = "expired"
		} else if expiresAt.Before(time.Now().Add(15 * 24 * time.Hour)) {
			status = "expiring"
		}

		cert := &store.Certificate{
			Name:      strings.TrimSpace(body.Name),
			Domain:    domain,
			UserID:    ownerID,
			Type:      "upload",
			AutoRenew: false,
			Status:    status,
			CertPEM:   []byte(certPEM),
			KeyPEM:    []byte(keyPEM),
			ExpiresAt: expiresAt,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if cert.Name == "" {
			cert.Name = domain
		}
		if err := s.store.CreateCertificate(ctx, cert); err != nil {
			writeInternalError(w, "create certificate", err)
			return
		}

		// Auto-bind to matching domain
		s.tryBindCertToDomain(ctx, cert, role, ownerID)

		writeJSON(w, http.StatusCreated, certSafeView(cert))

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleCertificateByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := getUserRole(ctx)
	userID, _ := ctx.Value(ctxKeyUserID).(string)
	idStr := strings.TrimPrefix(r.URL.Path, "/api/certificates/")
	// Handle /acme sub-path (routed separately, but guard against overlap)
	if idStr == "acme" || strings.HasPrefix(idStr, "acme/") {
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的证书ID"})
		return
	}

	existing, err := s.store.GetCertificate(ctx, id)
	if err != nil {
		writeInternalError(w, "get certificate", err)
		return
	}
	if existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "证书不存在"})
		return
	}
	if role != "admin" && existing.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权操作此证书"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, certSafeView(existing))

	case http.MethodDelete:
		if err := s.store.DeleteCertificate(ctx, id); err != nil {
			writeInternalError(w, "delete certificate", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

// acmeCertRequest is the body for POST /api/certificates/acme.
type acmeCertRequest struct {
	Domain string `json:"domain"`
	UserID string `json:"user_id"` // admin can specify
}

// handleACMECertificate requests certificate issuance via ACME (Let's Encrypt).
// If the domain already has an active cert, the request is rejected — only
// failed certs can be re-issued.
func (s *Servers) handleACMECertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	ctx := r.Context()
	role := getUserRole(ctx)
	userID, _ := ctx.Value(ctxKeyUserID).(string)

	var req acmeCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Domain) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "域名不能为空"})
		return
	}
	host := strings.ToLower(strings.TrimSpace(req.Domain))

	ownerID := userID
	if role == "admin" && strings.TrimSpace(req.UserID) != "" {
		ownerID = strings.TrimSpace(req.UserID)
	}

	// Serialise issuance for this (owner, host). Prevents the duplicate
	// ACME burn described on acmeIssueLocks: without the lock, two
	// concurrent requests both pass the "failed-record cleanup" gate,
	// both create pending rows, and both call the ACME backend — which
	// counts as two duplicate issuances against Let's Encrypt's weekly
	// quota per hostname.
	mu := acmeIssueLock(ownerID, host)
	mu.Lock()
	defer mu.Unlock()

	// Ensure domain is managed by this control server.
	domainCfg, err := s.store.GetDomainByName(ctx, host)
	if err != nil {
		writeInternalError(w, "get domain", err)
		return
	}
	if domainCfg == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "域名不在管理范围内"})
		return
	}
	if role != "admin" && domainCfg.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "无权为该域名申请证书"})
		return
	}

	// Non-admin users need an active product
	if role != "admin" {
		product, err := s.getUserActiveProduct(ctx, userID, "")
		if err != nil {
			writeInternalError(w, "get user product", err)
			return
		}
		if product == nil {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "无有效套餐，请先购买套餐"})
			return
		}
	}

	// Check if there's already a certificate for this user+domain.
	// Only allow re-issue if the existing cert is failed.
	if existing, _ := s.store.GetCertificateByDomain(ctx, host); existing != nil && existing.UserID == ownerID {
		if existing.Status != "failed" {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "该域名已有证书，状态为「" + existing.Status + "」，只有申请失败的证书才能重新申请",
			})
			return
		}
		// Delete the failed record so we can create a fresh one
		if err := s.store.DeleteCertificate(ctx, existing.ID); err != nil {
			writeInternalError(w, "delete failed certificate", err)
			return
		}
	}

	// Create a "pending" record first so the UI can show progress
	record := &store.Certificate{
		Name:      host,
		Domain:    host,
		UserID:    ownerID,
		Type:      "acme",
		AutoRenew: true,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.store.CreateCertificate(ctx, record); err != nil {
		writeInternalError(w, "create certificate record", err)
		return
	}

	// Actually issue the certificate
	mgr := s.ensureACMEIssuer()
	if mgr == nil {
		record.Status = "failed"
		record.FailReason = "ACME issuer not available"
		_ = s.store.UpdateCertificate(ctx, record)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "ACME管理器未初始化"})
		return
	}
	if mgr.Email == "" {
		mgr.Email = "admin@" + host
	}

	certPEM, keyPEM, notAfter, issueErr := s.issueCertViaACME(mgr, host)
	if issueErr != nil {
		record.Status = "failed"
		record.FailReason = issueErr.Error()
		record.UpdatedAt = time.Now()
		_ = s.store.UpdateCertificate(ctx, record)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("申请证书失败: %v", issueErr)})
		return
	}

	// Success — update the record
	record.CertPEM = certPEM
	record.KeyPEM = keyPEM
	record.ExpiresAt = notAfter
	record.Status = "active"
	record.FailReason = ""
	record.UpdatedAt = time.Now()
	if err := s.store.UpdateCertificate(ctx, record); err != nil {
		writeInternalError(w, "update certificate", err)
		return
	}

	// Bind to domain + publish
	syncIDs := s.bindCertAndPublish(ctx, record, domainCfg)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"id":            record.ID,
		"domain":        host,
		"expires_at":    notAfter,
		"sync_task_ids": syncIDs,
	})
}

// issueCertViaACME calls autocert to issue a Let's Encrypt certificate.
func (s *Servers) issueCertViaACME(mgr interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}, host string) (certPEM, keyPEM []byte, notAfter time.Time, err error) {
	hello := tlsClientHelloFor(host)
	tlsCert, err := mgr.GetCertificate(&hello)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	return encodeTLSCert(tlsCert)
}

// tryBindCertToDomain auto-binds a certificate to its matching domain if the
// domain's cert slot is empty. Used after certificate upload. Also enables
// HTTPS on the domain so the uploaded cert takes effect immediately.
func (s *Servers) tryBindCertToDomain(ctx context.Context, cert *store.Certificate, role, ownerID string) {
	dom, err := s.store.GetDomainByName(ctx, strings.ToLower(strings.TrimSpace(cert.Domain)))
	if err != nil || dom == nil {
		return
	}
	if role != "admin" && dom.UserID != ownerID {
		return
	}
	if dom.CertID != "" {
		return // already has a cert bound
	}
	dom.CertID = fmt.Sprintf("%d", cert.ID)
	dom.HTTPSEnabled = true
	dom.UpdatedAt = time.Now()
	if err := s.store.UpdateDomain(ctx, dom); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("domain", dom.Name).Msg("auto-bind uploaded cert failed")
		return
	}
	_ = s.startPublishTask(ctx, "auto", "domain:"+dom.ID, "domain:cert:upload:"+dom.Name, "", nil)
}

// bindCertAndPublish binds a certificate to a domain and publishes config.
// Used after ACME issuance.
func (s *Servers) bindCertAndPublish(ctx context.Context, cert *store.Certificate, domainCfg *store.Domain) []string {
	if domainCfg == nil {
		return nil
	}
	certIDStr := fmt.Sprintf("%d", cert.ID)
	if domainCfg.CertID != certIDStr || !domainCfg.HTTPSEnabled {
		domainCfg.CertID = certIDStr
		domainCfg.HTTPSEnabled = true
		domainCfg.UpdatedAt = time.Now()
		if err := s.store.UpdateDomain(ctx, domainCfg); err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("domain", domainCfg.Name).Msg("bind certificate to domain failed")
		}
	}
	task := s.startPublishTask(ctx, "auto", "domain:"+domainCfg.ID, "domain:cert:acme:"+domainCfg.Name, "", nil)
	if task != nil && task.ID != "" {
		log.Ctx(ctx).Info().Str("domain", domainCfg.Name).Str("task_id", task.ID).Msg("acme cert issued and config publish queued")
		return []string{task.ID}
	}
	return nil
}

// persistACMECertificate is used by the renewal loop to update an existing
// ACME certificate record in place. It finds the cert by domain, updates
// PEM/expiry/status, binds the domain, and publishes.
func (s *Servers) persistACMECertificate(
	ctx context.Context,
	certID int64,
	host string,
	certPEM, keyPEM []byte,
	expiresAt time.Time,
	domainCfg *store.Domain,
) error {
	existing, err := s.store.GetCertificate(ctx, certID)
	if err != nil {
		return fmt.Errorf("get certificate: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("no existing certificate record id=%d for domain %s", certID, host)
	}
	if domainCfg != nil && strings.TrimSpace(domainCfg.UserID) != "" && existing.UserID != domainCfg.UserID {
		return fmt.Errorf("certificate owner mismatch for domain %s", host)
	}

	existing.CertPEM = certPEM
	existing.KeyPEM = keyPEM
	existing.ExpiresAt = expiresAt
	existing.Status = "active"
	existing.FailReason = ""
	existing.UpdatedAt = time.Now()
	if err := s.store.UpdateCertificate(ctx, existing); err != nil {
		return fmt.Errorf("update certificate: %w", err)
	}

	s.bindCertAndPublish(ctx, existing, domainCfg)
	return nil
}

// validateDomainCertBinding ensures the cert exists, belongs to the domain
// owner (unless admin), and covers the target hostname.
func (s *Servers) validateDomainCertBinding(ctx context.Context, role, ownerID, domainName, certID string) error {
	certID = strings.TrimSpace(certID)
	if certID == "" {
		return nil
	}
	id, err := strconv.ParseInt(certID, 10, 64)
	if err != nil {
		return fmt.Errorf("无效的证书ID")
	}
	cert, err := s.store.GetCertificate(ctx, id)
	if err != nil {
		return err
	}
	if cert == nil {
		return fmt.Errorf("证书不存在")
	}
	if role != "admin" && cert.UserID != ownerID {
		return fmt.Errorf("无权使用该证书")
	}
	host := strings.ToLower(strings.TrimSpace(domainName))
	if host == "" {
		return fmt.Errorf("域名不能为空")
	}
	certDomain := strings.ToLower(strings.TrimSpace(cert.Domain))
	if certDomain == host {
		return nil
	}
	if len(cert.CertPEM) > 0 {
		if block, _ := pem.Decode(cert.CertPEM); block != nil {
			if leaf, perr := x509.ParseCertificate(block.Bytes); perr == nil && certCoversDomain(leaf, host) {
				return nil
			}
		}
	}
	return fmt.Errorf("证书与域名不匹配")
}

// tlsClientHelloFor builds the minimal ClientHelloInfo autocert needs.
func tlsClientHelloFor(host string) tls.ClientHelloInfo {
	return tls.ClientHelloInfo{ServerName: host}
}

// certCoversDomain reports whether the leaf cert's SAN list (or CN as a
// legacy fallback) authorises it to present itself for `host`. Both exact
// DNS names and single-label wildcards (RFC 6125 §6.4.3) are accepted.
// Case-insensitive; empty SAN + empty CN returns false.
func certCoversDomain(leaf *x509.Certificate, host string) bool {
	if leaf == nil || host == "" {
		return false
	}
	host = strings.ToLower(strings.TrimSpace(host))
	names := make([]string, 0, len(leaf.DNSNames)+1)
	for _, n := range leaf.DNSNames {
		names = append(names, strings.ToLower(strings.TrimSpace(n)))
	}
	if cn := strings.ToLower(strings.TrimSpace(leaf.Subject.CommonName)); cn != "" {
		names = append(names, cn)
	}
	for _, n := range names {
		if n == "" {
			continue
		}
		if n == host {
			return true
		}
		if strings.HasPrefix(n, "*.") {
			// Wildcard: matches exactly one subdomain label, i.e.
			// "*.example.com" matches "a.example.com" but not
			// "a.b.example.com" or "example.com" itself. This mirrors
			// RFC 6125 and what mainstream TLS clients enforce.
			suffix := n[1:] // ".example.com"
			if !strings.HasSuffix(host, suffix) {
				continue
			}
			prefix := strings.TrimSuffix(host, suffix)
			if prefix == "" || strings.Contains(prefix, ".") {
				continue
			}
			return true
		}
	}
	return false
}

// encodeTLSCert turns the in-memory *tls.Certificate returned by autocert
// into PEM bytes + expiry. PKCS#1 for RSA, PKCS#8 for others.
func encodeTLSCert(cert *tls.Certificate) (certPEM []byte, keyPEM []byte, notAfter time.Time, err error) {
	if cert == nil || len(cert.Certificate) == 0 {
		return nil, nil, time.Time{}, fmt.Errorf("empty certificate chain")
	}
	for _, der := range cert.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	switch pk := cert.PrivateKey.(type) {
	case *rsa.PrivateKey:
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pk)})
	default:
		b, merr := x509.MarshalPKCS8PrivateKey(pk)
		if merr != nil {
			return nil, nil, time.Time{}, fmt.Errorf("marshal private key: %w", merr)
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b})
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return nil, nil, time.Time{}, fmt.Errorf("failed to encode certificate/key")
	}
	leaf, perr := x509.ParseCertificate(cert.Certificate[0])
	if perr != nil {
		return certPEM, keyPEM, time.Time{}, fmt.Errorf("parse leaf: %w", perr)
	}
	return certPEM, keyPEM, leaf.NotAfter, nil
}
