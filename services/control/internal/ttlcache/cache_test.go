package ttlcache

import (
	"testing"
	"time"
)

func TestCache_SetGet(t *testing.T) {
	c := New[string](WithShards(8), WithGCInterval(0))
	t.Cleanup(c.Close)

	c.Set("k1", "v1", 200*time.Millisecond)
	v, ok := c.Get("k1")
	if !ok {
		t.Fatalf("expected ok")
	}
	if v != "v1" {
		t.Fatalf("v=%s", v)
	}
}

func TestCache_Expire(t *testing.T) {
	c := New[string](WithShards(4), WithGCInterval(0))
	t.Cleanup(c.Close)

	c.Set("k1", "v1", 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	_, ok := c.Get("k1")
	if ok {
		t.Fatalf("expected expired")
	}
}

func TestCache_Delete(t *testing.T) {
	c := New[string](WithShards(4), WithGCInterval(0))
	t.Cleanup(c.Close)

	c.Set("k1", "v1", 200*time.Millisecond)
	c.Delete("k1")
	_, ok := c.Get("k1")
	if ok {
		t.Fatalf("expected deleted")
	}
}

func TestCache_PurgeExpired(t *testing.T) {
	c := New[int](WithShards(8), WithGCInterval(0))
	t.Cleanup(c.Close)

	c.Set("a", 1, 10*time.Millisecond)
	c.Set("b", 2, 200*time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	removed := c.PurgeExpired()
	if removed != 1 {
		t.Fatalf("removed=%d", removed)
	}
	if c.Len() != 1 {
		t.Fatalf("len=%d", c.Len())
	}
}

func TestCache_CloseIdempotent(t *testing.T) {
	c := New[int](WithShards(2))
	c.Close()
	c.Close()
}

