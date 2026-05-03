package server

// Background sweep that evicts nodehub sessions whose underlying node
// agent is genuinely gone. This is the companion to nodehub.ClearConfigStream:
// ClearConfigStream intentionally keeps a session around when its config
// stream drops (so transient reconnects don't lose the hub binding and
// break Publisher.Publish), but that means a crashed/powered-off host
// would otherwise linger forever — bloating hub.Count(), making every
// subsequent publish task look "partially failed", and causing dns_sync
// to keep handing out dead IPs.
//
// The sweeper runs on a fixed 1-minute tick with a 5-minute staleness
// window. The window is deliberately wider than the node agent's
// heartbeat + backoff worst case so that a node that is merely between
// reconnects is never evicted.

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// hubSessionSweepInterval is how often we check for orphan sessions.
	// Keeping it at 1min bounds the "ghost node in the hub" window to
	// at most ~6min (interval + stale-window) which is tight enough to
	// avoid noticeable UI/DNS drift and loose enough not to hot-spin
	// on the mutex during normal operation.
	hubSessionSweepInterval = time.Minute

	// hubSessionStaleAfter is the heartbeat age past which a
	// non-streaming session is treated as offline. The node agent sends
	// heartbeats continuously while alive, so 5 minutes is comfortably
	// beyond any reasonable network blip or control-plane restart.
	hubSessionStaleAfter = 5 * time.Minute
)

// hubSessionSweeper is a long-running goroutine started by Serve. It
// blocks until ctx is cancelled.
func (s *Servers) hubSessionSweeper(ctx context.Context) {
	if s == nil || s.hub == nil {
		return
	}
	ticker := time.NewTicker(hubSessionSweepInterval)
	defer ticker.Stop()

	log.Info().
		Dur("interval", hubSessionSweepInterval).
		Dur("stale_after", hubSessionStaleAfter).
		Msg("nodehub sweeper started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("nodehub sweeper stopped")
			return
		case <-ticker.C:
			removed := s.hub.SweepOffline(hubSessionStaleAfter)
			if len(removed) == 0 {
				continue
			}
			log.Info().
				Strs("node_ids", removed).
				Int("count", len(removed)).
				Msg("nodehub sweeper: evicted offline sessions")

			// A removed node may still have been an A-record target in
			// the active DNS zone. triggerDNSSync is debounced and a
			// no-op when DNS auto-sync is disabled, so it's safe to
			// call unconditionally here.
			s.triggerDNSSync("", "nodehub:cleanup")
		}
	}
}
