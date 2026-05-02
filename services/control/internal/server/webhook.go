package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// UpgradeEvent is payload from upgrade webhook.
type UpgradeEvent struct {
	Event       string `json:"event"`
	Product     string `json:"product"`
	Version     string `json:"version"`
	Channel     string `json:"channel"`
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
	DownloadURL string `json:"download_url"`
	Checksum    string `json:"checksum"`
	Signature   string `json:"signature"`
	Changelog   string `json:"changelog"`
	Timestamp   string `json:"timestamp"`
}

// HandleUpgradeWebhook handles upgrade webhook callbacks.
func (s *Servers) HandleUpgradeWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read webhook body")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("X-Webhook-Signature")
	if !s.verifyWebhookSignature(body, signature) {
		log.Warn().Msg("webhook signature verification failed")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var event UpgradeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Error().Err(err).Msg("failed to parse webhook event")
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if event.Timestamp != "" {
		ts, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			log.Warn().Err(err).Msg("invalid webhook timestamp")
			http.Error(w, "invalid timestamp", http.StatusUnauthorized)
			return
		}
		if time.Since(ts).Abs() > 5*time.Minute {
			log.Warn().Time("ts", ts).Msg("webhook timestamp too far from now")
			http.Error(w, "stale request", http.StatusUnauthorized)
			return
		}
	}

	if event.Event != "build.created" {
		log.Warn().Str("event", event.Event).Msg("unknown event type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "event ignored"})
		return
	}

	if err := s.handleUpgradeEvent(&event); err != nil {
		log.Error().Err(err).Msg("failed to handle upgrade event")
		http.Error(w, "failed to process event", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("product", event.Product).
		Str("version", event.Version).
		Str("platform", event.Platform).
		Str("arch", event.Arch).
		Msg("upgrade event processed")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// verifyWebhookSignature verifies webhook HMAC signature.
func (s *Servers) verifyWebhookSignature(body []byte, signature string) bool {
	webhookSecret := s.cfg.WebhookSecret
	if webhookSecret == "" {
		log.Warn().Msg("webhook secret not configured, rejecting request")
		return false
	}

	if signature == "" {
		return false
	}
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	expectedSig := strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	_, _ = mac.Write(body)
	actualSig := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expectedSig), []byte(actualSig))
}

// handleUpgradeEvent stores latest build metadata.
func (s *Servers) handleUpgradeEvent(event *UpgradeEvent) error {
	// Keep in-memory upgrade versions map in sync with webhook events.
	s.upgradeVersionsMu.Lock()
	defer s.upgradeVersionsMu.Unlock()

	key := event.Product + "-" + event.Platform + "-" + event.Arch
	s.upgradeVersions[key] = &UpgradeVersion{
		Product:     event.Product,
		Version:     event.Version,
		Channel:     event.Channel,
		Platform:    event.Platform,
		Arch:        event.Arch,
		DownloadURL: event.DownloadURL,
		Checksum:    event.Checksum,
		Changelog:   event.Changelog,
		UpdatedAt:   time.Now(),
	}

	log.Info().
		Str("key", key).
		Str("version", event.Version).
		Msg("upgrade version saved")

	return nil
}

// UpgradeVersion describes a latest build artifact.
type UpgradeVersion struct {
	Product     string
	Version     string
	Channel     string
	Platform    string
	Arch        string
	DownloadURL string
	Checksum    string
	Changelog   string
	UpdatedAt   time.Time
}
