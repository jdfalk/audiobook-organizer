// file: internal/scanner/unit_test.go
// version: 1.0.0
// guid: a2b3c4d5-e6f7-8901-abcd-ef2345678901

package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"errors"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// extractQuotedValue (0% -> 100%)
// ---------------------------------------------------------------------------

func TestExtractQuotedValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple quoted", `TITLE "My Book"`, "My Book"},
		{"file reference", `FILE "track01.mp3" WAVE`, "track01.mp3"},
		{"no quotes", "TITLE My Book", ""},
		{"single quote only", `TITLE "unterminated`, ""},
		{"empty quotes", `TITLE ""`, ""},
		{"nested quotes", `TITLE "first" "second"`, "first"},
		{"quotes in middle", `some "value" here`, "value"},
		{"empty input", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractQuotedValue(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// SetScanCache / ClearScanCache (0% -> 100%)
// ---------------------------------------------------------------------------

func TestSetScanCacheAndClear(t *testing.T) {
	// Ensure clean state
	ClearScanCache()

	cache := map[string]database.ScanCacheEntry{
		"/tmp/a.m4b": {Mtime: 100, Size: 200},
	}
	SetScanCache(cache)

	globalScanCacheMu.RLock()
	assert.NotNil(t, globalScanCache)
	assert.Len(t, globalScanCache, 1)
	globalScanCacheMu.RUnlock()

	ClearScanCache()

	globalScanCacheMu.RLock()
	assert.Nil(t, globalScanCache)
	globalScanCacheMu.RUnlock()
}

// ---------------------------------------------------------------------------
// shouldSkipFile — additional edge cases
// ---------------------------------------------------------------------------

func TestShouldSkipFileNeedsRescan(t *testing.T) {
	cache := map[string]database.ScanCacheEntry{
		"/a.m4b": {Mtime: 10, Size: 20, NeedsRescan: true},
		"/b.m4b": {Mtime: 10, Size: 20, NeedsRescan: false},
	}
	// NeedsRescan forces re-scan even when mtime+size match
	assert.False(t, shouldSkipFile("/a.m4b", 10, 20, cache))
	// No rescan needed and mtime+size match -> skip
	assert.True(t, shouldSkipFile("/b.m4b", 10, 20, cache))
	// Different size -> don't skip
	assert.False(t, shouldSkipFile("/b.m4b", 10, 99, cache))
	// Different mtime -> don't skip
	assert.False(t, shouldSkipFile("/b.m4b", 99, 20, cache))
}

// ---------------------------------------------------------------------------
// isInitialToken
// ---------------------------------------------------------------------------

func TestIsInitialToken(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"J.", true},
		{"K.", true},
		{"Z.", true},
		{"A.", true},
		{"a.", false},  // lowercase
		{"JK", false},  // no period
		{"J..", false}, // too long
		{".", false},   // too short
		{"J", false},   // no period
		{"AB.", false}, // too long
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, isInitialToken(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// looksLikeTitleCandidate
// ---------------------------------------------------------------------------

func TestLooksLikeTitleCandidate(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"The Great Gatsby", true},
		{"A Tale of Two Cities", true},
		{"An Example", true},
		{"  the padded  ", true},
		{"Stephen King", false},
		{"My Book", false},
		{"", false},
		{"another thing", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikeTitleCandidate(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// isValidAuthor — edge cases not covered
// ---------------------------------------------------------------------------

func TestIsValidAuthorEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"chapter space", "chapter 5", false},
		{"chapter prefix", "chapter", false},
		{"volume prefix", "volume 1", false},
		{"disc prefix", "disc 2", false},
		{"valid name", "Jane Austen", true},
		{"numeric", "42", false},
		{"book prefix", "book 1", false},
		{"part prefix", "part one", false},
		{"vol prefix", "vol 3", false},
		{"normal word", "something", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidAuthor(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// ComputeSegmentFileHash (0% -> covered)
// ---------------------------------------------------------------------------

func TestComputeSegmentFileHash(t *testing.T) {
	t.Run("small file", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "seg.m4b")
		require.NoError(t, os.WriteFile(path, []byte("segment data for hashing"), 0o644))

		hash, err := ComputeSegmentFileHash(path)
		require.NoError(t, err)
		assert.Len(t, hash, 64) // SHA-256 hex

		// Deterministic
		hash2, err := ComputeSegmentFileHash(path)
		require.NoError(t, err)
		assert.Equal(t, hash, hash2)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ComputeSegmentFileHash("/no/such/file.m4b")
		assert.Error(t, err)
	})

	t.Run("empty file", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "empty.m4b")
		require.NoError(t, os.WriteFile(path, []byte{}, 0o644))

		hash, err := ComputeSegmentFileHash(path)
		require.NoError(t, err)
		assert.Len(t, hash, 64)
	})
}

// ---------------------------------------------------------------------------
// computeHashFromReader — small-file path (19% -> higher)
// ---------------------------------------------------------------------------

func TestComputeHashFromReaderSmallFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "small.bin")
	data := []byte("small content for hashing test")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	hash, err := computeHashFromReader(f, int64(len(data)))
	require.NoError(t, err)
	assert.Len(t, hash, 64)

	// Compare with ComputeFileHash for consistency
	fHash, err := ComputeFileHash(path)
	require.NoError(t, err)
	assert.Equal(t, fHash, hash)
}

// ---------------------------------------------------------------------------
// parseCueFile (0% -> covered)
// ---------------------------------------------------------------------------

func TestParseCueFile(t *testing.T) {
	t.Run("valid cue sheet", func(t *testing.T) {
		tmp := t.TempDir()

		// Create referenced audio files
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "track01.mp3"), []byte("audio"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "track02.mp3"), []byte("audio"), 0o644))

		cueContent := `TITLE "My Audiobook"
PERFORMER "Some Author"
FILE "track01.mp3" MP3
  TRACK 01 AUDIO
    INDEX 01 00:00:00
FILE "track02.mp3" MP3
  TRACK 02 AUDIO
    INDEX 01 05:00:00
`
		cuePath := filepath.Join(tmp, "book.cue")
		require.NoError(t, os.WriteFile(cuePath, []byte(cueContent), 0o644))

		title, files := parseCueFile(cuePath)
		assert.Equal(t, "My Audiobook", title)
		assert.Len(t, files, 2)
		assert.Contains(t, files, filepath.Join(tmp, "track01.mp3"))
		assert.Contains(t, files, filepath.Join(tmp, "track02.mp3"))
	})

	t.Run("missing referenced files", func(t *testing.T) {
		tmp := t.TempDir()
		cueContent := `TITLE "Ghost Book"
FILE "missing.mp3" MP3
`
		cuePath := filepath.Join(tmp, "ghost.cue")
		require.NoError(t, os.WriteFile(cuePath, []byte(cueContent), 0o644))

		title, files := parseCueFile(cuePath)
		assert.Equal(t, "Ghost Book", title)
		assert.Empty(t, files)
	})

	t.Run("nonexistent cue file", func(t *testing.T) {
		title, files := parseCueFile("/no/such/file.cue")
		assert.Empty(t, title)
		assert.Nil(t, files)
	})

	t.Run("no title", func(t *testing.T) {
		tmp := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.mp3"), []byte("x"), 0o644))

		cueContent := `FILE "a.mp3" MP3
  TRACK 01 AUDIO
`
		cuePath := filepath.Join(tmp, "notitle.cue")
		require.NoError(t, os.WriteFile(cuePath, []byte(cueContent), 0o644))

		title, files := parseCueFile(cuePath)
		assert.Empty(t, title)
		assert.Len(t, files, 1)
	})
}

// ---------------------------------------------------------------------------
// parseM3UFile (0% -> covered)
// ---------------------------------------------------------------------------

func TestParseM3UFile(t *testing.T) {
	t.Run("valid m3u", func(t *testing.T) {
		tmp := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "track1.mp3"), []byte("a"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "track2.mp3"), []byte("b"), 0o644))

		m3uContent := "#EXTM3U\n#EXTINF:300,Track 1\ntrack1.mp3\n#EXTINF:200,Track 2\ntrack2.mp3\n"
		m3uPath := filepath.Join(tmp, "playlist.m3u")
		require.NoError(t, os.WriteFile(m3uPath, []byte(m3uContent), 0o644))

		files := parseM3UFile(m3uPath)
		assert.Len(t, files, 2)
	})

	t.Run("empty lines and comments only", func(t *testing.T) {
		tmp := t.TempDir()
		m3uContent := "#EXTM3U\n# comment\n\n\n"
		m3uPath := filepath.Join(tmp, "empty.m3u")
		require.NoError(t, os.WriteFile(m3uPath, []byte(m3uContent), 0o644))

		files := parseM3UFile(m3uPath)
		assert.Empty(t, files)
	})

	t.Run("nonexistent m3u", func(t *testing.T) {
		files := parseM3UFile("/no/file.m3u")
		assert.Nil(t, files)
	})

	t.Run("missing referenced files", func(t *testing.T) {
		tmp := t.TempDir()
		m3uContent := "nonexistent.mp3\n"
		m3uPath := filepath.Join(tmp, "bad.m3u")
		require.NoError(t, os.WriteFile(m3uPath, []byte(m3uContent), 0o644))

		files := parseM3UFile(m3uPath)
		assert.Empty(t, files)
	})

	t.Run("absolute paths", func(t *testing.T) {
		tmp := t.TempDir()
		absFile := filepath.Join(tmp, "abs.mp3")
		require.NoError(t, os.WriteFile(absFile, []byte("x"), 0o644))

		m3uContent := absFile + "\n"
		m3uPath := filepath.Join(tmp, "abs.m3u")
		require.NoError(t, os.WriteFile(m3uPath, []byte(m3uContent), 0o644))

		files := parseM3UFile(m3uPath)
		assert.Len(t, files, 1)
		assert.Equal(t, absFile, files[0])
	})
}

// ---------------------------------------------------------------------------
// groupFilesIntoBooks — edge cases (56% -> higher)
// ---------------------------------------------------------------------------

func TestGroupFilesIntoBooks(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		books := groupFilesIntoBooks(nil)
		assert.Empty(t, books)
	})

	t.Run("single file", func(t *testing.T) {
		books := groupFilesIntoBooks([]string{"/tmp/only.m4b"})
		require.Len(t, books, 1)
		assert.Equal(t, "/tmp/only.m4b", books[0].FilePath)
		assert.Equal(t, ".m4b", books[0].Format)
	})

	t.Run("single file mp3", func(t *testing.T) {
		books := groupFilesIntoBooks([]string{"/tmp/song.mp3"})
		require.Len(t, books, 1)
		assert.Equal(t, ".mp3", books[0].Format)
	})
}

// ---------------------------------------------------------------------------
// findPlaylistGroupings — with CUE + M3U (24% -> higher)
// ---------------------------------------------------------------------------

func TestFindPlaylistGroupings(t *testing.T) {
	t.Run("cue groups audio files", func(t *testing.T) {
		tmp := t.TempDir()

		// Create audio files
		audioFiles := []string{
			filepath.Join(tmp, "track1.mp3"),
			filepath.Join(tmp, "track2.mp3"),
			filepath.Join(tmp, "unrelated.mp3"),
		}
		for _, f := range audioFiles {
			require.NoError(t, os.WriteFile(f, []byte("audio"), 0o644))
		}

		// Create cue file referencing first two
		cueContent := `TITLE "My Book"
FILE "track1.mp3" MP3
  TRACK 01 AUDIO
FILE "track2.mp3" MP3
  TRACK 02 AUDIO
`
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "book.cue"), []byte(cueContent), 0o644))

		groups := findPlaylistGroupings(tmp, audioFiles)
		assert.NotEmpty(t, groups)

		// Should have a group for "My Book" with 2 files
		found := false
		for _, files := range groups {
			if len(files) == 2 {
				found = true
			}
		}
		assert.True(t, found, "expected a group with 2 files")
	})

	t.Run("m3u groups audio files", func(t *testing.T) {
		tmp := t.TempDir()

		audioFiles := []string{
			filepath.Join(tmp, "a.mp3"),
			filepath.Join(tmp, "b.mp3"),
		}
		for _, f := range audioFiles {
			require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
		}

		m3uContent := "a.mp3\nb.mp3\n"
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "playlist.m3u"), []byte(m3uContent), 0o644))

		groups := findPlaylistGroupings(tmp, audioFiles)
		assert.NotEmpty(t, groups)
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		groups := findPlaylistGroupings("/no/such/dir", nil)
		assert.Nil(t, groups)
	})

	t.Run("no playlists", func(t *testing.T) {
		tmp := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "file.mp3"), []byte("x"), 0o644))

		groups := findPlaylistGroupings(tmp, []string{filepath.Join(tmp, "file.mp3")})
		assert.Empty(t, groups)
	})
}

// ---------------------------------------------------------------------------
// extractInfoFromPath — additional patterns for coverage
// ---------------------------------------------------------------------------

func TestExtractInfoFromPathAdditional(t *testing.T) {
	t.Run("plain filename no separator", func(t *testing.T) {
		book := &Book{FilePath: "/tmp/MyBook.m4b"}
		extractInfoFromPath(book)
		assert.Equal(t, "MyBook", book.Title)
	})

	t.Run("chapter suffix stripped", func(t *testing.T) {
		book := &Book{FilePath: "/tmp/Some Title-3 Chapter 5.m4b"}
		extractInfoFromPath(book)
		assert.NotEmpty(t, book.Title)
		assert.NotContains(t, book.Title, "Chapter 5")
	})

	t.Run("title with article and author via underscore", func(t *testing.T) {
		book := &Book{FilePath: "/tmp/The Great Gatsby_F. Scott Fitzgerald.m4b"}
		extractInfoFromPath(book)
		assert.NotEmpty(t, book.Title)
	})

	t.Run("both sides names underscore with article title", func(t *testing.T) {
		// Both sides look like person names, but left starts with "The"
		book := &Book{FilePath: "/tmp/The Jane Doe_John Smith.m4b"}
		extractInfoFromPath(book)
		assert.NotEmpty(t, book.Title)
	})

	t.Run("dash pattern with existing author", func(t *testing.T) {
		book := &Book{FilePath: "/tmp/Title - Author Name.m4b", Author: "Existing"}
		extractInfoFromPath(book)
		// Should use series fallback since author is already set
		assert.Equal(t, "Existing", book.Author)
	})
}

// ---------------------------------------------------------------------------
// extractAuthorFromDirectory — translator pattern
// ---------------------------------------------------------------------------

func TestExtractAuthorFromDirectoryTranslator(t *testing.T) {
	got := extractAuthorFromDirectory("/media/Leo Tolstoy - translator - Some Person/book.m4b")
	assert.Equal(t, "Leo Tolstoy", got)
}

// ---------------------------------------------------------------------------
// parseFilenameForAuthor — coverage gaps
// ---------------------------------------------------------------------------

func TestParseFilenameForAuthorNoSeparator(t *testing.T) {
	title, author := parseFilenameForAuthor("no separators here")
	assert.Empty(t, title)
	assert.Empty(t, author)
}

// ---------------------------------------------------------------------------
// ProcessBooksParallel — with progress callback
// ---------------------------------------------------------------------------

func TestProcessBooksParallelWithProgressCallback(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() { config.AppConfig.SupportedExtensions = oldExts })
	config.AppConfig.SupportedExtensions = []string{".m4b"}

	SetScanner(nil)
	t.Cleanup(func() { SetScanner(nil) })

	tmp := t.TempDir()
	books := []Book{
		{FilePath: filepath.Join(tmp, "b1.m4b"), Format: ".m4b"},
		{FilePath: filepath.Join(tmp, "b2.m4b"), Format: ".m4b"},
	}
	for _, b := range books {
		require.NoError(t, os.WriteFile(b.FilePath, []byte("content"), 0o644))
	}

	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(book *Book) error { return nil }

	var progressCalls int
	progressFn := func(processed, total int, path string) {
		progressCalls++
	}

	err := ProcessBooksParallel(t.Context(), books, 2, progressFn, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, progressCalls)
}

// ---------------------------------------------------------------------------
// Book struct field coverage — SegmentFiles in groupFilesIntoBooks
// ---------------------------------------------------------------------------

func TestGroupFilesIntoBooksNoAlbumMultipleFiles(t *testing.T) {
	// When quickReadAlbum returns "" for all files (non-audio files),
	// each file becomes its own book
	tmp := t.TempDir()
	files := make([]string, 3)
	for i := range files {
		p := filepath.Join(tmp, filepath.Base(t.Name())+string(rune('a'+i))+".m4b")
		require.NoError(t, os.WriteFile(p, []byte("not real audio"), 0o644))
		files[i] = p
	}

	books := groupFilesIntoBooks(files)
	// With no album tags, each becomes its own book (no album grouping)
	assert.GreaterOrEqual(t, len(books), 1)
}

// ---------------------------------------------------------------------------
// isUniqueConstraintError (0% -> 100%)
// ---------------------------------------------------------------------------

func TestIsUniqueConstraintError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"unique constraint", errors.New("UNIQUE constraint failed: authors.name"), true},
		{"duplicate key", errors.New("duplicate key value violates"), true},
		{"uppercase unique", errors.New("Unique Constraint violation"), true},
		{"unrelated error", errors.New("connection refused"), false},
		{"empty error", errors.New(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isUniqueConstraintError(tt.err))
		})
	}
}

// ---------------------------------------------------------------------------
// preserveExistingFields (50% -> higher)
// ---------------------------------------------------------------------------

func TestPreserveExistingFields(t *testing.T) {
	narrator := "John Narrator"
	publisher := "Big Publisher"
	lang := "en"
	year := 2020
	coverURL := "http://example.com/cover.jpg"
	isbn10 := "0123456789"
	isbn13 := "978-0123456789"
	asin := "B0123456"
	edition := "1st"
	desc := "A great book"
	olID := "OL123"
	hcID := "HC456"
	gbID := "GB789"
	itunesPID := "ABCD1234"
	versionNotes := "Original version"
	seqNum := 3

	existing := &database.Book{
		Narrator:            &narrator,
		Publisher:           &publisher,
		Language:            &lang,
		PrintYear:           &year,
		CoverURL:            &coverURL,
		ISBN10:              &isbn10,
		ISBN13:              &isbn13,
		ASIN:                &asin,
		Edition:             &edition,
		Description:         &desc,
		OpenLibraryID:       &olID,
		HardcoverID:         &hcID,
		GoogleBooksID:       &gbID,
		ITunesPersistentID:  &itunesPID,
		VersionNotes:        &versionNotes,
		SeriesSequence:      &seqNum,
	}

	scanned := &database.Book{} // all nil

	preserveExistingFields(scanned, existing)

	assert.Equal(t, &narrator, scanned.Narrator)
	assert.Equal(t, &publisher, scanned.Publisher)
	assert.Equal(t, &lang, scanned.Language)
	assert.Equal(t, &year, scanned.PrintYear)
	assert.Equal(t, &coverURL, scanned.CoverURL)
	assert.Equal(t, &isbn10, scanned.ISBN10)
	assert.Equal(t, &isbn13, scanned.ISBN13)
	assert.Equal(t, &asin, scanned.ASIN)
	assert.Equal(t, &edition, scanned.Edition)
	assert.Equal(t, &desc, scanned.Description)
	assert.Equal(t, &olID, scanned.OpenLibraryID)
	assert.Equal(t, &hcID, scanned.HardcoverID)
	assert.Equal(t, &gbID, scanned.GoogleBooksID)
	assert.Equal(t, &itunesPID, scanned.ITunesPersistentID)
	assert.Equal(t, &versionNotes, scanned.VersionNotes)
	assert.Equal(t, &seqNum, scanned.SeriesSequence)
}

func TestPreserveExistingFieldsDoesNotOverwrite(t *testing.T) {
	existingNarr := "Old Narrator"
	scannedNarr := "New Narrator"

	existing := &database.Book{Narrator: &existingNarr}
	scanned := &database.Book{Narrator: &scannedNarr}

	preserveExistingFields(scanned, existing)
	assert.Equal(t, &scannedNarr, scanned.Narrator, "scanned value should not be overwritten")
}

func TestPreserveExistingFieldsZeroSequence(t *testing.T) {
	zero := 0
	existingSeq := 5
	scanned := &database.Book{SeriesSequence: &zero}
	existing := &database.Book{SeriesSequence: &existingSeq}

	preserveExistingFields(scanned, existing)
	assert.Equal(t, &existingSeq, scanned.SeriesSequence, "zero sequence should be replaced by existing")
}

// ---------------------------------------------------------------------------
// countAudioFilesInDir (0% -> 100%)
// ---------------------------------------------------------------------------

func TestCountAudioFilesInDir(t *testing.T) {
	t.Run("mixed files", func(t *testing.T) {
		tmp := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.m4b"), []byte("x"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "b.mp3"), []byte("x"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "c.txt"), []byte("x"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, "subdir"), 0o755))

		count := countAudioFilesInDir(tmp, []string{".m4b", ".mp3"})
		assert.Equal(t, 2, count)
	})

	t.Run("no audio files", func(t *testing.T) {
		tmp := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "readme.txt"), []byte("x"), 0o644))

		count := countAudioFilesInDir(tmp, []string{".m4b"})
		assert.Equal(t, 0, count)
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		count := countAudioFilesInDir("/no/such/dir", []string{".m4b"})
		assert.Equal(t, 0, count)
	})

	t.Run("case insensitive extensions", func(t *testing.T) {
		tmp := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "book.M4B"), []byte("x"), 0o644))

		count := countAudioFilesInDir(tmp, []string{".m4b"})
		assert.Equal(t, 1, count)
	})

	t.Run("empty dir", func(t *testing.T) {
		tmp := t.TempDir()
		count := countAudioFilesInDir(tmp, []string{".m4b", ".mp3"})
		assert.Equal(t, 0, count)
	})
}

// ---------------------------------------------------------------------------
// computeHashFromReader — large file path (19% -> higher)
// ---------------------------------------------------------------------------

func TestComputeHashFromReaderLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large file hash test in short mode")
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "large.bin")

	// Create a file just over 100MB threshold
	f, err := os.Create(path)
	require.NoError(t, err)

	const targetSize = 101 * 1024 * 1024 // 101 MB
	chunk := make([]byte, 1024*1024)      // 1 MB chunks
	for i := 0; i < 101; i++ {
		for j := range chunk {
			chunk[j] = byte(i ^ j)
		}
		_, err := f.Write(chunk)
		require.NoError(t, err)
	}
	f.Close()

	// Now test computeHashFromReader
	f2, err := os.Open(path)
	require.NoError(t, err)
	defer f2.Close()

	hash, err := computeHashFromReader(f2, targetSize)
	require.NoError(t, err)
	assert.Len(t, hash, 64)

	// Verify it matches ComputeFileHash
	cfHash, err := ComputeFileHash(path)
	require.NoError(t, err)
	assert.Equal(t, cfHash, hash)
}

// ---------------------------------------------------------------------------
// createBookFilesForBook — nil store early return (0% -> partial)
// ---------------------------------------------------------------------------

func TestCreateBookFilesForBookNilStore(t *testing.T) {
	origStore := database.GetGlobalStore()
	database.SetGlobalStore(nil)
	t.Cleanup(func() { database.SetGlobalStore(origStore) })

	// Should return immediately without panic
	createBookFilesForBook("/tmp/test.m4b", nil, defaultLog)
}

// ---------------------------------------------------------------------------
// ProcessFile — empty path error
// ---------------------------------------------------------------------------

func TestProcessFileEmptyPath(t *testing.T) {
	_, _, _, err := ProcessFile("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty file path")
}

func TestProcessFileNonexistent(t *testing.T) {
	_, _, _, err := ProcessFile("/no/such/file.m4b")
	assert.Error(t, err)
}

func TestProcessFileDirectory(t *testing.T) {
	tmp := t.TempDir()
	meta, mi, hash, err := ProcessFile(tmp)
	require.NoError(t, err)
	assert.NotNil(t, meta)
	assert.Nil(t, mi, "directories should have no mediainfo")
	assert.Empty(t, hash, "directories should have no hash")
}

// ---------------------------------------------------------------------------
// quickReadAlbum — non-audio file returns empty
// ---------------------------------------------------------------------------

func TestQuickReadAlbumNonAudio(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "notaudio.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	album := quickReadAlbum(path)
	assert.Empty(t, album)
}

func TestQuickReadAlbumNonexistent(t *testing.T) {
	album := quickReadAlbum("/no/such/file.m4b")
	assert.Empty(t, album)
}

// ---------------------------------------------------------------------------
// ProcessBooksParallel — incremental cache skip path
// ---------------------------------------------------------------------------

func TestProcessBooksParallelWithScanCache(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() { config.AppConfig.SupportedExtensions = oldExts })
	config.AppConfig.SupportedExtensions = []string{".m4b"}

	SetScanner(nil)
	t.Cleanup(func() { SetScanner(nil) })

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "cached.m4b")
	require.NoError(t, os.WriteFile(filePath, []byte("cached content"), 0o644))

	fi, err := os.Stat(filePath)
	require.NoError(t, err)

	// Pre-populate cache so the file is skipped
	cache := map[string]database.ScanCacheEntry{
		filePath: {Mtime: fi.ModTime().Unix(), Size: fi.Size(), NeedsRescan: false},
	}
	SetScanCache(cache)
	t.Cleanup(func() { ClearScanCache() })

	books := []Book{{FilePath: filePath, Format: ".m4b"}}

	saveCalled := false
	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(book *Book) error {
		saveCalled = true
		return nil
	}

	err = ProcessBooksParallel(t.Context(), books, 1, nil, nil)
	assert.NoError(t, err)
	assert.False(t, saveCalled, "save should not be called for cached file")
}

// ---------------------------------------------------------------------------
// preserveExistingFields — iTunes and version fields
// ---------------------------------------------------------------------------

func TestPreserveExistingFieldsITunesAndVersion(t *testing.T) {
	dateAdded := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	playCount := 5
	lastPlayed := time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC)
	rating := 80
	bookmark := int64(5025)
	importSrc := "iTunes XML"
	isPrimary := true
	vgID := "vg-123"
	markedDel := true
	markedDelAt := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	abYear := 2019
	workID := "w-42"
	narrJSON := `["narrator1","narrator2"]`

	existing := &database.Book{
		ITunesDateAdded:      &dateAdded,
		ITunesPlayCount:      &playCount,
		ITunesLastPlayed:     &lastPlayed,
		ITunesRating:         &rating,
		ITunesBookmark:       &bookmark,
		ITunesImportSource:   &importSrc,
		IsPrimaryVersion:     &isPrimary,
		VersionGroupID:       &vgID,
		MarkedForDeletion:    &markedDel,
		MarkedForDeletionAt:  &markedDelAt,
		AudiobookReleaseYear: &abYear,
		WorkID:               &workID,
		NarratorsJSON:        &narrJSON,
	}

	scanned := &database.Book{}
	preserveExistingFields(scanned, existing)

	assert.Equal(t, &dateAdded, scanned.ITunesDateAdded)
	assert.Equal(t, &playCount, scanned.ITunesPlayCount)
	assert.Equal(t, &lastPlayed, scanned.ITunesLastPlayed)
	assert.Equal(t, &rating, scanned.ITunesRating)
	assert.Equal(t, &bookmark, scanned.ITunesBookmark)
	assert.Equal(t, &importSrc, scanned.ITunesImportSource)
	assert.Equal(t, &isPrimary, scanned.IsPrimaryVersion)
	assert.Equal(t, &vgID, scanned.VersionGroupID)
	assert.Equal(t, &markedDel, scanned.MarkedForDeletion)
	assert.Equal(t, &markedDelAt, scanned.MarkedForDeletionAt)
	assert.Equal(t, &abYear, scanned.AudiobookReleaseYear)
	assert.Equal(t, &workID, scanned.WorkID)
	assert.Equal(t, &narrJSON, scanned.NarratorsJSON)
}
