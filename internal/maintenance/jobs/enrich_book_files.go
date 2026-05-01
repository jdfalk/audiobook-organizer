// file: internal/maintenance/jobs/enrich_book_files.go
// version: 1.1.0
// guid: a1000009-0000-0000-0000-000000000009
// last-edited: 2026-05-01

package jobs

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&enrichBookFilesJob{}) }

var trackNumRe = regexp.MustCompile(`^(\d+)[\s._\-]`)

type enrichBookFilesJob struct{}

func (j *enrichBookFilesJob) ID() string          { return "enrich-book-files" }
func (j *enrichBookFilesJob) Name() string     { return "Enrich Book Files" }
func (j *enrichBookFilesJob) Category() string { return "files" }
func (j *enrichBookFilesJob) DefaultParams() any { return struct{ DryRun bool `json:"dry_run"` }{DryRun: false} }
func (j *enrichBookFilesJob) Description() string { return "Backfill track numbers for book_files from filenames" }
func (j *enrichBookFilesJob) CanResume() bool     { return false }
func (j *enrichBookFilesJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	files, err := store.GetAllBookFiles()
	if err != nil {
		return err
	}
	reporter.SetTotal(len(files))
	updated := 0
	for i := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reporter.Increment()
		bf := files[i]
		if bf.TrackNumber != 0 {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(bf.FilePath), filepath.Ext(bf.FilePath))
		m := trackNumRe.FindStringSubmatch(stem)
		if m == nil {
			continue
		}
		n, perr := strconv.Atoi(m[1])
		if perr != nil || n <= 0 {
			continue
		}
		if !dryRun {
			bf.TrackNumber = n
			if uerr := store.UpdateBookFile(bf.ID, &bf); uerr != nil {
				msg := uerr.Error()
				reporter.Log("error", "enrich-book-files: UpdateBookFile failed", &msg)
				continue
			}
		}
		updated++
	}
	_ = updated
	reporter.Log("info", "enrich-book-files complete", nil)
	return nil
}
