// file: main_test.go
// version: 1.0.0
// guid: 9c3cc5d7-3d49-4e97-a0c1-9b2e38a9986f

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func TestRunSuccess(t *testing.T) {
	origExecute := executeCmd
	executeCmd = func() error { return nil }
	defer func() {
		executeCmd = origExecute
	}()

	origQueue := operations.GlobalQueue
	operations.GlobalQueue = nil
	defer func() {
		_ = operations.ShutdownQueue(100 * time.Millisecond)
		operations.GlobalQueue = origQueue
	}()

	if code := run(); code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestRunError(t *testing.T) {
	origExecute := executeCmd
	executeCmd = func() error { return fmt.Errorf("boom") }
	defer func() {
		executeCmd = origExecute
	}()

	origQueue := operations.GlobalQueue
	operations.GlobalQueue = nil
	defer func() {
		_ = operations.ShutdownQueue(100 * time.Millisecond)
		operations.GlobalQueue = origQueue
	}()

	if code := run(); code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}
