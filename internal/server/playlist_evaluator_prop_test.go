// file: internal/server/playlist_evaluator_prop_test.go
// version: 1.0.0
// guid: bcc094f5-1645-44d3-be21-3087888fdaea

// Property-based tests for the smart-playlist evaluator in
// internal/server/playlist_evaluator.go. These properties express
// invariants that must hold regardless of input, drawing their fuzz
// inputs from the shared rapidgen generators:
//
//   - Limit is respected: EvaluateSmartPlaylist(limit=N) returns at
//     most N IDs. Truncation happens AFTER sort, so any N-sized cap
//     on an arbitrarily-large candidate set must hold.
//   - Empty query errors: empty or whitespace-only queries return an
//     error without crashing or returning a non-nil result slice.
//   - Deterministic evaluation: same query + same seeded index +
//     same user yields byte-identical results across repeated calls.
//   - Sort is stable: passing a sortJSON and re-evaluating preserves
//     exact ID order; a stable in-memory sort must be idempotent.
//   - Per-user filter isolation: user A's read_status=finished rows
//     never appear in user B's `read_status:finished` evaluation,
//     no matter what books either user has touched.
//
// Each property uses a fresh PebbleStore + bleve index per
// rapid.Check iteration so state never leaks between shrinks or
// between properties.

package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/search"
	"github.com/jdfalk/audiobook-organizer/internal/testutil/rapidgen"
	"pgregory.net/rapid"
)

// buildPropEvalFixture creates a fresh PebbleStore and Bleve index,
// seeds them with n random Books (matching rapidgen.Book), and
// returns the store, index, and the list of book IDs assigned by
// CreateBook. Each Book is indexed under a shared author keyword
// ("common-author") so property queries can match the full set
// without depending on the randomly-generated titles.
func buildPropEvalFixture(t *testing.T, rt *rapid.T, n int) (*database.PebbleStore, *search.BleveIndex, []string) {
	t.Helper()
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	idx, err := search.Open(filepath.Join(t.TempDir(), "bleve"))
	if err != nil {
		t.Fatalf("bleve open: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		b := rapidgen.Book(rt)
		created, err := store.CreateBook(b)
		if err != nil {
			t.Fatalf("create book: %v", err)
		}
		yr := 0
		if b.PrintYear != nil {
			yr = *b.PrintYear
		}
		if err := idx.IndexBook(search.BookDocument{
			BookID: created.ID,
			Title:  b.Title,
			Author: "common-author",
			Format: b.Format,
			Year:   yr,
		}); err != nil {
			t.Fatalf("index book: %v", err)
		}
		ids = append(ids, created.ID)
	}
	return store, idx, ids
}

// TestProp_LimitIsRespected verifies that
// EvaluateSmartPlaylist(limit=N) returns at most N IDs for any query
// that produces a non-trivial candidate set.
func TestProp_LimitIsRespected(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(1, 12).Draw(rt, "n_books")
		limit := rapid.IntRange(1, 20).Draw(rt, "limit")

		store, idx, _ := buildPropEvalFixture(t, rt, n)

		got, err := EvaluateSmartPlaylist(store, idx, "author:common-author", "", limit, "_local")
		if err != nil {
			rt.Fatalf("evaluate: %v", err)
		}
		if len(got) > limit {
			rt.Fatalf("limit=%d violated: got %d ids", limit, len(got))
		}
	})
}

// TestProp_EmptyQueryErrors verifies that empty / whitespace-only
// queries always return a non-nil error and a nil result slice.
func TestProp_EmptyQueryErrors(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Draw a random whitespace-only string (possibly empty).
		ws := rapid.StringMatching(`[ \t\n\r]{0,16}`).Draw(rt, "whitespace")

		store, idx, _ := buildPropEvalFixture(t, rt, 1)

		got, err := EvaluateSmartPlaylist(store, idx, ws, "", 0, "_local")
		if err == nil {
			rt.Fatalf("empty query %q should error, got %v", ws, got)
		}
		if got != nil {
			rt.Fatalf("empty query should return nil ids, got %v", got)
		}
	})
}

// TestProp_DeterministicEvaluation verifies that evaluating the same
// query against the same seeded store+index twice yields the same
// result IDs in the same order.
func TestProp_DeterministicEvaluation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(1, 10).Draw(rt, "n_books")
		limit := rapid.IntRange(0, 20).Draw(rt, "limit")

		store, idx, _ := buildPropEvalFixture(t, rt, n)

		first, err := EvaluateSmartPlaylist(store, idx, "author:common-author", "", limit, "_local")
		if err != nil {
			rt.Fatalf("first evaluate: %v", err)
		}
		second, err := EvaluateSmartPlaylist(store, idx, "author:common-author", "", limit, "_local")
		if err != nil {
			rt.Fatalf("second evaluate: %v", err)
		}
		if len(first) != len(second) {
			rt.Fatalf("non-deterministic lengths: %d vs %d", len(first), len(second))
		}
		for i := range first {
			if first[i] != second[i] {
				rt.Fatalf("non-deterministic at %d: %q vs %q (first=%v second=%v)",
					i, first[i], second[i], first, second)
			}
		}
	})
}

// TestProp_SortIsStable verifies that evaluating twice with the same
// sortJSON produces byte-identical order. sortBookIDs uses
// sort.SliceStable with a deterministic comparator, so the second
// call must reproduce the first call exactly.
func TestProp_SortIsStable(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(2, 10).Draw(rt, "n_books")
		field := rapid.SampledFrom([]string{"title", "year", "date_added"}).Draw(rt, "sort_field")
		direction := rapid.SampledFrom([]string{"asc", "desc"}).Draw(rt, "sort_dir")
		sortJSON := `[{"field":"` + field + `","direction":"` + direction + `"}]`

		store, idx, _ := buildPropEvalFixture(t, rt, n)

		first, err := EvaluateSmartPlaylist(store, idx, "author:common-author", sortJSON, 0, "_local")
		if err != nil {
			rt.Fatalf("first evaluate: %v", err)
		}
		second, err := EvaluateSmartPlaylist(store, idx, "author:common-author", sortJSON, 0, "_local")
		if err != nil {
			rt.Fatalf("second evaluate: %v", err)
		}
		if len(first) != len(second) {
			rt.Fatalf("sort produced different lengths: %d vs %d", len(first), len(second))
		}
		for i := range first {
			if first[i] != second[i] {
				rt.Fatalf("sort not stable at idx %d: %q vs %q", i, first[i], second[i])
			}
		}
	})
}

// TestProp_PerUserFilterIsolation verifies that per-user state
// written for user A never leaks into user B's evaluation. Filtering
// B on `read_status:finished` must return zero books when only A has
// any finished rows.
func TestProp_PerUserFilterIsolation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(1, 8).Draw(rt, "n_books")
		store, idx, bookIDs := buildPropEvalFixture(t, rt, n)

		// Mark every book finished for user "alice".
		now := time.Now()
		for _, id := range bookIDs {
			if err := store.SetUserBookState(&database.UserBookState{
				UserID:         "alice",
				BookID:         id,
				Status:         database.UserBookStatusFinished,
				StatusManual:   true,
				LastActivityAt: now,
			}); err != nil {
				rt.Fatalf("seed alice state for %s: %v", id, err)
			}
		}

		// Alice sees all her finished books.
		aliceGot, err := EvaluateSmartPlaylist(
			store, idx, "author:common-author read_status:finished", "", 0, "alice")
		if err != nil {
			rt.Fatalf("evaluate alice: %v", err)
		}
		if len(aliceGot) != n {
			rt.Fatalf("alice should see %d finished books, got %d: %v",
				n, len(aliceGot), aliceGot)
		}

		// Bob has no state at all — finished filter must reject every book.
		bobGot, err := EvaluateSmartPlaylist(
			store, idx, "author:common-author read_status:finished", "", 0, "bob")
		if err != nil {
			rt.Fatalf("evaluate bob: %v", err)
		}
		if len(bobGot) != 0 {
			rt.Fatalf("bob should see 0 finished books (no state), got %d: %v",
				len(bobGot), bobGot)
		}
	})
}
