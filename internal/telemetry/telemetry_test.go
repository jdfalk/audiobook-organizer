// file: internal/telemetry/telemetry_test.go
// version: 1.0.0
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a

package telemetry

import (
	"context"
	"testing"
)

func TestInitOTEL_NoEndpoint(t *testing.T) {
	cfg := &Config{
		ExporterEndpoint: "",
		ServiceName:      "test",
		Enabled:          false,
	}

	shutdown, err := InitOTEL(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InitOTEL with no endpoint returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown function should not be nil")
	}

	// No-op shutdown should not error
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}
}

func TestInitOTEL_WithInvalidEndpoint(t *testing.T) {
	cfg := &Config{
		ExporterEndpoint: "invalid://endpoint",
		ServiceName:      "test",
		Enabled:          true,
	}

	// Should fail due to invalid endpoint format
	_, err := InitOTEL(context.Background(), cfg)
	if err == nil {
		t.Fatal("InitOTEL with invalid endpoint should return error")
	}
}
