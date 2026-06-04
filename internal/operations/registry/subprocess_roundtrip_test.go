// file: internal/operations/registry/subprocess_roundtrip_test.go
// version: 1.0.0
// guid: 9e0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b
//
// Subprocess re-exec roundtrip smoke test (MAYDEPLOY-A / A2).
//
// This file contains the TestMain stub that acts as the child when the test
// binary is run with --operation-runner and the TEST_SUBPROCESS_CHILD env var.
// The actual roundtrip test lives in subprocess_test.go so that the helper
// stays in a separate file.

package registry_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/operations/registry"
)

// testChildEnvVar gates the in-test child-mode handler installed by TestMain.
const testChildEnvVar = "TEST_SUBPROCESS_CHILD"

// TestMain installs a stub child-mode handler when the gate env var is set.
// Otherwise it runs the test suite normally.
func TestMain(m *testing.M) {
	if os.Getenv(testChildEnvVar) == "1" && len(os.Args) >= 2 && os.Args[1] == "--operation-runner" {
		runStubChild()
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
