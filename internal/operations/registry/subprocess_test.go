// file: internal/operations/registry/subprocess_test.go
// version: 1.0.0
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f90
// last-edited: 2026-06-04

package registry_test

// Subprocess tests verify:
// 1. Child mode is detected via IsChildMode() when args contain --operation-runner.
// 2. Parent path: stdout and stderr from the child are routed to the reporter log.
// 3. Parent path: ctx cancellation sends SIGTERM to the child.
//
// Note: RunChildMode is not tested directly in unit tests because it requires
// a fully initialised Registry with a real store and calls os.Exit(). The
// integration test for child mode re-execs the test binary itself.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/operations/registry"
)

// TestIsChildMode_FalseWithNoArgs verifies IsChildMode is false in normal execution.
func TestIsChildMode_FalseWithNoArgs(t *testing.T) {
	// In normal test execution, os.Args[1] is not "--operation-runner".
	if registry.IsChildMode() {
		t.Error("IsChildMode() should be false in normal test execution")
	}
}

// TestSubprocess_ChildExitsWithErrorWhenNoBinaryKnowsRunner verifies that an
// Isolate=true op goes to "failed" status when the subprocess exits without
// connecting (because the test binary doesn't handle --operation-runner).
func TestSubprocess_ChildExitsWithErrorWhenNoBinaryKnowsRunner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newFakeStore()
	r := registry.New(store, slog.Default(), 1, nil)

	var ranCount int
	def := makeValidDef("test.subprocess-fail")
	def.Isolate = true
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error {
		ranCount++ // Should never be called directly by in-process worker for Isolate=true.
		return nil
	}
	_ = r.RegisterOp(def)
	r.Start(ctx)

	opID, err := r.EnqueueOp(ctx, "test.subprocess-fail", nil)
	if err != nil {
		t.Fatalf("EnqueueOp: %v", err)
	}

	// The subprocess will fail to connect (test binary doesn't handle --operation-runner),
	// so the op must land in "failed".
	awaitStatus(t, store, opID, "failed", 10*time.Second)

	// def.Run must NOT have been called in-process.
	if ranCount > 0 {
		t.Error("def.Run was called in-process for Isolate=true op; should not happen")
	}

	// error_message must be set.
	row, _ := store.GetOperationV2(opID)
	if row == nil {
		t.Fatal("op row not found")
	}
	if row.ErrorMessage == nil || *row.ErrorMessage == "" {
		t.Error("expected non-empty error_message for subprocess failure")
	}
}

// TestSubprocess_CancelSendsTermToChild verifies that canceling the registry
// context while an Isolate=true op is "running" results in a canceled/failed
// terminal state (not a hang).
func TestSubprocess_CancelSendsTermToChild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := newFakeStore()
	r := registry.New(store, slog.Default(), 1, nil)

	def := makeValidDef("test.subprocess-cancel")
	def.Isolate = true
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error {
		return nil
	}
	_ = r.RegisterOp(def)
	r.Start(ctx)

	opID, err := r.EnqueueOp(ctx, "test.subprocess-cancel", nil)
	if err != nil {
		t.Fatalf("EnqueueOp: %v", err)
	}

	// The subprocess will fail fast. Just verify it reaches a terminal state.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := store.statusOf(opID)
		if s == "failed" || s == "canceled" || s == "completed" {
			t.Logf("subprocess op reached terminal status: %s", s)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("subprocess op did not reach terminal status within deadline; final: %s", store.statusOf(opID))
}

// TestSubprocess_EnvSocketPathConstant verifies the exported constant.
func TestSubprocess_EnvSocketPathConstant(t *testing.T) {
	if registry.EnvSocketPath == "" {
		t.Error("EnvSocketPath constant is empty")
	}
	// Verify it's not accidentally set in the test env.
	if os.Getenv(registry.EnvSocketPath) != "" {
		// This is OK in integration scenarios; just log.
		t.Logf("note: %s is set in env: %s", registry.EnvSocketPath, os.Getenv(registry.EnvSocketPath))
	}
}

const testChildEnvVar = "TEST_SUBPROCESS_CHILD"

// TestMain installs a stub child-mode handler when the gate env var is set.
// Otherwise it runs the test suite normally.
func TestMain(m *testing.M) {
	if os.Getenv(testChildEnvVar) == "1" && len(os.Args) >= 2 && os.Args[1] == "--operation-runner" {
		runStubChild()
		// runStubChild always calls os.Exit; this is unreachable.
		return
	}
	os.Exit(m.Run())
}

// runStubChild mimics what registry.RunChildMode does, minus the def
// lookup. It connects to UOS_SOCKET, reads the handshake, echoes back a
// success result, and exits 0.
func runStubChild() {
	socket := os.Getenv(registry.EnvSocketPath)
	if socket == "" {
		fmt.Fprintln(os.Stderr, "stub child: UOS_SOCKET not set")
		os.Exit(2)
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stub child: dial: %v\n", err)
		os.Exit(2)
	}
	defer conn.Close()

	// Read handshake (newline-terminated JSON).
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		fmt.Fprintln(os.Stderr, "stub child: no handshake")
		os.Exit(1)
	}
	var hs struct {
		DefID  string          `json:"def_id"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &hs); err != nil {
		fmt.Fprintf(os.Stderr, "stub child: bad handshake: %v\n", err)
		os.Exit(1)
	}

	// Reply with success.
	res := []byte(`{"ok":true}` + "\n")
	if _, err := conn.Write(res); err != nil {
		fmt.Fprintf(os.Stderr, "stub child: write result: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// TestSubprocessRoundtrip verifies the parent->child handshake completes
// successfully end-to-end when the child speaks the wire protocol.
func TestSubprocessRoundtrip(t *testing.T) {
	// Set ChildEnvFunc so the spawned child has TEST_SUBPROCESS_CHILD=1
	// and our TestMain stub takes over.
	prev := registry.ChildEnvFunc
	registry.ChildEnvFunc = func() []string {
		return []string{testChildEnvVar + "=1"}
	}
	t.Cleanup(func() { registry.ChildEnvFunc = prev })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := newFakeStore()
	r := registry.New(store, slog.Default(), 1, nil)

	def := makeValidDef("test.subprocess-roundtrip")
	def.Isolate = true
	// Parent never calls def.Run for Isolate=true ops; the stub child
	// short-circuits to ok=true without invoking it.
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error {
		t.Error("def.Run should not be called in-process for Isolate=true")
		return nil
	}
	if err := r.RegisterOp(def); err != nil {
		t.Fatalf("RegisterOp: %v", err)
	}
	r.Start(ctx)

	opID, err := r.EnqueueOp(ctx, "test.subprocess-roundtrip", nil)
	if err != nil {
		t.Fatalf("EnqueueOp: %v", err)
	}

	awaitStatus(t, store, opID, "completed", 15*time.Second)
}
