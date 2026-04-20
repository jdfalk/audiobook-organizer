// file: internal/plugins/deluge/plugin_test.go
// version: 1.0.0

package deluge

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/plugin"
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

func TestPlugin_Capabilities(t *testing.T) {
	p := &Plugin{}
	caps := p.Capabilities()
	if len(caps) != 1 {
		t.Fatalf("len(Capabilities()) = %d, want 1", len(caps))
	}
	if caps[0] != plugin.CapDownloadClient {
		t.Errorf("Capabilities()[0] = %q, want %q", caps[0], plugin.CapDownloadClient)
	}
}

func TestPlugin_Init_MissingURL(t *testing.T) {
	p := &Plugin{}
	err := p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"password": "secret"},
	})
	if err == nil {
		t.Fatal("Init() should fail when web_url is missing")
	}
}

func TestPlugin_Init_ValidConfig(t *testing.T) {
	p := &Plugin{}
	err := p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{
			"web_url":  "http://localhost:8112",
			"password": "deluge",
		},
	})
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if p.client == nil {
		t.Fatal("client should be set after Init")
	}
}

func TestPlugin_HealthCheck_NotInitialized(t *testing.T) {
	p := &Plugin{}
	err := p.HealthCheck()
	if err == nil {
		t.Fatal("HealthCheck() should fail when not initialized")
	}
}

func TestPlugin_TestConnection_NotInitialized(t *testing.T) {
	p := &Plugin{}
	err := p.TestConnection()
	if err == nil {
		t.Fatal("TestConnection() should fail when not initialized")
	}
}

func TestPlugin_ListTorrents_NotInitialized(t *testing.T) {
	p := &Plugin{}
	_, err := p.ListTorrents()
	if err == nil {
		t.Fatal("ListTorrents() should fail when not initialized")
	}
}

func TestPlugin_MoveStorage_NotInitialized(t *testing.T) {
	p := &Plugin{}
	err := p.MoveStorage("abc123", "/tmp/dest")
	if err == nil {
		t.Fatal("MoveStorage() should fail when not initialized")
	}
}

func TestPlugin_Shutdown(t *testing.T) {
	p := &Plugin{}
	// Init first
	_ = p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{
			"web_url":  "http://localhost:8112",
			"password": "deluge",
		},
	})
	err := p.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}
	if p.client != nil {
		t.Fatal("client should be nil after Shutdown")
	}
}

// Compile-time interface check
var _ plugin.DownloadClient = (*Plugin)(nil)
