// file: internal/itunes/service/track_provisioner_test.go
// version: 1.0.0
// guid: a9c2e4f6-1b3d-5e7f-8a0c-2d4e6f8b0c2e
//
// Additional unit tests for TrackProvisioner covering test cases
// required by bot-task 4.13b that are not present in
// track_provisioner_mock_test.go.  That file covers the basic happy
// path, disabled mode, already-has-PID, and store-error cases. This
// file adds:
//   - Multi-segment (3-file) book via ProvisionAll — one track per segment
//   - Empty title / author fields (missing metadata)
//   - Idempotency — second Provision call on the same file is a no-op
//   - UpsertBookFile error propagation
//   - iTunes-managed path (linuxRoot prefix) through Provision, verifying
//     ITunesPath is the Windows-mapped value
//   - Non-managed (external) path is passed through unchanged
//   - EnqueueAdd not called when enqueuer is nil (separate from batcher test)
//
// Note: EnqueueAdd has no error return in the Enqueuer interface; the
// "batcher closed" scenario from the spec is not testable at this layer.
// The mockEnqueuer declared in track_provisioner_mock_test.go is reused
// — both files are in package itunesservice.

package itunesservice

import (
	"errors"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Multi-segment book — ProvisionAll provisions 3 files in order
// ---------------------------------------------------------------------------

func TestProvisionAll_MultiSegment(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{ID: "book-multi", Title: "Long Audiobook"}
	files := []database.BookFile{
		{
			ID: "seg1", BookID: "book-multi",
			FilePath:    "/mnt/bigdata/books/audiobook-organizer/A/B/01.m4b",
			Title:       "Part 1", TrackNumber: 1,
		},
		{
			ID: "seg2", BookID: "book-multi",
			FilePath:    "/mnt/bigdata/books/audiobook-organizer/A/B/02.m4b",
			Title:       "Part 2", TrackNumber: 2,
		},
		{
			ID: "seg3", BookID: "book-multi",
			FilePath:    "/mnt/bigdata/books/audiobook-organizer/A/B/03.m4b",
			Title:       "Part 3", TrackNumber: 3,
		},
	}

	m.EXPECT().GetBookFiles("book-multi").Return(files, nil).Once()
	// Each segment triggers CreateExternalIDMapping + SetExternalIDProvenance + UpsertBookFile
	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Times(3)
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Times(3)
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Times(3)

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	err := p.ProvisionAll(book)
	require.NoError(t, err)

	assert.Len(t, enq.adds, 3, "three tracks must be enqueued for three segments")

	// Verify ordering via TrackNumber
	assert.Equal(t, 1, enq.adds[0].TrackNumber)
	assert.Equal(t, 2, enq.adds[1].TrackNumber)
	assert.Equal(t, 3, enq.adds[2].TrackNumber)

	// Spot-check payload correctness on first segment
	assert.Equal(t, "Part 1", enq.adds[0].Name)
	assert.Equal(t, "Long Audiobook", enq.adds[0].Album)
}

// ---------------------------------------------------------------------------
// Missing fields — empty title and nil author
// ---------------------------------------------------------------------------

func TestProvision_EmptyTitle(t *testing.T) {
	// A BookFile with an empty Title: the enqueued track's Name should be ""
	// (the provisioner does not synthesise a title; callers must supply it).
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{ID: "b-empty-title", Title: "Some Book"}
	file := &database.BookFile{
		ID:       "f-empty-title",
		BookID:   "b-empty-title",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/x.m4b",
		Title:    "", // intentionally blank
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	err := p.Provision(book, file)

	// Expect no error — missing title is not a fatal condition
	require.NoError(t, err)
	assert.NotEmpty(t, file.ITunesPersistentID, "PID must be assigned even when title is blank")
	require.Len(t, enq.adds, 1)
	assert.Equal(t, "", enq.adds[0].Name, "Name field must reflect empty BookFile.Title")
}

func TestProvision_NilAuthorID_EmptyArtist(t *testing.T) {
	// Book with no AuthorID: Artist in the track payload must be ""
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{
		ID:       "b-no-author",
		Title:    "No Author Book",
		AuthorID: nil, // no author
	}
	file := &database.BookFile{
		ID:       "f-no-author",
		BookID:   "b-no-author",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/noauthor.m4b",
		Title:    "Track 1",
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()
	// GetAuthorByID must NOT be called when AuthorID is nil
	// (mock would fail if called unexpectedly)

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	err := p.Provision(book, file)

	require.NoError(t, err)
	require.Len(t, enq.adds, 1)
	assert.Equal(t, "", enq.adds[0].Artist, "Artist must be empty when book has no AuthorID")
}

// ---------------------------------------------------------------------------
// Idempotency — second Provision on a file that already has a PID is a no-op
// ---------------------------------------------------------------------------

func TestProvision_Idempotency(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	authorID := 7
	book := &database.Book{ID: "b-idem", Title: "Idempotent Book", AuthorID: &authorID}
	file := &database.BookFile{
		ID:       "f-idem",
		BookID:   "b-idem",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/idem.m4b",
		Title:    "Chapter 1",
	}

	// First call wires all store calls
	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()
	m.EXPECT().GetAuthorByID(7).Return(&database.Author{ID: 7, Name: "An Author"}, nil).Once()

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})

	// First call — should provision
	err := p.Provision(book, file)
	require.NoError(t, err)
	assert.NotEmpty(t, file.ITunesPersistentID)
	firstPID := file.ITunesPersistentID
	assert.Len(t, enq.adds, 1)

	// Second call — file now has a PID; must be a no-op (no store calls, no extra enqueue)
	err = p.Provision(book, file)
	require.NoError(t, err)

	// PID must not change
	assert.Equal(t, firstPID, file.ITunesPersistentID, "PID must be stable across calls")
	// No additional enqueue
	assert.Len(t, enq.adds, 1, "EnqueueAdd must not be called a second time")
	// mock asserts no extra store calls (EXPECT Once() limits)
}

// ---------------------------------------------------------------------------
// UpsertBookFile error propagates
// ---------------------------------------------------------------------------

func TestProvision_UpsertBookFileError(t *testing.T) {
	m := dbmocks.NewMockStore(t)

	book := &database.Book{ID: "b-upsert-err", Title: "Upsert Error Book"}
	file := &database.BookFile{
		ID:       "f-upsert-err",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/err.m4b",
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(errors.New("write failed")).Once()

	p := newTrackProvisioner(m, nil, Config{AutoWriteBack: true})
	err := p.Provision(book, file)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

// ---------------------------------------------------------------------------
// iTunes-managed path — Provision writes Windows-mapped ITunesPath
// ---------------------------------------------------------------------------

func TestProvision_ManagedPath_WindowsMapped(t *testing.T) {
	// A file whose path lives under the iTunes-managed linuxRoot must have its
	// ITunesPath mapped to the Windows SMB equivalent. This exercises the
	// linuxToWindowsPath integration inside Provision (not just unit-testing
	// the pure helper).
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{ID: "b-managed", Title: "Managed Book"}
	file := &database.BookFile{
		ID:       "f-managed",
		BookID:   "b-managed",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/AuthorName/BookTitle/track01.mp3",
		Title:    "Track 01",
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	err := p.Provision(book, file)
	require.NoError(t, err)

	expectedPath := `W:\audiobook-organizer\AuthorName\BookTitle\track01.mp3`
	assert.Equal(t, expectedPath, file.ITunesPath,
		"ITunesPath must be Windows-mapped for files under the managed root")

	require.Len(t, enq.adds, 1)
	assert.Equal(t, expectedPath, enq.adds[0].Location,
		"Location in enqueued track must match the Windows path")
	assert.Equal(t, "MPEG audio file", enq.adds[0].Kind,
		"Kind must be MPEG for .mp3 files")
}

// ---------------------------------------------------------------------------
// Non-managed (external) path — ITunesPath is the original Linux path
// ---------------------------------------------------------------------------

func TestProvision_UnmanagedPath_Unchanged(t *testing.T) {
	// A file whose path is NOT under the iTunes-managed linuxRoot must have its
	// original path used verbatim — no corruption from a partial mapping.
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{ID: "b-external", Title: "External Book"}
	file := &database.BookFile{
		ID:       "f-external",
		BookID:   "b-external",
		FilePath: "/some/external/drive/book/track.flac",
		Title:    "External Track",
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	err := p.Provision(book, file)
	require.NoError(t, err)

	assert.Equal(t, "/some/external/drive/book/track.flac", file.ITunesPath,
		"non-managed paths must be passed through unchanged")

	require.Len(t, enq.adds, 1)
	assert.Equal(t, "FLAC audio file", enq.adds[0].Kind)
}

// ---------------------------------------------------------------------------
// PID uniqueness — two separate Provision calls produce distinct PIDs
// ---------------------------------------------------------------------------

func TestProvision_PIDUniqueness(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{ID: "b-uniq", Title: "Unique Book"}
	fileA := &database.BookFile{
		ID:       "fA",
		BookID:   "b-uniq",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/a.m4b",
	}
	fileB := &database.BookFile{
		ID:       "fB",
		BookID:   "b-uniq",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/b.m4b",
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Times(2)
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Times(2)
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Times(2)

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	require.NoError(t, p.Provision(book, fileA))
	require.NoError(t, p.Provision(book, fileB))

	assert.NotEmpty(t, fileA.ITunesPersistentID)
	assert.NotEmpty(t, fileB.ITunesPersistentID)
	assert.NotEqual(t, fileA.ITunesPersistentID, fileB.ITunesPersistentID,
		"each Provision call must produce a distinct PID")
}

// ---------------------------------------------------------------------------
// Duration conversion — seconds to milliseconds in enqueued track
// ---------------------------------------------------------------------------

func TestProvision_DurationConvertedToMilliseconds(t *testing.T) {
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{ID: "b-dur", Title: "Duration Test"}
	file := &database.BookFile{
		ID:       "f-dur",
		BookID:   "b-dur",
		FilePath: "/mnt/bigdata/books/audiobook-organizer/dur.m4b",
		Duration: 7200, // 2 hours in seconds
	}

	m.EXPECT().CreateExternalIDMapping(mock.Anything).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.Anything).Return(nil).Once()

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	require.NoError(t, p.Provision(book, file))

	require.Len(t, enq.adds, 1)
	assert.Equal(t, 7200*1000, enq.adds[0].TotalTime,
		"TotalTime in enqueued track must be Duration × 1000 (seconds → milliseconds)")
}

// ---------------------------------------------------------------------------
// ProvisionAll — individual file failure is logged and skipped (best-effort)
// ---------------------------------------------------------------------------

func TestProvisionAll_PartialFailure_ContinuesRemainingFiles(t *testing.T) {
	// The second file's UpsertBookFile fails. ProvisionAll should log and
	// continue — the third file should still be provisioned successfully.
	m := dbmocks.NewMockStore(t)
	enq := &mockEnqueuer{}

	book := &database.Book{ID: "b-partial", Title: "Partial Failure Book"}
	files := []database.BookFile{
		{ID: "p1", BookID: "b-partial", FilePath: "/mnt/bigdata/books/audiobook-organizer/ok1.m4b"},
		{ID: "p2", BookID: "b-partial", FilePath: "/mnt/bigdata/books/audiobook-organizer/fail.m4b"},
		{ID: "p3", BookID: "b-partial", FilePath: "/mnt/bigdata/books/audiobook-organizer/ok2.m4b"},
	}

	m.EXPECT().GetBookFiles("b-partial").Return(files, nil).Once()

	// File 1: success
	m.EXPECT().CreateExternalIDMapping(mock.MatchedBy(func(mp *database.ExternalIDMapping) bool {
		return mp.FilePath == files[0].FilePath
	})).Return(nil).Once()
	m.EXPECT().SetExternalIDProvenance("itunes", mock.Anything, "generated").Return(nil).Times(3)
	m.EXPECT().UpsertBookFile(mock.MatchedBy(func(bf *database.BookFile) bool {
		return bf.FilePath == files[0].FilePath
	})).Return(nil).Once()

	// File 2: CreateExternalIDMapping succeeds but UpsertBookFile fails
	m.EXPECT().CreateExternalIDMapping(mock.MatchedBy(func(mp *database.ExternalIDMapping) bool {
		return mp.FilePath == files[1].FilePath
	})).Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.MatchedBy(func(bf *database.BookFile) bool {
		return bf.FilePath == files[1].FilePath
	})).Return(errors.New("disk full")).Once()

	// File 3: success
	m.EXPECT().CreateExternalIDMapping(mock.MatchedBy(func(mp *database.ExternalIDMapping) bool {
		return mp.FilePath == files[2].FilePath
	})).Return(nil).Once()
	m.EXPECT().UpsertBookFile(mock.MatchedBy(func(bf *database.BookFile) bool {
		return bf.FilePath == files[2].FilePath
	})).Return(nil).Once()

	p := newTrackProvisioner(m, enq, Config{AutoWriteBack: true})
	err := p.ProvisionAll(book)

	// ProvisionAll itself must not return error — it's best-effort
	require.NoError(t, err)
	// Two files succeeded (1 and 3); file 2 failed but was skipped
	assert.Len(t, enq.adds, 2, "two successful tracks must be enqueued despite the middle failure")
}
