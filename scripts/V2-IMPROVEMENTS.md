<!-- file: scripts/V2-IMPROVEMENTS.md -->
<!-- version: 1.0.0 -->
<!-- guid: d3e4f5a6-b7c8-9d0e-1f2a-3b4c5d6e7f8a -->

# Version 2 Script Improvements

## Overview

The V2 test script (`test-organize-import-v2.py`) provides a **book-centric**
view of your audiobook collection instead of a file-centric view, with
intelligent duplicate detection.

## Key Improvements Over V1

### 1. Book Grouping

**V1 Behavior:** Each file was treated as a separate entry

- 100 chapter files = 100 entries in the report
- Hard to see which files belong together
- Inflated book counts

**V2 Behavior:** Files are grouped by book

- 100 chapter files = 1 book entry with 100 files listed
- Clear view of multi-file audiobooks
- Accurate book counts

### 2. Duplicate/Version Detection

**V1 Behavior:** No duplicate detection

- Same book in different formats appeared as separate unrelated entries
- No way to identify redundant copies

**V2 Behavior:** Intelligent version linking

- Same book in different formats/qualities are linked together
- Each version shows:
  - Format (m4b, mp3, flac, etc.)
  - Number of files
  - Location/directory
- Duplicate groups have unique IDs (DUP0001, DUP0002, etc.)

### 3. Multi-File Book Handling

**V1 Behavior:** Each chapter file was a separate book

- Lost the connection between chapters
- Couldn't tell if files were complete

**V2 Behavior:** Chapters grouped under parent book

- Shows total file count per version
- Lists chapter numbers if detected
- Indicates if book is multi-file vs single-file

### 4. Better Statistics

**V1 Stats:**

- Total files processed
- Books with series

**V2 Stats:**

- Total books identified (accurate count)
- Multi-file books count
- Duplicate versions count
- Files per book
- Version information per book

## Example Comparison

### Same Data in V1 Output

```text
Book 1: /path/book/chapter01.mp3 - My Book
Book 2: /path/book/chapter02.mp3 - My Book
Book 3: /path/book/chapter03.mp3 - My Book
...
Book 15: /path/book/chapter15.mp3 - My Book
Book 16: /other/path/book.m4b - My Book
```

Result: 16 "books" reported (actually 1 book with 2 versions)

### Same Data in V2 Output

```text
[abc123] Author Name - My Book
  Series: My Series #1
  Confidence: high
  ⚠️  DUPLICATE: Group DUP0001 (2 versions)

  Version 1: .mp3 - 15 files (multi-file)
    Directory: /path/book/
    Files:
      - chapter01.mp3 (Chapter 1)
      - chapter02.mp3 (Chapter 2)
      - chapter03.mp3 (Chapter 3)
      ... and 12 more files

  Version 2: .m4b - 1 file
    Directory: /other/path/
    Files:
      - book.m4b
```

Result: 1 book with 2 versions (accurate)

## Use Cases

### When to Use V1

- Detailed file-level analysis
- Debugging filename patterns
- Testing extraction on individual files
- Quick validation of small samples

### When to Use V2 (Recommended)

- **Understanding your collection** - Get accurate book counts
- **Finding duplicates** - Identify redundant copies to clean up
- **Multi-file books** - See which books are split across multiple files
- **Large-scale analysis** - Process tens of thousands of files efficiently
- **Reporting** - Generate meaningful statistics about your library

## Technical Details

### Book Grouping Logic

1. Files in the same directory are grouped together
2. Chapter files are detected by pattern (Chapter 01, Track 01, etc.)
3. Metadata is extracted from the primary file (first non-chapter file)
4. All files in the group share the same book metadata

### Duplicate Detection Logic

1. Each book gets a unique ID based on normalized metadata:
   - Author name (normalized, lowercased)
   - Series name (normalized, lowercased)
   - Title (normalized, lowercased)

2. Books with matching IDs are linked as versions

3. Each version shows:
   - Unique version ID
   - Format and file count
   - Base directory
   - Complete file list

### Example Duplicate Group

Two versions of "The Martian" by Andy Weir:

```json
{
  "group_id": "DUP0042",
  "title": "The Martian",
  "author": "Andy Weir",
  "version_count": 2,
  "versions": [
    {
      "version_id": "abc123",
      "format": ".m4b",
      "file_count": 1,
      "directory": "/audiobooks/andy-weir/the-martian/"
    },
    {
      "version_id": "def456",
      "format": ".mp3",
      "file_count": 22,
      "directory": "/downloads/the-martian-mp3/"
    }
  ]
}
```

## Performance

- **V1:** Processes files one-by-one, can be slower for large collections
- **V2:** Groups files by directory first, more efficient for large datasets

### Recommended Limits

- **Testing:** Start with 10,000-20,000 files
- **Full run:** Can handle 100,000+ files (your ~187k file list)
- **Memory:** V2 uses slightly more memory to maintain group structures

## Command Reference

### Run V2 with 20,000 files

```bash
./scripts/test-organize-import-v2.py /path/to/file-list-books --limit 20000
```

### Run V2 with full list

```bash
./scripts/test-organize-import-v2.py /path/to/file-list-books
```

### Custom output file

```bash
./scripts/test-organize-import-v2.py /path/to/file-list-books \
  --limit 50000 \
  --output my-analysis.json \
  --sample 10
```

## Output Files

### V1 Output

- `organize-test-report.json` - File-centric list

### V2 Output

- `organize-test-report-v2.json` - Book-centric list with versions
- Includes `duplicates` section with grouped versions
- More detailed file information per book

## Next Steps

After running V2:

1. **Review duplicate groups** - Decide which versions to keep
2. **Check multi-file books** - Verify completeness
3. **Examine low-confidence books** - May need manual review
4. **Compare with Go implementation** - Validate extraction logic
5. **Use data to improve organizer** - Update patterns as needed
