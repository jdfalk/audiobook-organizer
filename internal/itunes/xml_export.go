// file: internal/itunes/xml_export.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package itunes

import (
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// ExportableBook contains the fields needed to export a book to iTunes XML.
type ExportableBook struct {
	Title       string
	Author      string
	FilePath    string // Windows path (e.g., W:\audiobook-organizer\Author\Title\file.m4b)
	Duration    int64  // milliseconds
	Format      string // file extension: m4b, mp3, m4a
	Genre       string
	TrackNumber int
	DiscNumber  int
	Year        int
}

// formatToKind maps file extensions to iTunes Kind strings.
func formatToKind(format string) string {
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "m4b":
		return "AAC audio file"
	case "m4a":
		return "AAC audio file"
	case "mp3":
		return "MPEG audio file"
	default:
		return "MPEG audio file"
	}
}

// encodeWindowsPathToURL converts a Windows file path to an iTunes file:// URL.
// It handles the path regardless of the platform the code runs on.
func encodeWindowsPathToURL(windowsPath string) string {
	// Normalize backslashes to forward slashes
	path := strings.ReplaceAll(windowsPath, `\`, "/")

	// Ensure leading slash for drive-letter paths (e.g., W:/... -> /W:/...)
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}

	// Split into segments and encode each one individually
	// to preserve / and : while encoding spaces and special chars
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		// Don't encode drive letter segments like "W:"
		if len(seg) == 2 && seg[1] == ':' {
			continue
		}
		segments[i] = url.PathEscape(seg)
	}
	encoded := strings.Join(segments, "/")

	// url.PathEscape does not encode & since it's valid in URL paths,
	// but it must be encoded for the URL to be safe inside XML.
	encoded = strings.ReplaceAll(encoded, "&", "%26")

	return "file://localhost" + encoded
}

// xmlEscape escapes a string for safe inclusion in XML text content.
func xmlEscape(s string) string {
	return html.EscapeString(s)
}

const itunesExportTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Major Version</key><integer>1</integer>
	<key>Minor Version</key><integer>1</integer>
	<key>Application Version</key><string>12.13.10</string>
	<key>Music Folder</key><string>file://localhost/W:/itunes/iTunes%20Media/</string>
	<key>Library Persistent ID</key><string>0000000000000001</string>
	<key>Tracks</key>
	<dict>
{{- range .Tracks}}
		<key>{{.ID}}</key>
		<dict>
			<key>Track ID</key><integer>{{.ID}}</integer>
			<key>Name</key><string>{{.Name}}</string>
			<key>Artist</key><string>{{.Artist}}</string>
			<key>Album</key><string>{{.Album}}</string>
			<key>Genre</key><string>{{.Genre}}</string>
			<key>Kind</key><string>{{.Kind}}</string>
			<key>Total Time</key><integer>{{.TotalTime}}</integer>
{{- if gt .Year 0}}
			<key>Year</key><integer>{{.Year}}</integer>
{{- end}}
{{- if gt .TrackNumber 0}}
			<key>Track Number</key><integer>{{.TrackNumber}}</integer>
{{- end}}
{{- if gt .DiscNumber 0}}
			<key>Disc Number</key><integer>{{.DiscNumber}}</integer>
{{- end}}
			<key>Location</key><string>{{.Location}}</string>
		</dict>
{{- end}}
	</dict>
	<key>Playlists</key>
	<array>
		<dict>
			<key>Name</key><string>Audiobook Organizer Import</string>
			<key>Playlist ID</key><integer>1</integer>
			<key>All Items</key><true/>
			<key>Playlist Items</key>
			<array>
{{- range .Tracks}}
				<dict><key>Track ID</key><integer>{{.ID}}</integer></dict>
{{- end}}
			</array>
		</dict>
	</array>
</dict>
</plist>
`

// templateTrack is the data passed to the template for each track.
type templateTrack struct {
	ID          int
	Name        string
	Artist      string
	Album       string
	Genre       string
	Kind        string
	TotalTime   int64
	Year        int
	TrackNumber int
	DiscNumber  int
	Location    string
}

// templateData is the top-level data passed to the template.
type templateData struct {
	Tracks []templateTrack
}

// ExportBooksToITunesXML generates a valid iTunes Library XML file from the
// given books. The output can be imported into iTunes via
// "File > Library > Import Playlist".
func ExportBooksToITunesXML(books []ExportableBook, outputPath string) error {
	tmpl, err := template.New("itunes").Parse(itunesExportTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	tracks := make([]templateTrack, len(books))
	for i, book := range books {
		genre := book.Genre
		if genre == "" {
			genre = "Audiobook"
		}

		name := xmlEscape(book.Title)
		artist := xmlEscape(book.Author)
		album := xmlEscape(book.Title)
		genreEsc := xmlEscape(genre)

		tracks[i] = templateTrack{
			ID:          i + 1,
			Name:        name,
			Artist:      artist,
			Album:       album,
			Genre:       genreEsc,
			Kind:        formatToKind(book.Format),
			TotalTime:   book.Duration,
			Year:        book.Year,
			TrackNumber: book.TrackNumber,
			DiscNumber:  book.DiscNumber,
			Location:    encodeWindowsPathToURL(book.FilePath),
		}
	}

	data := templateData{Tracks: tracks}

	// Ensure output directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write to a temp file first, then rename for atomicity
	tmpPath := outputPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() {
		f.Close()
		os.Remove(tmpPath)
	}()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close output file: %w", err)
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
