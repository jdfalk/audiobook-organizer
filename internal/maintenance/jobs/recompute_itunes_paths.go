// file: internal/maintenance/jobs/recompute_itunes_paths.go
// version: 1.1.0
// guid: a1000013-0000-0000-0000-000000000013
// last-edited: 2026-05-01

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
)

func init() { maintenance.Register(&recomputeITunesPathsJob{}) }

type recomputeITunesPathsJob struct{}

func (j *recomputeITunesPathsJob) ID() string          { return "recompute-itunes-paths" }
func (j *recomputeITunesPathsJob) Name() string     { return "Recompute iTunes Paths" }
func (j *recomputeITunesPathsJob) Category() string { return "itunes" }
func (j *recomputeITunesPathsJob) DefaultParams() any { return struct{ DryRun bool `json:"dry_run"` }{DryRun: false} }
func (j *recomputeITunesPathsJob) Description() string { return "Recompute iTunes path mapping for all book files" }
func (j *recomputeITunesPathsJob) CanResume() bool     { return false }
func (j *recomputeITunesPathsJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
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
		want := metafetch.ComputeITunesPath(bf.FilePath)
		if want == bf.ITunesPath {
			continue
		}
		if !dryRun {
			bf.ITunesPath = want
			if uerr := store.UpdateBookFile(bf.ID, &bf); uerr != nil {
				msg := uerr.Error()
				reporter.Log("error", "recompute-itunes-paths: UpdateBookFile failed", &msg)
				continue
			}
		}
		updated++
	}
	_ = updated
	reporter.Log("info", "recompute-itunes-paths complete", nil)
	return nil
}
