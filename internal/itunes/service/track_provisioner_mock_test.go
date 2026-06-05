// file: internal/itunes/service/track_provisioner_mock_test.go
// version: 1.0.0
// guid: f1a2b3c4-d5e6-7f8a-9b0c-1d2e3f4a5b6c

package itunesservice

import (
	"errors"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	dbmocks "github.com/falkcorp/audiobook-organizer/internal/database/mocks"
	"github.com/falkcorp/audiobook-organizer/internal/itunes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Pure-function tests (no deps)
// ---------------------------------------------------------------------------

func TestLinuxToWindowsPath_KnownPrefix(t *testing.T) {
	in := "/mnt/bigdata/books/audiobook-organizer/Author/Book/track.m4b"
	got := linuxToWindowsPath(in)
	assert.Equal(t, `W:\audiobook-organizer\Author\Book\track.m4b`, got)
}

func TestLinuxToWindowsPath_UnknownPrefix(t *testing.T) {
	in := "/some/other/path/file.m4b"
	got := linuxToWindowsPath(in)
	assert.Equal(t, in, got, "unknown prefix must be returned unchanged")
}

func TestLinuxToWindowsPath_Empty(t *testing.T) {
	assert.Equal(t, "", linuxToWindowsPath(""))
}

func TestKindFromExt(t *testing.T) {
	cases := []struct {
		ext  string
		want string
	}{
		{".m4b", "AAC audio file"},
		{".m4a", "AAC audio file"},
		{".aac", "AAC audio file"},
		{".mp3", "MPEG audio file"},
		{".ogg", "Ogg Vorbis file"},
		{".flac", "FLAC audio file"},
		{".wav", "WAV audio file"},
		{".wma", "AAC audio file"}, // unknown → default
		{"", "AAC audio file"},     // empty → default
	}
	for _, tc := range cases {
		t.Run(tc.ext, func(t *testing.T) {
			assert.Equal(t, tc.want, kindFromExt(tc.ext))
		})
	}
}

// ---------------------------------------------------------------------------
// mockEnqueuer captures calls for assertions.
// ---------------------------------------------------------------------------

type mockEnqueuer struct {
	adds     []itunes.ITLNewTrack
	removes  []string
	enqueues []string
}

func (m *mockEnqueuer) Enqueue(bookID string)           { m.enqueues = append(m.enqueues, bookID) }
func (m *mockEnqueuer) EnqueueAdd(t itunes.ITLNewTrack) { m.adds = append(m.adds, t) }
func (m *mockEnqueuer) EnqueueRemove(pid string)        { m.removes = append(m.removes, pid) }

// ---------------------------------------------------------------------------
// TrackProvisioner constructor / SetEnqueuer
// ---------------------------------------------------------------------------

func TestNewTrackProvisioner_NilEnqueuer(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	p := newTrackProvisioner(m, nil, Config{})
	require.NotNil(t, p)
	assert.Nil(t, p.enqueuer)
}

func TestSetEnqueuer(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	p := newTrackProvisioner(m, nil, Config{})
	enq := &mockEnqueuer{}
	p.SetEnqueuer(enq)
	assert.Equal(t, enq, p.enqueuer)
}

// ---------------------------------------------------------------------------
// Provision — skips when AutoWriteBack is false
// ---------------------------------------------------------------------------

func TestProvision_SkipsWhenAutoWriteBackDisabled(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	p := newTrackProvisioner(m, nil, Config{AutoWriteBack: false})

	book := &database.Book{ID: "b1"}
	file := &database.BookFile{ID: "f1", BookID: "b1"}

	err := p.Provision(book, file)
	require.NoError(t, err)
	assert.Equal(t, "", file.ITunesPersistentID, "PID must not be set when disabled")
}

// ---------------------------------------------------------------------------
// Provision — skips when file already has a PID
// ---------------------------------------------------------------------------

func TestProvision_SkipsAlreadyHasPID(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	p := newTrackProvisioner(m, nil, Config{AutoWriteBack: true})

	book := &database.Book{ID: "b1"}
	file := &database.BookFile{ID: "f1", ITunesPersistentID: "EXISTINGPID"}

	err := p.Provision(book, file)
	require.NoError(t, err)
	// Mock would fail on any unexpected call — verifies no store access
}

// TestProvision_SkipsNonPrimary asserts non-primary book versions never
// get a PID generated or an iTunes EnqueueAdd. Past behavior (no filter)
// was the source of large amounts of duplicate iTunes tracks.
func TestProvision_SkipsNonPrimary(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}
	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})

	notPrimary := false
	book := &database.Book{ID: "b1", IsPrimaryVersion: &notPrimary}
	file := &database.BookFile{ID: "f1"}

	err := p.Provision(book, file)
	require.NoError(t, err)
	assert.Empty(t, enq.adds, "non-primary book must not enqueue ITL add")
	// Mock would fail on any unexpected store call — verifies no PID side-effects.
}

// ---------------------------------------------------------------------------
// Provision — happy path
// ---------------------------------------------------------------------------

func TestProvision_HappyPath(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	authorID := 42
	book := &database.Book{
		ID:       "book-happy",
		Title:    "Great Audiobook",
		AuthorID: &authorID,
	}
	file := &database.BookFile{
		ID:           "file-happy",
		BookID:       "book-happy",
		FilePath:     "/mnt/bigdata/books/audiobook-organizer/Author/Book/track.m4b",
		Title:        "Chapter 1",
		TrackNumber:  1,
		FileSize:     12345678,
		Duration:     3600,
		BitrateKbps:  128,
		SampleRateHz: 44100,
		DiscNumber:   1,
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()
	m.EXPECT().GetAuthorByID(42).Return(&database.Author{ID: 42, Name: "J.K. Rowling"}, nil).Once()

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	err := p.Provision(book, file)

	require.NoError(t, err)
	assert.NotEmpty(t, file.ITunesPersistentID, "PID must be set after Provision")
	assert.Equal(t, `W:\audiobook-organizer\Author\Book\track.m4b`, file.ITunesPath)

	require.Len(t, enq.adds, 1)
	added := enq.adds[0]
	assert.Equal(t, file.ITunesPath, added.Location)
	assert.Equal(t, "Chapter 1", added.Name)
	assert.Equal(t, "Great Audiobook", added.Album)
	assert.Equal(t, "J.K. Rowling", added.Artist)
	assert.Equal(t, "Audiobook", added.Genre)
	assert.Equal(t, "AAC audio file", added.Kind)
	assert.Equal(t, 3600*1000, added.TotalTime, "duration must be converted to ms")
}

// ---------------------------------------------------------------------------
// Provision — nil enqueuer (no panic)
// ---------------------------------------------------------------------------

func TestProvision_NilEnqueuerNoEnqueue(t *testing.T) {
	m := dbmocks.NewMockStore(t)

	authorID := 1
	book := &database.Book{ID: "b2", Title: "Book", AuthorID: &authorID}
	file := &database.BookFile{
		ID:       "f2",
		BookID:   "b2",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/f.m4b",
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()
	// bookAuthor is only called inside the enqueuer block — nil enqueuer skips it

	p := newTrackProvisioner(m, nil, Config{AutoWriteBack: true})
	err := p.Provision(book, file)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Provision — CreateExternalIDMapping error propagates
// ---------------------------------------------------------------------------

func TestProvision_CreateMappingError(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	book := &database.Book{ID: "b3", Title: "Book"}
	file := &database.BookFile{
		ID:       "f3",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/x.m4b",
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(errors.New("db error")).Once()

	p := newTrackProvisioner(m, nil, Config{AutoWriteBack: true})
	err := p.Provision(book, file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

// ---------------------------------------------------------------------------
// ProvisionAll — provisions new files, skips files that already have PIDs
// ---------------------------------------------------------------------------

func TestProvisionAll_HappyPath(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{ID: "bookA", Title: "Book A"}
	files := []database.BookFile{
		{ID: "fA1", BookID: "bookA", FilePath: "/mnt/bigdata/books/audiobook-organizer/a.m4b"},
		// fA2 already has a PID — Provision must skip it
		{ID: "fA2", BookID: "bookA", FilePath: "/mnt/bigdata/books/audiobook-organizer/b.mp3", ITunesPersistentID: "EXISTING"},
	}

	m.EXPECT().GetBookFiles("bookA").Return(files, nil).Once()
	// Only fA1 triggers store calls
	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	err := p.ProvisionAll(book)
	require.NoError(t, err)

	assert.Len(t, enq.adds, 1, "only one new track should be enqueued")
}

// ---------------------------------------------------------------------------
// ProvisionAll — GetBookFiles error propagates
// ---------------------------------------------------------------------------

func TestProvisionAll_GetBookFilesError(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	book := &database.Book{ID: "bErr"}

	m.EXPECT().GetBookFiles("bErr").Return(nil, errors.New("db down")).Once()

	p := newTrackProvisioner(m, nil, Config{AutoWriteBack: true})
	err := p.ProvisionAll(book)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// bookAuthor — nil AuthorID returns empty string
// ---------------------------------------------------------------------------

func TestBookAuthor_NilAuthorID(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	p := newTrackProvisioner(m, nil, Config{})
	book := &database.Book{ID: "b", AuthorID: nil}
	assert.Equal(t, "", p.bookAuthor(book))
}

// ---------------------------------------------------------------------------
// bookAuthor — store error returns empty string (not a fatal error)
// ---------------------------------------------------------------------------

func TestBookAuthor_StoreError(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	id := 99
	m.EXPECT().GetAuthorByID(99).Return(nil, errors.New("not found")).Once()

	p := newTrackProvisioner(m, nil, Config{})
	book := &database.Book{ID: "b", AuthorID: &id}
	assert.Equal(t, "", p.bookAuthor(book))
}
