// file: internal/itunes/generate_test_itls.go
// version: 1.0.0
// guid: e0f1a2b3-4c5d-6e7f-8a9b-0c1d2e3f4a5b

package itunes

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// GenerateTestITLSuite creates a suite of synthetic ITL test cases under
// outputDir. Each test case lives in a numbered subfolder containing an
// "iTunes Library.itl" and a "test-info.json" describing the test.
//
// The books and bookFiles slices supply real data for the full-library test
// case. If they are nil/empty the full-library test is skipped.
func GenerateTestITLSuite(
	outputDir string,
	books []database.Book,
	bookFiles []database.BookFile,
) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// Linux -> Windows path mapping
	const linuxRoot = "/mnt/bigdata/books/audiobook-organizer/"
	const windowsRoot = `W:\audiobook-organizer\`

	linuxToWindows := func(p string) string {
		if strings.HasPrefix(p, linuxRoot) {
			return windowsRoot + strings.ReplaceAll(
				strings.TrimPrefix(p, linuxRoot), "/", `\`,
			)
		}
		return p
	}

	// Build book-file lookup by book ID
	filesByBook := make(map[string][]database.BookFile)
	for _, bf := range bookFiles {
		filesByBook[bf.BookID] = append(filesByBook[bf.BookID], bf)
	}

	generators := []struct {
		name string
		fn   func(dir string) error
	}{
		{"01-blank", func(dir string) error {
			return genBlank(dir)
		}},
		{"02-single-track", func(dir string) error {
			return genSingleTrack(dir, linuxToWindows)
		}},
		{"03-ten-tracks", func(dir string) error {
			return genMultiTrack(dir, 10, linuxToWindows)
		}},
		{"04-hundred-tracks", func(dir string) error {
			return genMultiTrack(dir, 100, linuxToWindows)
		}},
		{"05-full-library", func(dir string) error {
			return genFullLibrary(dir, books, bookFiles, linuxToWindows)
		}},
		{"06-updated-locations", func(dir string) error {
			return genUpdatedLocations(dir, books, bookFiles, linuxToWindows)
		}},
		{"07-mixed-sources", func(dir string) error {
			return genMixedSources(dir, books, bookFiles, linuxToWindows)
		}},
		{"08-unicode-paths", func(dir string) error {
			return genUnicodePaths(dir, linuxToWindows)
		}},
		{"09-missing-files", func(dir string) error {
			return genMissingFiles(dir, linuxToWindows)
		}},
		{"10-duplicate-pids", func(dir string) error {
			return genDuplicatePIDs(dir, linuxToWindows)
		}},
	}

	for _, g := range generators {
		dir := filepath.Join(outputDir, g.name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", g.name, err)
		}
		if err := g.fn(dir); err != nil {
			return fmt.Errorf("generating %s: %w", g.name, err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Test-info JSON written alongside each .itl
// ---------------------------------------------------------------------------

type testInfo struct {
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	ExpectedTrackCount int             `json:"expected_track_count"`
	ExpectMissingFiles bool            `json:"expect_missing_files,omitempty"`
	AllowMissingFiles  bool            `json:"allow_missing_files,omitempty"`
	Tracks             []testInfoTrack `json:"tracks,omitempty"`
	GeneratedAt        string          `json:"generated_at"`
}

type testInfoTrack struct {
	PersistentID string `json:"persistent_id"`
	Location     string `json:"location"`
	Name         string `json:"name"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
}

func writeTestInfo(dir string, info testInfo) error {
	info.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "test-info.json"), data, 0644)
}

// ---------------------------------------------------------------------------
// Synthetic ITL builder (exported, non-test version of the test helper)
// ---------------------------------------------------------------------------

// synTrack holds parameters for a single synthetic track.
type synTrack struct {
	pid      [8]byte
	location string
	name     string
	album    string
	artist   string
	genre    string
	kind     string
}

// buildSyntheticITLFromTracks creates a complete ITL binary from a list of
// synthetic tracks. The resulting file uses BE (pre-v10) format, version
// 12.0.0, uncompressed. This matches the existing test-helper format used
// in itl_test.go.
func buildSyntheticITLFromTracks(tracks []synTrack) []byte {
	version := "12.0.0"

	var payload bytes.Buffer
	for i, tr := range tracks {
		trackID := i + 1

		// htim: 156-byte track header
		htimLen := 156
		htim := make([]byte, htimLen)
		copy(htim[0:4], "htim")
		writeUint32BE(htim, 4, uint32(htimLen))
		writeUint32BE(htim, 8, uint32(htimLen))
		writeUint32BE(htim, 16, uint32(trackID))
		copy(htim[128:136], tr.pid[:])
		payload.Write(htim)

		// hohm 0x02: Name
		if tr.name != "" {
			payload.Write(buildHohmChunk(0x02, tr.name))
		}
		// hohm 0x03: Album
		if tr.album != "" {
			payload.Write(buildHohmChunk(0x03, tr.album))
		}
		// hohm 0x04: Artist
		if tr.artist != "" {
			payload.Write(buildHohmChunk(0x04, tr.artist))
		}
		// hohm 0x05: Genre
		if tr.genre != "" {
			payload.Write(buildHohmChunk(0x05, tr.genre))
		}
		// hohm 0x06: Kind
		if tr.kind != "" {
			payload.Write(buildHohmChunk(0x06, tr.kind))
		}
		// hohm 0x0D: Location
		if tr.location != "" {
			payload.Write(buildHohmChunk(0x0D, tr.location))
		}
	}

	payloadBytes := payload.Bytes()
	encrypted := itlEncrypt(&hdfmHeader{version: version}, payloadBytes)

	fileLen := uint32(len(encrypted)) + 17 + uint32(len(version))
	hdr := buildHdfmHeader(version, nil, fileLen, 0)

	var file bytes.Buffer
	file.Write(hdr)
	file.Write(encrypted)
	return file.Bytes()
}

// writeITLToDir writes itlData as "iTunes Library.itl" in dir.
func writeITLToDir(dir string, itlData []byte) error {
	return os.WriteFile(filepath.Join(dir, "iTunes Library.itl"), itlData, 0644)
}

// randomPID returns a cryptographically random 8-byte persistent ID.
func randomPID() [8]byte {
	var pid [8]byte
	_, _ = rand.Read(pid[:])
	return pid
}

// pidHex returns the hex string of a PID.
func pidHex(pid [8]byte) string {
	return hex.EncodeToString(pid[:])
}

// ---------------------------------------------------------------------------
// Individual test case generators
// ---------------------------------------------------------------------------

// 01-blank: Empty library (0 tracks)
func genBlank(dir string) error {
	itlData := buildSyntheticITLFromTracks(nil)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}
	return writeTestInfo(dir, testInfo{
		Name:               "01-blank",
		Description:        "Empty iTunes library with zero tracks",
		ExpectedTrackCount: 0,
	})
}

// 02-single-track: 1 track pointing to a file
func genSingleTrack(dir string, toWin func(string) string) error {
	pid := randomPID()
	loc := toWin("/mnt/bigdata/books/audiobook-organizer/Test Author/Test Book/test.m4b")

	tracks := []synTrack{{
		pid:      pid,
		location: loc,
		name:     "Test Book - Chapter 1",
		album:    "Test Book",
		artist:   "Test Author",
		genre:    "Audiobook",
		kind:     "AAC audio file",
	}}

	itlData := buildSyntheticITLFromTracks(tracks)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}

	return writeTestInfo(dir, testInfo{
		Name:               "02-single-track",
		Description:        "Single track pointing to a test M4B file",
		ExpectedTrackCount: 1,
		AllowMissingFiles:  true,
		Tracks: []testInfoTrack{{
			PersistentID: pidHex(pid),
			Location:     loc,
			Name:         "Test Book - Chapter 1",
			Artist:       "Test Author",
			Album:        "Test Book",
		}},
	})
}

// 03-ten-tracks / 04-hundred-tracks: N tracks with various formats
func genMultiTrack(dir string, count int, toWin func(string) string) error {
	formats := []struct {
		ext  string
		kind string
	}{
		{".m4b", "AAC audio file"},
		{".m4a", "AAC audio file"},
		{".mp3", "MPEG audio file"},
		{".m4b", "AAC audio file"},
		{".ogg", "Ogg Vorbis file"},
	}

	tracks := make([]synTrack, count)
	infoTracks := make([]testInfoTrack, count)

	for i := 0; i < count; i++ {
		pid := randomPID()
		f := formats[i%len(formats)]
		author := fmt.Sprintf("Author %03d", i+1)
		title := fmt.Sprintf("Book %03d", i+1)
		loc := toWin(fmt.Sprintf("/mnt/bigdata/books/audiobook-organizer/%s/%s/chapter%s", author, title, f.ext))

		tracks[i] = synTrack{
			pid:      pid,
			location: loc,
			name:     fmt.Sprintf("%s - Chapter 1", title),
			album:    title,
			artist:   author,
			genre:    "Audiobook",
			kind:     f.kind,
		}

		infoTracks[i] = testInfoTrack{
			PersistentID: pidHex(pid),
			Location:     loc,
			Name:         tracks[i].name,
			Artist:       author,
			Album:        title,
		}
	}

	itlData := buildSyntheticITLFromTracks(tracks)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}

	name := fmt.Sprintf("%02d-tracks", count)
	if count == 10 {
		name = "03-ten-tracks"
	} else if count == 100 {
		name = "04-hundred-tracks"
	}

	return writeTestInfo(dir, testInfo{
		Name:               name,
		Description:        fmt.Sprintf("%d tracks with various audio formats", count),
		ExpectedTrackCount: count,
		AllowMissingFiles:  true,
		Tracks:             infoTracks,
	})
}

// 05-full-library: All books from the database with real PIDs and paths
func genFullLibrary(
	dir string,
	books []database.Book,
	bookFiles []database.BookFile,
	toWin func(string) string,
) error {
	if len(bookFiles) == 0 {
		// No data — write empty ITL with a note
		itlData := buildSyntheticITLFromTracks(nil)
		if err := writeITLToDir(dir, itlData); err != nil {
			return err
		}
		return writeTestInfo(dir, testInfo{
			Name:               "05-full-library",
			Description:        "Full library test (skipped: no book files provided)",
			ExpectedTrackCount: 0,
		})
	}

	// Build a lookup for book titles by ID
	bookTitles := make(map[string]string)
	for _, b := range books {
		bookTitles[b.ID] = b.Title
	}

	var tracks []synTrack
	var infoTracks []testInfoTrack

	for _, bf := range bookFiles {
		if bf.ITunesPersistentID == "" {
			continue
		}

		pid, err := hexToPID(bf.ITunesPersistentID)
		if err != nil {
			continue
		}

		loc := toWin(bf.FilePath)
		bookTitle := bookTitles[bf.BookID]

		tracks = append(tracks, synTrack{
			pid:      pid,
			location: loc,
			name:     bf.Title,
			album:    bookTitle,
			genre:    "Audiobook",
			kind:     kindFromFormat(bf.Format),
		})

		infoTracks = append(infoTracks, testInfoTrack{
			PersistentID: bf.ITunesPersistentID,
			Location:     loc,
			Name:         bf.Title,
			Album:        bookTitle,
		})
	}

	itlData := buildSyntheticITLFromTracks(tracks)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}

	return writeTestInfo(dir, testInfo{
		Name:               "05-full-library",
		Description:        fmt.Sprintf("Full library: %d tracks from %d books", len(tracks), len(books)),
		ExpectedTrackCount: len(tracks),
		AllowMissingFiles:  true,
		Tracks:             infoTracks,
	})
}

// 06-updated-locations: All tracks rewritten to audiobook-organizer paths
func genUpdatedLocations(
	dir string,
	books []database.Book,
	bookFiles []database.BookFile,
	toWin func(string) string,
) error {
	if len(bookFiles) == 0 {
		itlData := buildSyntheticITLFromTracks(nil)
		if err := writeITLToDir(dir, itlData); err != nil {
			return err
		}
		return writeTestInfo(dir, testInfo{
			Name:               "06-updated-locations",
			Description:        "Updated locations test (skipped: no book files provided)",
			ExpectedTrackCount: 0,
		})
	}

	bookTitles := make(map[string]string)
	for _, b := range books {
		bookTitles[b.ID] = b.Title
	}

	var tracks []synTrack
	var infoTracks []testInfoTrack

	for _, bf := range bookFiles {
		if bf.FilePath == "" {
			continue
		}

		var pid [8]byte
		if bf.ITunesPersistentID != "" {
			p, err := hexToPID(bf.ITunesPersistentID)
			if err == nil {
				pid = p
			} else {
				pid = randomPID()
			}
		} else {
			pid = randomPID()
		}

		// Use the organized file path (AO path), not the iTunes path
		loc := toWin(bf.FilePath)
		bookTitle := bookTitles[bf.BookID]

		tracks = append(tracks, synTrack{
			pid:      pid,
			location: loc,
			name:     bf.Title,
			album:    bookTitle,
			genre:    "Audiobook",
			kind:     kindFromFormat(bf.Format),
		})

		infoTracks = append(infoTracks, testInfoTrack{
			PersistentID: pidHex(pid),
			Location:     loc,
			Name:         bf.Title,
			Album:        bookTitle,
		})
	}

	itlData := buildSyntheticITLFromTracks(tracks)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}

	return writeTestInfo(dir, testInfo{
		Name:               "06-updated-locations",
		Description:        fmt.Sprintf("All %d tracks with audiobook-organizer paths", len(tracks)),
		ExpectedTrackCount: len(tracks),
		AllowMissingFiles:  true,
		Tracks:             infoTracks,
	})
}

// 07-mixed-sources: Some tracks with iTunes paths, some with AO paths
func genMixedSources(
	dir string,
	books []database.Book,
	bookFiles []database.BookFile,
	toWin func(string) string,
) error {
	if len(bookFiles) == 0 {
		itlData := buildSyntheticITLFromTracks(nil)
		if err := writeITLToDir(dir, itlData); err != nil {
			return err
		}
		return writeTestInfo(dir, testInfo{
			Name:               "07-mixed-sources",
			Description:        "Mixed sources test (skipped: no book files provided)",
			ExpectedTrackCount: 0,
		})
	}

	bookTitles := make(map[string]string)
	for _, b := range books {
		bookTitles[b.ID] = b.Title
	}

	var tracks []synTrack
	var infoTracks []testInfoTrack

	for i, bf := range bookFiles {
		pid := randomPID()
		if bf.ITunesPersistentID != "" {
			if p, err := hexToPID(bf.ITunesPersistentID); err == nil {
				pid = p
			}
		}

		// Alternate: even-indexed use iTunes path, odd use AO path
		var loc string
		if i%2 == 0 && bf.ITunesPath != "" {
			loc = toWin(bf.ITunesPath)
		} else {
			loc = toWin(bf.FilePath)
		}

		if loc == "" {
			continue
		}

		bookTitle := bookTitles[bf.BookID]

		tracks = append(tracks, synTrack{
			pid:      pid,
			location: loc,
			name:     bf.Title,
			album:    bookTitle,
			genre:    "Audiobook",
			kind:     kindFromFormat(bf.Format),
		})

		infoTracks = append(infoTracks, testInfoTrack{
			PersistentID: pidHex(pid),
			Location:     loc,
			Name:         bf.Title,
			Album:        bookTitle,
		})
	}

	itlData := buildSyntheticITLFromTracks(tracks)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}

	return writeTestInfo(dir, testInfo{
		Name:               "07-mixed-sources",
		Description:        fmt.Sprintf("Mixed sources: %d tracks (iTunes + AO paths)", len(tracks)),
		ExpectedTrackCount: len(tracks),
		AllowMissingFiles:  true,
		Tracks:             infoTracks,
	})
}

// 08-unicode-paths: Tracks with non-ASCII characters in paths
func genUnicodePaths(dir string, toWin func(string) string) error {
	unicodePaths := []struct {
		author string
		title  string
	}{
		{"Стругацкие", "Пикник на обочине"},
		{"村上春樹", "ノルウェイの森"},
		{"José Saramago", "Ensaio sobre a Cegueira"},
		{"Günter Grass", "Die Blechtrommel"},
		{"Ólafur Jóhann Ólafsson", "Restoration"},
		{"Héctor Abad Faciolince", "El Olvido que Seremos"},
		{"François Mauriac", "Thérèse Desqueyroux"},
		{"Czesław Miłosz", "Zniewolony Umysł"},
	}

	tracks := make([]synTrack, len(unicodePaths))
	infoTracks := make([]testInfoTrack, len(unicodePaths))

	for i, up := range unicodePaths {
		pid := randomPID()
		loc := toWin(fmt.Sprintf("/mnt/bigdata/books/audiobook-organizer/%s/%s/chapter.m4b", up.author, up.title))

		tracks[i] = synTrack{
			pid:      pid,
			location: loc,
			name:     fmt.Sprintf("%s - Chapter 1", up.title),
			album:    up.title,
			artist:   up.author,
			genre:    "Audiobook",
			kind:     "AAC audio file",
		}

		infoTracks[i] = testInfoTrack{
			PersistentID: pidHex(pid),
			Location:     loc,
			Name:         tracks[i].name,
			Artist:       up.author,
			Album:        up.title,
		}
	}

	itlData := buildSyntheticITLFromTracks(tracks)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}

	return writeTestInfo(dir, testInfo{
		Name:               "08-unicode-paths",
		Description:        "Tracks with non-ASCII characters in author/title paths",
		ExpectedTrackCount: len(tracks),
		AllowMissingFiles:  true,
		Tracks:             infoTracks,
	})
}

// 09-missing-files: Tracks pointing to files that do not exist
func genMissingFiles(dir string, toWin func(string) string) error {
	count := 5
	tracks := make([]synTrack, count)
	infoTracks := make([]testInfoTrack, count)

	for i := 0; i < count; i++ {
		pid := randomPID()
		loc := toWin(fmt.Sprintf(
			"/mnt/bigdata/books/audiobook-organizer/NONEXISTENT_AUTHOR_%d/NONEXISTENT_BOOK_%d/missing_file.m4b",
			i+1, i+1,
		))

		tracks[i] = synTrack{
			pid:      pid,
			location: loc,
			name:     fmt.Sprintf("Missing Book %d - Chapter 1", i+1),
			album:    fmt.Sprintf("Missing Book %d", i+1),
			artist:   fmt.Sprintf("Missing Author %d", i+1),
			genre:    "Audiobook",
			kind:     "AAC audio file",
		}

		infoTracks[i] = testInfoTrack{
			PersistentID: pidHex(pid),
			Location:     loc,
			Name:         tracks[i].name,
			Artist:       tracks[i].artist,
			Album:        tracks[i].album,
		}
	}

	itlData := buildSyntheticITLFromTracks(tracks)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}

	return writeTestInfo(dir, testInfo{
		Name:               "09-missing-files",
		Description:        "Tracks pointing to files that intentionally do not exist",
		ExpectedTrackCount: count,
		ExpectMissingFiles: true,
		AllowMissingFiles:  true,
		Tracks:             infoTracks,
	})
}

// 10-duplicate-pids: Intentionally duplicate persistent IDs
func genDuplicatePIDs(dir string, toWin func(string) string) error {
	// Use the same PID for multiple tracks — this tests iTunes' handling of
	// conflicting persistent IDs
	sharedPID := randomPID()
	count := 4
	tracks := make([]synTrack, count)
	infoTracks := make([]testInfoTrack, count)

	for i := 0; i < count; i++ {
		pid := sharedPID // intentional duplicate
		loc := toWin(fmt.Sprintf(
			"/mnt/bigdata/books/audiobook-organizer/Duplicate Author/Duplicate Book %d/chapter.m4b",
			i+1,
		))

		tracks[i] = synTrack{
			pid:      pid,
			location: loc,
			name:     fmt.Sprintf("Duplicate PID Track %d", i+1),
			album:    fmt.Sprintf("Duplicate Book %d", i+1),
			artist:   "Duplicate Author",
			genre:    "Audiobook",
			kind:     "AAC audio file",
		}

		infoTracks[i] = testInfoTrack{
			PersistentID: pidHex(pid),
			Location:     loc,
			Name:         tracks[i].name,
			Artist:       "Duplicate Author",
			Album:        tracks[i].album,
		}
	}

	itlData := buildSyntheticITLFromTracks(tracks)
	if err := writeITLToDir(dir, itlData); err != nil {
		return err
	}

	return writeTestInfo(dir, testInfo{
		Name:               "10-duplicate-pids",
		Description:        fmt.Sprintf("%d tracks sharing the same persistent ID (conflict test)", count),
		ExpectedTrackCount: count,
		AllowMissingFiles:  true,
		Tracks:             infoTracks,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// kindFromFormat returns the iTunes "Kind" string for a given audio format.
func kindFromFormat(format string) string {
	switch strings.ToLower(format) {
	case "m4b", "m4a", "aac":
		return "AAC audio file"
	case "mp3":
		return "MPEG audio file"
	case "ogg":
		return "Ogg Vorbis file"
	case "flac":
		return "FLAC audio file"
	case "wav":
		return "WAV audio file"
	default:
		return "AAC audio file"
	}
}
