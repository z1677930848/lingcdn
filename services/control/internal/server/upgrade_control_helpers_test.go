package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferUpgradeArtifactSuffix(t *testing.T) {
	cases := []struct {
		u    string
		want string
	}{
		{"https://example.com/a/control.zip", ".zip"},
		{"https://example.com/a/control.zip?tk=1", ".zip"},
		{"https://example.com/a/control.tgz", ".tgz"},
		{"https://example.com/a/control.tar.gz", ".tar.gz"},
		{"https://example.com/a/control.tar.gz?tk=1", ".tar.gz"},
		{"https://example.com/a/control", ""},
	}
	for _, c := range cases {
		if got := inferUpgradeArtifactSuffix(c.u); got != c.want {
			t.Fatalf("url=%q got=%q want=%q", c.u, got, c.want)
		}
	}
}

func TestStageFileToDir(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(srcPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	outDir := t.TempDir()
	staged, err := stageFileToDir(srcPath, outDir, ".lingcdn-test-*")
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(staged) })

	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("read staged: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("unexpected content: %q", string(got))
	}
}
