package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"github.com/lingcdn/control/internal/store"
)

const (
	defaultMonitorInterval   = 30 * time.Second
	maxMonitorConcurrency    = 32
	maxMonitorTimeoutSeconds = 60
)

func (s *Servers) nodeActiveMonitorLoop(ctx context.Context) {
	ticker := time.NewTicker(defaultMonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runNodeActiveMonitors(ctx)
		}
	}
}

func (s *Servers) runNodeActiveMonitors(ctx context.Context) {
	if s.store == nil {
		return
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msg("failed to list nodes for active monitor")
		return
	}

	sem := make(chan struct{}, maxMonitorConcurrency)
	var wg sync.WaitGroup

	for _, n := range nodes {
		if n == nil || !n.MonitorEnabled {
			continue
		}
		node := n
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.probeNode(ctx, node)
		}()
	}
	wg.Wait()
}

func (s *Servers) probeNode(ctx context.Context, n *store.Node) {
	host := strings.TrimSpace(n.PublicIP)
	if host == "" {
		host = strings.TrimSpace(n.Hostname)
	}

	timeout := n.MonitorTimeout
	if timeout <= 0 {
		timeout = 5
	}
	if timeout > maxMonitorTimeoutSeconds {
		timeout = maxMonitorTimeoutSeconds
	}
	timeoutDur := time.Duration(timeout) * time.Second

	proto := strings.ToLower(strings.TrimSpace(n.MonitorProtocol))
	if proto == "" {
		proto = "http"
	}

	// 未配置监控端口时，使用节点默认服务端口（80）
	port := n.MonitorPort
	if port <= 0 {
		port = 80
	}

	var err error
	start := time.Now()
	switch proto {
	case "http":
		err = httpProbe(ctx, host, port, timeoutDur)
	case "tcp":
		err = tcpProbe(ctx, host, port, timeoutDur)
	case "ping":
		err = pingProbe(ctx, host, timeoutDur)
	default:
		err = fmt.Errorf("unsupported protocol: %s", proto)
	}

	latencyMs := int(time.Since(start).Milliseconds())
	if latencyMs < 0 {
		latencyMs = 0
	}

	prevFailCount := n.MonitorFailCount
	failCount := prevFailCount
	ok := err == nil
	if ok {
		failCount = 0
	} else {
		failCount++
	}

	msg := ""
	if err != nil {
		msg = err.Error()
	}

	_ = s.store.UpdateNodeMonitorResult(ctx, n.ID, store.NodeMonitorResult{
		LastOK:        ok,
		LastError:     msg,
		LastAt:        time.Now(),
		LastLatencyMs: latencyMs,
		FailCount:     failCount,
	})

	// 当节点健康状态发生变化时，触发 DNS 同步和通知
	threshold := n.MonitorFailThreshold
	if threshold <= 0 {
		threshold = 3
	}
	wasDown := prevFailCount >= threshold
	isDown := failCount >= threshold
	if wasDown != isDown {
		if isDown {
			log.Info().Str("node", n.Hostname).Str("ip", n.PublicIP).Int("fail_count", failCount).Msg("node monitor: node down, triggering DNS sync")
			s.triggerDNSSync("", "monitor:node_down")
			// Send notification when node goes down. notifyNodeOffline is a
			// method on *Servers (always bound, never nil), so we call it
			// unconditionally — the previous `!= nil` guard was dead code
			// that the Go 1.25 compiler now flags as a build error.
			go s.notifyNodeOffline(n.Hostname)
		} else {
			log.Info().Str("node", n.Hostname).Str("ip", n.PublicIP).Msg("node monitor: node recovered, triggering DNS sync")
			s.triggerDNSSync("", "monitor:node_recovered")
		}
	}
}

func httpProbe(ctx context.Context, host string, port int, timeout time.Duration) error {
	if host == "" {
		return errors.New("missing node host")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	url := "http://" + addr + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.CopyN(io.Discard, resp.Body, 1)
	_ = resp.Body.Close()
	return nil
}

func tcpProbe(ctx context.Context, host string, port int, timeout time.Duration) error {
	if host == "" {
		return errors.New("missing node host")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func pingProbe(ctx context.Context, host string, timeout time.Duration) error {
	if host == "" {
		return errors.New("missing node host")
	}
	ip, err := resolveIP(ctx, host)
	if err != nil {
		return err
	}

	var network string
	var proto int
	var echoType icmp.Type
	var listenAddr string
	if ip.To4() != nil {
		network = "ip4:icmp"
		proto = 1
		echoType = ipv4.ICMPTypeEcho
		listenAddr = "0.0.0.0"
	} else {
		network = "ip6:ipv6-icmp"
		proto = 58
		echoType = ipv6.ICMPTypeEchoRequest
		listenAddr = "::"
	}

	c, err := icmp.ListenPacket(network, listenAddr)
	if err != nil {
		// 没有 CAP_NET_RAW 权限时，自动降级为 TCP 80 端口探测
		if strings.Contains(strings.ToLower(err.Error()), "operation not permitted") ||
			strings.Contains(strings.ToLower(err.Error()), "permission denied") ||
			strings.Contains(strings.ToLower(err.Error()), "socket:") {
			log.Warn().Str("host", host).Msg("ping 探测失败: 无 ICMP 权限，自动降级为 TCP 80 探测")
			return tcpProbe(ctx, host, 80, timeout)
		}
		return err
	}
	defer func() { _ = c.Close() }()

	id := os.Getpid() & 0xffff
	msg := icmp.Message{
		Type: echoType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  1,
			Data: []byte("lingcdn"),
		},
	}
	b, err := msg.Marshal(nil)
	if err != nil {
		return err
	}

	_ = c.SetDeadline(time.Now().Add(timeout))
	if _, err := c.WriteTo(b, &net.IPAddr{IP: ip}); err != nil {
		return err
	}

	reply := make([]byte, 1500)
	n, _, err := c.ReadFrom(reply)
	if err != nil {
		return err
	}
	rm, err := icmp.ParseMessage(proto, reply[:n])
	if err != nil {
		return err
	}

	switch rm.Type {
	case ipv4.ICMPTypeEchoReply, ipv6.ICMPTypeEchoReply:
		return nil
	default:
		return fmt.Errorf("unexpected icmp reply: %v", rm.Type)
	}
}

func resolveIP(ctx context.Context, host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return ip, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if addr.IP.To4() != nil {
			return addr.IP, nil
		}
	}
	if len(addrs) > 0 {
		return addrs[0].IP, nil
	}
	return nil, errors.New("no ip resolved")
}
