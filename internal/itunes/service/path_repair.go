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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
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
	// bookIDExtractor pulls AUDIOBOOK_ORGANIZER_ID from one audio
	// file. Production wires this to metadata.ExtractMetadata.
	// Tests inject deterministic fakes.
	bookIDExtractor bookIDExtractor
}

// newPathRepairer wires a PathRepairer. nil enqueuer skips the
// write-back enqueue step (used by dry-run-only tests).
func newPathRepairer(store pathRepairerStore, enqueuer Enqueuer, queue operations.Queue, cfg PathRepairConfig) *PathRepairer {
	return &PathRepairer{
		store:           store,
		enqueuer:        enqueuer,
		queue:           queue,
		cfg:             cfg,
		bookIDExtractor: extractBookOrganizerID,
	}
}

// extractBookOrganizerID is the production extractor used by the
// fsTagScanner. Reads embedded metadata and returns the
// AUDIOBOOK_ORGANIZER_ID tag value (book-organizer book ID).
func extractBookOrganizerID(audioFilePath string) (string, error) {
	md, err := metadata.ExtractMetadata(audioFilePath, nil)
	if err != nil {
		return "", err
	}
	return md.BookOrganizerID, nil
}

// iTunesPathRepairResult is the per-run tally returned in progress
// logs and the operation result. Field names mirror the dry-run JSON
// payload that callers consume.
type iTunesPathRepairResult struct {
	XMLTracks        int                `json:"xml_tracks"`
	Missing          int                `json:"missing"`
	AutoResolved     int                `json:"auto_resolved"`
	NeedsReview      int                `json:"needs_review"`
	Unresolved       int                `json:"unresolved"`
	Enqueued         int                `json:"enqueued"`
	DryRun           bool               `json:"dry_run"`
	ReportPath       string             `json:"report_path,omitempty"`
	NeedsReviewItems []needsReviewItem  `json:"needs_review_items,omitempty"`
	Errors           []string           `json:"errors,omitempty"`
}

// needsReviewItem is one fuzzy-resolved missing track requiring human
// confirmation. Emitted by tier C; never auto-applied.
type needsReviewItem struct {
	PID        string           `json:"pid"`
	OldPath    string           `json:"old_path"`
	Title      string           `json:"title,omitempty"`
	Candidates []tierCCandidate `json:"candidates"`
}

// Tier C tuning constants. Threshold matches the user-confirmed 0.85
// Jaro-Winkler equivalent on the matcher 0–100 scale.
const (
	tierCThreshold = 85
	tierCTopN      = 3
)

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

// Repair is the operation body. Wraps repairWithResult so the
// queue-side closure has the (ctx, id, progress) → error signature
// the operations.Queue expects.
func (r *PathRepairer) Repair(ctx context.Context, opID string, dryRun bool, progress operations.ProgressReporter) error {
	_, err := r.repairWithResult(ctx, opID, dryRun, progress)
	return err
}

// resolvedTrack records one resolution decision. Used inside the
// operation worker for logging + report assembly.
type resolvedTrack struct {
	PID     string `json:"pid"`
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Tier    string `json:"tier"`
	BookID  string `json:"book_id,omitempty"`
}

// repairWithResult is the operation body. Returns the result struct so
// tests can assert on counts; the JSON-encoded form is also persisted
// to the operation row via UpdateOperationResultData.
func (r *PathRepairer) repairWithResult(ctx context.Context, opID string, dryRun bool, progress operations.ProgressReporter) (iTunesPathRepairResult, error) {
	if r.store == nil {
		return iTunesPathRepairResult{}, fmt.Errorf("database not initialized")
	}
	if r.cfg.XMLPath == "" {
		return iTunesPathRepairResult{}, fmt.Errorf("iTunes XMLPath not configured")
	}

	_ = progress.Log("info", fmt.Sprintf("iTunes path repair started: xml=%s dry_run=%t", r.cfg.XMLPath, dryRun), nil)

	lib, err := itunes.ParseLibrary(r.cfg.XMLPath)
	if err != nil {
		return iTunesPathRepairResult{}, fmt.Errorf("parse iTunes library: %w", err)
	}

	result := iTunesPathRepairResult{XMLTracks: len(lib.Tracks), DryRun: dryRun}
	_ = progress.UpdateProgress(0, len(lib.Tracks), "scanning iTunes locations")

	// Tier B is built lazily — only constructed if tier A leaves
	// residue. Walking the audiobook root is the expensive step and
	// we don't want to pay it on libraries where every iTunes path
	// resolves cleanly via the DB.
	var tierB tagScanner
	getTierB := func() tagScanner {
		if tierB == nil {
			if r.cfg.AudiobookRoot == "" || r.bookIDExtractor == nil {
				tierB = noopTagScanner{}
			} else {
				_ = progress.Log("info",
					fmt.Sprintf("tier B: scanning audiobook root for embedded book IDs root=%s", r.cfg.AudiobookRoot), nil)
				tierB = newFSTagScanner(r.cfg.AudiobookRoot, r.bookIDExtractor)
			}
		}
		return tierB
	}

	scanned := 0
	for _, track := range lib.Tracks {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		scanned++
		if !itunes.IsAudiobook(track) {
			continue
		}
		decoded, derr := itunes.DecodeLocation(track.Location)
		if derr != nil || decoded == "" {
			continue
		}
		if pathExists(decoded) {
			continue
		}
		result.Missing++

		// Single PID → bookID lookup shared by tier A and tier B.
		bookID := lookupBookID(r.store, track.PersistentID)

		// Tier A: bookID → DB-known on-disk path
		if newPath, ok := resolveTierA(r.store, track.PersistentID, bookID, pathExists); ok {
			result.AutoResolved++
			_ = progress.Log("info",
				fmt.Sprintf("repair pid=%s old=%s new=%s tier=A action=%s",
					track.PersistentID, decoded, newPath, applyAction(dryRun)), nil)
			continue
		}

		// Tier B: embedded AUDIOBOOK_ORGANIZER_ID tag scan
		if newPath, ok := resolveTierB(getTierB(), bookID, pathExists); ok {
			result.AutoResolved++
			_ = progress.Log("info",
				fmt.Sprintf("repair pid=%s old=%s new=%s tier=B action=%s",
					track.PersistentID, decoded, newPath, applyAction(dryRun)), nil)
			continue
		}

		// Tier C: fuzzy candidates for human review. Never auto-applied.
		info := trackInfo{Title: track.Name, OldBasename: filepath.Base(decoded)}
		candidates := resolveTierC(getTierB().allPaths(), info, tierCThreshold, tierCTopN)
		if len(candidates) > 0 {
			result.NeedsReview++
			result.NeedsReviewItems = append(result.NeedsReviewItems, needsReviewItem{
				PID:        track.PersistentID,
				OldPath:    decoded,
				Title:      track.Name,
				Candidates: candidates,
			})
			_ = progress.Log("info",
				fmt.Sprintf("repair pid=%s old=%s tier=C candidates=%d action=review",
					track.PersistentID, decoded, len(candidates)), nil)
			continue
		}

		result.Unresolved++
		_ = progress.Log("debug",
			fmt.Sprintf("missing pid=%s old=%s tier=ABC unresolved", track.PersistentID, decoded), nil)
	}

	if err := persistRepairResult(r.store, opID, result); err != nil {
		log.Printf("[WARN] persist repair result: %v", err)
	}
	_ = operations.ClearState(r.store, opID)

	summary := fmt.Sprintf(
		"iTunes path repair complete: tracks=%d missing=%d auto=%d review=%d unresolved=%d enqueued=%d dry_run=%t",
		result.XMLTracks, result.Missing, result.AutoResolved, result.NeedsReview, result.Unresolved, result.Enqueued, result.DryRun,
	)
	_ = progress.Log("info", summary, nil)
	_ = progress.UpdateProgress(scanned, scanned, summary)
	return result, nil
}

// pathExists is the production existsFn for the resolver. Test code
// may inject a fake.
func pathExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// applyAction returns the human-readable action label for a single
// repair, distinguishing dry-run reports from real writes.
func applyAction(dryRun bool) string {
	if dryRun {
		return "report"
	}
	return "enqueue"
}

// persistRepairResult JSON-encodes the result and stores it on the
// operation row so the API can fetch the report after the run.
func persistRepairResult(store database.OperationStore, opID string, result iTunesPathRepairResult) error {
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return store.UpdateOperationResultData(opID, string(b))
}
