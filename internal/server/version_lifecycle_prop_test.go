// file: internal/server/version_lifecycle_prop_test.go
// version: 1.1.0
// guid: d4c4cd2b-c578-4a11-8229-83a516271b1b

// Property-based tests for BookVersion lifecycle transitions (spec 4.5 task 6).
//
// These tests exercise the state machine described in version_lifecycle.go:
//   active → trash → (restore → alt) | (purge → inactive_purged)
//
// Each property uses a fresh PebbleStore per rapid.Check invocation so rapid
// can shrink freely without cross-iteration state bleed. Tests call the
// lifecycle primitives (autoPromoteAlt, purgeVersion, UpdateBookVersion)
// directly rather than going through the HTTP layer — the handlers are thin
// wrappers that delegate to those primitives after parameter parsing.

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/testutil/rapidgen"
	"pgregory.net/rapid"
)

// mkTempDir creates a fresh temp dir for a rapid iteration and registers
// cleanup on the rapid.T so it is removed after the iteration (including
// after shrinking).
func mkTempDir(t *rapid.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "version-lifecycle-prop-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// labelFor uniquifies a rapid draw label by appending the step index. Rapid
// requires distinct labels for every Draw call in the same iteration.
func labelFor(base string, step int) string {
	return fmt.Sprintf("%s_%d", base, step)
}

// newPropLifecycleStore spins up a fresh PebbleStore in a temp directory.
// Each rapid iteration gets its own store so the state machine starts clean.
// rapid.T does not expose t.TempDir() directly, so we mint one with
// os.MkdirTemp and register cleanup on rapid.T — cleanup fires after both
// normal completion and shrinking.
func newPropLifecycleStore(t *rapid.T) database.Store {
	t.Helper()
	dir := mkTempDir(t)
	store, err := database.NewPebbleStore(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// trashVersion mimics handleTrashVersion's core logic: sets status to trash
// and auto-promotes an alt if the trashed version was active.
func trashVersion(store database.Store, ver *database.BookVersion) error {
	wasActive := ver.Status == database.BookVersionStatusActive
	ver.Status = database.BookVersionStatusTrash
	if err := store.UpdateBookVersion(ver); err != nil {
		return err
	}
	if wasActive {
		return autoPromoteAlt(store, ver.BookID)
	}
	return nil
}

// restoreVersion mimics handleRestoreVersion: trash → alt, errors otherwise.
func restoreVersion(store database.Store, ver *database.BookVersion) error {
	if ver.Status != database.BookVersionStatusTrash {
		return fmt.Errorf("version is not in trash (status=%s)", ver.Status)
	}
	ver.Status = database.BookVersionStatusAlt
	return store.UpdateBookVersion(ver)
}

// ----------------------------------------------------------------------------
// Property: trash is reversible — active → trash → restore lands on alt.
// ----------------------------------------------------------------------------

func TestProp_TrashIsReversible(t *testing.T) {
	if testing.Short() {
		t.Skip("slow property test; run without -short")
	}
	rapid.Check(t, func(t *rapid.T) {
		store := newPropLifecycleStore(t)

		book, err := store.CreateBook(rapidgen.Book(t))
		if err != nil {
			t.Fatalf("create book: %v", err)
		}

		// Start with a single active version. IngestDate must be set so
		// downstream lookups don't choke on zero time.
		ver, err := store.CreateBookVersion(rapidgen.BookVersionActive(t, book.ID))
		if err != nil {
			t.Fatalf("create version: %v", err)
		}

		// active → trash
		if err := trashVersion(store, ver); err != nil {
			t.Fatalf("trash: %v", err)
		}
		afterTrash, _ := store.GetBookVersion(ver.ID)
		if afterTrash.Status != database.BookVersionStatusTrash {
			t.Fatalf("expected trash, got %s", afterTrash.Status)
		}

		// trash → restore → alt
		if err := restoreVersion(store, afterTrash); err != nil {
			t.Fatalf("restore: %v", err)
		}
		afterRestore, _ := store.GetBookVersion(ver.ID)
		if afterRestore.Status != database.BookVersionStatusAlt {
			t.Fatalf("expected alt after restore, got %s", afterRestore.Status)
		}
	})
}

// ----------------------------------------------------------------------------
// Property: purge is irreversible. trash → purge → inactive_purged.
// Restore on a purged version must fail.
// ----------------------------------------------------------------------------

func TestProp_PurgeIsIrreversible(t *testing.T) {
	if testing.Short() {
		t.Skip("slow property test; run without -short")
	}
	rapid.Check(t, func(t *rapid.T) {
		store := newPropLifecycleStore(t)

		book, err := store.CreateBook(rapidgen.Book(t))
		if err != nil {
			t.Fatalf("create book: %v", err)
		}

		// Start with a trash version. We bypass active→trash here because
		// this property is about purge's terminal behavior.
		vgen := rapidgen.BookVersion(t, book.ID)
		vgen.Status = database.BookVersionStatusTrash
		ver, err := store.CreateBookVersion(vgen)
		if err != nil {
			t.Fatalf("create version: %v", err)
		}

		// trash → purge
		if err := purgeVersion(store, ver); err != nil {
			t.Fatalf("purge: %v", err)
		}
		afterPurge, _ := store.GetBookVersion(ver.ID)
		if afterPurge == nil {
			t.Fatalf("purged version must still exist in store (row retained for fingerprint)")
		}
		if afterPurge.Status != database.BookVersionStatusInactivePurged {
			t.Fatalf("expected inactive_purged, got %s", afterPurge.Status)
		}
		if afterPurge.PurgedDate == nil {
			t.Errorf("PurgedDate must be set after purge")
		}

		// Restore on a purged version must fail.
		if err := restoreVersion(store, afterPurge); err == nil {
			t.Fatalf("restore of a purged version must fail, but it succeeded")
		}
	})
}

// ----------------------------------------------------------------------------
// Property: auto-promote picks the alt with the latest ingest date.
// ----------------------------------------------------------------------------

func TestProp_AutoPromotePicksMostRecent(t *testing.T) {
	if testing.Short() {
		t.Skip("slow property test; run without -short")
	}
	rapid.Check(t, func(t *rapid.T) {
		store := newPropLifecycleStore(t)

		book, err := store.CreateBook(rapidgen.Book(t))
		if err != nil {
			t.Fatalf("create book: %v", err)
		}

		// Active version — this one gets trashed.
		active, err := store.CreateBookVersion(rapidgen.BookVersionActive(t, book.ID))
		if err != nil {
			t.Fatalf("create active: %v", err)
		}

		// N alt versions, each with a random IngestDate drawn from rapidgen.
		// We need at least 1 alt so auto-promote has something to pick.
		n := rapid.IntRange(1, 5).Draw(t, "n_alts")
		alts := make([]*database.BookVersion, 0, n)
		for i := 0; i < n; i++ {
			altGen := rapidgen.BookVersion(t, book.ID)
			altGen.Status = database.BookVersionStatusAlt
			created, err := store.CreateBookVersion(altGen)
			if err != nil {
				t.Fatalf("create alt %d: %v", i, err)
			}
			alts = append(alts, created)
		}

		// Identify the expected winner: the alt with the latest IngestDate.
		// Ties are broken by whichever comes first in the scan order — we
		// mirror that by using strict After() like the production code.
		expected := alts[0]
		for _, a := range alts[1:] {
			if a.IngestDate.After(expected.IngestDate) {
				expected = a
			}
		}

		// Trash the active version; this triggers autoPromoteAlt.
		if err := trashVersion(store, active); err != nil {
			t.Fatalf("trash active: %v", err)
		}

		// Verify the expected alt is now active.
		promoted, _ := store.GetBookVersion(expected.ID)
		if promoted == nil {
			t.Fatalf("promoted version vanished")
		}
		if promoted.Status != database.BookVersionStatusActive {
			t.Errorf("expected promoted version %s to be active, got %s",
				expected.ID, promoted.Status)
		}

		// And that no other alt was promoted.
		for _, a := range alts {
			if a.ID == expected.ID {
				continue
			}
			got, _ := store.GetBookVersion(a.ID)
			if got == nil {
				continue
			}
			if got.Status == database.BookVersionStatusActive {
				t.Errorf("unexpected promotion: alt %s with ingest %v became active "+
					"(expected winner was %s with ingest %v)",
					a.ID, a.IngestDate, expected.ID, expected.IngestDate)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// Property: single-active invariant — after any sequence of trash/restore
// operations, at most one version per book has status=active.
// ----------------------------------------------------------------------------

// lifecycleOp enumerates the operations the random walk can apply.
type lifecycleOp int

const (
	opTrash lifecycleOp = iota
	opRestore
	opPurgeFromTrash
)

func TestProp_SingleActiveInvariantMaintained(t *testing.T) {
	if testing.Short() {
		t.Skip("slow property test; run without -short")
	}
	rapid.Check(t, func(t *rapid.T) {
		store := newPropLifecycleStore(t)

		book, err := store.CreateBook(rapidgen.Book(t))
		if err != nil {
			t.Fatalf("create book: %v", err)
		}

		// Seed: 1 active + 1..4 alts. The active version acts as the
		// primary; alts are candidates for auto-promotion.
		if _, err := store.CreateBookVersion(rapidgen.BookVersionActive(t, book.ID)); err != nil {
			t.Fatalf("create active: %v", err)
		}

		nAlts := rapid.IntRange(1, 4).Draw(t, "n_alts")
		for i := 0; i < nAlts; i++ {
			altGen := rapidgen.BookVersion(t, book.ID)
			altGen.Status = database.BookVersionStatusAlt
			if _, err := store.CreateBookVersion(altGen); err != nil {
				t.Fatalf("create alt %d: %v", i, err)
			}
		}

		// Apply a random sequence of 1..8 operations. Each step picks a
		// random version and a random legal op for that version's status.
		steps := rapid.IntRange(1, 8).Draw(t, "n_steps")
		for step := 0; step < steps; step++ {
			all, err := store.GetBookVersionsByBookID(book.ID)
			if err != nil {
				t.Fatalf("list versions: %v", err)
			}
			if len(all) == 0 {
				break
			}
			idx := rapid.IntRange(0, len(all)-1).Draw(t, labelFor("step_target", step))
			target := all[idx]

			// Choose an op compatible with the current status.
			switch target.Status {
			case database.BookVersionStatusActive,
				database.BookVersionStatusAlt:
				// Trash is always legal for active/alt.
				if err := trashVersion(store, &target); err != nil {
					t.Fatalf("trash %s: %v", target.ID, err)
				}
			case database.BookVersionStatusTrash:
				// Either restore or purge — pick randomly.
				op := rapid.SampledFrom([]lifecycleOp{opRestore, opPurgeFromTrash}).
					Draw(t, labelFor("step_op", step))
				switch op {
				case opRestore:
					if err := restoreVersion(store, &target); err != nil {
						t.Fatalf("restore %s: %v", target.ID, err)
					}
				case opPurgeFromTrash:
					if err := purgeVersion(store, &target); err != nil {
						t.Fatalf("purge %s: %v", target.ID, err)
					}
				}
			default:
				// inactive_purged, pending, blocked_for_redownload — skip.
				continue
			}

			// After each operation, the single-active invariant must hold.
			after, err := store.GetBookVersionsByBookID(book.ID)
			if err != nil {
				t.Fatalf("list versions after step %d: %v", step, err)
			}
			activeCount := 0
			for _, v := range after {
				if v.Status == database.BookVersionStatusActive {
					activeCount++
				}
			}
			if activeCount > 1 {
				t.Fatalf("single-active invariant violated after step %d: %d active versions",
					step, activeCount)
			}
		}
	})
}
