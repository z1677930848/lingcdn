package server

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/config"
)

type geoIPState struct {
	SHA256    string    `json:"sha256"`
	SizeBytes int64     `json:"size_bytes"`
	UpdatedAt time.Time `json:"updated_at"`
	LastError string    `json:"last_error,omitempty"`
}

type GeoIPManager struct {
	cfg      config.Config
	mu       sync.RWMutex
	st       geoIPState
	resolver *geoIPResolver
}

// Optional fallback credentials for MaxMind. They are intentionally left
// blank in source — set the corresponding environment variables at deploy
// time if you want Refresh() to keep working when the admin-configured
// MAXMIND_LICENSE_KEY is empty. Hardcoding real credentials here will be
// blocked by GitHub push protection.
var (
	fallbackMaxMindLicenseKey = os.Getenv("LINGCDN_MAXMIND_FALLBACK_LICENSE_KEY")
	fallbackMaxMindAccountID  = os.Getenv("LINGCDN_MAXMIND_FALLBACK_ACCOUNT_ID")
)

func NewGeoIPManager(cfg config.Config) *GeoIPManager {
	return &GeoIPManager{
		cfg: cfg,
		st:  geoIPState{},
	}
}

func (m *GeoIPManager) targetPath() string {
	dir := strings.TrimSpace(m.cfg.GeoIPStorageDir)
	if dir == "" {
		dir = "data/geoip"
	}
	edition := strings.TrimSpace(m.cfg.GeoIPEdition)
	if edition == "" {
		edition = "GeoLite2-City"
	}
	return filepath.Join(dir, edition+".mmdb")
}

func (m *GeoIPManager) loadExisting() {
	path := m.targetPath()
	fi, err := os.Stat(path)
	if err != nil || fi == nil || fi.IsDir() {
		return
	}
	sha, size, err := sha256File(path)
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.mu.Lock()
	m.st.SHA256 = sha
	m.st.SizeBytes = size
	if m.st.UpdatedAt.IsZero() {
		m.st.UpdatedAt = fi.ModTime()
	}
	m.mu.Unlock()
}

func (m *GeoIPManager) Start(ctx context.Context) {
	m.loadExisting()
	go func() {
		_ = m.Refresh(ctx)
	}()

	interval := m.cfg.GeoIPUpdateInterval
	if interval <= 0 {
		interval = 168 * time.Hour
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.Refresh(ctx)
			}
		}
	}()
}

func (m *GeoIPManager) Snapshot() geoIPState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st
}

// Lookup 使用当前的 GeoIP 数据库解析 IP 地理位置。
func (m *GeoIPManager) Lookup(ip string) *GeoLocation {
	if m == nil {
		return nil
	}
	resolver := m.ensureResolver()
	if resolver == nil {
		return nil
	}
	return resolver.Lookup(ip)
}

func (m *GeoIPManager) ensureResolver() *geoIPResolver {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.resolver == nil {
		m.resolver = newGeoIPResolver(m.targetPath())
	} else if strings.TrimSpace(m.resolver.path) == "" {
		m.resolver.path = m.targetPath()
	}
	return m.resolver
}

func (m *GeoIPManager) setError(msg string) {
	m.mu.Lock()
	m.st.LastError = msg
	m.mu.Unlock()
}

func (m *GeoIPManager) setState(sha string, size int64, updatedAt time.Time) {
	m.mu.Lock()
	m.st.SHA256 = sha
	m.st.SizeBytes = size
	m.st.UpdatedAt = updatedAt
	m.st.LastError = ""
	m.mu.Unlock()
}

func (m *GeoIPManager) Refresh(ctx context.Context) error {
	licenseKey := strings.TrimSpace(m.cfg.MaxMindLicenseKey)
	if licenseKey == "" {
		licenseKey = fallbackMaxMindLicenseKey
	}
	if licenseKey == "" {
		return errors.New("MAXMIND_LICENSE_KEY is empty")
	}

	edition := strings.TrimSpace(m.cfg.GeoIPEdition)
	if edition == "" {
		edition = "GeoLite2-City"
	}

	dir := filepath.Dir(m.targetPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.setError(err.Error())
		return err
	}

	dlURL := fmt.Sprintf("https://download.maxmind.com/geoip/databases/%s/download?suffix=tar.gz", url.PathEscape(edition))
	accountID := fallbackMaxMindAccountID
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		m.setError(err.Error())
		return err
	}
	req.SetBasicAuth(accountID, licenseKey)
	req.Header.Set("User-Agent", "lingcdn-control/geoip")
	resp, err := client.Do(req)
	if err != nil {
		m.setError(err.Error())
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("maxmind http %d", resp.StatusCode)
		m.setError(err.Error())
		return err
	}

	expectedName := edition + ".mmdb"
	tmpFile, err := os.CreateTemp(dir, expectedName+".*.new")
	if err != nil {
		m.setError(err.Error())
		return err
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	cleanupTmp := func() { _ = os.Remove(tmpPath) }

	if err := extractMMDB(resp.Body, expectedName, tmpPath); err != nil {
		cleanupTmp()
		m.setError(err.Error())
		return err
	}

	sha, size, err := sha256File(tmpPath)
	if err != nil {
		cleanupTmp()
		m.setError(err.Error())
		return err
	}

	current := m.Snapshot().SHA256
	if current == "" {
		m.loadExisting()
		current = m.Snapshot().SHA256
	}
	if current != "" && strings.EqualFold(current, sha) {
		cleanupTmp()
		log.Info().Msg("geoip no update")
		return nil
	}

	target := m.targetPath()
	oldPath := target + ".old"
	_ = os.Remove(oldPath)
	if _, err := os.Stat(target); err == nil {
		_ = os.Rename(target, oldPath)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		cleanupTmp()
		m.setError(err.Error())
		return err
	}

	m.setState(sha, size, time.Now())
	m.mu.Lock()
	if m.resolver != nil {
		m.resolver.Close()
	}
	m.mu.Unlock()
	log.Info().Str("sha256", sha).Int64("bytes", size).Msg("geoip updated")
	return nil
}

func extractMMDB(src io.Reader, expectedName string, outPath string) error {
	gz, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.FileInfo() == nil || h.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(h.Name)
		if base != expectedName {
			continue
		}

		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, cpErr := io.Copy(f, tr)
		closeErr := f.Close()
		if cpErr != nil {
			return cpErr
		}
		if closeErr != nil {
			return closeErr
		}
		return nil
	}
	return fmt.Errorf("mmdb file not found in archive: %s", expectedName)
}

func sha256File(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	var size int64
	buf := make([]byte, 1024*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			size += int64(n)
			_, _ = h.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", size, err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
