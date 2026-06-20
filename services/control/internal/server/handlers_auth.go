package server

// Authentication + self-service endpoints: login, register (with optional
// email verification + captcha), password reset (email-code flow), captcha
// issuance/verification, and the currently-authenticated user/me endpoint
// plus password change. The in-memory password-reset token map and the
// server-side captcha session store live here because they're only used by
// these handlers. JWT issuance lives in auth.go so both the HTTP surface
// here and the gRPC side can share it.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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

	// Captcha is skipped on the first attempt from an IP; after a failed auth
	// attempt the same IP must solve captcha on subsequent tries. Rate limiters
	// still throttle brute-force traffic.
	if err := s.enforceAuthCaptcha(r, req.CaptchaToken, req.CaptchaAnswer); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	loginIP := getRequestIP(r)
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
		s.writeSystemLog(ctx, "login", "failed", "login failed: user not found", "", identifier, loginIP)
		markAuthCaptchaRequired(loginIP)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "用户名或密码错误"})
		return
	}
	if strings.TrimSpace(user.Status) == "disabled" {
		s.writeSystemLog(ctx, "login", "failed", "login failed: invalid credentials", user.ID, user.Username, loginIP)
		markAuthCaptchaRequired(loginIP)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "用户已被禁用"})
		return
	}

	if err := validatePassword(user.PasswordHash, req.Password); err != nil {
		s.writeSystemLog(ctx, "login", "failed", "login failed: invalid credentials", user.ID, user.Username, loginIP)
		markAuthCaptchaRequired(loginIP)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "用户名或密码错误"})
		return
	}

	token, err := issueJWT(s.cfg.AuthSecret, user, 24*time.Hour)
	if err != nil {
		writeInternalError(w, "issue JWT token", err)
		return
	}

	now := time.Now()
	loginLocation := s.resolveIPLocation(loginIP)
	if err := s.store.UpdateUserLastLogin(ctx, user.ID, now, loginIP, loginLocation); err != nil {
		log.Ctx(r.Context()).Warn().Err(err).Msg("failed to update last login")
	}

	clearAuthCaptchaRequired(loginIP)
	s.writeSystemLog(ctx, "login", "success", "login success", user.ID, user.Username, loginIP)
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

	if err := s.enforceAuthCaptcha(r, req.CaptchaToken, req.CaptchaAnswer); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
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
		if !emailVerificationTokenEquals(req.Email, code, verify.TokenHash) {
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

	clearAuthCaptchaRequired(getRequestIP(r))
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

	if err := s.enforceAuthCaptcha(r, req.CaptchaToken, req.CaptchaAnswer); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}

	emailAddr := strings.TrimSpace(strings.ToLower(req.Email))
	requestIP := getRequestIP(r)
	if emailAddr == "" || !strings.Contains(emailAddr, "@") {
		s.writeSystemLog(r.Context(), "email", "failed", "register email request failed: invalid email", "", emailAddr, requestIP)
		markAuthCaptchaRequired(requestIP)
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
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: registration disabled", "", emailAddr, requestIP)
		markAuthCaptchaRequired(requestIP)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "注册功能已关闭"})
		return
	}
	if normalized == nil || !normalized.RegisterEmailVerification {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: email verification disabled", "", emailAddr, requestIP)
		markAuthCaptchaRequired(requestIP)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱验证功能未开启"})
		return
	}

	if existing, _ := s.store.GetUserByLogin(ctx, emailAddr); existing != nil {
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: email already exists", "", emailAddr, requestIP)
		markAuthCaptchaRequired(requestIP)
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
		s.writeSystemLog(ctx, "email", "failed", "register email request failed: send email failed", "", emailAddr, requestIP)
		markAuthCaptchaRequired(requestIP)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "邮件发送失败"})
		return
	}
	s.writeSystemLog(ctx, "email", "success", "register email code sent", "", emailAddr, requestIP)

	clearAuthCaptchaRequired(requestIP)
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

	if err := s.enforceAuthCaptcha(r, req.CaptchaToken, req.CaptchaAnswer); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}

	emailAddr := strings.TrimSpace(strings.ToLower(req.Email))
	requestIP := getRequestIP(r)
	if emailAddr == "" || !strings.Contains(emailAddr, "@") {
		s.writeSystemLog(r.Context(), "email", "failed", "password reset email request failed: invalid email", "", emailAddr, requestIP)
		markAuthCaptchaRequired(requestIP)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮箱地址无效"})
		return
	}

	ctx, cancel := store.WithTimeout(r.Context())
	defer cancel()

	// Resolve global config first so the SMTP-misconfiguration error path
	// is identical regardless of whether the email is registered. Without
	// this ordering, a "邮件服务未配置" response would only be returned for
	// known accounts and would itself leak registration status.
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		writeInternalError(w, "get settings", err)
		return
	}
	normalized := s.applySettingsDefaults(settings)

	smtpCfg := s.smtpConfigFromSettings(normalized)
	if strings.TrimSpace(smtpCfg.SMTPHost) == "" || strings.TrimSpace(smtpCfg.SMTPFrom) == "" {
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: smtp not configured", "", emailAddr, requestIP)
		markAuthCaptchaRequired(requestIP)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "邮件服务未配置"})
		return
	}

	// Account-existence check. The HTTP response for "user not found" must
	// match the happy-path response so attackers can't enumerate registered
	// emails by hammering this endpoint with addresses; only the internal
	// system log distinguishes the two cases for operators.
	uniformOK := map[string]any{"ok": true, "expires_in": int(resetCodeTTL.Seconds())}
	user, err := s.store.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: user lookup failed", "", emailAddr, getRequestIP(r))
		writeInternalError(w, "get user by email", err)
		return
	}
	if user == nil {
		// Pretend success: same JSON shape, same status code, no hint that
		// the email is unregistered. Still record the miss server-side so
		// abuse can be investigated.
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: user not found", "", emailAddr, getRequestIP(r))
		writeJSON(w, http.StatusOK, uniformOK)
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
		s.writeSystemLog(ctx, "email", "failed", "password reset email request failed: send email failed", user.ID, user.Username, requestIP)
		markAuthCaptchaRequired(requestIP)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "邮件发送失败"})
		return
	}
	s.writeSystemLog(ctx, "email", "success", "password reset email code sent", user.ID, user.Username, requestIP)

	clearAuthCaptchaRequired(requestIP)
	writeJSON(w, http.StatusOK, uniformOK)
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
	if !emailVerificationTokenEquals(emailAddr, code, verify.TokenHash) {
		passwordResetMu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "验证码错误"})
		return
	}
	// Commit: remove the token so no concurrent request can reuse it.
	// Restored below if the password update fails.
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
		passwordResetMu.Lock()
		passwordResetTokens[emailAddr] = verify
		passwordResetMu.Unlock()
		writeInternalError(w, "update user password", err)
		return
	}

	s.writeSystemLog(ctx, "action", "success", "password reset success", user.ID, user.Username, getRequestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCaptcha issues an opaque captcha handle plus a base64-encoded PNG
// of distorted digits. The expected answer is held only on the server in
// captchaSessions; the client gets back nothing more than a random token.
// This is the security-critical change versus the previous design, which
// embedded the answer in the token (a base64-decode away from the client).
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
	token, err := issueCaptchaSession(answer)
	if err != nil {
		writeInternalError(w, "captcha session", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"question":   item.EncodeB64string(),
		"token":      token,
		"expires_in": int64(captchaTTL.Seconds()),
	})
}

// verifyCaptcha looks up the server-side captcha session for the supplied
// token and consumes it (single-use), then compares the answer. Both
// success and failure remove the session so a wrong guess cannot be
// retried with the same token. Tokens older than captchaTTL are rejected
// even before the answer is checked.
func (s *Servers) verifyCaptcha(token, answer string) error {
	if token == "" || answer == "" {
		return errors.New("请输入验证码")
	}
	sess := consumeCaptchaSession(token)
	if sess == nil {
		return errors.New("验证码无效或已过期")
	}
	expect := strings.TrimSpace(sess.answer)
	got := strings.TrimSpace(answer)
	if expect == "" {
		return errors.New("验证码无效或已过期")
	}
	// Compare numerically when both sides parse as integers (the default
	// driver emits 4-digit numbers); otherwise fall back to a constant-time
	// byte compare so leading zeros and string variants both work.
	if e, eErr := strconv.Atoi(expect); eErr == nil {
		if g, gErr := strconv.Atoi(got); gErr == nil {
			if e != g {
				return errors.New("验证码不正确")
			}
			return nil
		}
	}
	if len(expect) != len(got) ||
		subtle.ConstantTimeCompare([]byte(expect), []byte(got)) != 1 {
		return errors.New("验证码不正确")
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
	if gid := strings.TrimSpace(user.GroupID); gid != "" {
		u["group_id"] = gid
		if g, gerr := s.store.GetUserGroup(ctx, gid); gerr == nil && g != nil {
			u["group_name"] = g.Name
			if perms := normalizeUserGroupPermissions(g.Permissions); len(perms) > 0 {
				u["permissions"] = perms
			}
		}
	}
	if perms, ok := r.Context().Value(ctxKeyPermissions).([]string); ok && len(perms) > 0 {
		u["permissions"] = perms
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

// captchaTTL is how long an issued captcha is valid before consumption.
// Five minutes matches the previous HMAC token lifetime so the user-facing
// behavior (UI countdown, "code expired" copy) does not change.
const captchaTTL = 5 * time.Minute

// authCaptchaGateTTL is how long a client IP must supply captcha after a
// failed public-auth attempt. Aligns with the frontend localStorage gate.
const authCaptchaGateTTL = 30 * time.Minute

var (
	authCaptchaMu       sync.Mutex
	authCaptchaRequired = map[string]time.Time{} // ip -> expiry
)

func markAuthCaptchaRequired(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	authCaptchaMu.Lock()
	defer authCaptchaMu.Unlock()
	pruneAuthCaptchaRequiredLocked()
	authCaptchaRequired[ip] = time.Now().Add(authCaptchaGateTTL)
}

func clearAuthCaptchaRequired(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	authCaptchaMu.Lock()
	defer authCaptchaMu.Unlock()
	delete(authCaptchaRequired, ip)
}

func authCaptchaRequiredForIP(ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false
	}
	authCaptchaMu.Lock()
	defer authCaptchaMu.Unlock()
	pruneAuthCaptchaRequiredLocked()
	exp, ok := authCaptchaRequired[ip]
	return ok && time.Now().Before(exp)
}

func pruneAuthCaptchaRequiredLocked() {
	now := time.Now()
	for k, v := range authCaptchaRequired {
		if !now.Before(v) {
			delete(authCaptchaRequired, k)
		}
	}
}

// enforceAuthCaptcha skips captcha on the first attempt from an IP. After a
// prior failed auth attempt (or a bad captcha submission) the same IP must
// solve captcha before retrying.
func (s *Servers) enforceAuthCaptcha(r *http.Request, token, answer string) error {
	token = strings.TrimSpace(token)
	answer = strings.TrimSpace(answer)
	ip := getRequestIP(r)
	if token == "" && answer == "" {
		if authCaptchaRequiredForIP(ip) {
			return errors.New("请输入验证码")
		}
		return nil
	}
	if err := s.verifyCaptcha(token, answer); err != nil {
		markAuthCaptchaRequired(ip)
		return err
	}
	return nil
}

// captchaTokenLen is the byte length of the random handle generated for
// each captcha. 24 bytes (192 bits) is far beyond what's needed for a
// 5-minute single-use token but matches our other random-id sizing.
const captchaTokenLen = 24

// captchaSession records an issued captcha challenge. Only the answer and
// expiry are kept server-side; the client receives a random token only.
type captchaSession struct {
	answer    string
	expiresAt time.Time
}

var (
	captchaMu       sync.Mutex
	captchaSessions = make(map[string]*captchaSession)
)

// issueCaptchaSession generates a fresh random token bound to the supplied
// answer and stores it in captchaSessions. The token is the only thing the
// client gets back; the answer never leaves the process.
func issueCaptchaSession(answer string) (string, error) {
	buf := make([]byte, captchaTokenLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)

	captchaMu.Lock()
	defer captchaMu.Unlock()
	captchaSessions[token] = &captchaSession{
		answer:    strings.TrimSpace(answer),
		expiresAt: time.Now().Add(captchaTTL),
	}
	pruneCaptchaSessionsLocked()
	return token, nil
}

// consumeCaptchaSession atomically removes and returns the session for the
// given token. The deletion is unconditional so a wrong guess cannot be
// retried by replaying the token. Returns nil for unknown / expired
// tokens.
func consumeCaptchaSession(token string) *captchaSession {
	if token == "" {
		return nil
	}
	captchaMu.Lock()
	defer captchaMu.Unlock()
	s, ok := captchaSessions[token]
	if !ok {
		return nil
	}
	delete(captchaSessions, token)
	if time.Now().After(s.expiresAt) {
		return nil
	}
	return s
}

// pruneCaptchaSessionsLocked drops expired entries. Called opportunistically
// on each issue (cheap because the map only ever holds tokens issued within
// the last captchaTTL minutes). Must be called with captchaMu held.
func pruneCaptchaSessionsLocked() {
	now := time.Now()
	for k, v := range captchaSessions {
		if now.After(v.expiresAt) {
			delete(captchaSessions, k)
		}
	}
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

// emailVerificationTokenEquals constant-time compares a freshly derived
// (email, code) hash against the stored TokenHash. Using subtle here is
// defense-in-depth: the hashes are random-looking SHA-256 hex so the
// timing channel is narrow, but a plain `!=` short-circuits on the first
// differing byte and there's no reason to leak even that little.
func emailVerificationTokenEquals(email, code, stored string) bool {
	derived := hashEmailVerificationToken(email, code)
	if len(derived) != len(stored) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(derived), []byte(stored)) == 1
}

// passwordPolicyError returns a user-facing error message when the password does
// not meet the minimum policy, or an empty string if the password is acceptable.
// Policy: at least 8 chars, must contain at least one letter and one digit.
// Keep in sync with the frontend hint. Returns Chinese so messages match existing UX.
func passwordPolicyError(pw string) string {
	if len(pw) < 8 {
		return "密码长度不能少于8位"
	}
	// We don't reuse `unicode.IsLetter` / `unicode.IsDigit` here because the
	// frontend / docs both speak in terms of ASCII letters and digits, which
	// is what most operators expect for "letter + digit" rules. This avoids
	// false-positives for inputs that happen to contain non-Latin letters
	// or East-Asian fullwidth digits while missing real ASCII variety.
	hasLetter := false
	hasDigit := false
	for i := 0; i < len(pw); i++ {
		c := pw[i]
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			hasLetter = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
		if hasLetter && hasDigit {
			break
		}
	}
	if !hasLetter || !hasDigit {
		return "密码必须同时包含字母和数字"
	}
	return ""
}
