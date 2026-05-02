package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestExtractUIFromArtifact verifies the artifact extraction helper can
// find and write the bundled UI tree regardless of whether the archive
// wraps it in a versioned top-level directory (the normal release
// layout) or places files flat at the root.
func TestExtractUIFromArtifact(t *testing.T) {
	t.Run("versioned top dir", func(t *testing.T) {
		dir := t.TempDir()
		tarPath := filepath.Join(dir, "release.tar.gz")
		writeTarGz(t, tarPath, map[string]string{
			"lingcdn-control-9.9.9/lingcdn-control":   "binary",
			"lingcdn-control-9.9.9/ui/index.html":     "<html/>",
			"lingcdn-control-9.9.9/ui/assets/app.js":  "console.log(1)",
		})

		dst := filepath.Join(dir, "ui-out")
		n, err := extractUIFromArtifact(tarPath, dst)
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if n != 2 {
			t.Fatalf("expected 2 files, got %d", n)
		}
		if got := mustRead(t, filepath.Join(dst, "index.html")); got != "<html/>" {
			t.Fatalf("index.html: %q", got)
		}
		if got := mustRead(t, filepath.Join(dst, "assets", "app.js")); got != "console.log(1)" {
			t.Fatalf("assets/app.js: %q", got)
		}
	})

	t.Run("flat artifact", func(t *testing.T) {
		dir := t.TempDir()
		tarPath := filepath.Join(dir, "release-flat.tar.gz")
		writeTarGz(t, tarPath, map[string]string{
			"lingcdn-control":  "binary",
			"ui/index.html":    "<html/>",
			"ui/assets/app.js": "console.log(1)",
		})

		dst := filepath.Join(dir, "ui-out-flat")
		n, err := extractUIFromArtifact(tarPath, dst)
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if n != 2 {
			t.Fatalf("expected 2 files, got %d", n)
		}
	})

	t.Run("no ui bundled", func(t *testing.T) {
		dir := t.TempDir()
		tarPath := filepath.Join(dir, "release-nobui.tar.gz")
		writeTarGz(t, tarPath, map[string]string{
			"lingcdn-control-1.0.0/lingcdn-control": "binary",
			"lingcdn-control-1.0.0/README.md":       "hello",
		})

		dst := filepath.Join(dir, "ui-nothing")
		n, err := extractUIFromArtifact(tarPath, dst)
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 files (no ui bundle), got %d", n)
		}
	})
}

// TestCheckWritable exercises the install-feasibility probe. It must:
//   1) return writeStrategyRename when the parent dir is writable;
//   2) return writeStrategyInPlace when only the file itself is writable
//      (parent dir has been made read-only);
//   3) return writeStrategyNone when neither applies.
func TestCheckWritable(t *testing.T) {
	t.Run("parent dir writable -> rename", func(t *testing.T) {
		dir := t.TempDir()
		fp := filepath.Join(dir, "target")
		if err := os.WriteFile(fp, []byte("x"), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		strategy, err := checkWritable(fp)
		if err != nil {
			t.Fatalf("expected writable, got %v", err)
		}
		if strategy != writeStrategyRename {
			t.Fatalf("expected writeStrategyRename, got %v", strategy)
		}
	})

	t.Run("only file writable -> in-place", func(t *testing.T) {
		// Skip on non-POSIX where we can't meaningfully restrict dir perms
		// without admin, and on CI running as root which sees through mode
		// bits. Windows dir mode bits don't restrict CreateTemp the same
		// way POSIX does, so the branch is only reachable on linux/darwin.
		if runtime.GOOS == "windows" {
			t.Skip("dir mode bits do not gate CreateTemp on windows; in-place branch untestable")
		}
		if os.Geteuid() == 0 {
			t.Skip("root ignores mode bits; in-place branch untestable as root")
		}
		dir := t.TempDir()
		fp := filepath.Join(dir, "target")
		if err := os.WriteFile(fp, []byte("x"), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// Make the dir non-writable for the current user; the file remains
		// user-writable because it was created before the chmod.
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("chmod dir: %v", err)
		}
		// Restore permissions so t.TempDir cleanup succeeds.
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

		strategy, err := checkWritable(fp)
		if err != nil {
			t.Fatalf("expected in-place fallback, got err %v", err)
		}
		if strategy != writeStrategyInPlace {
			t.Fatalf("expected writeStrategyInPlace, got %v", strategy)
		}
	})
}

// --- helpers ---

func writeTarGz(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range entries {
		hdr := &tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0o644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tar: %v", err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
