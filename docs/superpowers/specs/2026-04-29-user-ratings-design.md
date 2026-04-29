<!-- file: docs/superpowers/specs/2026-04-29-user-ratings-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: a3f7c2e1-84b5-4d9a-b6f0-2c8e51d73a09 -->
<!-- last-edited: 2026-04-29 -->

# User Ratings — Design Spec

## Overview

Allow users to rate audiobooks across three dimensions: **Overall**, **Story**, and **Performance**. Ratings are stored as nullable floats (0–5, 0.5-step granularity) plus an optional free-text notes field. External ratings (Audible, Google) remain read-only display data and are shown alongside user ratings in the MetadataReviewDialog candidate cards.

---

## 1. Data Model

### 1.1 Book Struct Fields (already added in PR #517)

```go
// internal/database/store.go  — Book struct
UserRatingOverall     *float64 `json:"user_rating_overall"`
UserRatingStory       *float64 `json:"user_rating_story"`
UserRatingPerformance *float64 `json:"user_rating_performance"`
UserRatingNotes       *string  `json:"user_rating_notes"`
```

All four fields are **nullable**:

| Value | Meaning |
|-------|---------|
| `nil` / JSON `null` | User has not rated this dimension |
| `0.0` | User explicitly rated 0 stars (lowest) |
| `5.0` | User rated 5 stars (highest) |

A rating of `0.0` is semantically different from "not rated". The UI must distinguish between the two states (e.g., a "not yet rated" placeholder vs. an explicit zero-star rating).

### 1.2 Storage

Columns exist via `ensureExtendedBookColumns` in `internal/database/sqlite_store.go`. No migration file is required. Pebble stores the JSON blob; the fields round-trip through standard JSON marshal/unmarshal.

### 1.3 Valid Rating Values

- Range: `0.0` to `5.0` inclusive
- Step: `0.5`
- Valid discrete values: `0.0, 0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0`
- The backend must reject values outside this set with `400 Bad Request`.

---

## 2. API

### 2.1 Endpoint

```
PATCH /api/v1/audiobooks/:id/rating
```

- **Auth**: standard bearer token (same as all other mutating endpoints)
- **Content-Type**: `application/json`

### 2.2 Request Body

All fields are optional. Sending a field sets it. Sending `null` for a field clears it (sets DB column to NULL). Omitting a field leaves the existing value unchanged.

```json
{
  "overall":     4.5,
  "story":       4.0,
  "performance": 5.0,
  "notes":       "Fantastic narration; dragged a bit in the middle."
}
```

Clear a single dimension:

```json
{ "overall": null }
```

Clear everything:

```json
{ "overall": null, "story": null, "performance": null, "notes": null }
```

### 2.3 Response

On success: `200 OK` with the full updated `Book` object (same shape as `GET /api/v1/audiobooks/:id`).

On validation error: `400 Bad Request`

```json
{ "error": "overall must be between 0 and 5 in 0.5 increments" }
```

On not-found: `404 Not Found`.

### 2.4 Nullability Rules (detailed)

The request body uses a **pointer-of-pointer** sentinel pattern server-side (or explicit `json.RawMessage` parsing) so the handler can distinguish between three states:

| JSON value | Meaning |
|------------|---------|
| Field omitted | Do not touch this column |
| `null` | Set column to NULL (clear rating) |
| `4.5` | Set column to 4.5 |

Implementation guidance: decode into a struct where each rating field is `**float64`. The outer pointer being `nil` means "field was absent"; inner pointer being `nil` means "field was `null`"; non-nil inner pointer carries the value.

---

## 3. Frontend — Book Detail UI

### 3.1 Star Widget Component

A reusable `<StarRating>` component lives at `web/src/components/common/StarRating.tsx`.

Props:

```ts
interface StarRatingProps {
  label: string;           // "Overall", "Story", "Performance"
  value: number | null;    // null = not rated
  onChange: (v: number | null) => void;
  readOnly?: boolean;
}
```

Behavior:

- Renders 5 stars; each star has two clickable half-regions (left = N-0.5, right = N).
- MUI `Rating` component supports `precision={0.5}` — use it.
- Clicking the currently-selected value toggles back to `null` ("not rated").
- A small `✕ Clear` `IconButton` appears to the right when `value !== null`.
- `readOnly` disables interaction and hides the clear button (used for displaying Audible/Google ratings).

### 3.2 Book Detail Ratings Panel

Located in `web/src/components/audiobooks/BookDetail.tsx`, rendered as a collapsible `<Accordion>` section titled **"My Ratings"**, positioned below the main metadata panel.

Layout:

```
My Ratings                         [Save]
─────────────────────────────────────────
Overall      ★ ★ ★ ★ ☆   4.0    [✕]
Story        ★ ★ ★ ★ ★   5.0    [✕]
Performance  ★ ★ ★ ☆ ☆   3.0    [✕]

Notes
┌─────────────────────────────────────┐
│ Great performance, slow middle act. │
└─────────────────────────────────────┘
                                [Save]
```

- Changes are local-state until **Save** is clicked.
- **Save** calls `PATCH /api/v1/audiobooks/:id/rating` with only changed fields (delta patch).
- Optimistic update: immediately update local state, rollback with a snackbar on error.
- If all three star ratings are `null` and notes is empty, display a muted "Not yet rated" placeholder instead of the widget rows (expand accordion to add rating).

### 3.3 External Ratings (read-only)

When a book has Audible or Google metadata, show a separate read-only sub-section **"External Ratings"** with:

- Audible: up to 5 dimensions displayed as read-only `StarRating` widgets (overall + 4 sub-scores if present).
- Google: single rating, shown with its review count.
- These are never editable.

---

## 4. MetadataReviewDialog — Candidate Cards

`web/src/components/audiobooks/MetadataReviewDialog.tsx`

In `renderCompactRow` (and/or `renderTwoColumnCard`), add info chips to each candidate card showing the source's rating data:

### 4.1 Audible Ratings Chips

If the candidate has an Audible overall rating (field: `audible_rating_overall`):

```tsx
<Chip
  label={`★ ${candidate.audible_rating_overall?.toFixed(1)} Audible`}
  size="small"
  variant="outlined"
  color="default"
/>
```

If sub-scores are available (story, performance, value):

```tsx
<Chip label={`Story ${candidate.audible_rating_story?.toFixed(1)}`} size="small" />
<Chip label={`Perf ${candidate.audible_rating_performance?.toFixed(1)}`} size="small" />
```

### 4.2 Google Ratings Chip

```tsx
<Chip
  label={`★ ${candidate.google_rating?.toFixed(1)} Google (${candidate.google_ratings_count} ratings)`}
  size="small"
  variant="outlined"
/>
```

Only render each chip if the field is non-null.

---

## 5. Library Search / Filter

### 5.1 Search Syntax

Extend the existing filter parser to support:

| Expression | Meaning |
|------------|---------|
| `user_rating_overall > 4` | Overall rating strictly greater than 4 |
| `user_rating_overall >= 4` | Overall rating ≥ 4 |
| `user_rating_overall = 4.5` | Exact match |
| `user_rating_story <= 3` | Story rating ≤ 3 |
| `user_rating_performance > 0` | Performance has been rated (any positive value) |
| `user_rating_overall = null` | Not yet rated |

### 5.2 SQL Implementation

The filter parser (`internal/database/filter_parser.go` or equivalent) maps `user_rating_overall` → `user_rating_overall` column. NULL-aware comparisons:

```sql
-- "user_rating_overall > 4"
user_rating_overall > 4.0

-- "user_rating_overall = null"
user_rating_overall IS NULL
```

### 5.3 Frontend Filter UI

In the advanced search panel, add a **"My Ratings"** filter group with three range sliders (min 0, max 5, step 0.5) for Overall, Story, Performance. A "Not yet rated" checkbox per dimension emits the `= null` filter.

---

## 6. Bulk Quick-Rate from List View

### 6.1 UX Flow

1. User selects one or more books in the library list using checkboxes.
2. A bulk-action bar appears at the bottom of the screen (same bar used for batch-delete, etc.).
3. The bar includes a **"Rate"** button.
4. Clicking **Rate** opens a compact dialog:

```
Rate 12 books
─────────────────────────────────────────
Overall      [★ ★ ★ ★ ☆]   (leave blank = don't change)
Story        [★ ★ ★ ★ ☆]   (leave blank = don't change)
Performance  [★ ★ ★ ★ ☆]   (leave blank = don't change)
Notes        [                          ]

[Cancel]                          [Apply to 12 books]
```

5. Blank dimensions are not sent (existing values preserved).
6. Submit fires `N` parallel `PATCH /api/v1/audiobooks/:id/rating` requests (one per book). Use `Promise.allSettled` so partial failures don't block the rest.
7. Show a summary snackbar: "Rated 12 books (0 errors)" or "Rated 10 books (2 errors — see console)".

### 6.2 API

No new bulk endpoint is needed. The frontend fires individual PATCHes. If we later need a bulk endpoint for performance, add `POST /api/v1/audiobooks/batch-operations` action `"rate"`.

---

## 7. Non-Goals (Out of Scope for This Iteration)

- Rating history / changelog entries for rating changes
- Rating-based smart playlists
- Syncing user ratings to Audible or any external service
- Star ratings displayed in the library list columns (filter only, no column)
- Rating analytics / dashboard

---

## 8. Open Questions

1. Should clearing all three star ratings AND clearing notes via the dialog auto-collapse the accordion to "Not yet rated" state? (Suggested: yes, with a 300ms animation.)
2. Should the bulk quick-rate dialog have a "Clear ratings" mode in addition to "Set ratings"? (Deferred to follow-up.)
3. Audible sub-score field names in `MetadataCandidate` — confirm exact JSON keys with the Audible fetcher before implementing chips.
