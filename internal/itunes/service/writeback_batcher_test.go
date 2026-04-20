// file: internal/itunes/service/writeback_batcher_test.go
// version: 1.1.0
// guid: d4e5f6a7-b8c9-0123-def4-56789abcdef0

package itunesservice

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// withFakeITLHooks replaces the package-level itunes hooks with
// test doubles and restores them on cleanup. Lets every SafeWriteITL
// test drive the validate + apply paths without needing a real ITL
// fixture on disk — the fixture is fragile and format changes have
// broken it before.
func withFakeITLHooks(t *testing.T, validate func(string) error, apply func(in, out string, ops itunes.ITLOperationSet) (*itunes.ITLWriteBackResult, error)) {
	t.Helper()
	prevValidate := itlValidateFn
	prevApply := itlApplyOperationsFn
	itlValidateFn = validate
	itlApplyOperationsFn = apply
	t.Cleanup(func() {
		itlValidateFn = prevValidate
		itlApplyOperationsFn = prevApply
	})
}

// makeITL writes a placeholder "ITL" file for tests. Since our
// ValidateITL hook is mocked, the contents don't need to parse —
// they just need to exist so os.ReadFile succeeds during backup.
func makeITL(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("makeITL: %v", err)
	}
	return p
}

// TestSafeWriteITL_HappyPath covers the full success cycle:
// source validates, backup is written, apply produces a temp,
// temp validates, rename lands, final validates. End state: the
// original is replaced, one backup exists, no temp remains.
func TestSafeWriteITL_HappyPath(t *testing.T) {
	dir := t.TempDir()
	itlPath := makeITL(t, dir, "library.itl", "original-content")

	applyCalls := 0
	withFakeITLHooks(t,
		func(path string) error {
			// Every validate call passes.
			return nil
		},
		func(in, out string, ops itunes.ITLOperationSet) (*itunes.ITLWriteBackResult, error) {
			applyCalls++
			// Simulate the real function: read in, transform, write out.
			data, err := os.ReadFile(in)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(out, append(data, '!'), 0o644); err != nil {
				return nil, err
			}
			return &itunes.ITLWriteBackResult{UpdatedCount: 1, OutputPath: out}, nil
		},
	)

	err := SafeWriteITL(itlPath, itunes.ITLOperationSet{
		LocationUpdates: []itunes.ITLLocationUpdate{{PersistentID: "aa", NewLocation: "x"}},
	})
	if err != nil {
		t.Fatalf("SafeWriteITL happy path failed: %v", err)
	}
	if applyCalls != 1 {
		t.Errorf("expected 1 apply call, got %d", applyCalls)
	}

	// Original file now has the transform applied.
	out, _ := os.ReadFile(itlPath)
	if string(out) != "original-content!" {
		t.Errorf("final file content = %q, want %q", string(out), "original-content!")
	}

	// Exactly one backup exists.
	backups, _ := filepath.Glob(itlPath + ".bak-*")
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	// Backup contains the pre-write bytes.
	backup, _ := os.ReadFile(backups[0])
	if string(backup) != "original-content" {
		t.Errorf("backup content = %q, want %q", string(backup), "original-content")
	}

	// No .tmp left over.
	if _, err := os.Stat(itlPath + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp to be cleaned up")
	}
}

// TestSafeWriteITL_RefusesBrokenSource is the regression test for
// rule #1: never write to an ITL that's already corrupted. The
// validate call for the source path fails, and SafeWriteITL must
// abort without creating a backup or touching the file.
func TestSafeWriteITL_RefusesBrokenSource(t *testing.T) {
	dir := t.TempDir()
	itlPath := makeITL(t, dir, "library.itl", "corrupted")

	withFakeITLHooks(t,
		func(path string) error {
			return errors.New("bad magic bytes")
		},
		func(in, out string, ops itunes.ITLOperationSet) (*itunes.ITLWriteBackResult, error) {
			t.Fatal("apply must not be called when source validation fails")
			return nil, nil
		},
	)

	err := SafeWriteITL(itlPath, itunes.ITLOperationSet{
		LocationUpdates: []itunes.ITLLocationUpdate{{PersistentID: "aa", NewLocation: "x"}},
	})
	if err == nil {
		t.Fatal("expected error on broken source")
	}
	if !strings.Contains(err.Error(), "source ITL validation failed") {
		t.Errorf("expected source-validation error, got: %v", err)
	}

	// No backup created for a broken source.
	backups, _ := filepath.Glob(itlPath + ".bak-*")
	if len(backups) != 0 {
		t.Errorf("expected 0 backups for broken source, got %d", len(backups))
	}

	// Source is untouched.
	if data, _ := os.ReadFile(itlPath); string(data) != "corrupted" {
		t.Errorf("source modified despite validation failure: %q", string(data))
	}
}

// TestSafeWriteITL_TempValidationFailure verifies that when the
// post-write temp file doesn't validate, we delete the temp and
// abort cleanly with the original intact. This catches the case
// where ApplyITLOperations succeeds byte-wise but produces
// something iTunes can't read.
func TestSafeWriteITL_TempValidationFailure(t *testing.T) {
	dir := t.TempDir()
	itlPath := makeITL(t, dir, "library.itl", "original")

	// Validate passes for the source, fails for the temp.
	validateCalls := 0
	withFakeITLHooks(t,
		func(path string) error {
			validateCalls++
			if strings.HasSuffix(path, ".tmp") {
				return errors.New("temp file malformed")
			}
			return nil
		},
		func(in, out string, ops itunes.ITLOperationSet) (*itunes.ITLWriteBackResult, error) {
			return &itunes.ITLWriteBackResult{UpdatedCount: 1}, os.WriteFile(out, []byte("garbage"), 0o644)
		},
	)

	err := SafeWriteITL(itlPath, itunes.ITLOperationSet{
		LocationUpdates: []itunes.ITLLocationUpdate{{PersistentID: "aa", NewLocation: "x"}},
	})
	if err == nil {
		t.Fatal("expected error on temp validation failure")
	}
	if !strings.Contains(err.Error(), "validation of temp ITL failed") {
		t.Errorf("expected temp-validation error, got: %v", err)
	}

	// Temp file was cleaned up.
	if _, err := os.Stat(itlPath + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp to be cleaned up after validation failure")
	}

	// Original still has pre-write content.
	if data, _ := os.ReadFile(itlPath); string(data) != "original" {
		t.Errorf("original modified despite temp validation failure: %q", string(data))
	}
}

// TestSafeWriteITL_PostRenameRestore verifies the restore path:
// temp validates, rename lands, then post-rename validation fails
// (e.g., the filesystem corrupted it during the rename). We must
// restore from the backup we took before the write.
func TestSafeWriteITL_PostRenameRestore(t *testing.T) {
	dir := t.TempDir()
	itlPath := makeITL(t, dir, "library.itl", "original-bytes")

	// Validate passes for source, passes for temp, fails for the
	// final file after rename.
	validateCount := 0
	withFakeITLHooks(t,
		func(path string) error {
			validateCount++
			// Source (call 1) + temp (call 2) pass. The third call is
			// the post-rename validation on itlPath — fail that.
			if validateCount == 3 {
				return errors.New("corrupted after rename")
			}
			return nil
		},
		func(in, out string, ops itunes.ITLOperationSet) (*itunes.ITLWriteBackResult, error) {
			// Write the new content.
			return &itunes.ITLWriteBackResult{UpdatedCount: 1}, os.WriteFile(out, []byte("new-bytes"), 0o644)
		},
	)

	err := SafeWriteITL(itlPath, itunes.ITLOperationSet{
		LocationUpdates: []itunes.ITLLocationUpdate{{PersistentID: "aa", NewLocation: "x"}},
	})
	if err == nil {
		t.Fatal("expected error on post-rename validation failure")
	}
	if !strings.Contains(err.Error(), "restored from backup") {
		t.Errorf("expected restore message in error, got: %v", err)
	}

	// The final file should contain the ORIGINAL bytes (restored
	// from backup), not the new ones (written and then rolled back).
	data, _ := os.ReadFile(itlPath)
	if string(data) != "original-bytes" {
		t.Errorf("file not restored: got %q, want %q", string(data), "original-bytes")
	}
}

// TestPruneITLBackups verifies backup rotation: given N+1 backup
// files, pruneITLBackups keeps the N newest (by lex sort on the
// timestamp suffix) and removes the oldest.
func TestPruneITLBackups(t *testing.T) {
	dir := t.TempDir()
	itlPath := filepath.Join(dir, "test.itl")
	if err := os.WriteFile(itlPath, []byte("live"), 0o644); err != nil {
		t.Fatal(err)
	}

	stamps := []string{
		"20260101-000000", "20260102-000000", "20260103-000000",
		"20260104-000000", "20260105-000000", "20260106-000000",
		"20260107-000000", "20260108-000000",
	}
	for _, s := range stamps {
		if err := os.WriteFile(itlPath+".bak-"+s, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := pruneITLBackups(itlPath, 5); err != nil {
		t.Fatalf("pruneITLBackups: %v", err)
	}

	remaining, _ := filepath.Glob(itlPath + ".bak-*")
	if len(remaining) != 5 {
		t.Fatalf("expected 5 remaining, got %d: %v", len(remaining), remaining)
	}

	survivors := map[string]bool{}
	for _, p := range remaining {
		survivors[filepath.Base(p)] = true
	}
	for _, want := range []string{
		"test.itl.bak-20260104-000000",
		"test.itl.bak-20260105-000000",
		"test.itl.bak-20260106-000000",
		"test.itl.bak-20260107-000000",
		"test.itl.bak-20260108-000000",
	} {
		if !survivors[want] {
			t.Errorf("expected survivor %s, not found", want)
		}
	}
	for _, gone := range []string{
		"test.itl.bak-20260101-000000",
		"test.itl.bak-20260102-000000",
		"test.itl.bak-20260103-000000",
	} {
		if survivors[gone] {
			t.Errorf("expected %s to be pruned, still present", gone)
		}
	}
}

// TestPruneITLBackups_KeepZero is a no-op by contract: passing
// keep=0 means "don't prune". Callers that want to delete
// everything pass a very large keep and use Remove directly.
func TestPruneITLBackups_KeepZero(t *testing.T) {
	dir := t.TempDir()
	itlPath := filepath.Join(dir, "test.itl")
	_ = os.WriteFile(itlPath+".bak-20260101-000000", []byte("x"), 0o644)
	if err := pruneITLBackups(itlPath, 0); err != nil {
		t.Fatalf("pruneITLBackups: %v", err)
	}
	if _, err := os.Stat(itlPath + ".bak-20260101-000000"); err != nil {
		t.Error("backup was removed despite keep=0")
	}
}
