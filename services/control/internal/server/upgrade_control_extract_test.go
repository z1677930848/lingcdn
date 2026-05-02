package server

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"archive/zip"
	"bytes"
)

func TestResolveControlBinaryFromZipAndTemplateExtraction(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "control.zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipFile(t, zw, "bin/lingcdn-control", []byte("bin-ok"))
	writeZipFile(t, zw, "ui-template/index.html", []byte("<html>ok</html>"))
	writeZipFile(t, zw, "ui-template/assets/app.js", []byte("console.log('ok')"))
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	binPath, err := resolveControlBinaryFromArtifact(zipPath, "/lingcdn/control/bin/lingcdn-control")
	if err != nil {
		t.Fatalf("resolve binary: %v", err)
	}
	defer os.Remove(binPath)
	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read bin: %v", err)
	}
	if string(got) != "bin-ok" {
		t.Fatalf("bin content mismatch: %q", string(got))
	}

	dst := filepath.Join(dir, "ui-out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	n, err := extractTemplateDirTo(zipPath, "ui-template/", dst)
	if err != nil {
		t.Fatalf("extract template: %v", err)
	}
	if n != 2 {
		t.Fatalf("extracted=%d", n)
	}
	if _, err := os.Stat(filepath.Join(dst, "index.html")); err != nil {
		t.Fatalf("missing index.html: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "assets", "app.js")); err != nil {
		t.Fatalf("missing assets/app.js: %v", err)
	}
}

func TestTemplateTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bad.zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipFile(t, zw, "ui-template/../../evil.txt", []byte("evil"))
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	dst := filepath.Join(dir, "out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := extractTemplateDirTo(zipPath, "ui-template/", dst)
	if err == nil {
		t.Fatalf("expected error for traversal")
	}
}

func TestResolveControlBinaryFromTarGz(t *testing.T) {
	dir := t.TempDir()
	tgzPath := filepath.Join(dir, "control.tar.gz")

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTarFile(t, tw, "bin/lingcdn-control", []byte("bin-ok"))
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gz: %v", err)
	}
	if err := os.WriteFile(tgzPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tgz: %v", err)
	}

	binPath, err := resolveControlBinaryFromArtifact(tgzPath, "/lingcdn/control/bin/lingcdn-control")
	if err != nil {
		t.Fatalf("resolve binary: %v", err)
	}
	defer os.Remove(binPath)
	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read bin: %v", err)
	}
	if string(got) != "bin-ok" {
		t.Fatalf("bin content mismatch: %q", string(got))
	}
}

func TestResolveControlBinaryFromZipWithoutSuffix(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "control")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipFile(t, zw, "bin/lingcdn-control", []byte("bin-ok"))
	writeZipFile(t, zw, "ui/index.html", []byte("<html>ok</html>"))
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	binPath, err := resolveControlBinaryFromArtifact(zipPath, "/lingcdn/control/bin/lingcdn-control")
	if err != nil {
		t.Fatalf("resolve binary: %v", err)
	}
	defer os.Remove(binPath)

	dst := filepath.Join(dir, "ui-out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	n, err := extractTemplateDirTo(zipPath, "ui/", dst)
	if err != nil {
		t.Fatalf("extract template: %v", err)
	}
	if n != 1 {
		t.Fatalf("extracted=%d", n)
	}
	if _, err := os.Stat(filepath.Join(dst, "index.html")); err != nil {
		t.Fatalf("missing index.html: %v", err)
	}
}

func writeZipFile(t *testing.T, zw *zip.Writer, name string, content []byte) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create %s: %v", name, err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("zip write %s: %v", name, err)
	}
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, content []byte) {
	t.Helper()
	h := &tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatalf("tar header %s: %v", name, err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write %s: %v", name, err)
	}
}
