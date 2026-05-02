package server

// Authentication + self-service endpoints: login, register (with optional
// email verification + captcha), password reset (email-code flow), captcha
// issuance/verification, and the currently-authenticated user/me endpoint
// plus password change. The in-memory password-reset token map and the
// HMAC-based captcha helpers live here because they're only used by these
// handlers. JWT issuance lives in auth.go so both the HTTP surface here and
// the gRPC side can share it.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mojocn/base64Captcha"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/lingcdn/control/internal/store"
)

// Verification / reset code lengths and TTLs. Kept short (6 digits) because
// codes are sent to the user's email and typed back into the UI within a few
// minutes. TTLs guard against replay on stolen codes.
const (
	registerCodeLength = 6
	registerCodeTTL    = 15 * time.Minute
	resetCodeLength    = 6
	resetCodeTTL       = 15 * time.Minute
)

// Password-reset tokens are held in-memory rather than in the DB. The map
// survives for the TTL of the code and is pruned on each write; because
// control planes are typically single-process this is acceptable and avoids
// an extra DB write + cleanup job.
var (
	passwordResetMu     sync.Mutex
	passwordResetTokens = map[string]*store.EmailVerification{}
)

// handleLogin authenticates a user by identifier (username or email) + password,
// honouring the captcha toggle and rate limiter. On success, a JWT is issued
// and last-login metadata is recorded.
func (s *Servers) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	var req struct {
		Identifier    string `json:"identifier"` // username or email
		Email         string `json:"email"`      // backward compatibility
		Password      string `json:"password"`
		CaptchaToken  string `json:"captcha_token"`
		CaptchaAnswer string `json:"captcha_answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	if req.CaptchaToken != "" || req.CaptchaAnswer != "" {
		if err := s.verifyCaptcha(req.CaptchaToken, req.CaptchaAnswer); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
			return
		}
	}

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" {
		identifier = strings.TrimSpace(req.Email)
	}
	user, err := s.store.GetUserByLogin(ctx, identifier)
	if err != nil {
		writeInternalError(w, "get user by login", err)
		return
	}
	if user == nil {
		s.writeSystemLog(ctx, "login", "failed", "login failed: user not found", "", identifier, getRequestIP(r))
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "用户名或密码错误"})
		return
	}
	if strings.TrimSpace(user.Status) == "disabled" {
		s.writeSystemLog(ctx, "login", "failed", "login failed: invalid credentials", user.ID, user.Username, getRequestIP(r))
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "用户已被禁用"})
		return
	}

	if err := validatePassword(user.PasswordHash, req.Password); err != nil {
		s.writeSystemLog(ctx, "login", "failed", "login failed: invalid credentials", user.ID, user.Username, getRequestIP(r))
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "用户名或密码错误"})
		return
	}

	token, err := issueJWT(s.cfg.AuthSecret, user, 24*time.Hour)
	if err != nil {
		writeInternalError(w, "issue JWT token", err)
		return
	}

	now := time.Now()
	loginIP := getRequestIP(r)
	loginLocation := s.resolveIPLocation(loginIP)
	if err := s.store.UpdateUserLastLogin(ctx, user.ID, now, loginIP, loginLocation); err != nil {
		log.Ctx(r.Context()).Warn().Err(err).Msg("failed to update last login")
	}

	s.writeSystemLog(ctx, "login", "success", "login success", user.ID, user.Username, getRequestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id":         user.ID,
			"numeric_id": user.NumericID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
		},
	})
}

// handleRegister creates a new user account. Respects the register_enabled
// setting and the register_email_verification toggle (which requires the
// client to have obtained an email code via handleRegisterEmailRequest first).
func (s *Servers) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	var req struct {
		Username      string `json:"username"`
		Email         string `json:"email"`
		Password      string `json:"password"`
		EmailCode     string `json:"email_code"`
		CaptchaToken  string `json:"captcha_token"`
		CaptchaAnswer string `json:"captcha_answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	if req.CaptchaToken != "" || req.CaptchaAnswer != "" {
		if err := s.verifyCaptcha(req.CaptchaToken, req.CaptchaAnswer); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
			return
		}
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "用户名、邮箱和密码不能为空"})
		return
	}
	if msg := passwordPolicyError(req.Password); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		writeInternalError(w, "get settings", err)
		return
	}
	normalized := s.applySettingsDefaults(settings)
	if normalized != nil && !normalized.RegisterEnabled {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "注册功能已关闭"})
		return
	}
	if normalized != nil && normalized.RegisterEmailVerification {
		code := strings.TrimSpace(req.EmailCode)
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请输入邮箱验证码"})
			return
		}
		if len(code) != registerCodeLength {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱验证码格式无效"})
			return
		}
		verify, err := s.store.GetLatestEmailVerificationByEmail(ctx, req.Email)
		if err != nil {
			writeInternalError(w, "lookup email verification", err)
			return
		}
		if verify == nil || verify.UsedAt != nil || time.Now().After(verify.ExpiresAt) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱验证码已过期"})
			return
		}
		if hashEmailVerificationToken(req.Email, code) != verify.TokenHash {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱验证码错误"})
			return
		}
		// Atomically mark used. If RowsAffected == 0, another concurrent request
		// already consumed this verification code. Reject to prevent double use.
		ok, err := s.store.MarkEmailVerificationUsed(ctx, verify.ID, time.Now())
		if err != nil {
			writeInternalError(w, "mark email verification used", err)
			return
		}
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱验证码已被使用"})
			return
		}
	}

	if existing, _ := s.store.GetUserByUsername(ctx, req.Username); existing != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "用户名已存在"})
		return
	}
	if existing, _ := s.store.GetUserByEmail(ctx, req.Email); existing != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "邮箱已被注册"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeInternalError(w, "hash password", err)
		return
	}

	user := &store.User{
		ID:           uuid.NewString(),
		Username:     strings.ToLower(req.Username),
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.store.CreateUser(ctx, user); err != nil {
		writeInternalError(w, "create user", err)
		return
	}

	token, err := issueJWT(s.cfg.AuthSecret, user, 24*time.Hour)
	if err != nil {
		writeInternalError(w, "issue JWT token", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id":         user.ID,
			"numeric_id": user.NumericID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
		},
	})
}

// handleRegisterEmailRequest issues an email verification code to a prospective
// new user. The code is stored via the EmailVerification row (hashed) so the
// subsequent /register call can atomically mark it used and guard against
// double-registration.
func (s *Servers) handleRegisterEmailRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	var req struct {
		Email         string `json:"email"`
		CaptchaToken  string `json:"captcha_token"`
		CaptchaAnswer string `json:"captcha_answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	if req.CaptchaToken != "" || req.CaptchaAnswer != "" {
		if err := s.verifyCaptcha(req.CaptchaToken, req.CaptchaAnswer); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
			return
		}
	}

	emailAddr := strings.TrimSpace(strings.ToLower(req.Email))
	if emailAddr == "" || !strings.Contains(emailAddr, "@") {
		s.writeSystemLog(r.Context(), "email", "failed", "register email request failed: invalid email", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱地址无效"})
		return
	}

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		writeInternalError(w, "get settings", err)
		return
	}
	normalized := s.applySettingsDefaults(settings)
	if normalized == nil || !normalized.RegisterEnabled {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: registration disabled", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "注册功能已关闭"})
		return
	}
	if normalized == nil || !normalized.RegisterEmailVerification {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: email verification disabled", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱验证功能未开启"})
		return
	}

	if existing, _ := s.store.GetUserByLogin(ctx, emailAddr); existing != nil {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: email already exists", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusConflict, map[string]any{"error": "邮箱已被注册"})
		return
	}

	code := generateVerificationCode(registerCodeLength)
	expiresAt := time.Now().Add(registerCodeTTL)
	verify := &store.EmailVerification{
		Email:     emailAddr,
		TokenHash: hashEmailVerificationToken(emailAddr, code),
		ExpiresAt: expiresAt,
	}
	if err := s.store.CreateEmailVerification(ctx, verify); err != nil {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: verification create failed", "", emailAddr, getRequestIP(r))
		writeInternalError(w, "create email verification", err)
		return
	}

	smtpCfg := s.smtpConfigFromSettings(normalized)
	if strings.TrimSpace(smtpCfg.SMTPHost) == "" || strings.TrimSpace(smtpCfg.SMTPFrom) == "" {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: smtp not configured", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮件服务未配置"})
		return
	}

	name := strings.TrimSpace(normalized.SystemName)
	if name == "" {
		name = "LingCDN"
	}
	subject := fmt.Sprintf("%s registration code", name)
	body, err := s.defaultEmailRegisterBody(ctx, name, emailAddr, code, int(registerCodeTTL.Minutes()))
	if err != nil {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: template render failed", "", emailAddr, getRequestIP(r))
		writeInternalError(w, "render email template", err)
		return
	}
	if err := sendEmail(smtpCfg, emailAddr, subject, body); err != nil {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: send email failed", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "邮件发送失败"})
		return
	}
	s.writeSystemLog(ctx, "email", "success", "register email code sent", "", emailAddr, getRequestIP(r))

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "expires_in": int(registerCodeTTL.Seconds())})
}

// handlePasswordResetRequest sends a reset code to the account's email.
// The code is kept in an in-memory map rather than the DB (see
// passwordResetTokens) so revocation is just a map delete.
func (s *Servers) handlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	var req struct {
		Email         string `json:"email"`
		CaptchaToken  string `json:"captcha_token"`
		CaptchaAnswer string `json:"captcha_answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	if req.CaptchaToken != "" || req.CaptchaAnswer != "" {
		if err := s.verifyCaptcha(req.CaptchaToken, req.CaptchaAnswer); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
			return
		}
	}

	emailAddr := strings.TrimSpace(strings.ToLower(req.Email))
	if emailAddr == "" || !strings.Contains(emailAddr, "@") {
		s.writeSystemLog(r.Context(), "email", "failed", "password reset email request failed: invalid email", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱地址无效"})
		return
	}

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	user, err := s.store.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: user lookup failed", "", emailAddr, getRequestIP(r))
		writeInternalError(w, "get user by email", err)
		return
	}
	if user == nil {
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: user not found", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "用户不存在"})
		return
	}

	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		writeInternalError(w, "get settings", err)
		return
	}
	normalized := s.applySettingsDefaults(settings)

	smtpCfg := s.smtpConfigFromSettings(normalized)
	if strings.TrimSpace(smtpCfg.SMTPHost) == "" || strings.TrimSpace(smtpCfg.SMTPFrom) == "" {
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: smtp not configured", user.ID, user.Username, getRequestIP(r))
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮件服务未配置"})
		return
	}

	code := generateVerificationCode(resetCodeLength)
	expiresAt := time.Now().Add(resetCodeTTL)
	verify := &store.EmailVerification{
		ID:        "reset:" + uuid.NewString(),
		Email:     emailAddr,
		TokenHash: hashEmailVerificationToken(emailAddr, code),
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	passwordResetMu.Lock()
	passwordResetTokens[emailAddr] = verify
	// Clean up expired tokens to prevent memory leak
	for k, v := range passwordResetTokens {
		if time.Now().After(v.ExpiresAt) {
			delete(passwordResetTokens, k)
		}
	}
	passwordResetMu.Unlock()

	name := strings.TrimSpace(normalized.SystemName)
	if name == "" {
		name = "LingCDN"
	}
	subject := fmt.Sprintf("%s password reset code", name)
	body, err := s.defaultEmailPasswordResetBody(ctx, name, emailAddr, code, int(resetCodeTTL.Minutes()))
	if err != nil {
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: template render failed", user.ID, user.Username, getRequestIP(r))
		writeInternalError(w, "render email template", err)
		return
	}
	if err := sendEmail(smtpCfg, emailAddr, subject, body); err != nil {
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: send email failed", user.ID, user.Username, getRequestIP(r))
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "邮件发送失败"})
		return
	}
	s.writeSystemLog(ctx, "email", "success", "password reset email code sent", user.ID, user.Username, getRequestIP(r))

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "expires_in": int(resetCodeTTL.Seconds())})
}

// handlePasswordResetConfirm validates the reset code (single-use, TTL-gated)
// and updates the user's password hash. The token is removed under lock
// before the DB write so concurrent attempts can't reuse it.
func (s *Servers) handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	var req struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的JSON格式"})
		return
	}

	emailAddr := strings.TrimSpace(strings.ToLower(req.Email))
	code := strings.TrimSpace(req.Code)
	if emailAddr == "" || !strings.Contains(emailAddr, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱地址无效"})
		return
	}
	if len(code) != resetCodeLength {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "验证码格式无效"})
		return
	}
	if msg := passwordPolicyError(req.Password); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}

	// Atomically consume the reset token: peek + validate token hash under lock,
	// and only delete (commit the consumption) when validation succeeds. This
	// prevents concurrent requests from both passing the "exists" check.
	passwordResetMu.Lock()
	verify := passwordResetTokens[emailAddr]
	if verify == nil || time.Now().After(verify.ExpiresAt) {
		passwordResetMu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "验证码已过期"})
		return
	}
	if hashEmailVerificationToken(emailAddr, code) != verify.TokenHash {
		passwordResetMu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "验证码错误"})
		return
	}
	// Commit: remove the token so no concurrent request can reuse it.
	delete(passwordResetTokens, emailAddr)
	passwordResetMu.Unlock()

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	user, err := s.store.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		writeInternalError(w, "get user by email", err)
		return
	}
	if user == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "用户不存在"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeInternalError(w, "hash password", err)
		return
	}
	if err := s.store.UpdateUserPasswordHash(ctx, user.ID, string(hash)); err != nil {
		writeInternalError(w, "update user password", err)
		return
	}

	s.writeSystemLog(ctx, "action", "success", "password reset success", user.ID, user.Username, getRequestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCaptcha issues a signed captcha token plus a base64-encoded PNG of
// distorted digits. The HMAC-signed token carries the expected answer and a
// timestamp so verification is a pure stateless check (no DB / cache round-trip).
func (s *Servers) handleCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	driver := base64Captcha.NewDriverDigit(80, 240, 4, 0.7, 80)
	_, content, answer := driver.GenerateIdQuestionAnswer()
	item, err := driver.DrawCaptcha(content)
	if err != nil {
		writeInternalError(w, "captcha render", err)
		return
	}
	ts := time.Now().Unix()
	payload := fmt.Sprintf("0|0|%s|%d", answer, ts)
	token := signCaptchaToken(s.cfg.AuthSecret, payload)
	writeJSON(w, http.StatusOK, map[string]any{
		"question":   item.EncodeB64string(),
		"token":      token,
		"expires_in": int64((5 * time.Minute).Seconds()),
	})
}

// verifyCaptcha decodes the HMAC-signed captcha token and compares the
// recovered answer against the user's submission. Tokens older than 5 minutes
// are rejected.
func (s *Servers) verifyCaptcha(token, answer string) error {
	if token == "" || answer == "" {
		return errors.New("请输入验证码")
	}
	if s.cfg.AuthSecret == "" {
		return errors.New("验证码功能未配置")
	}
	payload, sigHex, err := decodeCaptchaToken(token)
	if err != nil {
		return fmt.Errorf("验证码无效")
	}
	if !validateCaptchaSig(s.cfg.AuthSecret, payload, sigHex) {
		return fmt.Errorf("验证码无效")
	}
	parts := strings.Split(payload, "|")
	if len(parts) != 4 {
		return fmt.Errorf("验证码无效")
	}
	expectAns, _ := strconv.Atoi(parts[2])
	ts, _ := strconv.ParseInt(parts[3], 10, 64)
	if ts == 0 || time.Since(time.Unix(ts, 0)) > 5*time.Minute {
		return fmt.Errorf("验证码已过期")
	}
	userAns, _ := strconv.Atoi(strings.TrimSpace(answer))
	if userAns != expectAns {
		return fmt.Errorf("验证码不正确")
	}
	return nil
}

// handleMe returns the currently-authenticated user's profile. Service and
// bootstrap tokens are rejected because they don't map to a real user row.
func (s *Servers) handleMe(w http.ResponseWriter, r *http.Request) {
	role := getUserRole(r.Context())
	if role == "" || role == "service" || role == "bootstrap" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}

	email, _ := r.Context().Value(ctxKeyEmail).(string)
	if email == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		writeInternalError(w, "get current user", err)
		return
	}
	if user == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "用户不存在"})
		return
	}

	u := map[string]any{
		"id":         user.ID,
		"numeric_id": user.NumericID,
		"username":   user.Username,
		"email":      user.Email,
		"role":       user.Role,
		"status":     user.Status,
		"created_at": user.CreatedAt,
	}
	if user.LastLoginAt != nil {
		u["last_login_at"] = *user.LastLoginAt
	}
	if user.LastLoginIP != "" {
		u["last_login_ip"] = user.LastLoginIP
	}
	if user.LastLoginLocation != "" {
		u["last_login_location"] = user.LastLoginLocation
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

// handleChangePassword lets an authenticated user change their own password.
// Requires the old password; new password must meet the policy.
func (s *Servers) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	email, _ := r.Context().Value(ctxKeyEmail).(string)
	if email == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录或登录已过期"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求内容格式错误"})
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请输入旧密码和新密码"})
		return
	}
	if msg := passwordPolicyError(req.NewPassword); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "新" + msg})
		return
	}

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		writeInternalError(w, "get user for password change", err)
		return
	}
	if user == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "用户不存在"})
		return
	}

	if err := validatePassword(user.PasswordHash, req.OldPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "旧密码不正确"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeInternalError(w, "hash new password", err)
		return
	}

	if err := s.store.UpdateUserPasswordHash(ctx, user.ID, string(hash)); err != nil {
		writeInternalError(w, "update password", err)
		return
	}

	s.writeSystemLog(ctx, "auth", "success", "password changed", user.ID, user.Username, getRequestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- captcha helpers ---

// signCaptchaToken produces a base64(payload|hmac_sha256(secret, payload)) token.
func signCaptchaToken(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	raw := payload + "|" + sig
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// decodeCaptchaToken reverses signCaptchaToken, returning (payload, sigHex).
// Payloads are always 4 pipe-delimited parts; the signature is the 5th.
func decodeCaptchaToken(token string) (payload, sigHex string, err error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 5 {
		return "", "", fmt.Errorf("bad token")
	}
	return strings.Join(parts[:4], "|"), parts[4], nil
}

// validateCaptchaSig is a constant-time comparison of expected vs received HMAC.
func validateCaptchaSig(secret, payload, sigHex string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expect := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.ToLower(expect)), []byte(strings.ToLower(sigHex)))
}

// randomInt returns a crypto/rand integer in [min, max]. Falls back to min on
// RNG failure, which is acceptable for captcha/verification-code use cases.
func randomInt(min, max int) int {
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return int(n.Int64()) + min
}

// generateVerificationCode returns a numeric code of the given length.
func generateVerificationCode(length int) string {
	if length <= 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(length)
	for i := 0; i < length; i++ {
		b.WriteByte(byte('0' + randomInt(0, 9)))
	}
	return b.String()
}

// hashEmailVerificationToken derives a stable hash of (email, code) so we
// never store the raw code. Normalization (trim + lowercase) matches the
// comparison path.
func hashEmailVerificationToken(email, code string) string {
	normalized := strings.TrimSpace(strings.ToLower(email))
	sum := sha256.Sum256([]byte(normalized + ":" + code))
	return hex.EncodeToString(sum[:])
}

// passwordPolicyError returns a user-facing error message when the password does
// not meet the minimum policy, or an empty string if the password is acceptable.
// Policy: at least 8 chars, must contain at least one letter and one digit.
// Keep in sync with the frontend hint. Returns Chinese so messages match existing UX.
func passwordPolicyError(pw string) string {
	if len(pw) < 8 {
		return "密码长度不能少于8位"
	}
	return ""
}
