// file: internal/serviceregistry/container_test.go
// version: 1.0.0

package serviceregistry

import (
	"context"
	"testing"
)

type fakeService struct {
	name     string
	postInit bool
	started  bool
	stopped  bool
}

func (f *fakeService) PostInit(ctx context.Context, c *Container) error {
	f.postInit = true
	return nil
}
func (f *fakeService) Start(ctx context.Context) error { f.started = true; return nil }
func (f *fakeService) Stop(ctx context.Context) error  { f.stopped = true; return nil }

func TestContainer_Build_RunsInDepOrder(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	order := []string{}
	Register(ServiceDef{Name: "leaf", Build: func(c *Container) (any, error) {
		order = append(order, "leaf")
		return &fakeService{name: "leaf"}, nil
	}})
	Register(ServiceDef{Name: "top", Needs: []string{"leaf"}, Build: func(c *Container) (any, error) {
		_ = Get[*fakeService](c, "leaf") // verify Get works mid-Build
		order = append(order, "top")
		return &fakeService{name: "top"}, nil
	}})

	c := NewContainer().Include("top")
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(order) != 2 || order[0] != "leaf" || order[1] != "top" {
		t.Fatalf("build order = %v, want [leaf top]", order)
	}
}

func TestContainer_Get_UndeclaredDepPanics(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	Register(ServiceDef{Name: "leaf", Build: func(c *Container) (any, error) {
		return "leaf-instance", nil
	}})
	// "naughty" does NOT declare "leaf" in Needs but tries to Get it.
	Register(ServiceDef{Name: "naughty", Build: func(c *Container) (any, error) {
		_ = Get[string](c, "leaf") // should panic
		return nil, nil
	}})

	c := NewContainer().Include("leaf", "naughty")
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for undeclared Get")
		}
	}()
	_ = c.Build(t.Context())
}

func TestContainer_PostInit_RunsAfterAllBuilds(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	Register(ServiceDef{Name: "a", Build: func(c *Container) (any, error) { return &fakeService{name: "a"}, nil }})
	Register(ServiceDef{Name: "b", Needs: []string{"a"}, Build: func(c *Container) (any, error) { return &fakeService{name: "b"}, nil }})

	c := NewContainer().Include("a", "b")
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := c.PostInit(t.Context()); err != nil {
		t.Fatalf("postinit: %v", err)
	}

	a := Get[*fakeService](c, "a")
	b := Get[*fakeService](c, "b")
	if !a.postInit || !b.postInit {
		t.Fatalf("postinit not called: a=%v b=%v", a.postInit, b.postInit)
	}
}

func TestContainer_Stop_ReverseOrder(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	stopOrder := []string{}
	Register(ServiceDef{Name: "a", Build: func(c *Container) (any, error) {
		return &recordingService{name: "a", sink: &stopOrder}, nil
	}})
	Register(ServiceDef{Name: "b", Needs: []string{"a"}, Build: func(c *Container) (any, error) {
		return &recordingService{name: "b", sink: &stopOrder}, nil
	}})
	Register(ServiceDef{Name: "c", Needs: []string{"b"}, Build: func(c *Container) (any, error) {
		return &recordingService{name: "c", sink: &stopOrder}, nil
	}})

	c := NewContainer().Include("c")
	if err := c.Build(t.Context()); err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := c.PostInit(t.Context()); err != nil {
		t.Fatalf("postinit: %v", err)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatalf("stop: %v", err)
	}

	want := []string{"c", "b", "a"}
	if len(stopOrder) != 3 {
		t.Fatalf("stop order = %v, want %v", stopOrder, want)
	}
	for i := range want {
		if stopOrder[i] != want[i] {
			t.Errorf("stopOrder[%d] = %q, want %q", i, stopOrder[i], want[i])
		}
	}
}

type recordingService struct {
	name string
	sink *[]string
}

func (r *recordingService) Start(ctx context.Context) error { return nil }
func (r *recordingService) Stop(ctx context.Context) error {
	*r.sink = append(*r.sink, r.name)
	return nil
}
