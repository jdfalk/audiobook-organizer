<!-- file: docs/plans/metadata-system.md -->
<!-- version: 2.0.0 -->
<!-- guid: d3e4f5a6-b7c8-9d0e-1f2a-3b4c5d6e7f8a -->
<!-- last-edited: 2026-01-31 -->

# Metadata System

## Overview

All metadata-related work: fixing current fetch issues, extending the data
model for multiple authors/narrators, improving metadata quality displays,
and adding provenance and release-group tracking.

---

## Active Fixes

### Metadata Fetch Fallback

Open Library search fails for titles that don't match exactly (e.g.,
translated titles like "The Odyssey"). The `stripChapterFromTitle` fix is
insufficient.

- Add fallback: if title search returns no results, retry with author name
  only
- Consider better error messaging explaining why no metadata was found
- Test against known edge cases (translated titles, subtitled works)

#### Current Code — `fetchAudiobookMetadata` in `internal/server/server.go` (line ~3553)

The handler currently follows this search cascade:

```go
// 1. Clean title only
results, err := client.SearchByTitle(searchTitle)

// 2. Raw title if cleaned title differs and returned nothing
if (err != nil || len(results) == 0) && searchTitle != book.Title {
    results, err = client.SearchByTitle(book.Title)
}

// 3. Title + author (only if AuthorID is set)
if (err != nil || len(results) == 0) && book.AuthorID != nil {
    author, authorErr := database.GlobalStore.GetAuthorByID(*book.AuthorID)
    if authorErr == nil && author != nil && author.Name != "" {
        results, err = client.SearchByTitleAndAuthor(searchTitle, author.Name)
        // also tries book.Title + author.Name if cleaned differs
    }
}
```

The problem: step 3 still requires a title. For translated or heavily
subtitled works, no title variant matches. The fix adds a **fourth step**:
author-only search.

#### Modified Search Cascade (add step 4)

```go
// 4. Author-only fallback — last resort when title is unrecognisable
if (err != nil || len(results) == 0) && book.AuthorID != nil {
    author, authorErr := database.GlobalStore.GetAuthorByID(*book.AuthorID)
    if authorErr == nil && author != nil && author.Name != "" {
        log.Printf("[INFO] fetchAudiobookMetadata: title search exhausted, trying author-only for book %s (author=%s)", book.ID, author.Name)
        results, err = client.SearchByAuthor(author.Name)
    }
}
```

#### New method — `SearchByAuthor` in `internal/metadata/openlibrary.go`

```go
// SearchByAuthor searches for books by author name only.
// Useful as a last-resort fallback when title variants all miss.
func (c *OpenLibraryClient) SearchByAuthor(author string) ([]BookMetadata, error) {
    authorQuery := url.QueryEscape(author)
    searchURL := fmt.Sprintf("%s/search.json?author=%s&limit=10", c.baseURL, authorQuery)

    resp, err := c.httpClient.Get(searchURL)
    if err != nil {
        return nil, fmt.Errorf("failed to search Open Library by author: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("Open Library API returned status %d", resp.StatusCode)
    }

    var searchResp SearchResponse
    if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
        return nil, fmt.Errorf("failed to decode search response: %w", err)
    }

    results := make([]BookMetadata, 0, len(searchResp.Docs))
    for _, doc := range searchResp.Docs {
        m := BookMetadata{
            Title:       doc.Title,
            PublishYear: doc.FirstPublishYear,
        }
        if len(doc.AuthorName) > 0 {
            m.Author = strings.Join(doc.AuthorName, ", ")
        }
        if len(doc.Publisher) > 0 {
            m.Publisher = doc.Publisher[0]
        }
        if len(doc.ISBN) > 0 {
            m.ISBN = doc.ISBN[0]
        }
        if len(doc.Language) > 0 {
            m.Language = doc.Language[0]
        }
        if doc.CoverI > 0 {
            m.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.CoverI)
        }
        results = append(results, m)
    }
    return results, nil
}
```

#### Error message improvement

After the full cascade, the error message currently says generically "no
metadata found". Replace with:

```go
if err != nil || len(results) == 0 {
    errorMsg := "no metadata found for this book in Open Library"
    if book.AuthorID != nil {
        author, _ := database.GlobalStore.GetAuthorByID(*book.AuthorID)
        if author != nil {
            errorMsg = fmt.Sprintf(
                "no metadata found for '%s' by '%s' in Open Library — " +
                "tried: title, title+author, author-only",
                book.Title, author.Name,
            )
        }
    }
    c.JSON(http.StatusNotFound, gin.H{"error": errorMsg})
    return
}
```

---

### Multiple Authors and Narrators

Books frequently have multiple authors or narrators. The current approach
concatenates names with a delimiter, which limits queryability and display.

**Design decision needed** — three options:

| Option | Pros | Cons |
| --- | --- | --- |
| Junction table (`book_authors`, `book_narrators`) | Fully queryable, clean joins | More complex queries, migration needed |
| JSON array in Book struct | Simple storage | Not directly queryable |
| Delimited string (current) | No schema change | Cannot filter/sort by individual author |

**Recommendation: Junction table.** Details below.

**UI considerations**: Display multiple authors as chips or comma-separated?
Affects both Library grid and Book Detail views. Same decision applies to
narrators.

#### Junction-Table Data Model

New Go structs (add to `internal/database/store.go` alongside existing types):

```go
// BookAuthor represents a many-to-many link between a book and an author.
// Role distinguishes primary author from co-author, editor, translator, etc.
type BookAuthor struct {
    BookID   string `json:"book_id"`   // ULID — FK → Book.ID
    AuthorID int    `json:"author_id"` // FK → Author.ID
    Role     string `json:"role"`      // "primary", "co_author", "editor", "translator"
    SortOrder int   `json:"sort_order"` // display ordering within the book
}

// BookNarrator represents a many-to-many link between a book and a narrator.
type BookNarrator struct {
    BookID     string `json:"book_id"`     // ULID — FK → Book.ID
    NarratorID int    `json:"narrator_id"` // FK → Narrator.ID (new table, same shape as Author)
    SortOrder  int    `json:"sort_order"`
}

// Narrator represents a narrator entity. Same shape as Author.
type Narrator struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}
```

#### PebbleDB Key Schema (new keys)

```
# Junction: book → authors
book_author:<book_id>:<author_id>   → BookAuthor JSON

# Junction: book → narrators
book_narrator:<book_id>:<narrator_id> → BookNarrator JSON

# Reverse index: author → books (for "books by this author" queries)
book_author:rev:<author_id>:<book_id> → "1"

# Reverse index: narrator → books
book_narrator:rev:<narrator_id>:<book_id> → "1"

# Narrator entity (same pattern as author:<id>)
narrator:<id>                → Narrator JSON
narrator:name:<name>         → narrator_id  (case-insensitive lookup)
counter:narrator             → next narrator ID
```

#### Migration (migration014Up)

```go
// migration014Up adds junction tables for book_authors and book_narrators.
func migration014Up(store Store) error {
    log.Println("  - Adding book_author and book_narrator junction tables")

    // SQLite path
    sqliteStore, ok := store.(*SQLiteStore)
    if ok {
        statements := []string{
            `CREATE TABLE IF NOT EXISTS narrators (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                name TEXT NOT NULL UNIQUE
            )`,
            `CREATE TABLE IF NOT EXISTS book_authors (
                book_id TEXT NOT NULL,
                author_id INTEGER NOT NULL,
                role TEXT NOT NULL DEFAULT 'primary',
                sort_order INTEGER NOT NULL DEFAULT 0,
                PRIMARY KEY (book_id, author_id),
                FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
                FOREIGN KEY (author_id) REFERENCES authors(id)
            )`,
            `CREATE TABLE IF NOT EXISTS book_narrators (
                book_id TEXT NOT NULL,
                narrator_id INTEGER NOT NULL,
                sort_order INTEGER NOT NULL DEFAULT 0,
                PRIMARY KEY (book_id, narrator_id),
                FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
                FOREIGN KEY (narrator_id) REFERENCES narrators(id)
            )`,
            "CREATE INDEX IF NOT EXISTS idx_book_authors_author ON book_authors(author_id)",
            "CREATE INDEX IF NOT EXISTS idx_book_narrators_narrator ON book_narrators(narrator_id)",
        }
        for _, stmt := range statements {
            if _, err := sqliteStore.db.Exec(stmt); err != nil {
                return fmt.Errorf("migration014: %s: %w", stmt, err)
            }
        }

        // Backfill: move existing single-author Book.AuthorID into junction table
        backfill := `
            INSERT OR IGNORE INTO book_authors (book_id, author_id, role, sort_order)
            SELECT id, author_id, 'primary', 0
            FROM books
            WHERE author_id IS NOT NULL
        `
        if _, err := sqliteStore.db.Exec(backfill); err != nil {
            return fmt.Errorf("migration014 backfill authors: %w", err)
        }

        // Backfill narrators: parse existing Narrator field (single name) into narrators table
        // and create junction entries. This is best-effort; NULL narrators are skipped.
        narratorBackfill := `
            INSERT OR IGNORE INTO narrators (name)
            SELECT DISTINCT narrator FROM books WHERE narrator IS NOT NULL AND narrator != ''
        `
        if _, err := sqliteStore.db.Exec(narratorBackfill); err != nil {
            return fmt.Errorf("migration014 backfill narrators: %w", err)
        }
        narratorJunction := `
            INSERT OR IGNORE INTO book_narrators (book_id, narrator_id, sort_order)
            SELECT b.id, n.id, 0
            FROM books b
            JOIN narrators n ON n.name = b.narrator
            WHERE b.narrator IS NOT NULL AND b.narrator != ''
        `
        if _, err := sqliteStore.db.Exec(narratorJunction); err != nil {
            return fmt.Errorf("migration014 backfill narrator junction: %w", err)
        }
        return nil
    }

    // PebbleDB path: backfill existing books' AuthorID and Narrator into junction keys
    pebbleStore, ok := store.(*PebbleStore)
    if !ok {
        log.Println("  - Unknown store type; skipping migration014")
        return nil
    }

    books, err := pebbleStore.GetAllBooks(1_000_000, 0)
    if err != nil {
        return fmt.Errorf("migration014 pebble: %w", err)
    }
    batch := pebbleStore.db.NewBatch()
    for _, b := range books {
        if b.AuthorID != nil {
            ba := BookAuthor{BookID: b.ID, AuthorID: *b.AuthorID, Role: "primary", SortOrder: 0}
            data, _ := json.Marshal(ba)
            key := fmt.Sprintf("book_author:%s:%d", b.ID, *b.AuthorID)
            _ = batch.Set([]byte(key), data, nil)
            revKey := fmt.Sprintf("book_author:rev:%d:%s", *b.AuthorID, b.ID)
            _ = batch.Set([]byte(revKey), []byte("1"), nil)
        }
        if b.Narrator != nil && *b.Narrator != "" {
            // Create or look up narrator
            name := *b.Narrator
            narrator, _ := pebbleStore.GetNarratorByName(name)
            if narrator == nil {
                narrator, _ = pebbleStore.CreateNarrator(name)
            }
            if narrator != nil {
                bn := BookNarrator{BookID: b.ID, NarratorID: narrator.ID, SortOrder: 0}
                data, _ := json.Marshal(bn)
                key := fmt.Sprintf("book_narrator:%s:%d", b.ID, narrator.ID)
                _ = batch.Set([]byte(key), data, nil)
                revKey := fmt.Sprintf("book_narrator:rev:%d:%s", narrator.ID, b.ID)
                _ = batch.Set([]byte(revKey), []byte("1"), nil)
            }
        }
    }
    return batch.Commit(pebble.Sync)
}
```

#### Query Changes

Getting all authors for a book (replaces single AuthorID lookup):

```go
// GetAuthorsForBook returns ordered authors for a book.
func (p *PebbleStore) GetAuthorsForBook(bookID string) ([]BookAuthor, error) {
    var results []BookAuthor
    prefix := []byte(fmt.Sprintf("book_author:%s:", bookID))
    iter, err := p.db.NewIter(&pebble.IterOptions{
        LowerBound: prefix,
        UpperBound: append(prefix, 0xFF),
    })
    if err != nil { return nil, err }
    defer iter.Close()

    for iter.First(); iter.Valid(); iter.Next() {
        var ba BookAuthor
        if err := json.Unmarshal(iter.Value(), &ba); err != nil { continue }
        results = append(results, ba)
    }
    // Sort by SortOrder ascending
    sort.Slice(results, func(i, j int) bool {
        return results[i].SortOrder < results[j].SortOrder
    })
    return results, nil
}
```

#### TypeScript Interface Changes (frontend)

```typescript
// Replace single author string with structured array
export interface BookAuthorEntry {
  author_id: number;
  name: string;       // denormalized for display
  role: string;       // "primary" | "co_author" | "editor" | "translator"
  sort_order: number;
}

export interface BookNarratorEntry {
  narrator_id: number;
  name: string;
  sort_order: number;
}

export interface Audiobook {
  // ... existing fields ...
  // Replace: author?: string;
  authors: BookAuthorEntry[];
  // Replace: narrator?: string;
  narrators: BookNarratorEntry[];
}

// Display helper — renders chip list or comma-separated depending on context
export function formatAuthors(authors: BookAuthorEntry[]): string {
  return authors
    .sort((a, b) => a.sort_order - b.sort_order)
    .map(a => a.name)
    .join(', ');
}
```

---

## Metadata Quality (P1)

Enhance Tags/Compare views in Book Detail:

- Show raw embedded tags and media info (bitrate, codec, sample rate)
- Display provenance per field (DB/edited, fetched, file tag)
- Expand Edit Metadata dialog with full fields (genre, ISBN, description,
  language)
- Backend support is merged (PR #79); frontend implementation is partial

### What `GET /api/v1/audiobooks/:id/tags` currently returns

Handler: `getAudiobookTags` in `internal/server/server.go` (line 1222).

```json
{
  "media_info": {
    "codec": "aac",
    "bitrate": 128,
    "sample_rate": 44100,
    "channels": 2,
    "bit_depth": null,
    "quality": "128kbps AAC",
    "duration": 42300
  },
  "tags": {
    "title": {
      "file_value": "Chapter 1",
      "fetched_value": "The Great Gatsby",
      "stored_value": "The Great Gatsby",
      "override_value": null,
      "override_locked": false,
      "effective_value": "The Great Gatsby",
      "effective_source": "fetched",
      "updated_at": "2026-01-30T12:00:00Z"
    },
    "author": { /* same shape */ },
    "series": { /* same shape */ }
  }
}
```

### What it needs to return (extended)

Add `genre`, `isbn`, `description`, `language`, `publisher`, `narrator`, and
`release_group` to the `tags` map. Each uses the same `MetadataProvenanceEntry`
shape. The function `buildMetadataProvenance` (line 289) already constructs
these entries — it just needs to emit the additional fields:

```go
// In buildMetadataProvenance, after existing field mappings, add:

provenance["genre"] = database.MetadataProvenanceEntry{
    FileValue:      nonEmptyOrNil(meta.Genre),
    StoredValue:    nonEmptyOrNil(stringOrEmpty(book.Genre)),
    EffectiveValue: effectiveField(book, state, "genre", meta.Genre),
    EffectiveSource: effectiveSource(state, "genre"),
}
provenance["isbn"] = database.MetadataProvenanceEntry{
    FileValue:      nonEmptyOrNil(meta.ISBN13),
    StoredValue:    nonEmptyOrNil(stringOrEmpty(book.ISBN13)),
    EffectiveValue: effectiveField(book, state, "isbn", meta.ISBN13),
    EffectiveSource: effectiveSource(state, "isbn"),
}
provenance["language"] = database.MetadataProvenanceEntry{
    FileValue:      nonEmptyOrNil(meta.Language),
    StoredValue:    nonEmptyOrNil(stringOrEmpty(book.Language)),
    EffectiveValue: effectiveField(book, state, "language", meta.Language),
    EffectiveSource: effectiveSource(state, "language"),
}
provenance["publisher"] = database.MetadataProvenanceEntry{
    FileValue:      nonEmptyOrNil(meta.Publisher),
    StoredValue:    nonEmptyOrNil(stringOrEmpty(book.Publisher)),
    EffectiveValue: effectiveField(book, state, "publisher", meta.Publisher),
    EffectiveSource: effectiveSource(state, "publisher"),
}
provenance["narrator"] = database.MetadataProvenanceEntry{
    FileValue:      nonEmptyOrNil(meta.Narrator),
    StoredValue:    nonEmptyOrNil(stringOrEmpty(book.Narrator)),
    EffectiveValue: effectiveField(book, state, "narrator", meta.Narrator),
    EffectiveSource: effectiveSource(state, "narrator"),
}
provenance["release_group"] = database.MetadataProvenanceEntry{
    FileValue:      nonEmptyOrNil(extractReleaseGroup(meta)),
    StoredValue:    nonEmptyOrNil(stringOrEmpty(book.ReleaseGroup)),
    EffectiveValue: effectiveField(book, state, "release_group", extractReleaseGroup(meta)),
    EffectiveSource: effectiveSource(state, "release_group"),
}
```

Helper:

```go
func nonEmptyOrNil(s string) interface{} {
    if strings.TrimSpace(s) == "" { return nil }
    return s
}
```

### React Component Changes — Expanded Edit Dialog

The Edit Metadata dialog needs new fields. Each field follows the same
pattern: a text input bound to the override value, a lock toggle, and a
"reset to fetched" link. Pseudocode for the new fields section:

```tsx
// MetadataEditDialog.tsx — add after existing title/author/series inputs

const expandedFields: ExpandedField[] = [
  { key: "genre",         label: "Genre",         placeholder: "Fiction / Mystery" },
  { key: "isbn",          label: "ISBN",          placeholder: "978-0-7432-7356-5" },
  { key: "language",      label: "Language",      placeholder: "en" },
  { key: "publisher",     label: "Publisher",     placeholder: "Scribner" },
  { key: "narrator",      label: "Narrator",      placeholder: "Kevin Spacey" },
  { key: "release_group", label: "Release Group", placeholder: "[PZG]" },
];

return (
  <>
    {/* ... existing title/author/series fields ... */}
    <Divider />
    <Typography variant="subtitle2">Extended Metadata</Typography>
    {expandedFields.map(field => (
      <ProvenanceFieldEditor
        key={field.key}
        fieldKey={field.key}
        label={field.label}
        placeholder={field.placeholder}
        provenance={tags?.tags?.[field.key]}
        value={overrides[field.key] ?? provenance?.effective_value ?? ""}
        locked={overrides[field.key]?.locked ?? false}
        onChange={(val) => setOverride(field.key, val)}
        onToggleLock={(locked) => setLock(field.key, locked)}
        onReset={() => clearOverride(field.key)}
      />
    ))}
  </>
);
```

---

## Future: Provenance & Release Group Tracking

### Release Group Support

Track which group released the audiobook for quality assessment:

- Add `release_group` field (e.g., `[PZG]`, `[AudioBook Bay]`)
- Parse from filename patterns: `[GroupName]`, `{GroupName}`, `-GroupName-`
- Display in Book Detail and Library views
- Filter by release group in search
- Stats: audiobooks per release group

#### Regex Patterns and Go Code

Release groups appear in filenames in three common patterns. The existing
`looksLikeReleaseGroupTag` function in `internal/metadata/metadata.go` (line
481) already detects `[...]` bracketed values and skips them during tag
normalization. Build on that detection to *extract* the group name:

```go
import "regexp"

var (
    // Matches [GroupName] anywhere in the string. Group 1 = name.
    bracketGroupRe = regexp.MustCompile(`\[([A-Za-z0-9_\-\.]+)\]`)

    // Matches {GroupName} anywhere in the string. Group 1 = name.
    braceGroupRe = regexp.MustCompile(`\{([A-Za-z0-9_\-\.]+)\}`)

    // Matches -GroupName at the end of the string (after the last space or dash).
    // Group 1 = name. Requires at least 2 chars to avoid matching "-1".
    trailingDashGroupRe = regexp.MustCompile(`[-_]([A-Za-z][A-Za-z0-9_]{1,30})$`)
)

// ExtractReleaseGroup extracts a release-group tag from a filename or path.
// Returns empty string if no group tag is detected.
func ExtractReleaseGroup(filename string) string {
    base := filepath.Base(filename)
    // Remove extension
    base = strings.TrimSuffix(base, filepath.Ext(base))

    // Priority 1: [BracketGroup]
    if m := bracketGroupRe.FindStringSubmatch(base); len(m) > 1 {
        return m[1]
    }
    // Priority 2: {BraceGroup}
    if m := braceGroupRe.FindStringSubmatch(base); len(m) > 1 {
        return m[1]
    }
    // Priority 3: -TrailingGroup (only if preceded by a space, indicating separation)
    // e.g. "Author - Title - 2020 -PZG.mp3"
    if m := trailingDashGroupRe.FindStringSubmatch(base); len(m) > 1 {
        candidate := m[1]
        // Reject if it looks like a year or number
        if _, err := strconv.Atoi(candidate); err != nil {
            return candidate
        }
    }
    return ""
}
```

#### Migration — add release_group to Book

```go
// In migration014Up (or a separate migration015Up if junction tables are separate):
// SQLite:
"ALTER TABLE books ADD COLUMN release_group TEXT"
// PebbleDB: field is added to the Book JSON struct — no key-level change needed.
// Add to database.Book:
//   ReleaseGroup *string `json:"release_group,omitempty"`
```

#### Integration point

In `scanner.go` `ProcessBooksParallel`, after metadata extraction, call:

```go
if rg := metadata.ExtractReleaseGroup(books[idx].FilePath); rg != "" {
    books[idx].ReleaseGroup = rg
}
```

Then persist via the existing `saveBookToDatabase` path (add `ReleaseGroup`
field to the `dbBook` construction block).

### Original Filename Preservation

- Add `original_filename` field (store complete name before organization)
- Display in Book Detail → Files tab
- Use for duplicate detection (same original filename = likely duplicate)

---

## Future: AI & Metadata Enhancements

- Confidence explanation tooltips for AI parsing results
- Batch AI parse queue for newly imported unparsed files
- Metadata merge policy editor (prefer source A unless missing field)
- Automatic language detection from text samples

### Batch AI Parse Queue — Integration with Operation Queue

The existing AI fallback in `scanner.go` (line 286) calls `aiParser.ParseFilename`
inline per-file. For a *batch* queue of unparsed books (e.g., after a bulk
import where AI was disabled or rate-limited), the work should be submitted to
`operations.GlobalQueue` as a single long-running operation:

```go
// internal/server/server.go — new handler, registered as:
//   api.POST("/operations/ai-parse-batch", s.startAIBatchParse)

func (s *Server) startAIBatchParse(c *gin.Context) {
    if operations.GlobalQueue == nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
        return
    }
    if !config.AppConfig.EnableAIParsing || config.AppConfig.OpenAIAPIKey == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not configured"})
        return
    }

    id := ulid.Make().String()
    _, err := database.GlobalStore.CreateOperation(id, "ai_batch_parse", nil)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    err = operations.GlobalQueue.Enqueue(id, "ai_batch_parse", operations.PriorityLow,
        func(ctx context.Context, progress operations.ProgressReporter) error {
            parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, true)
            if parser == nil || !parser.IsEnabled() {
                return fmt.Errorf("AI parser unavailable")
            }

            // Find books that were scanned with filename fallback or missing fields
            // Use a scan over all books; filter to those needing AI help.
            books, err := database.GlobalStore.GetAllBooks(100_000, 0)
            if err != nil {
                return fmt.Errorf("failed to list books: %w", err)
            }

            var needsParsing []database.Book
            for _, b := range books {
                // Heuristic: if title equals the bare filename (no extension),
                // it was never properly parsed.
                bare := strings.TrimSuffix(filepath.Base(b.FilePath), filepath.Ext(b.FilePath))
                if b.Title == bare || b.AuthorID == nil {
                    needsParsing = append(needsParsing, b)
                }
            }

            total := len(needsParsing)
            _ = progress.UpdateProgress(0, total, fmt.Sprintf("Found %d books needing AI parse", total))

            for i, book := range needsParsing {
                if progress.IsCanceled() {
                    return fmt.Errorf("ai_batch_parse canceled")
                }

                filename := filepath.Base(book.FilePath)
                aiCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
                aiMeta, aiErr := parser.ParseFilename(aiCtx, filename)
                cancel()

                if aiErr != nil {
                    _ = progress.Log("warn", fmt.Sprintf("AI parse failed for %s: %v", filename, aiErr), nil)
                    continue
                }
                if aiMeta == nil {
                    continue
                }

                // Apply AI results only to empty fields
                updated := false
                bare := strings.TrimSuffix(filepath.Base(book.FilePath), filepath.Ext(book.FilePath))
                if (book.Title == bare || book.Title == "") && aiMeta.Title != "" {
                    book.Title = aiMeta.Title
                    updated = true
                }
                if book.AuthorID == nil && aiMeta.Author != "" {
                    author, _ := database.GlobalStore.GetAuthorByName(aiMeta.Author)
                    if author == nil {
                        author, _ = database.GlobalStore.CreateAuthor(aiMeta.Author)
                    }
                    if author != nil {
                        book.AuthorID = &author.ID
                        updated = true
                    }
                }
                if (book.Narrator == nil || *book.Narrator == "") && aiMeta.Narrator != "" {
                    book.Narrator = &aiMeta.Narrator
                    updated = true
                }

                if updated {
                    if _, err := database.GlobalStore.UpdateBook(book.ID, &book); err != nil {
                        _ = progress.Log("error", fmt.Sprintf("Failed to update %s: %v", book.ID, err), nil)
                    }
                }

                _ = progress.UpdateProgress(i+1, total, fmt.Sprintf("Processed %d/%d", i+1, total))
            }
            return nil
        },
    )
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusAccepted, gin.H{"operation_id": id, "type": "ai_batch_parse"})
}
```

Rate-limiting note: OpenAI has per-minute token limits. If the batch is large
(>50 books), add a `time.Sleep(200 * time.Millisecond)` between calls inside
the loop to stay under default RPM limits.

---

## Dependencies

- Multiple authors/narrators requires a data model decision before
  implementation
- Release group parsing can proceed independently
- AI enhancements depend on OpenAI integration being stable

## References

- Audiobook model: `internal/database/store.go` (Book struct, line ~161)
- Open Library client: `internal/metadata/openlibrary.go`
- Provenance: `internal/database/pebble_store.go` (metadata_state keys)
- Tags handler: `internal/server/server.go` → `getAudiobookTags` (line ~1222)
- Fetch handler: `internal/server/server.go` → `fetchAudiobookMetadata` (line ~3553)
- Operation queue: `internal/operations/queue.go`
