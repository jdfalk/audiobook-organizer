// file: internal/itunes/service/path_repair.go
// version: 1.0.0
// guid: 01ad6c79-5f3f-4ee1-a07a-1f4b3a8c0d12
//
// PathRepairer dumps the iTunes XML, finds tracks whose Location no
// longer exists on disk, re-discovers the correct path via three tiers
// (PID → DB lookup; embedded AUDIOBOOK_ORGANIZER_PERSISTENT_ID tag
// scan; fuzzy filename + title match), and enqueues each fix through
// the existing WriteBackBatcher so the ITL learns the new locations
// during normal batched write-back.

package itunesservice

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// PathRepairConfig holds the immutable inputs the repairer needs:
// where to read the iTunes XML, and where the audiobook tree lives
// for tier-B/C disk scanning.
type PathRepairConfig struct {
	XMLPath       string
	AudiobookRoot string
}

// pathRepairerStore is the narrow slice of the service Store that
// PathRepairer needs. Identical surface to pathReconcilerStore today,
// declared separately so the two operations can evolve independently.
type pathRepairerStore interface {
	database.BookStore
	database.BookFileStore
	database.OperationStore
	database.ExternalIDStore
}

// PathRepairer is the operation worker.
type PathRepairer struct {
	store    pathRepairerStore
	enqueuer Enqueuer
	queue    operations.Queue
	cfg      PathRepairConfig
}

// newPathRepairer wires a PathRepairer. nil enqueuer skips the
// write-back enqueue step (used by dry-run-only tests).
func newPathRepairer(store pathRepairerStore, enqueuer Enqueuer, queue operations.Queue, cfg PathRepairConfig) *PathRepairer {
	return &PathRepairer{store: store, enqueuer: enqueuer, queue: queue, cfg: cfg}
}

// iTunesPathRepairResult is the per-run tally returned in progress
// logs and the operation result. Field names mirror the dry-run JSON
// payload that callers consume.
type iTunesPathRepairResult struct {
	XMLTracks    int      `json:"xml_tracks"`
	Missing      int      `json:"missing"`
	AutoResolved int      `json:"auto_resolved"`
	NeedsReview  int      `json:"needs_review"`
	Unresolved   int      `json:"unresolved"`
	Enqueued     int      `json:"enqueued"`
	DryRun       bool     `json:"dry_run"`
	ReportPath   string   `json:"report_path,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

// parseDryRun reads the apply= query parameter and returns whether the
// run should stay in dry-run mode. Any value not equal to "true" or
// "1" leaves dry-run on (safer default).
func parseDryRun(c *gin.Context) bool {
	apply := strings.ToLower(c.Query("apply"))
	if apply == "true" || apply == "1" {
		return false
	}
	return true
}

// Start kicks off a tracked operation that walks the iTunes XML,
// finds missing locations, and (in apply mode) enqueues path fixes
// through the WriteBackBatcher. Defaults to dry-run.
func (r *PathRepairer) Start(c *gin.Context) {
	if r.store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if r.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	dryRun := parseDryRun(c)

	id := ulid.Make().String()
	op, err := r.store.CreateOperation(id, "itunes_path_repair", nil)
	if err != nil {
		log.Printf("[ERROR] failed to create operation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create operation"})
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return r.Repair(ctx, id, dryRun, progress)
	}

	if err := r.queue.Enqueue(op.ID, "itunes_path_repair", operations.PriorityNormal, operationFunc); err != nil {
		log.Printf("[ERROR] failed to enqueue operation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue operation"})
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// Repair is the operation body. Step 1 scaffolds the worker shape;
// tier A/B/C resolution lands in subsequent commits.
func (r *PathRepairer) Repair(ctx context.Context, opID string, dryRun bool, progress operations.ProgressReporter) error {
	if r.store == nil {
		return fmt.Errorf("database not initialized")
	}
	_ = progress.Log("info", "iTunes path repair started", nil)
	result := iTunesPathRepairResult{DryRun: dryRun}
	_ = operations.ClearState(r.store, opID)
	_ = progress.Log("info", "iTunes path repair complete (scaffold; resolution tiers pending)", nil)
	_ = progress.UpdateProgress(0, 0, "scaffold")
	_ = result // populated in step 2+
	return nil
}
