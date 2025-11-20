// file: main.go
// version: 1.2.0
// guid: 5f6a7b8c-9d0e-1f2a-3b4c-5d6e7f8a9b0c

package main

import (
	"fmt"
	"os"

	"github.com/jdfalk/audiobook-organizer/cmd"
	"github.com/jdfalk/audiobook-organizer/internal/server"
)

func main() {
	// Set embedded filesystem for server (if built with embed_frontend tag)
	server.SetEmbeddedFS(WebFS)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
