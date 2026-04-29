<!-- file: docs/superpowers/specs/2026-04-29-audible-category-ladders-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3f7a1c2e-5b9d-4e8f-a2c1-6d0e3f8b2a47 -->
<!-- last-edited: 2026-04-29 -->

# Design: Audible Category Ladders as User Tags

## Overview

Audible's catalog API supports a `category_ladders` response group that returns
hierarchical genre/subject information. This document describes how to ingest
those categories into the audiobook-organizer tag system so they appear on book
detail pages, drive search, and remain idempotent on re-apply.

---

## Audible API Data Shape

When `category_ladders` is added to the `response_groups` query parameter,
each product's JSON includes a top-level array like:

```json
"category_ladders": [
  {
    "ladder": [
      { "id": "18685580011", "name": "Science Fiction & Fantasy" },
      { "id": "18685589011", "name": "Science Fiction" },
      { "id": "18685594011", "name": "Space Opera" }
    ],
    "root": "Audible Books & Originals"
  },
  {
    "ladder": [
      { "id": "18685580011", "name": "Science Fiction & Fantasy" },
      { "id": "18685589011", "name": "Science Fiction" }
    ],
    "root": "Audible Books & Originals"
  }
]
```

Key properties:

- `root` is always a top-level Audible navigation bucket (e.g. "Audible Books &
  Originals"). It is NOT a meaningful genre tag — skip it.
- Each `ladder` entry is an ordered path from broad to specific. Every node in
  the ladder (except the implicit root) is a candidate genre tag.
- A book commonly appears in two or three ladders with overlapping nodes. The
  tag layer must deduplicate.

---

## Tag Storage Strategy

Tags are stored in the `book_tags` table with schema:

```
book_id   TEXT NOT NULL
tag       TEXT NOT NULL
source    TEXT NOT NULL
created_at TIMESTAMP
UNIQUE(book_id, tag, source)   -- enforced by AddBookUserTag
```

Category ladder nodes are stored with `source = "audible_category"`. This
distinguishes them from human-applied tags (`source = "user"`) and system
provenance tags (`source = "system"`).

`AddBookUserTag` in `internal/database/sqlite_store.go` is already idempotent
via `INSERT OR IGNORE`, so re-applying the same candidate does not duplicate
tags.

---

## Ingestion Pipeline

### Step 1 — Fetch

`audibleResponseGroups` constant (in `internal/metadata/audible.go`) must
include `"category_ladders"`. The API then populates
`audibleProduct.CategoryLadders`.

### Step 2 — Parse

`productToMetadata()` iterates all ladders, collects all node `Name` fields
(skipping the root bucket itself, which has no ladder entry — the root is a
string field, not a node), deduplicates them in insertion order, and writes them
to `BookMetadata.CategoryTags []string`.

### Step 3 — Apply

`ApplyMetadataCandidate` in `internal/metafetch/service.go` (and its batch
equivalent) already calls `mfs.ApplyMetadataSystemTags(...)` after updating the
book record. After that call, a new loop over `meta.CategoryTags` calls
`mfs.db.AddBookUserTag(book.ID, tag, "audible_category")` for each tag.

Category tags intentionally bypass the `fields` allowlist mechanism — they are
additive enrichment, not a replacement of a specific book field.

### Step 4 — Tag Lifecycle

Tags accumulate; they are never automatically removed on re-apply. If a user
wants to remove a mistaken category tag, they use the existing tag-delete UI.
This matches the behavior of `metadata:source:*` and `metadata:language:*`
system tags.

---

## UI Presentation

### Tag chips

The frontend (`TagList` component) already renders chips for each `book_tags`
entry. The `source` field is available in the API response. Category tags should
render with the MUI `color="info"` chip variant (cyan/teal) to distinguish them
from:

- `color="default"` — user-added tags
- `color="secondary"` — system provenance tags (`metadata:source:*`, etc.)

### Tag tooltip

Hovering an `audible_category` chip should show "Genre from Audible" as the
tooltip. This keeps the distinction clear without requiring a legend.

### No new UI components needed

The existing chip rendering path already supports per-source colors via a
`sourceColorMap` lookup — add `"audible_category": "info"` to that map.

---

## Search

The existing `has_tag:"Science Fiction"` query syntax in the search parser
already drives a `book_tags` join. No parser changes are needed. Category tags
are immediately searchable after the first apply.

Example queries that will work once tags are populated:

```
has_tag:"Space Opera"
has_tag:"Science Fiction & Fantasy"
has_tag:"Mystery"
```

---

## Deduplication Rules

1. Within a single product, collect all node names across all ladders into a
   `map[string]struct{}` (insertion-order slice backed by the map for
   determinism).
2. Across re-applies, `AddBookUserTag`'s `INSERT OR IGNORE` is the
   deduplication boundary — no pre-check needed.
3. Case sensitivity: store tags exactly as Audible returns them (title case).
   The search layer already does case-insensitive LIKE matching.

---

## Error Handling

- If `category_ladders` is absent from the API response (e.g. ASIN lookups
  that don't include the group), `CategoryLadders` is nil — the loop is a
  no-op.
- `AddBookUserTag` errors are logged as `[WARN]` and do not fail the apply
  transaction. Category tagging is best-effort enrichment.

---

## Not In Scope

- Hierarchical / nested display of categories in the UI (flat chip list is
  sufficient for v1).
- Category-to-genre field mapping (the `genre` DB column is a free-text field
  managed separately; category tags are additive).
- Syncing category tags back into audio file ID3/MP4 tags (tag write-back
  covers standard fields only).
- Removing stale category tags when a book's Audible entry changes category.

---

## Acceptance Criteria

1. After applying an Audible metadata candidate, `book_tags` contains one row
   per ladder node with `source = "audible_category"`.
2. Re-applying the same candidate does not create duplicate rows.
3. Category tag chips render with `color="info"` in the book detail UI.
4. `has_tag:"Science Fiction"` returns books with that category tag.
5. Unit test: fake Audible JSON with two overlapping ladders → exactly the
   expected deduplicated tag set.
6. `go test ./internal/metadata/... ./internal/server/...` passes with no new
   failures.
