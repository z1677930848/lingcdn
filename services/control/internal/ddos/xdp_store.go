package ddos

import (
	"sync"
	"time"
)

type XdpSnapshot struct {
	Enabled   bool              `json:"enabled"`
	Interface string            `json:"interface"`
	UpdatedAt time.Time         `json:"updated_at"`
	Stats     map[string]uint64 `json:"stats"`
}

type XdpStore struct {
	mu     sync.RWMutex
	byNode map[string]*XdpSnapshot
}

func NewXdpStore() *XdpStore {
	return &XdpStore{
		byNode: make(map[string]*XdpSnapshot),
	}
}

func (s *XdpStore) Update(nodeID string, iface string, enabled *bool, stats map[string]uint64, at time.Time) {
	if nodeID == "" {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.byNode[nodeID]
	if !ok {
		cur = &XdpSnapshot{
			Stats: make(map[string]uint64),
		}
		s.byNode[nodeID] = cur
	}

	if iface != "" {
		cur.Interface = iface
	}
	if enabled != nil {
		cur.Enabled = *enabled
	}
	if len(stats) > 0 {
		if cur.Stats == nil {
			cur.Stats = make(map[string]uint64)
		}
		for k, v := range stats {
			cur.Stats[k] = v
		}
	}
	if at.After(cur.UpdatedAt) {
		cur.UpdatedAt = at
	}
}

func (s *XdpStore) Get(nodeID string) (*XdpSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.byNode[nodeID]
	if !ok || v == nil {
		return nil, false
	}
	out := &XdpSnapshot{
		Enabled:   v.Enabled,
		Interface: v.Interface,
		UpdatedAt: v.UpdatedAt,
	}
	if len(v.Stats) > 0 {
		out.Stats = make(map[string]uint64, len(v.Stats))
		for k, val := range v.Stats {
			out.Stats[k] = val
		}
	} else {
		out.Stats = map[string]uint64{}
	}
	return out, true
}

