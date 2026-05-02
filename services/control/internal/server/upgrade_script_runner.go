package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func downloadScriptToTemp(ctx context.Context, scriptURL, pattern string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("download script failed: status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	// Limit script download to 10MB to prevent abuse
	const maxScriptSize = 10 * 1024 * 1024
	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, maxScriptSize)); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func (s *Servers) runUpgradeScript(ctx context.Context, taskID, scriptPath string, args []string) error {
	cmd := exec.CommandContext(ctx, scriptPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	readLines := func(level string, r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			msg := strings.TrimSpace(scanner.Text())
			if msg == "" {
				continue
			}
			s.appendUpgradeLog(ctx, taskID, level, msg, "")
		}
	}

	wg.Add(2)
	go readLines("INFO", stdout)
	go readLines("ERROR", stderr)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
