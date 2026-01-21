// file: main.go
// version: 1.3.0
// guid: 5f6a7b8c-9d0e-1f2a-3b4c-5d6e7f8a9b0c

package main

import (
	"fmt"
	"os"

	"github.com/jdfalk/audiobook-organizer/cmd"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/server"
)

var executeCmd = cmd.Execute

func main() {
	os.Exit(run())
}

func run() int {
	// Set embedded filesystem for server (if built with embed_frontend tag)
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
