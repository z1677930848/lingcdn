package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

const nodeCommandPrefix = "lingcdn:cmd:"

// nodeUpgradeCommandTTL caps how long a queued upgrade command is retained
// waiting for ACK. This prevents a permanently offline / wiped node from
// holding an upgrade command indefinitely.
const nodeUpgradeCommandTTL = nodeUpgradeTimeout

type nodeUpgradeCommand struct {
	Type          string `json:"type"` // "upgrade"
	TaskID        string `json:"task_id"`
	TargetVersion string `json:"target_version"`
	Channel       string `json:"channel,omitempty"`
	Force         bool   `json:"force"`
}

func (c nodeUpgradeCommand) message() string {
	b, _ := json.Marshal(c)
	return nodeCommandPrefix + string(b)
}

// nodeUpgradeCommandState wraps a pending command with dispatch bookkeeping so
// we can implement at-least-once delivery with an ACK-based clear.
type nodeUpgradeCommandState struct {
	cmd            nodeUpgradeCommand
	queuedAt       time.Time // when the command was set
	lastDispatched time.Time // last heartbeat where we handed it to the node
	dispatchCount  int       // how many times it was included in a heartbeat response
}

var (
	nodeUpgradeMu     sync.Mutex
	nodeUpgradeByNode = make(map[string]*nodeUpgradeCommandState) // key: node_id
)

func setNodeUpgradeCommand(nodeID string, cmd nodeUpgradeCommand) {
	nodeUpgradeMu.Lock()
	defer nodeUpgradeMu.Unlock()
	nodeUpgradeByNode[nodeID] = &nodeUpgradeCommandState{
		cmd:      cmd,
		queuedAt: time.Now(),
	}
}

// getNodeUpgradeCommand returns the pending command (if any) for the node and
// marks it as dispatched. The command is NOT removed: subsequent heartbeats
// will re-receive it until ACK'd by ackNodeUpgradeCommand or its TTL elapses.
// Commands past the TTL are evicted and treated as "no command".
func getNodeUpgradeCommand(nodeID string) (nodeUpgradeCommand, bool) {
	nodeUpgradeMu.Lock()
	defer nodeUpgradeMu.Unlock()
	st, ok := nodeUpgradeByNode[nodeID]
	if !ok || st == nil {
		return nodeUpgradeCommand{}, false
	}
	if time.Since(st.queuedAt) > nodeUpgradeCommandTTL {
		delete(nodeUpgradeByNode, nodeID)
		return nodeUpgradeCommand{}, false
	}
	st.lastDispatched = time.Now()
	st.dispatchCount++
	return st.cmd, true
}

// ackNodeUpgradeCommand clears a pending upgrade command after the node has
// confirmed completion (e.g. a heartbeat reporting the target version). A
// matching taskID is required to avoid a stale ACK clearing a newer command.
func ackNodeUpgradeCommand(nodeID, taskID string) {
	nodeUpgradeMu.Lock()
	defer nodeUpgradeMu.Unlock()
	if st, ok := nodeUpgradeByNode[nodeID]; ok && st != nil {
		if taskID == "" || st.cmd.TaskID == taskID {
			delete(nodeUpgradeByNode, nodeID)
		}
	}
}

// clearNodeUpgradeCommand is retained for admin paths that cancel a queued
// command (e.g. operator aborts an upgrade). It unconditionally removes the
// pending command when taskID matches (or when empty, which forces removal).
func clearNodeUpgradeCommand(nodeID, taskID string) {
	ackNodeUpgradeCommand(nodeID, taskID)
}

// rehydrateNodeUpgradeCommands rebuilds the in-memory command map from
// persisted upgrade_tasks rows whose status is still "running". This is
// called once at control-plane startup (see Serve).
//
// Rationale: before this function existed, the in-memory map was the sole
// home for pending commands. A control-plane restart (crash, redeploy,
// OOM) between "operator clicks upgrade" and "node heartbeats in" dropped
// the command on the floor — the upgrade task row stayed in "running"
// for 30 minutes until the task timeout fired, and the operator never
// knew why.
//
// We derive each command's fields from the already-persisted task row, so
// the rehydrated command is byte-identical to what handleUpgradeNodes
// originally queued. `queuedAt` is set to time.Now() so the TTL window
// restarts at the new process; this is slightly more lenient than strict
// persistence but strictly better than dropping the command entirely.
//
// Tasks older than nodeUpgradeTimeout are skipped: they were already going
// to be marked "failed" by the timeout sweep, so re-dispatching is pointless.
func rehydrateNodeUpgradeCommands(ctx context.Context, s store.Store) {
	if s == nil {
		return
	}
	// Pull the recent upgrade task window. 200 is a conservative upper
	// bound: on a very active control plane this covers ~24h of activity,
	// which is far longer than any task could still be legitimately
	// running (30-minute timeout).
	tasks, err := s.ListUpgradeTasks(ctx, 200)
	if err != nil {
		log.Warn().Err(err).Msg("rehydrate node upgrade commands: failed to list tasks")
		return
	}
	now := time.Now()
	restored := 0
	for _, t := range tasks {
		if t == nil || strings.ToLower(strings.TrimSpace(t.Type)) != "node" {
			continue
		}
		if strings.ToLower(strings.TrimSpace(t.Status)) != "running" {
			continue
		}
		// Too old → would be failed by the timeout sweep anyway.
		if now.Sub(t.CreatedAt) > nodeUpgradeTimeout {
			continue
		}
		cmd := nodeUpgradeCommand{
			Type:          "upgrade",
			TaskID:        t.ID,
			TargetVersion: t.TargetVersion,
			Channel:       t.Channel,
		}
		nodeUpgradeMu.Lock()
		for _, nid := range t.NodeIDs {
			if strings.TrimSpace(nid) == "" {
				continue
			}
			// Don't clobber a command the operator just queued post-restart:
			// if a node already has a pending command, leave it alone.
			if _, exists := nodeUpgradeByNode[nid]; exists {
				continue
			}
			nodeUpgradeByNode[nid] = &nodeUpgradeCommandState{
				cmd:      cmd,
				queuedAt: now,
			}
			restored++
		}
		nodeUpgradeMu.Unlock()
	}
	if restored > 0 {
		log.Info().
			Int("restored", restored).
			Msg("rehydrated pending node upgrade commands after restart")
	}
}
