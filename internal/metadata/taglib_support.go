// file: internal/metadata/taglib_support.go
// version: 1.7.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

// TagLib native writer support (default). Falls back to CLI tools on failure.

package metadata

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	taglib "go.senan.xyz/taglib"
)

// standardTagKeys is the set of well-known tag keys that TagLib maps to native
// atoms/frames. Anything not in this set is considered a "custom" tag.
// Built at init time to avoid duplicate-key issues with taglib constants
// that resolve to the same string as a literal key.
var standardTagKeys map[string]bool

func init() {
	standardTagKeys = map[string]bool{
		"TITLE": true, "ARTIST": true, "ALBUM": true, "GENRE": true, "DATE": true,
		"COMMENT": true, "PERFORMER": true, "DESCRIPTION": true, "GROUPING": true,
	}
	// Add taglib constants (some may overlap with the literals above, which is fine).
	for _, k := range []string{
		taglib.AlbumArtist, taglib.Composer, taglib.Album,
		taglib.Language, taglib.MovementName, taglib.MovementNumber,
		taglib.ShowWorkMovement,
	} {
		standardTagKeys[k] = true
	}
}

// taglibAvailable indicates native taglib path compiled in
var taglibAvailable = true

// writeMetadataWithTaglib performs native metadata writing using TagLib.
// Supports basic fields; extended custom fields still require CLI fallback.
func writeMetadataWithTaglib(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	backupPath := filePath + ".backup"
	if err := fileops.SafeCopy(filePath, backupPath, config); err != nil {
		return fmt.Errorf("taglib backup failed: %w", err)
	}
	defer func() {
		if !config.PreserveOriginal {
			_ = os.Remove(backupPath)
		}
	}()

	abs, _ := filepath.Abs(filePath)

	// Build tag map (map[string][]string) according to README examples.
	// Use standard common tag names; TagLib accepts arbitrary keys.
	tags := make(map[string][]string)

	if title, ok := metadata["title"].(string); ok && title != "" {
		tags["TITLE"] = []string{title}
	}
	if artist, ok := metadata["artist"].(string); ok && artist != "" {
		// Prefer ALBUMARTIST if we have a single artist (semantic for audiobooks narrator/author)
		tags[taglib.AlbumArtist] = []string{artist}
		tags["ARTIST"] = []string{artist}
		// Overwrite legacy composer values so stale narrator data does not win
		// when metadata is extracted later.
		tags[taglib.Composer] = []string{artist}
	}
	if album, ok := metadata["album"].(string); ok && album != "" {
		tags[taglib.Album] = []string{album}
	}
	if genre, ok := metadata["genre"].(string); ok && genre != "" {
		tags["GENRE"] = []string{genre}
	}
	if year, ok := metadata["year"].(int); ok && year > 0 {
		tags["DATE"] = []string{fmt.Sprintf("%d", year)}
	}
	if narrator, ok := metadata["narrator"].(string); ok && narrator != "" {
		// Use PERFORMER for narrator (TagLib maps this to MP4 ©prf atom)
		tags["PERFORMER"] = []string{narrator}
		tags["NARRATOR"] = []string{narrator} // Also set custom key for ID3/Vorbis
	}
	if lang, ok := metadata["language"].(string); ok && lang != "" {
		tags[taglib.Language] = []string{strings.ToLower(lang)}
	}
	if pub, ok := metadata["publisher"].(string); ok && pub != "" {
		// LABEL is mapped by TagLib for MP4 (to ----:com.apple.iTunes:LABEL)
		tags["LABEL"] = []string{pub}
		tags["PUBLISHER"] = []string{pub} // Also set for ID3/Vorbis
	}
	if isbn10, ok := metadata["isbn10"].(string); ok && isbn10 != "" {
		tags["ISBN10"] = []string{isbn10}
	}
	if isbn13, ok := metadata["isbn13"].(string); ok && isbn13 != "" {
		tags["ISBN13"] = []string{isbn13}
	}
	if desc, ok := metadata["description"].(string); ok && desc != "" {
		tags["DESCRIPTION"] = []string{desc}
		tags["COMMENT"] = []string{desc} // Standard comment field
	}
	if series, ok := metadata["series"].(string); ok && series != "" {
		tags["SERIES"] = []string{series}                  // ID3/Vorbis custom
		tags[taglib.MovementName] = []string{series}       // MP4 ©mvn atom (TagLib mapped)
		tags["GROUPING"] = []string{series}                // MP4 ©grp atom (widely supported)
		if _, hasAlbum := metadata["album"]; !hasAlbum {
			tags[taglib.Album] = []string{""}
		}
	}
	if si, ok := metadata["series_index"].(int); ok && si > 0 {
		tags["SERIES_INDEX"] = []string{fmt.Sprintf("%d", si)}     // ID3/Vorbis custom
		tags[taglib.MovementNumber] = []string{fmt.Sprintf("%d", si)} // MP4 ©mvi atom
	}
	// Enable show-work-movement flag so players display series info
	if _, hasSeries := metadata["series"]; hasSeries {
		tags[taglib.ShowWorkMovement] = []string{"1"}
	}

	// Write custom AUDIOBOOK_ORGANIZER_* tags for book tracking
	customPairs := [][2]string{
		{TagBookID, "book_id"}, {TagISBN10, "isbn10"}, {TagISBN13, "isbn13"},
		{TagASIN, "asin"}, {TagOpenLibrary, "open_library_id"},
		{TagHardcover, "hardcover_id"}, {TagGoogleBooks, "google_books_id"},
		{TagEdition, "edition"}, {TagPrintYear, "print_year"},
	}
	for _, pair := range customPairs {
		if val, ok := metadata[pair[1]].(string); ok && val != "" {
			tags[pair[0]] = []string{val}
		}
	}
	tags[TagVersion] = []string{CustomTagVersion}

	if len(tags) == 0 {
		return fmt.Errorf("no writable metadata supplied")
	}

	// --- Instrumentation: log the full tag map being sent to taglib ---
	log.Printf("[TAG-DIAG] writeMetadataWithTaglib: file=%s, tag_count=%d", abs, len(tags))
	sortedKeys := make([]string, 0, len(tags))
	for k := range tags {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	customCount := 0
	for _, k := range sortedKeys {
		isCustom := !standardTagKeys[k]
		marker := ""
		if isCustom {
			marker = " [CUSTOM]"
			customCount++
		}
		log.Printf("[TAG-DIAG]   WRITE %s = %q%s", k, tags[k], marker)
	}
	log.Printf("[TAG-DIAG]   total=%d standard=%d custom=%d", len(tags), len(tags)-customCount, customCount)

	if err := taglib.WriteTags(abs, tags, 0); err != nil {
		log.Printf("[TAG-DIAG]   WriteTags ERROR: %v", err)
		if restoreErr := fileops.SafeCopy(backupPath, filePath, config); restoreErr != nil {
			return fmt.Errorf("taglib write failed and restore failed: write=%w restore=%v", err, restoreErr)
		}
		return fmt.Errorf("taglib write failed (restored): %w", err)
	}
	log.Printf("[TAG-DIAG]   WriteTags OK (no error)")

	// --- Instrumentation: read back and compare ---
	readBack, readErr := taglib.ReadTags(abs)
	if readErr != nil {
		log.Printf("[TAG-DIAG]   ReadTags VERIFY ERROR: %v", readErr)
	} else {
		log.Printf("[TAG-DIAG]   ReadTags returned %d keys", len(readBack))
		readKeys := make([]string, 0, len(readBack))
		for k := range readBack {
			readKeys = append(readKeys, k)
		}
		sort.Strings(readKeys)
		for _, k := range readKeys {
			log.Printf("[TAG-DIAG]   READ  %s = %q", k, readBack[k])
		}

		// Compare: which written tags survived?
		survived := 0
		missing := 0
		for _, k := range sortedKeys {
			if _, found := readBack[k]; found {
				survived++
			} else {
				log.Printf("[TAG-DIAG]   MISSING after round-trip: %s (wrote %q)", k, tags[k])
				missing++
			}
		}
		customSurvived := 0
		customMissing := 0
		for _, k := range sortedKeys {
			if standardTagKeys[k] {
				continue
			}
			if _, found := readBack[k]; found {
				customSurvived++
			} else {
				customMissing++
			}
		}
		log.Printf("[TAG-DIAG]   ROUND-TRIP: total survived=%d missing=%d | custom survived=%d missing=%d",
			survived, missing, customSurvived, customMissing)
	}

	return nil
}

// readTagsForDiag reads all tags from a file using taglib for diagnostic purposes.
// Exported logic is in taglib_support to keep the taglib import in one file.
func readTagsForDiag(filePath string) (map[string][]string, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	return taglib.ReadTags(abs)
}
