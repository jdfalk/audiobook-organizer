// file: internal/itunes/service/service_test.go
// version: 1.0.0
// guid: 4ab6d921-bccd-4265-b04b-31faaacd5826

package itunesservice

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewDisabled_ReturnsService(t *testing.T) {
	svc := NewDisabled()
	if svc == nil {
		t.Fatal("NewDisabled returned nil")
	}
	if svc.Enabled() {
		t.Error("disabled service should report Enabled() == false")
	}
}

func TestNew_WithDisabledConfig_ReturnsDisabledService(t *testing.T) {
	svc, err := New(Deps{Config: Config{Enabled: false}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.Enabled() {
		t.Error("service constructed with Enabled=false should report Enabled() == false")
	}
}

func TestService_StartShutdown_Disabled_NoOp(t *testing.T) {
	svc := NewDisabled()
	if err := svc.Start(context.Background()); err != nil {
		t.Errorf("Start on disabled: %v", err)
	}
	if err := svc.Shutdown(100 * time.Millisecond); err != nil {
		t.Errorf("Shutdown on disabled: %v", err)
	}
}

func TestErrITunesDisabled_Exported(t *testing.T) {
	// Sanity check that ErrITunesDisabled exists and is an error. Prevents
	// an accidental rename from breaking call sites that sentinel-check.
	if !errors.Is(ErrITunesDisabled, ErrITunesDisabled) {
		t.Fatal("ErrITunesDisabled failed errors.Is identity check")
	}
}
