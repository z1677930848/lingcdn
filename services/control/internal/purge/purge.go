package purge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/nodehub"
	"github.com/lingcdn/control/internal/store"
	"github.com/lingcdn/control/internal/taskutils"
	controlpb "github.com/lingcdn/control/proto/gen"
)

const defaultPurgeConcurrency = 16
const defaultPurgeResultTimeout = 15 * time.Second

// Service dispatches purge commands to nodes.
type Service struct {
	hub *nodehub.Hub
	rdb store.Redis

	mu       sync.RWMutex
	requests map[string]*PurgeRequest
	waiters  map[string]map[string]chan *controlpb.PurgeResult
}

// PurgeRequest tracks a purge operation.
type PurgeRequest struct {
	ID          string
	URLs        []string
	StartedAt   time.Time
	CompletedAt time.Time
	TotalNodes  int
	Results     map[string]*NodePurgeResult
}

// NodePurgeResult contains the purge result from a single node.
type NodePurgeResult struct {
	NodeID    string
	OK        bool
	Reason    string
	Timestamp time.Time
}

// New creates a new purge Service.
func New(hub *nodehub.Hub, rdb store.Redis) *Service {
	return &Service{
		hub:      hub,
		rdb:      rdb,
		requests: make(map[string]*PurgeRequest),
		waiters:  make(map[string]map[string]chan *controlpb.PurgeResult),
	}
}

// PurgeURLs sends purge commands to all connected nodes.
func (s *Service) PurgeURLs(ctx context.Context, urls []string) error {
	_, err := s.PurgeURLsWithID(ctx, urls)
	return err
}

func (s *Service) PurgeURLsWithID(ctx context.Context, urls []string) (string, error) {
	if len(urls) == 0 {
		return "", fmt.Errorf("no URLs to purge")
	}

	requestID := uuid.NewString()

	req := &PurgeRequest{
		ID:        requestID,
		URLs:      urls,
		StartedAt: time.Now(),
		Results:   make(map[string]*NodePurgeResult),
	}

	// Store request for tracking
	s.mu.Lock()
	s.requests[requestID] = req
	s.mu.Unlock()
	s.persistRequest(req)

	// Get all connected nodes
	nodeIDs := s.hub.ListNodeIDs()
	req.TotalNodes = len(nodeIDs)

	if len(nodeIDs) == 0 {
		log.Ctx(ctx).Warn().Msg("no nodes connected for purge")
		return "", fmt.Errorf("no nodes connected")
	}

	log.Ctx(ctx).Info().
		Str("request_id", requestID).
		Int("urls", len(urls)).
		Int("nodes", len(nodeIDs)).
		Msg("starting purge operation")

	// Build purge command
	cmd := &controlpb.PurgeCommand{
		Urls:      urls,
		RequestId: requestID,
	}

	// Send to all nodes
	var mu sync.Mutex
	_ = taskutils.RunConcurrent(ctx, nodeIDs, defaultPurgeConcurrency, func(ctx context.Context, nid string) error {
		resultCh := make(chan *controlpb.PurgeResult, 1)
		s.registerWaiter(requestID, nid, resultCh)
		defer s.unregisterWaiter(requestID, nid)

		if err := s.sendPurgeToNode(ctx, nid, cmd); err != nil {
			mu.Lock()
			req.Results[nid] = &NodePurgeResult{
				NodeID:    nid,
				OK:        false,
				Reason:    err.Error(),
				Timestamp: time.Now(),
			}
			mu.Unlock()
			return nil
		}

		timeout := defaultPurgeResultTimeout
		if d, ok := ctx.Deadline(); ok {
			if until := time.Until(d); until > 0 && until < timeout {
				timeout = until
			}
		}

		select {
		case <-ctx.Done():
			mu.Lock()
			req.Results[nid] = &NodePurgeResult{
				NodeID:    nid,
				OK:        false,
				Reason:    ctx.Err().Error(),
				Timestamp: time.Now(),
			}
			mu.Unlock()
		case res := <-resultCh:
			mu.Lock()
			req.Results[nid] = &NodePurgeResult{
				NodeID:    nid,
				OK:        res.GetOk(),
				Reason:    res.GetReason(),
				Timestamp: time.Now(),
			}
			mu.Unlock()
		case <-time.After(timeout):
			mu.Lock()
			req.Results[nid] = &NodePurgeResult{
				NodeID:    nid,
				OK:        false,
				Reason:    "timeout waiting for node purge result",
				Timestamp: time.Now(),
			}
			mu.Unlock()
		}
		return nil
	})

	req.CompletedAt = time.Now()
	s.persistRequest(req)

	// Count successes and failures
	successCount := 0
	failCount := 0
	for _, result := range req.Results {
		if result.OK {
			successCount++
		} else {
			failCount++
		}
	}

	log.Ctx(ctx).Info().
		Str("request_id", requestID).
		Int("success", successCount).
		Int("failed", failCount).
		Dur("duration", req.CompletedAt.Sub(req.StartedAt)).
		Msg("purge operation completed")

	if failCount > 0 {
		return requestID, fmt.Errorf("purge partially failed: %d/%d nodes failed", failCount, req.TotalNodes)
	}

	return requestID, nil
}

// PurgeURLsToNodes sends purge commands to specific nodes.
func (s *Service) PurgeURLsToNodes(ctx context.Context, urls []string, nodeIDs []string) error {
	if len(urls) == 0 {
		return fmt.Errorf("no URLs to purge")
	}
	if len(nodeIDs) == 0 {
		return fmt.Errorf("no nodes specified")
	}

	requestID := uuid.NewString()

	cmd := &controlpb.PurgeCommand{
		Urls:      urls,
		RequestId: requestID,
	}

	log.Ctx(ctx).Info().
		Str("request_id", requestID).
		Int("urls", len(urls)).
		Int("nodes", len(nodeIDs)).
		Msg("starting targeted purge operation")

	var errors []error
	for _, nodeID := range nodeIDs {
		if err := s.sendPurgeToNode(ctx, nodeID, cmd); err != nil {
			errors = append(errors, fmt.Errorf("node %s: %w", nodeID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("purge failed on %d nodes", len(errors))
	}

	return nil
}

// sendPurgeToNode sends a purge command to a specific node.
func (s *Service) sendPurgeToNode(ctx context.Context, nodeID string, cmd *controlpb.PurgeCommand) error {
	session, ok := s.hub.Get(nodeID)
	if !ok {
		return fmt.Errorf("node not connected")
	}

	if session.PurgeStream == nil {
		return fmt.Errorf("node has no purge stream")
	}

	if err := s.hub.SendPurge(nodeID, cmd); err != nil {
		return err
	}

	log.Ctx(ctx).Info().
		Str("node_id", nodeID).
		Str("request_id", cmd.RequestId).
		Int("urls", len(cmd.Urls)).
		Msg("purge command dispatched to node")

	return nil
}

func (s *Service) registerWaiter(requestID, nodeID string, ch chan *controlpb.PurgeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.waiters[requestID]
	if !ok {
		m = make(map[string]chan *controlpb.PurgeResult)
		s.waiters[requestID] = m
	}
	m[nodeID] = ch
}

func (s *Service) unregisterWaiter(requestID, nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.waiters[requestID]; ok {
		delete(m, nodeID)
		if len(m) == 0 {
			delete(s.waiters, requestID)
		}
	}
}

func (s *Service) ReportNodeResult(nodeID string, res *controlpb.PurgeResult) {
	if res == nil || res.GetRequestId() == "" {
		return
	}

	s.mu.Lock()
	req, reqOK := s.requests[res.GetRequestId()]
	if reqOK {
		if req.Results == nil {
			req.Results = make(map[string]*NodePurgeResult)
		}
		req.Results[nodeID] = &NodePurgeResult{
			NodeID:    nodeID,
			OK:        res.GetOk(),
			Reason:    res.GetReason(),
			Timestamp: time.Now(),
		}
		if req.TotalNodes > 0 && len(req.Results) >= req.TotalNodes && req.CompletedAt.IsZero() {
			req.CompletedAt = time.Now()
		}
		s.persistRequest(req)
	}

	var ch chan *controlpb.PurgeResult
	if m, ok := s.waiters[res.GetRequestId()]; ok {
		ch = m[nodeID]
	}
	s.mu.Unlock()

	if ch != nil {
		select {
		case ch <- res:
		default:
		}
	}
}

func (s *Service) redisKey(requestID string) string {
	return "purge:req:" + requestID
}

func (s *Service) persistRequest(req *PurgeRequest) {
	if s.rdb == nil || req == nil || req.ID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.rdb.SetJSON(ctx, s.redisKey(req.ID), req, 24*time.Hour); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("request_id", req.ID).Msg("failed to persist purge request")
	}
}

// GetRequest returns a purge request by ID.
func (s *Service) GetRequest(requestID string) (*PurgeRequest, bool) {
	s.mu.RLock()
	req, ok := s.requests[requestID]
	s.mu.RUnlock()
	if ok && req != nil {
		return req, true
	}
	if s.rdb == nil {
		return nil, false
	}
	var loaded PurgeRequest
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	found, err := s.rdb.GetJSON(ctx, s.redisKey(requestID), &loaded)
	if err != nil || !found {
		return nil, false
	}
	s.mu.Lock()
	s.requests[requestID] = &loaded
	s.mu.Unlock()
	return &loaded, true
}

// ListRequests returns recent purge requests.
func (s *Service) ListRequests(limit int) []*PurgeRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	requests := make([]*PurgeRequest, 0, len(s.requests))
	for _, req := range s.requests {
		requests = append(requests, req)
	}

	// Sort by start time (newest first) and limit
	// For simplicity, just return up to limit
	if len(requests) > limit {
		requests = requests[:limit]
	}

	return requests
}

// CleanupOldRequests removes purge requests older than maxAge.
func (s *Service) CleanupOldRequests(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	removed := 0

	for id, req := range s.requests {
		if now.Sub(req.StartedAt) > maxAge {
			delete(s.requests, id)
			removed++
		}
	}

	return removed
}
