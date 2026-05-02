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
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

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

		// Parse PEM to extract expiry
		var expiresAt time.Time
		block, _ := pem.Decode([]byte(certPEM))
		if block != nil {
			if leaf, err := x509.ParseCertificate(block.Bytes); err == nil {
				expiresAt = leaf.NotAfter
			}
		}

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
	s.bindCertAndPublish(ctx, record, domainCfg)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"id":         record.ID,
		"domain":     host,
		"expires_at": notAfter,
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
	if s.publisher != nil {
		if perr := s.publisher.Publish(ctx, "", nil); perr != nil {
			log.Ctx(ctx).Warn().Err(perr).Msg("publish after cert upload failed")
		}
	}
}

// bindCertAndPublish binds a certificate to a domain and publishes config.
// Used after ACME issuance.
func (s *Servers) bindCertAndPublish(ctx context.Context, cert *store.Certificate, domainCfg *store.Domain) {
	if domainCfg == nil {
		return
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
	if s.publisher != nil {
		if err := s.publisher.Publish(ctx, "", nil); err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("domain", domainCfg.Name).Msg("publish after acme cert failed")
		} else {
			log.Ctx(ctx).Info().Str("domain", domainCfg.Name).Msg("acme cert issued and config published")
		}
	}
}

// persistACMECertificate is used by the renewal loop to update an existing
// ACME certificate record in place. It finds the cert by domain, updates
// PEM/expiry/status, binds the domain, and publishes.
func (s *Servers) persistACMECertificate(
	ctx context.Context,
	host string,
	certPEM, keyPEM []byte,
	expiresAt time.Time,
	domainCfg *store.Domain,
) error {
	existing, err := s.store.GetCertificateByDomain(ctx, host)
	if err != nil {
		return fmt.Errorf("get certificate by domain: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("no existing certificate record for domain %s", host)
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

// tlsClientHelloFor builds the minimal ClientHelloInfo autocert needs.
func tlsClientHelloFor(host string) tls.ClientHelloInfo {
	return tls.ClientHelloInfo{ServerName: host}
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
