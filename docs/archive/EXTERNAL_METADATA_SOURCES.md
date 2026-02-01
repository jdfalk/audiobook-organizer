<!-- file: EXTERNAL_METADATA_SOURCES.md -->
<!-- version: 1.0.0 -->
<!-- guid: b1c2d3e4-f5a6-7890-bcde-1f2a3b4c5d6e -->
<!-- last-edited: 2026-01-25 -->

# External Metadata Sources - Current Implementation & Roadmap

**Date**: 2026-01-25 **Status**: ✅ **Open Library Integration Complete**

---

## Executive Summary

**Your MVP DOES include external metadata fetching!** The application currently
integrates with **Open Library API** for pulling book metadata.

### Current Implementation: ✅ Complete

| Feature                      | Status      | Details                                          |
| ---------------------------- | ----------- | ------------------------------------------------ |
| External metadata source     | ✅ Complete | Open Library API                                 |
| Single book metadata fetch   | ✅ Complete | `POST /api/v1/audiobooks/:id/fetch-metadata`     |
| Bulk metadata fetch          | ✅ Complete | `POST /api/v1/metadata/bulk-fetch`               |
| UI: Single book fetch        | ✅ Complete | "Fetch Metadata" button on book detail page      |
| UI: Bulk fetch               | ✅ Complete | "Bulk Fetch Metadata" dialog in Library page     |
| Metadata provenance tracking | ✅ Complete | Tracks file/fetched/stored/override values       |
| Override protection          | ✅ Complete | Won't overwrite locked or user-overridden fields |
| ISBN search support          | ✅ Complete | Can search by ISBN if available                  |

**Assessment**: ✅ **MVP requirement met** - External metadata fetching is fully
implemented

---

## Part 1: What's Currently Implemented

### 1. Open Library API Integration

**File**: `internal/metadata/openlibrary.go`

**Capabilities**:

- ✅ Search by title
- ✅ Search by title + author (more accurate)
- ✅ Search by ISBN
- ✅ Fetch cover images
- ✅ Fetch publication year
- ✅ Fetch publisher information
- ✅ Fetch language

**Implementation**:

```go
type OpenLibraryClient struct {
    httpClient *http.Client
    baseURL    string
}

// Search methods:
func (c *OpenLibraryClient) SearchByTitle(title string) ([]BookMetadata, error)
func (c *OpenLibraryClient) SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error)
func (c *OpenLibraryClient) GetBookByISBN(isbn string) (*BookMetadata, error)
```

**Metadata Fields Retrieved**:

- Title
- Author
- Publisher
- Publication Year
- ISBN
- Language
- Cover URL (from covers.openlibrary.org)

### 2. Backend API Endpoints

#### Single Book Fetch

**Endpoint**: `POST /api/v1/audiobooks/:id/fetch-metadata`

**Behavior**:

1. Fetches metadata from Open Library using book's title + author
2. Intelligently applies fetched data based on provenance rules:
   - ❌ Won't overwrite if field has `override_locked = true`
   - ❌ Won't overwrite if field has `override_value` set
   - ✅ Updates `fetched_value` for all fields
   - ✅ May update `effective_value` based on provenance hierarchy
3. Returns updated book with source attribution

**Response**:

```json
{
  "message": "metadata fetched and applied",
  "book": {
    /* updated book data */
  },
  "source": "Open Library"
}
```

#### Bulk Fetch

**Endpoint**: `POST /api/v1/metadata/bulk-fetch`

**Request**:

```json
{
  "book_ids": ["book-1", "book-2", "book-3"],
  "only_missing": true // Only update fields that are currently empty
}
```

**Behavior**:

- Fetches metadata for multiple books in one operation
- Respects override locks and user overrides
- Can optionally only fill missing fields (default: true)
- Returns per-book results showing what was applied

**Response**:

```json
{
  "results": [
    {
      "book_id": "book-1",
      "status": "success",
      "applied_fields": ["publisher", "language"],
      "fetched_fields": ["title", "author", "publisher", "language"]
    }
  ],
  "updated_count": 15,
  "source": "Open Library"
}
```

### 3. Frontend UI Integration

#### Book Detail Page

**Location**: `web/src/pages/BookDetail.tsx`

**Features**:

- ✅ "Fetch Metadata" button in action toolbar
- ✅ Displays success/error messages
- ✅ Shows which source was used (Open Library)
- ✅ Respects metadata provenance system
- ✅ Won't overwrite locked fields

**User Flow**:

1. User opens book detail page
2. User clicks "Fetch Metadata" button
3. Request sent to Open Library
4. Metadata applied based on provenance rules
5. Success toast shows "Metadata fetched from Open Library"
6. Page refreshes with updated data

#### Library Page - Bulk Fetch

**Location**: `web/src/pages/Library.tsx`

**Features**:

- ✅ "Bulk Fetch Metadata" dialog
- ✅ Checkbox: "Only fill missing fields" (default: true)
- ✅ Progress indicator during fetch
- ✅ Results summary showing success/failure count

**User Flow**:

1. User selects multiple books (or all books)
2. User clicks "Bulk Fetch Metadata" action
3. Dialog appears with options
4. User confirms
5. Metadata fetched for all selected books
6. Results dialog shows:
   - Total books processed
   - Successful fetches
   - Failed fetches
   - Fields updated per book

### 4. Metadata Provenance Integration

**How it works**:

The metadata fetch respects the provenance hierarchy:

1. **Override** (highest priority) - User-locked values
2. **Stored** - Current database values
3. **Fetched** - Values from Open Library ← **Fetched values stored here**
4. **File** - Values from audio file tags (lowest priority)

**Smart Application Logic**:

```
IF field has override_locked = true:
  → Don't update effective_value
  → Still save fetched_value for reference
ELSE IF field has override_value set:
  → Don't update effective_value
  → Still save fetched_value for reference
ELSE IF only_missing = true AND field has stored_value:
  → Don't update effective_value
  → Still save fetched_value for reference
ELSE:
  → Update stored_value with fetched data
  → Update effective_value
  → Save fetched_value
```

**Benefits**:

- Users can see fetched metadata without it overwriting their manual edits
- "Compare" tab shows all sources side-by-side
- Users can selectively apply fetched values they want

### 5. E2E Test Coverage

**Current Tests**:

- ✅ `book-detail.spec.ts`: Tests "Fetch Metadata" button
  - Test: "metadata refresh and AI parse actions"
  - Verifies metadata fetch updates book title

**Coverage**: Basic functionality tested (~30% of fetch workflows)

**Missing E2E Tests** (from E2E_TEST_PLAN.md):

- ❌ Bulk fetch workflow (select multiple → fetch → verify results)
- ❌ "Only fill missing fields" option behavior
- ❌ Verify fetched values don't overwrite locked fields
- ❌ View fetched values in Compare tab
- ❌ Apply fetched value from Compare tab
- ❌ Bulk fetch progress monitoring
- ❌ Bulk fetch error handling (API down, rate limit, no results)

---

## Part 2: What's NOT Implemented (Potential Enhancements)

### Alternative Metadata Sources

While Open Library is implemented, these sources are NOT currently integrated:

#### 1. Goodreads API ❌

**Status**: Not implemented **Why it would be valuable**:

- Richer book descriptions
- User ratings and reviews
- Better series information
- More comprehensive audiobook-specific data

**Challenge**: Goodreads API was shut down to new developers in 2020
**Alternative**: Could use web scraping (against ToS) or manual ISBN lookup

#### 2. Amazon/Audible API ❌

**Status**: Not implemented **Why it would be valuable**:

- Audiobook-specific metadata (narrator, runtime)
- Accurate audiobook publication dates
- Audiobook cover art (not book cover)
- Series information

**Challenge**: No official public API for Audible **Alternative**: Amazon
Product Advertising API (requires approval)

#### 3. Google Books API ❌

**Status**: Not implemented **Why it would be valuable**:

- Free, public API (no approval needed)
- Good coverage of published books
- Preview/description text
- Multiple ISBN formats

**Implementation Difficulty**: Easy (similar to Open Library)

#### 4. Library of Congress API ❌

**Status**: Not implemented **Why it would be valuable**:

- Authoritative metadata
- ISBN validation
- Subject classifications

**Implementation Difficulty**: Easy

#### 5. MusicBrainz API ❌

**Status**: Not implemented **Why it would be valuable**:

- Audiobook release information
- Narrator credits
- Production companies

**Implementation Difficulty**: Medium

### Enhanced Features (Not Implemented)

#### 1. Multiple Source Aggregation ❌

**What it would do**:

- Query multiple sources (Open Library + Google Books + Amazon)
- Merge results intelligently
- Show which source provided each field
- Let user choose preferred source per field

**Value**: More complete metadata, especially for obscure books

#### 2. Automatic ISBN Extraction ❌

**What it would do**:

- Extract ISBN from audio file metadata if present
- Use ISBN for more accurate searches
- Validate ISBN format

**Value**: More accurate metadata matches

#### 3. Fuzzy Matching Improvements ❌

**What it would do**:

- Better handling of subtitle variations
- Series number extraction from title
- Handle "Book 1 of Series" vs "Series: Book 1" formats

**Value**: Better automatic matching for complex titles

#### 4. Manual Source Selection ❌

**What it would do**:

- Let user choose which metadata source to use
- Per-book source preference
- Multiple results selection (choose best match)

**Value**: User control over data quality

#### 5. Scheduled Auto-Fetch ❌

**What it would do**:

- Automatically fetch metadata for new books after scan
- Configurable: immediate, daily, weekly
- Background job processing

**Value**: Reduces manual effort

---

## Part 3: Recommendations

### For MVP Release (Current State)

**Verdict**: ✅ **External metadata fetching is MVP-complete**

**What you have**:

- ✅ Working Open Library integration
- ✅ Single and bulk fetch capabilities
- ✅ Smart provenance-aware application
- ✅ UI for both single and bulk operations
- ✅ Basic E2E test coverage

**What's sufficient for MVP**:

- Open Library is free, reliable, and has good coverage
- Provenance system prevents data loss
- Users can manually edit any incorrect fetched data
- Bulk operations make it efficient for large libraries

**Recommendation**: ✅ **Ship MVP with current Open Library implementation**

### Post-MVP Enhancements (Priority Order)

#### Priority 1: Complete E2E Tests (2-3 hours)

**Why**: Validate existing functionality thoroughly

**Tests to add**:

1. Bulk fetch workflow (select → fetch → verify)
2. "Only fill missing fields" behavior
3. Verify locked fields aren't overwritten
4. Fetched values in Compare tab
5. Apply fetched values from Compare tab
6. Error handling (no results, API down)

**Outcome**: Confidence in existing feature

#### Priority 2: Google Books Integration (1-2 days)

**Why**: Second source for better coverage, especially obscure titles

**Implementation**:

- Add `internal/metadata/googlebooks.go`
- Similar API to Open Library client
- Fallback logic: Try Open Library → Google Books
- UI selector to choose preferred source

**Value**: Better metadata for books not in Open Library

#### Priority 3: Multiple Source Aggregation (3-4 days)

**Why**: Richer metadata from multiple sources

**Implementation**:

- Query all sources in parallel
- Merge results with conflict resolution
- Show source attribution per field
- Let user choose which source to trust per field

**Value**: Most complete metadata possible

#### Priority 4: Automatic Fetch After Scan (1-2 days)

**Why**: Reduces manual work for new imports

**Implementation**:

- Add configuration option: "Auto-fetch metadata after scan"
- Background job: Fetch metadata for all new import books
- Notification when complete
- Still respects "only_missing" logic

**Value**: Better user experience for large imports

### What NOT to Add

❌ **Goodreads scraping**: Against ToS, fragile, likely to break

❌ **Amazon API without approval**: Requires partnership approval, complex

❌ **Multiple manual source selection per book**: Over-complicated UI for
limited value

---

## Part 4: Current MVP Status

### Metadata Fetching Completeness Matrix

| Feature                  | Implemented | E2E Tested   | MVP Required    | Status           |
| ------------------------ | ----------- | ------------ | --------------- | ---------------- |
| External metadata source | ✅ Yes      | ✅ Partial   | ✅ Yes          | ✅ **MVP Ready** |
| Single book fetch        | ✅ Yes      | ✅ Yes       | ✅ Yes          | ✅ **MVP Ready** |
| Bulk fetch               | ✅ Yes      | ❌ No        | ✅ Yes          | ⚠️ **Needs E2E** |
| Override protection      | ✅ Yes      | ❌ No        | ✅ Yes          | ⚠️ **Needs E2E** |
| Provenance tracking      | ✅ Yes      | ✅ Excellent | ✅ Yes          | ✅ **MVP Ready** |
| Cover image fetch        | ✅ Yes      | ❌ No        | ⚠️ Nice-to-have | ⚠️ **Needs E2E** |
| ISBN search              | ✅ Yes      | ❌ No        | ⚠️ Nice-to-have | ⚠️ **Needs E2E** |
| Multiple sources         | ❌ No       | N/A          | ❌ No           | ⚠️ **Post-MVP**  |

### Gap Analysis for MVP

**Backend**: ✅ **100% Complete** for MVP requirements

- Open Library integration working
- Smart metadata application
- Bulk operations supported

**Frontend**: ✅ **100% Complete** for MVP requirements

- Single book fetch button
- Bulk fetch dialog
- Success/error messaging

**E2E Tests**: ⚠️ **30% Complete** for metadata fetching workflows

- Basic fetch tested
- Bulk fetch NOT tested
- Override protection NOT tested
- Error scenarios NOT tested

**Overall**: ⚠️ **Needs additional E2E testing before MVP release**

---

## Part 5: E2E Test Plan for Metadata Fetching

### New Tests Needed

**File**: `web/tests/e2e/metadata-fetching.spec.ts` (NEW)

```typescript
describe('External Metadata Fetching', () => {
  describe('Single Book Fetch', () => {
    test('fetches metadata from Open Library by title', async ({ page }) => {
      // GIVEN: Book with title "The Hobbit" but missing publisher
      // WHEN: User clicks "Fetch Metadata" button
      // THEN: Metadata is fetched from Open Library
      // AND: Publisher field is updated
      // AND: Toast shows "Metadata fetched from Open Library"
    });

    test('does not overwrite locked fields', async ({ page }) => {
      // GIVEN: Book with narrator field locked to "Custom Narrator"
      // WHEN: User clicks "Fetch Metadata"
      // AND: Open Library returns different narrator
      // THEN: Locked field is NOT updated
      // AND: Fetched value is saved but not applied
      // AND: Compare tab shows both values
    });

    test('updates fetched_value even when not applied', async ({ page }) => {
      // GIVEN: Book with all fields locked
      // WHEN: User clicks "Fetch Metadata"
      // THEN: No effective_values change
      // BUT: fetched_values are updated
      // AND: User can see fetched values in Compare tab
    });

    test('handles book not found in Open Library', async ({ page }) => {
      // GIVEN: Book with very obscure title
      // WHEN: User clicks "Fetch Metadata"
      // THEN: Shows error "No metadata found in Open Library"
      // AND: Book data is unchanged
    });

    test('handles Open Library API error', async ({ page }) => {
      // GIVEN: Open Library API returns 500 error
      // WHEN: User clicks "Fetch Metadata"
      // THEN: Shows error "Failed to fetch metadata"
      // AND: Provides "Retry" option
    });
  });

  describe('Bulk Fetch', () => {
    test('bulk fetches metadata for selected books', async ({ page }) => {
      // GIVEN: 5 books selected in library
      // WHEN: User clicks "Bulk Fetch Metadata"
      // AND: User confirms
      // THEN: Metadata fetched for all 5 books
      // AND: Shows progress "Fetching 3/5..."
      // WHEN: Complete
      // THEN: Shows "Fetched metadata for 5 books"
    });

    test('only fills missing fields when option selected', async ({ page }) => {
      // GIVEN: Book with publisher already set
      // WHEN: User bulk fetches with "Only fill missing" = true
      // THEN: Publisher is NOT updated
      // AND: Missing fields (language, ISBN) ARE updated
    });

    test('overwrites all fields when option disabled', async ({ page }) => {
      // GIVEN: Book with publisher already set
      // WHEN: User bulk fetches with "Only fill missing" = false
      // THEN: All non-locked fields are updated
      // INCLUDING publisher (overwritten)
    });

    test('shows detailed results for bulk fetch', async ({ page }) => {
      // GIVEN: Bulk fetch completed for 10 books
      // WHEN: Results dialog appears
      // THEN: Shows "10 books processed"
      // AND: Shows "8 successful, 2 failed"
      // AND: Lists failed books with reasons
      // AND: Lists which fields were updated per book
    });

    test('respects locked fields during bulk fetch', async ({ page }) => {
      // GIVEN: Book 1 has narrator locked, Books 2-5 don't
      // WHEN: User bulk fetches for all 5 books
      // THEN: Book 1 narrator is NOT updated
      // AND: Books 2-5 narrator IS updated
      // AND: Results show "narrator" in applied_fields for Books 2-5 only
    });

    test('handles partial failures in bulk fetch', async ({ page }) => {
      // GIVEN: Bulk fetch for 10 books
      // WHEN: 7 succeed, 3 fail (not found in Open Library)
      // THEN: Shows "7 successful, 3 failed"
      // AND: Lists failed books: "Book A: No metadata found"
      // AND: Successful books are updated
      // AND: Failed books are unchanged
    });

    test('cancels bulk fetch in progress', async ({ page }) => {
      // GIVEN: Bulk fetch running (processed 5/20 books)
      // WHEN: User clicks "Cancel"
      // THEN: Fetch operation stops
      // AND: Shows "Bulk fetch cancelled. Processed 5/20 books."
      // AND: First 5 books keep fetched metadata
      // AND: Remaining 15 books unchanged
    });
  });

  describe('Metadata Provenance Integration', () => {
    test('apply fetched value from Compare tab', async ({ page }) => {
      // GIVEN: Book has fetched publisher "Penguin Random House"
      // BUT: Current stored value is "Unknown"
      // WHEN: User opens Compare tab
      // THEN: Shows fetched value "Penguin Random House"
      // WHEN: User clicks "Use Fetched" button
      // THEN: Publisher updates to "Penguin Random House"
      // AND: Source changes to "override" (user chose it)
    });

    test('view all fetched values in Compare tab', async ({ page }) => {
      // GIVEN: Book fetched metadata from Open Library
      // WHEN: User opens Compare tab
      // THEN: "Fetched" column shows values from Open Library
      // AND: Shows values even if not currently applied
      // AND: Can apply any individual field
    });

    test('locked indicator prevents fetch application', async ({ page }) => {
      // GIVEN: Book has narrator locked
      // WHEN: User views Compare tab
      // THEN: Narrator row shows lock icon
      // AND: "Use Fetched" button is disabled for narrator
      // AND: Tooltip explains "Field is locked"
    });
  });

  describe('ISBN-based Search', () => {
    test('uses ISBN if available for more accurate fetch', async ({ page }) => {
      // GIVEN: Book has ISBN in metadata
      // WHEN: User clicks "Fetch Metadata"
      // THEN: Request uses ISBN for search
      // AND: Returns more accurate results
      // AND: Source shows "Open Library (via ISBN)"
    });

    test('falls back to title search if ISBN not found', async ({ page }) => {
      // GIVEN: Book has no ISBN
      // WHEN: User clicks "Fetch Metadata"
      // THEN: Uses title + author search
      // AND: Still retrieves metadata successfully
    });
  });

  describe('Cover Image Fetching', () => {
    test('fetches and displays cover image', async ({ page }) => {
      // GIVEN: Book has no cover image
      // WHEN: User clicks "Fetch Metadata"
      // AND: Open Library has cover image
      // THEN: Cover image URL is saved
      // AND: Cover displays on book card
      // AND: Cover displays on detail page
    });

    test('handles missing cover image gracefully', async ({ page }) => {
      // GIVEN: Open Library has no cover for this book
      // WHEN: User clicks "Fetch Metadata"
      // THEN: Other metadata is fetched successfully
      // AND: Cover remains default placeholder
      // AND: No error shown (cover is optional)
    });
  });
});
```

**Estimated Implementation Time**: 4-5 hours

---

## Conclusion

### Summary

**Your MVP DOES have external metadata fetching!** ✅

**Current State**:

- ✅ Open Library API fully integrated
- ✅ Single book fetch working
- ✅ Bulk fetch working
- ✅ Smart provenance-aware application
- ✅ UI components complete
- ⚠️ E2E tests incomplete (30% coverage)

**For MVP Release**:

1. ✅ **Backend implementation**: Complete
2. ✅ **Frontend implementation**: Complete
3. ⚠️ **E2E test coverage**: Needs 4-5 hours of work to add comprehensive tests
4. ✅ **Feature completeness**: Meets MVP requirements

**Recommendation**:

- Add metadata fetching E2E tests (4-5 hours) as part of Phase 1 testing
- Include in the E2E test plan from `E2E_TEST_PLAN.md`
- Then MVP is ready to ship with solid external metadata integration

**Post-MVP Enhancements** (Optional):

- Add Google Books API (second source)
- Add multiple source aggregation
- Add automatic fetch after scan
- NOT recommended: Goodreads (ToS issues), Amazon (no public API)

---

_Analysis completed_: 2026-01-25 _Status_: Feature implemented, needs E2E test
coverage _Recommendation_: Add E2E tests, then ship MVP
