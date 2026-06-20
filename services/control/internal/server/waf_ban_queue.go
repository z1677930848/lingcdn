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
// database is unreachable) enqueue waits briefly before dropping with a warning
// rather than blocking the caller indefinitely. A dedicated goroutine drains the queue in batches on a
// fixed interval.
type wafBanQueue struct {
	ch       chan *store.WAFBan
	cap      int
	store    store.Store
	interval time.Duration
	batch    int
	onFlush  func(context.Context, int)
	stop     chan struct{}
	wg       sync.WaitGroup
}

// newWAFBanQueue creates a queue wired to the given store. `bufferSize` caps
// in-memory buffered bans; `batchSize` is how many bans are flushed per tick;
// `interval` is the tick cadence.
func newWAFBanQueue(st store.Store, bufferSize, batchSize int, interval time.Duration, onFlush func(context.Context, int)) *wafBanQueue {
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
		onFlush:  onFlush,
		stop:     make(chan struct{}),
	}
}

// enqueue attempts to put a ban on the queue. It tries a non-blocking send
// first, then waits up to enqueueWait when the buffer is full. Returns true on
// success, false when the queue remains full after the wait.
const enqueueWait = 100 * time.Millisecond

func (q *wafBanQueue) enqueue(b *store.WAFBan) bool {
	if q == nil || b == nil {
		return false
	}
	select {
	case q.ch <- b:
		return true
	default:
	}
	timer := time.NewTimer(enqueueWait)
	defer timer.Stop()
	select {
	case q.ch <- b:
		return true
	case <-timer.C:
		log.Warn().Str("ip", b.IP).Dur("wait", enqueueWait).Msg("WAF ban queue full, dropping report")
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
		var lastPublish time.Time
		flush := func() {
			if len(buf) == 0 {
				return
			}
			// IMPORTANT: derive the flush ctx from context.Background(), NOT
			// the long-lived `ctx` passed to start(). That ctx is typically
			// the server's root context; during shutdown it is cancelled
			// *before* close(q.stop) fires, so inheriting from it would
			// make every CreateOrUpdateWAFBan return ctx.Cancelled and the
			// "drain best-effort on shutdown" branch below would drop
			// everything it was trying to save. A dedicated 5s budget lets
			// us finish the drain even after the outer ctx is gone, while
			// still bounding the work in the normal in-process path.
			fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			persisted := 0
			for _, b := range buf {
				if err := q.store.CreateOrUpdateWAFBan(fctx, b); err != nil {
					log.Ctx(fctx).Warn().Err(err).Str("ip", b.IP).Msg("failed to persist WAF ban (async)")
				} else {
					persisted++
				}
			}
			if persisted > 0 && q.onFlush != nil && time.Since(lastPublish) >= 2*time.Second {
				lastPublish = time.Now()
				q.onFlush(context.Background(), persisted)
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
	select {
	case <-q.stop:
		// already closed
	default:
		close(q.stop)
	}
	q.wg.Wait()
}
