<!-- file: docs/superpowers/bot-tasks/2026-04-29-audible-category-ladders.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8c4b2d1f-7e3a-4f9c-b5d2-1a6e0c8f3b74 -->
<!-- last-edited: 2026-04-29 -->

# Bot Task: CAT-1 — Audible Category Ladders

> **Read every word.** Do exactly what is written. Do not guess, do not
> improvise, do not add extra changes. If something is unclear, STOP and ask.

---

## Goal

Ingest Audible's `category_ladders` API data into the book tag system so that
genre categories (e.g. "Science Fiction", "Space Opera") are stored as user tags
with `source = "audible_category"` when a metadata candidate is applied.

---

## Files You Will Touch

| File | What changes |
|------|--------------|
| `internal/metadata/audible.go` | Add response group, add structs, populate `CategoryTags` |
| `internal/metadata/openlibrary.go` | Add `CategoryTags []string` to `BookMetadata` |
| `internal/metafetch/service.go` | After apply, loop over `CategoryTags` and call `AddBookUserTag` |
| `internal/metadata/audible_test.go` | New unit test for category ladder parsing |

Do NOT touch any other file unless explicitly told to.

---

## Step 1 — `internal/metadata/audible.go`

### 1a. Add `"category_ladders"` to the response groups constant

Find this exact line (line 104):

```go
const audibleResponseGroups = "product_desc,contributors,media,product_attrs,series,rating"
```

Replace it with:

```go
const audibleResponseGroups = "product_desc,contributors,media,product_attrs,series,rating,category_ladders"
```

### 1b. Add the two new structs

Find the closing brace of `audibleSeries` (which ends around line 102):

```go
type audibleSeries struct {
	ASIN     string `json:"asin"`
	Title    string `json:"title"`
	Sequence string `json:"sequence"`
}
```

IMMEDIATELY AFTER that closing brace (before the blank line before `const`),
insert:

```go
type audibleCategoryNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type audibleCategoryLadder struct {
	Ladder []audibleCategoryNode `json:"ladder"`
	Root   string                `json:"root"`
}
```

### 1c. Add `CategoryLadders` field to `audibleProduct`

Find the `audibleProduct` struct. It currently ends with:

```go
	Rating               *audibleRating    `json:"rating"`
}
```

Replace that closing section with:

```go
	Rating               *audibleRating         `json:"rating"`
	CategoryLadders      []audibleCategoryLadder `json:"category_ladders"`
}
```

### 1d. Populate `CategoryTags` in `productToMetadata`

Find the end of `productToMetadata`. It currently ends with:

```go
	// Ratings: overall, narrator performance, story quality.
	if p.Rating != nil {
		meta.AudibleRatingOverall = p.Rating.OverallDistribution.DisplayAverageRating
		meta.AudibleRatingPerformance = p.Rating.PerformanceDistribution.DisplayAverageRating
		meta.AudibleRatingStory = p.Rating.StoryDistribution.DisplayAverageRating
		meta.AudibleRatingCount = p.Rating.OverallDistribution.NumRatings
		meta.AudibleNumReviews = p.Rating.NumReviews
	}

	return meta
}
```

Replace that block with:

```go
	// Ratings: overall, narrator performance, story quality.
	if p.Rating != nil {
		meta.AudibleRatingOverall = p.Rating.OverallDistribution.DisplayAverageRating
		meta.AudibleRatingPerformance = p.Rating.PerformanceDistribution.DisplayAverageRating
		meta.AudibleRatingStory = p.Rating.StoryDistribution.DisplayAverageRating
		meta.AudibleRatingCount = p.Rating.OverallDistribution.NumRatings
		meta.AudibleNumReviews = p.Rating.NumReviews
	}

	// Category ladders: collect all node names from all ladders, deduplicate.
	// Each ladder is a path from broad to specific (e.g. "Science Fiction" →
	// "Space Opera"). The Root field is a navigation bucket, not a genre — skip it.
	if len(p.CategoryLadders) > 0 {
		seen := map[string]struct{}{}
		tags := make([]string, 0)
		for _, ladder := range p.CategoryLadders {
			for _, node := range ladder.Ladder {
				name := strings.TrimSpace(node.Name)
				if name == "" {
					continue
				}
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					tags = append(tags, name)
				}
			}
		}
		if len(tags) > 0 {
			meta.CategoryTags = tags
		}
	}

	return meta
}
```

---

## Step 2 — `internal/metadata/openlibrary.go`

This file contains the `BookMetadata` struct (around line 104). Find it:

```go
// BookMetadata represents enriched book metadata
type BookMetadata struct {
	Title          string
	Author         string
	Narrator       string
	Description    string
	Publisher      string
	PublishYear    int
	ISBN           string
	ASIN           string
	CoverURL       string
	Language       string
	Genre          string
	Series         string
	SeriesPosition string
	DurationSec int // audio runtime in seconds (Audible: runtime_length_min × 60)
```

Find the closing brace of the struct (after the Google ratings fields). Add
`CategoryTags` as the last field before the closing `}`:

```go
	// CategoryTags contains genre/subject tags from Audible's category_ladders
	// response group. Each element is a ladder node name (e.g. "Science Fiction",
	// "Space Opera"). Applied as book_tags with source="audible_category".
	CategoryTags []string
}
```

The full struct tail (what you are replacing) looks like:

```go
	// Google Books rating (1–5 scale).
	GoogleRatingAverage float64
	GoogleRatingCount   int
}
```

Replace it with:

```go
	// Google Books rating (1–5 scale).
	GoogleRatingAverage float64
	GoogleRatingCount   int

	// CategoryTags contains genre/subject tags from Audible's category_ladders
	// response group. Each element is a ladder node name (e.g. "Science Fiction",
	// "Space Opera"). Applied as book_tags with source="audible_category".
	CategoryTags []string
}
```

---

## Step 3 — `internal/metafetch/service.go`

### 3a. Find the right insertion point

Open `internal/metafetch/service.go`. Find the function `ApplyMetadataCandidate`
(around line 2502). Inside that function, find the call to
`mfs.ApplyMetadataSystemTags`:

```go
	mfs.ApplyMetadataSystemTags(id, candidate.Source, meta.Language)
```

### 3b. Insert the category tag loop AFTER that call

Replace:

```go
	mfs.ApplyMetadataSystemTags(id, candidate.Source, meta.Language)

	// Intentionally keep the metadata fetch cache after apply.
```

With:

```go
	mfs.ApplyMetadataSystemTags(id, candidate.Source, meta.Language)

	// Apply Audible category ladder tags. These are additive enrichment — they
	// are not controlled by the fields allowlist and do not fail the apply if
	// a tag write errors.
	for _, tag := range meta.CategoryTags {
		if err := mfs.db.AddBookUserTag(id, tag, "audible_category"); err != nil {
			log.Printf("[WARN] failed to apply category tag %q to book %s: %v", tag, id, err)
		}
	}

	// Intentionally keep the metadata fetch cache after apply.
```

### 3c. Verify the meta variable has CategoryTags

In `ApplyMetadataCandidate`, the `meta` variable is built from `candidate` fields.
The `CategoryTags` field is NOT in `MetadataCandidate` — it comes from
`BookMetadata` returned by the Audible client. However, the apply path receives
a `MetadataCandidate` (the stored search result), not the raw `BookMetadata`.

This means `meta.CategoryTags` will always be nil in the apply path UNLESS we
also add `CategoryTags` to `MetadataCandidate` and wire it through the search
result serialization.

**Do the following additional changes:**

#### 3c-i. Add `CategoryTags` to `MetadataCandidate` struct

Find `MetadataCandidate` struct (around line 137):

```go
type MetadataCandidate struct {
	Title          string  `json:"title"`
	Author         string  `json:"author"`
	...
	DurationDeltaSec int `json:"duration_delta_sec,omitempty"`
}
```

Add this field at the end (before the closing `}`):

```go
	// CategoryTags holds Audible category ladder node names (e.g. "Science Fiction").
	// Only populated for Audible-sourced candidates. Applied as book_tags on apply.
	CategoryTags []string `json:"category_tags,omitempty"`
```

#### 3c-ii. Populate `CategoryTags` in the candidate builder

Search for the function that converts `BookMetadata` to `MetadataCandidate`.
Grep for: `MetadataCandidate{` to find where candidates are constructed from
`BookMetadata`. It will look something like:

```go
candidate := MetadataCandidate{
    Title:    meta.Title,
    Author:   meta.Author,
    ...
    Source:   source,
    Score:    score,
}
```

Add `CategoryTags: meta.CategoryTags,` to every such construction site where
`meta` is a `metadata.BookMetadata`. Do NOT add it to construction sites that
build from DB records or other non-BookMetadata sources.

#### 3c-iii. Use `candidate.CategoryTags` in `ApplyMetadataCandidate`

In step 3b above, the loop uses `meta.CategoryTags`. Since `meta` is built from
`candidate` fields in `ApplyMetadataCandidate`, change the loop to use
`candidate.CategoryTags` directly (not `meta.CategoryTags`):

```go
	// Apply Audible category ladder tags. These are additive enrichment — they
	// are not controlled by the fields allowlist and do not fail the apply if
	// a tag write errors.
	for _, tag := range candidate.CategoryTags {
		if err := mfs.db.AddBookUserTag(id, tag, "audible_category"); err != nil {
			log.Printf("[WARN] failed to apply category tag %q to book %s: %v", tag, id, err)
		}
	}
```

---

## Step 4 — Tests

### 4a. Unit test: category ladder parsing (audible_test.go)

File: `internal/metadata/audible_test.go`

Add this test function. If the file does not exist, create it with the
appropriate package declaration `package metadata`:

```go
func TestProductToMetadata_CategoryLadders(t *testing.T) {
	client := NewAudibleClientWithBaseURL("http://unused")

	p := &audibleProduct{
		ASIN:  "B08G9PRS1K",
		Title: "Test Book",
		CategoryLadders: []audibleCategoryLadder{
			{
				Root: "Audible Books & Originals",
				Ladder: []audibleCategoryNode{
					{ID: "18685580011", Name: "Science Fiction & Fantasy"},
					{ID: "18685589011", Name: "Science Fiction"},
					{ID: "18685594011", Name: "Space Opera"},
				},
			},
			{
				Root: "Audible Books & Originals",
				Ladder: []audibleCategoryNode{
					{ID: "18685580011", Name: "Science Fiction & Fantasy"},
					{ID: "18685589011", Name: "Science Fiction"},
				},
			},
		},
	}

	meta := client.productToMetadata(p)

	// Expect 3 unique tags (deduplication across overlapping ladders)
	want := []string{"Science Fiction & Fantasy", "Science Fiction", "Space Opera"}
	if len(meta.CategoryTags) != len(want) {
		t.Fatalf("CategoryTags: got %d tags, want %d: %v", len(meta.CategoryTags), len(want), meta.CategoryTags)
	}
	for i, w := range want {
		if meta.CategoryTags[i] != w {
			t.Errorf("CategoryTags[%d]: got %q, want %q", i, meta.CategoryTags[i], w)
		}
	}
}

func TestProductToMetadata_NoCategoryLadders(t *testing.T) {
	client := NewAudibleClientWithBaseURL("http://unused")

	p := &audibleProduct{
		ASIN:  "B000001",
		Title: "No Genres",
	}

	meta := client.productToMetadata(p)

	if meta.CategoryTags != nil {
		t.Errorf("expected nil CategoryTags when no ladders, got: %v", meta.CategoryTags)
	}
}
```

### 4b. Integration test: AddBookUserTag is called

In `internal/metafetch/service_mock_test.go` (or a new file
`internal/metafetch/category_tags_test.go`), add a test that:

1. Builds a `MetadataCandidate` with `CategoryTags: []string{"Mystery", "Thriller"}`.
2. Calls `svc.ApplyMetadataCandidate(bookID, candidate, nil)`.
3. Asserts that the mock store's `AddBookUserTag` was called twice — once with
   `("Mystery", "audible_category")` and once with `("Thriller", "audible_category")`.

Use the existing mock store pattern already present in the test file.

---

## Step 5 — Verify

Run:

```bash
go test ./internal/metadata/... ./internal/metafetch/... ./internal/server/...
```

All tests must pass with no new failures. If a test fails, fix it before moving on.

---

## What NOT to Do

- Do NOT remove or rename any existing fields on `audibleProduct`,
  `BookMetadata`, or `MetadataCandidate`.
- Do NOT change the `audibleResponseGroups` line to anything other than exactly
  what is shown in step 1a. Do not reorder the existing groups.
- Do NOT add `CategoryTags` to the `fields` allowlist processing in
  `ApplyMetadataCandidate`. Category tags bypass the allowlist by design.
- Do NOT call `AddBookUserTag` with source `"user"` or `"system"`. The source
  must be exactly `"audible_category"`.
- Do NOT delete category tags during re-apply. The operation is additive only.
- Do NOT change the `book_tags` table schema. The existing schema supports this
  without modification.
- Do NOT add CategoryTags to the tag write-back path (audio file tags). This
  is book_tags enrichment only.

---

## PR Instructions

Branch name: `feat/cat-1-audible-category-ladders`

```bash
git checkout -b feat/cat-1-audible-category-ladders
# make your changes
git add internal/metadata/audible.go \
        internal/metadata/openlibrary.go \
        internal/metafetch/service.go \
        internal/metadata/audible_test.go
git commit -m "feat(metadata): ingest Audible category_ladders as user tags

Adds category_ladders response group to the Audible API request.
Each ladder node name is stored as a book_tag with source=audible_category
during ApplyMetadataCandidate. Idempotent via INSERT OR IGNORE."
git push -u origin feat/cat-1-audible-category-ladders
gh pr create \
  --title "feat(metadata): ingest Audible category_ladders as user tags" \
  --body "$(cat <<'EOF'
## Summary
- Adds \`category_ladders\` to \`audibleResponseGroups\` constant
- Parses ladder nodes into \`BookMetadata.CategoryTags []string\`
- Wires \`CategoryTags\` through \`MetadataCandidate\` so they survive serialization
- Calls \`AddBookUserTag(id, tag, \"audible_category\")\` in \`ApplyMetadataCandidate\`
- Unit tests verify deduplication across overlapping ladders

## Test plan
- [ ] \`go test ./internal/metadata/...\` passes (new unit tests)
- [ ] \`go test ./internal/metafetch/...\` passes (integration test)
- [ ] Apply an Audible candidate for a known SF book; verify tags appear in UI
- [ ] Re-apply the same candidate; verify no duplicate tags created
EOF
)"
```

After CI is green, merge with rebase (no squash):

```bash
gh pr merge --rebase
```
