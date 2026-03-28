# ITL Parser Rewrite + iTunes Path Field Design

**Date:** 2026-03-27
**Status:** Approved

## Goal

Fix the ITL binary parser to correctly handle iTunes v10+ format (which uses little-endian byte order and reversed chunk signatures), add a precomputed `itunes_path` field to every book record, and simplify write-back by removing the XML write-back path entirely.

## Background

The current ITL parser (`internal/itunes/itl.go`) has 4 critical bugs when parsing v10+ ITL files:

1. **Wrong encryption limit** — hardcodes `102400` instead of reading `max_crypt_size` from hdfm header offset 92
2. **Wrong chunk signatures** — looks for big-endian tags (`hdsm`, `htim`, `hohm`) but v10+ uses reversed little-endian tags (`msdh`, `mith`, `mhoh`)
3. **Wrong byte order** — reads all integer fields as big-endian but v10+ uses little-endian
4. **Wrong container structure** — `msdh` blocks have a `blockType` field (0x01=tracks, 0x02=playlists) that the parser ignores

These bugs cause `ParseITL()` to return 0 tracks on any modern iTunes library (v10+, including the production v12.13.10.3 file).

Additionally, the ITL write-back cannot construct correct iTunes paths because it relies on reverse path mapping from the organized Linux path, which doesn't have a mapping configured. The XML write-back is unnecessary since iTunes never reads its own XML file.

### References

- [libitlp](https://github.com/jeanthom/libitlp) — C library for ITL parsing, documents `max_crypt_size` at header offset 92 and LE chunk format
- [mrexodia blog post 1](https://mrexodia.github.io/reversing/2014/12/16/iTunes-Library-Format-1) — AES-128-ECB details, encryption boundary from offset 0x5C (92 decimal)
- [mrexodia blog post 2](https://mrexodia.github.io/reversing/2014/12/27/iTubes-Library-Format-2) — Post-decompression `msdh` container structure
- [mrexodia UFWB grammar](https://gist.github.com/mrexodia/0e0ddec9460e6aaca43f) — Complete ITL structure definition

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Parser file structure | Split into `itl.go` (shared), `itl_be.go` (pre-v10), `itl_le.go` (v10+) | Clean separation, each format testable independently |
| Path storage | `itunes_path` field on every book record | Precomputed at sync/organize time, no reverse calculation at write time |
| XML write-back | Remove entirely | iTunes never reads XML, only writes it. Dead code path. |
| Path mapping | Single config field: organizer root ↔ Windows root | Minimal config, covers the one mapping needed |
| Playlist parsing | Include in rewrite | Same chunk walker fixes apply, marginal extra work |

## Architecture

### ITL Parser File Split

```
internal/itunes/itl.go       — Shared types, structs, header parsing, encryption,
                                compression, ParseITL() entry point, UpdateITLLocations()
                                entry point. Detects version and dispatches to BE or LE.

internal/itunes/itl_be.go    — Pre-v10 big-endian format:
                                walkChunksBE(), parseHtim(), parseHohm(),
                                parseHpim(), parseHptm(), rewriteChunksBE()

internal/itunes/itl_le.go    — v10+ little-endian format:
                                walkChunksLE(), parseMith(), parseMhoh(),
                                parseMiph(), parseMtph(), rewriteChunksLE()
```

### ITL Binary Format — v10+ (Little-Endian)

```
┌─────────────────────────────────────────────────────────┐
│ hdfm header (big-endian always)                         │
│   offset 0:  "hdfm" magic                              │
│   offset 4:  header length (uint32 BE)                  │
│   offset 8:  file length (uint32 BE)                    │
│   offset 92: max_crypt_size (uint32 BE)                 │
│   offset 16: version string (e.g., "12.13.10.3")       │
├─────────────────────────────────────────────────────────┤
│ Encrypted payload (AES-128-ECB, first max_crypt_size    │
│ bytes only, rest is plaintext)                          │
│   Key: "BHUILuilfghuila3"                               │
├─────── after decryption ────────────────────────────────┤
│ Zlib-compressed data                                    │
├─────── after decompression ─────────────────────────────┤
│ msdh containers (little-endian)                         │
│   ┌─ msdh ──────────────────────┐                       │
│   │ sig: "msdh" (4)             │                       │
│   │ headerLen (uint32 LE)       │                       │
│   │ totalLen (uint32 LE)        │                       │
│   │ blockType (uint32 LE)       │                       │
│   │   0x01 = track list         │                       │
│   │   0x02 = playlist list      │                       │
│   │ sub-blocks:                 │                       │
│   │   mith (track)              │                       │
│   │   mhoh (metadata)           │                       │
│   │   miph (playlist)           │                       │
│   │   mtph (playlist item)      │                       │
│   └─────────────────────────────┘                       │
└─────────────────────────────────────────────────────────┘
```

### Chunk Tag Mapping (BE ↔ LE)

| Pre-v10 (BE) | v10+ (LE) | Meaning |
|--------------|-----------|---------|
| `hdsm` | `msdh` | Master section data handler (container) |
| `htim` | `mith` | Track item |
| `hohm` | `mhoh` | Metadata (name, artist, location, etc.) |
| `hpim` | `miph` | Playlist item |
| `hptm` | `mtph` | Playlist track reference |

### Encryption Fix

**Current (broken):**
```go
if isVersionAtLeast(version, 10) {
    if limit > 102400 {
        limit = 102400
    }
}
```

**Fixed:** Add `MaxCryptSize uint32` to the `hdfmHeader` struct, populated in `parseHdfmHeader()` from offset 92 (uint32 BE). Change `itlDecrypt` signature from `(version string, data []byte)` to `(hdr *hdfmHeader, data []byte)` so it has access to the parsed `MaxCryptSize`. If `MaxCryptSize > 0`, use it instead of the hardcoded 102400. Fall back to 102400 only if the header is too short to contain offset 92.

### ParseITL Flow

```go
func ParseITL(path string) (*ITLLibrary, error) {
    data := readFile(path)
    hdr := parseHdfmHeader(data)       // Always BE
    payload := data[hdr.headerLen:]
    decrypted := itlDecrypt(hdr, payload)  // Uses max_crypt_size from hdr
    decompressed := itlInflate(decrypted)

    lib := &ITLLibrary{}
    if detectLE(decompressed) {        // Check for "msdh" at start
        walkChunksLE(decompressed, lib)
    } else {
        walkChunksBE(decompressed, lib)
    }
    return lib, nil
}
```

### UpdateITLLocations Flow

Same dispatch pattern — detect format after decompression, use `rewriteChunksBE()` or `rewriteChunksLE()` accordingly. The rewrite functions match PIDs and replace location strings in the appropriate byte order.

## `itunes_path` Field

### Database

**Migration** adds `itunes_path TEXT` to books table:
```sql
ALTER TABLE books ADD COLUMN itunes_path TEXT;
```

**Book struct** gets `ITunesPath *string` field.

### Format

`itunes_path` stores a **percent-encoded `file://localhost/` URL** — the exact format iTunes uses internally in both XML and ITL. Example: `file://localhost/W:/audiobook-organizer/David%20Wong/Title/file.m4b`. This format is used consistently everywhere: stored from XML as-is, computed for organized books by building the Windows path then prepending `file://localhost/` and URL-encoding path segments.

### When It Gets Set

| Event | Action |
|-------|--------|
| iTunes sync/import | Store `<Location>` value from XML directly (already in `file://localhost/W:/...` format) |
| After organize | Compute from organized path + `itunes_organizer_root_mapping` config |
| After rename | Recompute from new path + mapping |
| Full organize (already organized) | Check if field is empty, compute if missing |
| Primary version change | No recomputation — each version has its own `itunes_path` |

### Path Computation

```
Linux path:  /mnt/bigdata/books/audiobook-organizer/David Wong/Title/file.m4b
Mapping:     /mnt/bigdata/books/audiobook-organizer → W:/audiobook-organizer
Result:      file://localhost/W:/audiobook-organizer/David%20Wong/Title/file.m4b
```

### Config

Reuse the existing `ITunesPathMappings []ITunesPathMap` array. The user adds a second entry for the organizer root:

```json
"itunes_path_mappings": [
  {"from": "W:/itunes/iTunes Media", "to": "/mnt/bigdata/books/itunes/iTunes Media"},
  {"from": "W:/audiobook-organizer", "to": "/mnt/bigdata/books/audiobook-organizer"}
]
```

No new config fields needed. The path computation function iterates all mappings to find the matching prefix. `From` = Windows/iTunes path prefix, `To` = Linux path prefix (matching the existing convention).

## Write-Back Simplification

### Remove XML Write-Back

Delete from codebase:
- `internal/itunes/writeback.go` — `WriteBack()`, `WriteBackOptions`, `WriteBackResult`
- `internal/itunes/writeback_test.go` — tests for XML write-back
- XML write-back code path in `WriteBackBatcher.flush()`

### `rewriteChunksLE` Behavior

The LE rewrite mirrors `rewriteChunksBE` but with reversed tags and LE byte order:

1. Walk `msdh` containers reading lengths with `readUint32LE`
2. Inside track-list containers (blockType=0x01), iterate `mith` blocks
3. Track each `mith`'s PID from offset 100 (same as BE, the PID bytes are a fixed 8-byte field)
4. For each `mhoh` metadata block after a `mith`, check if `hohmType` (at LE offset 12) is `0x0D` (file location) or `0x0B` (local URL)
5. If the current PID matches the update map, rebuild the `mhoh` block with the new location string using `encodeHohmString` (same encoding as BE — the string encoding is independent of chunk byte order)
6. Update the `msdh` container length fields after rewriting

The key difference from BE: all length/type fields are read and written as little-endian. The `mhoh` string encoding (UTF-16BE with encoding flag byte) remains the same since that's a string property, not a chunk structure property.

### Simplified Write-Back Flow

**`POST /api/v1/itunes/write-back-all`:**
1. Query all books with `itunes_persistent_id` set
2. For each PID, find the primary version in the version group
3. Read that version's `itunes_path` — skip if empty
4. Build `ITLLocationUpdate{PersistentID, NewLocation: itunes_path}`
5. Call `UpdateITLLocations()`

**`WriteBackBatcher.flush()`:**
1. For each pending book ID, read `itunes_path` from DB
2. Build ITL update
3. Call `UpdateITLLocations()`
4. No XML write-back step

## Files Affected

### Create
| File | Purpose |
|------|---------|
| `internal/itunes/itl_be.go` | Big-endian chunk walker (pre-v10), extracted from itl.go |
| `internal/itunes/itl_le.go` | Little-endian chunk walker (v10+), new implementation |

### Modify
| File | Change |
|------|--------|
| `internal/itunes/itl.go` | Keep shared types/header/crypto. Remove inline chunk walkers. Add format detection + dispatch. Fix `itlDecrypt` to read `max_crypt_size` from header. |
| `internal/itunes/itl_writeback_test.go` | Tests for both BE and LE write-back |
| `internal/database/migrations.go` | Migration 38: add `itunes_path TEXT` to books |
| `internal/database/store.go` | Add `ITunesPath` to Book struct |
| `internal/database/sqlite_store.go` | Include `itunes_path` in queries |
| `internal/database/pebble_store.go` | Include `itunes_path` in serialization |
| `internal/server/itunes.go` | Store `itunes_path` during sync, use for write-back-all |
| `internal/server/itunes_writeback_batcher.go` | Remove XML path, use `itunes_path` for ITL |
| `internal/server/metadata_fetch_service.go` | Compute `itunes_path` in rename pipeline (`runApplyPipeline`, `RunApplyPipelineRenameOnly`) |
| `internal/server/organize_preview_service.go` | Include `itunes_path` in organize preview |
| `internal/server/itunes.go` | Remove `ErrLibraryModified` usage (was from writeback.go). Move or remove fingerprint check. |

### Delete
| File | Reason |
|------|--------|
| `internal/itunes/writeback.go` | XML write-back — iTunes never reads XML |
| `internal/itunes/writeback_test.go` | Tests for deleted code |

## Out of Scope

- Playlist data model / sync / smart playlists (separate brainstorming session)
- Bidirectional play count sync (separate feature)
- External library abstraction layer (future spec after second library format is added)
- Empty folder cleanup (separate maintenance task)
