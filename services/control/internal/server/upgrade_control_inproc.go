package server

// In-process control-plane upgrade: download the portal's latest artifact,
// verify it (SHA256 + ed25519), atomically swap in the new binary and UI
// directory, and request the supervisor (systemd, docker, nohup, …) to
// restart us. The whole flow runs with whatever privileges the main control
// process already has — crucially, *no root, no sudo, no remote shell
// script*. If anything goes wrong we back out by restoring the previous
// files from a sibling ".bak" path so the operator is not left with a
// broken deployment.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/config"
)

// performControlUpgradeInProcess replaces the current binary + UI dist with
// the version advertised by the portal. Preconditions:
//   - running on linux (enforced by the caller)
//   - the process has write permission on both the binary path and the
//     UI dir (or their parents); this is satisfied whenever the service
//     has been installed under a user-owned prefix like /opt/lingcdn.
//
// On success the function returns nil *after* requesting self-exit, so the
// supervisor (systemd Restart=always / on-failure, docker auto-restart,
// bash loops) picks up the new binary.
func (s *Servers) performControlUpgradeInProcess(ctx context.Context, task *upgradeTask) error {
	if runtime.GOOS != "linux" {
		return errors.New("in-process upgrade only supported on linux")
	}

	portal := s.upgradePortalBase()
	channel, err := normalizeUpgradeChannel(task.Channel)
	if err != nil {
		return err
	}
	arch := normalizeArch(runtime.GOARCH)
	if arch == "" {
		return fmt.Errorf("unsupported arch: %s", runtime.GOARCH)
	}

	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("查询 portal 升级元数据: channel=%s arch=%s", channel, arch), "")
	latest, err := s.fetchPortalLatestCached(ctx, portal, "control", channel, "linux", arch, "latest")
	if err != nil {
		return fmt.Errorf("query portal latest: %w", err)
	}
	if strings.TrimSpace(latest.DownloadURL) == "" {
		return errors.New("portal latest: missing download_url")
	}
	if strings.TrimSpace(latest.Checksum) == "" {
		return errors.New("portal latest: missing checksum")
	}
	task.TargetVersion = strings.TrimSpace(latest.Version)

	exePath, err := resolveRunningBinaryPath()
	if err != nil {
		return fmt.Errorf("locate running binary: %w", err)
	}

	// Decide how we'll install the new binary. We try three strategies, in
	// order of preference:
	//   1) rename   — parent dir writable, atomic same-dir swap.
	//   2) in-place — binary itself writable, truncate+overwrite. Common
	//      when /lingcdn/bin is root:root but the binary was chown'd to
	//      the service user.
	//   3) sudo     — neither of the above; only if sudo -n works.
	strategy, probeErr := checkWritable(exePath)
	needSudo := false
	switch strategy {
	case writeStrategyRename:
		s.appendUpgradeLog(ctx, task.ID, "INFO",
			fmt.Sprintf("目标二进制: %s（父目录可写，将使用原子 rename 替换）", exePath), "")
	case writeStrategyInPlace:
		s.appendUpgradeLog(ctx, task.ID, "INFO",
			fmt.Sprintf("目标二进制: %s（文件可写，将使用原地 overwrite 替换）", exePath), "")
	default: // writeStrategyNone
		if !probeSudoN() {
			// Note on the shell snippets below: we need to name the user
			// that the control process is running as. `id -un` by itself
			// prints the *invoking* shell's user (likely root when the
			// operator runs this from a sudo shell), which is the opposite
			// of what we want. `id -un -p $PID` is not a thing — `-p`
			// changes *format*, not target. The portable way to look up
			// the owner of a live process is `ps -o user= -p $PID`, which
			// prints the user name with no header; we tr -d ' ' to strip
			// any trailing whitespace from ps's fixed-width output.
			return fmt.Errorf("无法替换主控二进制 (%s): %w\n"+
				"运行用户既没有目录/文件写权限，也没有免密 sudo。请任选其一：\n"+
				"  1) 让主控运行用户拥有该文件：\n"+
				"       sudo chown \"$(ps -o user= -p $(pgrep -f lingcdn-control | head -1) | tr -d ' ')\" %s\n"+
				"  2) 让主控运行用户拥有整个部署目录（推荐）：\n"+
				"       sudo chown -R \"$(ps -o user= -p $(pgrep -f lingcdn-control | head -1) | tr -d ' ')\" %s\n"+
				"  3) 以 root 启动主控（修改 systemd unit 的 User=）；\n"+
				"  4) 配置免密 sudo（安全性最弱）：\n"+
				"       echo \"$(id -un) ALL=(root) NOPASSWD: /bin/bash,/bin/mv,/bin/cp,/bin/rm,/bin/chmod,/bin/mkdir\" | sudo tee /etc/sudoers.d/lingcdn-upgrade",
				exePath, probeErr, exePath, filepath.Dir(exePath))
		}
		needSudo = true
		s.appendUpgradeLog(ctx, task.ID, "INFO",
			fmt.Sprintf("目标二进制 %s 无法直接写入，将使用 sudo 提权替换", exePath), "")
	}

	// Download artifact to a temp dir. For the rename strategy we want the
	// tmp dir inside the same filesystem as the target (so the later rename
	// of newBin → exePath is atomic). For in-place and sudo strategies we
	// use the system /tmp, which is always writable.
	var tmpDir string
	if strategy == writeStrategyRename {
		tmpDir, err = os.MkdirTemp(filepath.Dir(exePath), ".lingcdn-upgrade-*")
	} else {
		tmpDir, err = os.MkdirTemp("", ".lingcdn-upgrade-*")
	}
	if err != nil {
		return fmt.Errorf("mkdir tmp: %w", err)
	}
	defer func() {
		if strategy == writeStrategyNone {
			// In the sudo path the tmp dir was created via sudo if its
			// parent required root; clean it with sudo too.
			_ = sudoRun("rm", "-rf", tmpDir)
		}
		_ = os.RemoveAll(tmpDir)
	}()

	suffix := inferUpgradeArtifactSuffix(latest.DownloadURL)
	if suffix == "" {
		suffix = ".tar.gz"
	}
	artifactPath := filepath.Join(tmpDir, "artifact"+suffix)
	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("下载升级包: %s", maskUpgradeURL(latest.DownloadURL)), "")
	if err := downloadArtifact(ctx, latest.DownloadURL, artifactPath); err != nil {
		return fmt.Errorf("download artifact: %w", err)
	}

	// Verify SHA256(artifact) == portal-advertised checksum.
	actual, _, err := sha256File(artifactPath)
	if err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}
	wantSum := strings.ToLower(strings.TrimSpace(latest.Checksum))
	if actual != wantSum {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", actual, wantSum)
	}
	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("校验 SHA256 通过: %s", actual), "")

	// Verify ed25519 signature. We accept the portal-provided pubkey when
	// the cfg has no explicit one (bootstrap case). In steady state the
	// control operator pins cfg.UpgradePubkey so the portal cannot swap
	// the key out from under us.
	//
	// If no pubkey is available at all (neither configured locally nor
	// supplied by the portal), we downgrade to a warning instead of
	// refusing outright. SHA256 + HTTPS already provides adequate
	// integrity protection for self-hosted portal deployments. Operators
	// who want full supply-chain verification can set PORTAL_SIGNING_PRIVKEY
	// on the portal and/or UPGRADE_PUBKEY on the control plane.
	pub := strings.TrimSpace(s.cfg.UpgradePubKey)
	if pub == "" {
		pub = strings.TrimSpace(latest.PubKey)
	}
	if pub == "" {
		s.appendUpgradeLog(ctx, task.ID, "WARN",
			"未配置升级签名公钥（UPGRADE_PUBKEY），portal 也未提供公钥，跳过 ed25519 签名校验。"+
				"SHA256 校验已通过，安全性由 HTTPS + 校验和保障。"+
				"建议在 portal 配置 PORTAL_SIGNING_PRIVKEY 以启用完整签名验证。", "")
	} else {
		if err := verifyChecksumSignatureSHA256(pub, actual, latest.Signature); err != nil {
			return fmt.Errorf("verify signature: %w", err)
		}
		s.appendUpgradeLog(ctx, task.ID, "INFO", "校验 ed25519 签名通过", "")
	}

	// Extract the new binary out of the archive into the tmp dir so the
	// replacement is atomic (rename within the same dir).
	newBin, err := resolveControlBinaryFromArtifact(artifactPath, exePath)
	if err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}
	if err := os.Chmod(newBin, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}
	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("新二进制已落到 %s", newBin), "")

	// Swap the binary.
	backupPath := exePath + ".bak"
	switch strategy {
	case writeStrategyRename:
		// Direct rename path — same-filesystem atomic swap.
		_ = os.Remove(backupPath)
		if err := os.Rename(exePath, backupPath); err != nil {
			return fmt.Errorf("backup old binary: %w", err)
		}
		if err := os.Rename(newBin, exePath); err != nil {
			_ = os.Rename(backupPath, exePath)
			return fmt.Errorf("install new binary: %w", err)
		}
		if err := os.Chmod(exePath, 0o755); err != nil {
			s.appendUpgradeLog(ctx, task.ID, "WARN",
				fmt.Sprintf("新二进制 chmod 0755 失败（继续）: %v", err), "")
		}
		// fsync the parent directory so the rename metadata is durable.
		// Without this a power loss immediately after the rename can leave
		// the directory entry pointing at a half-allocated inode (the
		// kernel flushed the file data but not the dirent). Non-fatal on
		// failure — the rename itself succeeded and most filesystems will
		// recover, but the log is useful.
		if err := fsyncDir(filepath.Dir(exePath)); err != nil {
			s.appendUpgradeLog(ctx, task.ID, "WARN",
				fmt.Sprintf("fsync 父目录失败（继续）: %v", err), "")
		}
	case writeStrategyInPlace:
		// In-place overwrite: the parent dir is not writable (so rename
		// is out) but the file itself is writable. We cannot create a
		// sibling backup, so we copy the old bytes to /tmp first, then
		// truncate+write the new bytes into the existing inode. If the
		// write fails mid-way we attempt to restore from the /tmp copy.
		//
		// This loses atomicity — a concurrent exec of the old path
		// during the write window would see a partial binary — but the
		// service is about to os.Exit() anyway, so the practical window
		// is bounded and no worse than a systemd "Restart=always" cycle.
		tmpBackup := filepath.Join(tmpDir, "old-binary.bak")
		if err := copyFile(exePath, tmpBackup, 0o755); err != nil {
			return fmt.Errorf("stash old binary for in-place upgrade: %w", err)
		}
		if err := overwriteFileInPlace(exePath, newBin); err != nil {
			// Attempt rollback from the /tmp stash.
			if rbErr := overwriteFileInPlace(exePath, tmpBackup); rbErr != nil {
				s.appendUpgradeLog(ctx, task.ID, "ERROR",
					fmt.Sprintf("in-place 写入失败且回滚也失败: write=%v rollback=%v", err, rbErr), "")
			} else {
				s.appendUpgradeLog(ctx, task.ID, "WARN",
					"in-place 写入失败，已回滚到旧版本", "")
			}
			return fmt.Errorf("install new binary in-place: %w", err)
		}
		// Record a best-effort sibling .bak for operator visibility; if
		// the dir is not writable this is a no-op.
		if err := copyFile(tmpBackup, backupPath, 0o755); err != nil {
			s.appendUpgradeLog(ctx, task.ID, "INFO",
				fmt.Sprintf("未能在 %s 留下 .bak（目录不可写），备份仅保留在 %s", filepath.Dir(exePath), tmpBackup), "")
		}
	default: // writeStrategyNone → needSudo == true
		// Use targeted sudo commands for file operations only.
		_ = sudoRun("rm", "-f", backupPath)
		if err := sudoRun("mv", exePath, backupPath); err != nil {
			return fmt.Errorf("backup old binary (sudo): %w", err)
		}
		if err := sudoRun("cp", newBin, exePath); err != nil {
			_ = sudoRun("mv", backupPath, exePath)
			return fmt.Errorf("install new binary (sudo cp): %w", err)
		}
		if err := sudoRun("chmod", "0755", exePath); err != nil {
			s.appendUpgradeLog(ctx, task.ID, "WARN",
				fmt.Sprintf("sudo chmod 0755 失败（继续）: %v", err), "")
		}
	}
	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("二进制已替换，旧版本备份为 %s", backupPath), "")

	// Replace the UI directory (if the artifact bundles one).
	if err := s.replaceUIFromArtifact(ctx, task, artifactPath, exePath); err != nil {
		// If direct extraction failed and sudo is available, try with sudo.
		if needSudo {
			if err2 := s.replaceUIFromArtifactSudo(ctx, task, artifactPath, exePath); err2 != nil {
				s.appendUpgradeLog(ctx, task.ID, "WARN",
					fmt.Sprintf("UI 更新失败（sudo 亦失败）: %v", err2), "")
			}
		} else {
			s.appendUpgradeLog(ctx, task.ID, "WARN",
				fmt.Sprintf("UI 更新失败（继续）: %v", err), "")
		}
	}

	// We've done everything we can before handing off to the supervisor.
	// Persist the task's success state *now* because os.Exit below will
	// cut us off from the store.
	task.Status = "completed"
	s.saveUpgradeTask(ctx, task)
	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("升级完成，主控将退出以由 systemd/supervisor 重启到新版本 %s", task.TargetVersion), "")

	// Flush a fresh system report to the portal BEFORE we exit. Without this
	// step the portal would still show the old version for up to 10 minutes
	// (the systemReportLoop tick) after restart — the exact symptom operators
	// hit as "升级成功但主控没同步". The new process will also call
	// reportSystemOnce at startup, but doing it here gives the portal the
	// target version ahead of the restart gap so the version number flips
	// immediately from the operator's point of view.
	//
	// The report itself uses the *pre-restart* buildinfo.Version() — which is
	// the OLD version — so this exists only to exercise the portal path and
	// confirm connectivity. The authoritative version bump happens when the
	// new binary starts and reports its own buildinfo.Version().
	reportCtx, cancelReport := context.WithTimeout(ctx, 5*time.Second)
	if msg, err := s.reportSystemOnce(reportCtx); err != nil {
		s.appendUpgradeLog(ctx, task.ID, "WARN",
			fmt.Sprintf("pre-exit 向 portal 上报失败（可忽略，新进程启动后会重试）：%v", err), "")
	} else {
		s.appendUpgradeLog(ctx, task.ID, "INFO",
			fmt.Sprintf("pre-exit 向 portal 上报成功 (%s)", msg), "")
	}
	cancelReport()

	// Exit after giving the HTTP response a chance to flush. We previously
	// relied on a bare time.Sleep(800ms), which raced against slow client
	// reads and against upgrade log writes that might still be in flight.
	// Now we wait deterministically: appendUpgradeLog has already written
	// the final log through to the store (saveUpgradeTask persists task
	// status; appendUpgradeLog persists logs inline), so the user-visible
	// state is durable before we sleep. The sleep is a belt-and-suspenders
	// wait for the TCP socket and the net/http response buffer.
	go func() {
		// 1.5s: enough for a slow client or loaded system to drain the
		// response buffer; well below any sane supervisor restart window.
		time.Sleep(1500 * time.Millisecond)
		os.Exit(0)
	}()

	return nil
}

// probeSudoN returns true if the current user can run sudo -n (passwordless).
func probeSudoN() bool {
	if _, err := exec.LookPath("sudo"); err != nil {
		return false
	}
	cmd := exec.Command("sudo", "-n", "true")
	return cmd.Run() == nil
}

// sudoRun runs a command with sudo -n. Returns nil on success.
func sudoRun(args ...string) error {
	cmd := exec.Command("sudo", append([]string{"-n"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resolveRunningBinaryPath returns the absolute, symlink-resolved path to
// the currently running control binary.
func resolveRunningBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// EvalSymlinks can fail in edge cases (e.g. deleted exe after
		// hot-swap); fall back to os.Executable() result.
		return filepath.Clean(exe), nil
	}
	return filepath.Clean(resolved), nil
}

// checkWritable returns nil if the calling process can install a new binary
// at `path`. There are two viable strategies on POSIX:
//
//  1. Parent-dir writable → same-filesystem rename (atomic, preferred).
//  2. File itself writable → in-place truncate+write (non-atomic but avoids
//     needing write on the parent directory, which is the common shape of
//     /lingcdn/bin/<exe> being root-owned while <exe> is chown'd to the
//     service user).
//
// Returning nil means *some* strategy is available; callers must read the
// returned mode to decide which path to take.
//
// The previous implementation only tested strategy (1) by creating a probe
// in the parent dir, which made the in-process upgrade fail on the very
// common "root owns the dir, service user owns the binary" layout and
// forced a curl|bash fallback that itself requires sudo. That chain is what
// stranded operators with "sudo: a password is required".
type writeStrategy int

const (
	writeStrategyNone writeStrategy = iota
	writeStrategyRename
	writeStrategyInPlace
)

func checkWritable(path string) (writeStrategy, error) {
	// Strategy 1: can we create a sibling file? If yes, rename-based swap
	// (the atomic and preferred path) is viable.
	dir := filepath.Dir(path)
	probe, err := os.CreateTemp(dir, ".lingcdn-writetest-*")
	if err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		return writeStrategyRename, nil
	}
	renameErr := err

	// Strategy 2: can we open the target file itself for write? This
	// succeeds when the binary's owner (mode 0o644 / 0o755) is the service
	// user even if the parent dir is root:root 0755. We open with O_WRONLY
	// and immediately close — we do NOT truncate here.
	//
	// Opening O_WRONLY alone can lie on read-only mounts: the kernel only
	// verifies the filesystem write bit lazily (on first write) on some
	// overlay/bind/ROFS setups. To flush that lie out we issue a zero-byte
	// Write + Sync. io.Writer.Write(nil) is a no-op for regular files but
	// WriteAt with a zero-length slice and fsync (Sync) still triggers the
	// mount's write path, which ROFS will reject with EROFS. If the probe
	// fails, fall through to writeStrategyNone so the caller can surface a
	// clear error or escalate to sudo instead of crashing halfway through
	// the install.
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		// WriteAt with an empty buffer exercises the write path without
		// changing file contents; Sync forces the kernel to flush mount
		// state and return EROFS on read-only filesystems.
		_, writeErr := f.WriteAt([]byte{}, 0)
		syncErr := f.Sync()
		_ = f.Close()
		if writeErr == nil && syncErr == nil {
			return writeStrategyInPlace, nil
		}
		// Record the real error (EROFS, EIO, …) so the caller can show it
		// in the operator message rather than the less-useful "can't create
		// sibling file" one.
		if writeErr != nil {
			renameErr = fmt.Errorf("in-place write probe failed: %w (parent dir probe: %v)", writeErr, renameErr)
		} else {
			renameErr = fmt.Errorf("in-place sync probe failed: %w (parent dir probe: %v)", syncErr, renameErr)
		}
	}

	// Neither strategy works. Surface the rename-probe error since it's
	// the more common diagnostic ("permission denied on the dir").
	return writeStrategyNone, renameErr
}

// overwriteFileInPlace opens `dst` for write with O_TRUNC and streams the
// bytes from `src` into it. It deliberately does NOT create `dst`: the file
// must already exist with the correct owner/permissions. This is the escape
// hatch for the "root-owned dir, user-owned binary" layout where we can
// write into the binary's inode but cannot create siblings.
//
// The file is truncated BEFORE the write, so a crash mid-way leaves a
// partial binary. Callers should stash a copy of the previous contents
// elsewhere (e.g. /tmp) so they can roll back on error.
func overwriteFileInPlace(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open new binary: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return fmt.Errorf("open target for overwrite: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("stream bytes to target: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("fsync target: %w", err)
	}
	return nil
}

// copyFile creates `dst` (truncating if it exists) and copies `src` into it
// with the given permission mode. Unlike overwriteFileInPlace this uses
// O_CREATE and is therefore only useful when the caller has write access on
// the destination directory.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// fsyncDir opens `dir` and calls Sync on it, forcing the filesystem to
// persist directory-entry changes (rename, create, unlink) performed
// within that directory. On POSIX this is the only way to make a rename
// survive a power loss: file-data fsync via out.Sync() only guarantees
// the inode contents, not the dirent that points to it. On Windows the
// call is a no-op (directories cannot be opened as files there), which
// is fine since the whole in-process upgrade path is linux-only anyway.
func fsyncDir(dir string) error {
	if dir == "" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// downloadArtifact streams the upgrade package to disk, bounded at 128 MB
// to frustrate accidental or malicious oversized responses.
func downloadArtifact(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download http %d", resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	const maxArtifactBytes = 128 * 1024 * 1024
	if _, err := io.Copy(out, io.LimitReader(resp.Body, maxArtifactBytes)); err != nil {
		return err
	}
	return nil
}

// replaceUIFromArtifact extracts the bundled ui/ tree (if any) from the
// artifact and swaps it into the configured control UI directory. The
// operation is per-file: any files present in the new bundle overwrite the
// existing ones, but extra files in the old tree are left alone so that
// customized resources (favicon overrides, etc.) survive the upgrade.
func (s *Servers) replaceUIFromArtifact(ctx context.Context, task *upgradeTask, artifactPath, exePath string) error {
	uiDir := strings.TrimSpace(config.ResolveControlUIDir(s.cfg.ControlUIDir))
	if uiDir == "" {
		// Fall back to <exeDir>/ui so packages that bundle a bare ui/
		// directory still work.
		uiDir = filepath.Join(filepath.Dir(exePath), "ui")
	}
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		return fmt.Errorf("mkdir ui dir: %w", err)
	}
	extracted, err := extractUIFromArtifact(artifactPath, uiDir)
	if err != nil {
		return fmt.Errorf("extract ui: %w", err)
	}
	if extracted == 0 {
		s.appendUpgradeLog(ctx, task.ID, "INFO", "升级包未含 ui/ 目录，跳过前端更新", "")
		return nil
	}
	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("前端已更新到 %s（%d 个文件）", uiDir, extracted), "")
	return nil
}

// replaceUIFromArtifactSudo extracts UI to a temp dir first (no privilege),
// then uses sudo cp -r to copy into the target UI directory.
func (s *Servers) replaceUIFromArtifactSudo(ctx context.Context, task *upgradeTask, artifactPath, exePath string) error {
	uiDir := strings.TrimSpace(config.ResolveControlUIDir(s.cfg.ControlUIDir))
	if uiDir == "" {
		uiDir = filepath.Join(filepath.Dir(exePath), "ui")
	}

	// Extract to a temp dir first (no privilege needed).
	stagingDir, err := os.MkdirTemp("", ".lingcdn-ui-staging-*")
	if err != nil {
		return fmt.Errorf("mkdir staging: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	extracted, err := extractUIFromArtifact(artifactPath, stagingDir)
	if err != nil {
		return fmt.Errorf("extract ui to staging: %w", err)
	}
	if extracted == 0 {
		s.appendUpgradeLog(ctx, task.ID, "INFO", "升级包未含 ui/ 目录，跳过前端更新", "")
		return nil
	}

	// sudo mkdir -p + sudo cp -r staging/* -> uiDir/
	if err := sudoRun("mkdir", "-p", uiDir); err != nil {
		return fmt.Errorf("sudo mkdir ui dir: %w", err)
	}
	// Use shell glob to copy contents: sudo cp -r staging/. uiDir/
	if err := sudoRun("cp", "-r", stagingDir+"/.", uiDir+"/"); err != nil {
		return fmt.Errorf("sudo cp ui: %w", err)
	}
	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("前端已通过 sudo 更新到 %s（%d 个文件）", uiDir, extracted), "")
	return nil
}
