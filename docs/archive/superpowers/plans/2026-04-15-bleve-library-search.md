# Bleve Library Search — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Spec:** `docs/superpowers/specs/2026-04-11-bleve-library-search.md` (v1.1)
**Depends on:** Nothing. Can run in parallel with 4.4 DI.
**Unblocks:** 3.4 Playlists (smart playlist query engine uses Bleve).

---

### Task 1: Bleve index package + document model (1 PR)

**Files:**
- Create: `internal/search/bleve_index.go` — `BleveIndex` struct with `Open`, `Close`, `IndexBook`, `DeleteBook`, `Search`
- Create: `internal/search/document.go` — `BookDocument` struct (maps Book + Author + Series + Tags into a flat indexable doc)
- Create: `internal/search/bleve_index_test.go`
- Modify: `go.mod` — add `github.com/blevesearch/bleve/v2`

- [ ] Define `BookDocument` with fields: title, author, narrator, series, series_number, genre, year, language, publisher, description, format, duration, bitrate, tags ([]string), isbn, asin, library_state, has_cover (bool), file_path
- [ ] Custom analyzers: English stemmer + ASCII folding + lowercasing for text fields; keyword analyzer for format/genre/language/library_state
- [ ] Field boosts: title=3, author=2, series=1.5, narrator=1.2, description=0.5
- [ ] `IndexBook(book, author, series, tags)` builds a `BookDocument` and indexes it
- [ ] `DeleteBook(bookID)` removes from index
- [ ] `Search(queryString, limit, offset)` → `[]SearchResult{BookID, Score, Highlights}`
- [ ] Test: index 100 synthetic books, search by title/author/series, verify scoring order

---

### Task 2: DSL parser extension — new operators (1 PR)

**Files:**
- Modify: `web/src/utils/searchParser.ts` — add `&&`, `||`, `()`, `field:(a|b|c)`, `*`, `~`, `^N`, consistent `field:<op>value` for numeric
- Create: `web/src/utils/searchParser.test.ts` — comprehensive tests for new syntax
- Create: `internal/search/query_parser.go` — Go-side parser (mirrors the TS parser for server-side evaluation)
- Create: `internal/search/query_parser_test.go`

- [ ] Tokenizer: recognize `&&`, `||`, `(`, `)`, `|` inside field-value parens, `*`, `~`, `^N`, `:>`, `:<`, `:>=`, `:<=`
- [ ] AST nodes: `AndNode`, `OrNode`, `NotNode`, `FieldLeaf`, `GroupNode`, `ValueAltNode`
- [ ] Parse precedence: `NOT` binds tightest, then `AND`, then `OR` (standard)
- [ ] Backward compat: existing queries without new operators produce identical AST
- [ ] Tests: all worked examples from the spec + edge cases (empty, single-term, nested groups)

---

### Task 3: AST → Bleve query translator (1 PR)

**Files:**
- Create: `internal/search/bleve_translator.go` — `TranslateAST(ast) (query.Query, []PerUserFilter, error)`
- Create: `internal/search/bleve_translator_test.go`

- [ ] Walk AST nodes:
  - `AndNode` → `ConjunctionQuery`
  - `OrNode` → `DisjunctionQuery`
  - `NotNode` → `BooleanQuery{MustNot}`
  - `FieldLeaf` (text) → `MatchQuery` (default), `MatchPhraseQuery` (quoted), `PrefixQuery` (trailing `*`), `WildcardQuery` (embedded `*`), `FuzzyQuery` (`~`)
  - `FieldLeaf` (numeric) → `NumericRangeQuery`
  - `ValueAltNode` → `DisjunctionQuery` of `MatchQuery`s
  - Per-user fields (`read_status`, `progress_pct`, `last_played`) → NOT sent to Bleve; collected into `[]PerUserFilter` for Go post-filter
- [ ] Boost propagation via `^N` on any leaf
- [ ] Test: translate worked examples, verify Bleve query structure

---

### Task 4: Index lifecycle — build, update, startup (1 PR)

**Files:**
- Modify: `internal/server/server.go` — init Bleve index at startup, close on shutdown
- Create: `internal/search/index_updater.go` — hooks into book create/update/delete to keep index live
- Modify: `internal/server/audiobooks_handlers.go` — call `IndexBook` after create/update, `DeleteBook` after delete

- [ ] First startup: if index dir doesn't exist, trigger background full build (tracked operation `bleve_index_build`)
- [ ] Full build: iterate all books, index each with author/series/tag enrichment
- [ ] Incremental: after any book mutation, call `IndexBook` or `DeleteBook`
- [ ] Shutdown: close index cleanly
- [ ] Test: create book → search finds it; update title → search reflects new title; delete → search doesn't find it

---

### Task 5: Replace search endpoint (1 PR)

**Files:**
- Modify: `internal/server/audiobook_service.go` — replace `SearchBooks` call with Bleve search path
- Modify: `internal/server/audiobooks_handlers.go` — wire new search into `listAudiobooks` when a search query is present
- Create: `internal/search/post_filter.go` — Go-side post-filter for per-user fields

- [ ] If search query present → parse DSL → translate → Bleve search → per-user post-filter → return
- [ ] If no search query → existing PebbleDB paginated scan (unchanged)
- [ ] Highlights included in response for UI rendering
- [ ] Per-user post-filter: if AST contains `read_status` / `progress_pct` / `last_played`, load user state for candidate book IDs and filter in Go
- [ ] Test: e2e search via httptest, verify results + highlights

---

### Task 6: Frontend — search bar typeahead + highlighting (1 PR)

**Files:**
- Modify: `web/src/pages/Library.tsx` — auto-append `*` to last token during live typing (debounced)
- Modify: `web/src/components/audiobooks/AudiobookCard.tsx` or table row — render highlights from search response
- Modify: `web/src/utils/searchParser.ts` — sync with Go parser for consistent client-side validation

- [ ] Debounced search-as-you-type (existing), but now auto-appends `*` to last token
- [ ] Display highlighted fragments (e.g., "Author: **Isaac** Asimov") when present
- [ ] Error state: show inline syntax error from parser when query is malformed

---

### Task 7: Cleanup — remove old FTS5 + LIKE path (1 PR)

**Files:**
- Modify: `internal/database/sqlite_store.go` — remove `SearchBooks` FTS5 + LIKE + UNION implementation
- Modify: `internal/database/pebble_store.go` — remove `SearchBooks` prefix-scan implementation
- Remove: `fuzzyRankBooks` / `matcher.ScoreMatch` Go-side re-ranker (if no other callers)

- [ ] Verify no caller uses the old `SearchBooks` path
- [ ] Remove dead code
- [ ] Migration: drop `books_fts` FTS5 virtual table if SQLite is still in use

---

### Estimated effort

| Task | Size | Depends on |
|---|---|---|
| 1 (index + docs) | M | — |
| 2 (parser) | M | — |
| 3 (translator) | M | 1+2 |
| 4 (lifecycle) | M | 1 |
| 5 (search endpoint) | M | 3+4 |
| 6 (frontend) | S | 5 |
| 7 (cleanup) | S | 5 |
| **Total** | ~7 PRs, L overall | |

### Perf validation

After task 5, benchmark search latency at production scale (24K books):
- Target: <10ms for term match, <50ms for fuzzy, <100ms total typeahead budget
- Compare against old FTS5 path to confirm improvement
- Profile with pprof if any query type exceeds targets
