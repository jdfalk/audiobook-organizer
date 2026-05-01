// file: internal/maintenance/jobs/fix_author_narrator_swap.go
// version: 1.0.0
// guid: a1000003-0000-0000-0000-000000000003
// last-edited: 2026-05-03

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&fixAuthorNarratorSwapJob{}) }

type fixAuthorNarratorSwapJob struct{}

func (j *fixAuthorNarratorSwapJob) ID() string          { return "fix-author-narrator-swap" }
func (j *fixAuthorNarratorSwapJob) Description() string { return "Fix books where author and narrator fields are swapped" }
func (j *fixAuthorNarratorSwapJob) CanResume() bool     { return false }
func (j *fixAuthorNarratorSwapJob) Run(_ context.Context, _ database.Store, reporter maintenance.ProgressReporter, _ bool) error {
	reporter.Log("warn", "fix-author-narrator-swap: complex heuristics — use legacy /api/v1/maintenance/fix-author-narrator-swap route", nil)
	return nil
}
