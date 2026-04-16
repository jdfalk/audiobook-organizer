// file: internal/server/embedding_backfill.go
// version: 1.8.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-e1f2a3b4c5d6

package server

import (
	"context"
	"log"
)

// backfillVersionMarker identifies the current generation of the dedup
// backfill pipeline. Bumping this string causes a one-time re-run on the
// next startup so older deployments can pick up new rules. Rule history:
//   - v1: initial backfill (PR #203)
//   - v2: non-primary-version filtering, same-group pair skipping, and
//     Layer 1 exact checks during FullScan (PR #207)
//   - v3: skip books with empty/near-empty titles, skip Layer 1 + Layer 2
//     matches where books are distinct volumes of a numbered series
//     (PR #208, first iteration)
//   - v4: expanded series-marker regex to include "bk", "vol", "volume",
//     "number", "no", "part", "pt", "episode", "ep", "#", and added a
//     last-ditch digit-only-difference fallback to catch series volumes
//     whose marker the regex doesn't recognize
//
// The backfill stays idempotent — individual EmbedBook calls skip on
// cached text_hash — so re-running it just pays a few seconds of DB reads
// plus a purge pass to delete candidates that the new rules would have
// rejected.
const backfillVersionMarker = "embedding_backfill_v5_done"

// runEmbeddingBackfill embeds all books and authors on first startup and
// re-runs once after each backfill version bump.
func (s *Server) runEmbeddingBackfill() {
	store := s.Store()
	if store == nil || s.dedupEngine == nil {
		return
	}

	// Check if backfill already done at the current version
	if setting, err := store.GetSetting(backfillVersionMarker); err == nil && setting != nil && setting.Value == "true" {
		log.Printf("[INFO] Embedding backfill already complete (%s), skipping", backfillVersionMarker)
		return
	}
	log.Printf("[INFO] Starting embedding backfill (%s)...", backfillVersionMarker)

	// Use the server's background context so Shutdown can cancel this
	// goroutine instead of leaving it iterating Pebble while CloseStore
	// tries to tear down the database. If bgCtx is nil (e.g. unit tests
	// instantiating Server without NewServer), fall back to Background.
	ctx := s.bgCtx
	if ctx == nil {
		ctx = context.Background()
	}
	offset := 0

	// Honest counters: the previous version of this loop reported
	// "N books embedded" for every successful EmbedBook return, which
	// included non-primary skips, empty-title skips, and cached-hash
	// no-ops. A re-run against a stable library would report ~24K
	// "embedded" books even though zero API calls had been made and
	// roughly half the records were non-primary version siblings the
	// scorer never touches. We now count each EmbedStatus into its own
	// bucket and log a breakdown at the end.
	var (
		statEmbedded            int
		statCached              int
		statSkippedNonPrimary   int
		statSkippedEmptyTitle   int
		statErrors              int
	)
	visited := 0
	nextProgressAt := 500

	// Backfill books in batches
	for {
		// Honor shutdown — if the server is stopping, bail out fast so
		// we're not holding iterators / DB connections when CloseStore
		// runs. The marker stays unset on cancel, which is the right
		// thing: we want the next startup to resume the backfill.
		if ctx.Err() != nil {
			log.Printf("[INFO] Embedding backfill canceled during book loop at offset %d: %v", offset, ctx.Err())
			return
		}
		books, err := store.GetAllBooks(100, offset)
		if err != nil || len(books) == 0 {
			break
		}
		for _, book := range books {
			if ctx.Err() != nil {
				log.Printf("[INFO] Embedding backfill canceled mid-batch at offset %d", offset)
				return
			}
			status, err := s.dedupEngine.EmbedBook(ctx, book.ID)
			if err != nil {
				log.Printf("[WARN] backfill embed book %s: %v", book.ID, err)
				statErrors++
				visited++
				continue
			}
			switch status {
			case EmbedStatusEmbedded:
				statEmbedded++
			case EmbedStatusCached:
				statCached++
			case EmbedStatusSkippedNonPrimary:
				statSkippedNonPrimary++
			case EmbedStatusSkippedEmptyTitle:
				statSkippedEmptyTitle++
			}
			visited++
		}
		offset += len(books)
		if visited >= nextProgressAt {
			log.Printf("[INFO] Embedding backfill progress: %d books visited (embedded=%d cached=%d skipped_non_primary=%d skipped_empty_title=%d)",
				visited, statEmbedded, statCached, statSkippedNonPrimary, statSkippedEmptyTitle)
			for nextProgressAt <= visited {
				nextProgressAt += 500
			}
		}
	}
	log.Printf("[INFO] Book backfill complete: visited=%d embedded=%d cached=%d skipped_non_primary=%d skipped_empty_title=%d errors=%d",
		visited, statEmbedded, statCached, statSkippedNonPrimary, statSkippedEmptyTitle, statErrors)

	// Backfill authors
	authorCount := 0
	authors, err := store.GetAllAuthors()
	if err != nil {
		log.Printf("[WARN] backfill: failed to get authors: %v", err)
	} else {
		for _, author := range authors {
			if ctx.Err() != nil {
				log.Printf("[INFO] Embedding backfill canceled during author loop after %d authors", authorCount)
				return
			}
			if err := s.dedupEngine.EmbedAuthor(ctx, author.ID); err != nil {
				log.Printf("[WARN] backfill embed author %d: %v", author.ID, err)
			} else {
				authorCount++
			}
		}
	}
	log.Printf("[INFO] Embedded %d authors", authorCount)

	log.Printf("[INFO] Embedding backfill complete: %d books (embedded=%d, cached=%d), %d authors",
		visited, statEmbedded, statCached, authorCount)

	// Persist the backfill marker NOW, before FullScan runs. The embedding
	// work itself is complete; FullScan is a follow-up exact-match/similarity
	// pass that happens to live in the same function. It used to come right
	// before the SetSetting call, which meant any crash or panic during
	// FullScan — and we've been hitting a Pebble "element has outstanding
	// references" panic pretty reliably — would leave the marker unset and
	// the next restart would pointlessly re-embed 24K books from the cached
	// text_hash (fast but not free, and it blocks the API while it runs).
	// Writing the marker here makes the expensive part idempotent across
	// crashes. If FullScan fails, the user can still trigger a Re-scan from
	// the UI; they just won't re-pay for embedding work that's already done.
	if err := store.SetSetting(backfillVersionMarker, "true", "bool", false); err != nil {
		log.Printf("[WARN] failed to persist backfill marker: %v — backfill will re-run next startup", err)
	} else {
		log.Printf("[INFO] Embedding backfill marker persisted (%s)", backfillVersionMarker)
	}

	// Purge stale candidates from any previous scan before running a new
	// one. This is what cleans up the 16K+ non-primary / same-group rows
	// left over from pre-fix backfills — on subsequent startups, the
	// cleanup is a no-op because FullScan won't create those rows anymore.
	if deleted, err := s.dedupEngine.PurgeStaleCandidates(ctx); err != nil {
		log.Printf("[WARN] backfill: purge stale candidates error: %v", err)
	} else if deleted > 0 {
		log.Printf("[INFO] Purged %d stale dedup candidate(s) before initial scan", deleted)
	}

	// Run full dedup scan with a bucket-crossing progress logger (see
	// newDedupScanProgressLogger for why a naive `done%N == 0` check fails).
	// Wrapped in defer/recover because this is the block that has been
	// panicking with a Pebble ref-count error mid-scan. A panic here should
	// leave the process running — the marker is already set above, the
	// dedup queue is usable, and the crash was killing the whole server on
	// startup which meant the user couldn't even reach the UI to investigate.
	log.Printf("[INFO] Running initial dedup scan...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ERROR] Initial dedup scan panicked: %v — backfill marker is already set, server will continue", r)
			}
		}()
		progressFn := newDedupScanProgressLogger(1000, func(format string, args ...any) {
			log.Printf(format, args...)
		})
		if err := s.dedupEngine.FullScan(ctx, progressFn); err != nil {
			log.Printf("[WARN] Initial dedup scan failed: %v", err)
		}
	}()

	log.Printf("[INFO] Embedding backfill and initial dedup scan complete (%s)", backfillVersionMarker)
}

// newDedupScanProgressLogger returns a progress callback suitable for
// DedupEngine.FullScan that logs once every `interval` books processed (plus
// one final line at completion).
//
// It exists because FullScan passes `done = i+1` at a step of 10, so values
// are 1, 11, 21, ... which never satisfy `done % interval == 0` for interval
// ≥ 11. This previously hid all scan progress. The returned closure tracks
// the next threshold internally and advances past it on each bucket crossing,
// so progress lines appear at ~interval granularity regardless of the caller's
// step size.
func newDedupScanProgressLogger(interval int, logf func(format string, args ...any)) func(done, total int) {
	if interval <= 0 {
		interval = 1
	}
	nextLog := interval
	return func(done, total int) {
		if done >= nextLog || (total > 0 && done == total) {
			logf("[INFO] Dedup scan progress: %d/%d", done, total)
			for nextLog <= done {
				nextLog += interval
			}
		}
	}
}
