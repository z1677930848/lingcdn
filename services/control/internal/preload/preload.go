package preload

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

const defaultPreloadConcurrency = 16
const defaultPreloadResultTimeout = 30 * time.Second

// Service dispatches preload (cache warm-up) commands to nodes.
type Service struct {
	hub *nodehub.Hub
	rdb store.Redis

	mu       sync.RWMutex
	requests map[string]*PreloadRequest
	waiters  map[string]map[string]chan *controlpb.PreloadResult
}

// PreloadRequest tracks a preload operation.
type PreloadRequest struct {
	ID          string
	URLs        []string
	StartedAt   time.Time
	CompletedAt time.Time
	TotalNodes  int
	Results     map[string]*NodePreloadResult
}

// NodePreloadResult contains the preload result from a single node.
type NodePreloadResult struct {
	NodeID    string
	OK        bool
	Reason    string
	Loaded    int32
	Timestamp time.Time
}

// New creates a new preload Service.
func New(hub *nodehub.Hub, rdb store.Redis) *Service {
	return &Service{
		hub:      hub,
		rdb:      rdb,
		requests: make(map[string]*PreloadRequest),
		waiters:  make(map[string]map[string]chan *controlpb.PreloadResult),
	}
}

// PreloadURLs dispatches preload commands to all connected nodes.
func (s *Service) PreloadURLs(ctx context.Context, urls []string) (string, error) {
	if len(urls) == 0 {
		return "", fmt.Errorf("no URLs to preload")
	}

	requestID := uuid.NewString()
	req := &PreloadRequest{
		ID:        requestID,
		URLs:      urls,
		StartedAt: time.Now(),
		Results:   make(map[string]*NodePreloadResult),
	}

	s.mu.Lock()
	s.requests[requestID] = req
	s.mu.Unlock()
	s.persistRequest(req)

	nodeIDs := s.hub.ListNodeIDs()
	req.TotalNodes = len(nodeIDs)
	if len(nodeIDs) == 0 {
		log.Ctx(ctx).Warn().Msg("no nodes connected for preload")
		return requestID, fmt.Errorf("no nodes connected")
	}

	cmd := &controlpb.PreloadCommand{
		Urls:      urls,
		RequestId: requestID,
	}

	log.Ctx(ctx).Info().
		Str("request_id", requestID).
		Int("urls", len(urls)).
		Int("nodes", len(nodeIDs)).
		Msg("starting preload operation")

	var mu sync.Mutex
	_ = taskutils.RunConcurrent(ctx, nodeIDs, defaultPreloadConcurrency, func(ctx context.Context, nid string) error {
		resultCh := make(chan *controlpb.PreloadResult, 1)
		s.registerWaiter(requestID, nid, resultCh)
		defer s.unregisterWaiter(requestID, nid)

		if err := s.sendPreloadToNode(ctx, nid, cmd); err != nil {
			mu.Lock()
			req.Results[nid] = &NodePreloadResult{
				NodeID:    nid,
				OK:        false,
				Reason:    err.Error(),
				Timestamp: time.Now(),
			}
			mu.Unlock()
			return nil
		}

		timeout := defaultPreloadResultTimeout
		if d, ok := ctx.Deadline(); ok {
			if until := time.Until(d); until > 0 && until < timeout {
				timeout = until
			}
		}

		select {
		case <-ctx.Done():
			mu.Lock()
			req.Results[nid] = &NodePreloadResult{
				NodeID:    nid,
				OK:        false,
				Reason:    ctx.Err().Error(),
				Timestamp: time.Now(),
			}
			mu.Unlock()
		case res := <-resultCh:
			mu.Lock()
			req.Results[nid] = &NodePreloadResult{
				NodeID:    nid,
				OK:        res.GetOk(),
				Reason:    res.GetReason(),
				Loaded:    res.GetLoaded(),
				Timestamp: time.Now(),
			}
			mu.Unlock()
		case <-time.After(timeout):
			mu.Lock()
			req.Results[nid] = &NodePreloadResult{
				NodeID:    nid,
				OK:        false,
				Reason:    "timeout waiting for node preload result",
				Timestamp: time.Now(),
			}
			mu.Unlock()
		}
		return nil
	})

	req.CompletedAt = time.Now()
	s.persistRequest(req)

	failCount := 0
	for _, result := range req.Results {
		if !result.OK {
			failCount++
		}
	}

	if failCount > 0 {
		return requestID, fmt.Errorf("preload partially failed: %d/%d nodes failed", failCount, req.TotalNodes)
	}
	return requestID, nil
}

func (s *Service) sendPreloadToNode(ctx context.Context, nodeID string, cmd *controlpb.PreloadCommand) error {
	session, ok := s.hub.Get(nodeID)
	if !ok {
		return fmt.Errorf("node not connected")
	}
	if session.PreloadStream == nil {
		return fmt.Errorf("node has no preload stream")
	}
	if err := s.hub.SendPreload(nodeID, cmd); err != nil {
		return err
	}
	log.Ctx(ctx).Info().
		Str("node_id", nodeID).
		Str("request_id", cmd.RequestId).
		Int("urls", len(cmd.Urls)).
		Msg("preload command dispatched to node")
	return nil
}

func (s *Service) registerWaiter(requestID, nodeID string, ch chan *controlpb.PreloadResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.waiters[requestID]
	if !ok {
		m = make(map[string]chan *controlpb.PreloadResult)
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

func (s *Service) ReportNodeResult(nodeID string, res *controlpb.PreloadResult) {
	if res == nil || res.GetRequestId() == "" {
		return
	}

	s.mu.Lock()
	req, reqOK := s.requests[res.GetRequestId()]
	if reqOK {
		if req.Results == nil {
			req.Results = make(map[string]*NodePreloadResult)
		}
		req.Results[nodeID] = &NodePreloadResult{
			NodeID:    nodeID,
			OK:        res.GetOk(),
			Reason:    res.GetReason(),
			Loaded:    res.GetLoaded(),
			Timestamp: time.Now(),
		}
		if req.TotalNodes > 0 && len(req.Results) >= req.TotalNodes && req.CompletedAt.IsZero() {
			req.CompletedAt = time.Now()
		}
		s.persistRequest(req)
	}

	var ch chan *controlpb.PreloadResult
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
	return "preload:req:" + requestID
}

func (s *Service) persistRequest(req *PreloadRequest) {
	if s.rdb == nil || req == nil || req.ID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.rdb.SetJSON(ctx, s.redisKey(req.ID), req, 24*time.Hour); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("request_id", req.ID).Msg("failed to persist preload request")
	}
}

// GetRequest returns a preload request by ID.
func (s *Service) GetRequest(requestID string) (*PreloadRequest, bool) {
	s.mu.RLock()
	req, ok := s.requests[requestID]
	s.mu.RUnlock()
	if ok && req != nil {
		return req, true
	}
	if s.rdb == nil {
		return nil, false
	}
	var loaded PreloadRequest
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
