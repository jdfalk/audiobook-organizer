<!-- file: docs/organizer-integration-checklist.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e -->

# Organizer Integration Checklist

## Smart Author Parsing Integration

When the organizer refactoring is complete, verify these integration points:

### Code Changes to Review

- [ ] Check `internal/organizer/organizer.go` for metadata extraction calls
- [ ] Verify it uses `metadata.ExtractMetadata()` (v1.3.0+)
- [ ] Verify it uses `scanner.extractInfoFromPath()` for fallback (v1.4.0+)
- [ ] Confirm directory skip lists are applied consistently

### Testing Requirements

- [ ] Run test script: `./scripts/test-organize-import-v3.py` with 10k+ files
- [ ] Verify author detection rate >40% (was 49.5% in testing)
- [ ] Check that "newbooks", "books", etc. are NOT appearing as authors
- [ ] Confirm "Title - Author" patterns extract correctly
- [ ] Test edge cases: single-word titles, multi-part names

### Specific Test Cases

Test these filename patterns:

- [ ] `Neural Wraith - K.D. Robertson.mp3` → Author: "K.D. Robertson"
- [ ] `Sarah J. Maas - A Court of Wings.mp3` → Author: "Sarah J. Maas"
- [ ] `Gone 02 - Michael Grant.mp3` → Author: "Michael Grant"
- [ ] `J. K. Rowling - Harry Potter.mp3` → Author: "J. K. Rowling"

### Build Verification

```bash
# Verify compilation
go build ./...

# Run tests
go test ./internal/metadata
go test ./internal/scanner
go test ./internal/organizer

# Integration test
go run . scan /path/to/test/audiobooks
```

### Expected Improvements

After integration, should see:

- Fewer books with "Unknown_Author"
- Higher confidence ratings overall
- Correct author extraction from filenames
- No "newbooks" as author names

### Documentation Updates

- [ ] Update README.md if metadata extraction behavior changed
- [ ] Update technical documentation with new parsing logic
- [ ] Add examples of supported filename patterns

### Code Review Focus Areas

When reviewing organizer changes, pay attention to:

1. **Metadata extraction order**: Tags → Filename parsing → Directory fallback
2. **Error handling**: What happens when all methods fail?
3. **Performance**: Smart parsing adds minimal overhead
4. **Edge cases**: Empty strings, special characters, unicode names

### Rollback Plan

If issues arise:

1. Revert to metadata.go v1.2.0 and scanner.go v1.2.0
2. Disable smart parsing by removing `parseFilenameForAuthor()` calls
3. Fall back to simple " - " splitting behavior

### Contact Points

- Smart parsing implementation: See `docs/smart-author-parsing.md`
- Test results: See `test-10k-v3-fixed.json`
- Python reference: `scripts/test-organize-import-v3.py`
