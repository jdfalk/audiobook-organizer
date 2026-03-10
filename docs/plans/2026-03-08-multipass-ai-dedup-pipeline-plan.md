# Multi-Pass AI Author Dedup Pipeline — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the groups/full/combined AI review modes with a single automated multi-pass pipeline using two models, enrichment, cross-validation via logic tree, scan history, and author tombstones.

**Architecture:** Separate PebbleDB (`ai_scans.db`) stores scan history and raw I/O. A pipeline manager coordinates 4 phases (parallel scan → enrich → cross-validate → present). Author tombstones make merged IDs permanent redirects. Frontend simplifies to Authors/Books tabs with scan progress and history sidebar.

**Tech Stack:** Go 1.24, PebbleDB, OpenAI Batch API + real-time, React/TypeScript/MUI, gin router

**Design doc:** `docs/plans/2026-03-08-multipass-ai-dedup-pipeline-design.md`

---

## Task Dependency Graph

```
Task 1: AI Scan Store (PebbleDB)
Task 2: Author Tombstones (PebbleStore)     ← independent of Task 1
Task 3: Scan Data Types & Store Interface    ← depends on Task 1
Task 4: Pipeline Manager (phase transitions) ← depends on Task 3
Task 5: Batch API Integration               ← depends on Task 4
Task 6: Cross-Validation Logic Tree          ← depends on Task 3
Task 7: API Endpoints                        ← depends on Tasks 4, 5, 6
Task 8: Frontend - API Client               ← depends on Task 7
Task 9: Frontend - AI Authors Tab            ← depends on Task 8
Task 10: Frontend - Scan History Sidebar     ← depends on Task 8
Task 11: Wire Up & Integration Test          ← depends on all
```

---

### Task 1: AI Scan Store (PebbleDB)

Create a separate PebbleDB for scan data, following the OpenLibrary pattern.

**Files:**
- Create: `internal/database/ai_scan_store.go`
- Create: `internal/database/ai_scan_store_test.go`
- Modify: `internal/server/server.go` (open DB on startup, close on shutdown)

**Step 1: Write the failing test**

```go
// file: internal/database/ai_scan_store_test.go
package database

import (
    "os"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestNewAIScanStore(t *testing.T) {
    tmpdir := t.TempDir()
    store, err := NewAIScanStore(tmpdir + "/ai_scans.db")
    require.NoError(t, err)
    require.NotNil(t, store)
    defer store.Close()
}
```

**Step 2: Run test to verify it fails**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/database/ -run TestNewAIScanStore -v`
Expected: FAIL — `NewAIScanStore` undefined

**Step 3: Write minimal implementation**

```go
// file: internal/database/ai_scan_store.go
package database

import (
    "fmt"
    "log"
    "strconv"

    "github.com/cockroachdb/pebble/v2"
)

type AIScanStore struct {
    db *pebble.DB
}

func NewAIScanStore(path string) (*AIScanStore, error) {
    db, err := pebble.Open(path, &pebble.Options{
        FormatMajorVersion: pebble.FormatNewest,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to open AI scan DB: %w", err)
    }

    store := &AIScanStore{db: db}

    counters := []string{"scan", "scan_result"}
    for _, counter := range counters {
        key := fmt.Sprintf("counter:%s", counter)
        if _, closer, err := db.Get([]byte(key)); err == pebble.ErrNotFound {
            if err := db.Set([]byte(key), []byte("1"), pebble.Sync); err != nil {
                db.Close()
                return nil, fmt.Errorf("failed to initialize counter %s: %w", counter, err)
            }
        } else if err == nil {
            closer.Close()
        }
    }

    log.Printf("[INFO] AI Scan DB opened at %s", path)
    return store, nil
}

func (s *AIScanStore) Close() error {
    return s.db.Close()
}

func (s *AIScanStore) nextID(counter string) (int, error) {
    key := []byte(fmt.Sprintf("counter:%s", counter))
    value, closer, err := s.db.Get(key)
    if err != nil {
        return 0, err
    }
    defer closer.Close()

    id, err := strconv.Atoi(string(value))
    if err != nil {
        return 0, err
    }

    if err := s.db.Set(key, []byte(strconv.Itoa(id+1)), pebble.Sync); err != nil {
        return 0, err
    }
    return id, nil
}
```

**Step 4: Run test to verify it passes**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/database/ -run TestNewAIScanStore -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/database/ai_scan_store.go internal/database/ai_scan_store_test.go
git commit -m "feat: add AI scan store (PebbleDB) skeleton"
```

---

### Task 2: Author Tombstones

Add tombstone support to PebbleStore so merged author IDs permanently redirect.

**Files:**
- Modify: `internal/database/pebble_store.go` (add tombstone methods)
- Modify: `internal/database/pebble_store_test.go` (add tombstone tests)
- Modify: `internal/server/server.go` (add tombstone creation in merge handlers, ~lines 3000 and 6847)
- Modify: `internal/server/scheduler.go` (add tombstone chain resolution task)
- Modify: `internal/itunes/` (update iTunes sync to follow tombstones — when sync encounters a tombstoned author ID in its mapping, follow the redirect instead of creating a duplicate or updating a deleted record)
- Modify: library scan / import path scan code (when re-scanning finds an author that was tombstoned, resolve to canonical ID instead of creating a new author record)

**Step 1: Write the failing test**

```go
// Add to internal/database/pebble_store_test.go

func TestPebbleTombstones(t *testing.T) {
    store, cleanup := setupPebbleTestDB(t)
    defer cleanup()

    // Create two authors
    author1, err := store.CreateAuthor("J N Chaney")
    require.NoError(t, err)
    author2, err := store.CreateAuthor("J. N. Chaney")
    require.NoError(t, err)

    // Create tombstone: author1 → author2
    err = store.CreateAuthorTombstone(author1.ID, author2.ID)
    require.NoError(t, err)

    // GetAuthorByID should follow redirect
    found, err := store.GetAuthorByID(author1.ID)
    require.NoError(t, err)
    require.NotNil(t, found)
    require.Equal(t, author2.ID, found.ID)
    require.Equal(t, "J. N. Chaney", found.Name)
}

func TestPebbleTombstoneChainResolution(t *testing.T) {
    store, cleanup := setupPebbleTestDB(t)
    defer cleanup()

    a, _ := store.CreateAuthor("A")
    b, _ := store.CreateAuthor("B")
    c, _ := store.CreateAuthor("C")

    // A → B → C
    store.CreateAuthorTombstone(a.ID, b.ID)
    store.CreateAuthorTombstone(b.ID, c.ID)

    // Should resolve chain
    resolved, err := store.ResolveTombstoneChains()
    require.NoError(t, err)
    require.Equal(t, 1, resolved) // A→B collapsed to A→C

    // A should now point directly to C
    found, _ := store.GetAuthorByID(a.ID)
    require.Equal(t, c.ID, found.ID)
}
```

**Step 2: Run test to verify it fails**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/database/ -run TestPebbleTombstone -v`
Expected: FAIL — `CreateAuthorTombstone` undefined

**Step 3: Write implementation**

Add to `internal/database/pebble_store.go`:

```go
// Author Tombstone operations
// Key: author_tombstone:<old_id> → canonical_id (string)

func (p *PebbleStore) CreateAuthorTombstone(oldID, canonicalID int) error {
    key := []byte(fmt.Sprintf("author_tombstone:%d", oldID))
    return p.db.Set(key, []byte(strconv.Itoa(canonicalID)), pebble.Sync)
}

func (p *PebbleStore) GetAuthorTombstone(oldID int) (int, error) {
    key := []byte(fmt.Sprintf("author_tombstone:%d", oldID))
    value, closer, err := p.db.Get(key)
    if err == pebble.ErrNotFound {
        return 0, nil
    }
    if err != nil {
        return 0, err
    }
    defer closer.Close()
    return strconv.Atoi(string(value))
}

func (p *PebbleStore) ResolveTombstoneChains() (int, error) {
    iter, err := p.db.NewIter(&pebble.IterOptions{
        LowerBound: []byte("author_tombstone:"),
        UpperBound: []byte("author_tombstone;"),
    })
    if err != nil {
        return 0, err
    }
    defer iter.Close()

    // Collect all tombstones
    tombstones := map[int]int{}
    for iter.First(); iter.Valid(); iter.Next() {
        oldID, _ := strconv.Atoi(strings.TrimPrefix(string(iter.Key()), "author_tombstone:"))
        canonicalID, _ := strconv.Atoi(string(iter.Value()))
        tombstones[oldID] = canonicalID
    }

    // Resolve chains
    resolved := 0
    batch := p.db.NewBatch()
    for oldID, target := range tombstones {
        final := target
        seen := map[int]bool{oldID: true}
        for {
            next, exists := tombstones[final]
            if !exists || seen[final] {
                break
            }
            seen[final] = true
            final = next
        }
        if final != target {
            batch.Set([]byte(fmt.Sprintf("author_tombstone:%d", oldID)),
                []byte(strconv.Itoa(final)), nil)
            resolved++
        }
    }

    if resolved > 0 {
        return resolved, batch.Commit(pebble.Sync)
    }
    batch.Close()
    return 0, nil
}
```

Modify `GetAuthorByID` to follow tombstones (add at the beginning of the existing function, ~line 235):

```go
func (p *PebbleStore) GetAuthorByID(id int) (*Author, error) {
    key := []byte(fmt.Sprintf("author:%d", id))
    value, closer, err := p.db.Get(key)
    if err == pebble.ErrNotFound {
        // Check for tombstone redirect
        if canonicalID, tombErr := p.GetAuthorTombstone(id); tombErr == nil && canonicalID > 0 {
            return p.GetAuthorByID(canonicalID)
        }
        return nil, nil
    }
    // ... rest of existing function
```

Add tombstone creation to Store interface in `internal/database/store.go`:

```go
// Author Tombstones
CreateAuthorTombstone(oldID, canonicalID int) error
GetAuthorTombstone(oldID int) (int, error)
ResolveTombstoneChains() (int, error)
```

Add stubs to SQLiteStore and MockStore.

**Step 4: Run test to verify it passes**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/database/ -run TestPebbleTombstone -v`
Expected: PASS

**Step 5: Wire tombstones into merge handlers**

In `internal/server/server.go`, after every `store.DeleteAuthor(mergeID)` call (lines ~3000 and ~6847), add:

```go
store.CreateAuthorTombstone(mergeID, keepID)
```

**Step 6: Add scheduler task for chain resolution**

In `internal/server/scheduler.go`, register a daily maintenance task:

```go
ts.registerTask(TaskDefinition{
    Name:        "tombstone_cleanup",
    Description: "Resolve author tombstone chains",
    Category:    "maintenance",
    TriggerFn: func() (*database.Operation, error) {
        resolved, err := store.ResolveTombstoneChains()
        if err != nil {
            return nil, err
        }
        log.Printf("[INFO] Resolved %d tombstone chains", resolved)
        return nil, nil
    },
    IsEnabled:   func() bool { return true },
    GetInterval: func() time.Duration { return 24 * time.Hour },
    RunOnStart:  func() bool { return false },
})
```

**Step 7: Commit**

```bash
git add internal/database/pebble_store.go internal/database/pebble_store_test.go \
    internal/database/store.go internal/database/sqlite_store.go \
    internal/database/mock_store.go internal/server/server.go \
    internal/server/scheduler.go
git commit -m "feat: add author tombstones for permanent ID redirects after merge"
```

---

### Task 3: Scan Data Types & Store Methods

Define the Scan, Phase, and ScanResult types and CRUD operations on the AIScanStore.

**Files:**
- Modify: `internal/database/ai_scan_store.go` (add types and CRUD)
- Modify: `internal/database/ai_scan_store_test.go` (add CRUD tests)

**Step 1: Write the failing test**

```go
func TestAIScanStoreCRUD(t *testing.T) {
    store, err := NewAIScanStore(t.TempDir() + "/test.db")
    require.NoError(t, err)
    defer store.Close()

    // Create scan
    scan, err := store.CreateScan("batch", map[string]string{"groups": "gpt-5-mini", "full": "o4-mini"}, 4826)
    require.NoError(t, err)
    require.Equal(t, "pending", scan.Status)
    require.Equal(t, 4826, scan.AuthorCount)

    // Update status
    err = store.UpdateScanStatus(scan.ID, "scanning")
    require.NoError(t, err)

    // Get scan
    got, err := store.GetScan(scan.ID)
    require.NoError(t, err)
    require.Equal(t, "scanning", got.Status)

    // Create phase
    phase, err := store.CreatePhase(scan.ID, "groups_scan", "gpt-5-mini")
    require.NoError(t, err)
    require.Equal(t, "pending", phase.Status)

    // Update phase with batch ID
    err = store.UpdatePhaseStatus(scan.ID, "groups_scan", "submitted", "batch_abc123")
    require.NoError(t, err)

    // Get phases for scan
    phases, err := store.GetPhases(scan.ID)
    require.NoError(t, err)
    require.Len(t, phases, 1)
    require.Equal(t, "batch_abc123", phases[0].BatchID)

    // Save scan results
    result := &ScanResult{
        ScanID:    scan.ID,
        Agreement: "agreed",
        Suggestion: ScanSuggestion{
            Action:        "merge",
            CanonicalName: "J. N. Chaney",
            Confidence:    "high",
        },
    }
    err = store.SaveScanResult(result)
    require.NoError(t, err)

    // Get results
    results, err := store.GetScanResults(scan.ID)
    require.NoError(t, err)
    require.Len(t, results, 1)
    require.Equal(t, "agreed", results[0].Agreement)

    // List scans
    scans, err := store.ListScans()
    require.NoError(t, err)
    require.Len(t, scans, 1)

    // Delete scan
    err = store.DeleteScan(scan.ID)
    require.NoError(t, err)
    scans, _ = store.ListScans()
    require.Empty(t, scans)
}
```

**Step 2: Run test to verify it fails**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/database/ -run TestAIScanStoreCRUD -v`
Expected: FAIL

**Step 3: Write types and implementation**

Add to `internal/database/ai_scan_store.go`:

```go
import (
    "encoding/json"
    "strings"
    "time"
)

// Scan represents a full pipeline run.
type Scan struct {
    ID          int               `json:"id"`
    Status      string            `json:"status"` // pending, scanning, enriching, cross_validating, complete, failed
    Mode        string            `json:"mode"`   // batch, realtime
    Models      map[string]string `json:"models"` // {groups: "gpt-5-mini", full: "o4-mini"}
    AuthorCount int               `json:"author_count"`
    CreatedAt   time.Time         `json:"created_at"`
    CompletedAt *time.Time        `json:"completed_at,omitempty"`
}

// ScanPhase represents one phase of the pipeline.
type ScanPhase struct {
    ScanID      int              `json:"scan_id"`
    PhaseType   string           `json:"phase_type"` // groups_scan, full_scan, groups_enrich, full_enrich, cross_validate
    Status      string           `json:"status"`     // pending, submitted, processing, complete, failed
    BatchID     string           `json:"batch_id,omitempty"`
    Model       string           `json:"model"`
    InputData   json.RawMessage  `json:"input_data,omitempty"`
    OutputData  json.RawMessage  `json:"output_data,omitempty"`
    Suggestions json.RawMessage  `json:"suggestions,omitempty"`
    StartedAt   *time.Time       `json:"started_at,omitempty"`
    CompletedAt *time.Time       `json:"completed_at,omitempty"`
}

// ScanSuggestion is the normalized suggestion from any phase.
type ScanSuggestion struct {
    Action        string           `json:"action"`
    CanonicalName string           `json:"canonical_name"`
    Reason        string           `json:"reason"`
    Confidence    string           `json:"confidence"`
    AuthorIDs     []int            `json:"author_ids,omitempty"`
    GroupIndex    int              `json:"group_index,omitempty"`
    Roles         *SuggestionRoles `json:"roles,omitempty"`
    Source        string           `json:"source"` // groups_scan, full_scan, groups_enrich, full_enrich
}

// ScanResult is the final cross-validated output.
type ScanResult struct {
    ID         int            `json:"id"`
    ScanID     int            `json:"scan_id"`
    Agreement  string         `json:"agreement"` // agreed, groups_only, full_only, disagreed
    Suggestion ScanSuggestion `json:"suggestion"`
    Applied    bool           `json:"applied"`
    AppliedAt  *time.Time     `json:"applied_at,omitempty"`
}

// --- CRUD Methods ---

func (s *AIScanStore) CreateScan(mode string, models map[string]string, authorCount int) (*Scan, error) {
    id, err := s.nextID("scan")
    if err != nil {
        return nil, err
    }
    scan := &Scan{
        ID: id, Status: "pending", Mode: mode,
        Models: models, AuthorCount: authorCount,
        CreatedAt: time.Now(),
    }
    data, _ := json.Marshal(scan)
    if err := s.db.Set([]byte(fmt.Sprintf("scan:%d", id)), data, pebble.Sync); err != nil {
        return nil, err
    }
    return scan, nil
}

func (s *AIScanStore) GetScan(id int) (*Scan, error) {
    value, closer, err := s.db.Get([]byte(fmt.Sprintf("scan:%d", id)))
    if err == pebble.ErrNotFound {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    defer closer.Close()
    var scan Scan
    if err := json.Unmarshal(value, &scan); err != nil {
        return nil, err
    }
    return &scan, nil
}

func (s *AIScanStore) UpdateScanStatus(id int, status string) error {
    scan, err := s.GetScan(id)
    if err != nil || scan == nil {
        return fmt.Errorf("scan %d not found", id)
    }
    scan.Status = status
    if status == "complete" || status == "failed" {
        now := time.Now()
        scan.CompletedAt = &now
    }
    data, _ := json.Marshal(scan)
    return s.db.Set([]byte(fmt.Sprintf("scan:%d", id)), data, pebble.Sync)
}

func (s *AIScanStore) ListScans() ([]Scan, error) {
    iter, err := s.db.NewIter(&pebble.IterOptions{
        LowerBound: []byte("scan:0"),
        UpperBound: []byte("scan:;"),
    })
    if err != nil {
        return nil, err
    }
    defer iter.Close()

    var scans []Scan
    for iter.First(); iter.Valid(); iter.Next() {
        key := string(iter.Key())
        if strings.Contains(key, "_") { // skip scan_phase: and scan_result: keys
            continue
        }
        var sc Scan
        if err := json.Unmarshal(iter.Value(), &sc); err != nil {
            continue
        }
        scans = append(scans, sc)
    }
    return scans, nil
}

func (s *AIScanStore) DeleteScan(id int) error {
    batch := s.db.NewBatch()
    // Delete scan
    batch.Delete([]byte(fmt.Sprintf("scan:%d", id)), nil)
    // Delete phases
    phaseIter, _ := s.db.NewIter(&pebble.IterOptions{
        LowerBound: []byte(fmt.Sprintf("scan_phase:%d:", id)),
        UpperBound: []byte(fmt.Sprintf("scan_phase:%d;", id)),
    })
    if phaseIter != nil {
        for phaseIter.First(); phaseIter.Valid(); phaseIter.Next() {
            batch.Delete(iter.Key(), nil)
        }
        phaseIter.Close()
    }
    // Delete results
    resultIter, _ := s.db.NewIter(&pebble.IterOptions{
        LowerBound: []byte(fmt.Sprintf("scan_result:%d:", id)),
        UpperBound: []byte(fmt.Sprintf("scan_result:%d;", id)),
    })
    if resultIter != nil {
        for resultIter.First(); resultIter.Valid(); resultIter.Next() {
            batch.Delete(resultIter.Key(), nil)
        }
        resultIter.Close()
    }
    return batch.Commit(pebble.Sync)
}

func (s *AIScanStore) CreatePhase(scanID int, phaseType, model string) (*ScanPhase, error) {
    phase := &ScanPhase{
        ScanID: scanID, PhaseType: phaseType,
        Status: "pending", Model: model,
    }
    data, _ := json.Marshal(phase)
    key := fmt.Sprintf("scan_phase:%d:%s", scanID, phaseType)
    if err := s.db.Set([]byte(key), data, pebble.Sync); err != nil {
        return nil, err
    }
    return phase, nil
}

func (s *AIScanStore) GetPhase(scanID int, phaseType string) (*ScanPhase, error) {
    key := []byte(fmt.Sprintf("scan_phase:%d:%s", scanID, phaseType))
    value, closer, err := s.db.Get(key)
    if err == pebble.ErrNotFound {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    defer closer.Close()
    var phase ScanPhase
    json.Unmarshal(value, &phase)
    return &phase, nil
}

func (s *AIScanStore) UpdatePhaseStatus(scanID int, phaseType, status, batchID string) error {
    phase, err := s.GetPhase(scanID, phaseType)
    if err != nil || phase == nil {
        return fmt.Errorf("phase %s for scan %d not found", phaseType, scanID)
    }
    phase.Status = status
    if batchID != "" {
        phase.BatchID = batchID
    }
    now := time.Now()
    if status == "submitted" || status == "processing" {
        phase.StartedAt = &now
    }
    if status == "complete" || status == "failed" {
        phase.CompletedAt = &now
    }
    data, _ := json.Marshal(phase)
    return s.db.Set([]byte(fmt.Sprintf("scan_phase:%d:%s", scanID, phaseType)), data, pebble.Sync)
}

func (s *AIScanStore) SavePhaseData(scanID int, phaseType string, input, output, suggestions json.RawMessage) error {
    phase, err := s.GetPhase(scanID, phaseType)
    if err != nil || phase == nil {
        return fmt.Errorf("phase not found")
    }
    phase.InputData = input
    phase.OutputData = output
    phase.Suggestions = suggestions
    data, _ := json.Marshal(phase)
    return s.db.Set([]byte(fmt.Sprintf("scan_phase:%d:%s", scanID, phaseType)), data, pebble.Sync)
}

func (s *AIScanStore) GetPhases(scanID int) ([]ScanPhase, error) {
    iter, err := s.db.NewIter(&pebble.IterOptions{
        LowerBound: []byte(fmt.Sprintf("scan_phase:%d:", scanID)),
        UpperBound: []byte(fmt.Sprintf("scan_phase:%d;", scanID)),
    })
    if err != nil {
        return nil, err
    }
    defer iter.Close()

    var phases []ScanPhase
    for iter.First(); iter.Valid(); iter.Next() {
        var p ScanPhase
        json.Unmarshal(iter.Value(), &p)
        phases = append(phases, p)
    }
    return phases, nil
}

func (s *AIScanStore) SaveScanResult(result *ScanResult) error {
    if result.ID == 0 {
        id, err := s.nextID("scan_result")
        if err != nil {
            return err
        }
        result.ID = id
    }
    data, _ := json.Marshal(result)
    key := fmt.Sprintf("scan_result:%d:%06d", result.ScanID, result.ID)
    return s.db.Set([]byte(key), data, pebble.Sync)
}

func (s *AIScanStore) GetScanResults(scanID int) ([]ScanResult, error) {
    iter, err := s.db.NewIter(&pebble.IterOptions{
        LowerBound: []byte(fmt.Sprintf("scan_result:%d:", scanID)),
        UpperBound: []byte(fmt.Sprintf("scan_result:%d;", scanID)),
    })
    if err != nil {
        return nil, err
    }
    defer iter.Close()

    var results []ScanResult
    for iter.First(); iter.Valid(); iter.Next() {
        var r ScanResult
        json.Unmarshal(iter.Value(), &r)
        results = append(results, r)
    }
    return results, nil
}

func (s *AIScanStore) MarkResultApplied(scanID, resultID int) error {
    key := []byte(fmt.Sprintf("scan_result:%d:%06d", scanID, resultID))
    value, closer, err := s.db.Get(key)
    if err != nil {
        return err
    }
    defer closer.Close()

    var result ScanResult
    json.Unmarshal(value, &result)
    result.Applied = true
    now := time.Now()
    result.AppliedAt = &now
    data, _ := json.Marshal(result)
    return s.db.Set(key, data, pebble.Sync)
}
```

**Step 4: Run test to verify it passes**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/database/ -run TestAIScanStoreCRUD -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/database/ai_scan_store.go internal/database/ai_scan_store_test.go
git commit -m "feat: add scan/phase/result CRUD to AI scan store"
```

---

### Task 4: Pipeline Manager

Coordinates phase transitions. This is the core orchestrator.

**Files:**
- Create: `internal/server/ai_scan_pipeline.go`
- Create: `internal/server/ai_scan_pipeline_test.go`

**Step 1: Write the failing test**

```go
// file: internal/server/ai_scan_pipeline_test.go
package server

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestPipelinePhaseTransitions(t *testing.T) {
    // Test that completing groups_scan triggers groups_enrich
    pm := &PipelineManager{}
    next := pm.nextPhases("groups_scan", "complete", map[string]string{
        "groups_scan": "complete",
        "full_scan":   "processing",
    })
    require.Contains(t, next, "groups_enrich")
    require.NotContains(t, next, "cross_validate")
}

func TestPipelineCrossValidateReady(t *testing.T) {
    pm := &PipelineManager{}
    next := pm.nextPhases("full_enrich", "complete", map[string]string{
        "groups_scan":    "complete",
        "full_scan":      "complete",
        "groups_enrich":  "complete",
        "full_enrich":    "complete",
    })
    require.Contains(t, next, "cross_validate")
}
```

**Step 2: Run test to verify it fails**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/server/ -run TestPipeline -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// file: internal/server/ai_scan_pipeline.go
package server

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "sync"

    "github.com/jdfalk/audiobook-organizer/internal/ai"
    "github.com/jdfalk/audiobook-organizer/internal/database"
)

type PipelineManager struct {
    scanStore *database.AIScanStore
    mainStore database.Store
    parser    *ai.OpenAIParser
    server    *Server
    mu        sync.Mutex
}

func NewPipelineManager(scanStore *database.AIScanStore, mainStore database.Store, parser *ai.OpenAIParser, server *Server) *PipelineManager {
    return &PipelineManager{
        scanStore: scanStore,
        mainStore: mainStore,
        parser:    parser,
        server:    server,
    }
}

// nextPhases determines which phases should start based on current state.
func (pm *PipelineManager) nextPhases(completedPhase, status string, phaseStates map[string]string) []string {
    if status != "complete" {
        return nil
    }

    var next []string

    switch completedPhase {
    case "groups_scan":
        next = append(next, "groups_enrich")
    case "full_scan":
        next = append(next, "full_enrich")
    case "groups_enrich", "full_enrich":
        // Cross-validate when both enrichments are done
        groupsDone := phaseStates["groups_enrich"] == "complete" || phaseStates["groups_scan"] == "complete" && phaseStates["groups_enrich"] == ""
        fullDone := phaseStates["full_enrich"] == "complete" || phaseStates["full_scan"] == "complete" && phaseStates["full_enrich"] == ""
        if completedPhase == "groups_enrich" {
            groupsDone = true
        }
        if completedPhase == "full_enrich" {
            fullDone = true
        }
        if groupsDone && fullDone {
            next = append(next, "cross_validate")
        }
    }

    return next
}

// StartScan creates a new scan and kicks off Phase 1 (parallel scans).
func (pm *PipelineManager) StartScan(ctx context.Context, mode string) (*database.Scan, error) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    models := map[string]string{"groups": "gpt-5-mini", "full": "o4-mini"}

    authors, err := pm.mainStore.GetAllAuthors()
    if err != nil {
        return nil, fmt.Errorf("get authors: %w", err)
    }

    scan, err := pm.scanStore.CreateScan(mode, models, len(authors))
    if err != nil {
        return nil, fmt.Errorf("create scan: %w", err)
    }

    // Create phase records
    pm.scanStore.CreatePhase(scan.ID, "groups_scan", models["groups"])
    pm.scanStore.CreatePhase(scan.ID, "full_scan", models["full"])

    pm.scanStore.UpdateScanStatus(scan.ID, "scanning")

    // Launch both phases
    if mode == "batch" {
        go pm.runGroupsScanBatch(ctx, scan.ID, authors)
        go pm.runFullScanBatch(ctx, scan.ID, authors)
    } else {
        go pm.runGroupsScanRealtime(ctx, scan.ID, authors)
        go pm.runFullScanRealtime(ctx, scan.ID, authors)
    }

    return scan, nil
}

// OnPhaseComplete is called when a phase finishes. Triggers next phases.
func (pm *PipelineManager) OnPhaseComplete(ctx context.Context, scanID int, completedPhase string) {
    phases, _ := pm.scanStore.GetPhases(scanID)
    phaseStates := map[string]string{}
    for _, p := range phases {
        phaseStates[p.PhaseType] = p.Status
    }

    next := pm.nextPhases(completedPhase, "complete", phaseStates)
    for _, phaseType := range next {
        switch phaseType {
        case "groups_enrich":
            go pm.runEnrichment(ctx, scanID, "groups_scan", "groups_enrich")
        case "full_enrich":
            go pm.runEnrichment(ctx, scanID, "full_scan", "full_enrich")
        case "cross_validate":
            go pm.runCrossValidation(ctx, scanID)
        }
    }
}

// Phase implementations — stubs to be filled in Tasks 5 and 6

func (pm *PipelineManager) runGroupsScanRealtime(ctx context.Context, scanID int, authors []database.Author) {
    log.Printf("[AI Pipeline] Scan %d: starting groups scan (realtime)", scanID)
    // TODO: Task 5 — call parser.ReviewAuthorDuplicates
    pm.scanStore.UpdatePhaseStatus(scanID, "groups_scan", "complete", "")
    pm.OnPhaseComplete(ctx, scanID, "groups_scan")
}

func (pm *PipelineManager) runFullScanRealtime(ctx context.Context, scanID int, authors []database.Author) {
    log.Printf("[AI Pipeline] Scan %d: starting full scan (realtime)", scanID)
    // TODO: Task 5 — call parser.DiscoverAuthorDuplicates
    pm.scanStore.UpdatePhaseStatus(scanID, "full_scan", "complete", "")
    pm.OnPhaseComplete(ctx, scanID, "full_scan")
}

func (pm *PipelineManager) runGroupsScanBatch(ctx context.Context, scanID int, authors []database.Author) {
    log.Printf("[AI Pipeline] Scan %d: starting groups scan (batch)", scanID)
    // TODO: Task 5 — call parser.CreateBatchAuthorDedup for groups
    pm.scanStore.UpdatePhaseStatus(scanID, "groups_scan", "submitted", "")
}

func (pm *PipelineManager) runFullScanBatch(ctx context.Context, scanID int, authors []database.Author) {
    log.Printf("[AI Pipeline] Scan %d: starting full scan (batch)", scanID)
    // TODO: Task 5 — call parser.CreateBatchAuthorDedup for full
    pm.scanStore.UpdatePhaseStatus(scanID, "full_scan", "submitted", "")
}

func (pm *PipelineManager) runEnrichment(ctx context.Context, scanID int, sourcePhase, enrichPhase string) {
    log.Printf("[AI Pipeline] Scan %d: starting enrichment for %s", scanID, sourcePhase)
    // TODO: Task 5 — fetch book titles, resubmit uncertain
    pm.scanStore.UpdatePhaseStatus(scanID, enrichPhase, "complete", "")
    pm.OnPhaseComplete(ctx, scanID, enrichPhase)
}

func (pm *PipelineManager) runCrossValidation(ctx context.Context, scanID int) {
    log.Printf("[AI Pipeline] Scan %d: starting cross-validation", scanID)
    // TODO: Task 6 — logic tree
    pm.scanStore.UpdateScanStatus(scanID, "complete")
}
```

**Step 4: Run test to verify it passes**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/server/ -run TestPipeline -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/ai_scan_pipeline.go internal/server/ai_scan_pipeline_test.go
git commit -m "feat: add pipeline manager with phase transition logic"
```

---

### Task 5: Fill In Phase Implementations (AI Calls)

Wire up the actual OpenAI calls for each phase. This fills in the TODO stubs from Task 4.

**Files:**
- Modify: `internal/server/ai_scan_pipeline.go` (fill in phase methods)
- Modify: `internal/ai/openai_parser.go` (add model parameter to review functions if needed)
- Modify: `internal/ai/openai_batch.go` (add groups batch support)

**Step 1: Implement realtime groups scan**

Fill `runGroupsScanRealtime`:
- Call `s.server.FindDuplicateAuthors(authors, 0.9, ...)` to get heuristic groups
- Build `[]ai.AuthorDedupInput` from groups
- Call `pm.parser.ReviewAuthorDuplicates(ctx, inputs)` with the groups model
- Save input/output/suggestions to phase via `SavePhaseData`
- Update phase status → "complete"
- Call `OnPhaseComplete`

**Step 2: Implement realtime full scan**

Fill `runFullScanRealtime`:
- Build `[]ai.AuthorDiscoveryInput` from all authors (include sample book titles)
- Chunk into batches of ~500
- Call `pm.parser.DiscoverAuthorDuplicates(ctx, chunk)` for each
- Aggregate suggestions, save to phase
- Update → "complete", trigger next

**Step 3: Implement batch scan submissions**

Fill `runGroupsScanBatch` and `runFullScanBatch`:
- Same input building as realtime
- Call `pm.parser.CreateBatchAuthorDedup(ctx, inputs)` for full
- Add `CreateBatchAuthorReview(ctx, inputs)` to `openai_batch.go` for groups
- Save batch_id to phase status
- Polling handled by scheduler (Task 5, Step 5)

**Step 4: Implement enrichment phase**

Fill `runEnrichment`:
- Load suggestions from source phase
- Filter medium/low confidence
- For each uncertain suggestion, fetch book titles from `pm.mainStore`
- Build enriched prompt with book evidence
- Submit (batch or realtime depending on scan mode)
- Merge enriched results back: if confidence upgraded, replace original suggestion

**Step 5: Add batch polling to scheduler**

Register a scheduler task `ai_scan_batch_poll` that:
- Runs every 5 minutes
- Finds all scans with status "scanning" or "enriching"
- For each phase with status "submitted": calls `parser.CheckBatchStatus`
- If complete: downloads results, saves to phase, calls `OnPhaseComplete`
- Update scan status based on phase states

**Step 6: Commit**

```bash
git add internal/server/ai_scan_pipeline.go internal/ai/openai_parser.go internal/ai/openai_batch.go internal/server/scheduler.go
git commit -m "feat: wire up AI calls for all pipeline phases"
```

---

### Task 6: Cross-Validation Logic Tree

Implement the local comparison logic that runs after both scans complete.

**Files:**
- Create: `internal/server/ai_cross_validate.go`
- Create: `internal/server/ai_cross_validate_test.go`

**Step 1: Write the failing test**

```go
// file: internal/server/ai_cross_validate_test.go
package server

import (
    "testing"

    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/stretchr/testify/require"
)

func TestCrossValidateAgreed(t *testing.T) {
    groups := []database.ScanSuggestion{
        {Action: "merge", CanonicalName: "J. N. Chaney", AuthorIDs: []int{143, 5078}, Confidence: "high", Source: "groups_scan"},
    }
    full := []database.ScanSuggestion{
        {Action: "merge", CanonicalName: "J. N. Chaney", AuthorIDs: []int{143, 5078}, Confidence: "high", Source: "full_scan"},
    }
    results := CrossValidate(groups, full)
    require.Len(t, results, 1)
    require.Equal(t, "agreed", results[0].Agreement)
}

func TestCrossValidateDisagreed(t *testing.T) {
    groups := []database.ScanSuggestion{
        {Action: "merge", CanonicalName: "Emily Bauer", AuthorIDs: []int{100, 200}, Confidence: "high", Source: "groups_scan"},
    }
    full := []database.ScanSuggestion{
        {Action: "split", CanonicalName: "Emily Bauer", AuthorIDs: []int{100, 200}, Confidence: "high", Source: "full_scan"},
    }
    results := CrossValidate(groups, full)
    require.Len(t, results, 1)
    require.Equal(t, "disagreed", results[0].Agreement)
}

func TestCrossValidateOneSided(t *testing.T) {
    groups := []database.ScanSuggestion{
        {Action: "merge", CanonicalName: "Some Author", AuthorIDs: []int{100, 200}, Confidence: "high", Source: "groups_scan"},
    }
    full := []database.ScanSuggestion{
        {Action: "reclassify", CanonicalName: "Brilliance Audio", AuthorIDs: []int{300}, Confidence: "high", Source: "full_scan"},
    }
    results := CrossValidate(groups, full)
    require.Len(t, results, 2)
    // Find each by agreement type
    agreements := map[string]int{}
    for _, r := range results {
        agreements[r.Agreement]++
    }
    require.Equal(t, 1, agreements["groups_only"])
    require.Equal(t, 1, agreements["full_only"])
}
```

**Step 2: Run test to verify it fails**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/server/ -run TestCrossValidate -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// file: internal/server/ai_cross_validate.go
package server

import "github.com/jdfalk/audiobook-organizer/internal/database"

// CrossValidate compares groups and full scan suggestions using a logic tree.
func CrossValidate(groups, full []database.ScanSuggestion) []database.ScanResult {
    var results []database.ScanResult
    fullMatched := make([]bool, len(full))

    for _, g := range groups {
        matchIdx := -1
        for i, f := range full {
            if fullMatched[i] {
                continue
            }
            if hasOverlappingIDs(g.AuthorIDs, f.AuthorIDs) || g.CanonicalName == f.CanonicalName {
                matchIdx = i
                break
            }
        }

        if matchIdx >= 0 {
            fullMatched[matchIdx] = true
            f := full[matchIdx]
            agreement := "disagreed"
            sug := g // default to groups suggestion

            if g.Action == f.Action {
                agreement = "agreed"
                // Use higher confidence
                if confidenceRank(f.Confidence) > confidenceRank(g.Confidence) {
                    sug.Confidence = f.Confidence
                }
            }

            results = append(results, database.ScanResult{
                Agreement:  agreement,
                Suggestion: sug,
            })
        } else {
            results = append(results, database.ScanResult{
                Agreement:  "groups_only",
                Suggestion: g,
            })
        }
    }

    // Unmatched full suggestions
    for i, f := range full {
        if !fullMatched[i] {
            results = append(results, database.ScanResult{
                Agreement:  "full_only",
                Suggestion: f,
            })
        }
    }

    return results
}

func hasOverlappingIDs(a, b []int) bool {
    if len(a) == 0 || len(b) == 0 {
        return false
    }
    set := map[int]bool{}
    for _, id := range a {
        set[id] = true
    }
    for _, id := range b {
        if set[id] {
            return true
        }
    }
    return false
}

func confidenceRank(c string) int {
    switch c {
    case "high":
        return 3
    case "medium":
        return 2
    case "low":
        return 1
    default:
        return 0
    }
}
```

**Step 4: Run test to verify it passes**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/server/ -run TestCrossValidate -v`
Expected: PASS

**Step 5: Wire into pipeline manager**

Fill `runCrossValidation` in `ai_scan_pipeline.go`:
- Load groups and full phase suggestions
- Parse into `[]ScanSuggestion`
- Call `CrossValidate(groups, full)`
- Save each `ScanResult` to store
- Update scan status → "complete"

**Step 6: Commit**

```bash
git add internal/server/ai_cross_validate.go internal/server/ai_cross_validate_test.go internal/server/ai_scan_pipeline.go
git commit -m "feat: add cross-validation logic tree for scan comparison"
```

---

### Task 7: API Endpoints

Add the new REST endpoints for scans.

**Files:**
- Modify: `internal/server/server.go` (add routes and handlers)

**Step 1: Register routes**

Add to `setupRoutes` in `server.go`, near the existing AI review routes:

```go
// AI Scan Pipeline
aiScans := protected.Group("/ai/scans")
aiScans.POST("", s.startAIScan)
aiScans.GET("", s.listAIScans)
aiScans.GET("/:id", s.getAIScan)
aiScans.GET("/:id/results", s.getAIScanResults)
aiScans.POST("/:id/apply", s.applyAIScanResults)
aiScans.DELETE("/:id", s.deleteAIScan)
aiScans.GET("/compare", s.compareAIScans)
```

**Step 2: Implement handlers**

```go
func (s *Server) startAIScan(c *gin.Context) {
    var req struct {
        Mode string `json:"mode"` // "batch" or "realtime"
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        req.Mode = "realtime"
    }
    if req.Mode != "batch" && req.Mode != "realtime" {
        req.Mode = "realtime"
    }

    scan, err := s.pipelineManager.StartScan(c.Request.Context(), req.Mode)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(202, scan)
}

func (s *Server) listAIScans(c *gin.Context) {
    scans, err := s.aiScanStore.ListScans()
    // return scans list
}

func (s *Server) getAIScan(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    scan, _ := s.aiScanStore.GetScan(id)
    phases, _ := s.aiScanStore.GetPhases(id)
    // return scan + phases
}

func (s *Server) getAIScanResults(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    results, _ := s.aiScanStore.GetScanResults(id)
    // return results with agreement filter support
}

func (s *Server) applyAIScanResults(c *gin.Context) {
    // Parse result IDs from body
    // For each result: apply the suggestion using existing merge/rename/split/etc logic
    // Mark result as applied
    // Write tombstones for merges
}

func (s *Server) deleteAIScan(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    s.aiScanStore.DeleteScan(id)
    c.JSON(200, gin.H{"status": "deleted"})
}

func (s *Server) compareAIScans(c *gin.Context) {
    aID, _ := strconv.Atoi(c.Query("a"))
    bID, _ := strconv.Atoi(c.Query("b"))
    resultsA, _ := s.aiScanStore.GetScanResults(aID)
    resultsB, _ := s.aiScanStore.GetScanResults(bID)
    // Compare: new in B, resolved from A, unchanged
}
```

**Step 3: Wire server startup**

In `NewServer` or `setupRoutes`:
- Open `ai_scans.db` PebbleDB alongside main DB
- Create `PipelineManager` with references to both stores + parser
- Store as `s.aiScanStore` and `s.pipelineManager`

**Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat: add AI scan pipeline REST endpoints"
```

---

### Task 8: Frontend — API Client

Add TypeScript API functions for the new scan endpoints.

**Files:**
- Modify: `web/src/services/api.ts`

**Step 1: Add types and functions**

```typescript
// --- AI Scan Pipeline Types ---

export interface AIScan {
  id: number;
  status: 'pending' | 'scanning' | 'enriching' | 'cross_validating' | 'complete' | 'failed';
  mode: 'batch' | 'realtime';
  models: { groups: string; full: string };
  author_count: number;
  created_at: string;
  completed_at?: string;
}

export interface AIScanPhase {
  scan_id: number;
  phase_type: string;
  status: string;
  batch_id?: string;
  model: string;
  started_at?: string;
  completed_at?: string;
}

export interface AIScanResult {
  id: number;
  scan_id: number;
  agreement: 'agreed' | 'groups_only' | 'full_only' | 'disagreed';
  suggestion: {
    action: string;
    canonical_name: string;
    reason: string;
    confidence: string;
    author_ids?: number[];
    roles?: SuggestionRoles;
    source: string;
  };
  applied: boolean;
  applied_at?: string;
}

export interface AIScanDetail extends AIScan {
  phases: AIScanPhase[];
}

export interface AIScanComparison {
  new_in_b: AIScanResult[];
  resolved_from_a: AIScanResult[];
  unchanged: AIScanResult[];
}

// --- API Functions ---

export async function startAIScan(mode: 'batch' | 'realtime'): Promise<AIScan> {
  const response = await fetch(`${API_BASE}/ai/scans`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to start AI scan');
  return response.json();
}

export async function listAIScans(): Promise<AIScan[]> {
  const response = await fetch(`${API_BASE}/ai/scans`);
  if (!response.ok) throw await buildApiError(response, 'Failed to list AI scans');
  return response.json().then(d => d.scans || []);
}

export async function getAIScan(id: number): Promise<AIScanDetail> {
  const response = await fetch(`${API_BASE}/ai/scans/${id}`);
  if (!response.ok) throw await buildApiError(response, 'Failed to get AI scan');
  return response.json();
}

export async function getAIScanResults(id: number): Promise<AIScanResult[]> {
  const response = await fetch(`${API_BASE}/ai/scans/${id}/results`);
  if (!response.ok) throw await buildApiError(response, 'Failed to get scan results');
  return response.json().then(d => d.results || []);
}

export async function applyAIScanResults(scanID: number, resultIDs: number[]): Promise<void> {
  const response = await fetch(`${API_BASE}/ai/scans/${scanID}/apply`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ result_ids: resultIDs }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to apply scan results');
}

export async function deleteAIScan(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/ai/scans/${id}`, { method: 'DELETE' });
  if (!response.ok) throw await buildApiError(response, 'Failed to delete scan');
}

export async function compareAIScans(a: number, b: number): Promise<AIScanComparison> {
  const response = await fetch(`${API_BASE}/ai/scans/compare?a=${a}&b=${b}`);
  if (!response.ok) throw await buildApiError(response, 'Failed to compare scans');
  return response.json();
}
```

**Step 2: Commit**

```bash
git add web/src/services/api.ts
git commit -m "feat: add AI scan pipeline API client functions"
```

---

### Task 9: Frontend — AI Authors Tab

Replace the current groups/full/combined tabs with the unified pipeline UI.

**Files:**
- Modify: `web/src/pages/BookDedup.tsx`

**Step 1: Replace AIReviewTab**

Replace the current `AIReviewTab` component (lines ~1984-2007) with:

```tsx
function AIReviewTab() {
  const [searchParams, setSearchParams] = useSearchParams();
  const aiSub = searchParams.get('aisub') || 'authors';
  const setAiSub = (v: string) => {
    setSearchParams(prev => { prev.set('aisub', v); return prev; });
  };

  return (
    <Box>
      <Tabs value={aiSub} onChange={(_, v) => setAiSub(v)}>
        <Tab value="authors" label="Authors" />
        <Tab value="books" label="Books" />
      </Tabs>
      {aiSub === 'authors' && <AIAuthorPipelinePage />}
      {aiSub === 'books' && (
        <Box sx={{ p: 4, textAlign: 'center' }}>
          <Typography color="text.secondary">Book deduplication coming soon.</Typography>
        </Box>
      )}
    </Box>
  );
}
```

**Step 2: Create AIAuthorPipelinePage**

New component that replaces `AIAuthorSubPage` and `AIAuthorCombinedSubPage`:

```tsx
function AIAuthorPipelinePage() {
  const [scan, setScan] = useState<api.AIScanDetail | null>(null);
  const [results, setResults] = useState<api.AIScanResult[]>([]);
  const [scans, setScans] = useState<api.AIScan[]>([]);
  const [batchMode, setBatchMode] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [agreementFilter, setAgreementFilter] = useState<string>('all');
  const [loading, setLoading] = useState(false);

  // Load scan list on mount
  useEffect(() => { api.listAIScans().then(setScans); }, []);

  // Poll active scan status
  useEffect(() => {
    if (!scan || scan.status === 'complete' || scan.status === 'failed') return;
    const interval = setInterval(async () => {
      const updated = await api.getAIScan(scan.id);
      setScan(updated);
      if (updated.status === 'complete') {
        const res = await api.getAIScanResults(scan.id);
        setResults(res);
        clearInterval(interval);
      }
    }, 5000);
    return () => clearInterval(interval);
  }, [scan?.id, scan?.status]);

  const startScan = async () => {
    setLoading(true);
    const newScan = await api.startAIScan(batchMode ? 'batch' : 'realtime');
    const detail = await api.getAIScan(newScan.id);
    setScan(detail);
    setLoading(false);
  };

  const loadScan = async (id: number) => {
    const detail = await api.getAIScan(id);
    setScan(detail);
    const res = await api.getAIScanResults(id);
    setResults(res);
    setHistoryOpen(false);
  };

  const applySelected = async () => {
    if (!scan) return;
    await api.applyAIScanResults(scan.id, Array.from(selected));
    const res = await api.getAIScanResults(scan.id);
    setResults(res);
    setSelected(new Set());
  };

  const filteredResults = results.filter(r =>
    agreementFilter === 'all' || r.agreement === agreementFilter
  );

  return (
    <Box>
      {/* Header bar */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 2 }}>
        <Button variant="contained" onClick={startScan} disabled={loading}>
          Run Scan
        </Button>
        <FormControlLabel
          control={<Switch checked={batchMode} onChange={(_, v) => setBatchMode(v)} />}
          label="Batch mode (50% cheaper, hours)"
        />
        <Button onClick={() => setHistoryOpen(true)}>Scan History</Button>
      </Box>

      {/* Scan progress */}
      {scan && scan.status !== 'complete' && scan.status !== 'failed' && (
        <ScanProgressBar scan={scan} />
      )}

      {/* Results */}
      {scan?.status === 'complete' && (
        <>
          {/* Agreement filter tabs */}
          <Tabs value={agreementFilter} onChange={(_, v) => setAgreementFilter(v)}>
            <Tab value="all" label={`All (${results.length})`} />
            <Tab value="agreed" label={`Agreed (${results.filter(r => r.agreement === 'agreed').length})`} />
            <Tab value="groups_only" label={`Groups Only (${results.filter(r => r.agreement === 'groups_only').length})`} />
            <Tab value="full_only" label={`Full Only (${results.filter(r => r.agreement === 'full_only').length})`} />
            <Tab value="disagreed" label={`Disagreed (${results.filter(r => r.agreement === 'disagreed').length})`} />
          </Tabs>

          {/* Apply button */}
          {selected.size > 0 && (
            <Button variant="contained" color="success" onClick={applySelected}>
              Apply {selected.size} Selected
            </Button>
          )}

          {/* Suggestion cards — reuse existing card component pattern */}
          {filteredResults.map(result => (
            <ScanResultCard
              key={result.id}
              result={result}
              selected={selected.has(result.id)}
              onSelect={(id) => {
                const next = new Set(selected);
                next.has(id) ? next.delete(id) : next.add(id);
                setSelected(next);
              }}
            />
          ))}
        </>
      )}

      {/* History sidebar */}
      <Drawer anchor="right" open={historyOpen} onClose={() => setHistoryOpen(false)}>
        <ScanHistorySidebar scans={scans} onSelect={loadScan} />
      </Drawer>
    </Box>
  );
}
```

**Step 3: Create ScanProgressBar component**

```tsx
function ScanProgressBar({ scan }: { scan: api.AIScanDetail }) {
  const phaseOrder = ['groups_scan', 'full_scan', 'groups_enrich', 'full_enrich', 'cross_validate'];
  const phaseLabels: Record<string, string> = {
    groups_scan: 'Groups', full_scan: 'Full',
    groups_enrich: 'Enrich (Groups)', full_enrich: 'Enrich (Full)',
    cross_validate: 'Cross-Validate',
  };
  const phaseMap = Object.fromEntries((scan.phases || []).map(p => [p.phase_type, p]));

  return (
    <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', mb: 2 }}>
      {phaseOrder.map(pt => {
        const phase = phaseMap[pt];
        const status = phase?.status || 'pending';
        const icon = status === 'complete' ? '✓' : status === 'failed' ? '✗' : status === 'pending' ? '○' : '→';
        return (
          <Chip key={pt} label={`${icon} ${phaseLabels[pt]}`}
            color={status === 'complete' ? 'success' : status === 'failed' ? 'error' : 'default'}
            variant={status === 'processing' || status === 'submitted' ? 'filled' : 'outlined'}
          />
        );
      })}
    </Box>
  );
}
```

**Step 4: Create ScanResultCard component**

Reuse the existing suggestion card pattern from lines ~1940-1968, adapting for `AIScanResult`:

```tsx
function ScanResultCard({ result, selected, onSelect }: {
  result: api.AIScanResult; selected: boolean;
  onSelect: (id: number) => void;
}) {
  const sug = result.suggestion;
  const actionColors: Record<string, 'success' | 'warning' | 'error' | 'info' | 'default'> = {
    merge: 'success', split: 'warning', rename: 'default', alias: 'info', reclassify: 'error',
  };

  return (
    <Card sx={{ mb: 1, opacity: result.applied ? 0.5 : 1 }}>
      <CardContent sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <Checkbox checked={selected} onChange={() => onSelect(result.id)} disabled={result.applied} />
        <Chip label={sug.action} color={actionColors[sug.action] || 'default'} size="small" />
        <Chip label={sug.confidence} size="small" variant="outlined" />
        <Chip label={result.agreement} size="small" variant="outlined"
          color={result.agreement === 'agreed' ? 'success' : result.agreement === 'disagreed' ? 'error' : 'default'} />
        <Typography variant="subtitle1" sx={{ fontWeight: 'bold' }}>{sug.canonical_name}</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ ml: 'auto' }}>{sug.reason}</Typography>
        {result.applied && <Chip label="Applied" size="small" color="success" />}
      </CardContent>
    </Card>
  );
}
```

**Step 5: Commit**

```bash
git add web/src/pages/BookDedup.tsx
git commit -m "feat: replace AI review tabs with unified pipeline UI"
```

---

### Task 10: Frontend — Scan History Sidebar

**Files:**
- Modify: `web/src/pages/BookDedup.tsx` (add sidebar component)

**Step 1: Create ScanHistorySidebar**

```tsx
function ScanHistorySidebar({ scans, onSelect }: {
  scans: api.AIScan[]; onSelect: (id: number) => void;
}) {
  const [compareMode, setCompareMode] = useState(false);
  const [compareIds, setCompareIds] = useState<number[]>([]);
  const [comparison, setComparison] = useState<api.AIScanComparison | null>(null);

  const toggleCompare = (id: number) => {
    setCompareIds(prev => {
      if (prev.includes(id)) return prev.filter(x => x !== id);
      if (prev.length >= 2) return [prev[1], id];
      return [...prev, id];
    });
  };

  const runCompare = async () => {
    if (compareIds.length === 2) {
      const result = await api.compareAIScans(compareIds[0], compareIds[1]);
      setComparison(result);
    }
  };

  return (
    <Box sx={{ width: 400, p: 2 }}>
      <Typography variant="h6">Scan History</Typography>
      <FormControlLabel
        control={<Switch checked={compareMode} onChange={(_, v) => setCompareMode(v)} />}
        label="Compare mode"
      />

      <List>
        {scans.map(scan => (
          <ListItem key={scan.id} disablePadding>
            {compareMode && (
              <Checkbox checked={compareIds.includes(scan.id)} onChange={() => toggleCompare(scan.id)} />
            )}
            <ListItemButton onClick={() => !compareMode && onSelect(scan.id)}>
              <ListItemText
                primary={`Scan #${scan.id} — ${new Date(scan.created_at).toLocaleDateString()}`}
                secondary={`${scan.author_count} authors · ${scan.status} · ${scan.mode}`}
              />
            </ListItemButton>
          </ListItem>
        ))}
      </List>

      {compareMode && compareIds.length === 2 && (
        <Button variant="contained" onClick={runCompare} fullWidth>
          Compare Scan #{compareIds[0]} vs #{compareIds[1]}
        </Button>
      )}

      {comparison && (
        <Box sx={{ mt: 2 }}>
          <Typography variant="subtitle2">New: {comparison.new_in_b.length}</Typography>
          <Typography variant="subtitle2">Resolved: {comparison.resolved_from_a.length}</Typography>
          <Typography variant="subtitle2">Unchanged: {comparison.unchanged.length}</Typography>
        </Box>
      )}
    </Box>
  );
}
```

**Step 2: Commit**

```bash
git add web/src/pages/BookDedup.tsx
git commit -m "feat: add scan history sidebar with compare mode"
```

---

### Task 11: Wire Up & Integration Test

Connect everything: server startup opens ai_scans.db, creates PipelineManager, registers batch polling task.

**Files:**
- Modify: `internal/server/server.go` (add `aiScanStore` and `pipelineManager` fields to Server struct, initialize on startup)
- Modify: `internal/server/scheduler.go` (register batch poll task)
- Create: `internal/server/ai_scan_pipeline_integration_test.go`

**Step 1: Add fields to Server struct**

```go
type Server struct {
    // ... existing fields ...
    aiScanStore     *database.AIScanStore
    pipelineManager *PipelineManager
}
```

**Step 2: Initialize on startup**

In server initialization (after main store is opened):

```go
// Open AI scan store alongside main DB
aiScanDBPath := filepath.Join(filepath.Dir(mainDBPath), "ai_scans.db")
aiScanStore, err := database.NewAIScanStore(aiScanDBPath)
if err != nil {
    return nil, fmt.Errorf("failed to open AI scan store: %w", err)
}
s.aiScanStore = aiScanStore
s.pipelineManager = NewPipelineManager(aiScanStore, s.store, s.parser, s)
```

**Step 3: Close on shutdown**

In server shutdown:

```go
if s.aiScanStore != nil {
    s.aiScanStore.Close()
}
```

**Step 4: Register batch poll scheduler task**

```go
ts.registerTask(TaskDefinition{
    Name:        "ai_scan_batch_poll",
    Description: "Poll OpenAI batch API for in-progress AI scans",
    Category:    "ai",
    TriggerFn:   func() (*database.Operation, error) { return s.pipelineManager.PollBatches(context.Background()) },
    IsEnabled:   func() bool { return s.parser != nil && s.parser.IsEnabled() },
    GetInterval: func() time.Duration { return 5 * time.Minute },
    RunOnStart:  func() bool { return true },
})
```

**Step 5: Write integration test**

```go
func TestAIScanPipelineIntegration(t *testing.T) {
    // Create temp stores
    mainStore, cleanup := setupTestServer(t)
    defer cleanup()
    scanStore, _ := database.NewAIScanStore(t.TempDir() + "/scans.db")
    defer scanStore.Close()

    // Create some test authors
    mainStore.CreateAuthor("J N Chaney")
    mainStore.CreateAuthor("J. N. Chaney")
    mainStore.CreateAuthor("Brilliance Audio")

    // Create scan
    scan, err := scanStore.CreateScan("realtime", map[string]string{"groups": "test", "full": "test"}, 3)
    require.NoError(t, err)
    require.Equal(t, "pending", scan.Status)

    // Verify phase transitions
    scanStore.CreatePhase(scan.ID, "groups_scan", "test")
    scanStore.CreatePhase(scan.ID, "full_scan", "test")

    scanStore.UpdatePhaseStatus(scan.ID, "groups_scan", "complete", "")
    scanStore.UpdatePhaseStatus(scan.ID, "full_scan", "complete", "")

    phases, _ := scanStore.GetPhases(scan.ID)
    require.Len(t, phases, 2)
}
```

**Step 6: Verify full build**

Run: `GOEXPERIMENT=jsonv2 go build ./...`
Expected: compiles without errors

Run: `GOEXPERIMENT=jsonv2 go test ./internal/database/ ./internal/server/ -v -count=1`
Expected: all tests pass

**Step 7: Commit**

```bash
git add internal/server/server.go internal/server/scheduler.go \
    internal/server/ai_scan_pipeline_integration_test.go
git commit -m "feat: wire up AI scan pipeline — server startup, scheduler, integration test"
```

---

## Summary

| Task | What | Files | Dependencies |
|------|------|-------|-------------|
| 1 | AI Scan Store skeleton | `ai_scan_store.go` | None |
| 2 | Author tombstones | `pebble_store.go`, `server.go`, `scheduler.go` | None |
| 3 | Scan types + CRUD | `ai_scan_store.go` | Task 1 |
| 4 | Pipeline manager | `ai_scan_pipeline.go` | Task 3 |
| 5 | AI call implementations | `ai_scan_pipeline.go`, `openai_*.go` | Task 4 |
| 6 | Cross-validation logic | `ai_cross_validate.go` | Task 3 |
| 7 | REST endpoints | `server.go` | Tasks 4, 5, 6 |
| 8 | Frontend API client | `api.ts` | Task 7 |
| 9 | AI Authors tab UI | `BookDedup.tsx` | Task 8 |
| 10 | Scan history sidebar | `BookDedup.tsx` | Task 8 |
| 11 | Wire up + integration | `server.go`, `scheduler.go` | All |
