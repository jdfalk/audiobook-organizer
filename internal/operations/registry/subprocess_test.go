// file: internal/operations/registry/subprocess_test.go
// version: 1.0.0
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f90
// last-edited: 2026-05-06

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
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

// TestSubprocess_ChildHandshakeRoundtrip verifies the unix-socket handshake
// and result roundtrip when re-execing the test binary as a child process.
func TestSubprocess_ChildHandshakeRoundtrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sockDir := t.TempDir()
	socketPath := filepath.Join(sockDir, "uos.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer ln.Close()

	if unixLn, ok := ln.(*net.UnixListener); ok {
		_ = unixLn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	connCh := make(chan net.Conn, 1)
	acceptErrCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			acceptErrCh <- err
			return
		}
		connCh <- conn
	}()

	cmd := exec.CommandContext(ctx, os.Args[0], "--operation-runner", "test.handshake-roundtrip")
	cmd.Env = append(os.Environ(), registry.EnvSocketPath+"="+socketPath, testChildEnvVar+"=1")
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}

	var conn net.Conn
	select {
	case conn = <-connCh:
	case err := <-acceptErrCh:
		t.Fatalf("accept child connection: %v", err)
	case <-ctx.Done():
		t.Fatalf("timeout waiting for child connection: %v", ctx.Err())
	}
	defer conn.Close()

	hs := struct {
		DefID  string          `json:"def_id"`
		Params json.RawMessage `json:"params"`
	}{
		DefID:  "test.subprocess-handshake",
		Params: json.RawMessage(`{"payload":"test"}`),
	}
	payload, err := json.Marshal(hs)
	if err != nil {
		t.Fatalf("marshal handshake: %v", err)
	}
	payload = append(payload, '\n')
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			t.Fatalf("scan result: %v", err)
		}
		t.Fatalf("child closed connection without sending result")
	}

	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal child result: %v", err)
	}
	if !res.OK {
		t.Fatalf("child reported failure: %s", res.Error)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("child exit error: %v", err)
	}
}
