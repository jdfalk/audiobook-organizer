// file: internal/plugins/itunes/plugin_test.go
// version: 1.0.0
// guid: a7b8c9d0-e1f2-3456-ghij-567890123456
// last-edited: 2026-05-07

package itunes

import (
	"testing"
)

// TestPlugin_NilGuard tests that the plugin handles nil service gracefully.
// This is critical for server initialization when iTunes is disabled.
func TestPlugin_NilGuard(t *testing.T) {
	p := New(nil, nil)
	if err := p.Register(nil); err != nil {
		t.Fatalf("nil guard: %v", err)
	}
}
