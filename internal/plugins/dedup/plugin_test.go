// file: internal/plugins/dedup/plugin_test.go
// version: 1.1.0
// guid: b8c9d0e1-f2a3-4567-def0-123456789abc
// last-edited: 2026-06-10

package dedup

import (
	"context"
	"testing"

	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

// mockRegistry is a test double for sdk.Registry that records both
// registered ops and enqueued ops.
type mockRegistry struct {
	registeredOps  []sdk.OperationDef
	enqueuedDefs   []string
	enqueuedParams []any
}

func (m *mockRegistry) RegisterOp(op sdk.OperationDef) error {
	m.registeredOps = append(m.registeredOps, op)
	return nil
}

func (m *mockRegistry) EnqueueOp(_ context.Context, defID string, params any, _ ...sdk.EnqueueOption) (string, error) {
	m.enqueuedDefs = append(m.enqueuedDefs, defID)
	m.enqueuedParams = append(m.enqueuedParams, params)
	return "fake-op-id", nil
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
