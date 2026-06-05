// file: internal/maintenance/jobs/hash_chain_integrity.go
// version: 1.0.0
// guid: e0f1a2b3-c4d5-6789-0abc-def012345678
// last-edited: 2026-06-07

package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

const (
	hashChainIntegrityJobID             = "hash-chain-integrity"
	hashChainIntegrityAlertErrorClass = "hash-chain-integrity"
)

type hashChainIntegrityJob struct{}

type fileErrorRecorder interface {
	RecordFileError(filePath, bookID, errClass, message string) error
}

func init() {
	maintenance.Register(&hashChainIntegrityJob{})
}

func (j *hashChainIntegrityJob) ID() string {
	return hashChainIntegrityJobID
}

func (j *hashChainIntegrityJob) Name() string {
	return "Hash chain integrity alert"
}

func (j *hashChainIntegrityJob) Category() string {
	return "files"
}

func (j *hashChainIntegrityJob) DefaultParams() any {
	return struct {
		DryRun bool `json:"dry_run"`
	}{DryRun: false}
}

func (j *hashChainIntegrityJob) Description() string {
	return "Flag files where the current hash does not match the original hash without an AO tag write"
}

func (j *hashChainIntegrityJob) CanResume() bool {
	return false
}

func (j *hashChainIntegrityJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	files, err := store.GetAllBookFiles()
	if err != nil {
		return err
	}
	reporter.SetTotal(len(files))
	recorder, recorderAvailable := store.(fileErrorRecorder)
	flagged := 0
	for i := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reporter.Increment()
		bf := files[i]
		if bf.FileHash == "" || bf.OriginalFileHash == "" || bf.FileHash == bf.OriginalFileHash {
			continue
		}
		if bf.PostMetadataHash != "" {
			continue
		}
		flagged++
		if recorderAvailable && !dryRun {
			detail := fmt.Sprintf("file_hash=%s original_file_hash=%s", bf.FileHash, bf.OriginalFileHash)
			if recErr := recorder.RecordFileError(bf.FilePath, bf.BookID, hashChainIntegrityAlertErrorClass, detail); recErr != nil {
				details := fmt.Sprintf("recorder failure: %v", recErr)
				reporter.Log("error", fmt.Sprintf("Failed to persist integrity alert for %s", bf.FilePath), &details)
				return fmt.Errorf("recording integrity alert for %s: %w", bf.FilePath, recErr)
			}
		}
	}

	recorderPersisted := recorderAvailable && !dryRun
	summary := fmt.Sprintf("Hash chain integrity scan complete: flagged %d files (dry_run=%t, recorder=%t)", flagged, dryRun, recorderPersisted)
	reporter.Log("info", summary, nil)
	if flagged > 0 && dryRun {
		reporter.Log("warn", "dry_run=true; flagged files were not persisted", nil)
	}
	if flagged > 0 && !dryRun && !recorderAvailable {
		reporter.Log("warn", "file error recorder unavailable; alerts cannot be persisted", nil)
	}
	slog.Info("hash-chain-integrity complete", "flagged", flagged, "dry_run", dryRun, "recorder_available", recorderAvailable)
	return nil
}
