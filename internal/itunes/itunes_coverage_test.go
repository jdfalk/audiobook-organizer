// file: internal/itunes/itunes_coverage_test.go
// version: 1.0.0

package itunes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- XML Export coverage ---

func TestCoverage_ExportBooksToITunesXML_EmptyBooks(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "empty.xml")

	err := ExportBooksToITunesXML([]ExportableBook{}, outputPath)
	if err != nil {
		t.Fatalf("ExportBooksToITunesXML failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if len(data) == 0 {
		t.Error("output file is empty")
	}
	// Should still be valid XML
	content := string(data)
	if !strings.Contains(content, "<?xml") {
		t.Error("missing XML declaration")
	}
	if !strings.Contains(content, "<plist") {
		t.Error("missing plist root")
	}
}

func TestCoverage_ExportBooksToITunesXML_WithBooks(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "export.xml")

	books := []ExportableBook{
		{
			Title:       "The Great Book",
			Author:      "John Author",
			FilePath:    `W:\audiobooks\John Author\The Great Book\book.m4b`,
			Duration:    3600000,
			Format:      "m4b",
			Genre:       "Fiction",
			TrackNumber: 1,
			DiscNumber:  1,
			Year:        2024,
		},
		{
			Title:    "Another Book",
			Author:   "Jane Writer",
			FilePath: `W:\audiobooks\Jane Writer\Another Book\book.mp3`,
			Duration: 7200000,
			Format:   "mp3",
		},
	}

	err := ExportBooksToITunesXML(books, outputPath)
	if err != nil {
		t.Fatalf("ExportBooksToITunesXML failed: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	content := string(data)

	if !strings.Contains(content, "The Great Book") {
		t.Error("missing book title")
	}
	if !strings.Contains(content, "John Author") {
		t.Error("missing author")
	}
	if !strings.Contains(content, "file://localhost") {
		t.Error("missing file URL")
	}
}

func TestCoverage_formatToKind(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{"m4b", "AAC audio file"},
		{"m4a", "AAC audio file"},
		{"mp3", "MPEG audio file"},
		{".m4b", "AAC audio file"},
		{"flac", "MPEG audio file"},
		{"", "MPEG audio file"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got := formatToKind(tt.format)
			if got != tt.want {
				t.Errorf("formatToKind(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestCoverage_encodeWindowsPathToURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // partial match
	}{
		{"drive letter", `W:\audiobooks\test.m4b`, "file://localhost/W:"},
		{"forward slashes", "W:/audiobooks/test.m4b", "file://localhost/W:"},
		{"spaces encoded", `W:\audio books\test.m4b`, "file://localhost/W:/audio%20books"},
		{"unix path", "/mnt/books/test.m4b", "file://localhost/mnt/books"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeWindowsPathToURL(tt.input)
			if !strings.Contains(got, tt.want) {
				t.Errorf("encodeWindowsPathToURL(%q) = %q, want contains %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCoverage_xmlEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"plain text", "plain text"},
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{`he said "hi"`, "he said &#34;hi&#34;"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := xmlEscape(tt.input)
			if got != tt.want {
				t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Parser coverage ---

func TestCoverage_ParseLibrary_NonexistentFile(t *testing.T) {
	_, err := ParseLibrary("/nonexistent/file.xml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestCoverage_ParseLibrary_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	xmlPath := filepath.Join(tmpDir, "bad.xml")
	if err := os.WriteFile(xmlPath, []byte("not xml at all"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseLibrary(xmlPath)
	if err == nil {
		t.Error("expected error for invalid content")
	}
}

func TestCoverage_ParseLibrary_MinimalXML(t *testing.T) {
	tmpDir := t.TempDir()
	xmlPath := filepath.Join(tmpDir, "minimal.xml")
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Major Version</key><integer>1</integer>
	<key>Minor Version</key><integer>1</integer>
	<key>Application Version</key><string>12.0</string>
	<key>Music Folder</key><string>file://localhost/test/</string>
	<key>Tracks</key>
	<dict>
	</dict>
	<key>Playlists</key>
	<array>
	</array>
</dict>
</plist>`

	if err := os.WriteFile(xmlPath, []byte(xmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	lib, err := ParseLibrary(xmlPath)
	if err != nil {
		t.Fatalf("ParseLibrary failed: %v", err)
	}
	if lib == nil {
		t.Fatal("expected non-nil library")
	}
	if len(lib.Tracks) != 0 {
		t.Errorf("expected 0 tracks, got %d", len(lib.Tracks))
	}
}

// --- ExportableBook struct coverage ---

func TestCoverage_ExportableBook_Struct(t *testing.T) {
	book := ExportableBook{
		Title:       "Test",
		Author:      "Author",
		FilePath:    "/path/to/file.m4b",
		Duration:    3600000,
		Format:      "m4b",
		Genre:       "Audiobook",
		TrackNumber: 1,
		DiscNumber:  1,
		Year:        2025,
	}
	if book.Title != "Test" {
		t.Error("Title not set")
	}
}

// --- Library struct coverage ---

func TestCoverage_Track_Struct(t *testing.T) {
	track := Track{
		TrackID:      1,
		PersistentID: "ABC123",
		Name:         "Chapter 1",
		Artist:       "Author",
		AlbumArtist:  "Album Artist",
		Album:        "Book Title",
		Genre:        "Audiobook",
		Kind:         "AAC audio file",
		Year:         2025,
		Size:         1000000,
		TotalTime:    60000,
		PlayCount:    5,
		Rating:       80,
		Bookmark:     30000,
		Bookmarkable: true,
		TrackNumber:  1,
		TrackCount:   10,
		DiscNumber:   1,
		DiscCount:    1,
	}
	if track.PersistentID != "ABC123" {
		t.Error("PersistentID not set")
	}
}

func TestCoverage_Playlist_Struct(t *testing.T) {
	pl := Playlist{
		PlaylistID: 1,
		Name:       "My Playlist",
		TrackIDs:   []int{1, 2, 3},
	}
	if len(pl.TrackIDs) != 3 {
		t.Error("TrackIDs not set correctly")
	}
}
