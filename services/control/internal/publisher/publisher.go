package publisher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/compiler"
	"github.com/lingcdn/control/internal/nodehub"
	"github.com/lingcdn/control/internal/store"
	controlpb "github.com/lingcdn/control/proto/gen"
)

// Publisher handles config rollout and rollback to nodes.
type Publisher struct {
	hub      *nodehub.Hub
	compiler *compiler.Compiler
	store    store.Store

	mu          sync.RWMutex
	publishing  bool
	lastPublish *PublishResult
}

// PublishResult contains the result of a publish operation.
type PublishResult struct {
	Version      string
	StartedAt    time.Time
	CompletedAt  time.Time
	TotalNodes   int
	SuccessNodes int
	FailedNodes  int
	Errors       map[string]string
}

// New creates a new Publisher.
func New(hub *nodehub.Hub, comp *compiler.Compiler, s store.Store) *Publisher {
	return &Publisher{
		hub:      hub,
		compiler: comp,
		store:    s,
	}
}

// Publish compiles and distributes configuration to specified nodes.
// If nodeIDs is empty, publishes to all connected nodes.
func (p *Publisher) Publish(ctx context.Context, version string, nodeIDs []string) error {
	_, err := p.PublishWithResult(ctx, version, nodeIDs)
	return err
}

func (p *Publisher) PublishWithResult(ctx context.Context, version string, nodeIDs []string) (*PublishResult, error) {
	p.mu.Lock()
	if p.publishing {
		p.mu.Unlock()
		return nil, fmt.Errorf("publish already in progress")
	}
	p.publishing = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.publishing = false
		p.mu.Unlock()
	}()

	result := &PublishResult{
		Version:   version,
		StartedAt: time.Now(),
		Errors:    make(map[string]string),
	}

	// Get config version
	var cv *store.ConfigVersion
	var err error

	if version == "" {
		// Compile new version
		cv, err = p.compiler.CompileAndStore(ctx, "publisher")
		if err != nil {
			return p.finishResult(result, fmt.Errorf("compile config: %w", err))
		}
		result.Version = cv.Version
	} else {
		// Use existing version
		cv, err = p.store.GetConfigVersion(ctx, version)
		if err != nil {
			return p.finishResult(result, fmt.Errorf("get config version: %w", err))
		}
		if cv == nil {
			return p.finishResult(result, fmt.Errorf("config version not found: %s", version))
		}
		result.Version = cv.Version
	}

	deliveryID := uuid.NewString()

	// Build config envelope
	env := &controlpb.ConfigEnvelope{
		Version:    cv.Version,
		Payload:    cv.Payload,
		Checksum:   cv.Checksum,
		DeliveryId: deliveryID,
	}

	// Determine target nodes. We MUST surface a ListNodes error rather than
	// silently treating it as "no disabled nodes": dropping that error
	// previously meant a transient DB blip would let us push config to nodes
	// the operator had explicitly taken offline.
	nodes, err := p.store.ListNodes(ctx)
	if err != nil {
		return p.finishResult(result, fmt.Errorf("list nodes: %w", err))
	}
	disabled := make(map[string]bool)
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if strings.EqualFold(n.Status, "disabled") {
			disabled[n.ID] = true
		}
	}
	var targetNodes []string
	if len(nodeIDs) > 0 {
		for _, id := range nodeIDs {
			if disabled[id] {
				log.Ctx(ctx).Warn().Str("node_id", id).Msg("skip publish to disabled node")
				continue
			}
			targetNodes = append(targetNodes, id)
		}
	} else {
		for _, id := range p.hub.ListNodeIDs() {
			if disabled[id] {
				log.Ctx(ctx).Debug().Str("node_id", id).Msg("skip disabled node in publish")
				continue
			}
			targetNodes = append(targetNodes, id)
		}
	}

	result.TotalNodes = len(targetNodes)

	log.Ctx(ctx).Info().
		Str("version", cv.Version).
		Str("delivery_id", deliveryID).
		Int("target_nodes", len(targetNodes)).
		Msg("starting config publish")

	ackedNodes := make([]string, 0, len(targetNodes))
	for _, nodeID := range targetNodes {
		p.hub.ClearConfigAck(nodeID, cv.Version, deliveryID)
		if err := p.hub.SendConfig(nodeID, env); err != nil {
			result.FailedNodes++
			result.Errors[nodeID] = err.Error()
			log.Ctx(ctx).Warn().
				Str("node_id", nodeID).
				Err(err).
				Msg("failed to send config to node")
		} else {
			ackedNodes = append(ackedNodes, nodeID)
		}
	}

	ackTimeout := 30 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < ackTimeout {
			ackTimeout = remaining
		}
	}
	ackCtx, cancel := context.WithTimeout(ctx, ackTimeout)
	defer cancel()

	// Fan-out the ack waits. The previous serial loop shared a single 30s
	// budget across all nodes, so a single slow ack at the front of the
	// list could exhaust the deadline and force every later node to time
	// out without ever being checked. With concurrent waits each node
	// gets the full ackTimeout to respond, capped only by the parent ctx
	// deadline (which we already mirror into ackTimeout above).
	type ackOutcome struct {
		nodeID string
		ack    nodehub.ConfigAckResult
		err    error
	}
	results := make(chan ackOutcome, len(ackedNodes))
	var wg sync.WaitGroup
	for _, nodeID := range ackedNodes {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			ack, err := p.hub.WaitForConfigAck(ackCtx, id, cv.Version, deliveryID)
			results <- ackOutcome{nodeID: id, ack: ack, err: err}
		}(nodeID)
	}
	wg.Wait()
	close(results)

	for outcome := range results {
		if outcome.err != nil {
			result.FailedNodes++
			result.Errors[outcome.nodeID] = "wait config ack: " + outcome.err.Error()
			log.Ctx(ctx).Warn().
				Str("node_id", outcome.nodeID).
				Str("version", cv.Version).
				Err(outcome.err).
				Msg("config ack not received")
			continue
		}
		if !outcome.ack.OK {
			result.FailedNodes++
			if strings.TrimSpace(outcome.ack.Reason) == "" {
				result.Errors[outcome.nodeID] = "node rejected config"
			} else {
				result.Errors[outcome.nodeID] = outcome.ack.Reason
			}
			log.Ctx(ctx).Warn().
				Str("node_id", outcome.nodeID).
				Str("version", cv.Version).
				Str("reason", outcome.ack.Reason).
				Msg("node rejected config")
			continue
		}
		result.SuccessNodes++
	}

	result.CompletedAt = time.Now()

	p.mu.Lock()
	p.lastPublish = result
	p.mu.Unlock()

	log.Ctx(ctx).Info().
		Str("version", cv.Version).
		Str("delivery_id", deliveryID).
		Int("success", result.SuccessNodes).
		Int("failed", result.FailedNodes).
		Dur("duration", result.CompletedAt.Sub(result.StartedAt)).
		Msg("config publish completed")

	if result.FailedNodes > 0 {
		return result, fmt.Errorf("publish partially failed: %d/%d nodes failed", result.FailedNodes, result.TotalNodes)
	}

	return result, nil
}

func (p *Publisher) finishResult(result *PublishResult, err error) (*PublishResult, error) {
	if result == nil {
		return nil, err
	}
	result.CompletedAt = time.Now()
	if err != nil && result.Errors != nil {
		result.Errors["_"] = err.Error()
	}
	p.mu.Lock()
	p.lastPublish = result
	p.mu.Unlock()
	return result, err
}

// PublishToNode sends the latest config to a specific node.
func (p *Publisher) PublishToNode(ctx context.Context, nodeID string) error {
	cv, err := p.compiler.GetLatestConfig(ctx)
	if err != nil {
		return fmt.Errorf("get latest config: %w", err)
	}
	if cv == nil {
		// No config yet, compile one
		cv, err = p.compiler.CompileAndStore(ctx, "publisher")
		if err != nil {
			return fmt.Errorf("compile config: %w", err)
		}
	}

	env := &controlpb.ConfigEnvelope{
		Version:    cv.Version,
		Payload:    cv.Payload,
		Checksum:   cv.Checksum,
		DeliveryId: uuid.NewString(),
	}

	p.hub.ClearConfigAck(nodeID, cv.Version, env.DeliveryId)
	if err := p.hub.SendConfig(nodeID, env); err != nil {
		return err
	}
	ackCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ack, err := p.hub.WaitForConfigAck(ackCtx, nodeID, cv.Version, env.DeliveryId)
	if err != nil {
		return fmt.Errorf("wait config ack: %w", err)
	}
	if !ack.OK {
		if strings.TrimSpace(ack.Reason) == "" {
			return fmt.Errorf("node rejected config")
		}
		return fmt.Errorf("node rejected config: %s", ack.Reason)
	}
	return nil
}

// Rollback reverts to a previous configuration version.
func (p *Publisher) Rollback(ctx context.Context, version string) error {
	cv, err := p.store.GetConfigVersion(ctx, version)
	if err != nil {
		return fmt.Errorf("get config version: %w", err)
	}
	if cv == nil {
		return fmt.Errorf("config version not found: %s", version)
	}

	log.Ctx(ctx).Info().
		Str("version", version).
		Msg("rolling back to previous config version")

	return p.Publish(ctx, version, nil)
}

// GetLastPublishResult returns the result of the last publish operation.
func (p *Publisher) GetLastPublishResult() *PublishResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastPublish
}

// IsPublishing returns true if a publish is currently in progress.
func (p *Publisher) IsPublishing() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.publishing
}

// BuildConfigEnvelope creates a config envelope from a stored version.
func (p *Publisher) BuildConfigEnvelope(ctx context.Context, version string) (*controlpb.ConfigEnvelope, error) {
	var cv *store.ConfigVersion
	var err error

	if version == "" {
		cv, err = p.compiler.GetLatestConfig(ctx)
	} else {
		cv, err = p.store.GetConfigVersion(ctx, version)
	}

	if err != nil {
		return nil, err
	}
	if cv == nil {
		return nil, fmt.Errorf("config version not found: version=%q", version)
	}

	// Recalculate checksum to verify integrity
	checksum := sha256.Sum256(cv.Payload)
	checksumHex := hex.EncodeToString(checksum[:])

	return &controlpb.ConfigEnvelope{
		Version:  cv.Version,
		Payload:  cv.Payload,
		Checksum: checksumHex,
	}, nil
}
