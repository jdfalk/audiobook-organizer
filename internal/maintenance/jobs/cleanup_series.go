// file: internal/maintenance/jobs/cleanup_series.go
// version: 1.0.0
// guid: a1000002-0000-0000-0000-000000000002
// last-edited: 2026-05-03

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&cleanupSeriesJob{}) }

type cleanupSeriesJob struct{}

func (j *cleanupSeriesJob) ID() string          { return "cleanup-series" }
func (j *cleanupSeriesJob) Description() string { return "Cleanup and normalize series names" }
func (j *cleanupSeriesJob) CanResume() bool     { return false }
func (j *cleanupSeriesJob) Run(_ context.Context, _ database.Store, reporter maintenance.ProgressReporter, _ bool) error {
	reporter.Log("warn", "cleanup-series: requires server services — use legacy /api/v1/maintenance/cleanup-series route", nil)
	return nil
}
