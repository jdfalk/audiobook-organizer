// file: main.go
// version: 1.5.0
// guid: 5f6a7b8c-9d0e-1f2a-3b4c-5d6e7f8a9b0c

package main

import (
	"fmt"
	"os"

	"github.com/jdfalk/audiobook-organizer/cmd"
	"github.com/jdfalk/audiobook-organizer/internal/server"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

var executeCmd = cmd.Execute

func main() {
	os.Exit(run())
}

func run() int {
	// Set version everywhere
	cmd.SetVersion(version)
	server.SetVersion(version)
	server.SetEmbeddedFS(WebFS)

	if err := executeCmd(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
