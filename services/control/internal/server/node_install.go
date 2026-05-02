package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/lingcdn/control/internal/buildinfo"
	"github.com/lingcdn/control/internal/store"
)

const (
	defaultNodeInstallTTLMinutes = 60
	maxNodeInstallLogLines       = 200
)

var (
	nodeInstallSSHConnectTimeout = 10 * time.Second
	nodeInstallSSHCommandTimeout = 180 * time.Second
	nodeInstallRegisterWait      = 30 * time.Second
	nodeInstallRegisterPoll      = 2 * time.Second
	newNodeInstallSSHClient      = func(ctx context.Context, target nodeInstallSSHRequest) (nodeInstallSSHClient, error) {
		return dialNodeInstallSSHClient(ctx, target)
	}
)

type nodeInstallCommandRequest struct {
	PortalBase    string
	ScriptURL     string
	MasterHost    string
	MasterVersion string
	MasterChannel string
	TTLMinutes    int
	RequestHost   string
}

type nodeInstallCommandSpec struct {
	Command       string    `json:"command"`
	MasterHost    string    `json:"master_host"`
	MasterToken   string    `json:"master_token"`
	MasterVersion string    `json:"master_version"`
	MasterChannel string    `json:"master_channel"`
	ExpiresAt     time.Time `json:"expires_at"`
	PortalBase    string    `json:"portal_base"`
	ScriptURL     string    `json:"script_url"`
	Style         string    `json:"style"`
}

type nodeInstallSSHRequest struct {
	SSHHost     string `json:"ssh_host"`
	SSHPort     int    `json:"ssh_port"`
	SSHUser     string `json:"ssh_user"`
	SSHPassword string `json:"ssh_password"`
	MasterHost  string `json:"master_host"`
	TTLMinutes  int    `json:"ttl_minutes"`
	PortalBase  string `json:"portal_base"`
}

type nodeInstallSSHNodeSummary struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	PublicIP string `json:"public_ip,omitempty"`
	Status   string `json:"status,omitempty"`
}

type nodeInstallSSHResponse struct {
	OK             bool                       `json:"ok"`
	Status         string                     `json:"status"`
	Message        string                     `json:"message"`
	Logs           []string                   `json:"logs"`
	RemoteHostname string                     `json:"remote_hostname,omitempty"`
	MasterHost     string                     `json:"master_host"`
	ExpiresAt      time.Time                  `json:"expires_at"`
	Node           *nodeInstallSSHNodeSummary `json:"node,omitempty"`
}

type sshRunResult struct {
	Combined   string
	ExitStatus int
}

type nodeInstallSSHClient interface {
	Run(ctx context.Context, command string) (sshRunResult, error)
	Close() error
}

type realNodeInstallSSHClient struct {
	client             *ssh.Client
	hostKeyFingerprint string
}

type nodeInstallLogBuffer struct {
	lines       []string
	limit       int
	dropped     int
	sensitive   []string
	truncateMsg string
}

func newNodeInstallLogBuffer(limit int, sensitive ...string) *nodeInstallLogBuffer {
	if limit <= 0 {
		limit = maxNodeInstallLogLines
	}
	filtered := make([]string, 0, len(sensitive))
	for _, item := range sensitive {
		item = strings.TrimSpace(item)
		if item != "" {
			filtered = append(filtered, item)
		}
	}
	return &nodeInstallLogBuffer{
		limit:       limit,
		sensitive:   filtered,
		truncateMsg: "[logs truncated]",
	}
}

func (b *nodeInstallLogBuffer) Add(line string) {
	if b == nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	for _, item := range b.sensitive {
		line = strings.ReplaceAll(line, item, "***")
	}
	if len(b.lines) < b.limit {
		b.lines = append(b.lines, line)
		return
	}
	b.dropped++
}

func (b *nodeInstallLogBuffer) AddOutput(output string) {
	if b == nil {
		return
	}
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	for _, line := range strings.Split(output, "\n") {
		b.Add(line)
	}
}

func (b *nodeInstallLogBuffer) Lines() []string {
	if b == nil {
		return nil
	}
	if b.dropped == 0 {
		out := make([]string, len(b.lines))
		copy(out, b.lines)
		return out
	}
	if len(b.lines) == 0 {
		return []string{b.truncateMsg}
	}
	out := make([]string, 0, len(b.lines))
	keep := len(b.lines)
	if keep >= b.limit {
		keep = b.limit - 1
	}
	if keep < 0 {
		keep = 0
	}
	out = append(out, b.lines[:keep]...)
	out = append(out, fmt.Sprintf("%s (omitted %d lines)", b.truncateMsg, b.dropped))
	return out
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

// buildNodeInstallFilebeatTail returns the Filebeat-related flags appended
// to the generated install command. Triggered whenever
// settings.ElasticsearchURL is configured — node-side Filebeat is now
// the only delivery path for access/error logs to Elasticsearch.
func (s *Servers) buildNodeInstallFilebeatTail(ctx context.Context) string {
	if s == nil || s.store == nil {
		return ""
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil || settings == nil {
		return ""
	}
	esURL := strings.TrimSpace(settings.ElasticsearchURL)
	if esURL == "" {
		return ""
	}
	host, port, protocol, perr := parseESEndpoint(esURL)
	if perr != nil || host == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString(" --es_host ")
	b.WriteString(shellQuote(host))
	if port != "" {
		b.WriteString(" --es_port ")
		b.WriteString(shellQuote(port))
	}
	if protocol != "" {
		b.WriteString(" --es_protocol ")
		b.WriteString(shellQuote(protocol))
	}
	if u := strings.TrimSpace(settings.ElasticsearchUser); u != "" {
		b.WriteString(" --es_user ")
		b.WriteString(shellQuote(u))
	}
	if p := settings.ElasticsearchPass; p != "" {
		b.WriteString(" --es_pass ")
		b.WriteString(shellQuote(p))
	}
	if idx := strings.TrimSpace(settings.ElasticsearchIndex); idx != "" && idx != "cdn-access" {
		b.WriteString(" --es_index_prefix ")
		b.WriteString(shellQuote(idx))
	}
	return b.String()
}

// parseESEndpoint extracts host / port / scheme from a configured ES URL.
// Default port is supplied (443 for https, 9200 otherwise) so the install
// script does not have to guess. Returns empty host on parse failure so
// the caller can fall through and emit no Filebeat flags.
func parseESEndpoint(raw string) (host, port, protocol string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", errors.New("empty url")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", err
	}
	protocol = strings.ToLower(strings.TrimSpace(u.Scheme))
	if protocol != "http" && protocol != "https" {
		return "", "", "", fmt.Errorf("unsupported ES scheme %q", u.Scheme)
	}
	host = strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", "", "", errors.New("missing host")
	}
	port = strings.TrimSpace(u.Port())
	if port == "" {
		if protocol == "https" {
			port = "443"
		} else {
			port = "9200"
		}
	}
	return host, port, protocol, nil
}

func defaultNodeInstallVersion() string {
	// The install script generator stamps the master's version into the
	// script so the remote node installs a compatible client. Sourcing this
	// from buildinfo (rather than an env var) means the value is always
	// the real binary version, never a forgotten env entry from a prior
	// deploy.
	return buildinfo.Version()
}

// defaultNodeInstallChannel derives the upgrade channel from the running master's
// version string.  If the version contains "beta" (e.g. 1.0.0-beta.1), the channel
// is "beta"; otherwise the master is considered "stable".
func defaultNodeInstallChannel() string {
	if ch := strings.TrimSpace(os.Getenv("UPGRADE_CHANNEL")); ch != "" {
		return strings.ToLower(ch)
	}
	version := strings.ToLower(defaultNodeInstallVersion())
	if strings.Contains(version, "beta") {
		return "beta"
	}
	return "stable"
}

func appendNodeInstallTimestamp(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	if strings.Contains(raw, "?") {
		return raw + "&t=" + ts
	}
	return raw + "?t=" + ts
}

func firstNonEmptyLine(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func (s *Servers) resolveNodeInstallMasterHost(masterHost, requestHost string) (string, error) {
	_, grpcPort, err := net.SplitHostPort(s.cfg.GRPCAddr)
	if err != nil || grpcPort == "" {
		grpcPort = "9443"
	}
	resolveIPHostPort := func(hostPort string) (string, bool) {
		host := strings.TrimSpace(hostPort)
		port := grpcPort
		if h, p, e := net.SplitHostPort(hostPort); e == nil {
			host = strings.TrimSpace(h)
			port = strings.TrimSpace(p)
		} else if strings.Contains(hostPort, ":") {
			host = strings.TrimSpace(hostPort)
		}
		if net.ParseIP(host) == nil {
			return "", false
		}
		if port == "" {
			port = grpcPort
		}
		return net.JoinHostPort(host, port), true
	}
	if strings.TrimSpace(masterHost) != "" {
		if v, ok := resolveIPHostPort(masterHost); ok {
			return v, nil
		}
		return "", fmt.Errorf("master_host %q must be a public IP or IP:port", masterHost)
	}
	if strings.TrimSpace(s.cfg.PublicGRPCEndpoint) != "" {
		if v, ok := resolveIPHostPort(s.cfg.PublicGRPCEndpoint); ok {
			return v, nil
		}
		return "", fmt.Errorf("PUBLIC_GRPC_ENDPOINT %q must be a public IP or IP:port", s.cfg.PublicGRPCEndpoint)
	}
	if strings.TrimSpace(s.cfg.PublicIP) != "" {
		if v, ok := resolveIPHostPort(s.cfg.PublicIP); ok {
			return v, nil
		}
		return "", fmt.Errorf("PUBLIC_IP %q must be a public IP", s.cfg.PublicIP)
	}
	hostOnly := strings.TrimSpace(requestHost)
	if h, _, e := net.SplitHostPort(hostOnly); e == nil {
		hostOnly = h
	}
	if v, ok := resolveIPHostPort(hostOnly); ok {
		return v, nil
	}
	return "", fmt.Errorf("configure PUBLIC_IP or PUBLIC_GRPC_ENDPOINT with a public IP (request host %q is not usable)", requestHost)
}

// preInstallNodeLicenseCheck verifies the current license still allows
// provisioning another node before the install command / SSH installer hands
// out a working bootstrap token. The remote hostname is unknown at install
// time (it is read by `hostname` on the target box), so the check is always
// performed as if the caller is a brand new node — same fail-closed policy
// that licenseAllowsNewNode applies during gRPC RegisterNode.
//
// Returning an error here is what surfaces "节点数已达上限" to the admin UI;
// without this short-circuit the UI happily showed the install command even
// when MaxNodes was reached, and the admin only found out after the install
// script ran on the target host.
func (s *Servers) preInstallNodeLicenseCheck(ctx context.Context) error {
	st, allowed, err := s.licenseAllowsNewNode(ctx, false)
	if err != nil {
		return fmt.Errorf("授权检查失败：%w", err)
	}
	if allowed {
		return nil
	}
	reason := strings.TrimSpace(st.Reason)
	if reason == "" {
		reason = "当前授权不允许新增节点"
	}
	return errors.New(reason)
}

func (s *Servers) buildNodeInstallCommand(ctx context.Context, req nodeInstallCommandRequest) (*nodeInstallCommandSpec, error) {
	portal := s.portalBase()
	if portal == "" {
		return nil, errors.New("portal base is required")
	}
	scriptURL := portal + "/node_install.sh"
	masterHost, err := s.resolveNodeInstallMasterHost(req.MasterHost, req.RequestHost)
	if err != nil {
		return nil, err
	}
	version := strings.TrimSpace(req.MasterVersion)
	if version == "" {
		version = "latest"
	}
	channel := strings.TrimSpace(req.MasterChannel)
	if channel == "" {
		channel = defaultNodeInstallChannel()
	}
	ttlMinutes := req.TTLMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = defaultNodeInstallTTLMinutes
	}
	token, exp, err := s.store.CreateBootstrapToken(ctx, "install generated", time.Duration(ttlMinutes)*time.Minute)
	if err != nil {
		return nil, err
	}
	scriptURL = appendNodeInstallTimestamp(scriptURL)
	cmd := fmt.Sprintf(
		"curl -fsSL -o node_install.sh %s && bash node_install.sh --master_host %s --master_token %s --master_version %s --upgrade_channel %s --portal_base %s%s%s",
		shellQuote(scriptURL),
		shellQuote(masterHost),
		shellQuote(token),
		shellQuote(version),
		shellQuote(channel),
		shellQuote(portal),
		func() string {
			if strings.TrimSpace(s.cfg.UpgradePubKey) == "" {
				return ""
			}
			return " --upgrade_pubkey " + shellQuote(s.cfg.UpgradePubKey)
		}(),
		s.buildNodeInstallFilebeatTail(ctx),
	)
	return &nodeInstallCommandSpec{
		Command:       cmd,
		MasterHost:    masterHost,
		MasterToken:   token,
		MasterVersion: version,
		MasterChannel: channel,
		ExpiresAt:     exp,
		PortalBase:    portal,
		ScriptURL:     scriptURL,
		Style:         "master",
	}, nil
}

// handleNodeInstallCommand returns a ready-to-copy install command (curl + bash) using portal install script.
func (s *Servers) handleNodeInstallCommand(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if err := s.preInstallNodeLicenseCheck(ctx); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
		return
	}
	ttlMinutes := 0
	if v := strings.TrimSpace(r.URL.Query().Get("ttl_minutes")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			ttlMinutes = n
		}
	}
	spec, err := s.buildNodeInstallCommand(ctx, nodeInstallCommandRequest{
		PortalBase:    r.URL.Query().Get("portal_base"),
		ScriptURL:     r.URL.Query().Get("script_url"),
		MasterHost:    r.URL.Query().Get("master_host"),
		MasterVersion: r.URL.Query().Get("master_version"),
		MasterChannel: r.URL.Query().Get("master_channel"),
		TTLMinutes:    ttlMinutes,
		RequestHost:   r.Host,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"command":        spec.Command,
		"master_host":    spec.MasterHost,
		"master_token":   spec.MasterToken,
		"master_version": spec.MasterVersion,
		"master_channel": spec.MasterChannel,
		"expires_at":     spec.ExpiresAt,
		"portal_base":    spec.PortalBase,
		"script_url":     spec.ScriptURL,
		"style":          spec.Style,
	})
}

func (s *Servers) handleNodeInstallSSH(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if err := s.preInstallNodeLicenseCheck(ctx); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
		return
	}
	var req nodeInstallSSHRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("无效的JSON: %v", err)})
		return
	}
	req.SSHHost = strings.TrimSpace(req.SSHHost)
	req.SSHUser = strings.TrimSpace(req.SSHUser)
	req.SSHPassword = strings.TrimSpace(req.SSHPassword)
	if req.SSHHost == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SSH主机不能为空"})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.SSHPort <= 0 || req.SSHPort > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SSH端口必须在1-65535之间"})
		return
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}
	if req.SSHUser != "root" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SSH用户必须为root"})
		return
	}
	if req.SSHPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SSH密码不能为空"})
		return
	}

	installSpec, err := s.buildNodeInstallCommand(ctx, nodeInstallCommandRequest{
		PortalBase:  req.PortalBase,
		MasterHost:  req.MasterHost,
		TTLMinutes:  req.TTLMinutes,
		RequestHost: r.Host,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	logs := newNodeInstallLogBuffer(maxNodeInstallLogLines, installSpec.MasterToken)
	resp := nodeInstallSSHResponse{
		OK:         false,
		Status:     "failed",
		MasterHost: installSpec.MasterHost,
		ExpiresAt:  installSpec.ExpiresAt,
	}
	logs.Add(fmt.Sprintf("starting SSH connection: %s:%d", req.SSHHost, req.SSHPort))
	connectCtx, connectCancel := context.WithTimeout(ctx, nodeInstallSSHConnectTimeout)
	client, err := newNodeInstallSSHClient(connectCtx, req)
	connectCancel()
	if err != nil {
		logs.Add("SSH connection failed")
		logs.Add(err.Error())
		resp.Message = "SSH connection failed"
		resp.Logs = logs.Lines()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	defer client.Close()
	logs.Add("SSH connection established")
	if rc, ok := client.(*realNodeInstallSSHClient); ok && rc.hostKeyFingerprint != "" {
		logs.Add(fmt.Sprintf("remote host key fingerprint: %s", rc.hostKeyFingerprint))
	}

	remoteHostname, ok := runNodeInstallSSHCheck(ctx, client, logs, "hostname", "failed to read remote hostname", "remote hostname detected")
	if !ok {
		resp.Message = "failed to read remote hostname"
		resp.Logs = logs.Lines()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.RemoteHostname = firstNonEmptyLine(remoteHostname)

	if _, ok := runNodeInstallSSHCheck(ctx, client, logs, "command -v bash >/dev/null 2>&1", "remote host is missing bash", "bash detected"); !ok {
		resp.Message = "remote host is missing bash"
		resp.Logs = logs.Lines()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if _, ok := runNodeInstallSSHCheck(ctx, client, logs, "command -v curl >/dev/null 2>&1", "remote host is missing curl", "curl detected"); !ok {
		resp.Message = "remote host is missing curl"
		resp.Logs = logs.Lines()
		writeJSON(w, http.StatusOK, resp)
		return
	}

	logs.Add("starting remote install command")
	installCtx, installCancel := context.WithTimeout(ctx, nodeInstallSSHCommandTimeout)
	installResult, installErr := client.Run(installCtx, "bash -lc "+shellQuote(installSpec.Command))
	installCancel()
	logs.AddOutput(installResult.Combined)
	if installErr != nil {
		if errors.Is(installErr, context.DeadlineExceeded) || errors.Is(installCtx.Err(), context.DeadlineExceeded) {
			logs.Add("remote install timed out")
			resp.Message = "remote install timed out"
		} else {
			logs.Add("remote install command failed")
			logs.Add(installErr.Error())
			resp.Message = "remote install command failed"
		}
		resp.Logs = logs.Lines()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	logs.Add("remote install command completed")

	if resp.RemoteHostname != "" {
		node, err := s.waitForNodeByHostname(ctx, resp.RemoteHostname, nodeInstallRegisterWait)
		if err == nil && node != nil {
			resp.OK = true
			resp.Status = "installed"
			resp.Message = "install and registration completed"
			resp.Node = &nodeInstallSSHNodeSummary{
				ID:       node.ID,
				Hostname: node.Hostname,
				PublicIP: node.PublicIP,
				Status:   node.Status,
			}
			logs.Add("node registration detected")
			resp.Logs = logs.Lines()
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}

	resp.OK = true
	resp.Status = "installed_waiting_register"
	resp.Message = "install completed, waiting for node registration"
	logs.Add("install command succeeded but node registration is not visible yet")
	resp.Logs = logs.Lines()
	writeJSON(w, http.StatusOK, resp)
}

func runNodeInstallSSHCheck(ctx context.Context, client nodeInstallSSHClient, logs *nodeInstallLogBuffer, command, failedMessage, successMessage string) (string, bool) {
	result, err := client.Run(ctx, command)
	logs.AddOutput(result.Combined)
	if err != nil {
		logs.Add(failedMessage)
		return result.Combined, false
	}
	if strings.TrimSpace(successMessage) != "" {
		logs.Add(successMessage)
	}
	return result.Combined, true
}

func (s *Servers) waitForNodeByHostname(ctx context.Context, hostname string, wait time.Duration) (*store.Node, error) {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return nil, errors.New("hostname required")
	}
	if wait <= 0 {
		return nil, errors.New("wait duration must be positive")
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(nodeInstallRegisterPoll)
	defer ticker.Stop()

	check := func() (*store.Node, error) {
		storeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return s.store.GetNodeByHostname(storeCtx, hostname)
	}
	if node, err := check(); err == nil && node != nil {
		return node, nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("node registration wait timeout: hostname %q not found within %v", hostname, wait)
		case <-ticker.C:
			node, err := check()
			if err != nil {
				return nil, err
			}
			if node != nil {
				return node, nil
			}
		}
	}
}

// fingerprintCallback returns an ssh.HostKeyCallback that records the remote host key
// fingerprint into the provided pointer and always accepts the connection.
// This is used so the install log can include the fingerprint for the admin to verify
// after the fact.  Full known-hosts verification is not feasible here because the control
// plane typically connects to freshly provisioned servers with no prior host key on file.
func fingerprintCallback(out *string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		*out = ssh.FingerprintSHA256(key)
		return nil
	}
}

func dialNodeInstallSSHClient(ctx context.Context, target nodeInstallSSHRequest) (nodeInstallSSHClient, error) {
	address := net.JoinHostPort(strings.TrimSpace(target.SSHHost), strconv.Itoa(target.SSHPort))
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer conn.SetDeadline(time.Time{})
	}
	var hostKeyFingerprint string
	cfg := &ssh.ClientConfig{
		User:            target.SSHUser,
		Auth:            []ssh.AuthMethod{ssh.Password(target.SSHPassword)},
		HostKeyCallback: fingerprintCallback(&hostKeyFingerprint),
	}
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &realNodeInstallSSHClient{client: ssh.NewClient(clientConn, chans, reqs), hostKeyFingerprint: hostKeyFingerprint}, nil
}

func (c *realNodeInstallSSHClient) Run(ctx context.Context, command string) (sshRunResult, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return sshRunResult{}, err
	}
	defer session.Close()

	type runOutput struct {
		output []byte
		err    error
	}
	ch := make(chan runOutput, 1)
	go func() {
		out, err := session.CombinedOutput(command)
		ch <- runOutput{output: out, err: err}
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return sshRunResult{}, ctx.Err()
	case res := <-ch:
		result := sshRunResult{
			Combined: string(res.output),
		}
		if res.err == nil {
			return result, nil
		}
		var exitErr *ssh.ExitError
		if errors.As(res.err, &exitErr) {
			result.ExitStatus = exitErr.ExitStatus()
		} else {
			result.ExitStatus = -1
		}
		return result, res.err
	}
}

func (c *realNodeInstallSSHClient) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
