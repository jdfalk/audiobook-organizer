// file: internal/cache/cache_test.go
// version: 1.1.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package cache

import (
	"testing"
	"time"
)

func TestGetSet(t *testing.T) {
	c := New[string]("test_getset", time.Minute)
	c.Set("k", "v")
	v, ok := c.Get("k")
	if !ok || v != "v" {
		t.Fatalf("expected v, got %q ok=%v", v, ok)
	}
}

func TestExpiry(t *testing.T) {
	c := New[int]("test_expiry", time.Millisecond)
	c.Set("k", 42)
	time.Sleep(5 * time.Millisecond)
	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected expired entry")
	}
}

func TestInvalidate(t *testing.T) {
	c := New[string]("test_invalidate", time.Minute)
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
	c := New[int]("test_invalidate_all", time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)
	c.InvalidateAll()
	_, ok := c.Get("a")
	if ok {
		t.Fatal("expected all invalidated")
	}
}

func TestName(t *testing.T) {
	c := New[int]("named", time.Minute)
	if got := c.Name(); got != "named" {
		t.Fatalf("expected name=named, got %q", got)
	}
}
