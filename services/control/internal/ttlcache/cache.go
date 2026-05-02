package ttlcache

import (
	"hash/fnv"
	"sync"
	"time"
)

type Option func(*options)

type options struct {
	shards     int
	gcInterval time.Duration
}

func WithShards(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.shards = n
		}
	}
}

func WithGCInterval(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.gcInterval = d
		}
	}
}

type entry[V any] struct {
	value    V
	expireAt int64
}

type shard[V any] struct {
	mu    sync.Mutex
	items map[string]entry[V]
}

type Cache[V any] struct {
	shards []*shard[V]

	gcStop chan struct{}
	gcDone chan struct{}
	once   sync.Once
}

func New[V any](opts ...Option) *Cache[V] {
	cfg := options{
		shards:     128,
		gcInterval: 5 * time.Second,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.shards <= 0 {
		cfg.shards = 128
	}

	c := &Cache[V]{
		shards: make([]*shard[V], cfg.shards),
		gcStop: make(chan struct{}),
		gcDone: make(chan struct{}),
	}
	for i := 0; i < cfg.shards; i++ {
		c.shards[i] = &shard[V]{items: make(map[string]entry[V])}
	}

	if cfg.gcInterval > 0 {
		go c.gcLoop(cfg.gcInterval)
	} else {
		close(c.gcDone)
	}
	return c
}

func (c *Cache[V]) Close() {
	c.once.Do(func() {
		close(c.gcStop)
		<-c.gcDone
	})
}

func (c *Cache[V]) Set(key string, value V, ttl time.Duration) {
	if ttl <= 0 {
		c.Delete(key)
		return
	}
	expireAt := time.Now().Add(ttl).UnixNano()
	s := c.pickShard(key)
	s.mu.Lock()
	s.items[key] = entry[V]{value: value, expireAt: expireAt}
	s.mu.Unlock()
}

func (c *Cache[V]) Get(key string) (V, bool) {
	var zero V
	s := c.pickShard(key)
	now := time.Now().UnixNano()
	s.mu.Lock()
	e, ok := s.items[key]
	if !ok {
		s.mu.Unlock()
		return zero, false
	}
	if e.expireAt > 0 && now > e.expireAt {
		delete(s.items, key)
		s.mu.Unlock()
		return zero, false
	}
	s.mu.Unlock()
	return e.value, true
}

func (c *Cache[V]) Delete(key string) {
	s := c.pickShard(key)
	s.mu.Lock()
	delete(s.items, key)
	s.mu.Unlock()
}

func (c *Cache[V]) Len() int {
	total := 0
	for _, s := range c.shards {
		s.mu.Lock()
		total += len(s.items)
		s.mu.Unlock()
	}
	return total
}

func (c *Cache[V]) PurgeExpired() int {
	now := time.Now().UnixNano()
	removed := 0
	for _, s := range c.shards {
		s.mu.Lock()
		for k, e := range s.items {
			if e.expireAt > 0 && now > e.expireAt {
				delete(s.items, k)
				removed++
			}
		}
		s.mu.Unlock()
	}
	return removed
}

func (c *Cache[V]) gcLoop(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	defer close(c.gcDone)

	for {
		select {
		case <-c.gcStop:
			return
		case <-t.C:
			c.PurgeExpired()
		}
	}
}

func (c *Cache[V]) pickShard(key string) *shard[V] {
	if len(c.shards) == 1 {
		return c.shards[0]
	}
	return c.shards[int(hashKey(key)%uint64(len(c.shards)))]
}

func hashKey(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

