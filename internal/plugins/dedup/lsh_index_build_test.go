// file: internal/plugins/dedup/lsh_index_build_test.go
// version: 1.0.0
// guid: c1cf5590-1bc1-4f88-9031-62333bcb593f
// last-edited: 2026-06-09

package dedup

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"math/rand"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/fingerprint"
	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

// synthRawLSH generates a synthetic raw chromaprint for testing.
// Uses a deterministic RNG to produce repeatable results.
func synthRawLSH(seed int64, frames int) []byte {
	rng := rand.New(rand.NewSource(seed))
	raw := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		binary.LittleEndian.PutUint32(raw[i*4:], rng.Uint32())
	}
	return raw
}

// mockLSHStore is a test double for LSHIndexStore. It tracks calls to
// PutLSHEntries and HasLSHIndex so the op-level test can assert correct
// behavior without a real PebbleDB instance.
//
// Note: mockLSHStore only needs to satisfy the LSHIndexStore interface
// (used via type assertion inside runLSHIndexBuild), not database.Store.
type mockLSHStore struct {
	files        []database.BookFile
	indexedFiles map[string]bool // HasLSHIndex returns true for these IDs
	putCalls     []string        // fileIDs passed to PutLSHEntries
	flagSet      bool
}

// database.Store compliance: use nil for the plugin.store field — the
// type assertion path in runLSHIndexBuild casts to LSHIndexStore directly.

func (m *mockLSHStore) GetAllBookFiles() ([]database.BookFile, error) {
	return m.files, nil
}
func (m *mockLSHStore) HasLSHIndex(id string) bool {
	return m.indexedFiles[id]
}
func (m *mockLSHStore) PutLSHEntries(fileID, _ string, _ []fingerprint.Subprint, _ []byte) error {
	m.putCalls = append(m.putCalls, fileID)
	return nil
}
func (m *mockLSHStore) IsLSHIndexBuilt() bool {
	return m.flagSet
}
func (m *mockLSHStore) SetLSHIndexBuilt() error {
	m.flagSet = true
	return nil
}
func (m *mockLSHStore) GetSetting(_ string) (*database.Setting, error) {
	return nil, nil
}
func (m *mockLSHStore) SetSetting(_, _, _ string, _ bool) error {
	return nil
}

// fakeReporter is a minimal sdk.Reporter that satisfies the interface.
type fakeReporter struct{}

func (f *fakeReporter) UpdateProgress(_, _ int, _ string) error { return nil }
func (f *fakeReporter) Log(_ slog.Level, _ string, _ ...slog.Attr) error {
	return nil
}
func (f *fakeReporter) Logger() *slog.Logger { return slog.Default() }
func (f *fakeReporter) Checkpoint(_ any) error {
	return nil
}
func (f *fakeReporter) IsCanceled() bool { return false }
func (f *fakeReporter) RunPhase(_ context.Context, _ string, fn func(context.Context, sdk.Reporter) error) error {
	return fn(context.Background(), f)
}
func (f *fakeReporter) Trigger(_ context.Context, _ string, _ any) error { return nil }
func (f *fakeReporter) SetCurrentItem(_ string)                           {}

// pluginWithMockStore creates a Plugin whose store satisfies LSHIndexStore
// via type assertion without needing to implement database.Store. We rely on
// the fact that p.store is a database.Store interface; for tests we can cast
// to any and let the type assertion in runLSHIndexBuild handle it.
//
// The trick: we wrap the mock in a helper type that embeds *database.MockStore
// from the mocks package and overrides only the LSHIndexStore methods — but
// that's heavyweight. Instead, because runLSHIndexBuild does:
//
//	lshStore, ok := p.store.(LSHIndexStore)
//
// …and p.store is just a database.Store interface, we can set p.store to a
// value that satisfies database.Store AND LSHIndexStore. Using mockLSHStoreAdapter
// which embeds a stub database.Store and adds the LSH methods.
type mockLSHStoreAdapter struct {
	database.Store // embed to satisfy the interface; methods we need are on the mock
	inner          *mockLSHStore
}

// Override only the LSHIndexStore methods.
func (a *mockLSHStoreAdapter) GetAllBookFiles() ([]database.BookFile, error) {
	return a.inner.GetAllBookFiles()
}
func (a *mockLSHStoreAdapter) HasLSHIndex(id string) bool {
	return a.inner.HasLSHIndex(id)
}
func (a *mockLSHStoreAdapter) PutLSHEntries(fileID, bookID string, subs []fingerprint.Subprint, bands []byte) error {
	return a.inner.PutLSHEntries(fileID, bookID, subs, bands)
}
func (a *mockLSHStoreAdapter) IsLSHIndexBuilt() bool {
	return a.inner.IsLSHIndexBuilt()
}
func (a *mockLSHStoreAdapter) SetLSHIndexBuilt() error {
	return a.inner.SetLSHIndexBuilt()
}
func (a *mockLSHStoreAdapter) GetSetting(key string) (*database.Setting, error) {
	return a.inner.GetSetting(key)
}
func (a *mockLSHStoreAdapter) SetSetting(k, v, dt string, internal bool) error {
	return a.inner.SetSetting(k, v, dt, internal)
}

// TestLSHIndexBuild_OpIndexesAllWithFingerprints verifies that the op:
//   - calls PutLSHEntries for files with fingerprints
//   - skips files without a fingerprint
//   - skips files that already have an LSH index entry (resumable)
//   - sets the completion flag on success
func TestLSHIndexBuild_OpIndexesAllWithFingerprints(t *testing.T) {
	fp := synthRawLSH(42, 57600)

	ms := &mockLSHStore{
		files: []database.BookFile{
			{ID: "file-1", BookID: "book-1", AcoustIDFingerprint: fp},
			{ID: "file-2", BookID: "book-2", AcoustIDFingerprint: fp},
			{ID: "file-3", BookID: "book-3", AcoustIDFingerprint: nil}, // no fp
			{ID: "file-4", BookID: "book-4", AcoustIDFingerprint: fp},
		},
		indexedFiles: map[string]bool{
			"file-4": true, // already indexed — should be skipped
		},
	}

	// Build a Plugin using the adapter as its store.
	// engine is nil — runLSHIndexBuild doesn't use it (no engine guard in this op).
	p := &Plugin{
		store: &mockLSHStoreAdapter{inner: ms},
	}

	err := p.runLSHIndexBuild(context.Background(), json.RawMessage("{}"), &fakeReporter{})
	if err != nil {
		t.Fatalf("runLSHIndexBuild: %v", err)
	}

	// file-1 and file-2 should be indexed; file-3 (no fp) and file-4 (already indexed) skipped.
	indexed := make(map[string]bool)
	for _, id := range ms.putCalls {
		indexed[id] = true
	}
	if !indexed["file-1"] {
		t.Errorf("file-1 not indexed")
	}
	if !indexed["file-2"] {
		t.Errorf("file-2 not indexed")
	}
	if indexed["file-3"] {
		t.Errorf("file-3 (no fingerprint) should NOT be indexed")
	}
	if indexed["file-4"] {
		t.Errorf("file-4 (already indexed) should NOT be re-indexed (skipped by HasLSHIndex)")
	}

	if !ms.flagSet {
		t.Errorf("completion flag not set after successful op run")
	}
}

// TestLSHIndexBuild_OpEmptyLibrary verifies the op exits cleanly on an
// empty store (no panic, correct "nothing to index" path).
func TestLSHIndexBuild_OpEmptyLibrary(t *testing.T) {
	ms := &mockLSHStore{
		files:        nil,
		indexedFiles: map[string]bool{},
	}
	p := &Plugin{store: &mockLSHStoreAdapter{inner: ms}}

	err := p.runLSHIndexBuild(context.Background(), json.RawMessage("{}"), &fakeReporter{})
	if err != nil {
		t.Fatalf("runLSHIndexBuild on empty store: %v", err)
	}
	if len(ms.putCalls) != 0 {
		t.Errorf("expected 0 PutLSHEntries calls, got %d", len(ms.putCalls))
	}
}

// TestLSHIndexBuild_OpNonLSHStore verifies a helpful error is returned
// when the plugin's store doesn't implement LSHIndexStore.
func TestLSHIndexBuild_OpNonLSHStore(t *testing.T) {
	// Use a nil store — p.store.(LSHIndexStore) will fail.
	p := &Plugin{store: nil}
	err := p.runLSHIndexBuild(context.Background(), json.RawMessage("{}"), &fakeReporter{})
	if err == nil {
		t.Fatal("expected error when store doesn't implement LSHIndexStore, got nil")
	}
}
