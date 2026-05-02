package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var errUnsupportedArtifact = errors.New("unsupported artifact")

func resolveControlBinaryFromArtifact(artifactPath, binaryPath string) (string, error) {
	lower := strings.ToLower(strings.TrimSpace(artifactPath))
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return resolveBinaryFromZip(artifactPath, binaryPath)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return resolveBinaryFromTarGz(artifactPath, binaryPath)
	default:
		kind, err := sniffArtifactKind(artifactPath)
		if err != nil {
			return "", err
		}
		switch kind {
		case "zip":
			return resolveBinaryFromZip(artifactPath, binaryPath)
		case "tgz":
			return resolveBinaryFromTarGz(artifactPath, binaryPath)
		default:
			return "", errUnsupportedArtifact
		}
	}
}

func extractTemplateDirTo(artifactPath, prefix, dst string) (int, error) {
	lower := strings.ToLower(strings.TrimSpace(artifactPath))
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractTemplateFromZip(artifactPath, prefix, dst)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractTemplateFromTarGz(artifactPath, prefix, dst)
	default:
		kind, err := sniffArtifactKind(artifactPath)
		if err != nil {
			return 0, err
		}
		switch kind {
		case "zip":
			return extractTemplateFromZip(artifactPath, prefix, dst)
		case "tgz":
			return extractTemplateFromTarGz(artifactPath, prefix, dst)
		default:
			return 0, errUnsupportedArtifact
		}
	}
}

func sniffArtifactKind(artifactPath string) (string, error) {
	f, err := os.Open(artifactPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var hdr [4]byte
	n, _ := io.ReadFull(f, hdr[:])
	if n >= 2 && hdr[0] == 'P' && hdr[1] == 'K' {
		return "zip", nil
	}
	if n >= 2 && hdr[0] == 0x1f && hdr[1] == 0x8b {
		return "tgz", nil
	}
	return "", errUnsupportedArtifact
}

func resolveBinaryFromZip(artifactPath, binaryPath string) (string, error) {
	zr, err := zip.OpenReader(artifactPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	candidates := buildBinaryCandidates(binaryPath)
	for _, f := range zr.File {
		name := normalizeArchivePath(f.Name)
		if !binaryMatches(name, candidates) {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		return writeTempFile(rc)
	}
	return "", fmt.Errorf("binary not found in artifact")
}

func resolveBinaryFromTarGz(artifactPath, binaryPath string) (string, error) {
	f, err := os.Open(artifactPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	candidates := buildBinaryCandidates(binaryPath)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name := normalizeArchivePath(h.Name)
		if !binaryMatches(name, candidates) {
			continue
		}
		return writeTempFile(tr)
	}
	return "", fmt.Errorf("binary not found in artifact")
}

func extractTemplateFromZip(artifactPath, prefix, dst string) (int, error) {
	zr, err := zip.OpenReader(artifactPath)
	if err != nil {
		return 0, err
	}
	defer zr.Close()

	normalizedPrefix := normalizePrefix(prefix)
	count := 0
	for _, f := range zr.File {
		name := normalizeArchivePathRaw(f.Name)
		if !strings.HasPrefix(name, normalizedPrefix) {
			continue
		}
		rel := strings.TrimPrefix(name, normalizedPrefix)
		if rel == "" {
			continue
		}
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if _, err := safeJoin(dst, rel); err != nil {
				return 0, err
			}
			continue
		}
		target, err := safeJoin(dst, rel)
		if err != nil {
			return 0, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		if err := writeFile(target, rc, f.Mode()); err != nil {
			rc.Close()
			return 0, err
		}
		rc.Close()
		count++
	}
	return count, nil
}

func extractTemplateFromTarGz(artifactPath, prefix, dst string) (int, error) {
	f, err := os.Open(artifactPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	normalizedPrefix := normalizePrefix(prefix)
	count := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}
		name := normalizeArchivePathRaw(h.Name)
		if !strings.HasPrefix(name, normalizedPrefix) {
			continue
		}
		rel := strings.TrimPrefix(name, normalizedPrefix)
		if rel == "" {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if _, err := safeJoin(dst, rel); err != nil {
				return 0, err
			}
			continue
		case tar.TypeReg:
			target, err := safeJoin(dst, rel)
			if err != nil {
				return 0, err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return 0, err
			}
			if err := writeFile(target, tr, os.FileMode(h.Mode)); err != nil {
				return 0, err
			}
			count++
		default:
			continue
		}
	}
	return count, nil
}

// extractUIFromArtifact finds the `ui/` sub-tree inside the release archive
// and writes its files into `dstDir`. The archive's top-level directory
// name (e.g. `lingcdn-control-1.0.9/`) is auto-detected, so callers don't
// need to know the release version.
//
// Returns the number of files written. 0 is a valid, non-error return when
// the artifact intentionally ships no UI bundle.
func extractUIFromArtifact(artifactPath, dstDir string) (int, error) {
	kind, err := artifactKind(artifactPath)
	if err != nil {
		return 0, err
	}
	topDir, err := detectArtifactTopDir(artifactPath, kind)
	if err != nil {
		return 0, err
	}
	// Prefix the caller asks extractTemplateDirTo to peel. Either
	// "<top>/ui/" (normal release layout) or bare "ui/" for archives
	// without a wrapping directory.
	prefix := "ui/"
	if topDir != "" {
		prefix = topDir + "/ui/"
	}
	return extractTemplateDirTo(artifactPath, prefix, dstDir)
}

// artifactKind classifies the archive by extension first, then magic bytes.
func artifactKind(artifactPath string) (string, error) {
	lower := strings.ToLower(strings.TrimSpace(artifactPath))
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip", nil
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tgz", nil
	}
	return sniffArtifactKind(artifactPath)
}

// detectArtifactTopDir returns the single common top-level directory of
// the archive, or "" if entries are flat at the root.
func detectArtifactTopDir(artifactPath, kind string) (string, error) {
	names, err := listArtifactNames(artifactPath, kind)
	if err != nil {
		return "", err
	}
	var top string
	for _, raw := range names {
		name := normalizeArchivePathRaw(raw)
		if name == "" {
			continue
		}
		idx := strings.Index(name, "/")
		if idx < 0 {
			// A bare file at the root — no single top dir.
			return "", nil
		}
		head := name[:idx]
		switch top {
		case "":
			top = head
		case head:
			// same top
		default:
			return "", nil
		}
	}
	return top, nil
}

func listArtifactNames(artifactPath, kind string) ([]string, error) {
	switch kind {
	case "zip":
		zr, err := zip.OpenReader(artifactPath)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		names := make([]string, 0, len(zr.File))
		for _, f := range zr.File {
			names = append(names, f.Name)
		}
		return names, nil
	case "tgz":
		f, err := os.Open(artifactPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		gr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		tr := tar.NewReader(gr)
		var names []string
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			names = append(names, h.Name)
		}
		return names, nil
	default:
		return nil, errUnsupportedArtifact
	}
}

func buildBinaryCandidates(binaryPath string) []string {
	target := normalizeArchivePath(strings.TrimPrefix(binaryPath, "/"))
	candidates := []string{target}
	if strings.HasPrefix(target, "lingcdn/control/") {
		candidates = append(candidates, strings.TrimPrefix(target, "lingcdn/control/"))
	}
	if strings.HasPrefix(target, "control/") {
		candidates = append(candidates, strings.TrimPrefix(target, "control/"))
	}
	base := path.Base(target)
	if base != target {
		candidates = append(candidates, base)
	}
	return candidates
}

func binaryMatches(name string, candidates []string) bool {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if name == c || strings.HasSuffix(name, "/"+c) {
			return true
		}
	}
	return false
}

func normalizeArchivePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return path.Clean(p)
}

func normalizeArchivePathRaw(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.TrimPrefix(p, "./")
}

func normalizePrefix(prefix string) string {
	normalized := normalizeArchivePath(prefix)
	if normalized == "." {
		normalized = ""
	}
	if normalized != "" && !strings.HasSuffix(normalized, "/") {
		normalized += "/"
	}
	return normalized
}

func safeJoin(base, rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == "." || rel == "" {
		return base, nil
	}
	target := filepath.Join(base, rel)
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	relToBase, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path traversal: %s", rel)
	}
	return targetAbs, nil
}

func writeTempFile(r io.Reader) (string, error) {
	tmp, err := os.CreateTemp("", "lingcdn-control-bin-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, r); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

func writeFile(path string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
