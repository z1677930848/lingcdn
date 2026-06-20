package server

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed scripts/node_install_local.sh
var nodeInstallLocalScript []byte

const localBundleMaxUploadBytes = 256 << 20 // 256 MiB

var bundleFilenameRE = regexp.MustCompile(`^lingcdn-(node|control)-(.+)-linux-(amd64|arm64)\.tar\.gz$`)

type localBundleRecord struct {
	Product   string    `json:"product"`
	Version   string    `json:"version"`
	Platform  string    `json:"platform"`
	Arch      string    `json:"arch"`
	Channel   string    `json:"channel"`
	Filename  string    `json:"filename"`
	Checksum  string    `json:"checksum"`
	SizeBytes int64     `json:"size_bytes"`
	UpdatedAt time.Time `json:"updated_at"`
}

// offlineLocalBundleEnabled is true when LICENSE_MODE=offline so node installs
// can pull artifacts from the control plane instead of auth.lingcdn.cloud.
func (s *Servers) offlineLocalBundleEnabled() bool {
	return s.isOfflineLicenseMode()
}

func (s *Servers) localBundleDir() string {
	dir := strings.TrimSpace(s.cfg.LocalBundleDir)
	if dir == "" {
		dir = "data/bundles"
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		return filepath.Join(wd, dir)
	}
	return dir
}

func parseBundleFilename(name string) (product, version, platform, arch string, ok bool) {
	m := bundleFilenameRE.FindStringSubmatch(strings.TrimSpace(name))
	if len(m) != 4 {
		return "", "", "", "", false
	}
	return m[1], m[2], "linux", m[3], true
}

func compareVersion(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	pb := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	for len(pa) < 4 {
		pa = append(pa, "0")
	}
	for len(pb) < 4 {
		pb = append(pb, "0")
	}
	for i := 0; i < 4; i++ {
		var na, nb int
		fmt.Sscanf(pa[i], "%d", &na)
		fmt.Sscanf(pb[i], "%d", &nb)
		if na > nb {
			return 1
		}
		if na < nb {
			return -1
		}
	}
	return strings.Compare(a, b)
}

func (s *Servers) scanLocalBundles() ([]localBundleRecord, error) {
	dir := s.localBundleDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]localBundleRecord, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		product, version, platform, arch, ok := parseBundleFilename(name)
		if !ok {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := ent.Info()
		if err != nil {
			continue
		}
		sum, err := bundleFileChecksum(path)
		if err != nil {
			continue
		}
		out = append(out, localBundleRecord{
			Product:   product,
			Version:   version,
			Platform:  platform,
			Arch:      arch,
			Channel:   "stable",
			Filename:  name,
			Checksum:  sum,
			SizeBytes: info.Size(),
			UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Product != out[j].Product {
			return out[i].Product < out[j].Product
		}
		if out[i].Arch != out[j].Arch {
			return out[i].Arch < out[j].Arch
		}
		return compareVersion(out[i].Version, out[j].Version) > 0
	})
	return out, nil
}

func bundleFileChecksum(path string) (string, error) {
	sum, _, err := sha256File(path)
	return sum, err
}

func (s *Servers) resolveLocalBundle(product, channel, platform, arch, version string, controlBase string) (*localBundleRecord, error) {
	product = strings.TrimSpace(product)
	if product == "" {
		product = "node"
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = "linux"
	}
	arch = strings.TrimSpace(arch)
	if arch == "" {
		arch = "amd64"
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = "stable"
	}
	version = strings.TrimSpace(version)
	if version == "" {
		version = "latest"
	}

	all, err := s.scanLocalBundles()
	if err != nil {
		return nil, err
	}
	var matches []localBundleRecord
	for _, b := range all {
		if b.Product != product || b.Platform != platform || b.Arch != arch {
			continue
		}
		if channel != "" && channel != "stable" && b.Channel != channel {
			continue
		}
		if version != "latest" && b.Version != version {
			continue
		}
		matches = append(matches, b)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no local bundle for product=%s arch=%s version=%s", product, arch, version)
	}
	if version == "latest" {
		best := matches[0]
		for _, b := range matches[1:] {
			if compareVersion(b.Version, best.Version) > 0 {
				best = b
			}
		}
		matches = []localBundleRecord{best}
	}
	rec := matches[0]
	_ = controlBase
	return &rec, nil
}

func (s *Servers) handleLocalNodeInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if !s.offlineLocalBundleEnabled() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(nodeInstallLocalScript)
}

func (s *Servers) handleLocalUpgradeLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if !s.offlineLocalBundleEnabled() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	q := r.URL.Query()
	controlBase := s.resolveControlHTTPBase(r.Host)
	rec, err := s.resolveLocalBundle(
		q.Get("product"),
		q.Get("channel"),
		q.Get("platform"),
		q.Get("arch"),
		q.Get("version"),
		controlBase,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	downloadURL := strings.TrimRight(controlBase, "/") + "/api/local/bundles/" + rec.Product + "/" + rec.Filename
	writeJSON(w, http.StatusOK, map[string]any{
		"product":      rec.Product,
		"version":      rec.Version,
		"channel":      rec.Channel,
		"platform":     rec.Platform,
		"arch":         rec.Arch,
		"download_url": downloadURL,
		"checksum":     rec.Checksum,
		"size_bytes":   rec.SizeBytes,
		"source":       "local",
	})
}

func (s *Servers) handleLocalBundleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if !s.offlineLocalBundleEnabled() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/local/bundles/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
		return
	}
	product, filename := parts[0], filepath.Base(parts[1])
	if product == "" || filename == "" || filename != parts[1] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid filename"})
		return
	}
	if _, _, _, _, ok := parseBundleFilename(filename); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid bundle filename"})
		return
	}
	path := filepath.Join(s.localBundleDir(), filename)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "bundle not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "open bundle failed"})
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bundle not found"})
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeContent(w, r, filename, info.ModTime(), f)
}

func (s *Servers) handleAdminLocalBundles(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if !s.offlineLocalBundleEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "本地节点包分发仅在 LICENSE_MODE=offline 时可用"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.scanLocalBundles()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"bundles": list,
			"dir":     s.localBundleDir(),
			"enabled": true,
		})
	case http.MethodPost:
		s.handleAdminLocalBundleUpload(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
	}
}

func (s *Servers) handleAdminLocalBundleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(localBundleMaxUploadBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart form: " + err.Error()})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing file field"})
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	if _, _, _, _, ok := parseBundleFilename(filename); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "文件名必须符合 lingcdn-node-x.y.z-linux-amd64.tar.gz 格式",
		})
		return
	}

	dir := s.localBundleDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "create bundle dir failed"})
		return
	}
	dest := filepath.Join(dir, filename)
	tmp := dest + ".uploading"
	out, err := os.Create(tmp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "create temp file failed"})
		return
	}
	written, copyErr := io.Copy(out, io.LimitReader(file, localBundleMaxUploadBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "save upload failed"})
		return
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "close temp file failed"})
		return
	}
	if written > localBundleMaxUploadBytes {
		_ = os.Remove(tmp)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file too large"})
		return
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "finalize upload failed"})
		return
	}
	sum, err := bundleFileChecksum(dest)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "checksum failed"})
		return
	}
	product, version, platform, arch, _ := parseBundleFilename(filename)
	info, _ := os.Stat(dest)
	rec := localBundleRecord{
		Product: product, Version: version, Platform: platform, Arch: arch,
		Channel: "stable", Filename: filename, Checksum: sum,
	}
	if info != nil {
		rec.SizeBytes = info.Size()
		rec.UpdatedAt = info.ModTime()
	}
	writeJSON(w, http.StatusCreated, map[string]any{"bundle": rec})
}

func (s *Servers) handleAdminLocalBundleDelete(w http.ResponseWriter, r *http.Request) {
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if !s.offlineLocalBundleEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "本地节点包分发仅在 LICENSE_MODE=offline 时可用"})
		return
	}
	filename := filepath.Base(strings.TrimPrefix(r.URL.Path, "/api/admin/local-bundles/"))
	if filename == "" || filename == "." {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid filename"})
		return
	}
	if _, _, _, _, ok := parseBundleFilename(filename); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid bundle filename"})
		return
	}
	path := filepath.Join(s.localBundleDir(), filename)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "bundle not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
