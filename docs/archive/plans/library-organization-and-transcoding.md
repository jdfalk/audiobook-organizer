<!-- file: docs/plans/library-organization-and-transcoding.md -->
<!-- version: 2.1.0 -->
<!-- guid: f5a6b7c8-d9e0-1f2a-3b4c-5d6e7f8a9b0c -->
<!-- last-edited: 2026-02-01 -->

# Library Organization and Transcoding

## Overview

Advanced file/folder naming templates (Sonarr/Radarr style), audio transcoding,
chapter management, and web download/export capabilities.

**Priority**: Post-MVP (vNext)

**Status: 🟡 In Progress (Active Bug complete as of 2026-02-01)**

---

## Active Bug

### Corrupted Organize Paths

Some books were organized with unresolved placeholders in their paths (e.g.,
`library/Unknown Author/{series}/...`). The template expansion fix has been
applied to prevent future occurrences, but existing corrupted paths need to be
identified and corrected.

**Status: ✅ Complete (2026-02-01)**

Corrupted path detection is handled via migration 14 (SQLite + PebbleDB), and
the diagnostics command can flag existing records for review.

#### Where Template Expansion Happens

File: `internal/organizer/organizer.go`. The call chain is:

1. `OrganizeBook(book)` (line 42) calls `generateTargetPath(book)` (line 107).
2. `generateTargetPath` calls `expandPattern` twice — once for the folder
   pattern, once for the file pattern.
3. `expandPattern` (line 132) performs all placeholder substitution.

#### The Bug — Placeholder Normalization

Line 133 in `organizer.go`:

```go
result := placeholderNormalizeRegex.ReplaceAllStringFunc(pattern, strings.ToLower)
```

`placeholderNormalizeRegex` is `\{[A-Za-z_]+\}`. This normalizes placeholder
*names* to lowercase before substitution (so `{Title}` becomes `{title}`). The
replacements map (line 187) uses lowercase keys like `"{title}"`, `"{author}"`,
`"{series}"`.

The bug: if a placeholder was typed with an unrecognised name (e.g.,
`{Series_Name}` instead of `{series}`), the normalization step lowercased it
to `{series_name}`, which has no entry in the replacements map. The code at
line 207–210 then tries to remove empty segments or replace with `""`, but
`removeEmptySegment` only strips patterns like ` - {placeholder}` or
`({placeholder})`. A bare `{series_name}` in a path segment survived and was
written to disk.

The fix (already applied) adds a post-substitution guard at line 216:

```go
if leftoverPlaceholderRegex.MatchString(result) {
    return "", fmt.Errorf("unresolved placeholders in pattern result: %s", result)
}
```

This causes `expandPattern` to return an error for any leftover `{...}` tokens,
which propagates up to `OrganizeBook` and prevents the corrupted file from
being written. The book stays at its original path.

#### Finding and Fixing Existing Corrupted Paths

A one-time cleanup migration (or standalone script) is needed to identify books
whose `FilePath` contains literal brace-delimited placeholders and either
re-organize them or flag them for manual review.

Pseudocode for the cleanup logic (run as a migration or admin script):

```go
// CleanupCorruptedOrganizedPaths scans all books for paths containing
// unresolved placeholder tokens and re-organizes them.
func CleanupCorruptedOrganizedPaths(store database.Store, cfg *config.Config) (fixed int, flagged int, err error) {
    corruptedRe := regexp.MustCompile(`\{[A-Za-z_]+\}`)
    books, err := store.GetAllBooks(1_000_000, 0)
    if err != nil {
        return 0, 0, fmt.Errorf("failed to list books: %w", err)
    }

    org := organizer.NewOrganizer(cfg)

    for _, book := range books {
        if !corruptedRe.MatchString(book.FilePath) {
            continue
        }

        // Attempt to generate a clean target path with current metadata
        cleanPath, expandErr := org.GenerateTargetPath(&book) // expose this or use OrganizeBook
        if expandErr != nil {
            // Metadata is still insufficient — flag for manual review
            book.LibraryState = stringPtr("needs_review")
            if _, err := store.UpdateBook(book.ID, &book); err != nil {
                log.Printf("[WARN] cleanup: failed to flag book %s: %v", book.ID, err)
            }
            flagged++
            continue
        }

        // If the file still exists at the corrupted path, move it
        if _, statErr := os.Stat(book.FilePath); statErr == nil {
            if moveErr := os.Rename(book.FilePath, cleanPath); moveErr != nil {
                log.Printf("[WARN] cleanup: rename failed for %s → %s: %v", book.FilePath, cleanPath, moveErr)
                flagged++
                continue
            }
        }

        // Update DB with the clean path
        book.FilePath = cleanPath
        if _, err := store.UpdateBook(book.ID, &book); err != nil {
            log.Printf("[WARN] cleanup: failed to update book %s: %v", book.ID, err)
            continue
        }
        fixed++
    }
    return fixed, flagged, nil
}
```

---

## Advanced Naming Templates

A comprehensive templating system for controlling how files and folders are
named when organized into the library.

### Available Placeholders

| Category  | Placeholders                                                  |
| --------- | ------------------------------------------------------------- |
| Title     | `{Title}`, `{CleanTitle}`, `{TitleThe}` (article handling)    |
| Author    | `{Author}`, `{AuthorLast}`, `{AuthorFirst}`                   |
| Narrator  | `{Narrator}`, `{NarratorLast}`, `{NarratorFirst}`             |
| Series    | `{Series}`, `{SeriesTitle}`, `{SeriesPosition}`               |
| Dates     | `{Year}`, `{PublishYear}`, `{AudiobookReleaseYear}`           |
| Publisher | `{Publisher}`, `{Language}`, `{Edition}`                      |
| Quality   | `{Quality}` (bitrate, codec, sample rate)                     |
| File      | `{Duration}`, `{FileSize}`, `{Format}`                        |
| IDs       | `{Genre}`, `{SubGenre}`, `{Tags}`, `{ISBN}`, `{ASIN}`, `{ISBN13}` |

### Format Modifiers

- Case: `{Title:upper}`, `{Title:lower}`, `{Title:title}`, `{Title:camel}`
- Truncation: `{Title:30}` (from end), `{Title:-30}` (from start)
- Padding: `{SeriesPosition:00}`, `{SeriesPosition:000}`
- Replacement: `{Title:replace(' ','.')}`
- Conditionals: `{Series:+[Series]}` (include only if series exists)

### Template Engine Implementation

The current `expandPattern` function (line 132 of `organizer.go`) performs
simple string replacement. To support modifiers and conditionals, replace the
single-pass `strings.ReplaceAll` loop with a regex-based token parser:

```go
import (
    "regexp"
    "strconv"
    "strings"
    "unicode"
)

// tokenRe matches {placeholder} or {placeholder:modifier} or {placeholder:+literal}
var tokenRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?::([^}]*))?\}`)

// ExpandTemplate parses and expands a template string using a values map.
// Values map keys are lowercase placeholder names (e.g. "title", "author").
// Returns the expanded string and any error.
func ExpandTemplate(pattern string, values map[string]string) (string, error) {
    var expandErr error

    result := tokenRe.ReplaceAllStringFunc(pattern, func(match string) string {
        if expandErr != nil {
            return match // short-circuit on first error
        }

        parts := tokenRe.FindStringSubmatch(match)
        // parts[1] = placeholder name, parts[2] = modifier (may be empty)
        name := strings.ToLower(parts[1])
        modifier := parts[2]

        value, exists := values[name]

        // --- Conditional modifier: {Series:+[Series]} ---
        // If modifier starts with '+', the entire token is included only if value is non-empty.
        // The literal after '+' replaces the token when value is present.
        if strings.HasPrefix(modifier, "+") {
            if value == "" {
                return "" // value absent — emit nothing
            }
            // Replace the special placeholder name inside the literal with the actual value.
            // e.g. {Series:+[{Series}]} → "[Mistborn]"
            literal := modifier[1:]
            literal = strings.ReplaceAll(literal, "{"+name+"}", value)
            return literal
        }

        if !exists || value == "" {
            return "" // empty value, placeholder removed
        }

        // --- Apply modifier ---
        return applyModifier(value, modifier)
    })

    if expandErr != nil {
        return "", expandErr
    }
    return result, nil
}

// applyModifier applies a single modifier string to a value.
func applyModifier(value, modifier string) string {
    if modifier == "" {
        return value
    }

    // Case modifiers
    switch modifier {
    case "upper":
        return strings.ToUpper(value)
    case "lower":
        return strings.ToLower(value)
    case "title":
        return strings.Title(value) //nolint:staticcheck // acceptable for filenames
    case "camel":
        return toCamelCase(value)
    }

    // Padding modifier: "00", "000", etc. (numeric-only modifier of all zeros)
    if allZeros(modifier) {
        width := len(modifier)
        num, err := strconv.Atoi(value)
        if err == nil {
            return fmt.Sprintf("%0*d", width, num)
        }
        // Non-numeric value — left-pad with spaces
        return fmt.Sprintf("%*s", width, value)
    }

    // Truncation: "30" = last 30 chars, "-30" = first 30 chars
    if n, err := strconv.Atoi(modifier); err == nil {
        if n > 0 && len(value) > n {
            return value[len(value)-n:]
        }
        if n < 0 && len(value) > -n {
            return value[:-n]
        }
        return value
    }

    // Replacement: "replace(' ','.')"
    if strings.HasPrefix(modifier, "replace(") && strings.HasSuffix(modifier, ")") {
        inner := modifier[len("replace(") : len(modifier)-1]
        // Parse two single-quoted arguments
        args := parseReplaceArgs(inner)
        if len(args) == 2 {
            return strings.ReplaceAll(value, args[0], args[1])
        }
    }

    return value // unknown modifier — return unchanged
}

func allZeros(s string) bool {
    for _, c := range s {
        if c != '0' { return false }
    }
    return len(s) > 0
}

func toCamelCase(s string) string {
    words := strings.Fields(s)
    for i, w := range words {
        if len(w) == 0 { continue }
        if i == 0 {
            words[i] = strings.ToLower(w)
        } else {
            runes := []rune(strings.ToLower(w))
            runes[0] = unicode.ToUpper(runes[0])
            words[i] = string(runes)
        }
    }
    return strings.Join(words, "")
}

// parseReplaceArgs extracts two single-quoted string arguments from "' ',' '"
func parseReplaceArgs(s string) []string {
    re := regexp.MustCompile(`'([^']*)'`)
    matches := re.FindAllStringSubmatch(s, 2)
    if len(matches) < 2 {
        return nil
    }
    return []string{matches[0][1], matches[1][1]}
}
```

#### Validation Logic

Before a template is saved to config, validate it:

```go
// ValidateTemplate checks a pattern string for common errors.
// Returns a list of validation messages (empty = valid).
func ValidateTemplate(pattern string) []string {
    var issues []string

    // 1. All placeholders must be known
    knownPlaceholders := map[string]bool{
        "title": true, "cleantitle": true, "titlethe": true,
        "author": true, "authorfirst": true, "authorlast": true,
        "narrator": true, "narratorfirst": true, "narratorlast": true,
        "series": true, "seriestitle": true, "seriesposition": true,
        "year": true, "publishyear": true, "audiobookreleaseyear": true,
        "publisher": true, "language": true, "edition": true,
        "quality": true, "duration": true, "filesize": true, "format": true,
        "genre": true, "subgenre": true, "tags": true,
        "isbn": true, "asin": true, "isbn13": true,
    }
    for _, m := range tokenRe.FindAllStringSubmatch(pattern, -1) {
        name := strings.ToLower(m[1])
        if !knownPlaceholders[name] {
            issues = append(issues, fmt.Sprintf("unknown placeholder: {%s}", m[1]))
        }
    }

    // 2. Must contain at least {title} or {series}
    lower := strings.ToLower(pattern)
    if !strings.Contains(lower, "{title}") && !strings.Contains(lower, "{series}") {
        issues = append(issues, "pattern must contain {Title} or {Series}")
    }

    // 3. Filesystem length — expanded result should not exceed 200 chars per segment.
    // We can't check this statically but warn if the static portion is already long.
    segments := strings.Split(pattern, "/")
    for _, seg := range segments {
        // Count non-placeholder characters as a rough lower bound
        stripped := tokenRe.ReplaceAllString(seg, "")
        if len(stripped) > 180 {
            issues = append(issues, fmt.Sprintf("segment '%s' has %d static chars (limit 200 total)", seg, len(stripped)))
        }
    }

    return issues
}
```

### Preset Folder Templates

- `{Author}/{Series}/{Title}` — standard series organization
- `{Author}/{Title} ({Year})` — author-centric flat
- `{Genre}/{Author}/{Title}` — genre-first hierarchy
- `{Language}/{Author}/{Series}` — language separation

### Preset File Templates

- `{Author} - {Title} ({Year})` — simple
- `{Series} - {SeriesPosition:00} - {Title}` — series format
- `{Author}.{Title}.{Year}.{Quality}` — dot-separated

### Additional Features

- Live preview of naming patterns during configuration
- Pattern validation (filesystem compatibility checks)
- Import existing patterns from other media managers
- Per-tag template overrides (different patterns per library section)

---

## Audio Transcoding & Optimization

Convert between audio formats, merge multi-file books, and optimize quality:

- MP3 to M4B conversion for multi-file books
- Chapter metadata preservation during transcoding
- Automatic chapter detection from file names/directory structure
- Cover art embedding in M4B files
- Configurable quality settings (bitrate, codec, sample rate)
- Batch transcoding with priority queue
- Original file preservation options (keep, replace, archive)
- Integration with download: serve M4B instead of ZIP for transcoded books

### ffmpeg Command Patterns

#### MP3 → M4B Single-File Conversion

```bash
# Single MP3 to M4B (re-encode to AAC)
ffmpeg -i input.mp3 \
  -c:a aac \
  -b:a 128k \
  -ar 44100 \
  -ac 2 \
  -map_metadata 0 \
  output.m4b
```

#### Multi-File MP3 → Single M4B with Chapters

Step 1: Generate a file list for concatenation.

```bash
# ffmpeg_input.txt — one line per file, in order
file '/path/to/ch01.mp3'
file '/path/to/ch02.mp3'
file '/path/to/ch03.mp3'
```

Step 2: Concatenate and re-encode:

```bash
ffmpeg -f concat -safe 0 -i ffmpeg_input.txt \
  -c:a aac \
  -b:a 128k \
  -ar 44100 \
  -ac 2 \
  output.m4b
```

Step 3: Inject chapter metadata using ffmpeg's `-metadata` chapter syntax.
Generate an FFMetadata file:

```
;FFMETADATA1
TITLE=The Great Gatsby
ARTIST=F. Scott Fitzgerald

[CHAPTER]
TIMEBASE=1/1000
START=0
END=142500
title=Chapter 1

[CHAPTER]
TIMEBASE=1/1000
START=142500
END=301200
title=Chapter 2
```

Then apply:

```bash
ffmpeg -i output.m4b -i chapters.ffmetadata \
  -map_metadata 1 \
  -map_chapters 1 \
  -c copy \
  output_with_chapters.m4b
```

#### Cover Art Embedding

```bash
# Embed cover.jpg into an existing M4B
ffmpeg -i output_with_chapters.m4b -i cover.jpg \
  -map 0:a \
  -map 1:0 \
  -c:a copy \
  -metadata:s:v title="Album cover" \
  -metadata:s:v comment="Cover (front)" \
  final.m4b
```

### Integration with the Operation Queue

Transcoding is a long-running I/O-heavy operation. Submit via `operations.GlobalQueue`:

```go
// Handler registered as: api.POST("/operations/transcode", s.startTranscode)

func (s *Server) startTranscode(c *gin.Context) {
    var req struct {
        BookID      string `json:"book_id" binding:"required"`
        OutputFormat string `json:"output_format"` // "m4b" (default)
        Bitrate     int    `json:"bitrate"`        // default 128
        KeepOriginal bool  `json:"keep_original"` // default true
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if req.OutputFormat == "" { req.OutputFormat = "m4b" }
    if req.Bitrate == 0      { req.Bitrate = 128 }

    id := ulid.Make().String()
    _, _ = database.GlobalStore.CreateOperation(id, "transcode", nil)

    _ = operations.GlobalQueue.Enqueue(id, "transcode", operations.PriorityNormal,
        func(ctx context.Context, progress operations.ProgressReporter) error {
            book, err := database.GlobalStore.GetBookByID(req.BookID)
            if err != nil || book == nil {
                return fmt.Errorf("book %s not found", req.BookID)
            }

            _ = progress.Log("info", fmt.Sprintf("Transcoding %s → %s @ %dkbps", book.Title, req.OutputFormat, req.Bitrate), nil)

            // Determine input files (single file or directory of segments)
            inputFiles := collectInputFiles(book)

            // Build ffmpeg command
            outputPath := strings.TrimSuffix(book.FilePath, filepath.Ext(book.FilePath)) + "." + req.OutputFormat
            cmd, err := buildTranscodeCommand(inputFiles, outputPath, req.Bitrate, req.OutputFormat)
            if err != nil {
                return fmt.Errorf("failed to build ffmpeg command: %w", err)
            }

            // Execute
            if err := cmd.RunContext(ctx); err != nil {
                return fmt.Errorf("ffmpeg failed: %w", err)
            }

            // Update book record
            if !req.KeepOriginal {
                os.Remove(book.FilePath)
            }
            book.FilePath = outputPath
            book.Format = req.OutputFormat
            _, _ = database.GlobalStore.UpdateBook(book.ID, book)

            _ = progress.UpdateProgress(1, 1, "Transcode complete")
            return nil
        },
    )

    c.JSON(http.StatusAccepted, gin.H{"operation_id": id, "type": "transcode"})
}
```

Chapter detection from filenames: when collecting input files for a
multi-file book, sort by the numeric prefix in each filename (e.g.,
`01_chapter1.mp3`, `02_chapter2.mp3`). Use the existing
`metadata.DetectVolumeNumber` to extract the number, or fall back to
lexicographic sort.

---

## File Handling Improvements

- Concurrent organize operations with folder-level locking
- Metadata tag writing improvements (add narrator, series sequence tags)
- Chapter file merging strategy (combine small segments automatically)

### Concurrent Organize with Folder-Level Locking

The current `startOrganize` handler processes books sequentially inside a
single goroutine (line 2530 of `server.go`). Two books targeting the same
output folder can race on `os.MkdirAll` or file creation. The fix: lock at
the target folder level, not globally.

```go
import "sync"

// folderMu is a global registry of per-folder mutexes.
// Access is protected by folderMuMapLock.
var (
    folderMuMapLock sync.Mutex
    folderMuMap     = make(map[string]*sync.Mutex)
)

// acquireFolderLock returns (and lazily creates) a mutex for the given
// directory path, then locks it. Call the returned function to unlock.
func acquireFolderLock(dir string) func() {
    folderMuMapLock.Lock()
    mu, exists := folderMuMap[dir]
    if !exists {
        mu = &sync.Mutex{}
        folderMuMap[dir] = mu
    }
    folderMuMapLock.Unlock()

    mu.Lock()
    return mu.Unlock
}
```

Usage inside the organize loop:

```go
// In the organize operation function, replace the sequential loop:
var wg sync.WaitGroup
sem := make(chan struct{}, 4) // 4 concurrent organizers

for _, book := range booksToOrganize {
    if progress.IsCanceled() { break }

    wg.Add(1)
    sem <- struct{}{}
    go func(b database.Book) {
        defer wg.Done()
        defer func() { <-sem }()

        // Compute target dir BEFORE acquiring lock
        targetDir := filepath.Dir(computeTargetPath(org, &b))

        // Lock the target folder
        unlock := acquireFolderLock(targetDir)
        defer unlock()

        newPath, err := org.OrganizeBook(&b)
        if err != nil {
            _ = progress.Log("error", fmt.Sprintf("Failed to organize %s: %v", b.Title, err), nil)
            return
        }
        // ... update DB, hash, etc.
        _ = newPath
    }(book)
}
wg.Wait()
```

This allows books targeting *different* folders to proceed concurrently while
serializing writes within the same folder. The semaphore (capacity 4) caps
total parallelism to avoid flooding the filesystem with concurrent I/O.

---

## Web Download & Export

- Download individual audiobook files via web UI
- Automatic ZIP creation for multi-file books
- Progress indicators for ZIP creation and download
- Configurable download formats (original files, ZIP, M4B)
- Batch download support for multiple books
- Resume support for interrupted downloads

---

## Dependencies

- Naming templates depend on multiple authors/narrators being resolved (see
  [`metadata-system.md`](metadata-system.md))
- Transcoding requires an audio processing library (e.g., ffmpeg integration)
- Web download depends on safe file operations

## References

- Current organize logic: `internal/organizer/organizer.go` — `expandPattern` (line 132), `OrganizeBook` (line 42)
- Template expansion bug: `placeholderNormalizeRegex` + leftover check (lines 31, 216)
- Safe file operations: copy-first with SHA256 verification
- Operation queue integration: `internal/operations/queue.go`
- Organize handler: `internal/server/server.go` → `startOrganize` (line ~2459)
- Metadata system: [`metadata-system.md`](metadata-system.md)
