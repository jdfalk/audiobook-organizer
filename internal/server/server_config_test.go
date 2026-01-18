// file: internal/server/server_config_test.go
// version: 1.0.0
// guid: 6c4b2a1d-8e9f-4b7c-9d0e-1f2a3b4c5d6e

package server

import "testing"

func TestGetDefaultServerConfig_DisablesWriteTimeout(t *testing.T) {
	// Arrange
	cfg := GetDefaultServerConfig()

	// Act
	got := cfg.WriteTimeout

	// Assert
	if got != 0 {
		t.Fatalf("expected WriteTimeout to be 0, got %v", got)
	}
	if cfg.ReadTimeout == 0 {
		t.Fatalf("expected ReadTimeout to be non-zero")
	}
}
