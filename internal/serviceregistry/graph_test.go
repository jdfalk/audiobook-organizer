// file: internal/serviceregistry/graph_test.go
// version: 1.0.0

package serviceregistry

import (
	"errors"
	"testing"
)

func TestResolve_LexStableOrder(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	// Register A, B, C, D all depending on nothing. Lex order should be A,B,C,D.
	for _, name := range []string{"zebra", "alpha", "mike", "delta"} {
		n := name
		Register(ServiceDef{
			Name:  n,
			Build: func(c *Container) (any, error) { return n, nil },
		})
	}

	c := NewContainer().IncludeAll()
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := c.Names()
	want := []string{"alpha", "delta", "mike", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolve_CycleDetected(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	Register(ServiceDef{Name: "a", Needs: []string{"b"}, Build: func(c *Container) (any, error) { return nil, nil }})
	Register(ServiceDef{Name: "b", Needs: []string{"a"}, Build: func(c *Container) (any, error) { return nil, nil }})

	c := NewContainer().Include("a")
	err := c.Resolve()
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
}

func TestResolve_UnknownDep(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	Register(ServiceDef{Name: "a", Needs: []string{"nonexistent"}, Build: func(c *Container) (any, error) { return nil, nil }})

	c := NewContainer().Include("a")
	err := c.Resolve()
	if !errors.Is(err, ErrUnknownService) {
		t.Fatalf("expected ErrUnknownService, got %v", err)
	}
}

func TestResolve_TransitiveClosure(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	Register(ServiceDef{Name: "leaf", Build: func(c *Container) (any, error) { return "leaf", nil }})
	Register(ServiceDef{Name: "mid", Needs: []string{"leaf"}, Build: func(c *Container) (any, error) { return "mid", nil }})
	Register(ServiceDef{Name: "top", Needs: []string{"mid"}, Build: func(c *Container) (any, error) { return "top", nil }})

	// Include only "top" — should pull mid + leaf transitively.
	c := NewContainer().Include("top")
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := c.Names()
	want := []string{"leaf", "mid", "top"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolve_OverrideIsLeaf(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	// "real" needs "store"; "store" is not registered, only overridden.
	Register(ServiceDef{Name: "real", Needs: []string{"store"}, Build: func(c *Container) (any, error) {
		return Get[string](c, "store"), nil
	}})

	c := NewContainer().Override("store", "mock-store").Include("real")
	if err := c.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := c.Names()
	// store appears first (no deps), then real.
	if len(got) != 2 || got[0] != "store" || got[1] != "real" {
		t.Fatalf("got %v, want [store real]", got)
	}
}
