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
