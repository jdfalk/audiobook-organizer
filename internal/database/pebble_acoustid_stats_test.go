// file: internal/database/pebble_acoustid_stats_test.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1234-efab-234567890123
// last-edited: 2026-05-16

package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAcoustIDStats_Empty(t *testing.T) {
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()
	ps := store.(*PebbleStore)

	stats, err := ps.GetAcoustIDStats()
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, 0, stats.TotalFiles)
	assert.Equal(t, 0, stats.WithFingerprint)
	assert.Empty(t, stats.ByLibrary)
}

func TestGetAcoustIDStats_Mixed(t *testing.T) {
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	importPath := "/lib/audiobooks"
	src := "audible"
	asin1, asin2 := "B001", "B002"

	books := []Book{
		{Title: "Book With Files", MetadataSource: &src, ASIN: &asin1, SourceImportPath: &importPath},
		{Title: "Book No FP", MetadataSource: &src, ASIN: &asin2, SourceImportPath: &importPath},
	}
	for i := range books {
		created, err := store.CreateBook(&books[i])
		require.NoError(t, err)
		books[i].ID = created.ID
	}

	// File with fingerprint
	f1 := &BookFile{BookID: books[0].ID, FilePath: "/lib/audiobooks/book1.m4b", AcoustIDSeg0: "seg0abc"}
	require.NoError(t, store.CreateBookFile(f1))

	// File without fingerprint
	f2 := &BookFile{BookID: books[1].ID, FilePath: "/lib/audiobooks/book2.m4b"}
	require.NoError(t, store.CreateBookFile(f2))

	ps := store.(*PebbleStore)
	stats, err := ps.GetAcoustIDStats()
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalFiles)
	assert.Equal(t, 1, stats.WithFingerprint, "only one file has a fingerprint segment")
	assert.Len(t, stats.ByLibrary, 1, "both files belong to same library root")
	assert.Equal(t, "/lib/audiobooks", stats.ByLibrary[0].LibraryRoot)
	assert.Equal(t, 2, stats.ByLibrary[0].TotalFiles)
	assert.Equal(t, 1, stats.ByLibrary[0].WithFingerprint)
}

func TestGetAcoustIDStats_AllSegmentsChecked(t *testing.T) {
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	importPath := "/lib"
	src := "audible"
	asin := "B003"
	book := Book{Title: "Seg Test", MetadataSource: &src, ASIN: &asin, SourceImportPath: &importPath}
	created, err := store.CreateBook(&book)
	require.NoError(t, err)

	// File where only seg6 is populated (not seg0)
	f := &BookFile{BookID: created.ID, FilePath: "/lib/seg6.m4b", AcoustIDSeg6: "last-seg-only"}
	require.NoError(t, store.CreateBookFile(f))

	ps := store.(*PebbleStore)
	stats, err := ps.GetAcoustIDStats()
	require.NoError(t, err)
	assert.Equal(t, 1, stats.WithFingerprint, "AcoustIDSeg6 alone should count as having a fingerprint")
}
