<!-- file: docs/superpowers/specs/2026-04-27-deluge-move-storage-integration-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 46e8090b-359a-84a8-2516-89ab67801f01 -->

# Deluge `move_storage` Integration — finishing 3.1 and 3.2

**TODO IDs:** 3.1 (centralization deluge tail), 3.2 (bulk-undo torrent tail)
**Audience:** human reviewer
**Companion bot recipes:** see [Bot tasks](#bot-tasks).
**Size:** Each sub-task is S — one PR, ~150 LOC.

## Status of prerequisites

Already built:
- `internal/deluge/client.go:214` — `MoveStorage(torrentIDs []string, destPath string) error`. Tested.
- `internal/plugins/deluge/plugin.go:93` — `Plugin.MoveStorage` wrapper that handles uninitialized-plugin case.
- `internal/database` — `BookVersion.TorrentHash` field; `GetBookVersionByTorrentHash` lookup.
- `internal/config/config.go:241` — `DelugeMoveEnabled` config flag (default false).

Not yet wired:
- Library centralization (3.1) does not call `MoveStorage` when it moves a torrent-sourced book into `.versions/`.
- Bulk-organize undo (3.2) does not call `MoveStorage` when it reverses a torrent move.

## Why this matters

Without `MoveStorage`, reorganizing a torrent breaks seeding — Deluge keeps watching the old path, sees the file gone, and either re-downloads or stops the torrent. Users with active seedboxes need this integration to use the centralization feature at all.

## Design decisions

**Gate everything behind `DelugeMoveEnabled`.** Defaults off. A user who runs without Deluge sees no behavior change. A user who has Deluge but doesn't want move-storage calls (perhaps they manage seeding manually) opts out.

**Best-effort, not transactional.** If Deluge is unreachable, log the failure and proceed with the file move anyway. The user's torrent client breakage is recoverable; blocking centralization on Deluge availability is worse UX.

**Pre-flight check is OK to skip.** A pure pre-flight ("is Deluge alive?") adds latency. Trust the call; handle the error if it fails.

**Use `BookVersion.TorrentHash`, not `Book.TorrentHash`.** Per the schema today, the source-of-truth is on BookVersion. A book may have multiple versions, only some torrent-sourced.

**Concurrent moves are serialized through Deluge's RPC.** No client-side locking needed; Deluge handles it.

## Wiring points

### 3.1 — Centralization (move into .versions/)

`internal/server/centralization*.go` (search for the file that owns the move flow). After the file rename succeeds and the BookVersion is updated to point at the new path, before returning success:

```go
if cfg.DelugeMoveEnabled && bookVersion.TorrentHash != "" {
    err := delugePlugin.MoveStorage(bookVersion.TorrentHash, newDir)
    if err != nil {
        log.Printf("[centralization] deluge move_storage failed for hash=%s: %v (continuing)", bookVersion.TorrentHash, err)
        // Activity log entry surfaces the failure to the user. Do not return error.
    }
}
```

### 3.2 — Undo organize

`internal/server/undo*.go` (or wherever bulk-organize undo lives). After the rename-back succeeds and BookVersion's path is restored:

```go
if cfg.DelugeMoveEnabled && bookVersion.TorrentHash != "" {
    err := delugePlugin.MoveStorage(bookVersion.TorrentHash, restoredDir)
    if err != nil {
        log.Printf("[undo] deluge move_storage failed for hash=%s: %v (continuing)", bookVersion.TorrentHash, err)
    }
}
```

## Risk

Low. The integration is gated behind the config flag. If `MoveStorage` returns an error we log and proceed, so the worst case is "torrent client desyncs" — exactly the state we have today before this integration ships. No regression possible.

## Out of scope

- A retry loop / persistent queue for failed moves. If a user wants belt-and-suspenders reliability, file a follow-up after the simple integration ships and gathers field experience.
- qBittorrent / Transmission integrations. Pluggable via the Plugin interface but not included here.

## Bot tasks {#bot-tasks}

| ID | Title | Bot recipe |
|---|---|---|
| **3.1-deluge** | Wire `MoveStorage` into centralization path | [`bot-tasks/2026-04-27-deluge-centralization.md`](../bot-tasks/2026-04-27-deluge-centralization.md) |
| **3.2-deluge** | Wire `MoveStorage` into undo path | [`bot-tasks/2026-04-27-deluge-undo.md`](../bot-tasks/2026-04-27-deluge-undo.md) |
