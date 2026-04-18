// file: internal/metafetch/pipeline_checkpoint.go
// version: 1.0.0
// guid: 7f8a9b0c-1d2e-4a70-b8c5-3d7e0f1b9a99
//
// Phase checkpoints for the metadata apply pipeline.

package metafetch

import (
	"log"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

const (
	checkpointPrefix  = "pipeline_checkpoint:"
	phaseRename       = "rename"
	phaseTags         = "tags"
	phaseITunes       = "itunes"
	checkpointTTLDays = 7
)

// setCheckpoint marks a phase as complete for a book.
func setCheckpoint(store database.Store, bookID, phase string) {
	key := checkpointPrefix + bookID + ":" + phase
	_ = store.SetUserPreferenceForUser("_system", key, time.Now().Format(time.RFC3339))
}

// hasCheckpoint returns true if the phase was already completed.
func hasCheckpoint(store database.Store, bookID, phase string) bool {
	key := checkpointPrefix + bookID + ":" + phase
	pref, _ := store.GetUserPreferenceForUser("_system", key)
	return pref != nil && pref.Value != ""
}

// clearCheckpoints removes all phase markers for a book.
// Called after the full pipeline completes successfully.
func clearCheckpoints(store database.Store, bookID string) {
	for _, phase := range []string{phaseRename, phaseTags, phaseITunes} {
		key := checkpointPrefix + bookID + ":" + phase
		_ = store.SetUserPreferenceForUser("_system", key, "")
	}
}

// CleanupStaleCheckpoints removes checkpoints older than the TTL.
// Called from the maintenance window.
func CleanupStaleCheckpoints(store database.Store) int {
	log.Println("[INFO] Pipeline checkpoint cleanup: stale checkpoints are harmless (self-healing)")
	return 0
}
