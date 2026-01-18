// file: internal/metadata/metadata.go
// version: 1.7.2
// guid: 9d0e1f2a-3b4c-5d6e-7f8a-9b0c1d2e3f4a

package metadata

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dhowden/tag"
)

var yearPattern = regexp.MustCompile(`(\d{4})`)

// Metadata holds audio file metadata
type Metadata struct {
	Title       string
	Artist      string
	Album       string
	Genre       string
	Series      string
	SeriesIndex int
	Comments    string
	Year        int
	// Extended fields (best-effort; may be empty)
	Narrator  string
	Language  string
	Publisher string
	ISBN10    string
	ISBN13    string
}

type fieldCandidate struct {
	value  string
	source string
}

func pickFirstNonEmpty(candidates ...fieldCandidate) (string, string) {
	for _, candidate := range candidates {
		if trimmed := cleanTagValue(candidate.value); trimmed != "" {
			return trimmed, candidate.source
		}
	}
	return "", ""
}

func setFieldSource(sources map[string]string, field, source string) {
	if sources == nil || source == "" {
		return
	}
	sources[field] = source
}

func sourceOrUnknown(sources map[string]string, field string) string {
	if sources == nil {
		return "unset"
	}
	if value, ok := sources[field]; ok && value != "" {
		return value
	}
	return "unset"
}

// ExtractMetadata reads metadata from audio files
func ExtractMetadata(filePath string) (Metadata, error) {
	var metadata Metadata
	log.Printf("[DEBUG] metadata: extracting tags for %s", filePath)
	fieldSources := map[string]string{}
	seriesIndexSource := ""
	fallbackUsed := false
	authorFromArtist := false

	f, err := os.Open(filePath)
	if err != nil {
		return metadata, fmt.Errorf("error opening file: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		log.Printf("[WARN] metadata: failed to read tags for %s: %v; falling back to filename parsing", filePath, err)
		metadata = extractFromFilename(filePath)
		if metadata.SeriesIndex == 0 {
			metadata.SeriesIndex = DetectVolumeNumber(metadata.Title)
		}
		return metadata, nil
	}

	raw := m.Raw()
	if len(raw) > 0 {
		rawKeys := make([]string, 0, len(raw))
		for key := range raw {
			rawKeys = append(rawKeys, key)
		}
		sort.Strings(rawKeys)
		log.Printf("[TRACE] metadata: raw tag keys for %s: %v", filePath, rawKeys)
	}

	albumValue, albumSource := pickFirstNonEmpty(
		fieldCandidate{value: m.Album(), source: "tag.Album"},
		fieldCandidate{value: getRawString(raw, "TALB", "\xa9alb", "©alb", "album"), source: "raw.album"},
	)
	metadata.Album = cleanTagValue(albumValue)
	if metadata.Album != "" {
		setFieldSource(fieldSources, "album", albumSource)
	}

	titleValue, titleSource := pickFirstNonEmpty(
		fieldCandidate{value: m.Title(), source: "tag.Title"},
		fieldCandidate{value: getRawString(raw, "\xa9nam", "©nam", "title", "TIT2"), source: "raw.title"},
		fieldCandidate{value: metadata.Album, source: "album"},
	)
	metadata.Title = cleanTagValue(titleValue)
	if metadata.Title != "" {
		setFieldSource(fieldSources, "title", titleSource)
	} else {
		metadata.Title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		setFieldSource(fieldSources, "title", "filename default (basename)")
	}

	composerValue, composerSource := pickFirstNonEmpty(
		fieldCandidate{value: m.Composer(), source: "tag.Composer"},
		fieldCandidate{value: getRawString(raw, "TCOM", "\xa9wrt", "composer"), source: "raw.composer"},
	)
	albumArtistValue, albumArtistSource := pickFirstNonEmpty(
		fieldCandidate{value: m.AlbumArtist(), source: "tag.AlbumArtist"},
		fieldCandidate{value: getRawString(raw, "TPE2", "ALBUMARTIST", "AlbumArtist", "album_artist", "aART"), source: "raw.album_artist"},
	)
	artistValue, artistSource := pickFirstNonEmpty(
		fieldCandidate{value: m.Artist(), source: "tag.Artist"},
		fieldCandidate{value: getRawString(raw, "TPE1", "artist", "\xa9ART", "©ART"), source: "raw.artist"},
	)
	if composerValue != "" {
		metadata.Artist = composerValue
		setFieldSource(fieldSources, "author", composerSource+" (composer)")
	} else if albumArtistValue != "" {
		metadata.Artist = cleanTagValue(albumArtistValue)
		if metadata.Artist != "" {
			setFieldSource(fieldSources, "author", albumArtistSource+" (album_artist)")
		}
	} else {
		metadata.Artist = cleanTagValue(artistValue)
		if metadata.Artist != "" {
			setFieldSource(fieldSources, "author", artistSource)
			authorFromArtist = true
		}
	}

	genreValue, genreSource := pickFirstNonEmpty(
		fieldCandidate{value: m.Genre(), source: "tag.Genre"},
		fieldCandidate{value: getRawString(raw, "TCON", "genre", "\xa9gen", "©gen"), source: "raw.genre"},
	)
	metadata.Genre = cleanTagValue(genreValue)
	if metadata.Genre != "" {
		setFieldSource(fieldSources, "genre", genreSource)
	}

	narratorValue, narratorSource := pickFirstNonEmpty(
		fieldCandidate{value: getRawString(raw, "PERFORMER", "Performer", "TXXX:NARRATOR", "TXXX:Narrator", "NARRATOR", "Narrator", "©nrt", "\xa9nrt"), source: "raw.narrator"},
		fieldCandidate{value: getRawString(raw, "TXXX:Reader", "READER"), source: "raw.reader"},
	)
	metadata.Narrator = cleanTagValue(narratorValue)
	if metadata.Narrator != "" {
		setFieldSource(fieldSources, "narrator", narratorSource)
	} else if !authorFromArtist {
		artistFallback := cleanTagValue(artistValue)
		if artistFallback != "" && artistFallback != metadata.Artist {
			metadata.Narrator = artistFallback
			setFieldSource(fieldSources, "narrator", artistSource)
		}
	}

	languageValue, languageSource := pickFirstNonEmpty(
		fieldCandidate{value: getRawString(raw, "TLAN", "LANGUAGE"), source: "raw.language"},
	)
	metadata.Language = cleanTagValue(languageValue)
	if metadata.Language != "" {
		setFieldSource(fieldSources, "language", languageSource)
	}

	publisherValue, publisherSource := pickFirstNonEmpty(
		fieldCandidate{value: getRawString(raw, "TPUB", "publisher", "©pub", "\xa9pub"), source: "raw.publisher"},
	)
	metadata.Publisher = cleanTagValue(publisherValue)
	if metadata.Publisher != "" {
		setFieldSource(fieldSources, "publisher", publisherSource)
	}

	commentValue, commentSource := pickFirstNonEmpty(
		fieldCandidate{value: m.Comment(), source: "tag.Comment"},
		fieldCandidate{value: getRawString(raw, "COMM", "comment", "©cmt", "\xa9cmt"), source: "raw.comment"},
	)
	metadata.Comments = cleanTagValue(commentValue)
	if metadata.Comments != "" {
		setFieldSource(fieldSources, "comment", commentSource)
	}

	metadata.Year = m.Year()
	yearSource := "tag.Year"
	if metadata.Year == 0 {
		yearSource = ""
		if yearStr := getRawString(raw, "TDRC", "TYER", "TDOR", "©day", "\xa9day"); yearStr != "" {
			if parsedYear, convErr := strconv.Atoi(extractYearDigits(yearStr)); convErr == nil {
				metadata.Year = parsedYear
				yearSource = "raw.year"
			}
		}
	}
	if metadata.Year != 0 && yearSource != "" {
		setFieldSource(fieldSources, "year", yearSource)
	}

	seriesValue, seriesSource := pickFirstNonEmpty(
		fieldCandidate{value: getRawString(raw, "TXXX:SERIES", "SERIES", "SERIES_NAME", "SERIESNAME", "GRP1", "TGID", "©grp", "\xa9grp"), source: "raw.series"},
	)
	metadata.Series = cleanTagValue(seriesValue)
	if metadata.Series != "" {
		setFieldSource(fieldSources, "series", seriesSource)
	}
	if metadata.Series == "" && strings.Contains(metadata.Album, " - ") {
		parts := strings.Split(metadata.Album, " - ")
		if len(parts) > 1 {
			metadata.Series = strings.TrimSpace(parts[0])
			seriesSource = "album-prefix"
			setFieldSource(fieldSources, "series", seriesSource)
		}
	}
	if metadata.Series == "" && metadata.Comments != "" {
		if extracted := extractSeriesFromComments(metadata.Comments); extracted != "" {
			metadata.Series = extracted
			seriesSource = "comment-extraction"
			setFieldSource(fieldSources, "series", seriesSource)
		}
	}
	if metadata.Series == "" {
		if series, idx := extractSeriesFromVolumeString(metadata.Album); series != "" {
			metadata.Series = series
			seriesSource = "album-volume"
			setFieldSource(fieldSources, "series", seriesSource)
			if metadata.SeriesIndex == 0 && idx > 0 {
				metadata.SeriesIndex = idx
				seriesIndexSource = "album-volume"
			}
		}
	}
	if metadata.Series == "" {
		if series, idx := extractSeriesFromVolumeString(metadata.Title); series != "" {
			metadata.Series = series
			seriesSource = "title-volume"
			setFieldSource(fieldSources, "series", seriesSource)
			if metadata.SeriesIndex == 0 && idx > 0 {
				metadata.SeriesIndex = idx
				seriesIndexSource = "title-volume"
			}
		}
	}
	if metadata.Series == "" {
		if metadata.Album != "" {
			metadata.Series = metadata.Album
			seriesSource = "album fallback"
			setFieldSource(fieldSources, "series", seriesSource)
		} else if metadata.Title != "" {
			metadata.Series = metadata.Title
			seriesSource = "title fallback"
			setFieldSource(fieldSources, "series", seriesSource)
		}
	}

	metadata.SeriesIndex = DetectVolumeNumber(metadata.Title)
	if metadata.SeriesIndex > 0 {
		seriesIndexSource = "title"
	}
	if metadata.SeriesIndex == 0 {
		if idx := DetectVolumeNumber(metadata.Album); idx > 0 {
			metadata.SeriesIndex = idx
			seriesIndexSource = "album"
		}
	}
	if metadata.SeriesIndex == 0 && metadata.Comments != "" {
		if idx := DetectVolumeNumber(metadata.Comments); idx > 0 {
			metadata.SeriesIndex = idx
			seriesIndexSource = "comment"
		}
	}

	if metadata.Title == "" || metadata.Artist == "" || metadata.Narrator == "" || metadata.Series == "" {
		fallback := extractFromFilename(filePath)
		fallbackUsed = true
		if metadata.Title == "" && fallback.Title != "" {
			metadata.Title = fallback.Title
			setFieldSource(fieldSources, "title", "filename pattern")
		}
		if metadata.Artist == "" && fallback.Artist != "" {
			metadata.Artist = fallback.Artist
			setFieldSource(fieldSources, "author", "filename pattern")
		}
		if metadata.Narrator == "" && fallback.Narrator != "" {
			metadata.Narrator = fallback.Narrator
			setFieldSource(fieldSources, "narrator", "filename pattern")
		}
		if metadata.Series == "" && fallback.Series != "" {
			metadata.Series = fallback.Series
			setFieldSource(fieldSources, "series", "filename pattern")
		}
		if metadata.SeriesIndex == 0 && fallback.SeriesIndex > 0 {
			metadata.SeriesIndex = fallback.SeriesIndex
			seriesIndexSource = "filename pattern"
		}
	}

	if metadata.ISBN13 == "" {
		isbn13Value, isbn13Source := pickFirstNonEmpty(
			fieldCandidate{value: getRawString(raw, "TXXX:ISBN13", "TXXX:ISBN", "ISBN13", "ISBN"), source: "raw.isbn13"},
		)
		metadata.ISBN13 = cleanTagValue(isbn13Value)
		if metadata.ISBN13 != "" {
			setFieldSource(fieldSources, "isbn13", isbn13Source)
		}
	}
	if metadata.ISBN10 == "" {
		isbn10Value, isbn10Source := pickFirstNonEmpty(
			fieldCandidate{value: getRawString(raw, "TXXX:ISBN10", "ISBN10"), source: "raw.isbn10"},
		)
		metadata.ISBN10 = cleanTagValue(isbn10Value)
		if metadata.ISBN10 != "" {
			setFieldSource(fieldSources, "isbn10", isbn10Source)
		}
	}

	if seriesIndexSource == "" && metadata.SeriesIndex > 0 {
		seriesIndexSource = "detected"
	}
	if fallbackUsed {
		log.Printf("[TRACE] metadata: filename fallback filled gaps for %s", filePath)
	}
	log.Printf(
		"[TRACE] metadata: sources for %s | title=%s | author=%s | series=%s | series_index=%s | narrator=%s | album=%s | publisher=%s | language=%s",
		filePath,
		sourceOrUnknown(fieldSources, "title"),
		sourceOrUnknown(fieldSources, "author"),
		sourceOrUnknown(fieldSources, "series"),
		seriesIndexSource,
		sourceOrUnknown(fieldSources, "narrator"),
		sourceOrUnknown(fieldSources, "album"),
		sourceOrUnknown(fieldSources, "publisher"),
		sourceOrUnknown(fieldSources, "language"),
	)

	log.Printf("[DEBUG] metadata: extracted for %s (title=%q author=%q series=%q position=%d)", filePath, metadata.Title, metadata.Artist, metadata.Series, metadata.SeriesIndex)
	return metadata, nil
}

func cleanTagValue(value string) string {
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func getRawString(raw map[string]interface{}, keys ...string) string {
	if raw == nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			if normalized := normalizeRawTagValue(value); normalized != "" {
				return normalized
			}
		}
		for actualKey, actualValue := range raw {
			if strings.EqualFold(actualKey, key) {
				if normalized := normalizeRawTagValue(actualValue); normalized != "" {
					return normalized
				}
			}
			if comm, ok := actualValue.(*tag.Comm); ok {
				if strings.EqualFold(comm.Description, key) {
					if normalized := strings.TrimSpace(comm.Text); normalized != "" {
						return normalized
					}
				}
			}
			if comm, ok := actualValue.(tag.Comm); ok {
				if strings.EqualFold(comm.Description, key) {
					if normalized := strings.TrimSpace(comm.Text); normalized != "" {
						return normalized
					}
				}
			}
		}
	}
	return ""
}

func normalizeRawTagValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		for _, s := range typed {
			if trimmed := strings.TrimSpace(s); trimmed != "" && !looksLikeReleaseGroupTag(trimmed) {
				return trimmed
			}
		}
		for _, s := range typed {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				return trimmed
			}
		}
	case []byte:
		if s := strings.TrimSpace(string(typed)); s != "" {
			return s
		}
	case *tag.Comm:
		if typed != nil {
			if s := strings.TrimSpace(typed.Text); s != "" {
				return s
			}
		}
	case tag.Comm:
		if s := strings.TrimSpace(typed.Text); s != "" {
			return s
		}
	default:
		if s := strings.TrimSpace(fmt.Sprint(typed)); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func extractSeriesFromComments(comment string) string {
	seriesMatches := []string{"Series:", "Series :", "Part of:"}
	for _, marker := range seriesMatches {
		if strings.Contains(comment, marker) {
			parts := strings.Split(comment, marker)
			if len(parts) > 1 {
				section := strings.TrimSpace(parts[1])
				end := strings.IndexAny(section, "\n\r.,;")
				if end > 0 {
					return section[:end]
				}
				return section
			}
		}
	}
	return ""
}

func looksLikeReleaseGroupTag(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && len(trimmed) > 2 {
		return true
	}
	return false
}

func extractYearDigits(value string) string {
	yearPattern := regexp.MustCompile(`(\d{4})`)
	if match := yearPattern.FindStringSubmatch(value); len(match) > 1 {
		return match[1]
	}
	return value
}

// extractFromFilename tries to extract metadata from filename when tags are unavailable
func extractFromFilename(filePath string) Metadata {
	var metadata Metadata

	filename := filepath.Base(filePath)
	// Remove extension
	filename = strings.TrimSuffix(filename, filepath.Ext(filename))

	// Remove leading track/chapter numbers (e.g., "01 - Title" or "001 Title")
	parts := strings.Split(filename, " ")
	if len(parts) > 0 {
		if _, err := strconv.Atoi(parts[0]); err == nil {
			filename = strings.Join(parts[1:], " ")
		}
	}
	filename = strings.TrimSpace(filename)

	// Remove chapter info from end (e.g., "Title-10 Chapter 10" -> "Title")
	re := regexp.MustCompile(`(?i)[-_]\d+\s+Chapter\s+\d+$`)
	filename = re.ReplaceAllString(filename, "")

	// Try underscore separator first (for Author_Title patterns)
	if strings.Contains(filename, "_") && !strings.Contains(filename, " - ") {
		parts := strings.SplitN(filename, "_", 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			if looksLikePersonName(right) && !looksLikePersonName(left) {
				metadata.Title = left
				metadata.Artist = right
				return metadata
			} else if looksLikePersonName(left) && !looksLikePersonName(right) {
				metadata.Title = right
				metadata.Artist = left
				return metadata
			}
		}
	}

	// Try to parse "Title - Author" or "Author - Title" patterns
	if strings.Contains(filename, " - ") {
		title, author := parseFilenameForAuthor(filename)
		if author != "" {
			metadata.Title = title
			metadata.Artist = author
		} else {
			// Fallback to old behavior if we can't determine author
			parts := strings.Split(filename, " - ")
			if len(parts) >= 2 {
				metadata.Series = parts[0]
				metadata.Title = parts[len(parts)-1]
			} else {
				metadata.Title = filename
			}
		}
	} else {
		metadata.Title = filename
	}

	// If we still don't have an artist, try to get from parent directory
	if metadata.Artist == "" {
		metadata.Artist = extractAuthorFromDirectory(filePath)
	}

	if metadata.SeriesIndex == 0 {
		metadata.SeriesIndex = DetectVolumeNumber(metadata.Title)
	}

	return metadata
}

// extractAuthorFromDirectory extracts author from directory name with validation
func extractAuthorFromDirectory(filePath string) string {
	dir := filepath.Dir(filePath)
	dirName := filepath.Base(dir)

	// Skip common non-author directory names
	skipDirs := map[string]bool{
		"books": true, "audiobooks": true, "newbooks": true, "downloads": true,
		"media": true, "audio": true, "library": true, "collection": true,
		"bt": true, "incomplete": true, "data": true,
	}

	if skipDirs[strings.ToLower(dirName)] {
		return ""
	}

	// Handle complex directory patterns like "Author, Co-Author - translator - Title"
	if strings.Contains(dirName, " - translator - ") || strings.Contains(dirName, " - narrated by - ") {
		re := regexp.MustCompile(`^([^-]+)\s*-\s*(?:translator|narrated by)\s*-`)
		matches := re.FindStringSubmatch(dirName)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// Extract author from "Author - Title" directory pattern
	if strings.Contains(dirName, " - ") {
		parts := strings.SplitN(dirName, " - ", 2)
		if len(parts) > 0 {
			author := strings.TrimSpace(parts[0])
			if isValidAuthor(author) {
				return author
			}
		}
	}

	// Use directory name if it's valid
	if isValidAuthor(dirName) {
		return dirName
	}

	return ""
}

// isValidAuthor checks if extracted author string is valid
func isValidAuthor(author string) bool {
	if author == "" {
		return false
	}

	author = strings.ToLower(author)

	// Skip invalid patterns
	if strings.HasPrefix(author, "book") || strings.HasPrefix(author, "chapter") ||
		strings.HasPrefix(author, "part") || strings.HasPrefix(author, "vol") ||
		strings.HasPrefix(author, "volume") || strings.HasPrefix(author, "disc") {
		return false
	}

	// Skip purely numeric (like "01", "02")
	if _, err := strconv.Atoi(author); err == nil {
		return false
	}

	// Skip chapter patterns
	if strings.HasPrefix(author, "chapter ") {
		return false
	}

	return true
} // parseFilenameForAuthor attempts to intelligently parse title and author from filename
// Handles patterns like "Title - Author" or "Author - Title"
// Returns (title, author) where author is empty string if pattern not detected
func parseFilenameForAuthor(filename string) (string, string) {
	parts := strings.Split(filename, " - ")
	if len(parts) != 2 {
		return "", "" // Not a simple two-part pattern
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	// Heuristic: check if right side looks like an author name
	rightIsName := looksLikePersonName(right)
	leftIsName := looksLikePersonName(left)

	if rightIsName && !leftIsName {
		// Pattern: "Title - Author"
		return left, right
	} else if leftIsName && !rightIsName {
		// Pattern: "Author - Title"
		return right, left
	} else if rightIsName {
		// Both could be names, prefer "Title - Author" pattern
		return left, right
	}

	// Couldn't determine, return empty author
	return "", ""
}

// looksLikePersonName checks if a string looks like a person's name
// Looks for patterns like "John Smith", "J. Smith", "J. K. Rowling"
func looksLikePersonName(s string) bool {
	if !isValidAuthor(s) {
		return false
	}

	// Check for initials like "J. K. Rowling" or "J.K. Rowling"
	if strings.Contains(s, ".") {
		// Count uppercase letters and periods
		uppers := 0
		for _, r := range s {
			if r >= 'A' && r <= 'Z' {
				uppers++
			}
		}
		if uppers >= 2 {
			return true
		}
	}

	// Check for multi-word names with proper capitalization
	words := strings.Fields(s)
	if len(words) >= 2 && len(words) <= 4 {
		// Check if all words start with uppercase
		allProperCase := true
		for _, word := range words {
			if len(word) == 0 || (word[0] < 'A' || word[0] > 'Z') {
				allProperCase = false
				break
			}
		}
		if allProperCase {
			return true
		}
	}

	// Check for "FirstName LastName" pattern (at least one space, proper case)
	if len(words) >= 2 {
		// First word starts with capital
		if len(words[0]) > 0 && words[0][0] >= 'A' && words[0][0] <= 'Z' {
			// Second word starts with capital
			if len(words[1]) > 0 && words[1][0] >= 'A' && words[1][0] <= 'Z' {
				return true
			}
		}
	}

	return false
}
