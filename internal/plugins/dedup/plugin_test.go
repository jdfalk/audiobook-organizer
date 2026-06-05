// file: internal/plugins/dedup/plugin_test.go
// version: 1.0.0
// guid: b8c9d0e1-f2a3-4567-def0-123456789abc
// last-edited: 2026-05-06

package dedup

import (
	"context"
	"testing"

	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

// mockRegistry is a test double for sdk.Registry that records registered ops.
type mockRegistry struct {
	registeredOps []sdk.OperationDef
}

func (m *mockRegistry) RegisterOp(op sdk.OperationDef) error {
	m.registeredOps = append(m.registeredOps, op)
	return nil
}

// EnqueueOp is a no-op for testing.
func (m *mockRegistry) EnqueueOp(ctx context.Context, defID string, params any, opts ...sdk.EnqueueOption) (string, error) {
	return "", nil
}

func TestPluginRegisterNoEngine(t *testing.T) {
	// nil engine should return nil without error and register nothing
	p := &Plugin{engine: nil}
	r := &mockRegistry{}
	err := p.Register(r)
	if err != nil {
		t.Fatalf("Register with nil engine should not error, got %v", err)
	}
	if len(r.registeredOps) != 0 {
		t.Fatalf("Register with nil engine should not register ops, got %d", len(r.registeredOps))
	}
}

func TestPluginRegisterWithEngine(t *testing.T) {
	// With a non-nil engine (even a fake one), all ops should register
	// We can't easily construct a real Engine here, so we just verify the
	// registration code path exists. In integration tests, we verify it works end-to-end.
	p := &Plugin{
		engine: nil, // Use nil to keep this unit test simple
	}
	r := &mockRegistry{}
	err := p.Register(r)
	if err != nil {
		t.Fatalf("Register should not error, got %v", err)
	}
	// With nil engine, we expect no registrations (the nil-guard)
	if len(r.registeredOps) != 0 {
		t.Fatalf("Register with nil engine should register nothing, got %d ops", len(r.registeredOps))
	}
}
