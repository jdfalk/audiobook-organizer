// file: internal/cache/cache_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package cache

import (
	"testing"
	"time"
)

func TestGetSet(t *testing.T) {
	c := New[string](time.Minute)
	c.Set("k", "v")
	v, ok := c.Get("k")
	if !ok || v != "v" {
		t.Fatalf("expected v, got %q ok=%v", v, ok)
	}
}

func TestExpiry(t *testing.T) {
	c := New[int](time.Millisecond)
	c.Set("k", 42)
	time.Sleep(5 * time.Millisecond)
	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected expired entry")
	}
}

func TestInvalidate(t *testing.T) {
	c := New[string](time.Minute)
	c.Set("a", "1")
	c.Set("b", "2")
	c.Invalidate("a")
	_, ok := c.Get("a")
	if ok {
		t.Fatal("expected a to be invalidated")
	}
	v, ok := c.Get("b")
	if !ok || v != "2" {
		t.Fatal("expected b to remain")
	}
}

func TestInvalidateAll(t *testing.T) {
	c := New[int](time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)
	c.InvalidateAll()
	_, ok := c.Get("a")
	if ok {
		t.Fatal("expected all invalidated")
	}
}
