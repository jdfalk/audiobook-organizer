// file: internal/plugins/dedup/lsh_index_build.go
// version: 1.2.0
// guid: e61b955e-93bf-4ea6-bb1f-7acd30491fdb
// last-edited: 2026-06-11

package dedup

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/fingerprint"
	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

// lshBuildBatchSize is the number of BookFiles processed per PebbleDB
// batch write. 1000 keeps individual batch sizes well under Pebble's
// internal 4 MB recommendation while amortizing commit overhead across
// a large library (~15K whole-file fingerprints × 64 bands each = ~1 M
// key writes for a full rebuild).
const lshBuildBatchSize = 1000

// LSHIndexStore is the narrow store interface required by the
// lsh-index-build op. Using a narrow interface keeps the op decoupled
// from the concrete *PebbleStore while remaining testable with a mock.
//
// The *PebbleStore satisfies this interface; other store implementations
// may return errors from the LSH methods (they carry no index).
type LSHIndexStore interface {
	// GetAllBookFiles returns every BookFile row. The op iterates all files
	// and indexes only those with a non-empty AcoustIDFingerprint.
	GetAllBookFiles() ([]database.BookFile, error)

	// HasLSHIndex reports whether a BookFile already has an fpidx_meta row.
	// The op uses this to skip already-indexed files on incremental re-runs —
	// PutLSHEntries is idempotent but skipping avoids unnecessary writes.
	HasLSHIndex(bookFileID string) bool

	// PutLSHEntries writes the fpidx: index rows and fpidx_meta: member list
	// for (fileID, bookID, subprints, bands) atomically. Idempotent.
	PutLSHEntries(fileID, bookID string, subs []fingerprint.Subprint, bands []byte) error

	// IsLSHIndexBuilt / SetLSHIndexBuilt manage the versioned completion flag
	// lsh_index_v1_done. The op sets the flag on successful completion and
	// checks it to support the "already done" fast-path (though the op is
	// always resumable — re-running it is safe and continues from unindexed files).
	IsLSHIndexBuilt() bool
	SetLSHIndexBuilt() error

	// GetSetting / SetSetting are used to read and write the completion flag
	// via the standard settings key-value store.
	GetSetting(key string) (*database.Setting, error)
	SetSetting(key, value, dataType string, internal bool) error
}

// lshIndexBuildDef returns the OperationDef for the dedup.lsh-index-build op.
//
// Design decisions:
//   - ConcurrencyKey "dedup.lsh-index" prevents the T013 probe-collector from
//     racing an in-progress build (T013 reads the same keys this op writes).
//   - ResumeRequeue: on crash/restart, the op re-enqueues from scratch — but
//     HasLSHIndex skips already-indexed files so it effectively resumes.
//   - Timeout 120m: a full library rebuild on a 15K-file corpus with 64 bands
//     each needs ~1 M Pebble writes; benchmarking shows ~10 min on a cold NVMe,
//     leaving 10× headroom for slow disks.
func (p *Plugin) lshIndexBuildDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "dedup.lsh-index-build",
		Plugin:          "dedup",
		DisplayName:     "Build LSH fingerprint index",
		Description:     "Builds the fpidx: secondary index over whole-file AcoustID fingerprints, enabling fast near-duplicate lookup without a full O(N) scan.",
		ResumePolicy:    sdk.ResumeRequeue,
		DefaultPriority: sdk.PriorityLow,
		ConcurrencyKey:  "dedup.lsh-index",
		Cancellable:     true,
		Isolate:         false,
		Timeout:         120 * time.Minute,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
		},
		Run: p.runLSHIndexBuild,
	}
}

// runLSHIndexBuild is the op runner for dedup.lsh-index-build.
//
// Algorithm:
//  1. Load all BookFiles.
//  2. For each file with a whole-file fingerprint, derive subprints via
//     fingerprint.Subprints, then call PutLSHEntries — unless HasLSHIndex
//     already returns true (incremental skip).
//  3. Files without a fingerprint are collected by BookID; on completion,
//     acoustid.fingerprint-rescan is enqueued for those books so the next
//     lsh-index-build run can pick them up.
//  4. Report progress every lshBuildBatchSize files.
//  5. On completion, set lsh_index_v1_done so T013 can gate on it.
func (p *Plugin) runLSHIndexBuild(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	// Obtain the LSH-capable store via type assertion. The concrete
	// *PebbleStore satisfies LSHIndexStore; SQLite and mock stores may not.
	lshStore, ok := p.store.(LSHIndexStore)
	if !ok {
		return fmt.Errorf("store does not implement LSHIndexStore (PebbleDB required)")
	}

	slog.Info("lsh-index-build: starting")
	loadProg := sdk.NewProgress(reporter, 0)
	loadProg.Start("Loading BookFiles for LSH indexing…")

	files, err := lshStore.GetAllBookFiles()
	if err != nil {
		return fmt.Errorf("lsh-index-build: load files: %w", err)
	}
	total := len(files)
	if total == 0 {
		loadProg.Done("No BookFiles found — nothing to index")
		slog.Info("lsh-index-build: no files, exiting")
		return nil
	}
	slog.Info("lsh-index-build: loaded files", "total", total)

	prog := sdk.NewProgress(reporter, total)
	prog.Start(fmt.Sprintf("Indexing LSH bands: 0 / %d files", total))

	var indexed, skipped, noFP, noFPPermFailed, errs int
	// Track unique book IDs whose files lack a fingerprint so we can
	// enqueue acoustid.fingerprint-rescan for them after the main loop.
	// Books are only added if at least one file has never been tried
	// (FingerprintFailedAt == nil) — permanently-failed files are excluded
	// to prevent an infinite rescan loop.
	noFPBookSet := make(map[string]struct{})

	for i, f := range files {
		// Respect cancellation at each file boundary.
		if reporter.IsCanceled() {
			slog.Info("lsh-index-build: canceled", "processed", i, "indexed", indexed)
			return context.Canceled
		}
		select {
		case <-ctx.Done():
			slog.Info("lsh-index-build: context done", "processed", i, "indexed", indexed)
			return ctx.Err()
		default:
		}

		// Files without a fingerprint: only enqueue for fingerprinting if
		// they haven't permanently failed. Permanently-failed files (too short,
		// corrupt, DRM) will never yield a fingerprint — don't trigger an
		// infinite rescan loop. AcoustIDFingerprintDurationSec > 0 is the
		// memdb-safe proxy for "has whole-file fp" (fingerprint blob stripped
		// from memdb rows by stripBookFileForMemdb).
		if len(f.AcoustIDFingerprint) == 0 && f.AcoustIDFingerprintDurationSec == 0 {
			noFP++
			if f.FingerprintFailedAt == nil {
				noFPBookSet[f.BookID] = struct{}{}
			} else {
				noFPPermFailed++
			}
			continue
		}

		// Incremental resume: skip files that already have an fpidx_meta row.
		// PutLSHEntries is idempotent, but the skip avoids unnecessary writes
		// when re-running after a partial build.
		if lshStore.HasLSHIndex(f.ID) {
			skipped++
			continue
		}

		// If the fingerprint bytes are nil but the duration proxy indicates a
		// fingerprint was written (memdb strips AcoustIDFingerprint to save RAM),
		// we cannot compute subprints. Skip silently; a Pebble-direct run
		// (UseMemDB=false or cold-start) will index this file on the next pass.
		if len(f.AcoustIDFingerprint) == 0 {
			continue
		}

		subs, bands, fpErr := fingerprint.Subprints(f.AcoustIDFingerprint)
		if fpErr != nil {
			// Misaligned fingerprint bytes — log and continue. Don't abort
			// the entire build for one corrupt row.
			reporter.Logger().Error("lsh-index-build: Subprints error",
				"file_id", f.ID, "error", fpErr)
			errs++
			continue
		}
		if len(subs) == 0 {
			// Fingerprint too short to sample (< 4 frames after edge trim).
			// Apply same permanent-failure exclusion as the noFP path above.
			noFP++
			if f.FingerprintFailedAt == nil {
				noFPBookSet[f.BookID] = struct{}{}
			} else {
				noFPPermFailed++
			}
			continue
		}

		if putErr := lshStore.PutLSHEntries(f.ID, f.BookID, subs, bands); putErr != nil {
			reporter.Logger().Error("lsh-index-build: PutLSHEntries error",
				"file_id", f.ID, "error", putErr)
			errs++
			continue
		}
		indexed++

		// Progress every lshBuildBatchSize files and at the last file.
		if (i+1)%lshBuildBatchSize == 0 || i == total-1 {
			prog.StepN(i+1, fmt.Sprintf(
				"Indexing LSH bands: %d / %d files (indexed=%d skipped=%d noFP=%d permFailed=%d errors=%d)",
				i+1, total, indexed, skipped, noFP, noFPPermFailed, errs))
			slog.Info("lsh-index-build: progress",
				"processed", i+1, "total", total,
				"indexed", indexed, "skipped", skipped,
				"no_fp", noFP, "no_fp_perm_failed", noFPPermFailed, "errors", errs)
		}
	}

	prog.Finalize("writing completion flag…")

	// Enqueue fingerprint-rescan for books that had unfingerprinted files.
	// A subsequent lsh-index-build run will pick them up once fingerprinted.
	if len(noFPBookSet) > 0 {
		noFPBookIDs := make([]string, 0, len(noFPBookSet))
		for id := range noFPBookSet {
			noFPBookIDs = append(noFPBookIDs, id)
		}
		if p.registry != nil {
			_, enqErr := p.registry.EnqueueOp(ctx, "acoustid.fingerprint-rescan", map[string]any{
				"scope":    "books",
				"book_ids": noFPBookIDs,
			})
			if enqErr != nil {
				reporter.Logger().Warn("lsh-index-build: failed to enqueue fingerprint-rescan",
					"books", len(noFPBookIDs), "error", enqErr)
			} else {
				reporter.Logger().Info("lsh-index-build: queued fingerprint-rescan for unfingerprinted books",
					"books", len(noFPBookIDs), "files", noFP)
				slog.Info("lsh-index-build: enqueued fingerprint-rescan",
					"books", len(noFPBookIDs), "files", noFP)
			}
		} else {
			reporter.Logger().Warn("lsh-index-build: registry unavailable, cannot enqueue fingerprint-rescan",
				"no_fp_books", len(noFPBookIDs), "no_fp_files", noFP)
		}
	}

	// Mark the index as built so T013's probe-collector can enable itself.
	if flagErr := lshStore.SetLSHIndexBuilt(); flagErr != nil {
		reporter.Logger().Error("lsh-index-build: failed to set completion flag", "error", flagErr)
		// Non-fatal: the index is built even if the flag write fails.
		// The op will simply re-index skippable files on the next run,
		// but no data is lost.
	}

	summary := fmt.Sprintf(
		"LSH index build complete — %d indexed, %d skipped (already indexed), %d no-fingerprint (%d books queued for rescan, %d permanently failed), %d errors (of %d files)",
		indexed, skipped, noFP, len(noFPBookSet), noFPPermFailed, errs, total)
	prog.Done(summary)
	slog.Info("lsh-index-build: complete",
		"indexed", indexed, "skipped", skipped,
		"no_fp", noFP, "no_fp_books", len(noFPBookSet),
		"no_fp_perm_failed", noFPPermFailed, "errors", errs, "total", total)
	return nil
}
