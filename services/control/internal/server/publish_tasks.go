package server

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lingcdn/control/internal/store"
)

// Publisher busy signal: a sentinel error returned by the publisher when another
// publish is already in progress. startPublishTask retries on this error only.
var errPublisherBusy = errors.New("publisher busy")

// Maximum number of finished publish tasks retained in memory. Older tasks are
// trimmed off the front to bound memory use across long-running control planes.
const publishTaskHistoryCap = 500

type publishTask struct {
	ID           string            `json:"id"`
	Trigger      string            `json:"trigger"` // auto | manual
	Subject      string            `json:"subject,omitempty"`
	Reason       string            `json:"reason"`
	Version      string            `json:"version"`
	NodeIDs      []string          `json:"node_ids"`
	Status       string            `json:"status"` // running | success | failed
	Message      string            `json:"message"`
	StartedAt    time.Time         `json:"started_at"`
	CompletedAt  time.Time         `json:"completed_at"`
	TotalNodes   int               `json:"total_nodes"`
	SuccessNodes int               `json:"success_nodes"`
	FailedNodes  int               `json:"failed_nodes"`
	Errors       map[string]string `json:"errors,omitempty"`
}

var (
	publishTaskMu   sync.Mutex
	publishTaskList []*publishTask
)

// isPublisherBusy classifies an error as a transient "publisher busy" signal.
// We accept either the typed sentinel or the legacy message-based detection so
// this function keeps working if an old publisher implementation returns a
// plain error.
func isPublisherBusy(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errPublisherBusy) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "already in progress")
}

// appendPublishTaskLocked appends to the task list and trims the oldest entries
// to enforce publishTaskHistoryCap. Caller must hold publishTaskMu.
func appendPublishTaskLocked(t *publishTask) {
	publishTaskList = append(publishTaskList, t)
	if n := len(publishTaskList); n > publishTaskHistoryCap {
		// Drop the oldest (n - cap) entries. Copy into a fresh slice so the
		// underlying array can be collected eventually.
		trimmed := make([]*publishTask, publishTaskHistoryCap)
		copy(trimmed, publishTaskList[n-publishTaskHistoryCap:])
		publishTaskList = trimmed
	}
}

func (s *Servers) startPublishTask(ctx context.Context, trigger, subject, reason, version string, nodeIDs []string) *publishTask {
	t := &publishTask{
		ID:        uuid.NewString(),
		Trigger:   strings.ToLower(strings.TrimSpace(trigger)),
		Subject:   strings.TrimSpace(subject),
		Reason:    strings.TrimSpace(reason),
		Version:   strings.TrimSpace(version),
		Status:    "running",
		Message:   "running",
		StartedAt: time.Now(),
		Errors:    make(map[string]string),
	}
	if t.Trigger == "" {
		t.Trigger = "auto"
	}

	targetNodes := s.resolvePublishTargetNodes(ctx, nodeIDs)
	t.NodeIDs = targetNodes
	t.TotalNodes = len(targetNodes)

	publishTaskMu.Lock()
	appendPublishTaskLocked(t)
	publishTaskMu.Unlock()
	s.emitPublishTaskEvent(t)

	go func(task *publishTask) {
		taskCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if s == nil || s.publisher == nil {
			publishTaskMu.Lock()
			task.Status = "failed"
			task.Message = "publisher not configured"
			task.CompletedAt = time.Now()
			publishTaskMu.Unlock()
			s.emitPublishTaskEvent(task)
			return
		}

		var err error
	retryLoop:
		for attempt := 0; attempt < 6; attempt++ {
			err = s.publisher.Publish(taskCtx, task.Version, task.NodeIDs)
			if err == nil {
				break
			}
			if !isPublisherBusy(err) {
				// Non-retryable: give up immediately.
				break
			}
			// Exponential backoff: 1s, 2s, 3s, ... capped by loop length.
			select {
			case <-taskCtx.Done():
				break retryLoop
			case <-time.After(time.Duration(1+attempt) * time.Second):
			}
		}

		res := s.publisher.GetLastPublishResult()
		publishTaskMu.Lock()
		if res != nil && strings.TrimSpace(res.Version) != "" {
			task.Version = res.Version
			task.CompletedAt = res.CompletedAt
			task.SuccessNodes = res.SuccessNodes
			task.FailedNodes = res.FailedNodes
			for k, v := range res.Errors {
				task.Errors[k] = v
			}
		} else {
			task.CompletedAt = time.Now()
		}

		if err != nil {
			task.Status = "failed"
			task.Message = err.Error()
			publishTaskMu.Unlock()
			s.emitPublishTaskEvent(task)
			return
		}
		task.Status = "success"
		task.Message = "ok"
		publishTaskMu.Unlock()
		s.emitPublishTaskEvent(task)
	}(t)

	return t
}

// emitPublishTaskEvent pushes a task-state snapshot through the SSE broker so
// connected browsers can update sync indicators in real time.
func (s *Servers) emitPublishTaskEvent(t *publishTask) {
	if s == nil || s.sseBroker == nil || t == nil {
		return
	}
	s.sseBroker.notifyTask(syncTaskEvent{
		Kind:        "publish",
		ID:          t.ID,
		Subject:     t.Subject,
		Status:      t.Status,
		Message:     t.Message,
		StartedAt:   t.StartedAt,
		CompletedAt: t.CompletedAt,
	})
}

func (s *Servers) resolvePublishTargetNodes(ctx context.Context, nodeIDs []string) []string {
	if s == nil || s.hub == nil {
		return nil
	}
	disabled := make(map[string]bool)
	if s.store != nil {
		ctxStore, cancel := store.WithTimeout(ctx)
		nodes, err := s.store.ListNodes(ctxStore)
		cancel()
		if err == nil {
			for _, n := range nodes {
				if n == nil {
					continue
				}
				if strings.EqualFold(n.Status, "disabled") {
					disabled[n.ID] = true
				}
			}
		}
	}

	var target []string
	if len(nodeIDs) > 0 {
		for _, id := range nodeIDs {
			id = strings.TrimSpace(id)
			if id == "" || disabled[id] {
				continue
			}
			target = append(target, id)
		}
		return target
	}
	for _, id := range s.hub.ListNodeIDs() {
		if id == "" || disabled[id] {
			continue
		}
		target = append(target, id)
	}
	return target
}

func listPublishTasks(limit int) []*publishTask {
	publishTaskMu.Lock()
	defer publishTaskMu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	n := len(publishTaskList)
	start := 0
	if n > limit {
		start = n - limit
	}
	out := make([]*publishTask, 0, n-start)
	for _, t := range publishTaskList[start:] {
		if t != nil {
			cp := *t
			if t.Errors != nil {
				cp.Errors = make(map[string]string, len(t.Errors))
				for k, v := range t.Errors {
					cp.Errors[k] = v
				}
			}
			if len(t.NodeIDs) > 0 {
				cp.NodeIDs = append([]string(nil), t.NodeIDs...)
			}
			out = append(out, &cp)
		}
	}
	return out
}

func getPublishTask(id string) (*publishTask, bool) {
	publishTaskMu.Lock()
	defer publishTaskMu.Unlock()
	for _, t := range publishTaskList {
		if t != nil && t.ID == id {
			cp := *t
			if t.Errors != nil {
				cp.Errors = make(map[string]string, len(t.Errors))
				for k, v := range t.Errors {
					cp.Errors[k] = v
				}
			}
			if len(t.NodeIDs) > 0 {
				cp.NodeIDs = append([]string(nil), t.NodeIDs...)
			}
			return &cp, true
		}
	}
	return nil, false
}
