<!-- file: docs/itunes-race-condition.md -->
<!-- version: 1.0.0 -->

# iTunes Library Race Condition

**Status:** Known issue, unresolved  
**Severity:** High — can cause data loss in the iTunes library  
**Date:** 2026-04-04

## The Problem

When Audiobook Organizer (AO) writes back to the iTunes Library.itl file, it reads
the current .itl, modifies track locations, and writes a new .itl. This is not atomic
with respect to iTunes itself.

### Race Scenario 1: iTunes adds a book while AO writes

```
T1: AO reads iTunes Library.itl (10,000 tracks)
T2: User adds a new book in iTunes (now 10,001 tracks in memory)
T3: AO writes modified .itl (10,000 tracks with updated locations)
T4: iTunes still has 10,001 tracks in memory
T5: When iTunes exits, it writes its version → AO's changes are lost
    OR
T5: iTunes re-reads the .itl → the new book vanishes
```

### Race Scenario 2: AO writes while iTunes is saving

```
T1: iTunes starts writing .itl (file partially written)
T2: AO reads the partially-written .itl → corrupt data or parse error
T3: AO writes its version → overwrites iTunes' partial write
T4: iTunes finishes its write → overwrites AO's version
```

### Race Scenario 3: Concurrent metadata edits

```
T1: User edits a book's title in iTunes
T2: AO writes back location changes to the same .itl
T3: AO's write doesn't include the title change (it only modifies locations)
T4: But if we do a full .itl rewrite, the title change in iTunes is lost
```

## Current Mitigations

1. **Backup before write**: We create `.bak` before modifying the .itl
2. **Validation after write**: We parse the written .itl to verify it's valid
3. **ACL fix**: We set proper permissions after writing so iTunes can read it
4. **Separate file**: AO writes to `.itunes-writeback/iTunes Library.itl`, not
   directly to the live iTunes library. The user must manually copy or iTunes
   must be configured to read from this path.

## Proposed Solutions

### Short-term: mtime-based conflict detection

Before writing:
1. Read the .itl file's mtime
2. Compare with the mtime we recorded during our last read
3. If mtime changed → someone else modified the file → abort and re-sync

```go
func (s *Server) safeWriteITL(itlPath string, updates []ITLLocationUpdate) error {
    stat, _ := os.Stat(itlPath)
    currentMtime := stat.ModTime()
    
    if s.lastITLReadMtime != nil && !currentMtime.Equal(*s.lastITLReadMtime) {
        return fmt.Errorf("ITL file modified since last read (expected %v, got %v) — re-sync required",
            s.lastITLReadMtime, currentMtime)
    }
    
    // Proceed with write...
    // After successful write, update lastITLReadMtime
}
```

### Medium-term: Track-count reconciliation

Before writing:
1. Parse the current .itl to get the current track count and PID set
2. Compare with our last known state
3. If new tracks appeared → merge them into our database before writing
4. If tracks disappeared → flag for investigation

### Long-term: Event-based sync via iTunes COM API

Instead of file-level read/write:
1. Use the iTunes COM API on Windows to subscribe to library change events
2. When iTunes adds/removes/modifies a track, AO gets notified immediately
3. AO pushes changes via COM API instead of .itl file manipulation
4. Eliminates file-level race conditions entirely

This requires the Windows-side automation harness (see `scripts/itunes-test-runner.ps1`).

### Alternative: Advisory file locking

The .itl file is on a ZFS volume shared via SMB. We could use:
1. **SMB opportunistic locks (oplocks)**: Request an exclusive oplock before writing
2. **Sidecar lock file**: Create `.itunes-writeback/iTunes Library.itl.lock` before writing
3. **Cross-platform advisory lock**: Use `flock()` on Linux (won't help with Windows iTunes)

SMB oplocks are the most practical since both AO (Linux) and iTunes (Windows) access the file via SMB.

## Recommendations

1. **Immediate**: Add mtime check before every write-back (low effort, catches most races)
2. **Next sprint**: Add track-count reconciliation (medium effort, catches the "new book" race)
3. **Future**: COM API integration eliminates the problem entirely (high effort, best solution)

## Related Files

- `internal/server/organize_service.go:writeBackITLLocations` — auto write-back after organize
- `internal/server/itunes.go:handleITunesWriteBackAll` — manual write-back endpoint
- `internal/itunes/itl.go:UpdateITLLocations` — the actual .itl rewrite
- `scripts/itunes-test-runner.ps1` — Windows COM API test harness (foundation for future COM integration)
