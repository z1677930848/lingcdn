package server

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const checksumMsgV1Pre = "lingcdn:v1:sha256:"

// performControlUpgrade orchestrates a control-plane self-upgrade. The
// *default* path is the in-process upgrade (download → verify → swap →
// self-exit), which requires no root and no external shell script. The
// legacy curl|bash path remains as an automatic fallback for sites where
// the in-process swap can't work (e.g. the binary lives under
// /usr/local/bin owned by root and the operator has wired up systemd as
// root on purpose).
func (s *Servers) performControlUpgrade(ctx context.Context, task *upgradeTask) error {
	s.appendUpgradeLog(ctx, task.ID, "INFO", "开始升级主控", "")
	if runtime.GOOS != "linux" {
		return errors.New("升级仅支持 linux 环境")
	}

	// Preferred path: self-contained in-process upgrade. No root, no
	// sudo, no shell. Succeeds on any deployment where the service user
	// owns its install dir.
	if inProcErr := s.performControlUpgradeInProcess(ctx, task); inProcErr == nil {
		return nil
	} else {
		s.appendUpgradeLog(ctx, task.ID, "WARN",
			fmt.Sprintf("内置升级失败，将回退到脚本升级：%v", inProcErr), "")
	}

	portal := s.upgradePortalBase()
	channel, err := normalizeUpgradeChannel(task.Channel)
	if err != nil {
		return err
	}

	serviceName := strings.TrimSpace(s.cfg.ControlService)
	if serviceName == "" {
		serviceName = "lingcdn-control"
	}

	scriptURL := strings.TrimRight(portal, "/") + "/control_update.sh"

	// 安全校验：确保脚本 URL 来自可信的 HTTPS 门户
	trustedPortalURL, err := url.Parse(portal)
	if err != nil {
		return fmt.Errorf("升级源地址配置非法: %w", err)
	}
	parsedScriptURL, err := url.Parse(scriptURL)
	if err != nil {
		return fmt.Errorf("升级脚本地址不合法: %w", err)
	}
	if !strings.EqualFold(parsedScriptURL.Scheme, "https") || !strings.EqualFold(parsedScriptURL.Hostname(), trustedPortalURL.Hostname()) {
		return fmt.Errorf("升级脚本地址不可信（仅允许 %s）：%s", maskUpgradeURL(portal), maskUpgradeURL(scriptURL))
	}

	// 使用 curl -fsSL <url> | bash -s -- <args> 执行升级
	curlCmd := fmt.Sprintf("curl -fsSL '%s' | bash -s -- --channel '%s' --service_name '%s'",
		scriptURL, channel, serviceName)

	// control_update.sh 需要 root 权限。主控进程如果不是 root，优先用
	// sudo -n 提权；若 sudo 也不可用则记录清晰的运维指引再失败，避免
	// 以前的"请使用 root 用户执行此脚本"错误把运维卡在无线索的状态。
	priv := choosePrivilegeEscalation()

	// Short-circuit on priv.mode == "none": running `curl | bash` without
	// privilege is guaranteed to fail at the first `systemctl stop` inside
	// the script, and we already know sudo is unavailable (choosePrivilegeEscalation
	// probed it). Burning 10s on a doomed download just to produce the same
	// error message later is worse than refusing immediately with a pointer
	// at the fix.
	if priv.mode == "none" {
		s.appendUpgradeLog(ctx, task.ID, "ERROR",
			"无法执行脚本升级：主控以非 root 身份运行，且 sudo -n（免密 sudo）不可用。\n"+
				"请任选其一来恢复在线升级能力：\n"+
				"  1) 让主控运行用户拥有部署目录（推荐，无需任何 sudo）：\n"+
				"       sudo chown -R \"$(ps -o user= -p $(pgrep -f lingcdn-control | head -1))\" /lingcdn\n"+
				"  2) 配置免密 sudo：\n"+
				"       echo \"$(id -un) ALL=(root) NOPASSWD: /bin/bash,/bin/mv,/bin/cp,/bin/rm,/bin/chmod,/bin/mkdir,/bin/systemctl\" | sudo tee /etc/sudoers.d/lingcdn-upgrade\n"+
				"  3) 以 root 运行主控（修改 systemd unit 的 User= 字段）。",
			"")
		return errors.New("脚本升级路径被阻断：无 root 且无免密 sudo（详见日志指引）")
	}

	scriptPath, args := buildUpgradeCommand(priv, curlCmd)

	s.appendUpgradeLog(ctx, task.ID, "INFO",
		fmt.Sprintf("执行升级命令（提权=%s）: %s", priv.mode, curlCmd), "")

	if err := s.runUpgradeScript(ctx, task.ID, scriptPath, args); err != nil {
		// priv.mode can only be "root" or "sudo" here (we short-circuited
		// "none" above). If we got here with sudo, something other than
		// the passwordless probe has gone wrong (e.g. sudoers restricts
		// which commands are allowed, or /bin/bash isn't permitted).
		if priv.mode == "sudo" {
			s.appendUpgradeLog(ctx, task.ID, "ERROR",
				"sudo -n 提权通过，但执行脚本仍失败。可能是 sudoers 限制了允许的命令。\n"+
					"解决方式：\n"+
					"  echo \"$(id -un) ALL=(root) NOPASSWD: /bin/bash\" | sudo tee /etc/sudoers.d/lingcdn-upgrade",
				"")
		}
		return fmt.Errorf("执行 control_update.sh 失败: %w", err)
	}
	s.appendUpgradeLog(ctx, task.ID, "INFO", "control_update.sh 执行完成", "")

	// Notify the portal before the script's systemctl-restart race closes
	// our window. The script itself does `systemctl restart`, so by the
	// time it returns 0, either (a) systemd has already SIGTERM'd us and
	// this block never runs because we're dead, or (b) it will kill us in
	// the next instant. Path (a) is fine: the NEW process will do its
	// startup reportSystemOnce. Path (b) benefits from us getting a
	// best-effort report out first so operators don't see a ~10 minute
	// stale-version window between the restart and the next periodic tick.
	//
	// We do NOT os.Exit here: that's the script's / systemd's job. If we
	// survive this call, we return nil and the handler completes normally;
	// systemd/nohup will restart us shortly when the script's own restart
	// fires.
	reportCtx, cancelReport := context.WithTimeout(ctx, 5*time.Second)
	if msg, err := s.reportSystemOnce(reportCtx); err != nil {
		s.appendUpgradeLog(ctx, task.ID, "WARN",
			fmt.Sprintf("pre-restart 向 portal 上报失败（可忽略，新进程启动后会重试）：%v", err), "")
	} else {
		s.appendUpgradeLog(ctx, task.ID, "INFO",
			fmt.Sprintf("pre-restart 向 portal 上报成功 (%s)", msg), "")
	}
	cancelReport()

	return nil
}

// privilegeEscalation describes how to escalate to root when launching the
// upgrade script. Kept as a tiny value type so the logic (euid + sudo probe)
// can be unit-tested without actually executing anything.
type privilegeEscalation struct {
	mode string // "root" | "sudo" | "none"
}

// choosePrivilegeEscalation decides at runtime how the upgrade subprocess
// should acquire root:
//   - already root (euid 0) → run directly.
//   - non-root but `sudo -n true` succeeds → wrap with sudo.
//   - otherwise → fall through; the caller should surface an operator hint
//     instead of pointlessly invoking a sudo that will just fail.
//
// NOTE: the previous implementation only tested `exec.LookPath("sudo")`.
// That produced the exact failure sequence the user hit on 1.0.6:
//
//	[WARN] 内置升级失败... sudo -n 也不可用      ← probeSudoN already said no
//	[INFO] 执行升级命令（提权=sudo）: curl ...   ← but we tried anyway
//	[ERROR] sudo: a password is required        ← inevitable
//
// probeSudoN is cheap (single subprocess) and is the same signal the
// in-process path already uses, so reusing it here keeps both paths in
// lockstep.
func choosePrivilegeEscalation() privilegeEscalation {
	if os.Geteuid() == 0 {
		return privilegeEscalation{mode: "root"}
	}
	if probeSudoN() {
		return privilegeEscalation{mode: "sudo"}
	}
	return privilegeEscalation{mode: "none"}
}

// buildUpgradeCommand renders the (scriptPath, args) tuple to hand to
// os/exec based on the privilege decision. Separated from
// choosePrivilegeEscalation so tests can drive the rendering with a
// hand-constructed mode instead of depending on the host's euid/sudo.
func buildUpgradeCommand(priv privilegeEscalation, shellCmd string) (string, []string) {
	switch priv.mode {
	case "sudo":
		// -n: never prompt for a password. -E: keep selected environment
		// (some distros strip PATH under sudo and break `curl`/`bash`).
		return "sudo", []string{"-n", "-E", "/bin/bash", "-c", shellCmd}
	default:
		return "/bin/bash", []string{"-c", shellCmd}
	}
}

func normalizeArch(goarch string) string {
	switch strings.ToLower(strings.TrimSpace(goarch)) {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return ""
	}
}

func verifyChecksumSignatureSHA256(pubKeyBase64, checksumHex, sigBase64 string) error {
	pubKeyBase64 = strings.TrimSpace(pubKeyBase64)
	if pubKeyBase64 == "" {
		return errors.New("pubkey empty")
	}
	pub, err := base64.StdEncoding.DecodeString(pubKeyBase64)
	if err != nil {
		return fmt.Errorf("decode pubkey: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid pubkey length: %d", len(pub))
	}

	sigBase64 = strings.TrimSpace(sigBase64)
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: %d", len(sig))
	}

	checksumHex = strings.ToLower(strings.TrimSpace(checksumHex))
	if len(checksumHex) != 64 {
		return fmt.Errorf("invalid checksum length: %d", len(checksumHex))
	}
	if _, err := hex.DecodeString(checksumHex); err != nil {
		return fmt.Errorf("invalid checksum hex: %w", err)
	}

	msg := []byte(checksumMsgV1Pre + checksumHex)
	if !ed25519.Verify(ed25519.PublicKey(pub), msg, sig) {
		return errors.New("ed25519 verify failed")
	}
	return nil
}

func inferUpgradeArtifactSuffix(downloadURL string) string {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return ""
	}
	p := strings.ToLower(strings.TrimSpace(u.Path))
	switch {
	case strings.HasSuffix(p, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(p, ".tgz"):
		return ".tgz"
	case strings.HasSuffix(p, ".zip"):
		return ".zip"
	default:
		return ""
	}
}

func maskUpgradeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func stageFileToDir(srcPath, dstDir, pattern string) (string, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", err
	}

	dst, err := os.CreateTemp(dstDir, pattern)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, h), src); err != nil {
		_ = os.Remove(dst.Name())
		return "", err
	}

	if err := os.Chmod(dst.Name(), 0o755); err != nil {
		_ = os.Remove(dst.Name())
		return "", err
	}

	return filepath.Clean(dst.Name()), nil
}
