// file: internal/server/library_list_warmer.go
// version: 1.0.0
// guid: 7e8d9a0b-1c2d-3e4f-5a6b-7c8d9e0f1a2b

// Pre-warms svc.audiobookService.listCache by firing the queries the UI
// is most likely to hit on first load — library page (first few pages,
// title asc + desc), default plain list. Runs once at startup after
// memdb is published; otherwise the first user load eats the full
// pushdown + cache-miss cost (~3+ minutes on a 50K-book library).

package server

import (
	"context"
	"log/slog"
	"time"

	audiobookspkg "github.com/jdfalk/audiobook-organizer/internal/audiobooks"
)

// memReadyChecker is satisfied by *database.PebbleStore. Decoupled
// behind an interface so tests can stub it.
type memReadyChecker interface {
	IsMemReady() bool
}

// warmAudiobookListCache waits for memdb to publish, then fires the
// common library-page queries to populate svc.audiobookService.listCache.
// All queries run sequentially against the AudiobookService — no extra
// HTTP overhead.
func (s *Server) warmAudiobookListCache() {
	if s.audiobookService == nil {
		return
	}
	store := s.Store()
	checker, ok := store.(memReadyChecker)
	if !ok {
		// Store doesn't expose memdb state — skip; we'd just warm cold
		// queries with no upside.
		return
	}

	// Wait up to 5 minutes for memdb. Memdb warmup on a 50K-book DB
	// typically takes ~2.5 min, so 5 min is comfortable headroom.
	deadline := time.Now().Add(5 * time.Minute)
	for !checker.IsMemReady() {
		if time.Now().After(deadline) {
			slog.Warn("library list warm-up: memdb not ready after 5 min, skipping")
			return
		}
		time.Sleep(2 * time.Second)
	}

	started := time.Now()
	ctx := context.Background()

	// Each query mirrors what the UI sends on a fresh library-page load.
	// Pagination keys are independent cache entries, so we warm a lot of
	// pages — RAM here means less Pebble cache thrash + zero cold-miss
	// for the user. The server has plenty of memory.
	type qry struct {
		name    string
		limit   int
		offset  int
		filters audiobookspkg.ListFilters
	}
	primaryTrue := true
	queries := []qry{}
	// Default UI sort: title asc, primary only — warm the first 20 pages
	// (400 books deep). Covers nearly all browsing without the user
	// hitting a cold cache.
	for off := 0; off < 400; off += 20 {
		queries = append(queries, qry{
			name:    "title-asc-primary",
			limit:   20,
			offset:  off,
			filters: audiobookspkg.ListFilters{IsPrimaryVersion: &primaryTrue, SortBy: "title", SortOrder: "asc"},
		})
	}
	// Title desc — first 5 pages.
	for off := 0; off < 100; off += 20 {
		queries = append(queries, qry{
			name:    "title-desc-primary",
			limit:   20,
			offset:  off,
			filters: audiobookspkg.ListFilters{IsPrimaryVersion: &primaryTrue, SortBy: "title", SortOrder: "desc"},
		})
	}
	// "-review:matched" — books that have NOT yet been matched to metadata.
	// One of the most common filters per user feedback. First 10 pages.
	for off := 0; off < 200; off += 20 {
		queries = append(queries, qry{
			name:   "review-not-matched",
			limit:  20,
			offset: off,
			filters: audiobookspkg.ListFilters{
				IsPrimaryVersion: &primaryTrue,
				SortBy:           "title",
				SortOrder:        "asc",
				FieldFilters: []audiobookspkg.FieldFilter{
					{Field: "review", Value: "matched", Negated: true},
				},
			},
		})
	}
	// "review:matched" inverse (already-matched books).
	for off := 0; off < 100; off += 20 {
		queries = append(queries, qry{
			name:   "review-matched",
			limit:  20,
			offset: off,
			filters: audiobookspkg.ListFilters{
				IsPrimaryVersion: &primaryTrue,
				SortBy:           "title",
				SortOrder:        "asc",
				FieldFilters: []audiobookspkg.FieldFilter{
					{Field: "review", Value: "matched"},
				},
			},
		})
	}
	// library_state filter — organized vs imported. Common left-rail filter.
	for _, state := range []string{"organized", "imported"} {
		for off := 0; off < 100; off += 20 {
			queries = append(queries, qry{
				name:   "library-state-" + state,
				limit:  20,
				offset: off,
				filters: audiobookspkg.ListFilters{
					IsPrimaryVersion: &primaryTrue,
					LibraryState:     state,
					SortBy:           "title",
					SortOrder:        "asc",
				},
			})
		}
	}
	// Plain list, no filter — sidebar/import flow first page.
	queries = append(queries, qry{name: "plain-page-1", limit: 20, offset: 0})
	queries = append(queries, qry{
		name:    "primary-no-sort",
		limit:   20,
		offset:  0,
		filters: audiobookspkg.ListFilters{IsPrimaryVersion: &primaryTrue},
	})

	slog.Info("library list warm-up starting", "queries", len(queries))
	hits, misses := 0, 0
	for _, q := range queries {
		qStart := time.Now()
		_, err := s.audiobookService.GetAudiobooks(ctx, q.limit, q.offset, "", nil, nil, q.filters)
		if err != nil {
			misses++
			slog.Warn("library list warm-up query failed",
				"name", q.name, "offset", q.offset, "err", err)
			continue
		}
		hits++
		slog.Debug("library list warm-up query ok",
			"name", q.name, "offset", q.offset,
			"duration_ms", time.Since(qStart).Milliseconds())
	}
	slog.Info("library list warm-up complete",
		"queries_warmed", hits,
		"failures", misses,
		"duration_ms", time.Since(started).Milliseconds(),
	)
}
