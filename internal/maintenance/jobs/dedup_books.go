// file: internal/maintenance/jobs/dedup_books.go
// version: 1.0.0
// guid: a1000010-0000-0000-0000-000000000010
// last-edited: 2026-05-03

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&dedupBooksJob{}) }

type dedupBooksJob struct{}

func (j *dedupBooksJob) ID() string          { return "dedup-books" }
func (j *dedupBooksJob) Description() string { return "Detect and merge duplicate books" }
func (j *dedupBooksJob) CanResume() bool     { return false }
func (j *dedupBooksJob) Run(_ context.Context, _ database.Store, reporter maintenance.ProgressReporter, _ bool) error {
	reporter.Log("warn", "dedup-books: requires server services — use legacy /api/v1/maintenance/dedup-books route", nil)
	return nil
}
