<!-- file: docs/smart-author-parsing.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d -->

# Smart Author Parsing Implementation

## Overview

Enhanced the audiobook organizer to intelligently extract author information from filenames with patterns like "Title - Author" or "Author - Title", instead of blindly treating the first part as a series name.

## Problem

Previously, filenames like:

- `Neural Wraith - K.D. Robertson.mp3`
- `Sarah J. Maas - A Court of Wings and Ruin.mp3`

Would incorrectly extract:

- Title: "K.D. Robertson" or "A Court of Wings and Ruin"
- Author: "newbooks" (from directory name)
- Series: "Neural Wraith" or "Sarah J. Maas"

## Solution

Added intelligent parsing that:

1. Detects patterns with " - " separator
2. Uses heuristics to identify which part is the author name
3. Looks for proper name patterns (capitalized words, initials with periods, etc.)
4. Correctly assigns Title and Author fields

## Files Modified

### Go Implementation

1. **internal/metadata/metadata.go** (v1.2.0 → v1.3.0)
   - Added `parseFilenameForAuthor()` function
   - Added `looksLikePersonName()` helper
   - Enhanced `extractFromFilename()` to use smart parsing
   - Added skip list for common directory names ("newbooks", "books", etc.)

2. **internal/scanner/scanner.go** (v1.2.0 → v1.4.0)
   - Added `parseFilenameForAuthor()` function
   - Added `looksLikePersonName()` helper
   - Enhanced `extractInfoFromPath()` to use smart parsing
   - Added skip list for common directory names

### Python Test Script

**scripts/test-organize-import-v3.py** (v1.0.0 → v1.1.0)

- Added `PathExtractor.parse_title_author_from_filename()` method
- Enhanced metadata extraction to use smart parsing
- Added SKIP_DIRS list for directory filtering

## Results

With 10k file test:

- **Before**: 1 author detected (0.1%)
- **After**: 392 authors detected (49.5%)
- **Confidence**: 100 books now have "medium" confidence (up from 0)

Examples of correct parsing:

- "Neural Wraith - K.D. Robertson" → Title: "Neural Wraith", Author: "K.D. Robertson"
- "Sarah J. Maas - A Court of Wings and Ruin" → Title: "A Court...", Author: "Sarah J. Maas"
- "Michael Grant - Gone 02 - Hunger" → Title: "Gone 02 - Hunger", Author: "Michael Grant"

## Integration Points

When the organizer code is updated/refactored, ensure:

1. **Metadata extraction** uses the enhanced `metadata.ExtractMetadata()` function
2. **Path parsing** uses the enhanced `scanner.extractInfoFromPath()` function
3. **Directory filtering** respects the skip lists for common directory names
4. **Fallback behavior** still works when patterns aren't detected

## Testing

To verify the changes work:

```bash
# Run the Python test script
./scripts/test-organize-import-v3.py /path/to/file-list --limit 10000

# Build and test Go code
go build ./internal/metadata ./internal/scanner
go test ./internal/metadata ./internal/scanner
```

## Pattern Detection Heuristics

The code identifies author names by looking for:

- Initials with periods: "J. K. Rowling", "K.D. Robertson"
- Multiple capitalized words: "Sarah J. Maas", "Michael Grant"
- Proper name format: "FirstName LastName" (2-4 words, all capitalized)

When both sides of " - " look like names, defaults to "Title - Author" pattern.

## Skipped Directory Names

The following directory names are ignored when extracting author from path:

- books, audiobooks, newbooks
- downloads, media, audio
- library, collection
- bt, incomplete, data

This prevents "newbooks" from being used as an author name.
