// file: internal/plugins/deluge/plugin_test.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b5c-6d7e8f9a0b1c
// last-edited: 2026-05-07

package deluge

import (
	"testing"

	delugeclient "github.com/falkcorp/audiobook-organizer/internal/deluge"
)

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	if p.ID() != "deluge" {
		t.Errorf("ID() = %q, want %q", p.ID(), "deluge")
	}
	if p.Name() != "Deluge" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Deluge")
	}
	if p.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "1.0.0")
	}
}

func TestPlugin_Register_NilClient(t *testing.T) {
	p := &Plugin{client: nil}
	// Should return nil without registering anything
	err := p.Register(nil)
	if err != nil {
		t.Fatalf("Register() with nil client should not error: %v", err)
	}
}

func TestPlugin_Register_NilCache(t *testing.T) {
	// Create a mock client
	client, err := delugeclient.New("http://localhost:8112", "deluge")
	if err != nil {
		t.Fatalf("Failed to create deluge client: %v", err)
	}

	p := &Plugin{client: client, cache: nil}
	// Should return nil without registering anything
	err = p.Register(nil)
	if err != nil {
		t.Fatalf("Register() with nil cache should not error: %v", err)
	}
}

func TestNew(t *testing.T) {
	client, err := delugeclient.New("http://localhost:8112", "deluge")
	if err != nil {
		t.Fatalf("Failed to create deluge client: %v", err)
	}

	cache := delugeclient.NewProtectedPathCache(client, []string{"/protected"})
	p := New(client, cache, nil)

	if p.client != client {
		t.Error("New() did not set client correctly")
	}
	if p.cache != cache {
		t.Error("New() did not set cache correctly")
	}
}
