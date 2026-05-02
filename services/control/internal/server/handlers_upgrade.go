package server

// Upgrade control/node orchestration: UI stubs for surfacing current vs
// latest version, admin-triggered control-plane upgrade, per-node node
// upgrade workflow, task/log history. All tasks persist to the store
// asynchronously via saveUpgradeTask so the UI can resume progress after a
// control restart.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/buildinfo"
	"github.com/lingcdn/control/internal/store"
)

// nodeHeartbeatInterval mirrors the node-side heartbeat cadence
// (services/node/src/main.rs: `tokio::time::interval(Duration::from_secs(30))`).
// If you bump it there, bump it here.
const nodeHeartbeatInterval = 30 * time.Second

// nodeOnlineForUpgradeWindow is the heartbeat-freshness tolerance used when
// deciding whether a node is reachable for an upgrade dispatch. We require
// the last heartbeat to be within this window; older than that, we skip.
//
// Chosen as 4× the heartbeat interval (2 minutes). The old 90s value was
// only 3× the interval, which meant a single dropped heartbeat plus typical
// DB-write jitter was enough to trip the timeout and have the UI show
// "communication timeout" for a node the operator could see as online.
// 4× leaves room for one dropped heartbeat + one retry round.
const nodeOnlineForUpgradeWindow = 4 * nodeHeartbeatInterval

// isNodeOnlineForUpgrade returns true if the node is reachable enough for a
// heartbeat-dispatched upgrade command to be delivered.
//
// The authoritative source is the in-process hub: that's what powers the
// UI's "online" indicator. Before this fix, the upgrade code only looked at
// n.LastHeartbeat in the store — which is written by a warn-only code path
// (`UpdateNodeStatus` / `UpdateNodeHeartbeatInfo`). A transient DB slow
// write meant the hub showed online but the store was stale, and the
// operator saw "node in online but upgrade refused".
//
// The store row is still consulted as a fallback so nodes that registered
// on a different control instance (HA) are not unfairly skipped.
func (s *Servers) isNodeOnlineForUpgrade(n *store.Node) (bool, string) {
	if n == nil {
		return false, "node not found"
	}
	if strings.EqualFold(strings.TrimSpace(n.Status), "disabled") {
		return false, "node disabled"
	}
	// Primary signal: in-process hub session.
	if s.hub != nil {
		if sess, ok := s.hub.Get(n.ID); ok && sess != nil {
			if time.Since(sess.LastSeen) <= nodeOnlineForUpgradeWindow {
				return true, ""
			}
		}
	}
	// Fallback: DB-recorded heartbeat (for HA where the hub session lives
	// on a peer instance, or for a node whose session was just reaped).
	if n.LastHeartbeat.IsZero() {
		return false, "never connected"
	}
	if time.Since(n.LastHeartbeat) > nodeOnlineForUpgradeWindow {
		return false, "communication timeout"
	}
	return true, ""
}
func (s *Servers) handleUpgradeInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	ctx := r.Context()
	if getUserRole(ctx) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	channel := controlBuildChannel()
	if qch := strings.TrimSpace(r.URL.Query().Get("channel")); qch != "" {
		if normalized, err := normalizeUpgradeChannel(qch); err == nil {
			channel = normalized
		}
	}

	current := buildinfo.Version()

	info := upgradeInfo{
		CurrentVersion: current,
		LatestVersion:  current,
		Channel:        channel,
		Mode:           "portal",
		Notes:          []string{},
		CheckedAt:      time.Now(),
		Source:         "portal",
	}

	portal := s.upgradePortalBase()
	arch := normalizeArch(runtime.GOARCH)
	if arch == "" {
		arch = "amd64"
	}

	// Query control latest version from portal.
	if up, err := fetchPortalLatest(ctx, portal, "control", channel, "linux", arch, "latest"); err == nil {
		info.LatestVersion = strings.TrimSpace(up.Version)
		info.Checksum = strings.TrimSpace(up.Checksum)
		info.DownloadURL = strings.TrimSpace(up.DownloadURL)
		info.Changelog = strings.TrimSpace(up.Changelog)
		info.Signature = strings.TrimSpace(up.Signature)
		info.SigAlg = strings.TrimSpace(up.SigAlg)
		info.SigTarget = strings.TrimSpace(up.SigTarget)
		info.PubKey = strings.TrimSpace(up.PubKey)
	} else {
		info.Notes = append(info.Notes, fmt.Sprintf("failed to query control latest version: %v", err))
	}

	// Query node latest version per architecture.
	if up, err := fetchPortalLatest(ctx, portal, "node", channel, "linux", "amd64", "latest"); err == nil {
		info.NodeLatestAMD64 = strings.TrimSpace(up.Version)
	}
	if up, err := fetchPortalLatest(ctx, portal, "node", channel, "linux", "arm64", "latest"); err == nil {
		info.NodeLatestARM64 = strings.TrimSpace(up.Version)
	}
	if info.NodeLatestAMD64 != "" {
		info.NodeLatest = info.NodeLatestAMD64
	} else if info.NodeLatestARM64 != "" {
		info.NodeLatest = info.NodeLatestARM64
	}
	if info.NodeLatestAMD64 != "" && info.NodeLatestARM64 != "" && info.NodeLatestAMD64 != info.NodeLatestARM64 {
		info.Notes = append(info.Notes, fmt.Sprintf("note: node latest versions differ by arch (amd64=%s, arm64=%s)", info.NodeLatestAMD64, info.NodeLatestARM64))
	}

	if s.store != nil {
		nodes, err := s.store.ListNodes(ctx)
		if err == nil {
			target := strings.TrimSpace(info.NodeLatest)
			for _, n := range nodes {
				status := "unknown"
				switch {
				case strings.TrimSpace(n.Version) == "":
					status = "unknown"
				case target == "":
					// Node latest version unknown — assume current version is up to date
					status = "up_to_date"
				case strings.TrimSpace(n.Version) == target:
					status = "up_to_date"
				default:
					status = "upgrade_needed"
				}
				info.Nodes = append(info.Nodes, upgradeNode{
					ID:             n.ID,
					Hostname:       n.Hostname,
					CurrentVersion: n.Version,
					TargetVersion:  target,
					Status:         status,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, info)
}

func (s *Servers) handleUpgradeControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	var req struct {
		Channel string `json:"channel"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Determine upgrade channel:
	//   1. Caller-specified `channel` (if present and valid) — lets the UI
	//      offer an explicit "upgrade from beta" flow.
	//   2. Otherwise fall back to the compile-time build channel.
	//
	// Until this fix the request body was parsed but then thrown away —
	// the function always used controlBuildChannel(), which made the
	// UI's channel picker a no-op.
	channel := controlBuildChannel()
	if reqChannel := strings.TrimSpace(req.Channel); reqChannel != "" {
		if normalized, err := normalizeUpgradeChannel(reqChannel); err == nil {
			channel = normalized
		}
	}
	task := &upgradeTask{
		ID:            uuid.NewString(),
		Type:          "control",
		TargetVersion: "",
		Channel:       channel,
		Status:        "running",
		CreatedAt:     time.Now(),
	}
	s.saveUpgradeTask(r.Context(), task)

	go func() {
		ctx := context.Background()
		if err := s.performControlUpgrade(ctx, task); err != nil {
			s.appendUpgradeLog(ctx, task.ID, "ERROR", fmt.Sprintf("主控升级失败: %v", err), "")
			task.Status = "failed"
			s.saveUpgradeTask(ctx, task)
			return
		}
		task.Status = "completed"
		s.saveUpgradeTask(ctx, task)
	}()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"task_id": task.ID,
		"message": "control upgrade task created",
	})
}

func (s *Servers) handleUpgradeNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	var req struct {
		NodeIDs       []string `json:"node_ids"`
		TargetVersion string   `json:"target_version"`
		Force         bool     `json:"force"`
		Channel       string   `json:"channel"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	ctx := r.Context()
	targetVersion := strings.TrimSpace(req.TargetVersion)
	if targetVersion == "" {
		targetVersion = "latest"
	}
	channel := controlBuildChannel()

	// If caller requests "latest", try to resolve a concrete version from portal (best-effort).
	if strings.EqualFold(targetVersion, "latest") {
		portal := s.upgradePortalBase()
		if portal != "" {
			verAMD64 := ""
			verARM64 := ""
			if up, err := fetchPortalLatest(ctx, portal, "node", channel, "linux", "amd64", "latest"); err == nil {
				verAMD64 = strings.TrimSpace(up.Version)
			}
			if up, err := fetchPortalLatest(ctx, portal, "node", channel, "linux", "arm64", "latest"); err == nil {
				verARM64 = strings.TrimSpace(up.Version)
			}
			if verAMD64 != "" && verARM64 != "" && verAMD64 == verARM64 {
				targetVersion = verAMD64
			}
		}
	}

	nodeIDs := req.NodeIDs
	if len(nodeIDs) == 0 {
		if s.store == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "未指定节点"})
			return
		}
		nodes, err := s.store.ListNodes(ctx)
		if err != nil {
			writeInternalError(w, "list nodes", err)
			return
		}
		for _, n := range nodes {
			if n != nil && n.ID != "" {
				nodeIDs = append(nodeIDs, n.ID)
			}
		}
	}

	// Filter out nodes with communication issues (heartbeat timeout or disabled).
	// Upgrade commands are delivered via heartbeat response, so unreachable nodes
	// will never receive the command.
	//
	// Previously this block compared n.LastHeartbeat from the store against a
	// bare 90-second threshold. Two bugs flowed from that: (1) the store
	// write in Heartbeat is warn-only, so transient DB failures left the
	// store stale while the hub showed the node online, producing false
	// "communication timeout" skips the operator could not explain; (2) 90s
	// is only 3× the 30s heartbeat cadence, small enough for one dropped
	// heartbeat to trip it. isNodeOnlineForUpgrade now checks the hub first
	// (the same signal the UI uses) and widens the window to 4× heartbeat.
	var validIDs []string
	var skippedNodes []map[string]string
	for _, nid := range nodeIDs {
		n, err := s.store.GetNode(ctx, nid)
		if err != nil || n == nil {
			skippedNodes = append(skippedNodes, map[string]string{
				"id": nid, "reason": "node not found",
			})
			continue
		}
		if ok, reason := s.isNodeOnlineForUpgrade(n); !ok {
			skippedNodes = append(skippedNodes, map[string]string{
				"id": nid, "hostname": n.Hostname, "reason": reason,
			})
			continue
		}
		validIDs = append(validIDs, nid)
	}
	nodeIDs = validIDs

	task := &upgradeTask{
		ID:            uuid.NewString(),
		TargetVersion: targetVersion,
		Channel:       channel,
		NodeIDs:       nodeIDs,
		Status:        "running",
		Type:          "node",
		CreatedAt:     time.Now(),
	}
	s.saveUpgradeTask(ctx, task)
	s.appendUpgradeLog(ctx, task.ID, "INFO", "node upgrade task started (heartbeat-dispatched)", "")
	s.appendUpgradeLog(ctx, task.ID, "INFO", fmt.Sprintf("target version: %s", targetVersion), "")
	s.appendUpgradeLog(ctx, task.ID, "INFO", fmt.Sprintf("portal: %s (channel=%s)", s.upgradePortalBase(), channel), "")

	// Log skipped nodes.
	for _, skip := range skippedNodes {
		s.appendUpgradeLog(ctx, task.ID, "WARNING", fmt.Sprintf("skipped node %s (%s): %s", skip["id"], skip["hostname"], skip["reason"]), skip["id"])
	}

	if len(nodeIDs) == 0 {
		msg := "no reachable nodes to upgrade"
		if len(skippedNodes) > 0 {
			msg = fmt.Sprintf("all %d node(s) skipped due to communication issues", len(skippedNodes))
		}
		task.Status = "failed"
		s.saveUpgradeTask(ctx, task)
		s.appendUpgradeLog(ctx, task.ID, "ERROR", msg, "")
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            false,
			"scheduled_ids": nodeIDs,
			"skipped":       skippedNodes,
			"target":        targetVersion,
			"task_id":       task.ID,
			"message":       msg,
		})
		return
	}

	for _, nid := range nodeIDs {
		setNodeUpgradeCommand(nid, nodeUpgradeCommand{
			Type:          "upgrade",
			TaskID:        task.ID,
			TargetVersion: targetVersion,
			Channel:       channel,
			Force:         req.Force,
		})
		s.appendUpgradeLog(ctx, task.ID, "INFO", fmt.Sprintf("upgrade command queued: target=%s force=%v", targetVersion, req.Force), nid)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"scheduled_ids": nodeIDs,
		"skipped":       skippedNodes,
		"target":        targetVersion,
		"task_id":       task.ID,
		"message":       fmt.Sprintf("node upgrade task created (target=%s); commands will be delivered via heartbeat", targetVersion),
	})
}

// Upgrade task logs
func (s *Servers) handleUpgradeTaskLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/system/upgrade/tasks/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "任务ID不能为空"})
		return
	}
	nodeID := r.URL.Query().Get("node_id")
	logs := s.listUpgradeLogs(r.Context(), id, nodeID)
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

// List upgrade tasks (latest N)
func (s *Servers) handleUpgradeTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}
	tasks := s.listUpgradeTasks(r.Context(), 50)
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// upgrade task helpers (store-aware, fallback to memory map)
func (s *Servers) saveUpgradeTask(ctx context.Context, t *upgradeTask) {
	if s.store != nil {
		_ = s.store.CreateUpgradeTask(ctx, &store.UpgradeTask{
			ID:            t.ID,
			TargetVersion: t.TargetVersion,
			Channel:       t.Channel,
			NodeIDs:       t.NodeIDs,
			Status:        t.Status,
			Type:          t.Type,
			CreatedAt:     t.CreatedAt,
		})
	}
	storeUpgradeTask(t)
}

func (s *Servers) appendUpgradeLog(ctx context.Context, id, level, msg, nodeID string) {
	if s.store != nil {
		_ = s.store.AppendUpgradeLog(ctx, id, store.UpgradeLog{
			TaskID:    id,
			NodeID:    nodeID,
			Level:     level,
			Message:   msg,
			Timestamp: time.Now(),
		})
	}
	addUpgradeLog(&upgradeTask{ID: id}, level, msg, nodeID)
}

func (s *Servers) listUpgradeLogs(ctx context.Context, id, nodeID string) []upgradeLog {
	if s.store != nil {
		if logs, err := s.store.ListUpgradeLogs(ctx, id, nodeID, 200); err == nil && len(logs) > 0 {
			out := make([]upgradeLog, len(logs))
			for i, l := range logs {
				out[i] = upgradeLog{
					Timestamp: l.Timestamp,
					Level:     l.Level,
					Message:   l.Message,
					NodeID:    l.NodeID,
				}
			}
			return out
		}
	}
	return getUpgradeLogs(id, nodeID)
}

func (s *Servers) listUpgradeTasks(ctx context.Context, limit int) []upgradeTask {
	if s.store != nil {
		if tasks, err := s.store.ListUpgradeTasks(ctx, limit); err == nil && len(tasks) > 0 {
			out := make([]upgradeTask, len(tasks))
			for i, t := range tasks {
				out[i] = upgradeTask{
					ID:            t.ID,
					TargetVersion: t.TargetVersion,
					Channel:       t.Channel,
					NodeIDs:       t.NodeIDs,
					Status:        t.Status,
					Type:          t.Type,
					CreatedAt:     t.CreatedAt,
				}
			}
			return out
		}
	}
	taskMu.Lock()
	defer taskMu.Unlock()
	res := make([]upgradeTask, 0, len(taskRepo))
	for _, t := range taskRepo {
		res = append(res, *t)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].CreatedAt.After(res[j].CreatedAt) })
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res
}

// handleUpgradePrecheck surfaces the runtime privilege picture to the UI so
// operators can tell *before* clicking "升级" whether an online upgrade will
// succeed. It does not execute the upgrade itself.
//
// The failure the user hit on 1.0.6 ("请使用 root 用户执行此脚本") happens
// because `control_update.sh` bails on non-root. This endpoint reports the
// same signal the server would use when it actually runs the upgrade, so the
// UI can prompt the operator to fix privileges up front.
func (s *Servers) handleUpgradePrecheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}
	if getUserRole(r.Context()) != "admin" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "仅管理员可操作"})
		return
	}

	priv := choosePrivilegeEscalation()
	resp := map[string]any{
		"os":              runtime.GOOS,
		"euid":            os.Geteuid(),
		"privilege_mode":  priv.mode, // "root" | "sudo" | "none"
		"can_auto_elevate": priv.mode == "root" || priv.mode == "sudo",
	}
	switch priv.mode {
	case "root":
		resp["status"] = "ok"
		resp["message"] = "主控以 root 身份运行，在线升级可直接执行。"
	case "sudo":
		resp["status"] = "ok"
		resp["message"] = "主控不是 root，但检测到 sudo，将通过 sudo -n 提权执行升级脚本。若 sudo 未配置免密或未允许 /bin/bash 将在执行时报错。"
	default:
		resp["status"] = "blocked"
		resp["message"] = "主控不是 root 运行，系统也没有 sudo 命令。在线升级会被 control_update.sh 拒绝。请让 systemd 以 root 启动主控，或为运行用户安装并配置 passwordless sudo。"
	}
	if runtime.GOOS != "linux" {
		resp["status"] = "unsupported"
		resp["message"] = "在线升级仅支持 Linux 主控。"
	}
	writeJSON(w, http.StatusOK, resp)
}

