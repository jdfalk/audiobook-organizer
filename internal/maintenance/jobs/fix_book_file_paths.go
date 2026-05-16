// file: internal/maintenance/jobs/fix_book_file_paths.go
// version: 1.1.0
// guid: a1000011-0000-0000-0000-000000000011
// last-edited: 2026-05-01

package jobs

import (
	"context"
	"os"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&fixBookFilePathsJob{}) }

type fixBookFilePathsJob struct{}

func (j *fixBookFilePathsJob) ID() string       { return "fix-book-file-paths" }
func (j *fixBookFilePathsJob) Name() string     { return "Fix Book File Paths" }
func (j *fixBookFilePathsJob) Category() string { return "files" }
func (j *fixBookFilePathsJob) DefaultParams() any {
	return struct {
		DryRun bool `json:"dry_run"`
	}{DryRun: true}
}
func (j *fixBookFilePathsJob) Description() string {
	return "Mark book_files as missing when they no longer exist on disk"
}
func (j *fixBookFilePathsJob) CanResume() bool { return false }
func (j *fixBookFilePathsJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	files, err := store.GetAllBookFiles()
	if err != nil {
		return err
	}
	reporter.SetTotal(len(files))
	marked := 0
	for i := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reporter.Increment()
		bf := files[i]
		if bf.Missing {
			continue
		}
		if _, serr := os.Stat(bf.FilePath); os.IsNotExist(serr) {
			if !dryRun {
				bf.Missing = true
				if uerr := store.UpdateBookFile(bf.ID, &bf); uerr != nil {
					msg := uerr.Error()
					reporter.Log("error", "fix-book-file-paths: UpdateBookFile failed", &msg)
					continue
				}
			}
			marked++
		}
	}
	_ = marked
	reporter.Log("info", "fix-book-file-paths complete", nil)
	return nil
}
