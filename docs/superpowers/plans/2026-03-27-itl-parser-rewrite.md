# ITL Parser Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the ITL binary parser to correctly handle iTunes v10+ format (little-endian, reversed chunk tags), add a precomputed `itunes_path` field to every book, and simplify write-back by removing XML write-back entirely.

**Architecture:** Split `itl.go` into three files: shared code (types, header, crypto), big-endian walker (pre-v10), and little-endian walker (v10+). Add `MaxCryptSize` to header parsing. Add `itunes_path TEXT` column via migration 38. Remove `writeback.go` (XML write-back). Update the write-back batcher to use precomputed `itunes_path`.

**Tech Stack:** Go, AES-128-ECB, zlib, SQLite migrations, PebbleDB JSON serialization

**Spec:** `docs/superpowers/specs/2026-03-27-itl-parser-rewrite-design.md`

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/itunes/itl.go` | Keep shared types/structs, header parsing (add `MaxCryptSize`), encryption (accept `*hdfmHeader`), compression, `ParseITL()` entry point with format detection dispatch, `UpdateITLLocations()` with format dispatch. Remove `walkChunks`, `walkHdsmContent`, `parseHtim`, `parseHohm`, `rewriteChunks`, `rewriteHdsmContent`, `shouldUpdateHohm`, `rewriteHohmLocation`. |
| Create | `internal/itunes/itl_be.go` | Big-endian chunk walker for pre-v10: `walkChunksBE()`, `walkHdsmContentBE()`, `parseHtimBE()`, `parseHohmBE()`, `rewriteChunksBE()`, `rewriteHdsmContentBE()`, `shouldUpdateHohmBE()`, `rewriteHohmLocationBE()` |
| Create | `internal/itunes/itl_le.go` | Little-endian chunk walker for v10+: `walkChunksLE()`, `parseMsdh()`, `parseMith()`, `parseMhoh()`, `rewriteChunksLE()`, `rewriteMsdhContent()` |
| Create | `internal/itunes/itl_le_test.go` | Tests for LE parsing and rewriting |
| Modify | `internal/itunes/itl_test.go` | Update existing tests for refactored function signatures |
| Delete | `internal/itunes/writeback.go` | XML write-back — dead code |
| Delete | `internal/itunes/writeback_test.go` | Tests for deleted code |
| Modify | `internal/itunes/fingerprint.go` | Keep `ErrLibraryModified` (used elsewhere) |
| Modify | `internal/database/migrations.go` | Migration 38: add `itunes_path TEXT` to books |
| Modify | `internal/database/store.go` | Add `ITunesPath *string` to Book struct |
| Modify | `internal/database/sqlite_store.go` | Add `itunes_path` to select/insert/update columns |
| Modify | `internal/database/pebble_store.go` | ITunesPath included via JSON tags automatically |
| Modify | `internal/server/itunes.go` | Store `itunes_path` during sync; fix `handleITunesWriteBackAll` to use `itunes_path`; remove `ErrLibraryModified` XML-related usage |
| Modify | `internal/server/itunes_writeback_batcher.go` | Remove XML write-back path, use `itunes_path` for ITL |
| Modify | `internal/server/metadata_fetch_service.go` | Compute `itunes_path` after organize/rename |

---

## Task 1: Add LE Helper Functions + Fix Header Parsing

**Files:**
- Modify: `internal/itunes/itl.go`

- [ ] **Step 1: Add little-endian read/write helpers**

Add to `itl.go` after the existing BE helpers:

```go
func readUint32LE(data []byte, offset int) uint32 {
	if offset+4 > len(data) { return 0 }
	return uint32(data[offset]) | uint32(data[offset+1])<<8 |
		uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
}

func readUint16LE(data []byte, offset int) uint16 {
	if offset+2 > len(data) { return 0 }
	return uint16(data[offset]) | uint16(data[offset+1])<<8
}

func writeUint32LE(buf []byte, offset int, val uint32) {
	buf[offset] = byte(val)
	buf[offset+1] = byte(val >> 8)
	buf[offset+2] = byte(val >> 16)
	buf[offset+3] = byte(val >> 24)
}
```

- [ ] **Step 2: Add `MaxCryptSize` to `hdfmHeader` struct**

```go
type hdfmHeader struct {
	headerLen       uint32
	fileLen         uint32
	unknown         uint32
	version         string
	headerRemainder []byte
	maxCryptSize    uint32 // from offset 92 in header
}
```

- [ ] **Step 3: Read `maxCryptSize` in `parseHdfmHeader`**

After the version string parsing (after `off += verLen`), add:

```go
// Read max_crypt_size at absolute offset 92 if header is large enough
if headerLen > 96 {
	hdr.maxCryptSize = readUint32BE(data, 92)
}
```

Where `hdr` is the return value being built.

- [ ] **Step 4: Change `itlDecrypt` signature to accept `*hdfmHeader`**

Change from `func itlDecrypt(version string, data []byte) []byte` to:

```go
func itlDecrypt(hdr *hdfmHeader, data []byte) []byte {
	if len(data) == 0 { return data }
	block, err := aes.NewCipher(itlAESKey)
	if err != nil { return data }
	bs := block.BlockSize()

	limit := len(data)
	if isVersionAtLeast(hdr.version, 10) {
		if hdr.maxCryptSize > 0 {
			limit = int(hdr.maxCryptSize)
		} else if limit > 102400 {
			limit = 102400 // fallback for old headers
		}
	}
	if limit > len(data) { limit = len(data) }
	limit = (limit / bs) * bs

	out := make([]byte, len(data))
	copy(out, data)
	for i := 0; i < limit; i += bs {
		block.Decrypt(out[i:i+bs], data[i:i+bs])
	}
	return out
}
```

- [ ] **Step 5: Update `itlEncrypt` signature similarly**

Change to accept `*hdfmHeader` and use `maxCryptSize`.

- [ ] **Step 6: Add format detection function**

```go
// detectLE checks if decompressed data uses little-endian msdh format (v10+)
func detectLE(data []byte) bool {
	if len(data) < 4 { return false }
	return string(data[0:4]) == "msdh"
}
```

- [ ] **Step 7: Update all callers of `itlDecrypt`/`itlEncrypt`**

In `parseITLData`, `UpdateITLLocations`, `InsertITLTracks`, `RewriteITLExtensions`, `InsertITLPlaylist` — pass `hdr` instead of `hdr.version`.

- [ ] **Step 8: Build and test**

Run: `go build ./internal/itunes/` and `go test ./internal/itunes/ -v`

- [ ] **Step 9: Commit**

```bash
git add internal/itunes/itl.go
git commit -m "feat: add LE helpers, read maxCryptSize from header, fix itlDecrypt signature"
```

---

## Task 2: Extract BE Walker to `itl_be.go`

**Files:**
- Create: `internal/itunes/itl_be.go`
- Modify: `internal/itunes/itl.go`

- [ ] **Step 1: Create `itl_be.go` with all BE functions**

Move these functions from `itl.go` to `itl_be.go` (rename with BE suffix):
- `walkChunks` → `walkChunksBE`
- `walkHdsmContent` → `walkHdsmContentBE`
- `parseHtim` → `parseHtimBE`
- `parseHohm` → `parseHohmBE`
- `parseHpim` → `parseHpimBE`
- `parseHptm` → `parseHptmBE`
- `parsePlaylistHohm` → `parsePlaylistHohmBE`
- `rewriteChunks` → `rewriteChunksBE`
- `rewriteHdsmContent` → `rewriteHdsmContentBE`
- `shouldUpdateHohm` → `shouldUpdateHohmBE`
- `rewriteHohmLocation` → `rewriteHohmLocationBE`

File header:
```go
// file: internal/itunes/itl_be.go
// version: 1.0.0
package itunes
```

These functions keep their exact implementation — only the names change. All use `readUint32BE`, `readUint16BE`, `readTag` (which reads bytes in order — works for both `hdsm` and `msdh` depending on what's in the data).

- [ ] **Step 2: Update `parseITLData` in `itl.go` to call `walkChunksBE`**

Replace `walkChunks(decompressed, lib)` with:

```go
if detectLE(decompressed) {
	walkChunksLE(decompressed, lib) // will be added in Task 3
} else {
	walkChunksBE(decompressed, lib)
}
```

Temporarily stub `walkChunksLE`:
```go
func walkChunksLE(data []byte, lib *ITLLibrary) {
	// TODO: implement in itl_le.go
}
```

- [ ] **Step 3: Update `UpdateITLLocations` to dispatch**

In the rewrite section, replace `rewriteChunks(decompressed, updateMap)` with:

```go
var newData []byte
var updatedCount int
if detectLE(decompressed) {
	newData, updatedCount = rewriteChunksLE(decompressed, updateMap)
} else {
	newData, updatedCount = rewriteChunksBE(decompressed, updateMap)
}
```

Temporarily stub `rewriteChunksLE`:
```go
func rewriteChunksLE(data []byte, updateMap map[string]string) ([]byte, int) {
	return data, 0 // TODO: implement in itl_le.go
}
```

- [ ] **Step 4: Remove old functions from `itl.go`**

Delete the original `walkChunks`, `walkHdsmContent`, `parseHtim`, `parseHohm`, `parseHpim`, `parseHptm`, `parsePlaylistHohm`, `rewriteChunks`, `rewriteHdsmContent`, `shouldUpdateHohm`, `rewriteHohmLocation` from `itl.go`.

- [ ] **Step 5: Build and test**

Run: `go build ./internal/itunes/` and `go test ./internal/itunes/ -v`
All existing tests should still pass (they use the BE format).

- [ ] **Step 6: Commit**

```bash
git add internal/itunes/itl.go internal/itunes/itl_be.go
git commit -m "refactor: extract BE chunk walker to itl_be.go"
```

---

## Task 3: Implement LE Walker in `itl_le.go`

**Files:**
- Create: `internal/itunes/itl_le.go`
- Create: `internal/itunes/itl_le_test.go`

- [ ] **Step 1: Write test — parse synthetic LE ITL data**

Create `itl_le_test.go` with a test that builds a minimal LE chunk structure and parses it:

```go
func TestWalkChunksLE_ParsesTracks(t *testing.T) {
	// Build minimal msdh container with one mith track and mhoh location
	data := buildSyntheticLEData(t)
	lib := &ITLLibrary{}
	walkChunksLE(data, lib)

	if len(lib.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(lib.Tracks))
	}
	if lib.Tracks[0].Location == "" {
		t.Error("expected non-empty location")
	}
}
```

The `buildSyntheticLEData` helper should construct:
- An `msdh` container (blockType=0x01) containing:
  - One `mith` track block with a known PID at offset 100
  - One `mhoh` metadata block (hohmType=0x0D) with a location string

All lengths and type fields use little-endian byte order.

- [ ] **Step 2: Write test — rewrite LE locations**

```go
func TestRewriteChunksLE_UpdatesLocation(t *testing.T) {
	data := buildSyntheticLEData(t)
	updateMap := map[string]string{
		testPIDHex: "file://localhost/W:/new/path.m4b",
	}
	newData, count := rewriteChunksLE(data, updateMap)
	if count != 1 {
		t.Fatalf("expected 1 update, got %d", count)
	}

	// Parse the rewritten data to verify
	lib := &ITLLibrary{}
	walkChunksLE(newData, lib)
	if lib.Tracks[0].Location != "file://localhost/W:/new/path.m4b" {
		t.Errorf("location not updated: %q", lib.Tracks[0].Location)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/itunes/ -run "TestWalkChunksLE|TestRewriteChunksLE" -v`
Expected: FAIL — functions not implemented

- [ ] **Step 4: Implement `walkChunksLE`**

```go
// walkChunksLE walks v10+ little-endian ITL data.
// Top-level chunks are msdh containers with blockType at offset 12.
func walkChunksLE(data []byte, lib *ITLLibrary) {
	offset := 0
	for offset+16 <= len(data) {
		tag := readTag(data, offset)
		if tag != "msdh" { break }

		_ = readUint32LE(data, offset+4) // headerLen (always 16 for msdh)
		totalLen := int(readUint32LE(data, offset+8))
		blockType := readUint32LE(data, offset+12)

		if totalLen < 16 || offset+totalLen > len(data) { break }

		contentStart := offset + 16 // msdh header is fixed at 16 bytes
		contentEnd := offset + totalLen

		switch blockType {
		case 0x01: // track list
			walkMsdhTracksLE(data, contentStart, contentEnd, lib)
		case 0x02: // playlist list
			walkMsdhPlaylistsLE(data, contentStart, contentEnd, lib)
		}

		offset += totalLen
	}
}
```

- [ ] **Step 5: Implement `walkMsdhTracksLE` and `parseMithLE` and `parseMhohLE`**

Parse `mith` tracks (same field layout as `htim` but read with LE functions) and `mhoh` metadata (same as `hohm` but LE lengths).

Key offsets in `mith` (same as `htim`):
- +16: TrackID, +32: DateModified, +36: Size, +40: TotalTime
- +76: PlayCount, +100: LastPlayDate, +108: Rating, +120: DateAdded
- +128: PersistentID (8 bytes, raw — NOT endian-dependent)

For `mhoh`: same layout as `hohm` but read lengths with `readUint32LE` instead of `readUint32BE`. The encoding flag and string data format are unchanged.

- [ ] **Step 5b: Implement playlist parsing in LE**

Add `walkMsdhPlaylistsLE` for blockType=0x02 containers. Parse `miph` (playlist header, LE equivalent of `hpim`) and `mtph` (playlist track reference, LE equivalent of `hptm`). Same struct fields as BE, just read lengths with LE functions.

- [ ] **Step 6: Implement `rewriteChunksLE`**

Same pattern as `rewriteChunksBE` but:
- Read tags and lengths with LE functions
- Walk `msdh` containers by blockType
- For track containers: iterate `mith` blocks, track PID, check `mhoh` for location updates
- Use `rewriteHohmLocationLE` which writes lengths as LE
- Update `msdh` container lengths after rewriting

- [ ] **Step 7: Run tests**

Run: `go test ./internal/itunes/ -run "TestWalkChunksLE|TestRewriteChunksLE" -v`
Expected: PASS

- [ ] **Step 8: Run all ITL tests**

Run: `go test ./internal/itunes/ -v`
Expected: ALL PASS (both BE and LE)

- [ ] **Step 9: Commit**

```bash
git add internal/itunes/itl_le.go internal/itunes/itl_le_test.go
git commit -m "feat: add little-endian chunk walker for v10+ ITL format"
```

---

## Task 4: Remove `walkChunksLE` / `rewriteChunksLE` Stubs + Delete XML Write-Back

**Files:**
- Modify: `internal/itunes/itl.go` — remove stubs
- Delete: `internal/itunes/writeback.go`
- Delete: `internal/itunes/writeback_test.go`
- Modify: `internal/server/itunes.go` — remove `ErrLibraryModified` XML usage
- Modify: `internal/server/itunes_writeback_batcher.go` — remove XML write-back path

- [ ] **Step 1: Remove stubs from `itl.go`**

Delete the temporary `walkChunksLE` and `rewriteChunksLE` stubs (they're now in `itl_le.go`).

- [ ] **Step 2: Delete writeback.go and writeback_test.go**

```bash
rm internal/itunes/writeback.go internal/itunes/writeback_test.go
```

- [ ] **Step 3: Verify `ErrLibraryModified` location**

Check if `ErrLibraryModified` is defined in `writeback.go` or `fingerprint.go`. If it's in `writeback.go`, move it to `fingerprint.go` before deleting `writeback.go`. The type and its `Error()` method must survive deletion.

- [ ] **Step 4: Fix compilation errors from deleted code**

Search for references to `WriteBack`, `WriteBackOptions`, `WriteBackResult`, `ValidateWriteBack`, `copyFile`, `writeLibrary` in the server package. Remove or update them.

In `internal/server/itunes.go`: find any usage of `ErrLibraryModified` related to XML write-back and remove it.

- [ ] **Step 4b: Update `itl_test.go` for signature changes**

The `itlDecrypt`/`itlEncrypt` signature changed in Task 1 to accept `*hdfmHeader`. Update any test in `itl_test.go` or `itl_writeback_test.go` that calls these functions directly.

- [ ] **Step 4: Simplify batcher — remove XML write-back**

In `internal/server/itunes_writeback_batcher.go`, in `flush()`:
- Remove the `xmlUpdates` variable and all XML-related code (lines 102-147 approximately)
- Remove the `itunes.WriteBack()` call
- Remove the fingerprint update after XML write
- Keep only the ITL write path (already there)

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add -A internal/itunes/ internal/server/itunes.go internal/server/itunes_writeback_batcher.go
git commit -m "feat: remove XML write-back, clean up stubs"
```

---

## Task 5: Add `itunes_path` Field — Migration + Store

**Files:**
- Modify: `internal/database/migrations.go`
- Modify: `internal/database/store.go`
- Modify: `internal/database/sqlite_store.go`

- [ ] **Step 1: Add migration 38**

In `migrations.go`, add:

```go
{
	Version:     38,
	Description: "Add itunes_path column to books table",
	Up: func(store Store) error {
		sqlStore, ok := store.(*SQLiteStore)
		if !ok { return nil }
		_, err := sqlStore.db.Exec("ALTER TABLE books ADD COLUMN itunes_path TEXT")
		return err
	},
},
```

- [ ] **Step 2: Add `ITunesPath` to Book struct**

In `store.go`, add to the Book struct after `ITunesImportSource`:

```go
ITunesPath         *string    `json:"itunes_path,omitempty"`
```

- [ ] **Step 3: Add `itunes_path` to `bookSelectColumns`**

In `sqlite_store.go`, add `itunes_path` to the column list (after `itunes_import_source`).

Also update `bookSelectColumnsQualified`.

- [ ] **Step 4: Update `scanBook` function**

Find the `scanBook` function that scans SQL rows into Book structs. Add `&b.ITunesPath` to the scan list in the correct position.

- [ ] **Step 5: Update `CreateBook` and `UpdateBook`**

In `CreateBook` and `UpdateBook` SQL statements, add `itunes_path` to the INSERT/UPDATE column and value lists.

- [ ] **Step 6: Build and test**

Run: `go build ./...` and `go test ./internal/database/ -v -count=1`

- [ ] **Step 7: Commit**

```bash
git add internal/database/migrations.go internal/database/store.go internal/database/sqlite_store.go
git commit -m "feat: add itunes_path column to books (migration 38)"
```

---

## Task 6: Store `itunes_path` During Sync + Fix Write-Back

**Files:**
- Modify: `internal/server/itunes.go`
- Modify: `internal/server/itunes_writeback_batcher.go`

- [ ] **Step 1: Store `itunes_path` during iTunes sync**

In `itunes.go`, find where books are updated during sync (the `executeITunesSync` function, where `store.UpdateBook` is called after matching a book with an iTunes PID). Before the update, set:

```go
existing.ITunesPath = &track.Location // track.Location is the raw file://localhost/... URL
```

This stores the original iTunes path for existing books during each sync.

- [ ] **Step 2: Fix `handleITunesWriteBackAll`**

Update the bulk write-back handler to:
1. Find the primary version for each PID (via version_group_id)
2. Read that version's `ITunesPath`
3. Use it directly as `NewLocation` — no `ReverseRemapPath` needed
4. Skip books where `ITunesPath` is nil/empty

- [ ] **Step 3: Fix batcher `flush()`**

In `itunes_writeback_batcher.go`, replace the per-book logic:

```go
// OLD: itunesPath := itunes.ReverseRemapPath(book.FilePath, pathMappings)
// NEW:
if book.ITunesPath == nil || *book.ITunesPath == "" {
	continue
}
itunesPath := *book.ITunesPath
```

Remove the pathMappings variable and `ReverseRemapPath` call.

- [ ] **Step 4: Build and test**

Run: `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/server/itunes.go internal/server/itunes_writeback_batcher.go
git commit -m "feat: store itunes_path during sync, use for write-back"
```

---

## Task 7: Compute `itunes_path` After Organize/Rename

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`

- [ ] **Step 1: Add `computeITunesPath` helper**

```go
// computeITunesPath converts a local file path to an iTunes file:// URL
// using the configured path mappings.
func computeITunesPath(localPath string) string {
	mappings := config.AppConfig.ITunesPathMappings
	for _, m := range mappings {
		// m.To = Linux prefix, m.From = Windows prefix
		if strings.HasPrefix(localPath, m.To) {
			windowsPath := m.From + localPath[len(m.To):]
			// URL-encode path segments
			encoded := url.PathEscape(windowsPath)
			// PathEscape encodes too aggressively — restore / and :
			encoded = strings.ReplaceAll(encoded, "%2F", "/")
			encoded = strings.ReplaceAll(encoded, "%3A", ":")
			return "file://localhost/" + encoded
		}
	}
	return "" // no mapping found
}
```

- [ ] **Step 2: Call after rename in `runApplyPipeline`**

After the book's `FilePath` is updated (around where `UpdateBook` is called with the new path), add:

```go
if itunesPath := computeITunesPath(book.FilePath); itunesPath != "" {
	book.ITunesPath = &itunesPath
}
```

- [ ] **Step 3: Call after rename in `RunApplyPipelineRenameOnly`**

Same as Step 2 — after `FilePath` is updated.

- [ ] **Step 4: Ensure organize sets `itunes_path` for already-organized books**

In the organize flow, when a book is already in the right place and doesn't move, still check if `ITunesPath` is empty and compute it if missing.

- [ ] **Step 5: Update organize preview to include `itunes_path`**

In `internal/server/organize_preview_service.go`, add the computed `itunes_path` to the preview response so the user can see what iTunes path will be set.

- [ ] **Step 6: Write test for `computeITunesPath`**

```go
func TestComputeITunesPath(t *testing.T) {
	// Set up config with path mapping
	config.AppConfig.ITunesPathMappings = []config.ITunesPathMap{
		{From: "W:/audiobook-organizer", To: "/mnt/bigdata/books/audiobook-organizer"},
	}
	tests := []struct{
		input, want string
	}{
		{"/mnt/bigdata/books/audiobook-organizer/Author/Title/file.m4b", "file://localhost/W:/audiobook-organizer/Author/Title/file.m4b"},
		{"/mnt/bigdata/books/audiobook-organizer/David Wong/Title/file.m4b", "file://localhost/W:/audiobook-organizer/David%20Wong/Title/file.m4b"},
		{"/some/other/path/file.m4b", ""}, // no matching mapping
	}
	for _, tt := range tests {
		got := computeITunesPath(tt.input)
		if got != tt.want {
			t.Errorf("computeITunesPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 7: Build and test**

Run: `go build ./...`

- [ ] **Step 8: Commit**

```bash
git add internal/server/metadata_fetch_service.go internal/server/organize_preview_service.go
git commit -m "feat: compute itunes_path after organize and rename"
```

---

## Task 8: Integration Test + Full Build

**Files:**
- Modify: `internal/itunes/itl_le_test.go`

- [ ] **Step 1: Add round-trip test with real-format LE data**

Test that builds a complete LE ITL structure (hdfm header + encrypted + compressed payload with msdh/mith/mhoh), parses it with `ParseITL`, and verifies tracks come back correctly.

- [ ] **Step 2: Add write-back round-trip test**

Test that writes a location update to the LE format and reads it back correctly.

- [ ] **Step 2b: Add `maxCryptSize` test**

Test that `itlDecrypt` uses `maxCryptSize` from header when set, and falls back to 102400 when not set.

- [ ] **Step 2c: Add LE playlist parsing test**

Test that `walkChunksLE` correctly parses `miph`/`mtph` blocks in a blockType=0x02 `msdh` container.

- [ ] **Step 3: Run all tests**

Run: `go test ./internal/itunes/ -v && go test ./internal/server/ -v && go test ./internal/database/ -v`

- [ ] **Step 4: Full build**

Run: `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/itunes/itl_le_test.go
git commit -m "test: add LE ITL round-trip integration tests"
```

---

## Summary

| Task | Description | Files | Steps |
|------|-------------|-------|-------|
| 1 | LE helpers + fix header/decrypt | `itl.go` | 9 |
| 2 | Extract BE walker to `itl_be.go` | `itl.go`, `itl_be.go` | 6 |
| 3 | Implement LE walker in `itl_le.go` | `itl_le.go`, `itl_le_test.go` | 9 |
| 4 | Remove stubs + delete XML write-back | `itl.go`, `writeback.go`, `itunes.go`, batcher | 6 |
| 5 | Migration 38 + `itunes_path` field | `migrations.go`, `store.go`, `sqlite_store.go` | 7 |
| 6 | Store `itunes_path` during sync + fix write-back | `itunes.go`, batcher | 5 |
| 7 | Compute `itunes_path` after organize/rename | `metadata_fetch_service.go` | 6 |
| 8 | Integration tests + full build | `itl_le_test.go` | 5 |
