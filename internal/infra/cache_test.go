package infra

import (
	"testing"
	"time"
)

func TestCache_HitAndMiss(t *testing.T) {
	c := NewCache[string](10*time.Second, 10)

	c.Put("key1", "value1")

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "value1" {
		t.Fatalf("expected value1, got %s", val)
	}

	_, ok = c.Get("missing")
	if ok {
		t.Fatal("expected cache miss for nonexistent key")
	}
}

func TestCache_TTLExpiration(t *testing.T) {
	c := NewCache[string](1*time.Millisecond, 10)

	c.Put("key1", "value1")
	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected cache miss after TTL expiration")
	}
}

func TestCache_Eviction(t *testing.T) {
	c := NewCache[string](10*time.Second, 2)

	c.Put("key1", "val1")
	c.Put("key2", "val2")
	c.Put("key3", "val3") // should evict key1 (oldest)

	if c.Len() > 2 {
		t.Fatalf("expected max 2 entries, got %d", c.Len())
	}

	// key3 should be present
	val, ok := c.Get("key3")
	if !ok || val != "val3" {
		t.Fatal("expected key3 to be in cache")
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := NewCache[string](10*time.Second, 10)

	c.Put("key1", "val1")
	c.Invalidate("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected cache miss after invalidation")
	}
}

func TestCache_Clear(t *testing.T) {
	c := NewCache[int](10*time.Second, 10)

	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)
	c.Clear()

	if c.Len() != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", c.Len())
	}
}

func TestCacheKey_Deterministic(t *testing.T) {
	k1 := CacheKey("/some/path", "opts1")
	k2 := CacheKey("/some/path", "opts1")
	if k1 != k2 {
		t.Fatalf("expected deterministic keys, got %s and %s", k1, k2)
	}
}

func TestCacheKey_DifferentOptions(t *testing.T) {
	k1 := CacheKey("/some/path", "opts1")
	k2 := CacheKey("/some/path", "opts2")
	if k1 == k2 {
		t.Fatal("expected different keys for different options")
	}
}
