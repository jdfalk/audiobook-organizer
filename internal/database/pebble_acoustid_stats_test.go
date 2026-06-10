// file: internal/database/pebble_acoustid_stats_test.go
// version: 1.1.0
// guid: e5f6a7b8-c9d0-1234-efab-234567890123
// last-edited: 2026-06-10

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

	// File with fingerprint (T020: use AcoustIDFingerprint, not seg fields).
	f1 := &BookFile{BookID: books[0].ID, FilePath: "/lib/audiobooks/book1.m4b", AcoustIDFingerprint: []byte{1, 2, 3, 4}}
	require.NoError(t, store.CreateBookFile(f1))

	// File without fingerprint
	f2 := &BookFile{BookID: books[1].ID, FilePath: "/lib/audiobooks/book2.m4b"}
	require.NoError(t, store.CreateBookFile(f2))

	ps := store.(*PebbleStore)
	stats, err := ps.GetAcoustIDStats()
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalFiles)
	assert.Equal(t, 1, stats.WithFingerprint, "only one file has a fingerprint")
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

	// File with only a whole-file fingerprint (T020: seg fields are no longer stored).
	f := &BookFile{BookID: created.ID, FilePath: "/lib/seg6.m4b", AcoustIDFingerprint: []byte{0xAB, 0xCD, 0xEF, 0x01}}
	require.NoError(t, store.CreateBookFile(f))

	ps := store.(*PebbleStore)
	stats, err := ps.GetAcoustIDStats()
	require.NoError(t, err)
	assert.Equal(t, 1, stats.WithFingerprint, "AcoustIDFingerprint should count as having a fingerprint")
}
