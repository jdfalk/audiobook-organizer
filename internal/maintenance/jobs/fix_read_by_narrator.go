// file: internal/maintenance/jobs/fix_read_by_narrator.go
// version: 1.0.0
// guid: a1000001-0000-0000-0000-000000000001
// last-edited: 2026-05-03

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&fixReadByNarratorJob{}) }

type fixReadByNarratorJob struct{}

func (j *fixReadByNarratorJob) ID() string          { return "fix-read-by-narrator" }
func (j *fixReadByNarratorJob) Description() string { return "Fix books where narrator was recorded as author" }
func (j *fixReadByNarratorJob) CanResume() bool     { return false }
func (j *fixReadByNarratorJob) Run(_ context.Context, _ database.Store, reporter maintenance.ProgressReporter, _ bool) error {
	reporter.Log("warn", "fix-read-by-narrator: complex parsing — use legacy /api/v1/maintenance/fix-read-by-narrator route", nil)
	return nil
}
