// file: internal/server/config_update_service_test.go
// version: 1.1.0
// guid: e5f6g7h8-i9j0-k1l2-m3n4-o5p6q7r8s9t0

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

func TestConfigUpdateService_ValidateUpdate_EmptyPayload(t *testing.T) {
	service := NewConfigUpdateService(nil)

	err := service.ValidateUpdate(map[string]any{})

	if err == nil {
		t.Error("expected error for empty payload")
	}
}

func TestConfigUpdateService_ExtractStringField(t *testing.T) {
	service := NewConfigUpdateService(nil)

	payload := map[string]any{
		"root_dir": "/library",
	}

	result, ok := service.ExtractStringField(payload, "root_dir")

	if !ok || result != "/library" {
		t.Errorf("expected '/library', got %q (ok=%v)", result, ok)
	}
}

func TestConfigUpdateService_ExtractBoolField(t *testing.T) {
	service := NewConfigUpdateService(nil)

	payload := map[string]any{
		"auto_organize": true,
	}

	result, ok := service.ExtractBoolField(payload, "auto_organize")

	if !ok || result != true {
		t.Errorf("expected true, got %v (ok=%v)", result, ok)
	}
}

func TestConfigUpdateService_ExtractIntField(t *testing.T) {
	service := NewConfigUpdateService(nil)

	payload := map[string]any{
		"concurrent_scans": float64(4),
	}

	result, ok := service.ExtractIntField(payload, "concurrent_scans")

	if !ok || result != 4 {
		t.Errorf("expected 4, got %d (ok=%v)", result, ok)
	}
}

func TestConfigUpdateService_ApplyUpdates_Success(t *testing.T) {
	service := NewConfigUpdateService(nil)

	updates := map[string]any{
		"root_dir": "/new/library",
	}

	originalDir := config.AppConfig.RootDir
	defer func() {
		config.AppConfig.RootDir = originalDir
	}()

	if err := service.ApplyUpdates(updates); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if config.AppConfig.RootDir != "/new/library" {
		t.Errorf("expected '/new/library', got %q", config.AppConfig.RootDir)
	}
}
