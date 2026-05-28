// file: internal/server/library_list_warmer.go
// version: 1.1.0
// guid: 7e8d9a0b-1c2d-3e4f-5a6b-7c8d9e0f1a2b

// Pre-warms svc.audiobookService.listCache by firing the queries the UI
// is most likely to hit on first load — library page (first few pages,
// title asc + desc), default plain list. Runs once at startup after
// memdb is published; otherwise the first user load eats the full
// pushdown + cache-miss cost (~3+ minutes on a 50K-book library).

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	audiobookspkg "github.com/jdfalk/audiobook-organizer/internal/audiobooks"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// buildListCacheRawQuery constructs the URL.RawQuery string in the exact
// order/format the React UI's URLSearchParams emits in getBooks(). Must
// match web/src/services/api.ts:getBooks line-for-line or the cache key
// won't collide with what the handler computes from c.Request.URL.RawQuery.
func buildListCacheRawQuery(limit, offset int, f audiobookspkg.ListFilters) string {
	var parts []string
	add := func(k, v string) { parts = append(parts, k+"="+url.QueryEscape(v)) }

	add("limit", strconv.Itoa(limit))
	add("offset", strconv.Itoa(offset))
	if f.SortBy != "" {
		add("sort_by", f.SortBy)
	}
	if f.SortOrder != "" {
		add("sort_order", f.SortOrder)
	}
	if len(f.Tags) > 0 {
		for _, t := range f.Tags {
			add("tags[]", t)
		}
	} else if f.Tag != "" {
		add("tag", f.Tag)
	}
	if f.LibraryState != "" {
		add("library_state", f.LibraryState)
	}
	// FieldFilters + PerUserFilters travel together in the UI's `filters`
	// JSON param; the handler splits them after Unmarshal. Combine here so
	// the cache key matches.
	combined := append([]audiobookspkg.FieldFilter{}, f.FieldFilters...)
	combined = append(combined, f.PerUserFilters...)
	if len(combined) > 0 {
		if b, err := json.Marshal(combined); err == nil {
			add("filters", string(b))
		}
	}
	if f.FingerprintStatus != "" {
		add("fingerprint_status", f.FingerprintStatus)
	}
	if f.CoveragePercentMin != nil {
		add("coverage_percent_min", strconv.Itoa(*f.CoveragePercentMin))
	}
	if f.CoveragePercentMax != nil {
		add("coverage_percent_max", strconv.Itoa(*f.CoveragePercentMax))
	}
	// UI always sets is_primary_version=true last; we only set it when the
	// caller asked for primary-only (the UI's default).
	if f.IsPrimaryVersion != nil && *f.IsPrimaryVersion {
		add("is_primary_version", "true")
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "&"
		}
		out += p
	}
	return out
}

func typeName(v interface{}) string { return fmt.Sprintf("%T", v) }

// memReadyChecker is satisfied by *database.PebbleStore. Decoupled
// behind an interface so tests can stub it.
type memReadyChecker interface {
	IsMemReady() bool
}

// storeUnwrapper is implemented by decorator types (e.g. indexedStore)
// that wrap an inner Store. Used to peel layers and reach the concrete
// PebbleStore for capability checks like IsMemReady.
type storeUnwrapper interface {
	Unwrap() database.Store
}

// unwrapStore peels Unwrap()-implementing decorators until reaching the
// innermost Store. Bounded to 8 levels as a sanity guard against cycles.
func unwrapStore(s database.Store) database.Store {
	for i := 0; i < 8; i++ {
		w, ok := s.(storeUnwrapper)
		if !ok {
			return s
		}
		inner := w.Unwrap()
		if inner == nil || inner == s {
			return s
		}
		s = inner
	}
	return s
}

// resolveDefaultUserID returns a UserID to warm per-user filtered
// queries against (read_status, progress_pct, ...). Prefers "admin",
// falls back to the first user in ListUsers. Returns "" if nothing
// is available — caller should skip per-user warm-up in that case.
func (s *Server) resolveDefaultUserID() string {
	store := s.Store()
	if store == nil {
		return ""
	}
	if u, err := store.GetUserByUsername("admin"); err == nil && u != nil {
		return u.ID
	}
	users, err := store.ListUsers()
	if err == nil && len(users) > 0 {
		return users[0].ID
	}
	return ""
}

// warmAudiobookListCache waits for memdb to publish, then fires the
// common library-page queries to populate svc.audiobookService.listCache.
// All queries run sequentially against the AudiobookService — no extra
// HTTP overhead.
func (s *Server) warmAudiobookListCache() {
	// HOTFIX 2026-05-28: disabled. Running 177 list queries against a
	// 392K-book memdb on startup OOM'd at 67GB peak. The transient
	// per-query working set (full-set load + filter + sort) is the
	// killer, not the cache itself. See critical-issues memory:
	// project_cache_warmup_memory_fix. Re-enable only with:
	//  - bounded concurrency (1 at a time, explicit runtime.GC between)
	//  - ONLY 1-3 queries the UI hits on first load, not 177 combos
	//  - a hard memory ceiling check before each call
	slog.Info("library list warm-up DISABLED (OOM hotfix 2026-05-28)")
	return
	if s.audiobookService == nil {
		return
	}
	// The Server's store is wrapped by indexedStore (and possibly other
	// decorators). Peel them to reach the concrete PebbleStore so the
	// IsMemReady() type assertion succeeds.
	rawStore := unwrapStore(s.Store())
	checker, ok := rawStore.(memReadyChecker)
	if !ok {
		slog.Warn("library list warm-up: store doesn't expose IsMemReady, skipping",
			"store_type", typeName(rawStore))
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
	// library_state filter — organized / imported / suspicious. Common
	// left-rail filter, first 5 pages each.
	for _, state := range []string{"organized", "imported", "suspicious"} {
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
	// Tag filters — favorites and read are the popular ones; first 5 pages each.
	for _, tag := range []string{"favorites", "read"} {
		for off := 0; off < 100; off += 20 {
			queries = append(queries, qry{
				name:   "tag-" + tag,
				limit:  20,
				offset: off,
				filters: audiobookspkg.ListFilters{
					IsPrimaryVersion: &primaryTrue,
					Tag:              tag,
					SortBy:           "title",
					SortOrder:        "asc",
				},
			})
		}
	}
	// -tag:read (unread) — first 5 pages.
	for off := 0; off < 100; off += 20 {
		queries = append(queries, qry{
			name:   "not-tag-read",
			limit:  20,
			offset: off,
			filters: audiobookspkg.ListFilters{
				IsPrimaryVersion: &primaryTrue,
				SortBy:           "title",
				SortOrder:        "asc",
				FieldFilters: []audiobookspkg.FieldFilter{
					{Field: "tag", Value: "read", Negated: true},
				},
			},
		})
	}
	// FieldFilter-style binary toggles: yes/no for has_cover, has_written,
	// needs_writeback, has_organized. First 5 pages each.
	binaryFields := []string{"has_cover", "has_written", "needs_writeback", "has_organized"}
	for _, field := range binaryFields {
		for _, val := range []string{"yes", "no"} {
			// needs_writeback:no isn't useful — skip to halve count.
			if field == "needs_writeback" && val == "no" {
				continue
			}
			for off := 0; off < 100; off += 20 {
				queries = append(queries, qry{
					name:   field + "-" + val,
					limit:  20,
					offset: off,
					filters: audiobookspkg.ListFilters{
						IsPrimaryVersion: &primaryTrue,
						SortBy:           "title",
						SortOrder:        "asc",
						FieldFilters: []audiobookspkg.FieldFilter{
							{Field: field, Value: val},
						},
					},
				})
			}
		}
	}
	// review:no_match — already covered review:matched + -review:matched
	// above; add the explicit "rejected" path.
	for off := 0; off < 60; off += 20 {
		queries = append(queries, qry{
			name:   "review-no-match",
			limit:  20,
			offset: off,
			filters: audiobookspkg.ListFilters{
				IsPrimaryVersion: &primaryTrue,
				SortBy:           "title",
				SortOrder:        "asc",
				FieldFilters: []audiobookspkg.FieldFilter{
					{Field: "review", Value: "no_match"},
				},
			},
		})
	}
	// language:en — top-of-list locale filter, first 5 pages.
	for off := 0; off < 100; off += 20 {
		queries = append(queries, qry{
			name:   "language-en",
			limit:  20,
			offset: off,
			filters: audiobookspkg.ListFilters{
				IsPrimaryVersion: &primaryTrue,
				SortBy:           "title",
				SortOrder:        "asc",
				FieldFilters: []audiobookspkg.FieldFilter{
					{Field: "language", Value: "en"},
				},
			},
		})
	}
	// NOT author:Unknown — exclude untagged books, first 5 pages.
	for off := 0; off < 100; off += 20 {
		queries = append(queries, qry{
			name:   "not-author-unknown",
			limit:  20,
			offset: off,
			filters: audiobookspkg.ListFilters{
				IsPrimaryVersion: &primaryTrue,
				SortBy:           "title",
				SortOrder:        "asc",
				FieldFilters: []audiobookspkg.FieldFilter{
					{Field: "author", Value: "Unknown", Negated: true},
				},
			},
		})
	}
	// Format filter — m4b, mp3, m4a. First 5 pages each.
	for _, fmt := range []string{"m4b", "mp3", "m4a"} {
		for off := 0; off < 100; off += 20 {
			queries = append(queries, qry{
				name:   "format-" + fmt,
				limit:  20,
				offset: off,
				filters: audiobookspkg.ListFilters{
					IsPrimaryVersion: &primaryTrue,
					SortBy:           "title",
					SortOrder:        "asc",
					FieldFilters: []audiobookspkg.FieldFilter{
						{Field: "format", Value: fmt},
					},
				},
			})
		}
	}
	// Compound triage queries straight from the filter cheatsheet — these
	// are how the user actually finds work to do. First 5 pages each.
	type compound struct {
		name string
		ff   []audiobookspkg.FieldFilter
		ls   string
	}
	compounds := []compound{
		// Fully processed books: review:matched has_written:yes has_organized:yes
		{name: "fully-processed", ff: []audiobookspkg.FieldFilter{
			{Field: "review", Value: "matched"},
			{Field: "has_written", Value: "yes"},
			{Field: "has_organized", Value: "yes"},
		}},
		// Organized but needs metadata + file write
		{name: "organized-needs-metadata-write", ls: "organized", ff: []audiobookspkg.FieldFilter{
			{Field: "review", Value: "matched", Negated: true},
			{Field: "has_written", Value: "yes", Negated: true},
		}},
		// Metadata applied but not written to files
		{name: "matched-not-written", ff: []audiobookspkg.FieldFilter{
			{Field: "review", Value: "matched"},
			{Field: "has_written", Value: "yes", Negated: true},
		}},
		// Written but not organized
		{name: "written-not-organized", ff: []audiobookspkg.FieldFilter{
			{Field: "review", Value: "matched"},
			{Field: "has_written", Value: "yes"},
			{Field: "has_organized", Value: "yes", Negated: true},
		}},
		// Matched but missing cover art
		{name: "matched-no-cover", ff: []audiobookspkg.FieldFilter{
			{Field: "review", Value: "matched"},
			{Field: "has_cover", Value: "no"},
		}},
		// Imported books needing metadata
		{name: "imported-needs-metadata", ls: "imported", ff: []audiobookspkg.FieldFilter{
			{Field: "review", Value: "matched", Negated: true},
		}},
	}
	for _, c := range compounds {
		for off := 0; off < 100; off += 20 {
			queries = append(queries, qry{
				name:   c.name,
				limit:  20,
				offset: off,
				filters: audiobookspkg.ListFilters{
					IsPrimaryVersion: &primaryTrue,
					LibraryState:     c.ls,
					SortBy:           "title",
					SortOrder:        "asc",
					FieldFilters:     c.ff,
				},
			})
		}
	}
	// Per-user state filters (read_status / progress_pct). These require
	// a UserID — only fire if we can resolve a default admin user.
	// Without UserID the per-user filter is silently skipped and the
	// warm-up entry would be a duplicate of an existing one.
	if adminID := s.resolveDefaultUserID(); adminID != "" {
		perUserGroups := []struct {
			name string
			pu   []audiobookspkg.FieldFilter
		}{
			{name: "read-finished", pu: []audiobookspkg.FieldFilter{{Field: "read_status", Value: "finished"}}},
			{name: "read-in-progress", pu: []audiobookspkg.FieldFilter{{Field: "read_status", Value: "in_progress"}}},
			{name: "read-not-finished", pu: []audiobookspkg.FieldFilter{{Field: "read_status", Value: "finished", Negated: true}}},
			{name: "progress-gt-75", pu: []audiobookspkg.FieldFilter{{Field: "progress_pct", Value: ">75"}}},
		}
		for _, g := range perUserGroups {
			for off := 0; off < 60; off += 20 {
				queries = append(queries, qry{
					name:   g.name,
					limit:  20,
					offset: off,
					filters: audiobookspkg.ListFilters{
						IsPrimaryVersion: &primaryTrue,
						SortBy:           "title",
						SortOrder:        "asc",
						PerUserFilters:   g.pu,
						UserID:           adminID,
					},
				})
			}
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
	hits, misses, cached := 0, 0, 0
	for _, q := range queries {
		qStart := time.Now()
		resp, err := s.buildAudiobookListResponse(ctx, q.limit, q.offset, "", nil, nil, q.filters, false)
		if err != nil {
			misses++
			slog.Warn("library list warm-up query failed",
				"name", q.name, "offset", q.offset, "err", err)
			continue
		}
		hits++
		// Populate the HTTP handler's listCache with a key that matches
		// what c.Request.URL.RawQuery will produce for the same UI URL.
		// PerUserFilters queries are deliberately not cached by the
		// handler (would leak across users) — skip them here too.
		if len(q.filters.PerUserFilters) == 0 {
			raw := buildListCacheRawQuery(q.limit, q.offset, q.filters)
			s.listCache.Set("list:"+raw, resp)
			cached++
		}
		slog.Debug("library list warm-up query ok",
			"name", q.name, "offset", q.offset,
			"duration_ms", time.Since(qStart).Milliseconds())
	}
	slog.Info("library list warm-up complete",
		"queries_warmed", hits,
		"cache_entries_populated", cached,
		"failures", misses,
		"duration_ms", time.Since(started).Milliseconds(),
	)
}
