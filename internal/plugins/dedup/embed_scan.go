// file: internal/plugins/dedup/embed_scan.go
// version: 1.0.0
// guid: e2f3a4b5-c6d7-8901-bcde-f12345678901
// last-edited: 2026-05-06

package dedup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dedupengine "github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) embedScanDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "dedup.embed-scan",
		Plugin:          "dedup",
		DisplayName:     "Embed all books",
		Description:     "Re-embeds every primary book that lacks a fresh embedding.",
		ResumePolicy:    sdk.ResumeRequeue,
		DefaultPriority: sdk.PriorityLow,
		ConcurrencyKey:  "dedup.embed-scan",
		Cancellable:     true,
		Isolate:         false,
		Timeout:         120 * time.Minute,
		Run:             p.runEmbedScan,
	}
}

func (p *Plugin) runEmbedScan(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	if p.engine == nil {
		return errors.New("dedup engine not available (embedding may be disabled or API key not configured)")
	}

	loadProg := sdk.NewProgress(reporter, 0)
	loadProg.Start("Loading books for embedding...")

	books, err := p.store.GetAllBooks(0, 0)
	if err != nil {
		return fmt.Errorf("load books: %w", err)
	}
	total := len(books)
	if total == 0 {
		loadProg.Done("No books to embed")
		return nil
	}

	prog := sdk.NewProgress(reporter, total)
	prog.Start(fmt.Sprintf("Embedding books: 0 / %d", total))

	var embedded, cached, skipped, errs int
	for i, book := range books {
		if reporter.IsCanceled() {
			return context.Canceled
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		status, embedErr := p.engine.EmbedBook(ctx, book.ID)
		if embedErr != nil {
			reporter.Logger().Error("embed error", "book_id", book.ID, "error", embedErr)
			errs++
		} else {
			switch status {
			case dedupengine.EmbedStatusEmbedded:
				embedded++
			case dedupengine.EmbedStatusCached:
				cached++
			default:
				skipped++
			}
		}

		if i%50 == 0 || i == total-1 {
			prog.StepN(i+1,
				fmt.Sprintf("Embedding books: %d / %d (new=%d cached=%d skipped=%d errors=%d)",
					i+1, total, embedded, cached, skipped, errs))
		}
	}

	prog.Finalize("writing results...")
	prog.Done(fmt.Sprintf("Embedding complete — %d new, %d cached, %d skipped, %d errors (of %d books)",
		embedded, cached, skipped, errs, total))
	return nil
}
