package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lingcdn/control/internal/store"
)

type fakeNodeInstallSSHClient struct {
	runFunc func(ctx context.Context, command string) (sshRunResult, error)
}

func (f *fakeNodeInstallSSHClient) Run(ctx context.Context, command string) (sshRunResult, error) {
	if f.runFunc == nil {
		return sshRunResult{}, nil
	}
	return f.runFunc(ctx, command)
}

func (f *fakeNodeInstallSSHClient) Close() error {
	return nil
}

func TestNodeInstallCommandUsesOfficialPortalBase(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/nodes/install-command?portal_base=auh.lingcdn.cloud&master_host=1.2.3.4", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("install-command: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}
	var out struct {
		Command    string `json:"command"`
		MasterHost string `json:"master_host"`
		PortalBase string `json:"portal_base"`
		ScriptURL  string `json:"script_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.PortalBase != "https://auth.lingcdn.cloud" {
		t.Fatalf("portal_base=%q", out.PortalBase)
	}
	if !strings.HasPrefix(out.ScriptURL, "https://auth.lingcdn.cloud/node_install.sh?") {
		t.Fatalf("script_url=%q", out.ScriptURL)
	}
	if out.MasterHost != "1.2.3.4:9443" {
		t.Fatalf("master_host=%q", out.MasterHost)
	}
	if !strings.Contains(out.Command, "https://auth.lingcdn.cloud/node_install.sh") {
		t.Fatalf("command=%q", out.Command)
	}
}

func TestNodeInstallSSHRejectsInvalidUser(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	body := bytes.NewBufferString(`{"ssh_host":"10.0.0.1","ssh_user":"ubuntu","ssh_password":"secret","master_host":"1.2.3.4"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/nodes/install-ssh", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("install-ssh: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(raw))
	}
}

func TestNodeInstallSSHConnectionFailure(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	oldFactory := newNodeInstallSSHClient
	newNodeInstallSSHClient = func(ctx context.Context, target nodeInstallSSHRequest) (nodeInstallSSHClient, error) {
		return nil, errors.New("dial tcp timeout")
	}
	t.Cleanup(func() { newNodeInstallSSHClient = oldFactory })

	resp := postNodeInstallSSH(t, ts.URL, token, map[string]any{
		"ssh_host":     "10.0.0.1",
		"ssh_user":     "root",
		"ssh_password": "secret",
		"master_host":  "1.2.3.4",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var out nodeInstallSSHResponse
	decodeJSONBody(t, resp.Body, &out)
	if out.OK || out.Status != "failed" || out.Message != "SSH connection failed" {
		t.Fatalf("unexpected response: %+v", out)
	}
	if len(out.Logs) == 0 || !strings.Contains(strings.Join(out.Logs, "\n"), "SSH") {
		t.Fatalf("logs=%v", out.Logs)
	}
}

func TestNodeInstallSSHPrecheckFailures(t *testing.T) {
	cases := []struct {
		name    string
		runFunc func(ctx context.Context, command string) (sshRunResult, error)
		message string
	}{
		{
			name: "missing bash",
			runFunc: func(ctx context.Context, command string) (sshRunResult, error) {
				switch {
				case command == "hostname":
					return sshRunResult{Combined: "node-a\n"}, nil
				case strings.Contains(command, "command -v bash"):
					return sshRunResult{}, errors.New("exit status 1")
				default:
					return sshRunResult{}, nil
				}
			},
			message: "remote host is missing bash",
		},
		{
			name: "missing curl",
			runFunc: func(ctx context.Context, command string) (sshRunResult, error) {
				switch {
				case command == "hostname":
					return sshRunResult{Combined: "node-a\n"}, nil
				case strings.Contains(command, "command -v bash"):
					return sshRunResult{}, nil
				case strings.Contains(command, "command -v curl"):
					return sshRunResult{}, errors.New("exit status 1")
				default:
					return sshRunResult{}, nil
				}
			},
			message: "remote host is missing curl",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ts, token := newControlTestServer(t, "")
			oldFactory := newNodeInstallSSHClient
			newNodeInstallSSHClient = func(ctx context.Context, target nodeInstallSSHRequest) (nodeInstallSSHClient, error) {
				return &fakeNodeInstallSSHClient{runFunc: tc.runFunc}, nil
			}
			t.Cleanup(func() { newNodeInstallSSHClient = oldFactory })

			resp := postNodeInstallSSH(t, ts.URL, token, map[string]any{
				"ssh_host":     "10.0.0.1",
				"ssh_user":     "root",
				"ssh_password": "secret",
				"master_host":  "1.2.3.4",
			})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status=%d", resp.StatusCode)
			}
			var out nodeInstallSSHResponse
			decodeJSONBody(t, resp.Body, &out)
			if out.OK || out.Message != tc.message {
				t.Fatalf("unexpected response: %+v", out)
			}
		})
	}
}

func TestNodeInstallSSHInstalledAndRegistered(t *testing.T) {
	s, ts, token := newControlTestServer(t, "")
	if err := s.store.CreateNode(context.Background(), &store.Node{
		ID:       "node-1",
		Hostname: "node-a",
		PublicIP: "203.0.113.10",
		Status:   "online",
	}); err != nil {
		t.Fatalf("create node: %v", err)
	}

	oldFactory := newNodeInstallSSHClient
	newNodeInstallSSHClient = func(ctx context.Context, target nodeInstallSSHRequest) (nodeInstallSSHClient, error) {
		return &fakeNodeInstallSSHClient{
			runFunc: func(ctx context.Context, command string) (sshRunResult, error) {
				switch {
				case command == "hostname":
					return sshRunResult{Combined: "node-a\n"}, nil
				case strings.Contains(command, "command -v bash"):
					return sshRunResult{}, nil
				case strings.Contains(command, "command -v curl"):
					return sshRunResult{}, nil
				case strings.HasPrefix(command, "bash -lc "):
					return sshRunResult{Combined: "download ok\ninstall ok\n"}, nil
				default:
					return sshRunResult{}, nil
				}
			},
		}, nil
	}
	t.Cleanup(func() { newNodeInstallSSHClient = oldFactory })

	resp := postNodeInstallSSH(t, ts.URL, token, map[string]any{
		"ssh_host":     "10.0.0.1",
		"ssh_user":     "root",
		"ssh_password": "secret",
		"master_host":  "1.2.3.4",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var out nodeInstallSSHResponse
	decodeJSONBody(t, resp.Body, &out)
	if !out.OK || out.Status != "installed" || out.Message != "install and registration completed" {
		t.Fatalf("unexpected response: %+v", out)
	}
	if out.Node == nil || out.Node.Hostname != "node-a" {
		t.Fatalf("node summary invalid: %+v", out.Node)
	}
}

func TestNodeInstallSSHInstalledWaitingRegister(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	oldFactory := newNodeInstallSSHClient
	oldWait := nodeInstallRegisterWait
	oldPoll := nodeInstallRegisterPoll
	newNodeInstallSSHClient = func(ctx context.Context, target nodeInstallSSHRequest) (nodeInstallSSHClient, error) {
		return &fakeNodeInstallSSHClient{
			runFunc: func(ctx context.Context, command string) (sshRunResult, error) {
				switch {
				case command == "hostname":
					return sshRunResult{Combined: "node-wait\n"}, nil
				case strings.Contains(command, "command -v bash"):
					return sshRunResult{}, nil
				case strings.Contains(command, "command -v curl"):
					return sshRunResult{}, nil
				case strings.HasPrefix(command, "bash -lc "):
					return sshRunResult{Combined: "install ok\n"}, nil
				default:
					return sshRunResult{}, nil
				}
			},
		}, nil
	}
	nodeInstallRegisterWait = 20 * time.Millisecond
	nodeInstallRegisterPoll = 5 * time.Millisecond
	t.Cleanup(func() {
		newNodeInstallSSHClient = oldFactory
		nodeInstallRegisterWait = oldWait
		nodeInstallRegisterPoll = oldPoll
	})

	resp := postNodeInstallSSH(t, ts.URL, token, map[string]any{
		"ssh_host":     "10.0.0.1",
		"ssh_user":     "root",
		"ssh_password": "secret",
		"master_host":  "1.2.3.4",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var out nodeInstallSSHResponse
	decodeJSONBody(t, resp.Body, &out)
	if !out.OK || out.Status != "installed_waiting_register" || out.Message != "install completed, waiting for node registration" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestNodeInstallSSHInstallFailure(t *testing.T) {
	_, ts, token := newControlTestServer(t, "")

	oldFactory := newNodeInstallSSHClient
	newNodeInstallSSHClient = func(ctx context.Context, target nodeInstallSSHRequest) (nodeInstallSSHClient, error) {
		return &fakeNodeInstallSSHClient{
			runFunc: func(ctx context.Context, command string) (sshRunResult, error) {
				switch {
				case command == "hostname":
					return sshRunResult{Combined: "node-b\n"}, nil
				case strings.Contains(command, "command -v bash"):
					return sshRunResult{}, nil
				case strings.Contains(command, "command -v curl"):
					return sshRunResult{}, nil
				case strings.HasPrefix(command, "bash -lc "):
					return sshRunResult{Combined: "permission denied\n"}, errors.New("exit status 1")
				default:
					return sshRunResult{}, nil
				}
			},
		}, nil
	}
	t.Cleanup(func() { newNodeInstallSSHClient = oldFactory })

	resp := postNodeInstallSSH(t, ts.URL, token, map[string]any{
		"ssh_host":     "10.0.0.1",
		"ssh_user":     "root",
		"ssh_password": "secret",
		"master_host":  "1.2.3.4",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var out nodeInstallSSHResponse
	decodeJSONBody(t, resp.Body, &out)
	if out.OK || out.Status != "failed" || out.Message != "remote install command failed" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

// TestNodeInstallCommandRejectedWhenNodeLimitReached 防止以下回归：
// 历史上 /api/nodes/install-command 仅校验管理员角色，从不调用
// licenseAllowsNewNode，导致 license 已达上限时管理员仍能拿到一份完全可用的
// 安装命令，UI 也不会提示。这是用户报告 "明明达到节点上限却还能装节点" 的
// 直接成因之一，单测固化预检逻辑。
func TestNodeInstallCommandRejectedWhenNodeLimitReached(t *testing.T) {
	s, ts, token := newControlTestServer(t, "")
	if err := s.store.CreateNode(context.Background(), &store.Node{
		ID:       "node-1",
		Hostname: "node-a",
		Status:   "online",
	}); err != nil {
		t.Fatalf("create node: %v", err)
	}
	s.setLicenseState(licenseState{
		Status:    "active",
		MaxNodes:  1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/nodes/install-command?master_host=1.2.3.4", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("install-command: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s, want 403", resp.StatusCode, string(raw))
	}
	var out struct {
		Error string `json:"error"`
	}
	decodeJSONBody(t, resp.Body, &out)
	if !strings.Contains(out.Error, "node limit reached") {
		t.Fatalf("error=%q, want contains 'node limit reached'", out.Error)
	}
}

// TestNodeInstallSSHRejectedWhenNodeLimitReached 与上一个测试对应，覆盖 SSH
// 远端安装入口；该入口里负责实际下发安装命令并等待节点注册，不预检的话
// 节点注册到主控之前不会有任何提示，会浪费目标主机的安装周期。
func TestNodeInstallSSHRejectedWhenNodeLimitReached(t *testing.T) {
	s, ts, token := newControlTestServer(t, "")
	if err := s.store.CreateNode(context.Background(), &store.Node{
		ID:       "node-1",
		Hostname: "node-a",
		Status:   "online",
	}); err != nil {
		t.Fatalf("create node: %v", err)
	}
	s.setLicenseState(licenseState{
		Status:    "active",
		MaxNodes:  1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	resp := postNodeInstallSSH(t, ts.URL, token, map[string]any{
		"ssh_host":     "10.0.0.1",
		"ssh_user":     "root",
		"ssh_password": "secret",
		"master_host":  "1.2.3.4",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s, want 403", resp.StatusCode, string(raw))
	}
	var out struct {
		Error string `json:"error"`
	}
	decodeJSONBody(t, resp.Body, &out)
	if !strings.Contains(out.Error, "node limit reached") {
		t.Fatalf("error=%q, want contains 'node limit reached'", out.Error)
	}
}

// TestNodeInstallCommandUnlicensedRejected 覆盖 license 未激活场景：
// 在 unlicensed 状态下 withAuth 中间件会先于 handler 返回 402 (Payment
// Required)，install-command 完全不可达。这是"上限已到 + 未授权"两条路径
// 中更早的那一条；handler 内部的 preInstallNodeLicenseCheck 主要用来兜
// 住 active license + MaxNodes 已达的场景（见前面两个测试）。
func TestNodeInstallCommandUnlicensedRejected(t *testing.T) {
	s, ts, token := newControlTestServer(t, "")
	s.setLicenseState(licenseState{Status: "unlicensed"})

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/nodes/install-command?master_host=1.2.3.4", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("install-command: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPaymentRequired {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s, want 402", resp.StatusCode, string(raw))
	}
}

// TestAdminCreateNodeRejectedWhenNodeLimitReached 防止管理员通过 POST
// /api/nodes 直接绕过授权检查手动建节点。
func TestAdminCreateNodeRejectedWhenNodeLimitReached(t *testing.T) {
	s, ts, token := newControlTestServer(t, "")
	if err := s.store.CreateNode(context.Background(), &store.Node{
		ID:       "node-1",
		Hostname: "node-a",
		Status:   "online",
	}); err != nil {
		t.Fatalf("create node: %v", err)
	}
	s.setLicenseState(licenseState{
		Status:    "active",
		MaxNodes:  1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	body := bytes.NewBufferString(`{"hostname":"node-b"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/nodes", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s, want 403", resp.StatusCode, string(raw))
	}
	var out struct {
		Error string `json:"error"`
	}
	decodeJSONBody(t, resp.Body, &out)
	if !strings.Contains(out.Error, "node limit reached") {
		t.Fatalf("error=%q, want contains 'node limit reached'", out.Error)
	}
}

func postNodeInstallSSH(t *testing.T, baseURL, token string, payload map[string]any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/nodes/install-ssh", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post install-ssh: %v", err)
	}
	return resp
}

func decodeJSONBody(t *testing.T, body io.ReadCloser, out any) {
	t.Helper()
	defer body.Close()
	if err := json.NewDecoder(body).Decode(out); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
