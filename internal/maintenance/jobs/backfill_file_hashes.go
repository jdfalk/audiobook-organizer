// file: internal/maintenance/jobs/backfill_file_hashes.go
// version: 1.1.0
// guid: a1000014-0000-0000-0000-000000000014
// last-edited: 2026-05-01

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
)

func init() { maintenance.Register(&backfillFileHashesJob{}) }

type backfillFileHashesJob struct{}

func (j *backfillFileHashesJob) ID() string          { return "backfill-file-hashes" }
func (j *backfillFileHashesJob) Name() string     { return "Backfill File Hashes" }
func (j *backfillFileHashesJob) Category() string { return "files" }
func (j *backfillFileHashesJob) DefaultParams() any { return struct{ DryRun bool `json:"dry_run"` }{DryRun: false} }
func (j *backfillFileHashesJob) Description() string { return "Compute and store file hashes for book_files missing them" }
func (j *backfillFileHashesJob) CanResume() bool     { return false }
func (j *backfillFileHashesJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	files, err := store.GetAllBookFiles()
	if err != nil {
		return err
	}
	reporter.SetTotal(len(files))
	hashed := 0
	for i := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reporter.Increment()
		bf := files[i]
		if bf.FileHash != "" {
			continue
		}
		hash, herr := scanner.ComputeFileHash(bf.FilePath)
		if herr != nil {
			msg := herr.Error()
			reporter.Log("warn", "backfill-file-hashes: hash failed for "+bf.FilePath, &msg)
			continue
		}
		if !dryRun {
			if serr := store.SetBookFileHash(bf.ID, hash); serr != nil {
				msg := serr.Error()
				reporter.Log("error", "backfill-file-hashes: SetBookFileHash failed", &msg)
				continue
			}
		}
		hashed++
	}
	_ = hashed
	reporter.Log("info", "backfill-file-hashes complete", nil)
	return nil
}
