<!-- file: docs/superpowers/bot-tasks/2026-04-27-deluge-undo.md -->
<!-- version: 1.0.0 -->
<!-- guid: 680ab22d-571c-a6ca-4738-abcd89a23123 -->

# BOT TASK: 3.2-deluge — Wire `MoveStorage` into undo path

**TODO ID:** 3.2-deluge
**Companion human design:** [`docs/superpowers/specs/2026-04-27-deluge-move-storage-integration-design.md`](../specs/2026-04-27-deluge-move-storage-integration-design.md)
**Pattern reference:** [`3.1-deluge`](2026-04-27-deluge-centralization.md) — read it first; this task is the same shape applied to a different code path.

## Branch

```
feat/3-2-deluge-undo
```

## Files

Locate first:
```
grep -rn "undo\|Undo" --include="*.go" internal/server/ | head -20
```

Likely:
- **Edit:** `internal/server/undo*.go` (or `bulk_undo*.go`)
- **Read:** the centralization PR shipped under 3.1-deluge for the exact wiring pattern (`s.delugePlugin.MoveStorage(...)`)

## Approach

This task is the symmetric inverse of 3.1-deluge. After the file rename-back succeeds and the BookVersion's `FilePath` is restored to the original location, call:

```go
if s.cfg.DelugeMoveEnabled && bookVersion.TorrentHash != "" {
    if err := s.delugePlugin.MoveStorage(bookVersion.TorrentHash, restoredDir); err != nil {
        s.activityWriter.Log(ctx, activity.Entry{
            Type:    "deluge-move-failed",
            Source:  "undo",
            Level:   "warn",
            Message: fmt.Sprintf("deluge move_storage rollback failed for %s: %v", bookVersion.TorrentHash, err),
        })
    }
}
```

The `Server.delugePlugin` field, if newly added in 3.1-deluge, is reused here. **Do not duplicate the field**; if the bot is running 3.2-deluge before 3.1-deluge has merged, surface as a NEEDS_REVIEW (sequence violation).

## Test cases

Same four cases as 3.1-deluge: enabled / disabled / no-hash / deluge-error. The only difference: the destination is the **original** path being restored, not a centralized one. Test fixtures should reflect this:

```go
bv := &database.BookVersion{
    TorrentHash: "abc123",
    FilePath:    "/library/.versions/...", // current centralized
}
// Pre-load operation_changes table with original path "/library/Author/Title".
s.undoOperation(ctx, opID)

require.Equal(t, "/library/Author/Title", fakeDeluge.LastDest())
```

## Step-by-step

Identical to 3.1-deluge Step 1–5. Use the same `fakeDelugePlugin` pattern.

## Definition of done

- [ ] `MoveStorage` call exists in the undo path, gated by `DelugeMoveEnabled` and non-empty `TorrentHash`
- [ ] Tests cover all four cases including "destination is the restored original path, not the centralized path"
- [ ] `make ci` green
- [ ] CHANGELOG prepended
- [ ] TODO.md `3.2-deluge` flipped to `[x]`; bulk-organize-undo `⏳` plan loses the deferred note

## When to STOP

NEEDS_REVIEW if:

- 3.1-deluge has not yet merged. The Server's deluge wiring is required first; do not branch from main without it.
- The undo flow restores the path via a different mechanism than the rename-back assumed in the spec (e.g. a symlink swap, a bind mount). Adapt or surface.
- Undo is bulk — the function processes N changes in a transaction. Confirm `MoveStorage` is called per torrent-sourced change, not once for the whole bulk. Loop placement matters.
