// file: main.go
// version: 1.4.0
// guid: 5f6a7b8c-9d0e-1f2a-3b4c-5d6e7f8a9b0c

package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // registers pprof handlers on default mux
	"os"

	"github.com/jdfalk/audiobook-organizer/cmd"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/server"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

var executeCmd = cmd.Execute

func main() {
	os.Exit(run())
}

func run() int {
	// Start pprof on a separate port (localhost only for security)
	go func() {
		log.Println("[INFO] pprof available at http://localhost:6060/debug/pprof/")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			log.Printf("[WARN] pprof server failed: %v", err)
		}
	}()

	// Set version everywhere
	cmd.SetVersion(version)
	server.SetVersion(version)
	server.SetEmbeddedFS(WebFS)

	// Early initialization of operation queue (without store). This allows
	// code paths that enqueue operations prior to full server startup to avoid nil checks.
	if operations.GlobalQueue == nil {
		operations.InitializeQueue(nil, 2) // store will be attached later once initialized
	}

	if err := executeCmd(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
