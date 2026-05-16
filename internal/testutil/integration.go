// file: internal/testutil/integration.go
// version: 1.4.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-07

package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/stretchr/testify/require"
)

// IntegrationEnv holds all resources for an integration test.
type IntegrationEnv struct {
	// Store is intentionally the full database.Store surface. Integration
	// tests poke at fixtures across any domain the scenario requires —
	// narrowing here forces churn in every test file for no real benefit
	// (see PR #394 for the regression this deliberate wide type prevents).
	Store     database.Store
	RootDir   string
	ImportDir string
	TempDir   string
	T         *testing.T
}

// SetupIntegration creates a real SQLite database, temp directories,
// and configures globals for integration testing.
func SetupIntegration(t *testing.T) (*IntegrationEnv, func()) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	tmpBase := t.TempDir()
	dbPath := filepath.Join(tmpBase, "test.db")
	rootDir := filepath.Join(tmpBase, "library")
	importDir := filepath.Join(tmpBase, "import")

	require.NoError(t, os.MkdirAll(rootDir, 0755))
	require.NoError(t, os.MkdirAll(importDir, 0755))

	store, err := database.NewSQLiteStore(dbPath)
	require.NoError(t, err)

	err = database.RunMigrations(store)
	require.NoError(t, err)

	database.SetGlobalStore(store)
	// Reset scanner's package-local store so it falls through to the
	// global we just set. Previous test runs in the same process may
	// have left a stale pkgStore via scanner.SetStore from NewServer.
	scanner.SetStore(nil)

	hub := realtime.NewEventHub()
	realtime.SetGlobalHub(hub)

	origCfg := config.AppConfig
	config.AppConfig = config.Config{
		DatabaseType:         "sqlite",
		DatabasePath:         dbPath,
		RootDir:              rootDir,
		EnableSQLite:         true,
		OrganizationStrategy: "copy",
		FolderNamingPattern:  "{author}/{title}",
		FileNamingPattern:    "{title}",
		SupportedExtensions:  []string{".m4b", ".mp3", ".m4a", ".flac", ".ogg"},
		AutoOrganize:         false,
	}

	env := &IntegrationEnv{
		Store:     store,
		RootDir:   rootDir,
		ImportDir: importDir,
		TempDir:   tmpBase,
		T:         t,
	}

	cleanup := func() {
		database.SetGlobalStore(nil)
		store.Close()
		config.AppConfig = origCfg
	}

	return env, cleanup
}

// CreateFakeAudiobook creates a minimal audiobook file in the given directory.
func (env *IntegrationEnv) CreateFakeAudiobook(dir, filename string) string {
	env.T.Helper()
	path := filepath.Join(dir, filename)
	require.NoError(env.T, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(env.T, os.WriteFile(path, []byte("fake-audiobook-data-"+filename), 0644))
	return path
}

// CopyFixture copies a real audio fixture to the target directory.
func (env *IntegrationEnv) CopyFixture(fixtureName, targetDir, targetName string) string {
	env.T.Helper()
	srcPath := filepath.Join(FindRepoRoot(env.T), "testdata", "fixtures", fixtureName)
	dstPath := filepath.Join(targetDir, targetName)
	require.NoError(env.T, os.MkdirAll(filepath.Dir(dstPath), 0755))
	data, err := os.ReadFile(srcPath)
	require.NoError(env.T, err, "fixture %s not found", fixtureName)
	require.NoError(env.T, os.WriteFile(dstPath, data, 0644))
	return dstPath
}

// FindRepoRoot walks up from CWD to find go.mod.
func FindRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

// WaitForOp polls until an operation completes or times out.
//
// Checks v2 first (operations enqueued via opRegistry.EnqueueOp land
// in the operations_v2 table) and falls back to v1 for legacy paths.
// Pass a database.Store (which embeds OpsV2Store) so v2 lookups work;
// the parameter type stays OperationStore for source-compat.
func WaitForOp(t *testing.T, store database.OperationStore, opID string, timeout time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		if v2, ok := store.(database.OpsV2Store); ok {
			if row, err := v2.GetOperationV2(opID); err == nil && row != nil {
				switch row.Status {
				case "completed", "failed", "canceled":
					return true
				}
			}
		}
		op, err := store.GetOperationByID(opID)
		return err == nil && op != nil && (op.Status == "completed" || op.Status == "failed")
	}, timeout, 100*time.Millisecond)
}

// CopyFile copies a file from src to dst.
func CopyFile(t *testing.T, src, dst string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0755))
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, data, 0644))
}
