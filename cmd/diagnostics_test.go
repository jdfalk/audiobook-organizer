// file: cmd/diagnostics_test.go
// version: 1.0.0
// guid: 5480d7f7-4a6a-4b7f-9d16-6b589c8a3c0b

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestTruncateString(t *testing.T) {
	if got := truncateString("short", 10); got != "short" {
		t.Fatalf("expected no truncation, got %q", got)
	}
	if got := truncateString("this is long", 4); got != "this..." {
		t.Fatalf("expected truncation, got %q", got)
	}
}

func TestHasPlaceholder(t *testing.T) {
	tokens := []string{"{author}", "{title}"}
	if !hasPlaceholder("/books/{Author}/file.mp3", tokens) {
		t.Fatal("expected placeholder match")
	}
	if hasPlaceholder("/books/clean/file.mp3", tokens) {
		t.Fatal("did not expect placeholder match")
	}
}

func TestPromptYesNo(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	_, _ = w.Write([]byte("yes\n"))
	_ = w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
	}()

	confirmed, err := promptYesNo("confirm")
	if err != nil {
		t.Fatalf("promptYesNo failed: %v", err)
	}
	if !confirmed {
		t.Fatal("expected confirmation")
	}
}

func TestRunDiagnosticsQueryErrors(t *testing.T) {
	origConfig := config.AppConfig
	defer func() {
		config.AppConfig = origConfig
	}()

	if err := runDiagnosticsQuery(0, "", false); err == nil {
		t.Fatal("expected error for invalid limit")
	}

	config.AppConfig.DatabaseType = "sqlite"
	if err := runDiagnosticsQuery(1, "book:", true); err == nil {
		t.Fatal("expected error for raw query with non-pebble db")
	}
}

func TestRunDiagnosticsQuerySuccess(t *testing.T) {
	origConfig := config.AppConfig
	defer func() {
		config.AppConfig = origConfig
	}()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "diag.db")
	store, err := database.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	_, err = store.CreateBook(&database.Book{
		Title:    "Diag Book",
		FilePath: "/tmp/diag.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create book: %v", err)
	}
	_ = store.Close()

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = dbPath
	config.AppConfig.EnableSQLite = true

	if err := runDiagnosticsQuery(5, "book:", false); err != nil {
		t.Fatalf("runDiagnosticsQuery failed: %v", err)
	}
}

func TestRunCleanupInvalidBooksDryRun(t *testing.T) {
	origConfig := config.AppConfig
	defer func() {
		config.AppConfig = origConfig
	}()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "cleanup.db")
	store, err := database.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	_, err = store.CreateBook(&database.Book{
		Title:    "Bad Book",
		FilePath: "/tmp/{author}/bad.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create book: %v", err)
	}
	_ = store.Close()

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = dbPath
	config.AppConfig.EnableSQLite = true

	if err := runCleanupInvalidBooks(false, true); err != nil {
		t.Fatalf("runCleanupInvalidBooks failed: %v", err)
	}
}

func TestRunRawPebbleQuery(t *testing.T) {
	tempDir := t.TempDir()
	store, err := database.NewPebbleStore(tempDir)
	if err != nil {
		t.Fatalf("failed to create pebble store: %v", err)
	}
	_, err = store.CreateBook(&database.Book{
		Title:    "Pebble Book",
		FilePath: "/tmp/pebble.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create book: %v", err)
	}
	_ = store.Close()

	config.AppConfig.DatabasePath = tempDir
	if err := runRawPebbleQuery(1, "book:"); err != nil {
		t.Fatalf("runRawPebbleQuery failed: %v", err)
	}
}

func TestExecuteHelp(t *testing.T) {
	tempDir := t.TempDir()

	origCfg := cfgFile
	origDBPath := databasePath
	origPlaylist := playlistDir
	defer func() {
		cfgFile = origCfg
		databasePath = origDBPath
		playlistDir = origPlaylist
	}()

	cfgFile = filepath.Join(tempDir, "config.yaml")
	databasePath = filepath.Join(tempDir, "db.sqlite")
	playlistDir = filepath.Join(tempDir, "playlists")

	rootCmd.SetArgs([]string{"--db", databasePath, "--playlists", playlistDir, "--help"})
	defer rootCmd.SetArgs(nil)

	if err := Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

func TestPromptYesNoNo(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	_, _ = w.Write([]byte("no\n"))
	_ = w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
	}()

	confirmed, err := promptYesNo("confirm")
	if err != nil {
		t.Fatalf("promptYesNo failed: %v", err)
	}
	if confirmed {
		t.Fatal("expected rejection")
	}
}

func TestTruncateStringWithBuffer(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("1234567890")
	got := truncateString(buf.String(), 6)
	if got != "123456..." {
		t.Fatalf("expected truncated buffer, got %q", got)
	}
}
