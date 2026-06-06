// file: internal/operations/registry/subprocess_roundtrip_test.go
// version: 1.0.0
// guid: 9e0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b
//
// Subprocess re-exec roundtrip smoke test (MAYDEPLOY-A / A2).
//
// This test exercises the full parent/child wire protocol implemented in
// subprocess.go end-to-end, without depending on main.go's
// IsChildMode/RunChildMode wiring (that's covered by the manual smoke
// test in the PR description). The strategy:
//
//   - TestMain notices the TEST_SUBPROCESS_CHILD env var. When set, the
//     test binary acts as the child: it connects to UOS_SOCKET, reads the
//     handshake, writes back the result the parent expects, and exits.
//   - TestSubprocessRoundtrip enqueues an Isolate=true op against a real
//     Registry. The parent re-execs THIS test binary; because the env var
//     is set (via ChildEnvFunc), the spawned child runs the stub above
//     and the parent observes a "completed" terminal state.
//
// The existing TestSubprocess_ChildExitsWithErrorWhenNoBinaryKnowsRunner
// relies on the test binary NOT handling --operation-runner, so we keep
// the child-mode behavior gated on TEST_SUBPROCESS_CHILD to avoid
// breaking that case.

package registry_test

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

    "github.com/falkcorp/audiobook-organizer/internal/operations/registry"
)

const (
	testChildEnvVar           = "TEST_SUBPROCESS_CHILD"
	testChildHandshakePathEnv = "TEST_SUBPROCESS_HANDSHAKE_PATH"
)

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

	if path := os.Getenv(testChildHandshakePathEnv); path != "" {
		snapshot := append([]byte(nil), scanner.Bytes()...)
		if err := os.WriteFile(path, snapshot, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "stub child: write handshake: %v\n", err)
			os.Exit(1)
		}
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
