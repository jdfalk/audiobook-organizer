// file: internal/itunes/service/path_repair_resolver_test.go
// version: 1.0.0
// guid: 8aef0d23-1c84-4f3d-9b41-2d70eaf1c7c0

package itunesservice

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// fileSet is a tiny fake filesystem for resolver tests — paths in the
// set are considered to exist on disk.
type fileSet map[string]bool

func (fs fileSet) exists(p string) bool { return fs[p] }

// ---------------------------------------------------------------------------
// resolveTierA — happy path: PID → book → file-level path exists on disk
// ---------------------------------------------------------------------------

func TestResolveTierA_FileLevelPID(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	m.EXPECT().GetBookByExternalID("itunes", "PID_FOO").
		Return("book-1", nil).Once()
	m.EXPECT().GetBookFiles("book-1").
		Return([]database.BookFile{
			{ID: "f1", FilePath: "/disk/old.m4b", ITunesPersistentID: "PID_OTHER"},
			{ID: "f2", FilePath: "/disk/new.m4b", ITunesPersistentID: "PID_FOO"},
		}, nil).Once()

	fs := fileSet{"/disk/new.m4b": true}
	got, ok := resolveTierA(m, "PID_FOO", fs.exists)
	assert.True(t, ok)
	assert.Equal(t, "/disk/new.m4b", got)
}

// ---------------------------------------------------------------------------
// resolveTierA — falls back to book.FilePath when no file-level PID match
// ---------------------------------------------------------------------------

func TestResolveTierA_FallbackToBookFilePath(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	m.EXPECT().GetBookByExternalID("itunes", "PID_BAR").
		Return("book-2", nil).Once()
	m.EXPECT().GetBookFiles("book-2").
		Return([]database.BookFile{
			{ID: "f1", FilePath: "/disk/seg1.mp3", ITunesPersistentID: ""},
		}, nil).Once()
	m.EXPECT().GetBookByID("book-2").
		Return(&database.Book{ID: "book-2", FilePath: "/disk/book2-folder/book.m4b"}, nil).Once()

	fs := fileSet{"/disk/book2-folder/book.m4b": true}
	got, ok := resolveTierA(m, "PID_BAR", fs.exists)
	assert.True(t, ok)
	assert.Equal(t, "/disk/book2-folder/book.m4b", got)
}

// ---------------------------------------------------------------------------
// resolveTierA — DB has the PID but the path doesn't exist on disk → unresolved
// ---------------------------------------------------------------------------

func TestResolveTierA_DBPathAlsoMissing(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	m.EXPECT().GetBookByExternalID("itunes", "PID_BAZ").
		Return("book-3", nil).Once()
	m.EXPECT().GetBookFiles("book-3").
		Return([]database.BookFile{
			{ID: "f1", FilePath: "/disk/also-gone.m4b", ITunesPersistentID: "PID_BAZ"},
		}, nil).Once()
	m.EXPECT().GetBookByID("book-3").
		Return(&database.Book{ID: "book-3", FilePath: "/disk/also-gone-book.m4b"}, nil).Once()

	fs := fileSet{} // nothing exists
	got, ok := resolveTierA(m, "PID_BAZ", fs.exists)
	assert.False(t, ok)
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// resolveTierA — PID has no DB mapping → unresolved (cheap path)
// ---------------------------------------------------------------------------

func TestResolveTierA_NoMapping(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	m.EXPECT().GetBookByExternalID("itunes", "PID_UNKNOWN").
		Return("", nil).Once()

	got, ok := resolveTierA(m, "PID_UNKNOWN", func(string) bool { return true })
	assert.False(t, ok)
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// resolveTierA — store error treated as unresolved (degrades gracefully)
// ---------------------------------------------------------------------------

func TestResolveTierA_StoreError(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	m.EXPECT().GetBookByExternalID("itunes", mock.Anything).
		Return("", assert.AnError).Once()

	got, ok := resolveTierA(m, "PID_X", func(string) bool { return true })
	assert.False(t, ok)
	assert.Empty(t, got)
}
