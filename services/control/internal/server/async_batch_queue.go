package server

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// asyncBatchQueue is a generic, bounded, non-blocking queue that batches items
// and flushes them via a caller-supplied function on a fixed interval or when
// the batch fills up. It generalises the pattern first introduced by
// wafBanQueue: hot paths (heartbeat, log-reporting, telemetry) enqueue without
// blocking on DB I/O, and a single background goroutine coalesces writes.
//
// Key design choices:
//   - Bounded channel with non-blocking enqueue: a slow flush or unreachable
//     DB cannot back-pressure heartbeat handlers. Overflow is dropped with a
//     logged warning rather than stalling the gRPC server.
//   - Batch flush takes the whole []T at once, giving the flush function the
//     opportunity to use a multi-row INSERT and collapse N round-trips into
//     one.
//   - Shutdown is best-effort: on ctx.Done() we drain what is already buffered
//     with a bounded secondary timeout, then return. This avoids the pitfall
//     of an unbounded wait that holds up process exit.
//
// Generics require Go 1.18+; this project already targets 1.25, so this is
// safe to use.
type asyncBatchQueue[T any] struct {
	name     string
	ch       chan T
	flush    func(context.Context, []T) error
	interval time.Duration
	batch    int
	stop     chan struct{}
	wg       sync.WaitGroup
}

// newAsyncBatchQueue wires up the queue but does not start the background
// goroutine. Call start(ctx) after construction.
//
//   - name is used only in logs to disambiguate multiple queues (e.g.
//     "node_logs", "waf_bans").
//   - flush is invoked with all pending items and is free to mutate or consume
//     the slice; it will not be retained after the call returns.
//   - bufferSize bounds the in-memory buffer; enqueue returns false when full.
//   - batchSize is the soft upper bound that triggers an immediate flush even
//     before the tick fires.
//   - interval is the tick cadence for time-based flushes.
func newAsyncBatchQueue[T any](
	name string,
	flush func(context.Context, []T) error,
	bufferSize, batchSize int,
	interval time.Duration,
) *asyncBatchQueue[T] {
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	if batchSize <= 0 {
		batchSize = 128
	}
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &asyncBatchQueue[T]{
		name:     name,
		ch:       make(chan T, bufferSize),
		flush:    flush,
		interval: interval,
		batch:    batchSize,
		stop:     make(chan struct{}),
	}
}

// enqueue non-blockingly offers an item to the queue. Returns true on success,
// false when the buffer is saturated — in which case the caller should log
// and move on rather than retry.
func (q *asyncBatchQueue[T]) enqueue(item T) bool {
	if q == nil || q.flush == nil {
		return false
	}
	select {
	case q.ch <- item:
		return true
	default:
		return false
	}
}

// start launches the background writer. Safe to call exactly once per queue;
// calling it twice will spawn two competing goroutines.
func (q *asyncBatchQueue[T]) start(ctx context.Context) {
	if q == nil || q.flush == nil {
		return
	}
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		ticker := time.NewTicker(q.interval)
		defer ticker.Stop()

		buf := make([]T, 0, q.batch)
		doFlush := func() {
			if len(buf) == 0 {
				return
			}
			// Use a fresh bounded context so shutdown cancellations propagate
			// but individual DB hangs cannot wedge the writer forever.
			fctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := q.flush(fctx, buf); err != nil {
				log.Ctx(fctx).Warn().
					Err(err).
					Str("queue", q.name).
					Int("batch", len(buf)).
					Msg("async batch flush failed")
			}
			cancel()
			buf = buf[:0]
		}

		// drainAndExit is called on both shutdown paths (ctx cancel, explicit
		// stop). It non-blockingly pulls everything still in the channel into
		// `buf`, then performs a single best-effort flush under a fresh
		// background context so the shutdown is not self-canceled when the
		// parent ctx has already expired.
		drainAndExit := func() {
			for {
				select {
				case item := <-q.ch:
					buf = append(buf, item)
				default:
					if len(buf) == 0 {
						return
					}
					drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					if err := q.flush(drainCtx, buf); err != nil {
						log.Warn().
							Err(err).
							Str("queue", q.name).
							Int("batch", len(buf)).
							Msg("async batch drain flush failed")
					}
					cancel()
					buf = buf[:0]
					return
				}
			}
		}

		for {
			select {
			case <-ctx.Done():
				drainAndExit()
				return
			case <-q.stop:
				drainAndExit()
				return
			case item := <-q.ch:
				buf = append(buf, item)
				if len(buf) >= q.batch {
					doFlush()
				}
			case <-ticker.C:
				doFlush()
			}
		}
	}()
}

// shutdown stops the background goroutine and waits for its final flush.
// Idempotent: safe to call more than once because the stop channel is only
// closed once under the mutex-free close-guard pattern used by sync.Once; we
// rely on callers treating shutdown as a single authoritative call (matching
// wafBanQueue's pre-existing convention).
func (q *asyncBatchQueue[T]) shutdown() {
	if q == nil {
		return
	}
	select {
	case <-q.stop:
		// already closed
	default:
		close(q.stop)
	}
	q.wg.Wait()
}
