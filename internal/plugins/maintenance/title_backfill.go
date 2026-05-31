// file: internal/plugins/maintenance/title_backfill.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/titleutil"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

type titleBackfillParams struct {
	DryRun bool `json:"dryRun"`
}

func (p *Plugin) titleBackfillDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "maintenance.title-backfill",
		Plugin:          "maintenance",
		DisplayName:     "Strip chapter prefixes from book titles",
		Description:     "Scans all books and removes leading chapter/track markers from Book.Title (e.g. '(76/85) Tarkin' → 'Tarkin'). Default dry-run previews changes; set dryRun=false to apply.",
		ResumePolicy:    sdk.ResumeDrop,
		DefaultPriority: sdk.PriorityLow,
		ConcurrencyKey:  "maintenance.title-backfill",
		Cancellable:     true,
		Isolate:         false,
		Timeout:         30 * time.Minute,
		Schedule:        nil,
		Capabilities:    []sdk.Capability{sdk.CapLibraryRead, sdk.CapLibraryWrite},
		Run:             p.runTitleBackfill,
	}
}

func (p *Plugin) runTitleBackfill(ctx context.Context, raw json.RawMessage, reporter sdk.Reporter) error {
	params := titleBackfillParams{DryRun: true} // safe default
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return fmt.Errorf("invalid params: %w", err)
		}
	}

	store := p.deps.Store()
	if store == nil {
		return fmt.Errorf("database not initialized")
	}

	if params.DryRun {
		_ = reporter.Log(slog.LevelInfo, "DRY RUN — no changes will be written")
	}

	const pageSize = 500
	offset := 0
	var totalScanned, totalChanged, totalSkipped, errCount int

	prog := sdk.NewProgress(reporter, 0) // total unknown at start
	prog.Start("Scanning books for poisoned titles...")

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		books, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			return fmt.Errorf("GetAllBooks offset=%d: %w", offset, err)
		}
		if len(books) == 0 {
			break
		}

		for _, book := range books {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			totalScanned++

			// TODO (learning-mode contribution): implement the decision predicate.
			// Given book.Title (the current stored title), decide:
			//   1. cleaned := titleutil.StripChapterPrefix(book.Title)
			//   2. If cleaned == book.Title or cleaned == "" → skip (log at Debug if changed is empty)
			//   3. Otherwise log "old → new" at Info, and if !params.DryRun call store.UpdateBook
			//
			// Constraints to consider:
			//   - An empty cleaned result means the entire title WAS the prefix (e.g. "03 - ").
			//     We should NOT write an empty title — skip and warn instead.
			//   - UpdateBook replaces ALL columns; preserve the rest of the book struct.
			//     Set book.Title = cleaned before passing &book.
			//   - Count: totalChanged++ on write (or dry-run would-write); totalSkipped++
			//     on skip; errCount++ on store error (log Warn, don't abort).
			//
			// ~8 lines of code. Fill in below:

			cleaned := titleutil.StripChapterPrefix(book.Title)

			switch {
			case cleaned == book.Title:
				// Nothing to strip — clean title
				totalSkipped++
			case cleaned == "":
				// Entire title was a prefix — don't blank the title
				_ = reporter.Log(slog.LevelWarn, fmt.Sprintf(
					"book %s: title %q stripped to empty, skipping", book.ID, book.Title))
				totalSkipped++
			default:
				_ = reporter.Log(slog.LevelInfo, fmt.Sprintf(
					"book %s: %q → %q", book.ID, book.Title, cleaned))
				totalChanged++
				if !params.DryRun {
					book.Title = cleaned
					if _, err := store.UpdateBook(book.ID, &book); err != nil {
						_ = reporter.Log(slog.LevelWarn, fmt.Sprintf(
							"book %s: UpdateBook failed: %v", book.ID, err))
						errCount++
						totalChanged-- // didn't actually change
					}
				}
			}
		}

		offset += len(books)
		prog.StepN(totalScanned, fmt.Sprintf("Scanned %d books, %d to update so far...", totalScanned, totalChanged))

		if len(books) < pageSize {
			break
		}
	}

	suffix := ""
	if params.DryRun {
		suffix = " (dry run — no writes)"
	}
	result := fmt.Sprintf("Scanned %d books: %d titles updated, %d skipped, %d errors%s",
		totalScanned, totalChanged, totalSkipped, errCount, suffix)
	_ = reporter.Log(slog.LevelInfo, result)
	prog.Done(result)

	if errCount > 0 {
		return fmt.Errorf("%d UpdateBook errors (see op log for details)", errCount)
	}
	return nil
}
