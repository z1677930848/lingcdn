package nodehub

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	controlpb "github.com/lingcdn/control/proto/gen"
)

// Hub manages online node sessions and config distribution.
type Hub struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// Session represents a connected node with its config stream.
type Session struct {
	NodeID       string
	Hostname     string
	Version      string
	Capabilities []string
	Status       string
	ConfigStream controlpb.NodeControl_StreamConfigServer
	PurgeStream  controlpb.NodeControl_StreamPurgeServer
	LastSeen     time.Time
	ConfigVer    string
	cancel       context.CancelFunc
	sendMu       sync.Mutex
}

// New creates a Hub.
func New() *Hub {
	return &Hub{
		sessions: make(map[string]*Session),
	}
}

// Add registers a new node session.
func (h *Hub) Add(ctx context.Context, s *Session) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If there's an existing session, cancel it
	if existing, ok := h.sessions[s.NodeID]; ok && existing.cancel != nil {
		existing.cancel()
	}

	s.LastSeen = time.Now()
	h.sessions[s.NodeID] = s

	log.Ctx(ctx).Info().
		Str("node_id", s.NodeID).
		Str("hostname", s.Hostname).
		Strs("capabilities", s.Capabilities).
		Msg("node session added")
}

// Remove deletes a node session.
func (h *Hub) Remove(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if s, ok := h.sessions[nodeID]; ok {
		if s.cancel != nil {
			s.cancel()
		}
		delete(h.sessions, nodeID)
		log.Info().Str("node_id", nodeID).Msg("node session removed")
	}
}

// Get returns a session if present.
func (h *Hub) Get(nodeID string) (*Session, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[nodeID]
	return s, ok
}

// UpdateHeartbeat updates the last seen time and status for a node.
func (h *Hub) UpdateHeartbeat(nodeID, status, configVer string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if s, ok := h.sessions[nodeID]; ok {
		s.LastSeen = time.Now()
		s.Status = status
		s.ConfigVer = configVer
	}
}

// SetConfigStream sets the config stream for a node session.
func (h *Hub) SetConfigStream(nodeID string, stream controlpb.NodeControl_StreamConfigServer, cancel context.CancelFunc) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if s, ok := h.sessions[nodeID]; ok {
		s.ConfigStream = stream
		s.cancel = cancel
		return true
	}
	return false
}

// ClearConfigStream unbinds the config stream from a session while keeping
// the session itself (and its metadata/cancel) intact. This is the correct
// cleanup on a transient stream disconnect: the node agent will reconnect
// and call SetConfigStream again — if the session had been Remove'd, the
// reconnect path would leave the hub with no session and every subsequent
// Publisher.Publish / hub.SendConfig would fail with ErrNodeNotConnected
// even though the node is actually online.
//
// The check `s.ConfigStream == stream` is a guard against the unwind of an
// older stream goroutine racing with a freshly-established one: if the node
// reconnected before the old defer fires, we must not clobber the new
// binding. Returns true only when the stored stream equals the caller's
// stream and was cleared.
func (h *Hub) ClearConfigStream(nodeID string, stream controlpb.NodeControl_StreamConfigServer) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	s, ok := h.sessions[nodeID]
	if !ok {
		return false
	}
	if s.ConfigStream != stream {
		// A newer stream has already bound to this session (fast reconnect),
		// or the binding was never set for this stream. Leave it alone.
		return false
	}
	s.ConfigStream = nil
	// Intentionally do NOT clear s.cancel here: cancel is tied to the
	// streamCtx that has already been cancelled by the outer defer, and
	// keeping a harmless pointer costs nothing. Zeroing it would risk
	// breaking a concurrent Add(existing.cancel()) path.
	return true
}

func (h *Hub) SetPurgeStream(nodeID string, stream controlpb.NodeControl_StreamPurgeServer) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if s, ok := h.sessions[nodeID]; ok {
		s.PurgeStream = stream
		return true
	}
	return false
}

// List returns all active sessions.
func (h *Hub) List() []*Session {
	h.mu.RLock()
	defer h.mu.RUnlock()

	sessions := make([]*Session, 0, len(h.sessions))
	for _, s := range h.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// ListNodeIDs returns all connected node IDs.
func (h *Hub) ListNodeIDs() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ids := make([]string, 0, len(h.sessions))
	for id := range h.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of connected nodes.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sessions)
}

// SendConfig sends a config envelope to a specific node.
func (h *Hub) SendConfig(nodeID string, env *controlpb.ConfigEnvelope) error {
	h.mu.RLock()
	s, ok := h.sessions[nodeID]
	h.mu.RUnlock()

	if !ok || s.ConfigStream == nil {
		return ErrNodeNotConnected
	}

	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.ConfigStream.Send(env)
}

func (h *Hub) SendPurge(nodeID string, cmd *controlpb.PurgeCommand) error {
	h.mu.RLock()
	s, ok := h.sessions[nodeID]
	h.mu.RUnlock()

	if !ok || s.PurgeStream == nil {
		return ErrNodeNotConnected
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.PurgeStream.Send(cmd)
}

// BroadcastConfig sends a config envelope to all connected nodes.
func (h *Hub) BroadcastConfig(env *controlpb.ConfigEnvelope) map[string]error {
	h.mu.RLock()
	sessions := make([]*Session, 0, len(h.sessions))
	for _, s := range h.sessions {
		sessions = append(sessions, s)
	}
	h.mu.RUnlock()

	errors := make(map[string]error)
	for _, s := range sessions {
		if s.ConfigStream == nil {
			errors[s.NodeID] = ErrNodeNotConnected
			continue
		}
		s.sendMu.Lock()
		err := s.ConfigStream.Send(env)
		s.sendMu.Unlock()
		if err != nil {
			errors[s.NodeID] = err
		}
	}
	return errors
}

// CleanupStale removes sessions that haven't sent a heartbeat recently.
func (h *Hub) CleanupStale(maxAge time.Duration) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	removed := 0

	for id, s := range h.sessions {
		if now.Sub(s.LastSeen) > maxAge {
			if s.cancel != nil {
				s.cancel()
			}
			delete(h.sessions, id)
			removed++
			log.Info().Str("node_id", id).Msg("removed stale node session")
		}
	}

	return removed
}

// SweepOffline removes sessions that both
//  1. currently have no bound config stream (ConfigStream == nil), AND
//  2. haven't received a heartbeat within the last maxAge
//
// This is the correct cleanup for sessions that ClearConfigStream left
// behind on a transient disconnect. As long as the node agent is still
// alive it will either reconnect (repopulating ConfigStream) or keep
// sending heartbeats (refreshing LastSeen) — so removing a session only
// happens when BOTH signals are stale, i.e. the host is genuinely gone.
//
// Returns the removed node IDs so the caller can log them and fan out
// follow-up notifications (DNS resync, purge bookkeeping, etc.). Unlike
// CleanupStale this method never touches an actively-streaming session,
// which is what makes it safe to run on a short interval without risk
// of killing nodes that are merely between heartbeats.
func (h *Hub) SweepOffline(maxAge time.Duration) []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	var removed []string

	for id, s := range h.sessions {
		if s == nil {
			continue
		}
		// Active stream → the node is reachable right now, do not touch.
		if s.ConfigStream != nil {
			continue
		}
		// Fresh heartbeat → the node is probably between config-stream
		// reconnects; its agent is still alive.
		if now.Sub(s.LastSeen) <= maxAge {
			continue
		}
		if s.cancel != nil {
			s.cancel()
		}
		delete(h.sessions, id)
		removed = append(removed, id)
		log.Info().Str("node_id", id).Dur("idle", now.Sub(s.LastSeen)).Msg("nodehub: removed offline node session")
	}

	return removed
}

// Errors
var (
	ErrNodeNotConnected = &NodeError{Message: "node not connected"}
)

// NodeError represents a node-related error.
type NodeError struct {
	Message string
}

func (e *NodeError) Error() string {
	return e.Message
}
