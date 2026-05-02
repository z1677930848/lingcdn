package taskutils

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunConcurrent_Limit(t *testing.T) {
	items := make([]int, 50)
	for i := range items {
		items[i] = i
	}

	var current int64
	var maxSeen int64

	err := RunConcurrent(context.Background(), items, 5, func(_ context.Context, _ int) error {
		n := atomic.AddInt64(&current, 1)
		for {
			old := atomic.LoadInt64(&maxSeen)
			if n <= old {
				break
			}
			if atomic.CompareAndSwapInt64(&maxSeen, old, n) {
				break
			}
		}

		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&current, -1)
		return nil
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if atomic.LoadInt64(&maxSeen) > 5 {
		t.Fatalf("maxSeen=%d", atomic.LoadInt64(&maxSeen))
	}
}

