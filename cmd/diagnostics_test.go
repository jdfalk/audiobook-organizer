// file: cmd/diagnostics_test.go
// version: 1.0.0
// guid: 8f9a0b1c-2d3e-4f5a-6b7c-8d9e0f1a2b3c

package cmd

import (
	"testing"
)

// TestDiagnosticsCommandExists tests that diagnostics command is initialized
func TestDiagnosticsCommandExists(t *testing.T) {
	// Arrange & Act
	if diagnosticsCmd == nil {
		t.Fatal("diagnosticsCmd should not be nil")
	}

	// Assert
	if diagnosticsCmd.Use != "diagnostics" {
		t.Errorf("Expected Use to be 'diagnostics', got '%s'", diagnosticsCmd.Use)
	}

	if diagnosticsCmd.Short == "" {
		t.Error("diagnosticsCmd.Short should not be empty")
	}

	if diagnosticsCmd.Long == "" {
		t.Error("diagnosticsCmd.Long should not be empty")
	}
}

// TestCleanupCommandExists tests that cleanup subcommand exists
func TestCleanupCommandExists(t *testing.T) {
	if cleanupCmd == nil {
		t.Fatal("cleanupCmd should not be nil")
	}

	if cleanupCmd.Use != "cleanup-invalid" {
		t.Errorf("Expected Use to be 'cleanup-invalid', got '%s'", cleanupCmd.Use)
	}

	if cleanupCmd.Short == "" {
		t.Error("cleanupCmd.Short should not be empty")
	}
}

// TestQueryCommandExists tests that query subcommand exists
func TestQueryCommandExists(t *testing.T) {
	if queryCmd == nil {
		t.Fatal("queryCmd should not be nil")
	}

	if queryCmd.Use != "query" {
		t.Errorf("Expected Use to be 'query', got '%s'", queryCmd.Use)
	}

	if queryCmd.Short == "" {
		t.Error("queryCmd.Short should not be empty")
	}
}

// TestCleanupCommandFlags tests cleanup command flags
func TestCleanupCommandFlags(t *testing.T) {
	flags := []string{"yes", "dry-run"}

	for _, flagName := range flags {
		t.Run(flagName, func(t *testing.T) {
			flag := cleanupCmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Errorf("Expected cleanupCmd to have flag '%s'", flagName)
			}
		})
	}
}

// TestQueryCommandFlags tests query command flags
func TestQueryCommandFlags(t *testing.T) {
	flags := []string{"limit", "prefix", "raw"}

	for _, flagName := range flags {
		t.Run(flagName, func(t *testing.T) {
			flag := queryCmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Errorf("Expected queryCmd to have flag '%s'", flagName)
			}
		})
	}
}
