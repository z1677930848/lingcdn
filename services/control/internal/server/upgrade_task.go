package server

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// nodeUpgradeTimeout is the maximum duration a node-upgrade task can stay
// in "running" state before it is automatically marked as failed/timed-out.
const nodeUpgradeTimeout = 30 * time.Minute

// upgradeTaskRepoCap bounds how many upgrade tasks are retained in memory.
// When exceeded, oldest non-running tasks are evicted; if all remaining tasks
// are still running we keep them (they must finish on their own).
const upgradeTaskRepoCap = 1000

// upgradeTaskRetention is how long a finished upgrade task stays in the
// in-memory repo before evictOldUpgradeTasksLocked becomes willing to drop
// it. Large enough that an operator refreshing the UI after a 24h deploy
// still sees history; small enough that a long-lived control plane doesn't
// grow its task cache unboundedly (combined with upgradeTaskRepoCap as a
// hard ceiling).
const upgradeTaskRetention = 7 * 24 * time.Hour

// upgradeTaskSweepInterval is how often the background sweeper scans for
// timed-out node upgrade tasks when no heartbeat has come in to trigger
// the check. Must be much smaller than nodeUpgradeTimeout so a task can't
// sit in "running" for much longer than its budget when all target nodes
// are offline.
const upgradeTaskSweepInterval = 60 * time.Second

// upgradeTaskSweeper periodically forces a timeout scan over running
// node-upgrade tasks. This complements checkNodeUpgradeTaskCompletion,
// which only runs on a heartbeat. Before the sweeper existed, if every
// target node went offline right after an upgrade was queued, the task
// would stay "running" indefinitely — nothing ever pushed it past the
// timeout — and the UI would be stuck showing an unfinished job.
//
// The sweeper makes progress by feeding a sentinel call into
// checkNodeUpgradeTaskCompletion. An empty nodeID + empty reportedVersion
// still exercises the timeout branch for every "running" task, because
// that branch is guarded only by `now.Sub(t.CreatedAt) > nodeUpgradeTimeout`
// — it does not require the nodeID to be part of the task.
func (s *Servers) upgradeTaskSweeper(ctx context.Context) {
	ticker := time.NewTicker(upgradeTaskSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepUpgradeTasksOnce(ctx)
		}
	}
}

func (s *Servers) sweepUpgradeTasksOnce(ctx context.Context) {
	checkNodeUpgradeTaskCompletion("", "",
		func(nid string) string {
			if s.store == nil {
				return ""
			}
			n, err := s.store.GetNode(ctx, nid)
			if err != nil || n == nil {
				return ""
			}
			return n.Version
		},
		func(taskID, newStatus string) {
			if s.store == nil {
				return
			}
			if err := s.store.UpdateUpgradeTaskStatus(ctx, taskID, newStatus); err != nil {
				log.Warn().Err(err).
					Str("task_id", taskID).
					Msg("sweep: failed to persist upgrade task status")
			}
		},
	)
}




var (
	taskMu   sync.Mutex
	taskRepo = make(map[string]*upgradeTask)
)

// evictOldUpgradeTasksLocked trims taskRepo to stay within upgradeTaskRepoCap.
// Caller must hold taskMu.
func evictOldUpgradeTasksLocked() {
	if len(taskRepo) <= upgradeTaskRepoCap {
		return
	}
	// Drop finished tasks older than the retention window first.
	cutoff := time.Now().Add(-upgradeTaskRetention)
	type candidate struct {
		id      string
		created time.Time
	}
	var cand []candidate
	for id, t := range taskRepo {
		if t == nil {
			delete(taskRepo, id)
			continue
		}
		// Never evict running tasks.
		if t.Status == "running" || t.Status == "pending" {
			continue
		}
		cand = append(cand, candidate{id: id, created: t.CreatedAt})
	}
	// Evict anything beyond retention window first.
	for _, c := range cand {
		if c.created.Before(cutoff) {
			delete(taskRepo, c.id)
		}
	}
	if len(taskRepo) <= upgradeTaskRepoCap {
		return
	}
	// Still over cap: evict the oldest finished tasks by created_at.
	cand = cand[:0]
	for id, t := range taskRepo {
		if t == nil || t.Status == "running" || t.Status == "pending" {
			continue
		}
		cand = append(cand, candidate{id: id, created: t.CreatedAt})
	}
	sort.Slice(cand, func(i, j int) bool { return cand[i].created.Before(cand[j].created) })
	excess := len(taskRepo) - upgradeTaskRepoCap
	for i := 0; i < excess && i < len(cand); i++ {
		delete(taskRepo, cand[i].id)
	}
}

func storeUpgradeTask(t *upgradeTask) {
	taskMu.Lock()
	defer taskMu.Unlock()
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.Type == "" {
		t.Type = "node"
	}
	taskRepo[t.ID] = t
	evictOldUpgradeTasksLocked()
}

func addUpgradeLog(t *upgradeTask, level, msg, nodeID string) {
	taskMu.Lock()
	defer taskMu.Unlock()
	// ensure exists
	if exist, ok := taskRepo[t.ID]; ok {
		t = exist
	} else {
		taskRepo[t.ID] = t
	}
	t.Logs = append(t.Logs, upgradeLog{
		Timestamp: time.Now(),
		Level:     strings.ToUpper(level),
		Message:   msg,
		NodeID:    nodeID,
	})
}

func getUpgradeLogs(id string, nodeID string) []upgradeLog {
	taskMu.Lock()
	defer taskMu.Unlock()
	t, ok := taskRepo[id]
	if !ok {
		return nil
	}
	if nodeID == "" {
		return t.Logs
	}
	var res []upgradeLog
	for _, l := range t.Logs {
		if l.NodeID == "" || l.NodeID == nodeID {
			res = append(res, l)
		}
	}
	return res
}

// checkNodeUpgradeTaskCompletion is called from the Heartbeat handler when a
// node reports its version.  It scans all running node-upgrade tasks and, if
// every targeted node has reached the target version, marks the task completed.
// It also enforces a timeout: tasks running longer than nodeUpgradeTimeout are
// marked as "failed" with a timeout message.
// The persistStatus callback is invoked (outside the lock) for any task whose
// status changed, so the caller can persist the new status to the database.
func checkNodeUpgradeTaskCompletion(nodeID, reportedVersion string, lookupVersion func(string) string, persistStatus func(id, status string)) {
	taskMu.Lock()

	reportedVersion = strings.TrimSpace(reportedVersion)
	now := time.Now()

	type statusChange struct {
		id     string
		status string
	}
	var changes []statusChange

	for _, t := range taskRepo {
		if t.Type != "node" || t.Status != "running" {
			continue
		}

		// Timeout check: if the task has been running too long, mark it failed.
		if now.Sub(t.CreatedAt) > nodeUpgradeTimeout {
			t.Status = "failed"
			t.Logs = append(t.Logs, upgradeLog{
				Timestamp: now,
				Level:     "ERROR",
				Message:   "upgrade task timed out (exceeded " + nodeUpgradeTimeout.String() + ")",
			})
			// Log which nodes didn't reach target version.
			target := strings.TrimSpace(t.TargetVersion)
			if target != "" && !strings.EqualFold(target, "latest") {
				for _, nid := range t.NodeIDs {
					ver := lookupVersion(nid)
					if strings.TrimSpace(ver) != target {
						t.Logs = append(t.Logs, upgradeLog{
							Timestamp: now,
							Level:     "WARNING",
							Message:   "node did not reach target version " + target + " (current: " + strings.TrimSpace(ver) + ")",
							NodeID:    nid,
						})
					}
				}
			}
			changes = append(changes, statusChange{id: t.ID, status: t.Status})
			continue
		}

		target := strings.TrimSpace(t.TargetVersion)
		if target == "" || strings.EqualFold(target, "latest") {
			continue
		}

		if reportedVersion == "" {
			continue
		}

		// Check whether this node is part of the task.
		found := false
		for _, nid := range t.NodeIDs {
			if nid == nodeID {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		// If this node now matches the target, log it and ACK the pending
		// upgrade command so it stops being redelivered to this node.
		if reportedVersion == target {
			t.Logs = append(t.Logs, upgradeLog{
				Timestamp: now,
				Level:     "INFO",
				Message:   "node version matches target: " + target,
				NodeID:    nodeID,
			})
			// Safe to call while holding taskMu: ackNodeUpgradeCommand uses an
			// independent mutex (nodeUpgradeMu). No lock-ordering concern
			// because no code path acquires taskMu while holding nodeUpgradeMu.
			ackNodeUpgradeCommand(nodeID, t.ID)
		}

		// Check all nodes in this task.
		allDone := true
		for _, nid := range t.NodeIDs {
			ver := reportedVersion
			if nid != nodeID {
				ver = strings.TrimSpace(lookupVersion(nid))
			}
			if ver != target {
				allDone = false
				break
			}
		}
		if allDone {
			t.Status = "completed"
			t.Logs = append(t.Logs, upgradeLog{
				Timestamp: now,
				Level:     "INFO",
				Message:   "all nodes upgraded to " + target,
			})
			changes = append(changes, statusChange{id: t.ID, status: t.Status})
		}
	}

	taskMu.Unlock()

	// Persist status changes outside the lock.
	if persistStatus != nil {
		for _, c := range changes {
			persistStatus(c.id, c.status)
		}
	}
}
