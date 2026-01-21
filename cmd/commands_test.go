// file: cmd/commands_test.go
// version: 1.0.0
// guid: 6f5b7d78-11d8-4c1a-a150-96d2c4a1a885

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/server"
)

func stubCommandDeps(t *testing.T) {
	t.Helper()

	origInit := initializeStore
	origClose := closeStore
	origScan := scanDirectory
	origProcess := processBooks
	origPlaylists := generatePlaylists
	origTags := updateSeriesTags
	origInitEncrypt := initEncryption
	origLoadConfig := loadConfigFromDB
	origSyncEnv := syncConfigFromEnv
	origInitQueue := initializeQueue
	origShutdownQueue := shutdownQueue
	origNewServer := newServer
	origDefaultCfg := getDefaultServerConfig
	origStart := startServer

	initializeStore = func(dbType, path string, enableSQLite bool) error {
		database.GlobalStore = database.NewMockStore()
		return nil
	}
	closeStore = func() error {
		database.GlobalStore = nil
		return nil
	}
	scanDirectory = func(rootDir string) ([]scanner.Book, error) {
		return []scanner.Book{}, nil
	}
	processBooks = func(books []scanner.Book) error {
		return nil
	}
	generatePlaylists = func() error {
		return nil
	}
	updateSeriesTags = func() error {
		return nil
	}
	initEncryption = func(dir string) error {
		return nil
	}
	loadConfigFromDB = func(store database.Store) error {
		return nil
	}
	syncConfigFromEnv = func() {}
	initializeQueue = func(store database.Store, workers int) {}
	shutdownQueue = func(timeout time.Duration) error { return nil }
	newServer = func() *server.Server { return &server.Server{} }
	getDefaultServerConfig = func() server.ServerConfig {
		return server.ServerConfig{Host: "localhost", Port: "8080"}
	}
	startServer = func(srv *server.Server, cfg server.ServerConfig) error { return nil }

	t.Cleanup(func() {
		initializeStore = origInit
		closeStore = origClose
		scanDirectory = origScan
		processBooks = origProcess
		generatePlaylists = origPlaylists
		updateSeriesTags = origTags
		initEncryption = origInitEncrypt
		loadConfigFromDB = origLoadConfig
		syncConfigFromEnv = origSyncEnv
		initializeQueue = origInitQueue
		shutdownQueue = origShutdownQueue
		newServer = origNewServer
		getDefaultServerConfig = origDefaultCfg
		startServer = origStart
		database.GlobalStore = nil
	})
}

func TestCommandsRunWithStubs(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.RootDir = tempDir
	config.AppConfig.EnableSQLite = true
	config.AppConfig.PlaylistDir = filepath.Join(tempDir, "playlists")

	if err := scanCmd.RunE(scanCmd, nil); err != nil {
		t.Fatalf("scanCmd failed: %v", err)
	}
	if err := playlistCmd.RunE(playlistCmd, nil); err != nil {
		t.Fatalf("playlistCmd failed: %v", err)
	}
	if err := tagCmd.RunE(tagCmd, nil); err != nil {
		t.Fatalf("tagCmd failed: %v", err)
	}
	if err := organizeCmd.RunE(organizeCmd, nil); err != nil {
		t.Fatalf("organizeCmd failed: %v", err)
	}
	if err := serveCmd.RunE(serveCmd, nil); err != nil {
		t.Fatalf("serveCmd failed: %v", err)
	}
}

func TestScanCommandErrorPaths(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.RootDir = tempDir
	config.AppConfig.EnableSQLite = true

	scanDirectory = func(rootDir string) ([]scanner.Book, error) {
		return nil, fmt.Errorf("scan failed")
	}
	if err := scanCmd.RunE(scanCmd, nil); err == nil {
		t.Fatal("expected scan command error")
	}

	scanDirectory = func(rootDir string) ([]scanner.Book, error) {
		return []scanner.Book{{FilePath: filepath.Join(tempDir, "book.m4b")}}, nil
	}
	processBooks = func(books []scanner.Book) error {
		return fmt.Errorf("process failed")
	}
	if err := scanCmd.RunE(scanCmd, nil); err == nil {
		t.Fatal("expected processBooks error")
	}
}

func TestServeCommandErrorPaths(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.EnableSQLite = true

	initEncryption = func(dir string) error { return fmt.Errorf("encrypt fail") }
	if err := serveCmd.RunE(serveCmd, nil); err == nil {
		t.Fatal("expected serve command to fail on encryption error")
	}

	initEncryption = func(dir string) error { return nil }
	startServer = func(srv *server.Server, cfg server.ServerConfig) error {
		return fmt.Errorf("start failed")
	}
	if err := serveCmd.RunE(serveCmd, nil); err == nil {
		t.Fatal("expected serve command to fail on start error")
	}
}
