// file: internal/server/pipeline_checkpoint_test.go
// version: 1.0.0
// guid: 8a9b0c1d-2e3f-4a70-b8c5-3d7e0f1b9a99

package server

import (
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestCheckpoint_SetAndHas(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if hasCheckpoint(store, "b1", phaseRename) {
		t.Error("should not have checkpoint before set")
	}

	setCheckpoint(store, "b1", phaseRename)

	if !hasCheckpoint(store, "b1", phaseRename) {
		t.Error("should have checkpoint after set")
	}
	if hasCheckpoint(store, "b1", phaseTags) {
		t.Error("different phase should not have checkpoint")
	}
	if hasCheckpoint(store, "b2", phaseRename) {
		t.Error("different book should not have checkpoint")
	}
}

func TestCheckpoint_ClearAll(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	setCheckpoint(store, "b1", phaseRename)
	setCheckpoint(store, "b1", phaseTags)
	setCheckpoint(store, "b1", phaseITunes)

	clearCheckpoints(store, "b1")

	for _, phase := range []string{phaseRename, phaseTags, phaseITunes} {
		if hasCheckpoint(store, "b1", phase) {
			t.Errorf("phase %s should be cleared", phase)
		}
	}
}

func TestCheckpoint_Idempotent(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	setCheckpoint(store, "b1", phaseRename)
	setCheckpoint(store, "b1", phaseRename) // double-set
	if !hasCheckpoint(store, "b1", phaseRename) {
		t.Error("double-set should still have checkpoint")
	}

	clearCheckpoints(store, "b1")
	clearCheckpoints(store, "b1") // double-clear
	if hasCheckpoint(store, "b1", phaseRename) {
		t.Error("double-clear should still be cleared")
	}
}
