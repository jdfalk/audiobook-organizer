<!-- file: scripts/QUICK-START.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e -->

# Quick Start: Test Audiobook Organizer

Run the test script to validate how the audiobook-organizer would process your file collection.

## One-Line Test Commands

### Test with full file list (~187k files)

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
./scripts/test-organize-import.py /Users/jdfalk/repos/scratch/file-list-books
```

### Quick test (first 1000 files)

```bash
./scripts/test-organize-import.py /Users/jdfalk/repos/scratch/file-list-books --limit 1000
```

### Medium test (10,000 files)

```bash
./scripts/test-organize-import.py /Users/jdfalk/repos/scratch/file-list-books --limit 10000 --output test-10k.json
```

## What You'll Get

1. **Console output** with summary statistics
2. **JSON report** with complete details (default: `organize-test-report.json`)
3. **Sample book details** showing extraction results

## Key Metrics to Watch

- **Books with series identified** - How many were recognized as part of a series
- **Confidence levels** - High/medium/low accuracy ratings
- **Top authors/series** - Verify correct identification
- **Common issues** - Problems to address

## Example Output Snippet

```text
SUMMARY:
  Total files processed: 186,736
  Books with series identified: 150,234
  Books with series position: 95,678

CONFIDENCE LEVELS:
  High confidence: 120,456
  Medium confidence: 45,890
  Low confidence: 20,390

TOP 10 AUTHORS:
  Brandon Sanderson: 125 books
  Timothy Zahn: 98 books
  ...
```

## Next Steps

1. Review the JSON report for details
2. Check books with low confidence or issues
3. Compare results with actual Go organizer behavior
4. Update extraction patterns if needed

See [TEST-ORGANIZE-README.md](TEST-ORGANIZE-README.md) for complete documentation.
