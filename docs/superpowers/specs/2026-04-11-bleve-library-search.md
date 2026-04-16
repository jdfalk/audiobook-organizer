<!-- file: docs/superpowers/specs/2026-04-11-bleve-library-search.md -->
<!-- version: 1.1.0 -->
<!-- guid: d2e3f4a5-b6c7-8901-2345-67890abcdef1 -->
<!-- last-edited: 2026-04-15 -->

# bleve Library Search Design

**Status:** Design spec — not yet implemented. v1.1 folds in decisions from the 2026-04-15 brainstorm (DSL integration, query usage map, new operators, default match mode, perf targets, per-user indexing future-work).
**Owner:** TBD.
**Parent tasks:** TODO.md DES-1 (primary); pairs with 3.4 playlists (shares the DSL) and 3.6 read/unread tracking (Go-side post-filter for per-user fields).
**Depends on:** Nothing. Standalone replacement for the current SQLite FTS5 + LIKE + Go-rerank search path.

## v1.1 summary of additions (2026-04-15)

Bleve becomes **THE engine** driving the library search bar and smart-playlist query evaluation — not a side-index used only for free-text. Our locked DSL (from §3.4 design) compiles to Bleve `query.Query` objects so one engine serves everything user-facing. New sections added below:

- **§ Integration with our query DSL** — translator from our AST to Bleve programmatic queries
- **§ Where Bleve drives behavior** — usage map, explicit "where Bleve does NOT run"
- **§ Syntax comparison table** — our DSL vs Bleve native string syntax, differences called out
- **§ Default match mode change** — substring → tokenized + stemmed, plus the UI auto-append-`*` workaround to preserve typeahead UX
- **§ New operators added to our DSL** — `*` (prefix/wildcard), `~` (fuzzy), `^N` (boost), consistent `field:<op>value` placement (e.g. `year:>2022`, not `year>2022`)
- **§ Perf targets** — concrete per-query-type timings at 24K-book scale, typeahead budget
- **§ Per-user indexing (deferred)** — shared-library index + Go post-filter for per-user fields (`read_status`, `progress_pct`, `last_played`); shard model documented as future work if scale demands it

The v1.0 document model, field boosts, index lifecycle, concurrency model, migration phases, and risks all stand.

## Integration with our query DSL

Our DSL (defined in the 3.4 playlists spec) is the user-facing query language. Bleve is the backend evaluator. Users don't see Bleve's native syntax — it's an implementation detail.

Translation pipeline:

```
User input ──▶ DSL parser ──▶ AST ──▶ translator ──▶ Bleve query.Query ──▶ Bleve index
                               │
                               └─▶ Go post-filter pass for per-user fields
```

Translator rules:

| Our AST node | Bleve query |
|---|---|
| `AND` | `ConjunctionQuery` |
| `OR` | `DisjunctionQuery` |
| `NOT` | `BooleanQuery{MustNot: [...]}` |
| Field leaf (single term) | `MatchQuery{Field, Term}` (tokenized + analyzed) |
| Field leaf (quoted, multi-word) | `MatchPhraseQuery{Field, Text}` |
| Field leaf with `*` wildcard | `WildcardQuery` (or `PrefixQuery` for trailing `*`) |
| Field leaf with `~` fuzzy | `FuzzyQuery{Field, Term, Fuzziness}` |
| Field leaf with `^N` boost | any above, with `boost` parameter |
| Within-field alt `field:(a\|b\|c)` | `DisjunctionQuery` of `MatchQuery`s |
| Numeric `field:>N` | `NumericRangeQuery{Min: N, Min: false}` (exclusive) |
| Numeric `field:>=N` | `NumericRangeQuery{Min: N, Min: true}` (inclusive) |
| Numeric `field:<N` / `field:<=N` | `NumericRangeQuery{Max: N, ...}` |
| Numeric `field:[A TO B]` | `NumericRangeQuery{Min: A, Max: B, both inclusive}` |
| Date range | `DateRangeQuery` |
| Per-user field (`read_status`, `progress_pct`, `last_played`) | **Not sent to Bleve.** Added to the Go post-filter list. Bleve returns a larger candidate set; Go narrows to matches for the calling user. |

Per-user post-filter:

1. Bleve evaluates the index-backed portion, returns scored candidate book IDs
2. Go pass applies per-user filters using the calling user's state from the `user_book_state` / `user_position` PebbleDB keys
3. Intersection returned, preserving Bleve's score order so relevance is honored

## Where Bleve drives behavior

| Surface | Uses Bleve | Notes |
|---|---|---|
| Library search bar (free-text + field:value) | **Yes** | Primary use. TF-IDF scoring, stemmed match, highlights. |
| Smart playlist query evaluation | **Yes** | Shares DSL + engine with search bar. Results preserve Bleve score ordering. |
| Quick filter autocomplete (typeahead on authors, series, tags) | **Yes** | `PrefixQuery` on indexed fields. Search bar auto-appends `*` to the last typed token. |
| "Similar books" on BookDetail (future 5.2) | **Yes** | Bleve's `more-like-this` / `SimilarQuery` fits naturally. |
| Book dedup candidate matching | No | Separate engine (embedding + heuristic) already in place. Different problem — similarity, not retrieval. |
| BookDetail direct lookup by book ID | No | PebbleDB point lookup is 10-20× faster than routing through Bleve. |
| Full-library paginated list (no search term, page 47 of everything) | No | PebbleDB prefix scan with cursor is the right tool; scoring irrelevant. |
| Activity log search | Later | Existing FTS works; migrate post-MVP if Bleve replaces the shared search infra. |
| Per-user filter fields (`read_status`, `progress_pct`, `last_played`) | No | Go post-filter over the current user's state; see integration section above. |

## Syntax comparison

Users write our DSL. Bleve syntax shown for reference (useful if we ever expose a "raw Bleve" advanced mode).

| Operation | Our DSL | Bleve native string | Notes |
|---|---|---|---|
| AND | space, `&&`, `AND` | space, `AND` | Bleve has no `&&` — our parser accepts both |
| OR | `OR`, `||` (double pipe) | `OR` only | Bleve has no `||` — our parser accepts both |
| NOT | `-`, `NOT` | `-`, `NOT` | Same |
| Within-field alt | `title:(a|b|c)` | `title:(a OR b OR c)` | Our `|` is sugar; translates to `DisjunctionQuery` of `MatchQuery`s |
| Range open | `year:>2000` | `year:>2000` | Colon before comparator on both sides |
| Range closed (inclusive) | `year:[2000 TO 2010]` | `year:[2000 TO 2010]` | Same |
| Prefix | `title:vamp*` | `title:vamp*` | Same |
| Wildcard | `title:*vamp*` | `title:*vamp*` | Same; slower than prefix on both sides |
| Fuzzy | `author:smith~` | `author:smith~` | Same |
| Boost | `title:vampire^3` | `title:vampire^3` | Same |
| Exact (reserved, future) | `title:=value` | (use Term query programmatically) | Bleve's string DSL has no exact operator — reserved in ours for later |
| Phrase | `title:"New Dawn"` | `title:"New Dawn"` | Same |

## Default match mode change (semantics)

v1.0 design implied field matches were tokenized. Making it explicit here as a locked-in semantic choice in v1.1.

| Query | Before (substring-anywhere, current SQLite LIKE) | After (tokenized + stemmed, Bleve) |
|---|---|---|
| `title:vampire` | matches "vampirically-inclined", "revampire" | matches "vampire", "vampires", "vampiric" (stemmer expands) |
| `title:vamp` | matches "The Vampire Diaries" | does NOT match (needs `vamp*`) |
| `author:mart` | matches "Martin", "Smart" | does NOT match (needs `mart*`) |

Tokenized + stemmed is better for audiobook discovery: noise matches ("vampirically") disappear, morphology ("vampires", "vampiric") is captured. Trade-off: partial-word queries need an explicit `*`.

**Typeahead workaround:** the library search bar UI auto-appends `*` to the last typed token in the free-text portion. `vamp` → `vamp*` behind the scenes, so users typing `vamp` still see "The Vampire Diaries" in live suggestions. Explicit queries (smart playlist definitions) receive no auto-append — if a user writes `title:vamp` in a saved query, they meant the whole word.

## New operators added to our DSL

Added in v1.1 to match Bleve's core capabilities. Consistent placement: always `field:<op><value>` or `field:<value><op>` — the colon sits between field and the value-expression, operators live inside the value-expression.

| Operator | Syntax | Meaning | Example |
|---|---|---|---|
| Prefix / suffix / substring wildcard | `*` | Star matches zero-or-more chars | `title:vamp*`, `title:*vamp`, `title:*vamp*` |
| Fuzzy | `~` | Edit-distance match (default distance 2) | `author:smith~` matches Smyth, Smiht |
| Boost | `^N` | Multiply relevance score by N for this clause | `title:vampire^3` |
| Numeric comparators | `:>`, `:<`, `:>=`, `:<=` | Inline with `field:` prefix (consistent style) | `year:>2022`, `duration:<3600` |
| Numeric range | `:[A TO B]` | Inclusive range (reserved; MVP unfolds to `&&`) | `year:[2000 TO 2010]` |
| Exact (reserved) | `:=` | Exact match, no tokenization | `title:="Twilight"` (future; MVP approximates) |

**Consistency rule:** every value-side operator goes inside the `field:` expression. No more `year>2022` — it's `year:>2022`. Same everywhere.

## Perf targets (at 24K-book scale)

Ballpark numbers on a modest machine; validate in phase 2 of the migration plan.

| Operation | Bleve | Current (FTS5 + LIKE + Go re-rank) | PebbleDB point lookup |
|---|---|---|---|
| Exact term match (`author:sanderson`) | 1-3 ms | 20-50 ms | — |
| Prefix (`title:vamp*`) | 2-10 ms | 100-500 ms (full scan via LIKE) | — |
| Fuzzy (`author:smith~`) | 10-50 ms | — (not supported) | — |
| Multi-field boolean (`a && b`) | 2-10 ms | 30-100 ms | — |
| Phrase (`"lord of the rings"`) | 1-5 ms | — (not supported as configured) | — |
| Direct get-book-by-ID | — (not the right tool) | — | 50-200 μs |
| Iterate all 24K books, no filter | — (not the right tool) | 200-500 ms | 100-300 ms (prefix scan) |

**Typeahead budget** (target <100 ms keystroke → suggestions rendered):

- Bleve prefix query: 2-10 ms
- HTTP round-trip: 10-30 ms (localhost) / 30-80 ms (LAN)
- React render: 20-40 ms
- **Total: 30-80 ms at 24K books. Comfortably inside the <100 ms target.**

**Caveats (honest):**

- **First-time index build** after upgrade: 10-30 seconds for 24K books. Runs in background, UI shows "indexing…" on first hit.
- **Per-document update**: sub-millisecond.
- **Disk**: ~20-30% of raw indexed text, so 50-100 MB for our library.
- **Memory**: Scorch backend, ~50-100 MB working set at current scale.
- **Cold start**: first query after process boot takes 100-500 ms to load index segments; warm after.

## Per-user indexing — deferred

MVP indexes shared library state only. Per-user fields (`read_status`, `progress_pct`, `last_played`) are **Go-side post-filters** applied to Bleve's candidate result set using the calling user's state from PebbleDB (`user_book_state`, `user_position`).

Why deferred:

- Current scale (dozens of users max per install) doesn't justify per-user indexes
- Post-filter on top of a scored candidate set is cheap — Bleve narrows from 24K to, say, 200; Go filters 200 → 50 in under a millisecond
- Per-user indexes would multiply disk + memory cost by N users and complicate index maintenance on every status change

When to revisit:

1. Hundreds of active users and post-filter becomes a hot path (visible in profiles)
2. Users want per-user smart playlists with complex per-user-field-first queries ("books I finished last month that nobody else finished") where post-filter over a large candidate set is actually slow
3. A different use case needs per-user full-text indexing (notes on books, per-user tags)

Options when the time comes:

- **Sharded index keyed on userID** — one Bleve index per user for per-user fields only; shared index remains for library state. Query fanout: union results.
- **Separate per-user small index** for status/progress, composed with shared library index at query time.
- **Materialize per-user views** — periodically snapshot (user × book) state into a join-friendly structure.

No code changes needed now to keep this door open; Go post-filter is just a function call that can be swapped for an index lookup later.

## Problem statement

Current library search is stitched together from three separate
mechanisms that each catch a different slice of what users actually
want. The result is correct-ish but lossy and slow to extend.

### Current implementation

Call site: `SQLiteStore.SearchBooks` in
`internal/database/sqlite_store.go:2617`.

Three UNION branches per query:

1. **Title FTS5** against an SQLite virtual table `books_fts` built by
   migration 017. Index is single-column: only the title.
2. **Author LIKE** against the `authors` table via `books.author_id`.
   Wildcard `%query%` — scans the whole authors table.
3. **Co-author LIKE** against the `book_authors` junction for secondary
   authors/narrators.

Results from all three branches are unioned and re-ranked in Go by
`fuzzyRankBooks`, which calls `matcher.ScoreMatch` (our own Levenshtein /
fuzzy scoring). The top-N after re-ranking is returned.

### What breaks with this

- **Only titles are full-text indexed.** Searching for a publisher,
  narrator, or series name doesn't use the FTS path at all — it either
  misses entirely or falls through to LIKE on the authors table (which
  can't find it anyway because it's on the wrong column).
- **No relevance scoring on author matches.** LIKE is binary: either the
  substring matches or it doesn't. Two authors named "Isaac Asimov" and
  "Isaac Newton" rank identically for the query "isaac". The Go-side
  fuzzy re-rank catches some of this but runs after the SQL has already
  decided which books to return, so any book whose author SQL missed is
  gone.
- **No highlighting.** The UI can't show "here's the matching part" to
  explain why a result was returned.
- **No faceting.** Filtering by genre + language + format has to be
  composed as SQL predicates on top of the search query, and we re-rank
  in Go after the SQL returns, which means faceting and relevance can't
  interact.
- **No stemming, no synonyms, no phrase queries.** "running" won't find
  "runs" or "ran". "Lord of the Rings" can't be scored as a phrase;
  only individual tokens.
- **Fuzzy re-rank in Go is an artifact of the bad SQL.** We pull more
  rows than we need, re-score, throw half away. The "re-rank pool" is
  literally `limit × 3` to compensate for SQL returning imprecise
  candidates. This is ~3× wasted work per query.

### What we'd gain from bleve

bleve is a pure-Go, embedded full-text search library with proven
production use (Couchbase, etcd, several Go CMSes). Key properties
relevant to us:

- **Multi-field document indexing.** We index title, author, narrator,
  series, description, publisher, tags, even filepath — all in one
  document with per-field analyzers (stemming, lowercasing,
  ASCII-folding).
- **Relevance scoring out of the box.** bleve's default scoring is
  TF-IDF with field-level boosts. A title-hit scores higher than a
  description-hit for the same term.
- **Phrase queries, fuzzy queries, prefix queries, wildcard queries**
  — all first-class. We stop re-implementing matcher.ScoreMatch.
- **Highlighting.** The response includes the matching fragment per
  field, which the UI can render as "Author: **Isaac** Asimov".
- **Faceting.** Per-field facet results come back with the query:
  "25 books by author=Asimov, 18 books with genre=Science Fiction,
  12 books in language=en". The UI can use these to power drill-downs
  without re-issuing the query.
- **Per-field analyzers.** The title field can be stemmed + lowercased;
  the filepath field can be raw; the narrator field can be a simple
  tokenizer. Mix and match.
- **Suggest / autocomplete** via the `fuzzy` query type — a
  three-character query returns useful results.

## Non-goals

- Replacing SQLite as the source of truth for book data. bleve is an
  index, not a store. Every document in bleve is denormalized from the
  authoritative Book row.
- Replacing the embedding-based dedup similarity search. Different
  shape, different problem — see the chromem-go spec in the same
  folder. bleve does token-level search, not semantic.
- Full-text search of book *content*. We only index metadata — title,
  author, narrator, series, tags, description. Actual audio content
  is not indexed (and wouldn't fit in bleve anyway).
- Replacing the advanced filter system (range queries on duration,
  year, bitrate, etc.). Those stay as SQL predicates for now.

## Architecture

### New package

`internal/search/bleve_index.go` — owns the bleve index handle, exposes
typed query / indexing methods. Single-process, one writer, many
readers. The index is a directory of files under the main data dir,
parallel to PebbleDB and the SQLite sidecars.

```
data/
├── audiobooks.pebble/      ← Pebble primary store
├── activity.db             ← SQLite activity log
├── embeddings.db           ← SQLite dedup candidates (eventually chromem for vectors)
└── search.bleve/           ← NEW: bleve library search index
```

### Document model

One bleve `Document` per primary Book. Non-primary version-group
members are NOT indexed separately — they inherit the primary's index
entry. Users searching for "Foundation" should find the version group,
not each of the 5 format-siblings.

```go
type BookSearchDoc struct {
    ID              string   // primary book ULID
    Title           string   // analyzer: english (stemmed, lowercased)
    Subtitle        string   // analyzer: english
    Author          string   // analyzer: simple (tokenized, lowercased, no stem)
    Narrator        string   // analyzer: simple
    Series          string   // analyzer: simple
    SeriesNumber    float64  // numeric
    Publisher       string   // analyzer: simple
    Description     string   // analyzer: english
    Genre           []string // analyzer: keyword (exact match only)
    Language        string   // analyzer: keyword
    Format          string   // analyzer: keyword
    Tags            []string // analyzer: keyword — user tags
    ISBN10          string   // analyzer: keyword
    ISBN13          string   // analyzer: keyword
    ASIN            string   // analyzer: keyword
    YearPublished   float64  // numeric
    Duration        float64  // numeric (seconds)
    AddedAt         string   // datetime
    FilePath        string   // analyzer: path (slash-tokenized)
}
```

Per-field analyzers matter. Title gets the English analyzer (stemming,
stopwords) so "running" matches "ran" and "of", "the" get dropped.
Author / narrator get a simple analyzer (tokenize on whitespace,
lowercase, no stem) because you don't want "Asimov" to stem to "Asimo".
Genre / language / format are keyword-only (exact match, no tokenizing)
so faceting and equality filters work correctly.

### Field boosts

When searching, boosts encode "where does the user usually look":

- `Title`     ×5  — title hits matter most
- `Series`    ×3
- `Author`    ×3
- `Narrator`  ×2
- `Tags`      ×2
- `Description` ×1
- All others ×1

Boosts are query-time, not index-time, so they can be tuned without
re-indexing. Start with these values; iterate based on user complaints.

### Index lifecycle

1. **Initial build.** On server start, check if `search.bleve/`
   exists. If not (first run after upgrade or after a rebuild), walk
   every primary book in Pebble and `Index` them. For 12K books this
   takes ~30-60 seconds (bleve docs: ~300-500 docs/sec on typical
   hardware). Runs in a `bgWG`-tracked goroutine so shutdown drains it
   before closing the underlying store.
2. **Incremental updates.** Every write path that touches user-visible
   fields calls `searchIndex.Upsert(book)`. Wired into:
   - `database.CreateBook` → via the new hook
   - `database.UpdateBook` → via the same hook
   - `MergeService.MergeBooks` → primary change, re-index the primary
   - Deletions → `searchIndex.Delete(bookID)`
   The hook follows the same pattern as `DedupOnImportHook` and
   `OrganizeCollisionHook`: package-level func var set by the server
   during startup, fire-and-forget with a bgWG-tracked goroutine.
3. **Periodic reconciliation.** A nightly maintenance task walks all
   primary books, compares the bleve `UpdatedAt` metadata against the
   Book `updated_at`, and re-indexes any drift. Catches anything the
   hooks missed (crash mid-write, shutdown drop, etc).
4. **Manual rebuild.** A maintenance endpoint
   `POST /api/v1/maintenance/rebuild-search-index` wipes and rebuilds
   from scratch. Useful after mass-imports or after tuning the analyzer
   config.

### Query path

Call site changes: `SQLiteStore.SearchBooks` becomes
`SearchService.Search(req SearchRequest) (SearchResponse, error)` —
the DB method stays as a fallback for when the bleve index isn't
available, gated on a config flag.

```go
type SearchRequest struct {
    Query        string            // raw user input — bleve QueryString syntax parsing
    Filters      map[string]any    // post-query filters (year range, duration range, etc.)
    Facets       []string          // which fields to return facets for
    Sort         string            // "relevance" (default), "title", "added_at", etc.
    Limit        int
    Offset       int
    Highlight    bool              // whether to return highlighted fragments
}

type SearchResponse struct {
    Hits       []SearchHit
    Total      uint64
    MaxScore   float64
    Facets     map[string]*FacetResult
    Took       time.Duration
}

type SearchHit struct {
    BookID      string
    Score       float64
    Fragments   map[string][]string // field -> highlighted fragments
}
```

The server's `listAudiobooks` endpoint grows a `?q=` parameter that
routes to `SearchService` when present. The existing sort/filter
plumbing stays for non-search browse.

### Concurrency

bleve supports concurrent reads and serializes writes internally. One
index handle is shared across the server. Writes happen from the hook
goroutines (bgWG-tracked); reads happen from HTTP request handlers.
No explicit locking needed on our side.

### Shutdown

`searchIndex.Close()` is called from the server's shutdown path after
`bgWG.Wait()` returns but before Pebble closes. This order matters —
the index hook callbacks might still be running when shutdown starts,
and we need to wait for them to finish indexing before closing the
bleve handle. The existing bgWG pattern handles this correctly.

## Migration plan

### Phase 1: add bleve alongside FTS5 (weeks 1-2)

- Add `github.com/blevesearch/bleve/v2` dependency.
- Implement `SearchService` wrapping a bleve index.
- Add config flag `SearchBackend` with values `"sqlite-fts5"` (default)
  and `"bleve"`.
- When `bleve` is selected: both FTS5 and bleve are populated (dual-write).
  Reads go to bleve. A canary compares top-10 results against the FTS5
  path at 1% sample rate and logs divergence.
- Indexing hook fires on CreateBook / UpdateBook / Delete (new
  `SearchIndexHook` package var).

### Phase 2: initial build (week 3)

- Maintenance endpoint `POST /api/v1/maintenance/rebuild-search-index`
  that walks every primary Book and indexes it.
- Idempotent: can be re-run after interruptions. Bleve's upsert is
  safe on re-index.
- Progress reported through the Operations queue (same pattern as the
  dedup scan — lessons from backlog 2.2 apply).

### Phase 3: cutover (week 4)

- Change default of `SearchBackend` to `"bleve"`.
- Keep FTS5 virtual table in place + dual-write for one more release
  as rollback insurance.
- Rollback is: flip the config flag back to `"sqlite-fts5"`, restart.
  No data loss.

### Phase 4: cleanup (month 2)

- Remove dual-write. Drop the FTS5 virtual table via a migration.
  Remove `SearchBooks` from the SQLite store. Remove `fuzzyRankBooks`
  and `matcher.ScoreMatch` from the codebase since bleve replaces them
  both.

## UX changes enabled by bleve

These are not strictly part of the implementation but unlock once
bleve lands:

- **Faceted browse**: "Search 'epic fantasy'" returns a result list
  plus sidebar facets "Authors (12): Sanderson (8), Tolkien (3), ...
  / Genre (4): Fantasy (18), Science Fiction (5), ...". UI can
  filter without re-querying.
- **Did-you-mean / suggest**: autocomplete as the user types, powered
  by prefix queries against the title + author fields.
- **Highlighted fragments**: result rows show the matching part in
  bold, giving users a clue why a result was returned.
- **Phrase queries**: quoted strings like `"lord of the rings"` match
  the phrase, not individual tokens.
- **Field-qualified search**: `author:asimov series:foundation` powered
  by bleve's QueryString syntax. Works today in the current UI textbox
  — no frontend changes required, bleve parses it.

## Benchmarks to run before committing

All against a snapshot of current production data (~12K primary books)
with a realistic query mix (20 queries drawn from actual user search
logs if we have them, otherwise hand-crafted: short single-token,
long phrase, author-only, misspelled, exact-match ISBN).

1. **Query latency p50/p95/p99** on each backend for the 20-query set.
2. **Cold-start latency** — time to serve first query after process
   start on each backend.
3. **Index build time** — full rebuild from scratch. bleve should be
   30-60s; FTS5 is currently ~5-10s but less feature-complete.
4. **Index disk size** — bleve is typically 2-3× the source text,
   FTS5 is ~1.5×. Measure both, document the tradeoff.
5. **Memory footprint** — resident memory with the index loaded at
   steady state.

Acceptance: bleve must beat FTS5 on **relevance quality** (measured by
manual inspection of top-10 for each of the 20 queries — does bleve
return more useful results?) AND be within 2× on latency. Relevance
is the whole point; latency is the constraint.

## Risks

### Index drift

If an UpdateBook write succeeds but the bleve upsert fails or the
goroutine dies, the index drifts from the DB. Users start seeing stale
titles in search results.

**Mitigation:** The nightly reconciliation walk catches drift. Also,
any divergence detected during the dual-write phase (Phase 1-3)
surfaces in logs. Add a metric `search_index_drift_count` that the
reconciler increments so we can alert on it.

### Disk usage

The bleve index adds 2-3× the text size on top of what we already
store. For 12K books × ~2 KB metadata each = ~50 MB of source text,
so ~100-150 MB of index. Trivial at current scale, worth measuring
at projected growth.

### Analyzer tuning

Default English analyzer is aggressive (strong stemming, large
stopword list). This may over-match. Example: "The Man Who Was
Thursday" stems "was" → dropped stopword, "thursday" → "thursdai",
"man" → "man". Query "thursday" still works. Query "the man" → both
tokens dropped → zero results.

**Mitigation:** Test with a query set including stopword-heavy titles
before committing. If English analyzer drops too aggressively, switch
to `standard` or build a custom analyzer with a shorter stopword list.

### bleve project maturity

bleve is mature (Couchbase, etcd, several years of production use),
but the v2 branch is newer than v1. Some features (the vector search
preview) are experimental.

**Mitigation:** Pin to a specific v2 tag. We only use the stable
core features (indexing, querying, faceting, highlighting) — nothing
from the experimental set.

### Query parse failures

bleve's QueryString parser can reject malformed input. Users who
type `foo AND` (trailing operator) currently get a best-effort FTS5
match; bleve throws a parse error.

**Mitigation:** Wrap bleve's parser with a try/fall-back:
1. Try to parse as QueryString.
2. On parse error, fall back to a `MatchQuery` over the `_all` field
   with the raw input.
Users who don't know the syntax still get results; users who do know
it get the full power.

## Out of scope (explicitly)

- Indexing actual audio content (transcripts, chapter markers).
  Different project. Probably uses a separate index anyway.
- Per-user search history / personalization. Current AO is
  single-user-ish; personalization is a post-multi-user feature.
- Semantic search via embeddings. Different problem, different
  solution — see the chromem-go spec.
- Replacing the library browse endpoint wholesale. bleve is used
  when `?q=` is present; browse without `?q=` stays on the existing
  SQL path. Sort/filter/pagination primitives are unchanged.
- Author / series / narrator standalone search endpoints. Those stay
  on SQL for now. Only the main library search switches to bleve.

## Open questions

1. **Version-group members: index the primary only, or all members?**
   Proposing primary-only. A user searching for "Foundation" probably
   wants the version group, not each individual format-sibling.
   But this means search results always point at the primary even if
   a non-primary sibling had the matching metadata. Can live with this.
2. **How do we handle soft-deleted / marked-for-deletion books?**
   Delete from index on soft-delete, re-index on restore. Add the
   hook to the soft-delete / restore code paths.
3. **Index format upgrade path.** bleve's on-disk format is stable
   within v2 but a major version jump might require a rebuild. Document
   the rebuild endpoint as the upgrade path.
4. **Multi-language titles.** A book with an English title AND a
   Japanese romaji alt-title (the future book_alternative_titles
   feature) — do we index the English analyzer on one field and a
   Japanese analyzer on another? Punt until alt-titles lands.
5. **Search inside user notes.** Not in the current schema but
   eventually. Bleve handles it trivially — just add another field.

## Next step

1. Review this spec.
2. Review the chromem-go spec in the same folder — decide whether to
   do them as separate projects or bundle as "Phase 2 of dedup stack
   evolution".
3. If accepted, create a Plan doc at
   `docs/superpowers/plans/2026-04-XX-bleve-library-search.md` with
   bite-sized tasks.
4. Run benchmarks first (especially relevance quality) before
   committing to implementation.
