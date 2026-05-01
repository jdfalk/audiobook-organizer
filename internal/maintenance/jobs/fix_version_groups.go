// file: internal/maintenance/jobs/fix_version_groups.go
// version: 1.0.0
// guid: a1000004-0000-0000-0000-000000000004
// last-edited: 2026-05-03

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&fixVersionGroupsJob{}) }

type fixVersionGroupsJob struct{}

func (j *fixVersionGroupsJob) ID() string          { return "fix-version-groups" }
func (j *fixVersionGroupsJob) Description() string { return "Fix and normalize version groups" }
func (j *fixVersionGroupsJob) CanResume() bool     { return false }
func (j *fixVersionGroupsJob) Run(_ context.Context, _ database.Store, reporter maintenance.ProgressReporter, _ bool) error {
	reporter.Log("warn", "fix-version-groups: requires server services — use legacy /api/v1/maintenance/fix-version-groups route", nil)
	return nil
}
