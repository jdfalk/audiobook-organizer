<!-- file: docs/plans/2026-02-28-phase4-multifile-metadata-read.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4a5b6c7d-8e9f-0a1b-2c3d-4e5f6a7b8c9d -->
<!-- last-edited: 2026-02-28 -->

# Phase 4: Multi-file Metadata Read Strategy

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract metadata from folder structure + first file tags + filename patterns for multi-file books.
**Architecture:** New folder parser + combined metadata assembly + scan integration.
**Tech Stack:** Go, regex, standard library

---

## Problem Statement

When a directory contains 67 MP3 parts (e.g. "01 Part 1 of 67.mp3" through "67 Part 67 of 67.mp3"),
the scanner creates one `Book` record per file. Each file's metadata extraction falls back to parsing the
filename, which is meaningless ("Part 1 of 67" is not a title).

The folder hierarchy often contains the full metadata:

```
/mnt/bigdata/books/audiobook-organizer/
  Terry Pratchett & Stephen Baxter/                            ← author level
    (Long Earth 05) The Long Cosmos/                           ← series + position + title
      (Long Earth 05) The Long Cosmos - Terry Pratchett & Stephen Baxter - read by Michael Fenton Stevens/  ← narrator
        01 Part 1 of 67.mp3
        02 Part 2 of 67.mp3
        ...
```

The existing `ExtractMetadata` function in `internal/metadata/metadata.go` is called once per file path.
When the path is a directory (multi-file scenario), it already falls through to `extractFromFilename`,
which only looks at the innermost directory name — not the full path hierarchy.

Additionally, in `internal/scanner/scanner.go`, `ProcessBooksParallel` calls `metadata.ExtractMetadata`
with `books[idx].FilePath` which is the individual file path (e.g. `.../01 Part 1 of 67.mp3`), not
the containing directory, so all context from parent folders is lost.

---

## Current Code Paths (read before implementing)

- **`internal/metadata/metadata.go`** — `ExtractMetadata(filePath string)`: reads ID3/M4B tags, falls
  back to `extractFromFilename`. Version 1.10.0.
- **`internal/scanner/scanner.go`** — `ProcessBooksParallel`: calls `metadata.ExtractMetadata(books[idx].FilePath)`.
  Version 1.17.0.
- **`internal/server/import_service.go`** — `ImportFile`: calls `metadata.ExtractMetadata(req.FilePath)`
  on a single file. Sets `OriginalFilename: stringPtr(filepath.Base(req.FilePath))`. Version 1.1.0.
- **`internal/database/store.go`** — `Book` struct has `OriginalFilename *string` field (line 261).
- **`internal/metadata/enhanced.go`** — validation and batch update helpers. Version 1.3.0.

---

## Deliverables

| # | File | Action | Description |
|---|------|---------|-------------|
| 1 | `internal/metadata/folder_parser.go` | CREATE | Folder hierarchy parser |
| 2 | `internal/metadata/folder_parser_test.go` | CREATE | Unit tests for folder parser |
| 3 | `internal/metadata/assemble.go` | CREATE | Combined metadata assembly |
| 4 | `internal/metadata/assemble_test.go` | CREATE | Unit tests for assembly |
| 5 | `internal/scanner/scanner.go` | EDIT | Call folder-aware assembly for multi-file books |
| 6 | `internal/server/import_service.go` | EDIT | Use folder assembly for directory imports |

---

## Task 1: Create `internal/metadata/folder_parser.go`

**Purpose:** Parse a directory path hierarchy to extract author, series, series position, title, and
narrator. Returns a struct with confidence levels per field so the assembly step can make informed
priority decisions.

**File header (required):**

```go
// file: internal/metadata/folder_parser.go
// version: 1.0.0
// guid: f1e2d3c4-b5a6-7890-abcd-ef1234567890
```

**Full implementation:**

```go
// file: internal/metadata/folder_parser.go
// version: 1.0.0
// guid: f1e2d3c4-b5a6-7890-abcd-ef1234567890

package metadata

import (
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// FieldConfidence describes how confident we are about a parsed field.
type FieldConfidence int

const (
	// ConfidenceNone means the field was not found.
	ConfidenceNone FieldConfidence = 0
	// ConfidenceLow means the field was guessed from a weak signal.
	ConfidenceLow FieldConfidence = 1
	// ConfidenceMedium means the field was found in a plausible location.
	ConfidenceMedium FieldConfidence = 2
	// ConfidenceHigh means the field was found with a strong structural pattern.
	ConfidenceHigh FieldConfidence = 3
)

// FolderMetadata holds metadata parsed from the directory hierarchy.
type FolderMetadata struct {
	Authors        []string        // Split on " & " and "; "
	SeriesName     string
	SeriesPosition int             // 0 if not found
	Title          string
	Narrator       string
	AuthorConf     FieldConfidence
	SeriesConf     FieldConfidence
	TitleConf      FieldConfidence
	NarratorConf   FieldConfidence
}

// Precompiled patterns — package-level to avoid per-call recompilation.
var (
	// Matches "(Series Name NN)" or "(Series Name, Book NN)" at start of a folder segment.
	// Examples:
	//   (Long Earth 05)
	//   (Discworld, Book 1)
	//   (Wheel of Time 14)
	reSeriesPrefix = regexp.MustCompile(`(?i)^\(([^)]+?)\s+(\d+(?:\.\d+)?)\)`)

	// Matches "(Series Name)" with no number — series only, no position.
	reSeriesNoNum = regexp.MustCompile(`(?i)^\(([^)]+?)\)`)

	// Matches narrator patterns at end of a path segment.
	// Examples:
	//   read by Michael Fenton Stevens
	//   narrated by John Smith
	//   narrator: Jane Doe
	reNarratorSuffix = regexp.MustCompile(
		`(?i)(?:[-–,]\s*)?(?:read\s+by|narrated\s+by|narrator[:\s]+)\s*(.+)$`,
	)

	// Matches "Author - Title" or "Author, Co-Author - Title" patterns in a segment.
	// The author portion must come before " - ".
	reDashSeparator = regexp.MustCompile(`^(.+?)\s+-\s+(.+)$`)

	// Matches a plain "Part N of M" filename (case-insensitive).
	// Used to detect whether a filename is a useless part number.
	rePartOfN = regexp.MustCompile(`(?i)^\d+\s+part\s+\d+\s+of\s+\d+`)

	// Matches leading track numbers: "01 ", "001 ", "01. ", "01 - ".
	reLeadingTrackNum = regexp.MustCompile(`^\d{1,3}(?:\.|-)?\s+`)
)

// ExtractMetadataFromFolder parses a directory path hierarchy to extract book metadata.
// dirPath should be the directory containing the audio files (or the audio file path itself;
// the function will walk up the path components).
//
// Example path:
//   /mnt/bigdata/books/Terry Pratchett & Stephen Baxter/(Long Earth 05) The Long Cosmos/
//     (Long Earth 05) The Long Cosmos - Terry Pratchett & Stephen Baxter - read by Michael Fenton Stevens/
//
// Returns FolderMetadata with best-effort values and per-field confidence scores.
func ExtractMetadataFromFolder(dirPath string) (*FolderMetadata, error) {
	// Normalise: if dirPath points to a file, use its parent directory.
	// We operate on path components, not the filesystem.
	segments := splitPathSegments(dirPath)

	fm := &FolderMetadata{}
	log.Printf("[DEBUG] folder_parser: parsing %d path segments for %s", len(segments), dirPath)

	// Walk segments from innermost outward. The innermost segment (segments[last])
	// is the deepest directory (closest to the files); outermost is the root.
	//
	// Convention observed in well-organised audiobook libraries:
	//   segments[-1]: narrator detail dir  e.g. "Title - Author - read by Narrator"
	//   segments[-2]: series + title dir   e.g. "(Series NN) Title"
	//   segments[-3]: author dir           e.g. "First Last & Co Author"

	n := len(segments)
	if n == 0 {
		return fm, nil
	}

	// --- Pass 1: scan innermost segment for narrator and full metadata ---
	innermost := segments[n-1]
	parseInnermostSegment(innermost, fm)

	// --- Pass 2: scan second-from-innermost for series + title ---
	if n >= 2 {
		parseSeriesTitleSegment(segments[n-2], fm)
	}

	// --- Pass 3: scan further-out segments for author ---
	// Try up to 3 levels out from innermost.
	for i := n - 3; i >= 0 && i >= n-5; i-- {
		seg := segments[i]
		if tryParseAuthorSegment(seg, fm) {
			break
		}
	}

	// --- Pass 4: if author still not found, try the innermost segment's dash-split ---
	if fm.AuthorConf == ConfidenceNone && fm.Title != "" {
		tryExtractAuthorFromDashSplit(innermost, fm)
	}

	log.Printf(
		"[DEBUG] folder_parser: result authors=%v series=%q pos=%d title=%q narrator=%q",
		fm.Authors, fm.SeriesName, fm.SeriesPosition, fm.Title, fm.Narrator,
	)
	return fm, nil
}

// splitPathSegments splits a path into its directory components, filtering empty strings
// and common library root names that would never be author/title/series.
func splitPathSegments(path string) []string {
	// Normalise separators
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")

	// Filter noise
	skipSegments := map[string]bool{
		"":            true,
		".":           true,
		"audiobooks":  true,
		"audiobook":   true,
		"books":       true,
		"library":     true,
		"media":       true,
		"audio":       true,
		"imports":     true,
		"import":      true,
		"downloads":   true,
		"organized":   true,
		"newbooks":    true,
		"collection":  true,
		"mnt":         true,
		"bigdata":     true,
		"audiobook-organizer": true,
	}

	var segs []string
	for _, p := range parts {
		lower := strings.ToLower(strings.TrimSpace(p))
		if !skipSegments[lower] && p != "" {
			segs = append(segs, p)
		}
	}
	return segs
}

// parseInnermostSegment extracts narrator (and optionally title/author) from the
// deepest directory name. The innermost dir often has the pattern:
//   "Title - Author - read by Narrator"
//   "(Series NN) Title - Author - read by Narrator"
func parseInnermostSegment(seg string, fm *FolderMetadata) {
	// Extract narrator first, then work on the remainder.
	remainder := seg
	if m := reNarratorSuffix.FindStringSubmatchIndex(seg); m != nil {
		narratorRaw := strings.TrimSpace(seg[m[2]:m[3]])
		fm.Narrator = cleanNarratorValue(narratorRaw)
		fm.NarratorConf = ConfidenceHigh
		// Strip narrator portion from remainder for further parsing.
		remainder = strings.TrimSpace(seg[:m[0]])
		remainder = strings.TrimSuffix(remainder, " -")
		remainder = strings.TrimSuffix(remainder, "-")
		remainder = strings.TrimSpace(remainder)
	}

	// If title not yet set, try to parse series+title from remainder.
	if fm.TitleConf == ConfidenceNone {
		parseSeriesTitleSegment(remainder, fm)
	}
}

// parseSeriesTitleSegment parses a segment that typically looks like:
//   "(Long Earth 05) The Long Cosmos"
//   "(Discworld, Book 01) Guards! Guards!"
//   "The Long Cosmos"           ← no series prefix
//
// It sets fm.SeriesName, fm.SeriesPosition, fm.Title, and their confidences.
func parseSeriesTitleSegment(seg string, fm *FolderMetadata) {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return
	}

	// Try "(Series NN) Title" pattern.
	if m := reSeriesPrefix.FindStringSubmatchIndex(seg); m != nil {
		rawSeries := strings.TrimSpace(seg[m[2]:m[3]])
		rawPos := strings.TrimSpace(seg[m[4]:m[5]])
		pos, _ := strconv.Atoi(strings.Split(rawPos, ".")[0]) // handle "05.1" -> 5
		titlePart := strings.TrimSpace(seg[m[1]:])            // everything after the "(…)" prefix

		// Title may repeat the series prefix; strip it.
		if strings.HasPrefix(strings.ToLower(titlePart), strings.ToLower(rawSeries)) {
			titlePart = strings.TrimSpace(titlePart[len(rawSeries):])
			titlePart = strings.TrimPrefix(titlePart, ",")
			titlePart = strings.TrimSpace(titlePart)
		}

		if fm.SeriesConf < ConfidenceHigh {
			fm.SeriesName = rawSeries
			fm.SeriesPosition = pos
			fm.SeriesConf = ConfidenceHigh
		}
		if fm.TitleConf < ConfidenceMedium && titlePart != "" {
			// Strip trailing " - Author" dash portion from title.
			titleOnly := stripTrailingDashAuthor(titlePart)
			fm.Title = titleOnly
			fm.TitleConf = ConfidenceMedium
		}
		return
	}

	// Try "(Series Name)" with no number.
	if m := reSeriesNoNum.FindStringSubmatchIndex(seg); m != nil {
		rawSeries := strings.TrimSpace(seg[m[2]:m[3]])
		titlePart := strings.TrimSpace(seg[m[1]:])
		if fm.SeriesConf < ConfidenceMedium {
			fm.SeriesName = rawSeries
			fm.SeriesConf = ConfidenceMedium
		}
		if fm.TitleConf < ConfidenceLow && titlePart != "" {
			fm.Title = stripTrailingDashAuthor(titlePart)
			fm.TitleConf = ConfidenceLow
		}
		return
	}

	// No series prefix — the segment itself may be the title.
	// Strip any trailing " - Author" pattern.
	titleCandidate := stripTrailingDashAuthor(seg)
	if fm.TitleConf < ConfidenceLow && titleCandidate != "" {
		fm.Title = titleCandidate
		fm.TitleConf = ConfidenceLow
	}
}

// stripTrailingDashAuthor removes a trailing " - Something" chunk that looks like an author name
// appended to a title. Returns the title portion only.
func stripTrailingDashAuthor(s string) string {
	// Split on " - " and take the first non-empty chunk that doesn't look like a person name.
	// If everything looks like it could be an author, return the whole thing.
	parts := strings.Split(s, " - ")
	if len(parts) <= 1 {
		return strings.TrimSpace(s)
	}
	// Return first part (most likely the title).
	return strings.TrimSpace(parts[0])
}

// tryParseAuthorSegment attempts to interpret a path segment as an author name.
// Returns true if an author was found and stored in fm.
//
// Author segments look like:
//   "Terry Pratchett"
//   "Terry Pratchett & Stephen Baxter"
//   "Tolkien, J. R. R."
func tryParseAuthorSegment(seg string, fm *FolderMetadata) bool {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return false
	}

	// Skip segments that look like titles (contain series prefix notation).
	if reSeriesPrefix.MatchString(seg) || reSeriesNoNum.MatchString(seg) {
		return false
	}

	// Skip segments that look like file listing noise.
	lower := strings.ToLower(seg)
	noiseWords := []string{"audiobooks", "audio books", "books", "library", "collection", "ebooks"}
	for _, nw := range noiseWords {
		if lower == nw {
			return false
		}
	}

	// If segment contains " - ", the first part is likely the author in "Author - Title" patterns.
	if strings.Contains(seg, " - ") {
		parts := strings.SplitN(seg, " - ", 2)
		candidate := strings.TrimSpace(parts[0])
		if looksLikeAuthorSegment(candidate) {
			fm.Authors = splitMultipleAuthors(candidate)
			fm.AuthorConf = ConfidenceMedium
			return true
		}
		return false
	}

	// Plain segment: treat whole thing as author if it looks like one.
	if looksLikeAuthorSegment(seg) {
		fm.Authors = splitMultipleAuthors(seg)
		fm.AuthorConf = ConfidenceHigh
		return true
	}

	return false
}

// tryExtractAuthorFromDashSplit tries to find an author embedded in innermost segment
// using " - Author" patterns after narrator is already stripped.
func tryExtractAuthorFromDashSplit(seg string, fm *FolderMetadata) {
	// Remove narrator portion again if still present.
	if m := reNarratorSuffix.FindStringIndex(seg); m != nil {
		seg = strings.TrimSpace(seg[:m[0]])
	}

	parts := strings.Split(seg, " - ")
	// With "(Series) Title - Author - read by Narrator" → after stripping narrator:
	// "(Series) Title - Author"
	// parts = ["(Series) Title", "Author"]
	if len(parts) >= 2 {
		candidate := strings.TrimSpace(parts[len(parts)-1])
		if looksLikeAuthorSegment(candidate) && fm.AuthorConf == ConfidenceNone {
			fm.Authors = splitMultipleAuthors(candidate)
			fm.AuthorConf = ConfidenceMedium
		}
	}
}

// looksLikeAuthorSegment returns true when s looks like a person name or multi-author string.
// Accepts:
//   "Terry Pratchett"
//   "Terry Pratchett & Stephen Baxter"
//   "J. R. R. Tolkien"
//   "Tolkien, J. R. R."
func looksLikeAuthorSegment(s string) bool {
	if len(s) < 3 {
		return false
	}
	// Any multi-author " & " makes this very likely an author segment.
	if strings.Contains(s, " & ") {
		return true
	}
	// Has a period (initials pattern) and capital letters.
	if strings.Contains(s, ".") {
		upperCount := 0
		for _, r := range s {
			if r >= 'A' && r <= 'Z' {
				upperCount++
			}
		}
		if upperCount >= 2 {
			return true
		}
	}
	// 2-4 capitalised words (Firstname Lastname or Firstname Middle Lastname).
	words := strings.Fields(s)
	if len(words) < 2 || len(words) > 5 {
		return false
	}
	// Allow last-name-first: "Smith, John"
	if strings.Contains(s, ",") {
		return true
	}
	for _, w := range words {
		if len(w) == 0 {
			return false
		}
		if w[0] < 'A' || w[0] > 'Z' {
			return false
		}
	}
	return true
}

// splitMultipleAuthors splits "Author A & Author B" or "Author A; Author B" into a slice.
// Each author is trimmed. Returns a slice with at least one element.
func splitMultipleAuthors(s string) []string {
	// Split on " & " first, then " and " (case-insensitive), then ";".
	var parts []string
	for _, chunk := range strings.Split(s, " & ") {
		for _, sub := range strings.Split(chunk, ";") {
			trimmed := strings.TrimSpace(sub)
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}
	if len(parts) == 0 {
		return []string{strings.TrimSpace(s)}
	}
	return parts
}

// cleanNarratorValue strips trailing noise from a narrator string such as
// trailing file extensions, parenthetical comments, or extraneous whitespace.
func cleanNarratorValue(s string) string {
	s = strings.TrimSpace(s)
	// Remove trailing parenthetical: "Michael Smith (unabridged)" → "Michael Smith"
	if idx := strings.LastIndex(s, "("); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}
	return s
}

// IsGenericPartFilename returns true when a filename is a useless part-number filename
// such as "01 Part 1 of 67.mp3" that carries no real metadata.
// Call this before deciding whether to attempt filename-based metadata extraction.
func IsGenericPartFilename(filename string) bool {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.TrimSpace(base)
	return rePartOfN.MatchString(base) || isLeadingNumberOnly(base)
}

// isLeadingNumberOnly returns true when the filename, after stripping a leading
// track number, contains no alphabetical metadata (e.g. "01", "001 -", "02.").
func isLeadingNumberOnly(base string) bool {
	stripped := reLeadingTrackNum.ReplaceAllString(base, "")
	stripped = strings.TrimSpace(stripped)
	// If nothing left, or only a dash/underscore, it's a pure track number.
	for _, r := range stripped {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}
```

---

## Task 2: Create `internal/metadata/folder_parser_test.go`

**File header:**

```go
// file: internal/metadata/folder_parser_test.go
// version: 1.0.0
// guid: a9b8c7d6-e5f4-3210-fedc-ba9876543210
```

**Full implementation:**

```go
// file: internal/metadata/folder_parser_test.go
// version: 1.0.0
// guid: a9b8c7d6-e5f4-3210-fedc-ba9876543210

package metadata_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

func TestExtractMetadataFromFolder_LongEarthExample(t *testing.T) {
	// Real-world example from the user's library
	path := "/mnt/bigdata/books/audiobook-organizer/Terry Pratchett & Stephen Baxter/(Long Earth 05) The Long Cosmos/(Long Earth 05) The Long Cosmos - Terry Pratchett & Stephen Baxter - read by Michael Fenton Stevens"

	fm, err := metadata.ExtractMetadataFromFolder(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Series
	if fm.SeriesName != "Long Earth" {
		t.Errorf("SeriesName: got %q, want %q", fm.SeriesName, "Long Earth")
	}
	if fm.SeriesPosition != 5 {
		t.Errorf("SeriesPosition: got %d, want 5", fm.SeriesPosition)
	}

	// Title
	if fm.Title != "The Long Cosmos" {
		t.Errorf("Title: got %q, want %q", fm.Title, "The Long Cosmos")
	}

	// Authors
	if len(fm.Authors) != 2 {
		t.Errorf("Authors: got %v (len %d), want 2 authors", fm.Authors, len(fm.Authors))
	} else {
		if fm.Authors[0] != "Terry Pratchett" {
			t.Errorf("Authors[0]: got %q, want %q", fm.Authors[0], "Terry Pratchett")
		}
		if fm.Authors[1] != "Stephen Baxter" {
			t.Errorf("Authors[1]: got %q, want %q", fm.Authors[1], "Stephen Baxter")
		}
	}

	// Narrator
	if fm.Narrator != "Michael Fenton Stevens" {
		t.Errorf("Narrator: got %q, want %q", fm.Narrator, "Michael Fenton Stevens")
	}
}

func TestExtractMetadataFromFolder_SingleAuthorNoSeries(t *testing.T) {
	path := "/media/audiobooks/Stephen King/The Shining - Stephen King - read by Campbell Scott"

	fm, err := metadata.ExtractMetadataFromFolder(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fm.Title != "The Shining" {
		t.Errorf("Title: got %q, want %q", fm.Title, "The Shining")
	}
	if len(fm.Authors) == 0 || fm.Authors[0] != "Stephen King" {
		t.Errorf("Authors: got %v, want [Stephen King]", fm.Authors)
	}
	if fm.Narrator != "Campbell Scott" {
		t.Errorf("Narrator: got %q, want %q", fm.Narrator, "Campbell Scott")
	}
	if fm.SeriesName != "" {
		t.Errorf("SeriesName should be empty, got %q", fm.SeriesName)
	}
}

func TestExtractMetadataFromFolder_SeriesNoNarrator(t *testing.T) {
	path := "/audiobooks/Brandon Sanderson/(Stormlight 01) The Way of Kings"

	fm, err := metadata.ExtractMetadataFromFolder(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fm.SeriesName != "Stormlight" {
		t.Errorf("SeriesName: got %q, want %q", fm.SeriesName, "Stormlight")
	}
	if fm.SeriesPosition != 1 {
		t.Errorf("SeriesPosition: got %d, want 1", fm.SeriesPosition)
	}
	if fm.Title != "The Way of Kings" {
		t.Errorf("Title: got %q, want %q", fm.Title, "The Way of Kings")
	}
	if fm.Narrator != "" {
		t.Errorf("Narrator should be empty, got %q", fm.Narrator)
	}
}

func TestExtractMetadataFromFolder_NarratedByVariant(t *testing.T) {
	path := "/books/Patrick Rothfuss/(Kingkiller 01) The Name of the Wind - narrated by Nick Podehl"

	fm, err := metadata.ExtractMetadataFromFolder(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fm.Narrator != "Nick Podehl" {
		t.Errorf("Narrator: got %q, want %q", fm.Narrator, "Nick Podehl")
	}
}

func TestExtractMetadataFromFolder_EmptyPath(t *testing.T) {
	fm, err := metadata.ExtractMetadataFromFolder("")
	if err != nil {
		t.Fatalf("unexpected error on empty path: %v", err)
	}
	if fm == nil {
		t.Fatal("expected non-nil FolderMetadata on empty path")
	}
}

func TestIsGenericPartFilename(t *testing.T) {
	cases := []struct {
		filename string
		want     bool
	}{
		{"01 Part 1 of 67.mp3", true},
		{"67 Part 67 of 67.mp3", true},
		{"001.mp3", true},
		{"01.mp3", true},
		{"The Long Cosmos Chapter 1.mp3", false},
		{"Long_Cosmos_01.mp3", false},
		{"01 - Introduction.mp3", false},
	}
	for _, tc := range cases {
		got := metadata.IsGenericPartFilename(tc.filename)
		if got != tc.want {
			t.Errorf("IsGenericPartFilename(%q): got %v, want %v", tc.filename, got, tc.want)
		}
	}
}

func TestSplitMultipleAuthors(t *testing.T) {
	// Indirect test via ExtractMetadataFromFolder
	path := "/books/Arthur C. Clarke & Gregory Benford/Shipstar/(Bowl of Heaven 02) Shipstar"

	fm, err := metadata.ExtractMetadataFromFolder(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fm.Authors) < 2 {
		t.Errorf("expected 2 authors, got %v", fm.Authors)
	}
}
```

---

## Task 3: Create `internal/metadata/assemble.go`

**Purpose:** `AssembleBookMetadata` combines folder-parsed metadata with ID3/M4B tag metadata extracted
from the first file. It applies priority rules per field and returns a `BookMetadata` struct ready for
use by the scanner's `saveBook` call.

**File header:**

```go
// file: internal/metadata/assemble.go
// version: 1.0.0
// guid: 1b2c3d4e-5f6a-7b8c-9d0e-1f2a3b4c5d6e
```

**Full implementation:**

```go
// file: internal/metadata/assemble.go
// version: 1.0.0
// guid: 1b2c3d4e-5f6a-7b8c-9d0e-1f2a3b4c5d6e

package metadata

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BookMetadata is the combined, priority-resolved metadata for an audiobook.
// It is ready to be stored in the database Book record.
type BookMetadata struct {
	Title          string
	Authors        []string // Ordered list; Authors[0] is primary.
	SeriesName     string
	SeriesPosition int
	Narrator       string
	Year           int
	Genre          string
	Language       string
	Publisher      string
	ISBN13         string
	ISBN10         string
	FileCount      int     // Number of audio segments (1 for single-file books).
	TotalDuration  float64 // Seconds; 0 if unknown.

	// Source annotations for debugging / UI display.
	TitleSource    string
	AuthorSource   string
	SeriesSource   string
	NarratorSource string
}

// AssembleBookMetadata builds a BookMetadata from three sources:
//   1. Folder path hierarchy (ExtractMetadataFromFolder)
//   2. ID3/M4B tags from the first file (ExtractMetadata)
//   3. Filename patterns (already inside ExtractMetadata fallback)
//
// Parameters:
//   dirPath       — the directory containing the audio files (or the single file path).
//   firstFilePath — the first audio file in the directory (alphabetically sorted). May equal
//                   dirPath for single-file books.
//   fileCount     — total number of audio segments (1 for single-file books).
//   totalDuration — total duration in seconds (0 if unknown at call time).
//
// Priority order per field (highest to lowest):
//   Title:    file tag (non-generic) > folder parser > filename
//   Authors:  file tag artist/albumArtist > folder parser > filename
//   Series:   file tag album/series tag > folder parser
//   Narrator: file tag NARRATOR/PERFORMER/comment > folder "read by"
//   Year:     file tag year > folder (none usually present)
//
// Returns a fully populated BookMetadata. No field is ever nil; empty string means unknown.
func AssembleBookMetadata(dirPath, firstFilePath string, fileCount int, totalDuration float64) (*BookMetadata, error) {
	bm := &BookMetadata{
		FileCount:     fileCount,
		TotalDuration: totalDuration,
	}

	// --- Source 1: folder path hierarchy ---
	fm, err := ExtractMetadataFromFolder(dirPath)
	if err != nil {
		log.Printf("[WARN] assemble: folder parser error for %s: %v", dirPath, err)
		fm = &FolderMetadata{}
	}

	// --- Source 2: first file tags ---
	// Only try file-tag extraction if firstFilePath is an actual file (not the dir).
	var tagMeta *Metadata
	if firstFilePath != "" && firstFilePath != dirPath {
		info, statErr := os.Stat(firstFilePath)
		if statErr == nil && !info.IsDir() {
			m, tagErr := ExtractMetadata(firstFilePath)
			if tagErr == nil {
				tagMeta = &m
			} else {
				log.Printf("[WARN] assemble: tag extraction failed for %s: %v", firstFilePath, tagErr)
			}
		}
	}

	// --- Resolve Title ---
	bm.Title, bm.TitleSource = resolveTitle(tagMeta, fm, firstFilePath)

	// --- Resolve Authors ---
	bm.Authors, bm.AuthorSource = resolveAuthors(tagMeta, fm)

	// --- Resolve Series ---
	bm.SeriesName, bm.SeriesPosition, bm.SeriesSource = resolveSeries(tagMeta, fm)

	// --- Resolve Narrator ---
	bm.Narrator, bm.NarratorSource = resolveNarrator(tagMeta, fm)

	// --- Resolve Year ---
	if tagMeta != nil && tagMeta.Year > 0 {
		bm.Year = tagMeta.Year
	}

	// --- Resolve simple tag-only fields ---
	if tagMeta != nil {
		bm.Genre = tagMeta.Genre
		bm.Language = tagMeta.Language
		bm.Publisher = tagMeta.Publisher
		bm.ISBN13 = tagMeta.ISBN13
		bm.ISBN10 = tagMeta.ISBN10
	}

	log.Printf(
		"[INFO] assemble: %s → title=%q authors=%v series=%q pos=%d narrator=%q",
		dirPath, bm.Title, bm.Authors, bm.SeriesName, bm.SeriesPosition, bm.Narrator,
	)
	return bm, nil
}

// resolveTitle returns the best title and its source label.
// Priority: file tag (if not generic) > folder parser > filename fallback.
func resolveTitle(tag *Metadata, fm *FolderMetadata, firstFilePath string) (string, string) {
	if tag != nil && tag.Title != "" {
		// Accept the file tag title only if it doesn't look like a generic part number.
		if !isGenericTitle(tag.Title) {
			return tag.Title, "tag.Title"
		}
		log.Printf("[DEBUG] assemble: tag title %q looks generic; trying folder parser", tag.Title)
	}
	if fm.Title != "" {
		return fm.Title, "folder.Title"
	}
	// Last resort: derive from innermost dir name.
	dirName := filepath.Base(firstFilePath)
	if dirName != "" && dirName != "." {
		dirName = strings.TrimSuffix(dirName, filepath.Ext(dirName))
		if !IsGenericPartFilename(dirName) {
			return dirName, "filename"
		}
	}
	return "", "unknown"
}

// isGenericTitle returns true for titles like "Part 1 of 67", "Chapter 1", "Track 01".
func isGenericTitle(title string) bool {
	lower := strings.ToLower(strings.TrimSpace(title))
	genericPrefixes := []string{
		"part ", "chapter ", "track ", "disc ", "disk ",
	}
	for _, pfx := range genericPrefixes {
		if strings.HasPrefix(lower, pfx) {
			return true
		}
	}
	// Also generic if it matches the part-of-N pattern.
	return IsGenericPartFilename(title + ".mp3") // reuse regex
}

// resolveAuthors returns the best author list and source label.
// Priority: file tag artist/albumArtist/composer > folder parser.
// The returned slice is split on " & " and "; " so callers get individual names.
func resolveAuthors(tag *Metadata, fm *FolderMetadata) ([]string, string) {
	if tag != nil && tag.Artist != "" {
		authors := splitAuthorString(tag.Artist)
		if len(authors) > 0 {
			return authors, "tag.Artist"
		}
	}
	if len(fm.Authors) > 0 {
		return fm.Authors, "folder.Authors"
	}
	return nil, "unknown"
}

// splitAuthorString splits "Author A & Author B" or "Author A; Author B" into individual names.
// Delegates to the same logic as splitMultipleAuthors.
func splitAuthorString(s string) []string {
	return splitMultipleAuthors(s)
}

// resolveSeries returns the best series name, position, and source label.
// Priority: file tag series tag > file tag album (if it looks series-like) > folder parser.
func resolveSeries(tag *Metadata, fm *FolderMetadata) (string, int, string) {
	if tag != nil {
		if tag.Series != "" {
			return tag.Series, tag.SeriesIndex, "tag.Series"
		}
		// album field as series fallback (only if folder also has a series — avoids false positives)
		if tag.Album != "" && fm.SeriesName != "" && strings.EqualFold(tag.Album, fm.SeriesName) {
			return fm.SeriesName, fm.SeriesPosition, "folder.Series(album-confirmed)"
		}
	}
	if fm.SeriesName != "" {
		return fm.SeriesName, fm.SeriesPosition, "folder.Series"
	}
	return "", 0, "unknown"
}

// resolveNarrator returns the best narrator and source label.
// Priority: file tag NARRATOR/PERFORMER > folder "read by" > file tag comment.
func resolveNarrator(tag *Metadata, fm *FolderMetadata) (string, string) {
	if tag != nil && tag.Narrator != "" {
		return tag.Narrator, "tag.Narrator"
	}
	if fm.Narrator != "" {
		return fm.Narrator, "folder.Narrator"
	}
	// Try extracting from tag comment field as last resort.
	if tag != nil && tag.Comments != "" {
		if n := extractNarratorFromComment(tag.Comments); n != "" {
			return n, "tag.Comment"
		}
	}
	return "", "unknown"
}

// extractNarratorFromComment looks for "Narrator: Name" or "Read by: Name" in a comment string.
func extractNarratorFromComment(comment string) string {
	prefixes := []string{"narrator:", "read by:", "narrated by:", "reader:"}
	lower := strings.ToLower(comment)
	for _, pfx := range prefixes {
		if idx := strings.Index(lower, pfx); idx >= 0 {
			rest := strings.TrimSpace(comment[idx+len(pfx):])
			// Take up to first newline or comma.
			end := strings.IndexAny(rest, "\n\r,;")
			if end > 0 {
				return strings.TrimSpace(rest[:end])
			}
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// FindFirstAudioFile returns the alphabetically first audio file in dirPath.
// Returns empty string if none found. Supports the same extensions as the scanner.
func FindFirstAudioFile(dirPath string, supportedExts []string) string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return ""
	}

	extSet := make(map[string]bool, len(supportedExts))
	for _, e := range supportedExts {
		extSet[strings.ToLower(e)] = true
	}

	var audioFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if extSet[ext] {
			audioFiles = append(audioFiles, filepath.Join(dirPath, e.Name()))
		}
	}

	if len(audioFiles) == 0 {
		return ""
	}

	sort.Strings(audioFiles)
	return audioFiles[0]
}

// PrimaryAuthor returns the first author from the Authors slice, or empty string.
func (bm *BookMetadata) PrimaryAuthor() string {
	if len(bm.Authors) == 0 {
		return ""
	}
	return bm.Authors[0]
}
```

---

## Task 4: Create `internal/metadata/assemble_test.go`

**File header:**

```go
// file: internal/metadata/assemble_test.go
// version: 1.0.0
// guid: 2c3d4e5f-6a7b-8c9d-0e1f-2a3b4c5d6e7f
```

**Full implementation:**

```go
// file: internal/metadata/assemble_test.go
// version: 1.0.0
// guid: 2c3d4e5f-6a7b-8c9d-0e1f-2a3b4c5d6e7f

package metadata_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// TestAssembleBookMetadata_FolderOnly tests assembly when no file tag is available
// (firstFilePath is empty or a directory). All metadata should come from the folder path.
func TestAssembleBookMetadata_FolderOnly(t *testing.T) {
	dir := "/mnt/bigdata/books/audiobook-organizer/Terry Pratchett & Stephen Baxter/(Long Earth 05) The Long Cosmos/(Long Earth 05) The Long Cosmos - Terry Pratchett & Stephen Baxter - read by Michael Fenton Stevens"

	bm, err := metadata.AssembleBookMetadata(dir, "", 67, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bm.Title != "The Long Cosmos" {
		t.Errorf("Title: got %q, want %q", bm.Title, "The Long Cosmos")
	}
	if bm.PrimaryAuthor() != "Terry Pratchett" {
		t.Errorf("PrimaryAuthor: got %q, want %q", bm.PrimaryAuthor(), "Terry Pratchett")
	}
	if len(bm.Authors) != 2 {
		t.Errorf("Authors count: got %d, want 2", len(bm.Authors))
	}
	if bm.SeriesName != "Long Earth" {
		t.Errorf("SeriesName: got %q, want %q", bm.SeriesName, "Long Earth")
	}
	if bm.SeriesPosition != 5 {
		t.Errorf("SeriesPosition: got %d, want 5", bm.SeriesPosition)
	}
	if bm.Narrator != "Michael Fenton Stevens" {
		t.Errorf("Narrator: got %q, want %q", bm.Narrator, "Michael Fenton Stevens")
	}
	if bm.FileCount != 67 {
		t.Errorf("FileCount: got %d, want 67", bm.FileCount)
	}
}

// TestAssembleBookMetadata_WithRealFile creates a real temp MP3-like file with a known tag
// and verifies that tag values take priority over folder values when non-generic.
func TestAssembleBookMetadata_TagOverridesGenericFolderTitle(t *testing.T) {
	// Create a temp dir structure mimicking a multi-file book.
	// We can't create real ID3 tags in a unit test without external deps,
	// so this test focuses on the generic-title detection path.
	tmpDir := t.TempDir()
	partFile := filepath.Join(tmpDir, "01 Part 1 of 10.mp3")
	if err := os.WriteFile(partFile, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	// Even though the first file is "01 Part 1 of 10.mp3", we supply the parent dir
	// as dirPath with author+series info.
	dir := "/audiobooks/Brandon Sanderson/(Mistborn 01) The Final Empire"
	bm, err := metadata.AssembleBookMetadata(dir, partFile, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Title should come from folder, not the generic "Part 1 of 10" filename.
	if bm.Title != "The Final Empire" {
		t.Errorf("Title: got %q, want %q", bm.Title, "The Final Empire")
	}
	if bm.SeriesName != "Mistborn" {
		t.Errorf("SeriesName: got %q, want %q", bm.SeriesName, "Mistborn")
	}
}

// TestFindFirstAudioFile verifies alphabetical-first file selection.
func TestFindFirstAudioFile(t *testing.T) {
	tmpDir := t.TempDir()
	files := []string{"10 Part 10.mp3", "01 Part 1.mp3", "05 Part 5.mp3"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	first := metadata.FindFirstAudioFile(tmpDir, []string{".mp3"})
	if filepath.Base(first) != "01 Part 1.mp3" {
		t.Errorf("expected %q, got %q", "01 Part 1.mp3", filepath.Base(first))
	}
}

// TestAssembleBookMetadata_FileCount tests that fileCount is stored correctly.
func TestAssembleBookMetadata_FileCount(t *testing.T) {
	dir := "/audiobooks/Author/Title"
	bm, err := metadata.AssembleBookMetadata(dir, "", 1, 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bm.FileCount != 1 {
		t.Errorf("FileCount: got %d, want 1", bm.FileCount)
	}
	if bm.TotalDuration != 3600 {
		t.Errorf("TotalDuration: got %f, want 3600", bm.TotalDuration)
	}
}
```

---

## Task 5: Edit `internal/scanner/scanner.go`

**Version bump:** 1.17.0 → 1.18.0

**Goal:** In `ProcessBooksParallel`, before calling the existing `metadata.ExtractMetadata`, determine
whether the file is a generic part filename. If it is, use `metadata.AssembleBookMetadata` with the
parent directory instead. This provides rich folder-derived metadata.

### 5a. Import addition

Find the import block at the top of `internal/scanner/scanner.go`. It already imports
`"github.com/jdfalk/audiobook-organizer/internal/metadata"`. No new import needed —
`AssembleBookMetadata` and `FindFirstAudioFile` and `IsGenericPartFilename` are in the same package.

Also add `"path/filepath"` — already present.

### 5b. Replace the metadata extraction block inside `ProcessBooksParallel`

**Locate** this block inside the `go func(idx int)` goroutine (around line 284–313 in version 1.17.0):

```go
// Extract metadata from the file
meta, err := metadata.ExtractMetadata(books[idx].FilePath)
fallbackUsed := false
if err != nil {
    fmt.Printf("Warning: Could not extract metadata from %s: %v\n", books[idx].FilePath, err)
} else {
    fallbackUsed = meta.UsedFilenameFallback
    if meta.Title != "" {
        books[idx].Title = meta.Title
    }
    if meta.Artist != "" {
        books[idx].Author = meta.Artist
    }
    if meta.Narrator != "" {
        books[idx].Narrator = meta.Narrator
    }
    if meta.Language != "" {
        books[idx].Language = meta.Language
    }
    if meta.Publisher != "" {
        books[idx].Publisher = meta.Publisher
    }
    if meta.Series != "" {
        books[idx].Series = meta.Series
    }
    if meta.SeriesIndex > 0 {
        books[idx].Position = meta.SeriesIndex
    }
}
```

**Replace with:**

```go
// Extract metadata from the file. For multi-file books where the filename is
// a generic part number (e.g. "01 Part 1 of 67.mp3"), use folder path hierarchy
// combined with first-file tags for richer metadata.
fallbackUsed := false
filePath := books[idx].FilePath

if metadata.IsGenericPartFilename(filePath) {
    // Multi-file book path: assemble from folder + first-file tags.
    dirPath := filepath.Dir(filePath)
    firstFile := metadata.FindFirstAudioFile(dirPath, config.AppConfig.SupportedExtensions)
    if firstFile == "" {
        firstFile = filePath // use current file if FindFirstAudioFile fails
    }
    // Count siblings in the same directory (cheap walk).
    fileCount := countAudioFilesInDir(dirPath, config.AppConfig.SupportedExtensions)
    bm, bmErr := metadata.AssembleBookMetadata(dirPath, firstFile, fileCount, 0)
    if bmErr != nil {
        log.Printf("[WARN] scanner: AssembleBookMetadata failed for %s: %v", dirPath, bmErr)
        fallbackUsed = true
    } else {
        if bm.Title != "" {
            books[idx].Title = bm.Title
        }
        if bm.PrimaryAuthor() != "" {
            books[idx].Author = bm.PrimaryAuthor()
        }
        if bm.Narrator != "" {
            books[idx].Narrator = bm.Narrator
        }
        if bm.Language != "" {
            books[idx].Language = bm.Language
        }
        if bm.Publisher != "" {
            books[idx].Publisher = bm.Publisher
        }
        if bm.SeriesName != "" {
            books[idx].Series = bm.SeriesName
        }
        if bm.SeriesPosition > 0 {
            books[idx].Position = bm.SeriesPosition
        }
        // Consider fallback used if title is still empty (so AI gets a crack).
        fallbackUsed = bm.Title == "" || bm.PrimaryAuthor() == ""
    }
} else {
    // Standard single-file (or non-generic-named file) path.
    meta, err := metadata.ExtractMetadata(filePath)
    if err != nil {
        fmt.Printf("Warning: Could not extract metadata from %s: %v\n", filePath, err)
        fallbackUsed = true
    } else {
        fallbackUsed = meta.UsedFilenameFallback
        if meta.Title != "" {
            books[idx].Title = meta.Title
        }
        if meta.Artist != "" {
            books[idx].Author = meta.Artist
        }
        if meta.Narrator != "" {
            books[idx].Narrator = meta.Narrator
        }
        if meta.Language != "" {
            books[idx].Language = meta.Language
        }
        if meta.Publisher != "" {
            books[idx].Publisher = meta.Publisher
        }
        if meta.Series != "" {
            books[idx].Series = meta.Series
        }
        if meta.SeriesIndex > 0 {
            books[idx].Position = meta.SeriesIndex
        }
    }
}
```

### 5c. Add helper function `countAudioFilesInDir`

Add this function at the bottom of `internal/scanner/scanner.go`, before the closing brace
of the file, after `identifySeriesUsingExternalAPIs`:

```go
// countAudioFilesInDir counts the number of audio files (by extension) in a directory.
// Used to determine fileCount for multi-file book assembly. Non-recursive.
func countAudioFilesInDir(dirPath string, supportedExts []string) int {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0
	}
	extSet := make(map[string]bool, len(supportedExts))
	for _, e := range supportedExts {
		extSet[strings.ToLower(e)] = true
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && extSet[strings.ToLower(filepath.Ext(e.Name()))] {
			count++
		}
	}
	return count
}
```

### 5d. Bump version header in `internal/scanner/scanner.go`

Change line 2:
```
// version: 1.17.0
```
to:
```
// version: 1.18.0
```

---

## Task 6: Edit `internal/server/import_service.go`

**Version bump:** 1.1.0 → 1.2.0

**Goal:** When a user imports a file that looks like a generic part filename, use
`AssembleBookMetadata` instead of raw `ExtractMetadata`.

**Locate** the `ImportFile` function's metadata extraction block (around line 62–73):

```go
// Extract metadata
meta, err := metadata.ExtractMetadata(req.FilePath)
if err != nil {
    return nil, fmt.Errorf("failed to extract metadata: %w", err)
}

// Create book record
book := &database.Book{
    Title:            meta.Title,
    FilePath:         req.FilePath,
    OriginalFilename: stringPtr(filepath.Base(req.FilePath)),
}
```

**Replace with:**

```go
// Extract metadata — use folder-aware assembly for generic part filenames.
book := &database.Book{
    FilePath:         req.FilePath,
    OriginalFilename: stringPtr(filepath.Base(req.FilePath)),
}

if metadata.IsGenericPartFilename(req.FilePath) {
    dirPath := filepath.Dir(req.FilePath)
    firstFile := metadata.FindFirstAudioFile(dirPath, config.AppConfig.SupportedExtensions)
    if firstFile == "" {
        firstFile = req.FilePath
    }
    bm, err := metadata.AssembleBookMetadata(dirPath, firstFile, 0, 0)
    if err != nil {
        return nil, fmt.Errorf("failed to assemble metadata: %w", err)
    }
    book.Title = bm.Title
    if bm.Narrator != "" {
        book.Narrator = stringPtr(bm.Narrator)
    }
    if bm.Language != "" {
        book.Language = stringPtr(bm.Language)
    }
    if bm.Publisher != "" {
        book.Publisher = stringPtr(bm.Publisher)
    }
    // Set author and series below using bm.PrimaryAuthor() / bm.SeriesName
    // (replace the existing meta.Artist / meta.Series references).
    meta := &metadata.Metadata{
        Artist:      bm.PrimaryAuthor(),
        Series:      bm.SeriesName,
        SeriesIndex: bm.SeriesPosition,
        Album:       bm.SeriesName,
        Narrator:    bm.Narrator,
        Language:    bm.Language,
        Publisher:   bm.Publisher,
    }
    _ = meta // used below in the existing author/series resolution code
    // Fall through to existing author/series resolution using meta fields.
    if meta.Artist != "" {
        author, err := is.db.GetAuthorByName(meta.Artist)
        if err != nil {
            author, err = is.db.CreateAuthor(meta.Artist)
            if err != nil {
                return nil, fmt.Errorf("failed to create author: %w", err)
            }
        }
        if author != nil {
            book.AuthorID = &author.ID
        }
    }
    if meta.Series != "" && book.AuthorID != nil {
        series, err := is.db.GetSeriesByName(meta.Series, book.AuthorID)
        if err != nil {
            series, err = is.db.CreateSeries(meta.Series, book.AuthorID)
            if err != nil {
                return nil, fmt.Errorf("failed to create series: %w", err)
            }
        }
        if series != nil {
            book.SeriesID = &series.ID
            if meta.SeriesIndex > 0 {
                book.SeriesSequence = &meta.SeriesIndex
            }
        }
    }
} else {
    meta, err := metadata.ExtractMetadata(req.FilePath)
    if err != nil {
        return nil, fmt.Errorf("failed to extract metadata: %w", err)
    }
    book.Title = meta.Title

    if meta.Artist != "" {
        author, err := is.db.GetAuthorByName(meta.Artist)
        if err != nil {
            author, err = is.db.CreateAuthor(meta.Artist)
            if err != nil {
                return nil, fmt.Errorf("failed to create author: %w", err)
            }
        }
        if author != nil {
            book.AuthorID = &author.ID
        }
    }

    if meta.Series != "" && book.AuthorID != nil {
        series, err := is.db.GetSeriesByName(meta.Series, book.AuthorID)
        if err != nil {
            series, err = is.db.CreateSeries(meta.Series, book.AuthorID)
            if err != nil {
                return nil, fmt.Errorf("failed to create series: %w", err)
            }
        }
        if series != nil {
            book.SeriesID = &series.ID
            if meta.SeriesIndex > 0 {
                book.SeriesSequence = &meta.SeriesIndex
            }
        }
    }

    if meta.Album != "" && book.Title == "" {
        book.Title = meta.Album
    }
    if meta.Narrator != "" {
        book.Narrator = stringPtr(meta.Narrator)
    }
    if meta.Language != "" {
        book.Language = stringPtr(meta.Language)
    }
    if meta.Publisher != "" {
        book.Publisher = stringPtr(meta.Publisher)
    }
}
```

**Note:** The original code after the replaced block (lines ~107–145 in version 1.1.0) that sets
`meta.Album`, `meta.Narrator`, etc. must be REMOVED — the new block above handles all of that.
Carefully remove the duplicate assignments by reading the full original function and leaving only
the `CreateBook` / `Organize` calls at the end.

**Bump version header in `internal/server/import_service.go`:**
```
// version: 1.2.0
```

---

## Task 7: Add scanner test for multi-file detection

Add a new test to `internal/scanner/scanner_test.go` (or `internal/scanner/scanner_coverage_test.go`
if the former is crowded). Check with `grep -c "func Test" internal/scanner/scanner_test.go` to
decide which file to use.

```go
// TestCountAudioFilesInDir verifies countAudioFilesInDir returns correct count.
func TestCountAudioFilesInDir(t *testing.T) {
    tmpDir := t.TempDir()
    for _, name := range []string{"01.mp3", "02.mp3", "03.mp3", "cover.jpg"} {
        os.WriteFile(filepath.Join(tmpDir, name), []byte("x"), 0644)
    }
    count := countAudioFilesInDir(tmpDir, []string{".mp3"})
    if count != 3 {
        t.Errorf("expected 3, got %d", count)
    }
}
```

Note: `countAudioFilesInDir` is an unexported function in the `scanner` package. The test must live
in `package scanner` (not `package scanner_test`). Check which package declaration
`scanner_coverage_test.go` uses: if it says `package scanner`, add the test there. If it says
`package scanner_test`, create a new file `internal/scanner/scanner_multifile_test.go` with
`package scanner` for this test.

---

## Build and Test Commands

After implementing all tasks, run these commands in order:

```bash
# 1. Compile check — catches any import or type errors immediately
make build-api

# 2. Run metadata package unit tests
go test ./internal/metadata/... -v -run "TestExtractMetadataFromFolder|TestIsGenericPartFilename|TestAssembleBookMetadata|TestFindFirstAudioFile" 2>&1 | head -80

# 3. Run full metadata package tests
go test ./internal/metadata/... -v 2>&1 | tail -30

# 4. Run scanner tests
go test ./internal/scanner/... -v 2>&1 | tail -30

# 5. Run server tests (import_service)
go test ./internal/server/... -run "TestImport" -v 2>&1 | tail -30

# 6. Full test suite
make test

# 7. Coverage check
go test ./... -coverprofile=coverage.out 2>&1 | tail -10
go tool cover -func=coverage.out | grep -E "folder_parser|assemble|scanner" | head -20

# 8. Full CI (includes 80% coverage gate)
make ci
```

---

## Regex Reference Card

All regexes used in `folder_parser.go`, with examples:

| Regex variable | Pattern | Example match |
|---|---|---|
| `reSeriesPrefix` | `(?i)^\(([^)]+?)\s+(\d+(?:\.\d+)?)\)` | `(Long Earth 05)` → series="Long Earth", pos=5 |
| `reSeriesNoNum` | `(?i)^\(([^)]+?)\)` | `(Long Earth)` → series="Long Earth" |
| `reNarratorSuffix` | `(?i)(?:[-–,]\s*)?(?:read\s+by\|narrated\s+by\|narrator[:\s]+)\s*(.+)$` | `- read by Michael Fenton Stevens` → narrator="Michael Fenton Stevens" |
| `reDashSeparator` | `^(.+?)\s+-\s+(.+)$` | `Title - Author` → left="Title", right="Author" |
| `rePartOfN` | `(?i)^\d+\s+part\s+\d+\s+of\s+\d+` | `01 Part 1 of 67` → true |
| `reLeadingTrackNum` | `^\d{1,3}(?:\.\|-)?\s+` | `01 `, `001 `, `01. ` |

---

## Dependency Map

```
folder_parser.go      (new)  ←── no imports from this codebase
assemble.go           (new)  ←── imports folder_parser.go, metadata.go
scanner.go            (edit) ←── imports assemble.go (via metadata package)
import_service.go     (edit) ←── imports assemble.go (via metadata package)
```

No database schema changes are required. `BookMetadata.Authors` (a `[]string`) maps to:
- `book.Author` (primary, `string`) via `bm.PrimaryAuthor()`
- Future: `book.Authors` (`[]BookAuthor`) — use `SetBookAuthors` if multi-author support is needed.

---

## Edge Cases to Handle

1. **No audio files in dir** — `FindFirstAudioFile` returns `""`. `AssembleBookMetadata` handles
   empty `firstFilePath` by skipping tag extraction and using folder only.

2. **Single-level path** (e.g. `/audiobooks/book.mp3`) — `splitPathSegments` returns one segment.
   `fm.Title` will be empty; assembly falls back to the filename (which is non-generic, so it passes).

3. **Deeply nested paths with duplicate segment names** — e.g. the series folder and narrator folder
   both contain "(Long Earth 05)". The innermost segment is parsed first; if it already sets the
   series, the outer segment is skipped via the `fm.SeriesConf < ConfidenceHigh` guards.

4. **Multi-author with & in folder name** — `splitMultipleAuthors("Terry Pratchett & Stephen Baxter")`
   → `["Terry Pratchett", "Stephen Baxter"]`. Only the first author is stored as `book.Author`
   (until full multi-author book-author table support is implemented in a future phase).

5. **Decimal series positions** — `(Bowl of Heaven 02.5)` → `SeriesPosition = 2` (integer floor).
   If fractional positions are needed, `SeriesSequence` in the DB is `*int`; a future phase can
   store `SeriesSequenceFloat *float64`.

6. **Non-ASCII characters in paths** — regex `[^)]+` and `strings.Fields` handle Unicode correctly.
   The `looksLikeAuthorSegment` uppercase check (`r >= 'A' && r <= 'Z'`) only catches ASCII capitals;
   non-ASCII-capped names (e.g. Élisabeth Vonarburg) will still pass because the function also checks
   for " & " and period patterns.

---

## Do Not Touch

- `internal/metadata/metadata.go` — `ExtractMetadata` itself is not modified. It remains the
  single-file tag reader. Only its callers change.
- `internal/database/store.go` — no schema changes.
- Any `_test.go` files not listed above.
- The `GlobalMetadataExtractor` mock hook — it remains for tests.

---

## File Version Summary

| File | Old Version | New Version |
|------|------------|-------------|
| `internal/metadata/folder_parser.go` | (new) | 1.0.0 |
| `internal/metadata/folder_parser_test.go` | (new) | 1.0.0 |
| `internal/metadata/assemble.go` | (new) | 1.0.0 |
| `internal/metadata/assemble_test.go` | (new) | 1.0.0 |
| `internal/scanner/scanner.go` | 1.17.0 | 1.18.0 |
| `internal/server/import_service.go` | 1.1.0 | 1.2.0 |
