// file: internal/maintenance/jobs/refetch_missing_authors.go
// version: 1.0.0
// guid: a1000012-0000-0000-0000-000000000012
// last-edited: 2026-05-03

package jobs

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&refetchMissingAuthorsJob{}) }

type refetchMissingAuthorsJob struct{}

func (j *refetchMissingAuthorsJob) ID() string          { return "refetch-missing-authors" }
func (j *refetchMissingAuthorsJob) Description() string { return "Report books missing an author record" }
func (j *refetchMissingAuthorsJob) CanResume() bool     { return false }
func (j *refetchMissingAuthorsJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, _ bool) error {
	books, err := store.GetAllBooks(0, 0)
	if err != nil {
		return err
	}
	reporter.SetTotal(len(books))
	count := 0
	for i := range books {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reporter.Increment()
		if books[i].AuthorID == nil {
			count++
			reporter.Log("info", "missing author: "+books[i].Title, nil)
		}
	}
	_ = count
	reporter.Log("info", "refetch-missing-authors complete", nil)
	return nil
}
