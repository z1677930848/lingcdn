package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/lingcdn/control/internal/compiler"
	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/ddos"
	"github.com/lingcdn/control/internal/nodehub"
	"github.com/lingcdn/control/internal/publisher"
	"github.com/lingcdn/control/internal/purge"
	"github.com/lingcdn/control/internal/store"
	controlpb "github.com/lingcdn/control/proto/gen"
)

type nodeControlServer struct {
	controlpb.UnimplementedNodeControlServer
	hub               *nodehub.Hub
	compiler          *compiler.Compiler
	publisher         *publisher.Publisher
	purge             *purge.Service
	store             store.Store
	xdpStore          *ddos.XdpStore
	nodeMonitor       *nodeMonitorRecorder
	notifyMonitor     func()
	dnsTrigger        func(subject, reason string)
	notifyNodeOffline func(string)
	cfg               config.Config
	licenseCheck      func(ctx context.Context, existing bool) (licenseState, bool, error)
	// Optional async writer for WAF ban persistence. When non-nil, heartbeat
	// handlers enqueue bans instead of writing them synchronously.
	wafBanQ *wafBanQueue
	// Reference to parent Servers for notification methods
	servers *Servers
}

func newNodeControlServer(hub *nodehub.Hub, compiler *compiler.Compiler, publisher *publisher.Publisher, purge *purge.Service, store store.Store, xdpStore *ddos.XdpStore, cfg config.Config, dnsTrigger func(subject, reason string), notifyNodeOffline func(string), licenseCheck func(ctx context.Context, existing bool) (licenseState, bool, error), nodeMonitor *nodeMonitorRecorder, notifyMonitor func(), servers *Servers) *nodeControlServer {
	return &nodeControlServer{
		hub:               hub,
		compiler:          compiler,
		publisher:         publisher,
		purge:             purge,
		store:             store,
		xdpStore:          xdpStore,
		cfg:               cfg,
		dnsTrigger:        dnsTrigger,
		notifyNodeOffline: notifyNodeOffline,
		licenseCheck:      licenseCheck,
		nodeMonitor:       nodeMonitor,
		notifyMonitor:     notifyMonitor,
		servers:           servers,
	}
}

func (s *nodeControlServer) RegisterNode(ctx context.Context, req *controlpb.RegisterNodeRequest) (*controlpb.RegisterNodeResponse, error) {
	publicIP := peerIP(ctx)

	// Admission checks (bootstrap token, license) still need the current row
	// to decide whether the caller is already-known. The subsequent write is
	// done atomically via RegisterOrRefreshNode, so any benign race on this
	// read is resolved in the DB (disabled takes precedence over refresh).
	existing, err := s.store.GetNodeByHostname(ctx, req.GetHostname())
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to check existing node")
		return nil, status.Error(codes.Internal, "failed to check existing node")
	}
	credential := strings.TrimSpace(req.GetBootstrapToken())
	if credential == "" {
		return nil, status.Error(codes.PermissionDenied, "bootstrap token required")
	}
	if err := s.authorizeNodeRegistration(ctx, existing, credential); err != nil {
		return nil, err
	}
	if s.licenseCheck != nil {
		lic, allowed, err := s.licenseCheck(ctx, existing != nil)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("license check failed")
			return nil, status.Error(codes.Internal, "license check failed")
		}
		if !allowed {
			return nil, status.Error(codes.PermissionDenied, stReason(lic, "license required"))
		}
	}

	// Generate a fresh token for every registration/re-registration. For brand
	// new nodes we also allocate a UUID; for re-registrations the DB keeps the
	// existing id via ON CONFLICT. We hash the token before persisting so the
	// raw token only exists on the wire back to the node.
	nodeToken := generateNodeToken()
	candidateID := uuid.NewString()
	if existing != nil {
		candidateID = existing.ID
	}
	now := time.Now()
	node := &store.Node{
		ID:           candidateID,
		Hostname:     req.GetHostname(),
		PublicIP:     publicIP,
		Version:      req.GetVersion(),
		Status:       "online",
		Region:       req.GetRegion(),
		Capabilities: req.GetCapabilities(),
		Token:        hashToken(nodeToken),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	nodeID, err := s.store.RegisterOrRefreshNode(ctx, node)
	if err != nil {
		if errors.Is(err, store.ErrNodeDisabled) {
			return nil, status.Error(codes.PermissionDenied, "node disabled by control plane")
		}
		log.Ctx(ctx).Error().Err(err).Msg("failed to register node")
		return nil, status.Error(codes.Internal, "failed to register node")
	}

	if existing != nil {
		log.Ctx(ctx).Info().
			Str("node_id", nodeID).
			Str("hostname", req.GetHostname()).
			Str("version", req.GetVersion()).
			Msg("node re-registered")
	} else {
		log.Ctx(ctx).Info().
			Str("node_id", nodeID).
			Str("hostname", req.GetHostname()).
			Strs("capabilities", req.GetCapabilities()).
			Msg("new node registered")
	}

	session := &nodehub.Session{
		NodeID:       nodeID,
		Hostname:     req.GetHostname(),
		Version:      req.GetVersion(),
		Capabilities: req.GetCapabilities(),
		Status:       "online",
	}
	s.hub.Add(ctx, session)
	if s.dnsTrigger != nil {
		s.dnsTrigger("", "node:register")
	}

	return &controlpb.RegisterNodeResponse{
		NodeId: nodeID,
		Token:  nodeToken,
	}, nil
}

func (s *nodeControlServer) authorizeNodeRegistration(ctx context.Context, existing *store.Node, credential string) error {
	if existing != nil {
		if existing.Token != "" && existing.Token == hashToken(credential) {
			return nil
		}
	}

	ok, err := s.store.ValidateBootstrapToken(ctx, credential)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("validate bootstrap token failed")
		return status.Error(codes.Internal, "validate bootstrap token failed")
	}
	if !ok {
		return status.Error(codes.PermissionDenied, "invalid bootstrap token")
	}
	return nil
}

func (s *nodeControlServer) Heartbeat(ctx context.Context, req *controlpb.HeartbeatRequest) (*controlpb.HeartbeatResponse, error) {
	nodeID := req.GetNodeId()
	if err := s.validateNodeToken(ctx, nodeID, req.GetToken()); err != nil {
		return nil, err
	}

	node, _ := s.store.GetNode(ctx, nodeID)
	if node != nil && strings.EqualFold(node.Status, "disabled") {
		return &controlpb.HeartbeatResponse{
			Ok:      false,
			Message: "node disabled by control plane",
		}, nil
	}

	var prevIP string
	if node != nil {
		prevIP = strings.TrimSpace(node.PublicIP)
	}

	s.hub.UpdateHeartbeat(nodeID, req.GetStatus(), req.GetVersion())
	if err := s.store.UpdateNodeStatus(ctx, nodeID, req.GetStatus(), req.GetVersion()); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("node_id", nodeID).Msg("failed to update node status in store")
	}

	// Check whether any running node-upgrade task can be marked completed
	// now that this node reported its version.
	checkNodeUpgradeTaskCompletion(nodeID, req.GetVersion(), func(nid string) string {
		if n, err := s.store.GetNode(ctx, nid); err == nil && n != nil {
			return n.Version
		}
		return ""
	}, func(taskID, newStatus string) {
		// Persist status-only change; must NOT use CreateUpgradeTask here because it
		// is an upsert that would clobber target_version/node_ids/channel/type when
		// only ID+Status are provided.
		if s.store != nil {
			if err := s.store.UpdateUpgradeTaskStatus(ctx, taskID, newStatus); err != nil {
				log.Ctx(ctx).Warn().Err(err).Str("task_id", taskID).Msg("failed to update upgrade task status")
			}
		}
	})

	if node != nil {
		newStatus := strings.ToLower(strings.TrimSpace(req.GetStatus()))
		if (newStatus == "offline" || newStatus == "stopped") && !strings.EqualFold(node.Status, newStatus) && !strings.EqualFold(node.Status, "disabled") {
			if s.notifyNodeOffline != nil {
				go s.notifyNodeOffline(node.Hostname)
			}
		}
	}

	if ip := peerIP(ctx); ip != "" {
		// Use narrow heartbeat interface (safer: can never clobber admin-managed columns).
		// Region is taken from the node's self-report only if the control plane has no GeoIP
		// resolver attached; otherwise fall back to the node's stored region (preventing
		// nodes from forging regions that affect traffic-map analytics).
		region := strings.TrimSpace(req.GetRegion())
		if region == "" && node != nil {
			region = node.Region
		}
		if err := s.store.UpdateNodeHeartbeatInfo(ctx, nodeID, ip, req.GetVersion(), region); err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("node_id", nodeID).Msg("failed to update node heartbeat info")
		}
		if prevIP == "" || prevIP != ip {
			if s.dnsTrigger != nil {
				s.dnsTrigger("", "node:ip_change")
			}
		}
	}

	// 处理节点上报的 WAF 拉黑 IP。如果挂了异步写队列，则入队非阻塞；
	// 否则降级为同步写（保留旧行为）。同步写的坏处是每个 ban 一次 DB 往返，
	// 心跳延迟会随 ban 数量线性增长，且可以被恶意节点滥用拖垮控制面。
	if bans := req.GetWafBans(); len(bans) > 0 {
		now := time.Now()
		for _, ban := range bans {
			if ban.GetIp() == "" {
				continue
			}
			expiresAt := time.Unix(ban.GetExpiresAtUnix(), 0)
			if expiresAt.Before(now) {
				continue // 已过期的不存储
			}
			wafBan := &store.WAFBan{
				IP:        ban.GetIp(),
				Reason:    ban.GetReason(),
				NodeID:    nodeID,
				ExpiresAt: expiresAt,
				CreatedAt: now,
			}
			if s.wafBanQ != nil {
				if !s.wafBanQ.enqueue(wafBan) {
					log.Ctx(ctx).Warn().Str("ip", ban.GetIp()).Msg("WAF ban queue full, dropping report")
				}
				continue
			}
			if err := s.store.CreateOrUpdateWAFBan(ctx, wafBan); err != nil {
				log.Ctx(ctx).Warn().Err(err).Str("ip", ban.GetIp()).Msg("failed to store WAF ban from node")
			}
		}
	}

	// 检查是否有手动设置的升级命令（at-least-once 投递）：
	// 我们在此处仅仅拿出命令并下发给节点，不在此删除。真正的清理发生在：
	// 1) checkNodeUpgradeTaskCompletion 检测到该节点版本达到目标版本后 ACK；
	// 2) 命令 TTL 过期后自动驱逐（getNodeUpgradeCommand 内实现）。
	// 这样即使节点重启/网络抖动丢失了一次心跳响应，下一次心跳仍会再次拿到命令。
	// 同样，如果节点已经是目标版本，它自己会识别并跳过重复命令。
	message := "ok"
	if cmd, ok := getNodeUpgradeCommand(nodeID); ok {
		message = cmd.message()
		log.Ctx(ctx).Info().
			Str("node_id", nodeID).
			Str("task_id", cmd.TaskID).
			Str("target_version", cmd.TargetVersion).
			Msg("dispatching upgrade command to node (awaiting ack via version convergence)")
	}

	return &controlpb.HeartbeatResponse{
		ConfigVersion: "",
		Message:       message,
		Ok:            true,
	}, nil
}

func (s *nodeControlServer) ReportWAFBan(ctx context.Context, req *controlpb.ReportWAFBanRequest) (*controlpb.ReportWAFBanResponse, error) {
	nodeID := req.GetNodeId()
	if err := s.validateNodeToken(ctx, nodeID, req.GetToken()); err != nil {
		return nil, err
	}

	ban := req.GetBan()
	if ban == nil || ban.GetIp() == "" {
		return &controlpb.ReportWAFBanResponse{
			Ok:      false,
			Message: "invalid ban data",
		}, nil
	}

	expiresAt := time.Unix(ban.GetExpiresAtUnix(), 0)
	if expiresAt.Before(time.Now()) {
		return &controlpb.ReportWAFBanResponse{
			Ok:      false,
			Message: "ban already expired",
		}, nil
	}

	// 检查是否已存在相同IP的拉黑记录
	existingBans, _ := s.store.ListWAFBans(ctx, 1000)
	var existingBan *store.WAFBan
	for _, b := range existingBans {
		if b.IP == ban.GetIp() {
			existingBan = b
			break
		}
	}

	// 如果已存在，合并处理：取更长的过期时间和更高的strikes
	wafBan := &store.WAFBan{
		IP:        ban.GetIp(),
		Reason:    ban.GetReason(),
		Strikes:   int(ban.GetStrikes()),
		NodeID:    nodeID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	if existingBan != nil {
		// 取更长的过期时间
		if existingBan.ExpiresAt.After(expiresAt) {
			wafBan.ExpiresAt = existingBan.ExpiresAt
		}
		// 取更高的strikes
		if existingBan.Strikes > wafBan.Strikes {
			wafBan.Strikes = existingBan.Strikes
		}
		wafBan.CreatedAt = existingBan.CreatedAt
	}

	if err := s.store.CreateOrUpdateWAFBan(ctx, wafBan); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("ip", ban.GetIp()).Msg("failed to store WAF ban")
		return &controlpb.ReportWAFBanResponse{
			Ok:      false,
			Message: fmt.Sprintf("failed to store ban for ip %s: %v", ban.GetIp(), err),
		}, nil
	}

	log.Ctx(ctx).Info().
		Str("node_id", nodeID).
		Str("ip", ban.GetIp()).
		Int32("strikes", ban.GetStrikes()).
		Msg("received WAF ban report from node")

	// 异步触发配置下发，将黑名单分发到所有节点
	go func() {
		pubCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.publisher.Publish(pubCtx, "", nil); err != nil {
			log.Ctx(pubCtx).Warn().Err(err).Msg("failed to publish config after WAF ban update")
		} else {
			log.Ctx(pubCtx).Info().Str("ip", ban.GetIp()).Msg("published WAF ban to all nodes")
		}
	}()

	return &controlpb.ReportWAFBanResponse{
		Ok:      true,
		Message: "ok",
	}, nil
}

func (s *nodeControlServer) StreamConfig(stream controlpb.NodeControl_StreamConfigServer) error {
	ctx := stream.Context()
	nodeID, err := s.getNodeIDFromMetadata(ctx)
	if err != nil {
		return err
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata, node-token is required")
	}
	toks := md.Get("node-token")
	if len(toks) == 0 || strings.TrimSpace(toks[0]) == "" {
		return status.Error(codes.Unauthenticated, "missing node-token in metadata")
	}
	if err := s.validateNodeToken(ctx, nodeID, toks[0]); err != nil {
		return err
	}
	if node, _ := s.store.GetNode(ctx, nodeID); node != nil && strings.EqualFold(node.Status, "disabled") {
		return status.Error(codes.PermissionDenied, "node disabled by control plane")
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.hub.SetConfigStream(nodeID, stream, cancel)
	defer func() {
		s.hub.Remove(nodeID)
		log.Ctx(ctx).Info().Str("node_id", nodeID).Msg("config stream ended")
	}()

	if env, err := s.publisher.BuildConfigEnvelope(ctx, ""); err == nil {
		if err := stream.Send(env); err != nil {
			return status.Errorf(codes.Internal, "send initial config: %v", err)
		}
		log.Ctx(ctx).Info().Str("version", env.Version).Msg("sent initial config")
	}

	for {
		select {
		case <-streamCtx.Done():
			return nil
		default:
		}

		ack, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		s.hub.UpdateHeartbeat(nodeID, "online", ack.GetVersion())
		if err := s.store.UpdateNodeStatus(ctx, nodeID, "online", ack.GetVersion()); err != nil {
			log.Ctx(ctx).Warn().Err(err).Msg("failed to update node config version")
		}

		log.Ctx(ctx).Info().
			Str("node_id", nodeID).
			Str("version", ack.GetVersion()).
			Bool("ok", ack.GetOk()).
			Str("reason", ack.GetReason()).
			Msg("config ack received")

		if !ack.GetOk() {
			log.Ctx(ctx).Error().
				Str("node_id", nodeID).
				Str("version", ack.GetVersion()).
				Str("reason", ack.GetReason()).
				Msg("node rejected config")
		}
	}
}

func (s *nodeControlServer) StreamPurge(stream controlpb.NodeControl_StreamPurgeServer) error {
	ctx := stream.Context()
	nodeID, err := s.getNodeIDFromMetadata(ctx)
	if err != nil {
		return err
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.InvalidArgument, "missing metadata")
	}
	nodeTokens := md.Get("node-token")
	if len(nodeTokens) == 0 {
		return status.Error(codes.InvalidArgument, "missing node-token in metadata")
	}
	if err := s.validateNodeToken(ctx, nodeID, nodeTokens[0]); err != nil {
		return err
	}
	if node, _ := s.store.GetNode(ctx, nodeID); node != nil && strings.EqualFold(node.Status, "disabled") {
		return status.Error(codes.PermissionDenied, "node disabled by control plane")
	}

	s.hub.SetPurgeStream(nodeID, stream)
	defer func() {
		s.hub.SetPurgeStream(nodeID, nil)
		log.Ctx(ctx).Info().Str("node_id", nodeID).Msg("purge stream ended")
	}()

	for {
		res, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		log.Ctx(ctx).Info().
			Str("node_id", nodeID).
			Str("request_id", res.GetRequestId()).
			Bool("ok", res.GetOk()).
			Str("reason", res.GetReason()).
			Msg("purge result received from node")
		s.purge.ReportNodeResult(nodeID, res)
	}
}

func (s *nodeControlServer) ReportMetrics(ctx context.Context, req *controlpb.MetricsBatch) (*controlpb.HeartbeatResponse, error) {
	nodeID, err := s.getNodeIDFromMetadata(ctx)
	if err != nil {
		return nil, err
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "missing metadata")
	}
	nodeTokens := md.Get("node-token")
	if len(nodeTokens) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing node-token in metadata")
	}
	if err := s.validateNodeToken(ctx, nodeID, nodeTokens[0]); err != nil {
		return nil, err
	}

	if s.xdpStore != nil {
		stats := map[string]uint64{}
		var enabled *bool
		iface := ""
		latestAt := time.Time{}

		for _, m := range req.GetMetrics() {
			name := strings.TrimSpace(m.GetName())
			if name == "" {
				continue
			}
			if ts := m.GetTimestampMs(); ts > 0 {
				at := time.UnixMilli(ts)
				if at.After(latestAt) {
					latestAt = at
				}
			}
			if lbl := m.GetLabels(); lbl != nil && iface == "" {
				if v, ok := lbl["iface"]; ok && strings.TrimSpace(v) != "" {
					iface = strings.TrimSpace(v)
				}
			}
			if name == "xdp_enabled" {
				v := m.GetValue() >= 0.5
				enabled = &v
				continue
			}
			if strings.HasPrefix(name, "xdp_") {
				key := strings.TrimPrefix(name, "xdp_")
				if key == "" {
					continue
				}
				stats[key] = uint64(m.GetValue())
			}
		}

		if enabled != nil || len(stats) > 0 {
			s.xdpStore.Update(nodeID, iface, enabled, stats, latestAt)
		}
	}

	val := func(name string) float64 {
		for _, m := range req.GetMetrics() {
			if strings.TrimSpace(m.GetName()) == name {
				return m.GetValue()
			}
		}
		return 0
	}

	tele := store.NodeTelemetry{
		CPUUsage:       val("cpu_usage_pct"),
		MemUsage:       val("mem_usage_pct"),
		DiskUsage:      val("disk_usage_pct"),
		CPUCount:       int32(val("cpu_count")),
		MemTotal:       int64(val("mem_total_bytes")),
		DiskTotal:      int64(val("disk_total_bytes")),
		BytesSent:      int64(val("bytes_sent")),
		BytesReceived:  int64(val("bytes_received")),
		TCPEstablished: int32(val("tcp_established")),
		TCPSynRecv:     int32(val("tcp_syn_recv")),
		TCPTimeWait:    int32(val("tcp_time_wait")),
		NginxRunning:   val("nginx_running") >= 0.5,
	}
	if err := s.store.UpdateNodeTelemetry(ctx, nodeID, tele); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("node_id", nodeID).Msg("failed to update node telemetry")
	}

	if s.nodeMonitor != nil {
		s.nodeMonitor.add(nodeID, nodeMetricPoint{
			At:             time.Now(),
			BytesSent:      tele.BytesSent,
			BytesReceived:  tele.BytesReceived,
			CPUUsage:       tele.CPUUsage,
			MemUsage:       tele.MemUsage,
			DiskUsage:      tele.DiskUsage,
			TCPEstablished: tele.TCPEstablished,
		})
	}
	if s.notifyMonitor != nil {
		s.notifyMonitor()
	}

	// Check resource thresholds and send notifications
	if s.servers != nil {
		go s.servers.checkResourceThresholds(ctx, nodeID, tele)
	}

	log.Ctx(ctx).Debug().Str("node_id", nodeID).Int("metrics", len(req.GetMetrics())).Msg("metrics batch received")
	return &controlpb.HeartbeatResponse{
		ConfigVersion: "",
		Message:       "metrics received",
		Ok:            true,
	}, nil
}

// ReportLogs is retired: nodes ship error/warn logs via the local
// error.log + Filebeat → Elasticsearch path. Returning Unimplemented
// keeps the proto contract intact (no need to regenerate stubs) and
// signals to any straggling old-version node that it should stop
// trying to upload logs over gRPC.
func (s *nodeControlServer) ReportLogs(ctx context.Context, req *controlpb.LogsBatch) (*controlpb.HeartbeatResponse, error) {
	_ = req
	return nil, status.Error(codes.Unimplemented, "node-side log shipping was removed; install Filebeat on the node to forward error.log to Elasticsearch")
}

func (s *nodeControlServer) Purge(ctx context.Context, req *controlpb.PurgeCommand) (*controlpb.PurgeResult, error) {
	log.Ctx(ctx).Info().
		Str("request_id", req.GetRequestId()).
		Int("urls", len(req.GetUrls())).
		Msg("purge command received from node")

	return &controlpb.PurgeResult{
		RequestId: req.GetRequestId(),
		Ok:        true,
		Reason:    "acknowledged",
	}, nil
}

func (s *nodeControlServer) Ping(ctx context.Context, req *controlpb.NodePingRequest) (*controlpb.NodePingResponse, error) {
	return &controlpb.NodePingResponse{
		Ok:      true,
		Message: "lingcdn-control node service",
	}, nil
}

// RequestCertificate 接收节点 CSR，签发短期证书（自带 CA 链），并保存到证书表。
func (s *nodeControlServer) RequestCertificate(ctx context.Context, req *controlpb.CertificateRequest) (*controlpb.CertificateResponse, error) {
	// 节点身份校验
	nodeID := strings.TrimSpace(req.GetNodeId())
	token := strings.TrimSpace(req.GetToken())
	if nodeID == "" || token == "" {
		return &controlpb.CertificateResponse{Ok: false, Reason: "node_id/token required"}, nil
	}
	if err := s.validateNodeToken(ctx, nodeID, token); err != nil {
		return nil, err
	}

	domain := strings.TrimSpace(req.GetDomain())
	csrPEM := strings.TrimSpace(req.GetCsrPem())
	if domain == "" || csrPEM == "" {
		return &controlpb.CertificateResponse{Ok: false, Reason: "domain/csr required"}, nil
	}

	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil || !strings.Contains(block.Type, "CERTIFICATE REQUEST") {
		return &controlpb.CertificateResponse{Ok: false, Reason: "invalid csr pem"}, nil
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return &controlpb.CertificateResponse{Ok: false, Reason: fmt.Sprintf("parse csr: %v", err)}, nil
	}
	if err := csr.CheckSignature(); err != nil {
		return &controlpb.CertificateResponse{Ok: false, Reason: fmt.Sprintf("csr signature invalid: %v", err)}, nil
	}

	now := time.Now().UTC()

	caCert, caKey, caPEM, err := s.loadOrCreateCA(now)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("load ca: %v", err))
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		serial = big.NewInt(now.UnixNano())
	}

	// 安全策略：始终以 req.Domain 为准，忽略 CSR 中的 SAN。
	// 如果 CSR 自带了 SAN，则要求 req.Domain 必须在其中，否则拒绝。
	if len(csr.DNSNames) > 0 {
		found := false
		for _, san := range csr.DNSNames {
			if strings.EqualFold(san, domain) {
				found = true
				break
			}
		}
		if !found {
			return &controlpb.CertificateResponse{
				Ok:     false,
				Reason: fmt.Sprintf("requested domain %q not found in CSR SANs %v", domain, csr.DNSNames),
			}, nil
		}
	}
	dnsNames := []string{domain}

	leafTmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               csr.Subject,
		DNSNames:              dnsNames,
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, csr.PublicKey, caKey)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("sign cert: %v", err))
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})

	if s.store != nil {
		record := &store.Certificate{
			Name:      domain,
			Domain:    domain,
			Type:      "self-signed",
			AutoRenew: false,
			Status:    "active",
			CertPEM:   certPEM,
			KeyPEM:    nil,
			ExpiresAt: leafTmpl.NotAfter,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.store.CreateCertificate(ctx, record); err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("domain", domain).Msg("create self-signed certificate record failed")
		}
	}

	return &controlpb.CertificateResponse{
		Ok:       true,
		CertPem:  string(certPEM),
		KeyPem:   "",
		ChainPem: string(caPEM),
		Reason:   "",
	}, nil
}

// GetCertificate allows nodes to fetch certificate material by cert_id on-demand.
// This is used to avoid embedding thousands of cert/key PEM blobs into the node runtime config.
func (s *nodeControlServer) GetCertificate(ctx context.Context, req *controlpb.GetCertificateRequest) (*controlpb.GetCertificateResponse, error) {
	certID := strings.TrimSpace(req.GetCertId())
	if certID == "" {
		return &controlpb.GetCertificateResponse{Ok: false, Reason: "cert_id required"}, nil
	}

	nodeID := strings.TrimSpace(req.GetNodeId())
	tok := strings.TrimSpace(req.GetToken())
	if nodeID == "" || tok == "" {
		return &controlpb.GetCertificateResponse{Ok: false, CertId: certID, Reason: "node_id/token required"}, nil
	}
	if err := s.validateNodeToken(ctx, nodeID, tok); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, status.Error(codes.Internal, "store unavailable")
	}

	certIDInt, parseErr := strconv.ParseInt(certID, 10, 64)
	if parseErr != nil {
		return &controlpb.GetCertificateResponse{Ok: false, CertId: certID, Reason: "invalid cert_id"}, nil
	}

	cert, err := s.store.GetCertificate(ctx, certIDInt)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("get certificate: %v", err))
	}
	if cert == nil {
		return &controlpb.GetCertificateResponse{Ok: false, CertId: certID, Reason: "certificate not found"}, nil
	}
	certIDResp := strconv.FormatInt(cert.ID, 10)
	if len(cert.CertPEM) == 0 {
		return &controlpb.GetCertificateResponse{
			Ok:     false,
			CertId: certIDResp,
			Domain: cert.Domain,
			Reason: "certificate material incomplete (missing cert_pem)",
		}, nil
	}

	// CSR 签发的证书只有 CertPEM、没有 KeyPEM（私钥在节点侧持有），
	// 此时仍然返回成功，让节点用本地私钥配对。
	return &controlpb.GetCertificateResponse{
		Ok:      true,
		CertId:  certIDResp,
		Domain:  cert.Domain,
		CertPem: string(cert.CertPEM),
		KeyPem:  string(cert.KeyPEM), // CSR 证书时为空，节点需用本地私钥
		Reason:  "",
	}, nil
}

// loadOrCreateCA 尝试从配置指定文件加载 CA，不存在时生成并落盘（若配置了路径）。
func (s *nodeControlServer) loadOrCreateCA(now time.Time) (*x509.Certificate, *rsa.PrivateKey, []byte, error) {
	certPath := strings.TrimSpace(s.cfg.CACertFile)
	keyPath := strings.TrimSpace(s.cfg.CAKeyFile)

	if certPath != "" && keyPath != "" {
		caCert, caKey, caPEM, err := loadCAFromFiles(certPath, keyPath)
		if err == nil {
			return caCert, caKey, caPEM, nil
		}
		log.Warn().Err(err).Msg("load CA failed, will regenerate")
	}

	// Generate new CA
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate ca key: %w", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject: pkix.Name{
			CommonName:   "LingCDN Local CA",
			Organization: []string{"LingCDN"},
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("sign ca: %w", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse ca: %w", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	// Persist if path configured
	if certPath != "" && keyPath != "" {
		if err := os.MkdirAll(dirOf(certPath), 0o755); err != nil {
			log.Warn().Err(err).Msg("create CA cert dir failed")
		} else {
			_ = os.WriteFile(certPath, caPEM, 0o640)
		}
		if err := os.MkdirAll(dirOf(keyPath), 0o750); err != nil {
			log.Warn().Err(err).Msg("create CA key dir failed")
		} else {
			keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caKey)})
			_ = os.WriteFile(keyPath, keyPEM, 0o600)
		}
	}

	return caCert, caKey, caPEM, nil
}

func loadCAFromFiles(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, []byte, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read CA cert from %s: %w", certPath, err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read CA key from %s: %w", keyPath, err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, nil, fmt.Errorf("decode cert PEM from %s: no valid PEM block found", certPath)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse CA certificate from %s: %w", certPath, err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, nil, fmt.Errorf("decode key PEM from %s: no valid PEM block found", keyPath)
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse CA private key from %s: %w", keyPath, err)
	}

	return cert, key, certPEM, nil
}

func dirOf(p string) string {
	if p == "" {
		return "."
	}
	if idx := strings.LastIndexAny(p, "/\\"); idx >= 0 {
		return p[:idx]
	}
	return "."
}

// Helper functions

func (s *nodeControlServer) validateNodeToken(ctx context.Context, nodeID, token string) error {
	if nodeID == "" {
		return status.Error(codes.InvalidArgument, "node_id is required")
	}
	if token == "" {
		return status.Error(codes.Unauthenticated, "node token is required")
	}

	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("node_id", nodeID).Msg("failed to get node for token validation")
		return status.Errorf(codes.Internal, "failed to get node %s: %v", nodeID, err)
	}
	if node == nil {
		return status.Errorf(codes.NotFound, "node %s not found", nodeID)
	}

	if node.Token == "" {
		// 节点在数据库中没有存储 token，这是异常状态（历史遗留或数据损坏），
		// 拒绝鉴权并记录告警，防止空 token 节点被永久免鉴权。
		log.Ctx(ctx).Warn().Str("node_id", nodeID).Msg("node has no stored token, rejecting authentication — re-register required")
		return status.Errorf(codes.Unauthenticated, "node %s has no stored token, re-registration required", nodeID)
	}

	if node.Token != hashToken(token) {
		return status.Errorf(codes.Unauthenticated, "invalid node token for node %s", nodeID)
	}

	return nil
}

func (s *nodeControlServer) getNodeIDFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.InvalidArgument, "missing metadata")
	}

	nodeIDs := md.Get("node-id")
	if len(nodeIDs) == 0 {
		return "", status.Error(codes.InvalidArgument, "missing node-id in metadata")
	}

	return nodeIDs[0], nil
}

func generateNodeToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func peerIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p == nil || p.Addr == nil {
		return ""
	}
	host := p.Addr.String()
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	return ip.String()
}
