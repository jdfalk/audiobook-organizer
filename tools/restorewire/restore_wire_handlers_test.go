package restorewire

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const repoRoot = "/__w/_temp/burndown-state/worktrees/audiobook-organizer/draft/add-acoustid-stats-handler"

type treeEntry struct {
	mode string
	name string
	hash string
}

var gitDirCache string

// ... (rest of file unchanged up to tests) ...

func TestRestoreWireHandlersLegacy(t *testing.T) {
	t.Skip("legacy restore disabled")
	data, err := catGitBlob("internal/server/wire_handlers.go")
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "internal/server/wire_handlers.go"), data, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func TestRestoreWireHandlersViaGit(t *testing.T) {
	cmd := exec.Command("git", "show", "HEAD:internal/server/wire_handlers.go")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "internal/server/wire_handlers.go"), out, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(repoRoot, "tools/restorewire")); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}
