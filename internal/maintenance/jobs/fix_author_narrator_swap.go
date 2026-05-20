// file: internal/maintenance/jobs/fix_author_narrator_swap.go
// version: 2.1.1
// guid: a1000003-0000-0000-0000-000000000003
// last-edited: 2026-05-01

package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"log/slog"
)

func init() { maintenance.Register(&fixAuthorNarratorSwapJob{}) }

type fixAuthorNarratorSwapJob struct{}

func (j *fixAuthorNarratorSwapJob) ID() string       { return "fix-author-narrator-swap" }
func (j *fixAuthorNarratorSwapJob) Name() string     { return "Fix Author/Narrator Swap" }
func (j *fixAuthorNarratorSwapJob) Category() string { return "library" }
func (j *fixAuthorNarratorSwapJob) DefaultParams() any {
	return struct {
		DryRun bool `json:"dry_run"`
	}{DryRun: true}
}
func (j *fixAuthorNarratorSwapJob) Description() string {
	return "Fix books where author and narrator fields are swapped"
}
func (j *fixAuthorNarratorSwapJob) CanResume() bool { return false }

func (j *fixAuthorNarratorSwapJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	const batchSize = 500
	offset := 0
	var found, applied int

	for {
		batch, err := store.GetAllBooks(batchSize, offset)
		if err != nil {
			return fmt.Errorf("failed to list books: %w", err)
		}
		if len(batch) == 0 {
			break
		}

		reporter.SetTotal(offset + len(batch))

		for i := range batch {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			book := &batch[i]
			if book.AuthorID == nil || book.Narrator == nil || *book.Narrator == "" {
				reporter.Increment()
				continue
			}

			author, aErr := store.GetAuthorByID(*book.AuthorID)
			if aErr != nil || author == nil {
				reporter.Increment()
				continue
			}

			if !strings.EqualFold(author.Name, *book.Narrator) {
				reporter.Increment()
				continue
			}

			found++
			if !dryRun {
				current, getErr := store.GetBookByID(book.ID)
				if getErr != nil || current == nil {
					slog.Error("Failed to fetch book", "book", book.ID, "getErr", getErr)
				} else {
					current.AuthorID = nil
					if _, updateErr := store.UpdateBook(book.ID, current); updateErr != nil {
						slog.Error("Failed to update book", "book", book.ID, "updateErr", updateErr)
					} else {
						applied++
					}
				}
			}
			reporter.Increment()
		}

		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	slog.Info("Done found applied dryRun", "found", found, "applied", applied, "dryRun", dryRun)
	return nil
}
