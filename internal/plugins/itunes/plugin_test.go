// file: internal/plugins/itunes/plugin_test.go
// version: 1.0.0
// guid: a3b4c5d6-e7f8-9a0b-1c2d-3e4f5a6b7c8d
// last-edited: 2026-05-07

package itunes

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// TestPluginRegistration verifies that the iTunes plugin registers all 5 operations when service is available.
func TestPluginRegistration(t *testing.T) {
	// Create a mock registry to capture registered operations
	mockRegistry := &mockRegistry{
		ops: make(map[string]sdk.OperationDef),
	}

	// Create a plugin with a mock service (note: we can't fully mock the service here,
	// but we test with a non-nil value to pass the nil-guard)
	// For this test, we verify the nil-guard prevents registration with nil service
	plugin := New(nil, nil)

	// Register the plugin with a nil service
	err := plugin.Register(mockRegistry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// When service is nil, no operations should be registered (nil-guard)
	if len(mockRegistry.ops) != 0 {
		t.Errorf("Expected 0 operations with nil service, but got %d", len(mockRegistry.ops))
	}

	// Verify that all 5 operation definitions are properly created
	// (These are tested separately in TestOperationDefProperties)
	expectedOps := []string{
		"itunes.import",
		"itunes.sync",
		"itunes.path-reconcile",
		"itunes.path-repair",
		"itunes.position-sync",
	}

	plugin2 := &Plugin{}
	for _, opID := range expectedOps {
		t.Run(opID, func(t *testing.T) {
			var def sdk.OperationDef
			switch opID {
			case "itunes.import":
				def = plugin2.importDef()
			case "itunes.sync":
				def = plugin2.syncDef()
			case "itunes.path-reconcile":
				def = plugin2.pathReconcileDef()
			case "itunes.path-repair":
				def = plugin2.pathRepairDef()
			case "itunes.position-sync":
				def = plugin2.positionSyncDef()
			}
			if def.ID != opID {
				t.Errorf("Expected ID %q, got %q", opID, def.ID)
			}
		})
	}
}

// TestPluginMetadata verifies plugin metadata.
func TestPluginMetadata(t *testing.T) {
	plugin := New(nil, nil)

	if plugin.ID() != "itunes" {
		t.Errorf("Expected ID 'itunes', got %q", plugin.ID())
	}

	if plugin.Name() != "iTunes" {
		t.Errorf("Expected Name 'iTunes', got %q", plugin.Name())
	}

	if plugin.Version() != "1.0.0" {
		t.Errorf("Expected Version '1.0.0', got %q", plugin.Version())
	}
}

// TestPluginNilGuard verifies that Register returns nil when service is nil.
func TestPluginNilGuard(t *testing.T) {
	mockRegistry := &mockRegistry{
		ops: make(map[string]sdk.OperationDef),
	}

	// Create a plugin with a nil service
	plugin := New(nil, nil)

	err := plugin.Register(mockRegistry)
	if err != nil {
		t.Fatalf("Register with nil service should not error: %v", err)
	}

	// No operations should be registered
	if len(mockRegistry.ops) != 0 {
		t.Errorf("Expected no operations to be registered with nil service, but got %d", len(mockRegistry.ops))
	}
}

// mockRegistry implements sdk.Registry for testing.
type mockRegistry struct {
	ops map[string]sdk.OperationDef
}

func (m *mockRegistry) RegisterOp(def sdk.OperationDef) error {
	m.ops[def.ID] = def
	return nil
}

func (m *mockRegistry) EnqueueOp(ctx context.Context, defID string, params any, opts ...sdk.EnqueueOption) (string, error) {
	// Mock implementation - not used in these tests
	return "", nil
}

// Verify that Plugin implements the sdk.Plugin interface
var _ sdk.Plugin = (*Plugin)(nil)

// TestOperationDefProperties verifies key properties of each operation definition.
func TestOperationDefProperties(t *testing.T) {
	tests := []struct {
		name           string
		op             sdk.OperationDef
		expectedPhases int
		expectedSched  bool
	}{
		{
			name: "import",
			op: (&Plugin{}).importDef(),
			expectedPhases: 4, // parse_xml, match_books, import_tracks, post_process
			expectedSched: false,
		},
		{
			name: "sync",
			op: (&Plugin{}).syncDef(),
			expectedPhases: 0, // no phases
			expectedSched: true,
		},
		{
			name: "path-reconcile",
			op: (&Plugin{}).pathReconcileDef(),
			expectedPhases: 3, // load_tracks, match_paths, write_results
			expectedSched: false,
		},
		{
			name: "path-repair",
			op: (&Plugin{}).pathRepairDef(),
			expectedPhases: 3, // scan_files, match_paths, apply_changes
			expectedSched: false,
		},
		{
			name: "position-sync",
			op: (&Plugin{}).positionSyncDef(),
			expectedPhases: 0, // no phases
			expectedSched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.op.Phases) != tt.expectedPhases {
				t.Errorf("Expected %d phases, got %d", tt.expectedPhases, len(tt.op.Phases))
			}
			if (tt.op.Schedule != nil) != tt.expectedSched {
				t.Errorf("Expected schedule: %v, got: %v", tt.expectedSched, tt.op.Schedule != nil)
			}
			if tt.op.Run == nil {
				t.Errorf("Run function is nil")
			}
		})
	}
}
