package nodehub

import (
	"context"
	"testing"
	"time"

	controlpb "github.com/lingcdn/control/proto/gen"
	"google.golang.org/grpc/metadata"
)

// stubConfigStream is a minimal implementation of
// controlpb.NodeControl_StreamConfigServer for equality tests. Using a
// pointer receiver + pointer value in the interface gives us two distinct
// interface values when we allocate separate instances.
type stubConfigStream struct{}

func (*stubConfigStream) Send(*controlpb.ConfigEnvelope) error { return nil }
func (*stubConfigStream) Recv() (*controlpb.ConfigAck, error)  { return nil, nil }
func (*stubConfigStream) SetHeader(metadata.MD) error          { return nil }
func (*stubConfigStream) SendHeader(metadata.MD) error         { return nil }
func (*stubConfigStream) SetTrailer(metadata.MD)               {}
func (*stubConfigStream) Context() context.Context             { return context.Background() }
func (*stubConfigStream) SendMsg(any) error                    { return nil }
func (*stubConfigStream) RecvMsg(any) error                    { return nil }

// TestClearConfigStream_PreservesSession is the regression test for the
// silent-dropped-session bug: the old StreamConfig defer used to call
// hub.Remove on every disconnect, so the node agent's reconnect would end
// up with no hub session and all subsequent Publisher.Publish calls
// quietly lost their target. ClearConfigStream must only unbind the stream
// and leave the session in the hub for future re-bind.
func TestClearConfigStream_PreservesSession(t *testing.T) {
	h := New()
	ctx := context.Background()

	h.Add(ctx, &Session{NodeID: "n1", Hostname: "node-1", Status: "online"})
	stream := &stubConfigStream{}

	if !h.SetConfigStream("n1", stream, nil) {
		t.Fatalf("SetConfigStream: expected true on existing session")
	}
	if !h.ClearConfigStream("n1", stream) {
		t.Fatalf("ClearConfigStream: expected true when stream matches")
	}

	// Session must still be in the hub so a later SetConfigStream can
	// rebind without requiring a full RegisterNode round-trip.
	sess, ok := h.Get("n1")
	if !ok || sess == nil {
		t.Fatalf("session was removed; ClearConfigStream must preserve it")
	}
	if sess.ConfigStream != nil {
		t.Fatalf("ConfigStream was not cleared")
	}
	// Re-binding should succeed.
	stream2 := &stubConfigStream{}
	if !h.SetConfigStream("n1", stream2, nil) {
		t.Fatalf("SetConfigStream after clear: expected true (session should still exist)")
	}
	if err := h.SendConfig("n1", &controlpb.ConfigEnvelope{Version: "v1"}); err != nil {
		t.Fatalf("SendConfig after rebind: unexpected error %v", err)
	}
}

// TestClearConfigStream_NoSession covers the defer-fires-after-Remove race:
// ClearConfigStream must be a no-op when the session has already been
// removed (for example by a concurrent admin delete) — it must not
// resurrect the session, it must not panic, and it must return false.
func TestClearConfigStream_NoSession(t *testing.T) {
	h := New()
	stream := &stubConfigStream{}
	if h.ClearConfigStream("ghost", stream) {
		t.Fatalf("ClearConfigStream on missing session should return false")
	}
}

// TestClearConfigStream_MismatchStream guards against the fast-reconnect
// race: the old stream goroutine's defer may fire after the node has
// already reconnected and installed a newer stream. In that case
// ClearConfigStream must not touch the newly-installed binding.
func TestClearConfigStream_MismatchStream(t *testing.T) {
	h := New()
	ctx := context.Background()

	h.Add(ctx, &Session{NodeID: "n1", Hostname: "node-1"})

	oldStream := &stubConfigStream{}
	newStream := &stubConfigStream{}
	h.SetConfigStream("n1", newStream, nil)

	if h.ClearConfigStream("n1", oldStream) {
		t.Fatalf("ClearConfigStream should return false when stream differs")
	}

	sess, _ := h.Get("n1")
	if sess == nil {
		t.Fatalf("session disappeared; mismatch-stream path must be a no-op")
	}
	if sess.ConfigStream != controlpb.NodeControl_StreamConfigServer(newStream) {
		t.Fatalf("new stream was incorrectly cleared")
	}
}

// TestSendConfig_DetectsClearedStream documents the invariant that
// Publisher relies on: after ClearConfigStream, SendConfig must report
// ErrNodeNotConnected so the publish task accounts it as a failure
// (rather than silently succeeding).
func TestSendConfig_DetectsClearedStream(t *testing.T) {
	h := New()
	ctx := context.Background()

	h.Add(ctx, &Session{NodeID: "n1"})
	stream := &stubConfigStream{}
	h.SetConfigStream("n1", stream, nil)
	h.ClearConfigStream("n1", stream)

	err := h.SendConfig("n1", &controlpb.ConfigEnvelope{Version: "v1"})
	if err == nil {
		t.Fatalf("SendConfig: expected error after stream cleared, got nil")
	}
	if err != ErrNodeNotConnected {
		t.Fatalf("SendConfig: expected ErrNodeNotConnected, got %v", err)
	}
}

// TestSweepOffline covers the four cases the sweeper must get right:
//  1. active stream → keep (never evict a reachable node)
//  2. stream cleared but heartbeat fresh → keep (agent is between
//     reconnects, evicting would cause a UI flap)
//  3. stream cleared AND heartbeat stale → evict (host is gone)
//  4. multiple sessions mixed → evict only the truly-offline ones
//
// The fourth case guards the "partial sweep" behaviour that the
// Servers.hubSessionSweeper callsite depends on when computing the
// trigger list for DNS resync.
func TestSweepOffline(t *testing.T) {
	h := New()
	now := time.Now()

	// Note: we bypass Hub.Add because Add unconditionally resets
	// LastSeen to time.Now(), which would defeat the whole point of
	// this test. Writing directly into h.sessions is legal because the
	// test is in the same package as the Hub implementation.
	activeStream := &stubConfigStream{}
	h.sessions = map[string]*Session{
		// 1. Active stream — must never be swept, even with ancient LastSeen.
		"active": {
			NodeID:       "active",
			Hostname:     "a",
			LastSeen:     now.Add(-1 * time.Hour),
			ConfigStream: activeStream,
		},
		// 2. Disconnected but heart-beating within the window — must stay,
		//    this is a node in the middle of a reconnect.
		"reconnecting": {
			NodeID:   "reconnecting",
			Hostname: "r",
			LastSeen: now.Add(-30 * time.Second),
		},
		// 3. Disconnected AND silent past the window — must be evicted.
		"dead": {
			NodeID:   "dead",
			Hostname: "d",
			LastSeen: now.Add(-10 * time.Minute),
		},
		// 4. Another dead one to verify the return list is complete.
		"dead2": {
			NodeID:   "dead2",
			Hostname: "d2",
			LastSeen: now.Add(-15 * time.Minute),
		},
	}

	removed := h.SweepOffline(5 * time.Minute)

	if len(removed) != 2 {
		t.Fatalf("SweepOffline: expected 2 evictions, got %d (%v)", len(removed), removed)
	}
	evicted := map[string]bool{}
	for _, id := range removed {
		evicted[id] = true
	}
	if !evicted["dead"] || !evicted["dead2"] {
		t.Fatalf("SweepOffline: missing expected evictions dead/dead2, got %v", removed)
	}

	if _, ok := h.Get("active"); !ok {
		t.Fatalf("SweepOffline must not evict active streams")
	}
	if _, ok := h.Get("reconnecting"); !ok {
		t.Fatalf("SweepOffline must not evict fresh-heartbeat reconnecting nodes")
	}
	if _, ok := h.Get("dead"); ok {
		t.Fatalf("SweepOffline failed to evict dead session")
	}
	if _, ok := h.Get("dead2"); ok {
		t.Fatalf("SweepOffline failed to evict dead2 session")
	}
}

// TestSweepOffline_EmptyHub ensures the sweeper is a harmless no-op on
// an empty hub (e.g. control plane just started, no node registered yet).
// Returning a nil slice is intentional and lets the caller short-circuit
// on `if len(removed) == 0`.
func TestSweepOffline_EmptyHub(t *testing.T) {
	h := New()
	removed := h.SweepOffline(time.Minute)
	if len(removed) != 0 {
		t.Fatalf("SweepOffline on empty hub: expected nil, got %v", removed)
	}
}
