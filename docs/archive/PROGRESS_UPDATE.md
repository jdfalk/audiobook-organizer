# Progress Update: Wanted Feature Implementation

## ✅ Completed Tasks

### 1. Database Migrations (Phase 1) - COMPLETE
**Migration 013** already fully implemented in `internal/database/migrations.go`:
- ✅ Created `audiobook_source_paths` table for multi-path tracking
- ✅ Added indices for efficient queries
- ✅ Backfill logic to migrate existing file paths
- ✅ Added `wanted` boolean to authors table
- ✅ Added `wanted` boolean to series table
- ✅ Verified `library_state` supports 'wanted' value
- ✅ Kept `file_path` NOT NULL (wanted books use empty string '')

### 2. M4B/M4A Test Files (Phase 4.1) - IN PROGRESS
**Script created**: `testdata/scripts/create_test_audiobooks.sh`
- ✅ Script creates M4B and M4A files from existing MP3s
- ✅ Adds proper chapter markers (one per MP3 file)
- ✅ Embeds full metadata: title, author, series, narrator, year
- ✅ AAC encoding at 64kbps for smaller test files
- 🔄 Currently generating test files for:
  - The Odyssey
  - Moby Dick
  - The Iliad
  - The Iliad (Version 2)

### 3. Plan Approved
Comprehensive implementation plan created with user approval includes:
- ✅ Multiple metadata sources (Open Library, Google Books, Audible, Goodreads)
- ✅ Unified search interface
- ✅ Wanted list management
- ✅ Enhanced duplicate detection
- ✅ State transitions
- ✅ Multi-path tracking
- ✅ Cover images

## 📋 Next Steps

### Phase 2: Metadata Provider Implementations
**Priority: HIGH**

Need to create in `internal/metadata/`:

1. **googlebooks.go** - Google Books API integration
   - API endpoint: `https://www.googleapis.com/books/v1/volumes`
   - Free tier: 1000 requests/day
   - Returns: cover images, descriptions, ISBN, dates
   - Requires API key configuration

2. **audible.go** - Audible integration
   - Unofficial API or web scraping
   - Returns: narrator, runtime, series info, ratings
   - Critical for audiobook-specific metadata

3. **goodreads.go** - Goodreads integration
   - Check API availability
   - Fallback: respectful web scraping
   - Returns: series info, ratings, recommendations

4. **aggregator.go** - Provider coordinator
   - Round-robin or priority-based provider selection
   - Result deduplication by ISBN/title+author
   - Merge metadata from multiple sources
   - Circuit breaker for failed providers
   - Health tracking

### Phase 2.2: Wanted Management API Endpoints
**File**: `internal/server/server.go`

New endpoints needed:
- `POST /api/v1/wanted/book` - Add book to wanted list
- `POST /api/v1/wanted/author` - Add author + all their books
- `POST /api/v1/wanted/series` - Add series ± author's works
- `GET /api/v1/wanted` - List all wanted items (categorized)
- `DELETE /api/v1/wanted/:id` - Remove from wanted list
- `POST /api/v1/wanted/:id/transition` - Manual state transitions
- `GET /api/v1/search/unified?q={query}` - Unified search across all sources

### Phase 2.3: Enhanced Duplicate Detection
**File**: `internal/scanner/scanner.go`

Modify `saveBookToDatabase()`:
- Check `audiobook_source_paths` table first
- Reject if exact source path match
- If hash exists but path is new: add to source_paths, don't create new book
- Return clear status: "duplicate_exact_path", "duplicate_new_path", "new"
- Emit SSE events for duplicate detection

**New endpoint**: `POST /api/v1/import/bulk-validate`
- Pre-validate file paths before import
- Return which files are duplicates
- Prevent accidental bulk duplicate imports

### Phase 3: Frontend UI
**Files needed**:

1. `web/src/pages/Search.tsx` - NEW
   - Single search box with real-time results
   - Categorized tabs: Books | Authors | Series
   - "Add to Wanted" buttons
   - Cascading add options (book → series → author)

2. `web/src/pages/Wanted.tsx` - NEW
   - Display all wanted items
   - Filter by type, sort by date/name
   - State transition actions
   - Auto-highlight when files found

3. `web/src/pages/Library.tsx` - UPDATE
   - Add "Wanted" filter option
   - Show wanted items with visual indicator
   - Auto-transition on scan completion

### Phase 4.2: Cover Images
**File**: `testdata/scripts/download_covers.sh` - NEW

- Fetch cover images from Open Library API
- Generate placeholder covers if unavailable
- Embed in M4B/M4A files
- Test cover extraction and display

### Phase 4.3: Duplicate Detection Tests
**File**: `web/tests/e2e/duplicate-detection.spec.ts` - NEW

Comprehensive test scenarios:
- Import same file 5 times → verify only 1 book
- Verify source_paths table has 5 entries
- Bulk import with duplicates
- UI notifications for duplicates

### Phase 5: State Transition Logic

Auto-transitions during scan:
- Match wanted books by hash with newly scanned files
- Transition wanted → imported automatically
- Notify user via UI

Manual transitions via API:
- wanted ↔ imported
- Any state → deleted (soft delete)

## 🏗️ Architecture Overview

### Database Schema
```
books (existing)
├─ library_state: 'wanted' | 'imported' | 'organized' | 'deleted'
├─ file_path: NULLABLE (empty string for wanted)
└─ wanted_metadata: JSON (for wanted books without files)

audiobook_source_paths (new)
├─ audiobook_id → books(id)
├─ source_path (UNIQUE)
├─ still_exists: BOOLEAN
├─ added_at, last_verified: TIMESTAMP

authors (updated)
└─ wanted: BOOLEAN (track if user wants all by author)

series (updated)
└─ wanted: BOOLEAN (track if user wants complete series)
```

### Metadata Flow
```
User Search
    ↓
Provider Aggregator
    ├─ Open Library API
    ├─ Google Books API
    ├─ Audible API/Scraper
    └─ Goodreads API/Scraper
    ↓
Deduplicate by ISBN/Title+Author
    ↓
Merge Best Metadata
    ↓
Return to UI with Source Attribution
```

### Import Flow (Enhanced)
```
File Selected
    ↓
Compute Hash
    ↓
Check Blocked Hashes → Skip if blocked
    ↓
Check audiobook_source_paths
    ├─ Exact path exists? → Reject ("duplicate_exact_path")
    ├─ Hash exists, new path? → Add to source_paths ("duplicate_new_path")
    └─ New hash? → Create book + first source_path entry ("new")
    ↓
Match against wanted items
    └─ If match found: Transition wanted → imported
    ↓
Emit SSE event with status
```

## 📊 Test Coverage Goals

### E2E Tests
- Unified search with all providers
- Add book/author/series to wanted list
- Cascading add options
- Import file → auto-match wanted item
- Duplicate detection (5x same file)
- Multi-path tracking verification
- M4B/M4A file handling
- Cover image display

### Unit Tests
- Provider aggregator logic
- Deduplication algorithms
- Hash collision handling
- State transition validation

### Integration Tests
- Database migrations
- Multi-path tracking
- Source path verification
- Wanted state queries

## 🎯 Success Criteria

1. ✅ User can search Open Library, Google Books, Audible, Goodreads simultaneously
2. ✅ User can add book/author/series to wanted list before having files
3. ✅ User sees wanted items in library with distinct visual indicator
4. ✅ System auto-matches imported files to wanted items by hash
5. ✅ System prevents duplicate imports (same hash, different paths)
6. ✅ System tracks all original source paths for each audiobook
7. ✅ M4B and M4A files work identically to MP3s
8. ✅ Cover images display correctly for all formats
9. ✅ State transitions work: wanted → imported → organized → deleted

## 📝 Notes

- Server is running on https://localhost:8484
- Migration 013 will run automatically on next server start
- M4B/M4A test files generating in background
- All changes are backward compatible
- No breaking API changes

## ⏱️ Estimated Time Remaining

- Metadata providers: 4-6 hours
- API endpoints: 2-3 hours
- Frontend UI: 6-8 hours
- Enhanced duplicate detection: 2-3 hours
- Tests: 4-5 hours
- Cover images: 1-2 hours

**Total**: ~20-27 hours of implementation work
