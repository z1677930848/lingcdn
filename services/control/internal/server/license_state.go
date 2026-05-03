package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/store"
)

// licenseState is the in-memory shape of the control plane license. It
// mirrors store.LicenseState but is kept separate so handlers/JSON tags can
// evolve without churning the persistence layer.
type licenseState struct {
	Status      string    `json:"status"`       // active | expired | revoked | unlicensed | limited | paused
	LicenseKey  string    `json:"license_key"`  // raw license key
	ExpiresAt   time.Time `json:"expires_at"`   // expiration timestamp
	MaxNodes    int       `json:"max_nodes"`    // node quota
	LastChecked time.Time `json:"last_checked"` // last portal verification time
	GraceUntil  time.Time `json:"grace_until"`  // grace period end time
	Reason      string    `json:"reason"`       // invalid reason
	UpdatedAt   time.Time `json:"updated_at"`
	PubKey      string    `json:"pubkey,omitempty"`
	// ReportSecret is the HMAC key the portal expects on /api/control/report.
	// It is delivered by the portal in the license verify response (only when
	// the request IP matches license.bind_ip), then cached here so subsequent
	// system reports can sign successfully without manual key sync between
	// portal config.yaml and control plane config.yaml.
	ReportSecret string `json:"report_secret,omitempty"`
}

// staticLicenseRegistry is the schema for an offline license registry hosted
// at a static URL or filesystem path. It is consulted when
// staticLicenseIndexURL() is non-empty.
type staticLicenseRegistry struct {
	Version   int                   `json:"version"`
	UpdatedAt string                `json:"updated_at"`
	Licenses  []staticLicenseRecord `json:"licenses"`
}

type staticLicenseRecord struct {
	KeyHash      string   `json:"key_hash"`
	KeyHint      string   `json:"key_hint"`
	CustomerName string   `json:"customer_name"`
	Status       string   `json:"status"`
	Type         string   `json:"type"`
	Plan         string   `json:"plan"`
	MaxNodes     int      `json:"max_nodes"`
	ExpiresAt    string   `json:"expires_at"`
	Reason       string   `json:"reason"`
	Features     []string `json:"features"`
}

// portalLicenseVerifyResponse is the JSON shape returned by the auth portal's
// /api/licenses/verify endpoint. Signed license payloads ride in Payload +
// Signature so the control plane can verify them offline.
type portalLicenseVerifyResponse struct {
	OK        bool   `json:"ok"`
	ProductID string `json:"product_id"`
	ExpireAt  string `json:"expire_at"`
	MaxNodes  int    `json:"max_nodes"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	Payload   string `json:"payload"`
	Checksum  string `json:"checksum"`
	Signature string `json:"signature"`
	SigAlg    string `json:"sig_alg"`
	SigTarget string `json:"sig_target"`
	PubKey    string `json:"pubkey"`
	// ReportSecret is the top-level (unsigned) fallback channel for the
	// system-report HMAC key. New portals also embed it inside Payload so
	// signed responses carry it tamper-evidently; this field exists for
	// portals that have not yet enabled PORTAL_SIGNING_PRIVKEY.
	ReportSecret string `json:"report_secret"`
}

func (s *Servers) licenseMode() string {
	return config.NormalizeLicenseMode(s.cfg.LicenseMode)
}

func (s *Servers) isOpenLicenseMode() bool {
	return false
}

func (s *Servers) isOfflineLicenseMode() bool {
	return false
}

func normalizeOpenLicenseState(st licenseState, now time.Time) licenseState {
	st.Status = "unlicensed"
	st.ExpiresAt = time.Time{}
	st.MaxNodes = 0
	st.LastChecked = now
	st.GraceUntil = time.Time{}
	st.Reason = "official online authorization is required via auth.lingcdn.cloud"
	st.UpdatedAt = now
	return st
}

func (s *Servers) currentLicenseStatus() licenseState {
	s.licenseMu.RLock()
	defer s.licenseMu.RUnlock()
	st := s.license
	return st
}

func (s *Servers) staticLicenseIndexURL() string {
	return ""
}

// setLicenseState atomically updates the in-memory license, persists it to
// both file and store (if configured), and republishes node config when the
// new state changes anything nodes care about. Persistence failure on either
// sink is logged at error level so operators can alert; total failure
// (neither sink succeeds) is itself logged at error so a restart-after-crash
// scenario does not silently lose authorization.
func (s *Servers) setLicenseState(st licenseState) {
	s.licenseMu.Lock()
	prev := s.license
	s.license = st
	s.licenseMu.Unlock()

	// Persist to file and store. Track success across sinks: at least one must succeed,
	// otherwise on next restart the license state is lost and customers face unexpected
	// downgrade. Log at error level so operators can alert on failures.
	fileOK := false
	storeOK := false

	if s.licenseFile != "" {
		if err := os.MkdirAll(filepath.Dir(s.licenseFile), 0o755); err != nil {
			log.Error().Err(err).Str("file", s.licenseFile).Msg("failed to create license directory")
		} else if err := os.WriteFile(s.licenseFile, mustJSON(st), 0o644); err != nil {
			log.Error().Err(err).Str("file", s.licenseFile).Msg("failed to persist license state to file")
		} else {
			fileOK = true
		}
	}
	if s.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := s.store.SetLicenseState(ctx, toStoreLicense(st)); err != nil {
			log.Error().Err(err).Msg("failed to persist license state to store")
		} else {
			storeOK = true
		}
		cancel()
	}
	if !fileOK && !storeOK {
		log.Error().
			Str("status", st.Status).
			Msg("license state not persisted anywhere; restart may lose authorization")
	}

	// Publish updated config when license changes affect node behavior.
	if s.publisher != nil && licenseStateAffectsNodes(prev, st) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.publisher.Publish(ctx, "", nil); err != nil {
				log.Warn().Err(err).Msg("publish after license update failed")
			}
		}()
	}
}

// licenseStateAffectsNodes returns true when the change between prev and next
// is observable to nodes (status flip, expiry change, or reason change). This
// gates the cost of a full publish.
func licenseStateAffectsNodes(prev, next licenseState) bool {
	if strings.ToLower(strings.TrimSpace(prev.Status)) != strings.ToLower(strings.TrimSpace(next.Status)) {
		return true
	}
	if !prev.ExpiresAt.Equal(next.ExpiresAt) {
		return true
	}
	if strings.TrimSpace(prev.Reason) != strings.TrimSpace(next.Reason) {
		return true
	}
	return false
}

// ensureLicenseStatus normalizes the in-memory license, defaulting blank
// status to "unlicensed" and demoting expired-active licenses to "expired".
// Callers receive the normalized snapshot.
func (s *Servers) ensureLicenseStatus() licenseState {
	s.licenseMu.Lock()
	defer s.licenseMu.Unlock()
	st := s.license
	now := time.Now()
	if st.Status == "" {
		st.Status = "unlicensed"
		st.Reason = "system unlicensed"
	}
	if st.Status == "active" && !st.ExpiresAt.IsZero() && now.After(st.ExpiresAt) {
		st.Status = "expired"
		st.Reason = "license expired"
		st.UpdatedAt = now
		s.license = st
	}
	return st
}

func (s *Servers) loadLicenseFromFile() error {
	if s.licenseFile == "" {
		return nil
	}
	data, err := os.ReadFile(s.licenseFile)
	if err != nil {
		return err
	}
	var st licenseState
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	if st.Status == "" {
		st.Status = "unlicensed"
		st.Reason = "system unlicensed"
	}
	s.setLicenseState(st)
	return nil
}

func (s *Servers) loadLicenseFromStore() error {
	if s.store == nil {
		return nil
	}
	ctx, cancel := store.WithTimeout(context.Background())
	defer cancel()
	st, err := s.store.GetLicenseState(ctx)
	if err != nil || st == nil {
		return err
	}
	s.setLicenseState(fromStoreLicense(*st))
	return nil
}

// mustJSON marshals v with indent and swallows errors. Only used for license
// state persistence where the input shape is fixed and a marshal error would
// indicate a programming bug rather than a runtime condition we can recover
// from.
func mustJSON(v any) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return b
}

func toStoreLicense(st licenseState) *store.LicenseState {
	return &store.LicenseState{
		Status:       st.Status,
		LicenseKey:   st.LicenseKey,
		ExpiresAt:    st.ExpiresAt,
		MaxNodes:     st.MaxNodes,
		LastChecked:  st.LastChecked,
		GraceUntil:   st.GraceUntil,
		Reason:       st.Reason,
		UpdatedAt:    st.UpdatedAt,
		PubKey:       st.PubKey,
		ReportSecret: st.ReportSecret,
	}
}

func fromStoreLicense(st store.LicenseState) licenseState {
	return licenseState{
		Status:       st.Status,
		LicenseKey:   st.LicenseKey,
		ExpiresAt:    st.ExpiresAt,
		MaxNodes:     st.MaxNodes,
		LastChecked:  st.LastChecked,
		GraceUntil:   st.GraceUntil,
		Reason:       st.Reason,
		UpdatedAt:    st.UpdatedAt,
		PubKey:       st.PubKey,
		ReportSecret: st.ReportSecret,
	}
}

// licenseVerifyLoop periodically verifies current license with portal.
func (s *Servers) licenseVerifyLoop(ctx context.Context) {
	interval := s.cfg.LicenseVerifyInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Trigger an immediate verify once at loop startup.
	runBGTask("license.verify", func() (string, error) {
		return s.verifyLicenseOnce(context.Background())
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runBGTask("license.verify", func() (string, error) {
				return s.verifyLicenseOnce(context.Background())
			})
		}
	}
}

// verifyLicenseOnce performs a single round-trip verification against the
// portal (or the static index, if configured) and updates local state. It
// returns a short status string for the bg-task log and an error for callers
// that wish to surface the failure.
func (s *Servers) verifyLicenseOnce(ctx context.Context) (string, error) {
	st := s.currentLicenseStatus()
	if st.LicenseKey == "" {
		return "skip: license key missing", nil
	}

	if indexURL := s.staticLicenseIndexURL(); indexURL != "" {
		return s.verifyLicenseOnceFromStaticIndex(ctx, indexURL, st)
	}

	respStatus, data, err := s.requestPortalLicenseVerify(ctx, st.LicenseKey)
	if err != nil {
		log.Warn().Err(err).Msg("license verify request failed")
		s.onLicenseVerifyFailed("verify request failed")
		return "", err
	}
	if !data.OK || respStatus != http.StatusOK {
		reason := data.Error
		if reason == "" {
			reason = fmt.Sprintf("verify failed: status %d", respStatus)
		}
		remoteStatus := normalizeVerifyFailureStatus(data.Status, data.Error)
		if remoteStatus == "" && respStatus == http.StatusUnauthorized && strings.EqualFold(strings.TrimSpace(data.Error), "license not active") {
			// Compatibility for older portal responses without explicit status.
			remoteStatus = "paused"
		}
		if remoteStatus != "" {
			s.applyVerifiedLicenseStatus(remoteStatus, reason)
		} else {
			s.onLicenseVerifyFailed(reason)
		}
		return "", errors.New(reason)
	}

	newState, err := s.buildVerifiedLicenseState(st, data)
	if err != nil {
		s.onLicenseVerifyFailed(err.Error())
		return "", err
	}

	newState.LastChecked = time.Now()
	newState.UpdatedAt = time.Now()
	newState.GraceUntil = time.Time{}
	s.setLicenseState(newState)
	log.Info().Msg("license verify ok")
	return "ok", nil
}

func (s *Servers) requestPortalLicenseVerify(ctx context.Context, licenseKey string) (int, portalLicenseVerifyResponse, error) {
	var data portalLicenseVerifyResponse
	client := &http.Client{Timeout: 10 * time.Second}
	body, _ := json.Marshal(map[string]any{
		"key": strings.TrimSpace(licenseKey),
	})
	url := s.portalBase() + "/api/licenses/verify"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, data, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return resp.StatusCode, data, err
	}
	return resp.StatusCode, data, nil
}

// buildVerifiedLicenseState merges the portal response into the current state,
// preferring the cryptographically signed payload when present. Falls back to
// flat fields when the portal returns an unsigned response (legacy mode).
func (s *Servers) buildVerifiedLicenseState(current licenseState, data portalLicenseVerifyResponse) (licenseState, error) {
	newState := current
	if data.Payload != "" && data.Signature != "" {
		pinnedPubKey := strings.TrimSpace(s.cfg.LicensePubKey)
		if pinnedPubKey == "" && !s.cfg.AllowInsecureLicensePubKey {
			return newState, errors.New("license pubkey not configured")
		}
		if pinnedPubKey == "" && s.cfg.AllowInsecureLicensePubKey {
			log.Warn().Msg("license pubkey not configured, falling back to pubkey from verify response (insecure)")
		}
		parsed, err := verifySignedLicense(data.Payload, data.Checksum, data.Signature, firstNonEmpty(pinnedPubKey, data.PubKey))
		if err != nil {
			return newState, fmt.Errorf("signature verify failed: %w", err)
		}
		newState = parsed
		newState.LicenseKey = current.LicenseKey
		// Don't drop a previously-cached report_secret if the portal omitted
		// it from this round-trip (e.g. running on an older portal that
		// signs the payload but doesn't yet embed report_secret in it).
		// The unsigned top-level field is the second-chance source.
		if strings.TrimSpace(newState.ReportSecret) == "" {
			if v := strings.TrimSpace(data.ReportSecret); v != "" {
				newState.ReportSecret = v
			} else {
				newState.ReportSecret = current.ReportSecret
			}
		}
		return newState, nil
	}

	newState.Status = "active"
	newState.Reason = ""
	newState.GraceUntil = time.Time{}
	if data.MaxNodes > 0 {
		newState.MaxNodes = data.MaxNodes
	}
	if data.ExpireAt != "" {
		if t, err := time.Parse(time.RFC3339, data.ExpireAt); err == nil {
			newState.ExpiresAt = t
		}
	}
	// Prefer a freshly-delivered report_secret; if the portal didn't send
	// one this round (legacy portal, or IP no longer matches bind_ip), keep
	// whatever we cached previously rather than clearing it.
	if v := strings.TrimSpace(data.ReportSecret); v != "" {
		newState.ReportSecret = v
	}
	return newState, nil
}

func (s *Servers) verifyLicenseOnceFromStaticIndex(ctx context.Context, indexURL string, current licenseState) (string, error) {
	newState, err := s.lookupLicenseFromStaticIndex(ctx, indexURL, current.LicenseKey)
	if err != nil {
		log.Warn().Err(err).Str("url", indexURL).Msg("static license verify request failed")
		s.onLicenseVerifyFailed("static registry verify failed")
		return "", err
	}

	now := time.Now()
	newState.LicenseKey = current.LicenseKey
	newState.LastChecked = now
	newState.UpdatedAt = now
	newState.GraceUntil = time.Time{}

	if newState.Status == "" {
		newState.Status = "active"
	}
	if strings.EqualFold(newState.Status, "suspended") {
		newState.Status = "paused"
	}
	if newState.Status == "active" && !newState.ExpiresAt.IsZero() && now.After(newState.ExpiresAt) {
		newState.Status = "expired"
		if strings.TrimSpace(newState.Reason) == "" {
			newState.Reason = "license expired"
		}
	}

	s.setLicenseState(newState)

	switch strings.ToLower(strings.TrimSpace(newState.Status)) {
	case "active", "expired", "limited":
		log.Info().Str("url", indexURL).Msg("static license verify ok")
		return "ok(static)", nil
	default:
		reason := strings.TrimSpace(newState.Reason)
		if reason == "" {
			reason = "license not active"
		}
		return "", errors.New(reason)
	}
}

func (s *Servers) lookupLicenseFromStaticIndex(ctx context.Context, indexURL, licenseKey string) (licenseState, error) {
	parsed, err := loadStaticLicenseRegistry(ctx, indexURL)
	if err != nil {
		return licenseState{}, err
	}
	keyHash := licenseKeyHash(licenseKey)
	for _, item := range parsed.Licenses {
		if !strings.EqualFold(strings.TrimSpace(item.KeyHash), keyHash) {
			continue
		}
		st := licenseState{
			Status:   strings.ToLower(strings.TrimSpace(item.Status)),
			MaxNodes: item.MaxNodes,
			Reason:   strings.TrimSpace(item.Reason),
		}
		if st.Status == "" {
			st.Status = "active"
		}
		if strings.TrimSpace(item.ExpiresAt) != "" {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(item.ExpiresAt)); err == nil {
				st.ExpiresAt = t
			}
		}
		return st, nil
	}
	return licenseState{
		Status: "unlicensed",
		Reason: "license not found in static registry",
	}, nil
}

// loadStaticLicenseRegistry fetches the registry over HTTP(S) or reads it
// from a local file path, depending on the indexURL scheme. Returning an
// empty registry is treated as a verify failure by callers, not silently
// passed through.
func loadStaticLicenseRegistry(ctx context.Context, indexURL string) (staticLicenseRegistry, error) {
	var parsed staticLicenseRegistry
	trimmedURL := strings.TrimSpace(indexURL)
	if trimmedURL == "" {
		return parsed, errors.New("static license index url is empty")
	}

	if strings.HasPrefix(strings.ToLower(trimmedURL), "http://") || strings.HasPrefix(strings.ToLower(trimmedURL), "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimmedURL, nil)
		if err != nil {
			return parsed, err
		}
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			return parsed, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return parsed, fmt.Errorf("static registry responded with %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return parsed, err
		}
		return parsed, nil
	}

	data, err := os.ReadFile(trimmedURL)
	if err != nil {
		return parsed, err
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func licenseKeyHash(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

// onLicenseVerifyFailed is called when verify could not reach the portal or
// returned an unparseable response. It implements the grace-period policy:
// active licenses with a configured grace window stay active until the window
// elapses, while never-active licenses fall back to "unlicensed" rather than
// "revoked" (which would imply something was once granted).
func (s *Servers) onLicenseVerifyFailed(reason string) {
	st := s.currentLicenseStatus()
	now := time.Now()
	reason = strings.TrimSpace(reason)
	// Keep active licenses in grace mode on transient verify failures.
	if strings.ToLower(strings.TrimSpace(st.Status)) == "active" &&
		!st.LastChecked.IsZero() &&
		s.cfg.LicenseGraceHours > 0 &&
		(st.ExpiresAt.IsZero() || st.ExpiresAt.After(now)) {
		if st.GraceUntil.IsZero() || st.GraceUntil.Before(now) {
			st.GraceUntil = now.Add(time.Duration(s.cfg.LicenseGraceHours) * time.Hour)
		}
		st.Reason = reason
		st.LastChecked = now
		st.UpdatedAt = now
		s.setLicenseState(st)
		return
	}

	// Preserve "unlicensed" status when the system was never licensed;
	// only mark as "revoked" if there was a prior active/expired license.
	prev := strings.ToLower(strings.TrimSpace(st.Status))
	if prev == "" || prev == "unlicensed" {
		st.Status = "unlicensed"
	} else {
		st.Status = "revoked"
	}
	st.Reason = reason
	st.LastChecked = now
	st.UpdatedAt = now
	s.setLicenseState(st)
}

// normalizeVerifyFailureStatus maps portal-returned status / reason text to
// our canonical status enum. "suspended" is normalized to "paused"; an empty
// status falls back to a heuristic on the reason text.
func normalizeVerifyFailureStatus(status, reason string) string {
	st := strings.ToLower(strings.TrimSpace(status))
	switch st {
	case "active", "expired", "revoked", "unlicensed", "limited", "paused", "suspended":
		if st == "suspended" {
			return "paused"
		}
		return st
	}

	lower := strings.ToLower(strings.TrimSpace(reason))
	switch lower {
	case "license paused", "license suspended":
		return "paused"
	case "license revoked":
		return "revoked"
	case "license expired":
		return "expired"
	case "license not found":
		return "unlicensed"
	}
	return ""
}

// applyVerifiedLicenseStatus is invoked when the portal returns a structured
// non-OK response (e.g. paused / revoked) — the verify itself succeeded but
// the license is no longer valid. Distinct from onLicenseVerifyFailed which
// covers transport errors.
func (s *Servers) applyVerifiedLicenseStatus(status, reason string) {
	st := s.currentLicenseStatus()
	now := time.Now()
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "suspended" {
		normalized = "paused"
	}
	if normalized == "" {
		normalized = "revoked"
	}
	st.Status = normalized
	st.Reason = strings.TrimSpace(reason)
	if st.Status == "paused" && st.Reason == "" {
		st.Reason = "system license is no longer valid, please reactivate via auth.lingcdn.cloud"
	}
	st.GraceUntil = time.Time{}
	st.LastChecked = now
	st.UpdatedAt = now
	s.setLicenseState(st)
}

// verifySignedLicense verifies signature against payload checksum, returns parsed licenseState.
func verifySignedLicense(payload, checksum, signature, pubkey string) (licenseState, error) {
	var parsed licenseState
	if payload == "" || signature == "" || pubkey == "" {
		return parsed, errors.New("missing payload/signature/pubkey")
	}
	// recompute checksum
	sum := sha256.Sum256([]byte(payload))
	sumHex := hex.EncodeToString(sum[:])
	if checksum != "" && !strings.EqualFold(checksum, sumHex) {
		return parsed, fmt.Errorf("checksum mismatch")
	}
	if err := verifyChecksumSignature(pubkey, sumHex, signature); err != nil {
		return parsed, err
	}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return parsed, err
	}
	parsed.PubKey = pubkey
	return parsed, nil
}

// verifyChecksumSignature verifies an ed25519 signature over the canonical
// "lingcdn:v1:sha256:<hex>" message. Length pre-checks ensure we never call
// ed25519.Verify with malformed inputs (which would otherwise panic).
func verifyChecksumSignature(pubKeyBase64, checksumHex, sigBase64 string) error {
	pubKeyBase64 = strings.TrimSpace(pubKeyBase64)
	if pubKeyBase64 == "" {
		return errors.New("missing pubkey")
	}
	pubRaw, err := base64.StdEncoding.DecodeString(pubKeyBase64)
	if err != nil {
		return fmt.Errorf("decode pubkey: %w", err)
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return fmt.Errorf("unexpected pubkey length: %d", len(pubRaw))
	}
	sigRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigBase64))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(sigRaw) != ed25519.SignatureSize {
		return fmt.Errorf("unexpected signature length: %d", len(sigRaw))
	}
	msg := "lingcdn:v1:sha256:" + strings.ToLower(checksumHex)
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), []byte(msg), sigRaw) {
		return errors.New("signature verify failed")
	}
	return nil
}
