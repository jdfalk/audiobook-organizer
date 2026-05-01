// file: internal/maintenance/jobs/cleanup_organize_mess.go
// version: 1.0.0
// guid: a1000007-0000-0000-0000-000000000007
// last-edited: 2026-05-03

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&cleanupOrganizeMess{}) }

type cleanupOrganizeMess struct{}

func (j *cleanupOrganizeMess) ID() string          { return "cleanup-organize-mess" }
func (j *cleanupOrganizeMess) Description() string { return "Clean up leftover organize artifacts and garbage directories" }
func (j *cleanupOrganizeMess) CanResume() bool     { return false }
func (j *cleanupOrganizeMess) Run(_ context.Context, _ database.Store, reporter maintenance.ProgressReporter, _ bool) error {
	reporter.Log("warn", "cleanup-organize-mess: requires server services — use legacy /api/v1/maintenance/cleanup-organize-mess route", nil)
	return nil
}
