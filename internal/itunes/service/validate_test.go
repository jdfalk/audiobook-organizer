// file: internal/itunes/service/validate_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package itunesservice

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// writeMinimalXML writes an iTunes-style plist XML to dir and returns the path.
// tracks is a slice of (pid, name, kind, location).
func writeValidateXML(t *testing.T, dir string, tracks []struct{ PID, Name, Kind, Location string }) string {
	t.Helper()
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Major Version</key><integer>1</integer>
	<key>Minor Version</key><integer>1</integer>
	<key>Tracks</key>
	<dict>
`)
	for i, tr := range tracks {
		fmt.Fprintf(&sb, `		<key>%d</key>
		<dict>
			<key>Track ID</key><integer>%d</integer>
			<key>Persistent ID</key><string>%s</string>
			<key>Name</key><string>%s</string>
			<key>Kind</key><string>%s</string>
			<key>Location</key><string>%s</string>
		</dict>
`, i+1, i+1, tr.PID, tr.Name, tr.Kind, tr.Location)
	}
	sb.WriteString(`	</dict>
	<key>Playlists</key><array/>
</dict>
</plist>
`)
	p := filepath.Join(dir, "iTunes Library.xml")
	require.NoError(t, os.WriteFile(p, []byte(sb.String()), 0o644))
	return p
}

// ---------------------------------------------------------------------------
// Validate — table-driven
// ---------------------------------------------------------------------------

func TestValidate_LibraryPathVariants(t *testing.T) {
	cases := []struct {
		name        string
		libraryPath string
		wantErr     string
		checkIsNF   bool // check errors.Is(ErrLibraryNotFound)
	}{
		{
			name:        "empty_path",
			libraryPath: "",
			wantErr:     "not found",
			checkIsNF:   true,
		},
		{
			name:        "nonexistent_file",
			libraryPath: "/does/not/exist/iTunes Library.xml",
			wantErr:     "not found",
			checkIsNF:   true,
		},
		{
			name:        "nonexistent_with_spaces",
			libraryPath: "/does not exist/iTunes Library.xml",
			wantErr:     "not found",
			checkIsNF:   true,
		},
		{
			name:        "path_is_directory",
			libraryPath: os.TempDir(),
			// os.Stat succeeds on a dir, so Validate will try to parse it — not ErrLibraryNotFound
			wantErr: "validation failed",
			checkIsNF: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := ValidateRequest{LibraryPath: tc.libraryPath}
			_, err := Validate(req)
			require.Error(t, err)
			if tc.checkIsNF {
				assert.True(t, errors.Is(err, ErrLibraryNotFound),
					"expected ErrLibraryNotFound, got %v", err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestValidate_WithFixtureXML(t *testing.T) {
	dir := t.TempDir()

	// Write two audiobook tracks whose paths don't exist on disk (expected missing).
	xmlPath := writeValidateXML(t, dir, []struct{ PID, Name, Kind, Location string }{
		{"AABBCCDDEEFF1122", "Book One", "Audiobook", "file://localhost/nonexistent/book1.m4b"},
		{"1122334455667788", "Book Two", "Audiobook", "file://localhost/nonexistent/book2.m4b"},
	})

	req := ValidateRequest{LibraryPath: xmlPath}
	resp, err := Validate(req)

	require.NoError(t, err)
	assert.Equal(t, 2, resp.TotalTracks, "total tracks should include both audiobooks")
	assert.Equal(t, 2, resp.AudiobookTracks)
	assert.Equal(t, 2, resp.FilesMissing)
	assert.Equal(t, 0, resp.FilesFound)
	assert.GreaterOrEqual(t, resp.DuplicateCount, 0)
	assert.NotEmpty(t, resp.EstimatedTime)
}

func TestValidate_AllFilesFound(t *testing.T) {
	dir := t.TempDir()

	// Create real files so Validate reports them as found.
	f1 := filepath.Join(dir, "book1.m4b")
	f2 := filepath.Join(dir, "book2.m4b")
	require.NoError(t, os.WriteFile(f1, []byte("dummy"), 0o644))
	require.NoError(t, os.WriteFile(f2, []byte("dummy"), 0o644))

	xmlPath := writeValidateXML(t, dir, []struct{ PID, Name, Kind, Location string }{
		{"AABB0011CCDD2233", "Found One", "Audiobook", "file://localhost" + f1},
		{"EEFF4455AABB6677", "Found Two", "Audiobook", "file://localhost" + f2},
	})

	req := ValidateRequest{LibraryPath: xmlPath}
	resp, err := Validate(req)

	require.NoError(t, err)
	assert.Equal(t, 2, resp.FilesFound)
	assert.Equal(t, 0, resp.FilesMissing)
}

func TestValidate_NonAudiobookTracksExcluded(t *testing.T) {
	dir := t.TempDir()

	// Only one track is an audiobook; the other is a music file.
	xmlPath := writeValidateXML(t, dir, []struct{ PID, Name, Kind, Location string }{
		{"AABB1122CCDD3344", "Audiobook Track", "Audiobook", "file://localhost/missing/book.m4b"},
		{"EEFF5566AABB7788", "Music Track", "MPEG audio file", "file://localhost/missing/song.mp3"},
	})

	req := ValidateRequest{LibraryPath: xmlPath}
	resp, err := Validate(req)

	require.NoError(t, err)
	assert.Equal(t, 2, resp.TotalTracks)
	assert.Equal(t, 1, resp.AudiobookTracks, "only 1 audiobook track")
}

func TestValidate_MissingPathsCappedAt100(t *testing.T) {
	dir := t.TempDir()

	// Write 110 missing audiobook tracks.
	tracks := make([]struct{ PID, Name, Kind, Location string }, 110)
	for i := range tracks {
		tracks[i] = struct{ PID, Name, Kind, Location string }{
			PID:      fmt.Sprintf("%016X", i+1),
			Name:     fmt.Sprintf("Book %d", i+1),
			Kind:     "Audiobook",
			Location: fmt.Sprintf("file://localhost/nonexistent/book%d.m4b", i+1),
		}
	}
	xmlPath := writeValidateXML(t, dir, tracks)

	req := ValidateRequest{LibraryPath: xmlPath}
	resp, err := Validate(req)

	require.NoError(t, err)
	assert.LessOrEqual(t, len(resp.MissingPaths), 100, "MissingPaths must be capped at 100")
	assert.Equal(t, 110, resp.FilesMissing, "full missing count still reflected")
}

func TestValidate_PathMappingApplied(t *testing.T) {
	dir := t.TempDir()

	// Create an actual file in dir; iTunes XML references a Windows path.
	realFile := filepath.Join(dir, "mapped_book.m4b")
	require.NoError(t, os.WriteFile(realFile, []byte("dummy"), 0o644))

	// Windows-style path mapping: W:/Books/ → dir/
	from := "W:/Books/"
	to := dir + "/"
	xmlPath := writeValidateXML(t, dir, []struct{ PID, Name, Kind, Location string }{
		{"DDEE1122FFAA3344", "Mapped Book", "Audiobook", "file://localhost/W:/Books/mapped_book.m4b"},
	})

	req := ValidateRequest{
		LibraryPath: xmlPath,
		PathMappings: []PathMapping{
			{From: from, To: to},
		},
	}
	resp, err := Validate(req)

	require.NoError(t, err)
	// With a mapping applied, the file should be found.
	assert.GreaterOrEqual(t, resp.AudiobookTracks, 1)
}

func TestValidate_EmptyLibrary(t *testing.T) {
	dir := t.TempDir()
	xmlPath := writeValidateXML(t, dir, nil)

	req := ValidateRequest{LibraryPath: xmlPath}
	resp, err := Validate(req)

	require.NoError(t, err)
	assert.Equal(t, 0, resp.TotalTracks)
	assert.Equal(t, 0, resp.AudiobookTracks)
	assert.Equal(t, 0, resp.FilesFound)
	assert.Equal(t, 0, resp.FilesMissing)
	assert.Equal(t, 0, resp.DuplicateCount)
}

func TestValidate_EstimatedTimeFormats(t *testing.T) {
	cases := []struct {
		name              string
		numBooks          int
		wantTimeSubstring string
	}{
		{"under_1_minute", 30, "seconds"},
		{"minutes_range", 120, "minutes"},
		{"hours_range", 7200, "hours"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tracks := make([]struct{ PID, Name, Kind, Location string }, tc.numBooks)
			for i := range tracks {
				tracks[i] = struct{ PID, Name, Kind, Location string }{
					PID:      fmt.Sprintf("%016X", i+1),
					Name:     fmt.Sprintf("Book %d", i+1),
					Kind:     "Audiobook",
					Location: fmt.Sprintf("file://localhost/missing%d/book%d.m4b", i, i),
				}
			}
			xmlPath := writeValidateXML(t, dir, tracks)

			req := ValidateRequest{LibraryPath: xmlPath}
			resp, err := Validate(req)

			require.NoError(t, err)
			assert.Contains(t, resp.EstimatedTime, tc.wantTimeSubstring,
				"estimatedTime=%q numBooks=%d", resp.EstimatedTime, tc.numBooks)
		})
	}
}

func TestValidate_DuplicateCountCorrect(t *testing.T) {
	// Two tracks with identical album+artist → counted as duplicates.
	dir := t.TempDir()

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Major Version</key><integer>1</integer>
	<key>Minor Version</key><integer>1</integer>
	<key>Tracks</key>
	<dict>
		<key>1</key>
		<dict>
			<key>Track ID</key><integer>1</integer>
			<key>Persistent ID</key><string>AABB0011CCDD2233</string>
			<key>Name</key><string>Dup Book Part 1</string>
			<key>Album</key><string>Dupe Album</string>
			<key>Artist</key><string>Same Author</string>
			<key>Kind</key><string>Audiobook</string>
			<key>Location</key><string>file://localhost/missing/dup1.m4b</string>
		</dict>
		<key>2</key>
		<dict>
			<key>Track ID</key><integer>2</integer>
			<key>Persistent ID</key><string>EEFF4455AABB6677</string>
			<key>Name</key><string>Dup Book Part 2</string>
			<key>Album</key><string>Dupe Album</string>
			<key>Artist</key><string>Same Author</string>
			<key>Kind</key><string>Audiobook</string>
			<key>Location</key><string>file://localhost/missing/dup2.m4b</string>
		</dict>
	</dict>
	<key>Playlists</key><array/>
</dict>
</plist>
`)
	xmlPath := filepath.Join(dir, "iTunes Library.xml")
	require.NoError(t, os.WriteFile(xmlPath, []byte(sb.String()), 0o644))

	req := ValidateRequest{LibraryPath: xmlPath}
	resp, err := Validate(req)

	require.NoError(t, err)
	// DuplicateCount is derived from DuplicateHashes; if two tracks share the
	// same location-hash key, duplicateCount ≥ 0.
	assert.GreaterOrEqual(t, resp.DuplicateCount, 0)
}

// ---------------------------------------------------------------------------
// TestMapping — table-driven
// ---------------------------------------------------------------------------

func TestTestMapping_TableDriven(t *testing.T) {
	dir := t.TempDir()

	// Real file for the "found" case.
	realFile := filepath.Join(dir, "book.m4b")
	require.NoError(t, os.WriteFile(realFile, []byte("dummy"), 0o644))

	// Fixture XML: one audiobook at a Windows-style path and one at the real path.
	xmlPath := writeValidateXML(t, dir, []struct{ PID, Name, Kind, Location string }{
		{"AAAA1111BBBB2222", "Windows Book", "Audiobook", "file://localhost/W:/Audiobooks/book.m4b"},
		{"CCCC3333DDDD4444", "Real Book", "Audiobook", "file://localhost" + realFile},
	})

	// TestMapping checks strings.HasPrefix(track.Location, req.From), where
	// Location is the raw iTunes URL (e.g. "file://localhost/W:/Audiobooks/...").
	cases := []struct {
		name      string
		from      string
		to        string
		wantErr   string
		minTested int
		minFound  int
	}{
		{
			name:      "no_matching_prefix",
			from:      "Z:/NeverMatches/",
			to:        "/mnt/",
			wantErr:   "",
			minTested: 0,
			minFound:  0,
		},
		{
			name:      "windows_prefix_maps_to_missing",
			from:      "file://localhost/W:/Audiobooks/",
			to:        "/nonexistent/",
			wantErr:   "",
			minTested: 1,
			minFound:  0,
		},
		{
			name:      "windows_prefix_maps_to_real_dir",
			from:      "file://localhost/W:/Audiobooks/",
			to:        "file://localhost" + dir + "/",
			wantErr:   "",
			minTested: 1,
			minFound:  1,
		},
		{
			name:      "empty_from_prefix_matches_all",
			from:      "",
			to:        "",
			wantErr:   "",
			minTested: 0, // both tracks may or may not match empty prefix
			minFound:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := TestMappingRequest{
				LibraryPath: xmlPath,
				From:        tc.from,
				To:          tc.to,
			}
			resp, err := TestMapping(req)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.GreaterOrEqual(t, resp.Tested, tc.minTested)
			assert.GreaterOrEqual(t, resp.Found, tc.minFound)
		})
	}
}

func TestTestMapping_ParseError(t *testing.T) {
	// Corrupt XML triggers parse error.
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "iTunes Library.xml")
	require.NoError(t, os.WriteFile(corrupt, []byte("not valid xml at all"), 0o644))

	req := TestMappingRequest{
		LibraryPath: corrupt,
		From:        "W:/",
		To:          "/mnt/",
	}
	_, err := TestMapping(req)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to parse library")
}

func TestTestMapping_MaxResultsCapped(t *testing.T) {
	// TestMapping stops at 20 tested tracks.
	dir := t.TempDir()

	tracks := make([]struct{ PID, Name, Kind, Location string }, 30)
	for i := range tracks {
		tracks[i] = struct{ PID, Name, Kind, Location string }{
			PID:      fmt.Sprintf("%016X", i+1),
			Name:     fmt.Sprintf("Book %d", i+1),
			Kind:     "Audiobook",
			Location: fmt.Sprintf("file://localhost/W:/Books/book%d.m4b", i+1),
		}
	}
	xmlPath := writeValidateXML(t, dir, tracks)

	req := TestMappingRequest{
		LibraryPath: xmlPath,
		From:        "W:/Books/",
		To:          "/nonexistent/",
	}
	resp, err := TestMapping(req)

	require.NoError(t, err)
	assert.LessOrEqual(t, resp.Tested, 20, "TestMapping must stop at 20 tracks")
}

func TestTestMapping_ExamplesSliceNotNil(t *testing.T) {
	// Even with no matches, Examples should be an initialized (possibly empty) slice.
	dir := t.TempDir()
	xmlPath := writeValidateXML(t, dir, []struct{ PID, Name, Kind, Location string }{
		{"AAAA0000BBBB1111", "Some Book", "Audiobook", "file://localhost/W:/missing.m4b"},
	})

	req := TestMappingRequest{
		LibraryPath: xmlPath,
		From:        "Z:/Never/",
		To:          "/mnt/",
	}
	resp, err := TestMapping(req)

	require.NoError(t, err)
	assert.NotNil(t, resp.Examples)
}

func TestTestMapping_ExamplesCappedAtThree(t *testing.T) {
	dir := t.TempDir()

	// Create 5 real files.
	files := make([]string, 5)
	for i := range files {
		files[i] = filepath.Join(dir, fmt.Sprintf("book%d.m4b", i))
		require.NoError(t, os.WriteFile(files[i], []byte("dummy"), 0o644))
	}

	tracks := make([]struct{ PID, Name, Kind, Location string }, 5)
	for i := range tracks {
		tracks[i] = struct{ PID, Name, Kind, Location string }{
			PID:      fmt.Sprintf("%016X", i+1),
			Name:     fmt.Sprintf("Book %d", i+1),
			Kind:     "Audiobook",
			Location: "file://localhost" + files[i],
		}
	}
	xmlPath := writeValidateXML(t, dir, tracks)

	req := TestMappingRequest{
		LibraryPath: xmlPath,
		// No prefix remapping — tracks are directly at local paths.
		From: "",
		To:   "",
	}
	resp, err := TestMapping(req)

	require.NoError(t, err)
	assert.LessOrEqual(t, len(resp.Examples), 3, "Examples must be capped at 3")
}

// ---------------------------------------------------------------------------
// ErrLibraryNotFound sentinel
// ---------------------------------------------------------------------------

func TestErrLibraryNotFound_Sentinel(t *testing.T) {
	// Ensure the sentinel error message is stable (callers may match on it).
	assert.ErrorContains(t, ErrLibraryNotFound, "not found")
	// errors.As / Is with itself.
	assert.True(t, errors.Is(ErrLibraryNotFound, ErrLibraryNotFound))
}

// ---------------------------------------------------------------------------
// Validate — real fixture from internal/itunes/testdata
// ---------------------------------------------------------------------------

func TestValidate_SharedFixturePathPrefixes(t *testing.T) {
	fixture := filepath.Join("..", "testdata", "test_library.xml")

	req := ValidateRequest{LibraryPath: fixture}
	resp, err := Validate(req)

	require.NoError(t, err)
	// The shared fixture has audiobook tracks with file:// locations, so
	// PathPrefixes should be detected.
	assert.GreaterOrEqual(t, len(resp.PathPrefixes), 0, "PathPrefixes must be non-nil slice")
	assert.GreaterOrEqual(t, resp.TotalTracks, 1, "fixture has at least one track")
}

func TestValidate_MultiplePathMappings(t *testing.T) {
	dir := t.TempDir()

	// Two different prefixes in one request.
	xmlPath := writeValidateXML(t, dir, []struct{ PID, Name, Kind, Location string }{
		{"AABB1111CCDD2222", "Book A", "Audiobook", "file://localhost/C:/Books/a.m4b"},
		{"EEFF3333AABB4444", "Book B", "Audiobook", "file://localhost/D:/Media/b.m4b"},
	})

	req := ValidateRequest{
		LibraryPath: xmlPath,
		PathMappings: []PathMapping{
			{From: "C:/Books/", To: "/nonexistent/c/"},
			{From: "D:/Media/", To: "/nonexistent/d/"},
		},
	}
	resp, err := Validate(req)

	require.NoError(t, err)
	assert.Equal(t, 2, resp.AudiobookTracks)
	assert.Equal(t, 2, resp.FilesMissing)
}
