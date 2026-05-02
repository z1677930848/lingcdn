package server

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

// wafBanQueue batches WAF ban persistence so that heartbeat handlers never
// block on database I/O. Each heartbeat can report many bans; doing a
// synchronous INSERT per ban would multiply heartbeat latency by the number of
// bans and create an easy DoS vector (a misbehaving node can stream unlimited
// bans and saturate the DB connection pool).
//
// The queue has a bounded buffer. When the buffer is full (flush is slow or
// database is unreachable) new enqueues are dropped with a warning rather than
// blocking the caller. A dedicated goroutine drains the queue in batches on a
// fixed interval.
type wafBanQueue struct {
	ch       chan *store.WAFBan
	cap      int
	store    store.Store
	interval time.Duration
	batch    int
	stop     chan struct{}
	wg       sync.WaitGroup
}

// newWAFBanQueue creates a queue wired to the given store. `bufferSize` caps
// in-memory buffered bans; `batchSize` is how many bans are flushed per tick;
// `interval` is the tick cadence.
func newWAFBanQueue(st store.Store, bufferSize, batchSize int, interval time.Duration) *wafBanQueue {
	if bufferSize <= 0 {
		bufferSize = 4096
	}
	if batchSize <= 0 {
		batchSize = 128
	}
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	return &wafBanQueue{
		ch:       make(chan *store.WAFBan, bufferSize),
		cap:      bufferSize,
		store:    st,
		interval: interval,
		batch:    batchSize,
		stop:     make(chan struct{}),
	}
}

// enqueue attempts to put a ban on the queue without blocking. Returns true on
// success, false when the queue is full.
func (q *wafBanQueue) enqueue(b *store.WAFBan) bool {
	if q == nil || b == nil {
		return false
	}
	select {
	case q.ch <- b:
		return true
	default:
		return false
	}
}

// start launches the background writer. Safe to call at most once per queue.
func (q *wafBanQueue) start(ctx context.Context) {
	if q == nil || q.store == nil {
		return
	}
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		ticker := time.NewTicker(q.interval)
		defer ticker.Stop()

		buf := make([]*store.WAFBan, 0, q.batch)
		flush := func() {
			if len(buf) == 0 {
				return
			}
			// Use a fresh, bounded context so shutdown signals cancel flushing.
			fctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			for _, b := range buf {
				if err := q.store.CreateOrUpdateWAFBan(fctx, b); err != nil {
					log.Ctx(fctx).Warn().Err(err).Str("ip", b.IP).Msg("failed to persist WAF ban (async)")
				}
			}
			cancel()
			buf = buf[:0]
		}

		for {
			select {
			case <-ctx.Done():
				// Drain best-effort on shutdown.
				flush()
				for {
					select {
					case b := <-q.ch:
						buf = append(buf, b)
						if len(buf) >= q.batch {
							flush()
						}
					default:
						flush()
						return
					}
				}
			case <-q.stop:
				flush()
				return
			case b := <-q.ch:
				buf = append(buf, b)
				if len(buf) >= q.batch {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}()
}

// shutdown stops the background goroutine and waits for it to finish.
func (q *wafBanQueue) shutdown() {
	if q == nil {
		return
	}
	close(q.stop)
	q.wg.Wait()
}
