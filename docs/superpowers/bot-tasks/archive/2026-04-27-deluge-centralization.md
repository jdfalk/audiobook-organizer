<!-- file: docs/superpowers/bot-tasks/2026-04-27-deluge-centralization.md -->
<!-- version: 1.0.0 -->
<!-- guid: 57f9a11c-460b-95b9-3627-9abc78912012 -->

# BOT TASK: 3.1-deluge — Wire `MoveStorage` into centralization path

**TODO ID:** 3.1-deluge
**Companion human design:** [`docs/superpowers/specs/2026-04-27-deluge-move-storage-integration-design.md`](../specs/2026-04-27-deluge-move-storage-integration-design.md)

## Branch

```
feat/3-1-deluge-centralization
```

## Files

Locate first:
```
grep -rn "centralization\|centralize" --include="*.go" internal/server/ | head -20
```

Likely:
- **Edit:** `internal/server/centralization*.go` (or `library_centralization*.go` — confirm with grep)
- **Read:** `internal/plugins/deluge/plugin.go`, `internal/deluge/client.go`
- **Read:** `internal/database/book_version.go` (for the `TorrentHash` field)
- **Edit:** the corresponding `*_test.go` for the centralization handler

## Step 1 — Inject the deluge plugin into the centralization handler

The centralization function needs access to the deluge plugin. If the surrounding `*Server` struct already has a `delugePlugin *deluge.Plugin` field, use it. If not, plumb one in via the constructor.

Search:
```
grep -rn "delugePlugin\|DelugePlugin" --include="*.go" internal/server/ | head
```

If no field exists, add one to `Server`:

```go
// in internal/server/server.go (or wherever Server is defined)
delugePlugin *delugeplugin.Plugin
```

And populate it in the constructor where other plugins are wired. **Do not invent a new construction path** — if the plugin is created somewhere already, just plumb the existing instance.

## Step 2 — Add the MoveStorage call

Find the function that performs the file move during centralization. After the move succeeds and the BookVersion has been updated to point at the new path, add:

```go
if s.cfg.DelugeMoveEnabled && bookVersion.TorrentHash != "" {
    if err := s.delugePlugin.MoveStorage(bookVersion.TorrentHash, newDir); err != nil {
        // Activity log surfaces the failure to the user; do NOT return the error
        // (centralization itself succeeded; the deluge call is best-effort per spec).
        s.activityWriter.Log(ctx, activity.Entry{
            Type:    "deluge-move-failed",
            Source:  "centralization",
            Level:   "warn",
            Message: fmt.Sprintf("deluge move_storage failed for %s: %v", bookVersion.TorrentHash, err),
        })
    }
}
```

Adjust field names to match the actual `activity.Entry` shape — read the package first.

## Step 3 — Test

Add to the existing centralization test file:

```go
func TestCentralization_CallsDelugeMoveStorage_WhenEnabled(t *testing.T) {
    fakeDeluge := &fakeDelugePlugin{}
    s := newTestServer(t, withDeluge(fakeDeluge), withConfig(&config.Config{DelugeMoveEnabled: true}))
    bv := &database.BookVersion{TorrentHash: "abc123", FilePath: "/old/path"}
    s.centralizeBookVersion(ctx, bv) // or whatever the method is called

    require.Equal(t, 1, fakeDeluge.MoveCount())
    require.Equal(t, "abc123", fakeDeluge.LastHash())
    require.Contains(t, fakeDeluge.LastDest(), ".versions") // or whatever the centralized layout uses
}

func TestCentralization_SkipsDelugeMoveStorage_WhenDisabled(t *testing.T) {
    fakeDeluge := &fakeDelugePlugin{}
    s := newTestServer(t, withDeluge(fakeDeluge), withConfig(&config.Config{DelugeMoveEnabled: false}))
    bv := &database.BookVersion{TorrentHash: "abc123", FilePath: "/old/path"}
    s.centralizeBookVersion(ctx, bv)

    require.Equal(t, 0, fakeDeluge.MoveCount())
}

func TestCentralization_SkipsDelugeMoveStorage_WhenNoTorrentHash(t *testing.T) {
    fakeDeluge := &fakeDelugePlugin{}
    s := newTestServer(t, withDeluge(fakeDeluge), withConfig(&config.Config{DelugeMoveEnabled: true}))
    bv := &database.BookVersion{TorrentHash: ""} // not torrent-sourced
    s.centralizeBookVersion(ctx, bv)

    require.Equal(t, 0, fakeDeluge.MoveCount())
}

func TestCentralization_DelugeError_DoesNotFailCentralization(t *testing.T) {
    fakeDeluge := &fakeDelugePlugin{moveErr: errors.New("deluge offline")}
    s := newTestServer(t, withDeluge(fakeDeluge), withConfig(&config.Config{DelugeMoveEnabled: true}))
    bv := &database.BookVersion{TorrentHash: "abc123"}
    err := s.centralizeBookVersion(ctx, bv)

    require.NoError(t, err) // best-effort; centralization succeeds
    // Verify a warn-level activity entry was emitted.
}
```

If `fakeDelugePlugin` doesn't exist, build a minimal one in the test file:

```go
type fakeDelugePlugin struct {
    moveErr  error
    lastHash string
    lastDest string
    moves    int
}
func (f *fakeDelugePlugin) MoveStorage(hash, dest string) error {
    f.moves++
    f.lastHash = hash
    f.lastDest = dest
    return f.moveErr
}
func (f *fakeDelugePlugin) MoveCount() int  { return f.moves }
func (f *fakeDelugePlugin) LastHash() string { return f.lastHash }
func (f *fakeDelugePlugin) LastDest() string { return f.lastDest }
```

## Step 4 — Verify

```
go vet ./...
make test
make ci
```

## Step 5 — Commit

```
feat(deluge): wire move_storage into centralization (TODO 3.1-deluge)

When centralizing a torrent-sourced book version into .versions/, call
deluge MoveStorage so the torrent client follows the new path. Gated by
DelugeMoveEnabled. Best-effort: errors logged via activity, centralization
succeeds regardless. Closes the deluge tail of TODO 3.1.

Spec: docs/superpowers/specs/2026-04-27-deluge-move-storage-integration-design.md
```

## Definition of done

- [ ] `MoveStorage` call exists in the centralization path, gated by `DelugeMoveEnabled` and non-empty `TorrentHash`
- [ ] All four test cases (enabled/disabled/no-hash/error) pass
- [ ] `make ci` green
- [ ] CHANGELOG prepended
- [ ] TODO.md `3.1-deluge` flipped to `[x]`; the `⏳` library-centralization plan in TODO.md has its deferred-deluge note removed (or pointed at this PR)

## When to STOP

NEEDS_REVIEW if:

- The centralization handler does not have an obvious "after move succeeds" hook; surface where the right insertion point lives.
- More than one place performs the centralization move (would mean the integration must duplicate). Surface the structural issue rather than wiring twice.
- The Server struct has no `delugePlugin` field and no obvious place to construct one from the existing config. Surface the wiring gap.
