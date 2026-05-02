package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAsyncBatchQueue_FlushesByBatchSize verifies that reaching batchSize
// triggers an immediate flush rather than waiting for the tick.
func TestAsyncBatchQueue_FlushesByBatchSize(t *testing.T) {
	var (
		mu      sync.Mutex
		flushed [][]int
	)
	flush := func(_ context.Context, items []int) error {
		mu.Lock()
		// copy the slice so the producer can't mutate what we capture.
		cp := make([]int, len(items))
		copy(cp, items)
		flushed = append(flushed, cp)
		mu.Unlock()
		return nil
	}

	// Large interval so we only see batch-triggered flushes.
	q := newAsyncBatchQueue[int]("test-batch", flush, 128, 4, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.start(ctx)
	defer q.shutdown()

	for i := 0; i < 4; i++ {
		if !q.enqueue(i) {
			t.Fatalf("enqueue %d unexpectedly failed", i)
		}
	}

	// Wait up to 1s for the batch flush to land; use polling to avoid
	// flakiness on slow CI.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := len(flushed) > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 || len(flushed[0]) != 4 {
		t.Fatalf("want 1 flush of 4 items, got %v", flushed)
	}
}

// TestAsyncBatchQueue_FlushesByInterval verifies a tick flushes whatever is
// buffered even when the batch hasn't filled.
func TestAsyncBatchQueue_FlushesByInterval(t *testing.T) {
	var count int32
	flush := func(_ context.Context, items []int) error {
		atomic.AddInt32(&count, int32(len(items)))
		return nil
	}

	// Small interval, large batch — tick should fire before batch fills.
	q := newAsyncBatchQueue[int]("test-interval", flush, 128, 1024, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.start(ctx)
	defer q.shutdown()

	for i := 0; i < 3; i++ {
		q.enqueue(i)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&count) == 3 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("interval flush did not fire: count=%d", atomic.LoadInt32(&count))
}

// TestAsyncBatchQueue_EnqueueDropsWhenFull verifies the non-blocking contract:
// once the buffer is saturated, enqueue must return false rather than block
// the caller (which, in production, is a gRPC heartbeat handler).
func TestAsyncBatchQueue_EnqueueDropsWhenFull(t *testing.T) {
	// Flush that never returns until the test signals it to. This parks
	// one item in the worker goroutine's internal buffer and lets us fill
	// the channel.
	release := make(chan struct{})
	flush := func(_ context.Context, _ []int) error {
		<-release
		return nil
	}

	// Tiny buffer so we can fill it; batchSize=1 ensures the first enqueue
	// enters the flush immediately (parking the worker on <-release).
	q := newAsyncBatchQueue[int]("test-drop", flush, 2, 1, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.start(ctx)

	// Give the worker a moment to start and consume the first enqueue,
	// then park inside flush waiting for release.
	first := q.enqueue(100)
	if !first {
		t.Fatal("first enqueue should have succeeded")
	}
	// Wait until the worker has consumed the first item (parked in flush).
	time.Sleep(50 * time.Millisecond)

	// Now fill the buffer (cap=2). These go into the channel, not into flush.
	if !q.enqueue(1) {
		t.Fatal("enqueue 1 should succeed (buffer not full)")
	}
	if !q.enqueue(2) {
		t.Fatal("enqueue 2 should succeed (buffer not full)")
	}

	// This must be dropped (non-blocking).
	start := time.Now()
	ok := q.enqueue(3)
	elapsed := time.Since(start)
	if ok {
		t.Fatal("enqueue 3 should have been dropped (buffer full)")
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("enqueue blocked for %v, must be non-blocking", elapsed)
	}

	close(release)
	q.shutdown()
}

// TestAsyncBatchQueue_ShutdownFlushesPending verifies that items sitting in
// the buffer at shutdown time are still flushed on a best-effort basis.
func TestAsyncBatchQueue_ShutdownFlushesPending(t *testing.T) {
	var count int32
	flush := func(_ context.Context, items []int) error {
		atomic.AddInt32(&count, int32(len(items)))
		return nil
	}

	// Long interval + large batch so nothing flushes via tick/batch; we
	// want to observe shutdown-path flushing specifically.
	q := newAsyncBatchQueue[int]("test-shutdown", flush, 64, 1024, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	q.start(ctx)

	for i := 0; i < 5; i++ {
		q.enqueue(i)
	}

	// Trigger shutdown via ctx cancel — this exercises the ctx.Done() drain
	// path in the worker rather than the explicit stop channel.
	cancel()

	// Wait for the worker to finish draining.
	q.shutdown()

	if atomic.LoadInt32(&count) != 5 {
		t.Fatalf("want 5 items flushed on shutdown, got %d", atomic.LoadInt32(&count))
	}
}

// TestAsyncBatchQueue_FlushErrorDoesNotCrash verifies the queue keeps running
// after a flush error: it logs and moves on, matching wafBanQueue's
// best-effort semantics. Lost items are acceptable here because the data
// (log entries, WAF bans) is inherently lossy and we'd rather stay up than
// deadlock the control plane on a slow DB.
func TestAsyncBatchQueue_FlushErrorDoesNotCrash(t *testing.T) {
	var (
		calls int32
		fail  atomic.Bool
	)
	fail.Store(true)
	flush := func(_ context.Context, items []int) error {
		atomic.AddInt32(&calls, 1)
		if fail.Load() {
			return errors.New("simulated flush failure")
		}
		return nil
	}

	q := newAsyncBatchQueue[int]("test-err", flush, 64, 1, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.start(ctx)
	defer q.shutdown()

	// First enqueue: flush fails, but worker must keep going.
	q.enqueue(1)
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&calls) == 0 {
		t.Fatal("flush was never called after first enqueue")
	}

	// Now let subsequent flushes succeed.
	fail.Store(false)
	q.enqueue(2)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&calls) >= 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("worker did not recover from flush error: calls=%d", atomic.LoadInt32(&calls))
}
