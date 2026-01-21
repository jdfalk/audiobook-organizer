// file: main_test.go
// version: 1.0.0
// guid: 9c3cc5d7-3d49-4e97-a0c1-9b2e38a9986f

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func TestMainHelp(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "test.db")
	playlistsPath := filepath.Join(tempDir, "playlists")

	origArgs := os.Args
	defer func() {
		os.Args = origArgs
	}()

	origQueue := operations.GlobalQueue
	operations.GlobalQueue = nil
	defer func() {
		_ = operations.ShutdownQueue(100 * time.Millisecond)
		operations.GlobalQueue = origQueue
	}()

	os.Args = []string{
		"audiobook-organizer",
		"--db",
		dbPath,
		"--playlists",
		playlistsPath,
		"--help",
	}

	main()
}
