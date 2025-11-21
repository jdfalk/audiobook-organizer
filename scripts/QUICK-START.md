<!-- file: scripts/QUICK-START.md -->
<!-- version: 1.1.0 -->
<!-- guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e -->

# Quick Start: Test Audiobook Organizer

Run the test script to validate how the audiobook-organizer would process your
file collection.

## 🆕 Version 3 Script (RECOMMENDED - Intelligent Metadata Extraction)

The V3 script uses multiple strategies to extract accurate metadata:

1. **Embedded tags**: Reads ID3/M4A tags from audio files using ffprobe
2. **Smart filename parsing**: Extracts author from patterns like "Title -
   Author" or "Author - Title"
3. **Fallback to paths**: Uses directory structure as last resort

This multi-layered approach dramatically improves author detection even when
files lack embedded metadata tags.

### Test with 20,000 files (recommended starting point)

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
./scripts/test-organize-import-v3.py /Users/jdfalk/repos/scratch/file-list-books --limit 20000
```

### Test with full file list (~187k files)

```bash
./scripts/test-organize-import-v3.py /Users/jdfalk/repos/scratch/file-list-books
```

### Quick test with fewer files

```bash
./scripts/test-organize-import-v3.py /Users/jdfalk/repos/scratch/file-list-books --limit 10000
```

## Version 2 Script (Path-based grouping)

The V2 script groups files by book and detects duplicates, but uses
directory/filename parsing instead of reading tags.

```bash
./scripts/test-organize-import-v2.py /Users/jdfalk/repos/scratch/file-list-books --limit 20000
```

## Version 1 Script (File-by-file analysis)

The V1 script processes each file individually (useful for detailed file-level
analysis).

```bash
./scripts/test-organize-import.py /Users/jdfalk/repos/scratch/file-list-books --limit 1000
```

## What You'll Get (V2 Script)

1. **Console output** with summary statistics
2. **JSON report** with complete details (default:
   `organize-test-report-v2.json`)
3. **Book-centric view** with all files grouped by book
4. **Duplicate detection** showing all versions of the same book
5. **Sample book details** with version and file information

## Key Metrics to Watch (V2)

- **Total books identified** - Unique books found (not file count)
- **Multi-file books** - Books with multiple chapters/parts
- **Duplicate versions** - Same book in different formats/editions
- **Confidence levels** - High/medium/low accuracy ratings
- **Top authors/series** - Verify correct identification

## Example Output Snippet (V2)

```text
SUMMARY:
  Total books identified: 1,659
  Total files processed: 10,782
  Multi-file books: 352
  Books with duplicate versions: 15
  Unique duplicate groups: 5
  Books with series identified: 1,360

DUPLICATE GROUPS:
  DUP0001: Richard Morgan - Altered Carbon
    Versions: 3
      - .m4b (1 files) in /path/to/version1
      - .mp3 (15 files) in /path/to/version2
      - .flac (15 files) in /path/to/version3
```

## Next Steps

1. Review the JSON report for details
2. Check books with low confidence or issues
3. Compare results with actual Go organizer behavior
4. Update extraction patterns if needed

See [TEST-ORGANIZE-README.md](TEST-ORGANIZE-README.md) for complete
documentation.
