// file: internal/maintenance/jobs/relink_report.go
// version: 1.0.0
// guid: a1000022-0000-0000-0000-000000000022
// last-edited: 2026-05-03

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&relinkReportJob{}) }

type relinkReportJob struct{}

func (j *relinkReportJob) ID() string          { return "relink-report" }
func (j *relinkReportJob) Description() string { return "Report missing iTunes-linked files that may be relinkable" }
func (j *relinkReportJob) CanResume() bool     { return false }
func (j *relinkReportJob) Run(_ context.Context, _ database.Store, reporter maintenance.ProgressReporter, _ bool) error {
	reporter.Log("warn", "relink-report: requires iTunes service — use legacy /api/v1/maintenance/relink-missing-to-itunes route", nil)
	return nil
}
