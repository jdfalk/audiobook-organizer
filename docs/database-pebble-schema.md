<!-- file: docs/database-pebble-schema.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8f6e2c1b-7d4a-4f86-9f2a-5a6b7c8d9e0f -->

# PebbleDB Keyspace Schema and Data Model

This document defines the PebbleDB keyspace layout, entity models, and query
patterns for the Audiobook Organizer. PebbleDB is a sorted key–value store;
we design with prefix-based keys for efficient scans, atomic batch writes, and
forward-compatible JSON values.

## Design goals

- Human-debuggable, prefix-based keys (colon delimited)
- O(1) access for primary entities; prefix scans for collections and indices
- Separate logical audiobook metadata from physical file segments
- Preserve playback progress across multi-file → single-file merges
- Immutable playback event log with derived aggregates
- Built-in migration/versioning for the keyspace

## Conventions

- IDs: ULID strings (26-char Crockford base32) for time-sortable uniqueness
- Values: JSON with a `version` field for forward compatibility
- Timestamps: RFC3339 strings
- Booleans and numbers use native JSON types
- Secondary indices are separate keys pointing to primary IDs

## Key prefixes

Global/meta:

- `meta:` — global metadata and counters
- `mig:` — migration records (applied migrations)

Users/auth:

- `u:` — users
- `ua:` — user auth secrets/hashes
- `sess:` — sessions
- `pref:` — user preferences
- `authz:` — role/permission maps

Domain data:

- `a:` — authors
- `s:` — series
- `b:` — books (logical audiobooks)
- `bf:` — book file segments (physical media files)
- `bfi:` — book→segment ordering index

Indexes (examples):

- `idx:user:username:&lt;lower&gt;` → `&lt;userULID&gt;`
- `idx:user:email:&lt;lower&gt;` → `&lt;userULID&gt;`
- `idx:author:name:&lt;normalized&gt;` → `&lt;authorULID&gt;`
- `idx:series:name:&lt;normalized&gt;` → `&lt;seriesULID&gt;`
- `idx:series:author:&lt;authorULID&gt;:&lt;seriesULID&gt;` → `1`
- `idx:book:author:&lt;authorULID&gt;:&lt;bookULID&gt;` → `1`
- `idx:book:series:&lt;seriesULID&gt;:&lt;posPadded&gt;:&lt;bookULID&gt;` → `1`
- `idx:book:title:&lt;normalized&gt;:&lt;bookULID&gt;` → `1`
- `idx:book:tag:&lt;tagLower&gt;:&lt;bookULID&gt;` → `1`
- Future: `idx:book:genre:&lt;normalized&gt;:&lt;bookULID&gt;` → `1`

Playlists and playback:

- `pl:` — playlists
- `pli:` — playlist items (ordered)
- `playe:` — playback events (append-only)
- `playp:` — playback progress (latest snapshot)
- `stats:` — derived aggregates

Operations:

- `op:` — operations (scan, organize, transcode, merge)
- `opl:` — operation logs

## Entity JSON schemas (values)

Each entity JSON includes a `version` for forwards compatibility.

### User

Key: `u:<userULID>`
{
  "id": "&lt;ulid&gt;",
  "username": "...",
  "email": "...",
  "password_hash_algo": "argon2id",
  "password_hash": "base64...",
  "created_at": "RFC3339",
  "updated_at": "RFC3339",
  "roles": ["admin", "user"],
  "status": "active|disabled",
  "version": 1
}

Indexes:

- `idx:user:username:&lt;lowerUsername&gt;` → `&lt;userULID&gt;`
- `idx:user:email:&lt;lowerEmail&gt;` → `&lt;userULID&gt;`

### Session

Key: `sess:<sessionULID>`
{
  "id": "&lt;ulid&gt;",
  "user_id": "&lt;userULID&gt;",
  "created_at": "...",
  "expires_at": "...",
  "ip": "...",
  "user_agent": "...",
  "revoked": false,
  "version": 1
}

Optional index: `idx:sess:user:<userULID>:<sessionULID>` → `1`

### User preferences

Per-key approach (fine-grained updates):

- `pref:&lt;userULID&gt;:&lt;prefKey&gt;` → raw JSON/string value

### Author

Key: `a:<authorULID>`
{
  "id": "&lt;ulid&gt;",
  "name": "...",
  "normalized_name": "...",
  "created_at": "...",
  "version": 1
}
Index: `idx:author:name:&lt;normalizedName&gt;` → `&lt;authorULID&gt;`

### Series

Key: `s:<seriesULID>`
{
  "id": "&lt;ulid&gt;",
  "name": "...",
  "normalized_name": "...",
  "author_id": "&lt;authorULID&gt;|null",
  "created_at": "...",
  "version": 1
}
Indexes:

- `idx:series:name:&lt;normalizedName&gt;` → `&lt;seriesULID&gt;`
- `idx:series:author:&lt;authorULID&gt;:&lt;seriesULID&gt;` → `1`

### Book (logical)

Key: `b:<bookULID>`
{
  "id": "&lt;ulid&gt;",
  "title": "...",
  "normalized_title": "...",
  "author_id": "&lt;authorULID&gt;|null",
  "series_id": "&lt;seriesULID&gt;|null",
  "series_position": 1,
  "description": "...",
  "published_year": 0,
  "cover_asset_id": "&lt;assetULID&gt;|null",
  "tags": ["..."],
  "created_at": "...",
  "updated_at": "...",
  "version": 1
}
Indexes:

- `idx:book:author:&lt;authorULID&gt;:&lt;bookULID&gt;` → `1`
- `idx:book:series:&lt;seriesULID&gt;:&lt;posPadded&gt;:&lt;bookULID&gt;` → `1`
- `idx:book:title:&lt;normalizedTitle&gt;:&lt;bookULID&gt;` → `1`
- `idx:book:tag:&lt;tagLower&gt;:&lt;bookULID&gt;` → `1`

### Book file segment (physical)

Key: `bf:<segmentULID>`
{
  "id": "&lt;ulid&gt;",
  "book_id": "&lt;bookULID&gt;",
  "file_path": "...",
  "format": "m4b|mp3|flac|...",
  "size_bytes": 0,
  "duration_seconds": 0,
  "hash_sha256": "hex",
  "track_number": 1,
  "total_tracks": 10,
  "active": true,
  "superseded_by": "&lt;segmentULID&gt;|null",
  "created_at": "...",
  "updated_at": "...",
  "version": 1
}
Ordering index:

- `bfi:&lt;bookULID&gt;:&lt;segmentOrderPadded&gt;` → `&lt;segmentULID&gt;`

On merge multi-file → single-file:

- Create new `bf` record for merged file
- Mark old segments `active=false` and `superseded_by=&lt;newSeg&gt;`
- Migrate progress offsets (see Playback progress)

### Playlist

Key: `pl:<playlistULID>`
{
  "id": "&lt;ulid&gt;",
  "name": "...",
  "user_id": "&lt;userULID&gt;|null",
  "created_at": "...",
  "updated_at": "...",
  "version": 1
}
Index: `idx:playlist:user:<userULID>:<playlistULID>` → `1`

Playlist items (ordered):

- `pli:&lt;playlistULID&gt;:&lt;positionPadded&gt;` → `&lt;bookULID&gt;`

### Playback event (immutable)

Key: `playe:<userULID>:<bookULID>:<timestampULID>`
{
  "user_id": "&lt;userULID&gt;",
  "book_id": "&lt;bookULID&gt;",
  "segment_id": "&lt;segmentULID&gt;",
  "position_seconds": 0,
  "event_type": "progress|start|pause|complete",
  "play_speed": 1.0,
  "created_at": "...",
  "version": 1
}

### Playback progress (latest snapshot)

Key: `playp:<userULID>:<bookULID>`
{
  "user_id": "&lt;userULID&gt;",
  "book_id": "&lt;bookULID&gt;",
  "segment_id": "&lt;segmentULID&gt;",
  "position_seconds": 0,
  "percent_complete": 0.0,
  "updated_at": "...",
  "version": 1
}

Durations mapping for offset conversion (merge help):

- Key: `b:duration_map:&lt;bookULID&gt;`
{
  "segments": [
    { "id": "&lt;segmentULID&gt;", "duration": 0, "active": true, "offset_start": 0 }
  ],
  "total_duration": 0,
  "version": 1
}

### Stats aggregates (derived)

- `stats:book:plays:&lt;bookULID&gt;` → integer
- `stats:user:listen_seconds:&lt;userULID&gt;` → integer
- `stats:book:listen_seconds:&lt;bookULID&gt;` → integer

### Operations and logs

Operation: `op:<operationULID>`
{
  "id": "&lt;ulid&gt;",
  "type": "scan|organize|transcode|merge",
  "status": "pending|running|completed|failed",
  "started_at": "...",
  "completed_at": "...|null",
  "error": "...|null",
  "progress": { "current": 0, "total": 0 },
  "created_by": "&lt;userULID&gt;|system",
  "version": 1
}

Log: `opl:<operationULID>:<seqPadded>`
{
  "seq": 0,
  "timestamp": "...",
  "level": "info|warn|error",
  "message": "...",
  "version": 1
}

Maintain `op:<operationULID>:next_seq` counter for log sequencing.

### Migrations

Record: `mig:&lt;versionPadded&gt;` → { "id": number, "applied_at": "...", "description": "...", "duration_ms": number }
Current version: `meta:version` → number

## Query patterns

- Find user by username: `get(idx:user:username:<lower>)` → `userID`, then `get(u:<id>)`
- List series by author: scan `idx:series:author:<authorID>:`
- List books in series ordered: scan `idx:book:series:<seriesID>:`
- Segments for book: scan `bfi:<bookID>:` then fetch `bf:<segmentID>`
- Recent playback events: reverse-iterate `playe:<userID>:<bookID>:`
- Recent operations: scan `op:` (ULID provides time order)

## Write patterns & atomicity

- Use Pebble batches for atomic multi-key writes (entity + indices)
- Idempotent creation by checking conflict indices first
- Prefer write primary key first, indices next (or within same batch)

## Security

- Password hashing: Argon2id; parameters in `meta:auth:argon2_params`
- Sessions: store only hashed secret/token (optional); expire via `expires_at`
- Sweeper job to delete expired `sess:` keys periodically

## TTL / Compaction

- Playback events may be pruned after aggregation (keep last N days or last N events)
- Compaction job updates `stats:` aggregates before deleting old `playe:` keys

## Migration strategy

On startup:

1. Read `meta:version` (initialize to 0 if missing)
2. Apply code-based migrations sequentially (add indices, backfill maps)
3. Write `mig:&lt;version&gt;` records and bump `meta:version`

## Multi-file → single-file merge procedure

1. Create new merged segment `bf:<newSeg>`
2. Compute segment cumulative offsets from `b:duration_map:<bookID>`
3. For each `playp:<userID>:<bookID>` referencing old segments:
   - `newPosition = oldSegmentOffsetStart + oldPosition`
   - Update snapshot to `{ segment_id: <newSeg>, position_seconds: newPosition }`
4. Mark old segments `active=false` and set `superseded_by=<newSeg>`
5. Append `opl:` entries to document the change

## Future extensions

- Cover assets: `asset:<assetULID>` records referencing filesystem paths and mime
- Full-text search: external engine (Bleve/Meilisearch) fed by change log
- Multi-tenant prefixing: `tenant:<tenantID>:` prepend to all keys
- Encryption-at-rest: selective field-level encryption in JSON values
