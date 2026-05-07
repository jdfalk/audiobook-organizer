// file: internal/plugins/maintenance/plugin_test.go
// version: 1.0.0
// guid: a3b4c5d6-e7f8-9012-6789-234567890123
// last-edited: 2026-05-07

package maintenance_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// stubRegistry implements sdk.Registry for counting registered ops.
type stubRegistry struct {
	ops []sdk.OperationDef
}

func (r *stubRegistry) RegisterOp(def sdk.OperationDef) error {
	r.ops = append(r.ops, def)
	return nil
}

func (r *stubRegistry) SetPluginMaxConcurrent(pluginID string, max int) {}

func TestMaintenancePlugin_Register_AllOpsHaveExplicitResumePolicy(t *testing.T) {
	t.Skip("requires full ServerDeps stub — enable after UOS-12 server-side wiring")
}

func TestMaintenancePlugin_AllOpsHaveCapabilities(t *testing.T) {
	t.Skip("requires full ServerDeps stub — enable after UOS-12 server-side wiring")
}

func TestMaintenancePlugin_HardRules(t *testing.T) {
	t.Skip("requires full ServerDeps stub — enable after UOS-12 server-side wiring")
}
