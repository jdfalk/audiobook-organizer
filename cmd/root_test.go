// file: cmd/root_test.go
// version: 1.0.0
// guid: 7eae8d0c-7fda-4f45-8f73-5d1e0c7c9f1a

package cmd

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/spf13/viper"
)

func TestFormatMetadataValue(t *testing.T) {
	if got := formatMetadataValue("  "); got != "(empty)" {
		t.Fatalf("expected empty placeholder, got %q", got)
	}
	if got := formatMetadataValue("Title"); got != "Title" {
		t.Fatalf("expected value passthrough, got %q", got)
	}
}

func TestSetupFileLogging(t *testing.T) {
	tempDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	prevWriter := log.Writer()
	prevFlags := log.Flags()
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	logFile, err := setupFileLogging()
	if err != nil {
		t.Fatalf("setupFileLogging failed: %v", err)
	}
	defer logFile.Close()

	if _, err := os.Stat(logFile.Name()); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

func TestInitConfigCreatesDirectories(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "test.db")
	playlistsPath := filepath.Join(tempDir, "playlists")

	origCfgFile := cfgFile
	origDBPath := databasePath
	origPlaylistDir := playlistDir
	origConfig := config.AppConfig
	defer func() {
		cfgFile = origCfgFile
		databasePath = origDBPath
		playlistDir = origPlaylistDir
		config.AppConfig = origConfig
	}()

	cfgFile = filepath.Join(tempDir, "config.yaml")
	databasePath = dbPath
	playlistDir = playlistsPath

	initConfig()

	if _, err := os.Stat(filepath.Dir(dbPath)); err != nil {
		t.Fatalf("expected database directory to exist: %v", err)
	}
	if _, err := os.Stat(playlistsPath); err != nil {
		t.Fatalf("expected playlist directory to exist: %v", err)
	}
}

func TestInitConfigUsesHomeConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".audiobook-organizer.yaml")
	if err := os.WriteFile(configPath, []byte("root_dir: /tmp\n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	origCfgFile := cfgFile
	origDBPath := databasePath
	origPlaylistDir := playlistDir
	origConfig := config.AppConfig
	defer func() {
		cfgFile = origCfgFile
		databasePath = origDBPath
		playlistDir = origPlaylistDir
		config.AppConfig = origConfig
	}()

	t.Setenv("HOME", tempDir)
	cfgFile = ""
	databasePath = ""
	playlistDir = ""

	viper.Reset()
	initConfig()
}

func TestPrintMetadataField(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	printMetadataField("Title", "")
	_ = w.Close()

	output, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if got := string(output); got == "" {
		t.Fatal("expected output to be written")
	}
}

func TestScanCommandMissingRootDir(t *testing.T) {
	tempDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	origConfig := config.AppConfig
	defer func() {
		config.AppConfig = origConfig
	}()

	config.AppConfig.RootDir = ""

	if err := scanCmd.RunE(scanCmd, nil); err == nil {
		t.Fatal("expected error when root directory is missing")
	}
}

func TestMetadataInspectRequiresFile(t *testing.T) {
	if err := metadataInspectCmd.RunE(metadataInspectCmd, nil); err == nil {
		t.Fatal("expected error when file is missing")
	}
}
