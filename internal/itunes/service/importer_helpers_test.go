// file: internal/itunes/service/importer_helpers_test.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package itunesservice

import (
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/itunes"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// calculatePercent
// ---------------------------------------------------------------------------

func TestCalculatePercent_Normal(t *testing.T) {
	assert.Equal(t, 50, calculatePercent(50, 100))
	assert.Equal(t, 0, calculatePercent(0, 100))
	assert.Equal(t, 100, calculatePercent(100, 100))
}

func TestCalculatePercent_ZeroTotal(t *testing.T) {
	assert.Equal(t, 0, calculatePercent(10, 0))
	assert.Equal(t, 0, calculatePercent(10, -1))
}

func TestCalculatePercent_Clamped(t *testing.T) {
	assert.Equal(t, 100, calculatePercent(200, 100))
}

// ---------------------------------------------------------------------------
// min
// ---------------------------------------------------------------------------

func TestMinHelper(t *testing.T) {
	assert.Equal(t, 3, min(3, 5))
	assert.Equal(t, 3, min(5, 3))
	assert.Equal(t, 3, min(3, 3))
}

// ---------------------------------------------------------------------------
// commonParentDir — method on Importer, computes filesystem path from tracks
// ---------------------------------------------------------------------------

func TestCommonParentDir_Empty(t *testing.T) {
	imp := &Importer{}
	opts := itunes.ImportOptions{}
	result := imp.commonParentDir(nil, opts)
	assert.Equal(t, "", result)
}

func TestCommonParentDir_SingleTrack(t *testing.T) {
	imp := &Importer{}
	opts := itunes.ImportOptions{}
	tracks := []*itunes.Track{
		{Location: "file:///mnt/books/Author/Book/track.m4b"},
	}
	result := imp.commonParentDir(tracks, opts)
	assert.Equal(t, "/mnt/books/Author/Book", result)
}

func TestCommonParentDir_SameDir(t *testing.T) {
	imp := &Importer{}
	opts := itunes.ImportOptions{}
	tracks := []*itunes.Track{
		{Location: "file:///mnt/books/Author/Book/track1.m4b"},
		{Location: "file:///mnt/books/Author/Book/track2.m4b"},
	}
	result := imp.commonParentDir(tracks, opts)
	assert.Equal(t, "/mnt/books/Author/Book", result)
}

func TestCommonParentDir_DifferentDirs(t *testing.T) {
	imp := &Importer{}
	opts := itunes.ImportOptions{}
	tracks := []*itunes.Track{
		{Location: "file:///mnt/books/Author/BookA/track1.m4b"},
		{Location: "file:///mnt/books/Author/BookB/track2.m4b"},
	}
	result := imp.commonParentDir(tracks, opts)
	assert.Equal(t, "/mnt/books/Author", result)
}

// ---------------------------------------------------------------------------
// incImportLinked — not covered by existing importer_test.go
// ---------------------------------------------------------------------------

func TestIncImportLinked(t *testing.T) {
	s := &itunesImportStatus{}
	incImportLinked(s)
	incImportLinked(s)
	assert.Equal(t, 2, s.Linked)
}
