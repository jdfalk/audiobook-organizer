# AI Jobs Batch Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route all bulk-scale LLM chat-completion work through the OpenAI Batch API via a new `internal/ai/aijobs` package + `ai_jobs` tracking table, while preserving the synchronous path for declared `Interactive` callers. Eliminates 429 `insufficient_quota` failures on maintenance runs.

**Architecture:** New generic `aijobs.Submit`/`aijobs.Dispatch` layer sits on top of the existing `BatchPoller`. A per-job row in `ai_jobs` tracks status; a per-row callback (registered by the feature) applies each batch result. Every current sync `Chat.Completions.New` call site is either migrated to this layer (`Bulk`) or marked with a `// PRIORITY: Interactive` comment; a CI test enforces the split.

**Tech Stack:** Go 1.24 (backend), SQLite + PebbleDB (dual-store schema), OpenAI Go SDK v2 (`github.com/openai/openai-go`), React/TypeScript + MUI (frontend).

**Design doc:** `docs/superpowers/specs/2026-04-24-aijobs-batch-migration-design.md`

**Branch:** `feat/aijobs-batch-migration` (worktree: `.worktrees/aijobs-batch-migration`)

---

## File Structure

**New files (Go backend):**
- `internal/database/ai_jobs_store.go` — store methods for the `ai_jobs` + `ai_job_payloads` tables
- `internal/database/ai_jobs_store_test.go` — unit tests for the store
- `internal/ai/aijobs/aijobs.go` — package entry: `Submit`, `Register`, `Dispatch`, registry
- `internal/ai/aijobs/aijobs_test.go` — unit tests for the package
- `internal/ai/aijobs/doc.go` — package godoc
- `internal/ai/priority_marker_test.go` — enforcement test for `Chat.Completions.New` call sites

**Modified files (Go backend):**
- `internal/database/migrations.go` — append migration 52
- `internal/database/iface_ops.go` (or appropriate iface file) — add `AIJobsStore` interface
- `internal/server/batch_poller.go` — register new `aijobs` handler
- `internal/server/server.go` — wire `aijobs` registry at startup
- `internal/ai/dedup_review.go` — migrate to `aijobs.Submit` (reference impl)
- `internal/ai/metadata_llm_review.go` — migrate to `aijobs.Submit`
- `internal/ai/openai_parser.go` — migrate 5 sync sites; split cover-image into Interactive/Bulk
- Callers of the above (maintenance, scanner, pipeline) — pass through the new `Bulk` entry points

**New files (frontend):**
- `web/src/pages/Diagnostics/AIJobsPanel.tsx`
- `web/src/pages/Diagnostics/AIJobsPanel.test.tsx`

**New/modified (API layer):**
- `internal/server/ai_jobs_handlers.go` — `GET /api/v1/ai-jobs`
- `internal/server/routes.go` (or equivalent) — register route

---

## Conventions

- **Every new Go file starts with a versioned header** (see repo convention):
  ```go
  // file: internal/ai/aijobs/aijobs.go
  // version: 1.0.0
  // guid: <fresh uuid — generate with `uuidgen | tr A-Z a-z`>
  ```
  Bump version on every edit within this plan.
- **Conventional commits only** (`feat:`, `refactor:`, `test:`, `fix:`, `docs:`, `chore:`).
- **Commit after each task's last step** unless the task explicitly says to defer.
- **Run `go build ./...` before committing any Go change** — fail-fast on compile errors.
- **Run `go test ./<changed-package>/...` before committing** — fail-fast on test regressions.

---

# Phase 1 — Foundation + Reference Migration (serial)

## Task 1.1 — Database migration + store

**Files:**
- Modify: `internal/database/migrations.go` (append new migration entry + `migration052Up` function)
- Create: `internal/database/ai_jobs_store.go`
- Create: `internal/database/ai_jobs_store_test.go`

- [ ] **Step 1: Add the migration entry in `migrations.go`**

Append to the `migrations` slice (after the Version 51 entry):

```go
	{
		Version:     52,
		Description: "Add ai_jobs and ai_job_payloads tables for unified batch tracking",
		Up:          migration052Up,
		Down:        nil,
	},
```

Append the function at the bottom of the file (after `migration050Up`):

```go
// migration052Up creates the ai_jobs tracking table and ai_job_payloads
// blob table used by the internal/ai/aijobs package to route bulk LLM work
// through the OpenAI Batch API.
func migration052Up(store Store) error {
	sqliteStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB: schema-free
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ai_jobs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			batch_id TEXT,
			custom_id_prefix TEXT NOT NULL,
			status TEXT NOT NULL,
			item_count INTEGER NOT NULL,
			success_count INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			row_errors TEXT,
			error_msg TEXT,
			submitted_at TIMESTAMP,
			completed_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_jobs_status_created ON ai_jobs(status, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_jobs_type_created ON ai_jobs(type, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_jobs_batch_id ON ai_jobs(batch_id) WHERE batch_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS ai_job_payloads (
			job_id TEXT PRIMARY KEY,
			items_json BLOB NOT NULL,
			FOREIGN KEY (job_id) REFERENCES ai_jobs(id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range stmts {
		if _, err := sqliteStore.db.Exec(stmt); err != nil {
			log.Printf("  - [WARN] migration 52: %v (continuing)", err)
		}
	}
	log.Println("  - Created ai_jobs, ai_job_payloads")
	return nil
}
```

- [ ] **Step 2: Write the store tests (TDD — these fail first)**

Create `internal/database/ai_jobs_store_test.go`:

```go
// file: internal/database/ai_jobs_store_test.go
// version: 1.0.0
// guid: <generate with uuidgen>

package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAIJobsStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(store))
	return store
}

func TestAIJobs_CreateAndGet(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	job := AIJob{
		ID:             "01TEST",
		Type:           "dedup_review",
		CustomIDPrefix: "01TEST",
		Status:         "pending",
		ItemCount:      5,
		CreatedAt:      time.Now(),
	}
	err := store.CreateAIJob(job, []byte(`[{"idx":1}]`))
	require.NoError(t, err)

	got, err := store.GetAIJob("01TEST")
	require.NoError(t, err)
	assert.Equal(t, "dedup_review", got.Type)
	assert.Equal(t, "pending", got.Status)
	assert.Equal(t, 5, got.ItemCount)

	payload, err := store.GetAIJobPayload("01TEST")
	require.NoError(t, err)
	assert.JSONEq(t, `[{"idx":1}]`, string(payload))
}

func TestAIJobs_UpdateStatus(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	job := AIJob{ID: "01TEST", Type: "x", CustomIDPrefix: "01TEST", Status: "pending", ItemCount: 1, CreatedAt: time.Now()}
	require.NoError(t, store.CreateAIJob(job, []byte("[]")))

	require.NoError(t, store.MarkAIJobSubmitted("01TEST", "batch_abc123"))
	got, err := store.GetAIJob("01TEST")
	require.NoError(t, err)
	assert.Equal(t, "submitted", got.Status)
	assert.Equal(t, "batch_abc123", got.BatchID)
	assert.False(t, got.SubmittedAt.IsZero())

	require.NoError(t, store.MarkAIJobCompleted("01TEST", "completed_with_errors", 3, 2, []AIJobRowError{
		{CustomID: "01TEST-4", Error: "boom"},
	}))
	got, err = store.GetAIJob("01TEST")
	require.NoError(t, err)
	assert.Equal(t, "completed_with_errors", got.Status)
	assert.Equal(t, 3, got.SuccessCount)
	assert.Equal(t, 2, got.ErrorCount)
	var errs []AIJobRowError
	require.NoError(t, json.Unmarshal([]byte(got.RowErrors), &errs))
	assert.Len(t, errs, 1)
	assert.Equal(t, "01TEST-4", errs[0].CustomID)
}

func TestAIJobs_MarkFailed(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	job := AIJob{ID: "01TEST", Type: "x", CustomIDPrefix: "01TEST", Status: "pending", ItemCount: 1, CreatedAt: time.Now()}
	require.NoError(t, store.CreateAIJob(job, []byte("[]")))

	require.NoError(t, store.MarkAIJobFailed("01TEST", "quota exceeded"))
	got, err := store.GetAIJob("01TEST")
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Equal(t, "quota exceeded", got.ErrorMsg)
}

func TestAIJobs_LookupByBatchID(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	job := AIJob{ID: "01TEST", Type: "x", CustomIDPrefix: "01TEST", Status: "pending", ItemCount: 1, CreatedAt: time.Now()}
	require.NoError(t, store.CreateAIJob(job, []byte("[]")))
	require.NoError(t, store.MarkAIJobSubmitted("01TEST", "batch_xyz"))

	got, err := store.GetAIJobByBatchID("batch_xyz")
	require.NoError(t, err)
	assert.Equal(t, "01TEST", got.ID)
}

func TestAIJobs_List(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	for i, status := range []string{"pending", "submitted", "completed"} {
		job := AIJob{
			ID:             string(rune('A'+i)) + "1",
			Type:           "dedup_review",
			CustomIDPrefix: "x",
			Status:         status,
			ItemCount:      1,
			CreatedAt:      time.Now(),
		}
		require.NoError(t, store.CreateAIJob(job, []byte("[]")))
	}

	all, err := store.ListAIJobs("", "", 10, 0)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	pending, err := store.ListAIJobs("dedup_review", "pending", 10, 0)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}
```

- [ ] **Step 3: Run tests — expect them to fail with "undefined: AIJob"**

Run: `go test ./internal/database/ -run TestAIJobs -v`
Expected: FAIL with `undefined: AIJob` (type does not exist yet).

- [ ] **Step 4: Create `internal/database/ai_jobs_store.go` with types + methods**

```go
// file: internal/database/ai_jobs_store.go
// version: 1.0.0
// guid: <generate with uuidgen>

package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// AIJob is one tracked bulk LLM job submitted through the aijobs package.
type AIJob struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	BatchID        string    `json:"batch_id,omitempty"`
	CustomIDPrefix string    `json:"custom_id_prefix"`
	Status         string    `json:"status"` // pending|submitted|completed|completed_with_errors|failed|expired
	ItemCount      int       `json:"item_count"`
	SuccessCount   int       `json:"success_count"`
	ErrorCount     int       `json:"error_count"`
	RowErrors      string    `json:"row_errors,omitempty"` // JSON-encoded []AIJobRowError
	ErrorMsg       string    `json:"error_msg,omitempty"`
	SubmittedAt    time.Time `json:"submitted_at,omitempty"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// AIJobRowError is one failed row within an otherwise successful batch.
type AIJobRowError struct {
	CustomID string `json:"custom_id"`
	Error    string `json:"error"`
}

const maxRowErrorsStored = 100

// CreateAIJob inserts the job row and its payload atomically.
func (s *SQLiteStore) CreateAIJob(job AIJob, payloadJSON []byte) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO ai_jobs
		(id, type, batch_id, custom_id_prefix, status, item_count, success_count, error_count, row_errors, error_msg, submitted_at, completed_at, created_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, 0, 0, NULL, NULL, NULL, NULL, ?)`,
		job.ID, job.Type, job.BatchID, job.CustomIDPrefix, job.Status, job.ItemCount, job.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert ai_job: %w", err)
	}
	_, err = tx.Exec(`INSERT INTO ai_job_payloads (job_id, items_json) VALUES (?, ?)`, job.ID, payloadJSON)
	if err != nil {
		return fmt.Errorf("insert ai_job_payload: %w", err)
	}
	return tx.Commit()
}

// GetAIJob returns the job row by ID.
func (s *SQLiteStore) GetAIJob(id string) (AIJob, error) {
	return scanAIJob(s.db.QueryRow(aiJobSelectColumns+` FROM ai_jobs WHERE id = ?`, id))
}

// GetAIJobByBatchID returns the job row owning the given OpenAI batch ID.
func (s *SQLiteStore) GetAIJobByBatchID(batchID string) (AIJob, error) {
	return scanAIJob(s.db.QueryRow(aiJobSelectColumns+` FROM ai_jobs WHERE batch_id = ?`, batchID))
}

// GetAIJobPayload returns the stored items_json blob.
func (s *SQLiteStore) GetAIJobPayload(id string) ([]byte, error) {
	var b []byte
	err := s.db.QueryRow(`SELECT items_json FROM ai_job_payloads WHERE job_id = ?`, id).Scan(&b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// MarkAIJobSubmitted flips status to "submitted" and stamps batch_id + submitted_at.
func (s *SQLiteStore) MarkAIJobSubmitted(id, batchID string) error {
	_, err := s.db.Exec(`UPDATE ai_jobs SET status = 'submitted', batch_id = ?, submitted_at = ? WHERE id = ?`,
		batchID, time.Now(), id)
	return err
}

// MarkAIJobCompleted records terminal success (possibly with per-row errors).
func (s *SQLiteStore) MarkAIJobCompleted(id, status string, successCount, errorCount int, rowErrors []AIJobRowError) error {
	if len(rowErrors) > maxRowErrorsStored {
		rowErrors = rowErrors[:maxRowErrorsStored]
	}
	var errorsJSON []byte
	if len(rowErrors) > 0 {
		var err error
		errorsJSON, err = json.Marshal(rowErrors)
		if err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE ai_jobs
		SET status = ?, success_count = ?, error_count = ?, row_errors = ?, completed_at = ?
		WHERE id = ?`,
		status, successCount, errorCount, nullableBytes(errorsJSON), time.Now(), id)
	return err
}

// MarkAIJobFailed records job-level failure (submission error, expiry).
func (s *SQLiteStore) MarkAIJobFailed(id, errMsg string) error {
	_, err := s.db.Exec(`UPDATE ai_jobs SET status = 'failed', error_msg = ?, completed_at = ? WHERE id = ?`,
		errMsg, time.Now(), id)
	return err
}

// ListAIJobs returns jobs filtered by type/status (either may be empty to skip that filter).
func (s *SQLiteStore) ListAIJobs(typeFilter, statusFilter string, limit, offset int) ([]AIJob, error) {
	q := aiJobSelectColumns + ` FROM ai_jobs WHERE 1=1`
	args := []any{}
	if typeFilter != "" {
		q += ` AND type = ?`
		args = append(args, typeFilter)
	}
	if statusFilter != "" {
		q += ` AND status = ?`
		args = append(args, statusFilter)
	}
	q += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AIJob
	for rows.Next() {
		j, err := scanAIJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

const aiJobSelectColumns = `SELECT id, type, COALESCE(batch_id, ''), custom_id_prefix, status, item_count, success_count, error_count, COALESCE(row_errors, ''), COALESCE(error_msg, ''), submitted_at, completed_at, created_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanAIJob(r rowScanner) (AIJob, error) {
	return scanAIJobRow(r)
}

func scanAIJobRow(r rowScanner) (AIJob, error) {
	var j AIJob
	var submitted, completed sql.NullTime
	err := r.Scan(&j.ID, &j.Type, &j.BatchID, &j.CustomIDPrefix, &j.Status, &j.ItemCount,
		&j.SuccessCount, &j.ErrorCount, &j.RowErrors, &j.ErrorMsg, &submitted, &completed, &j.CreatedAt)
	if err != nil {
		return AIJob{}, err
	}
	if submitted.Valid {
		j.SubmittedAt = submitted.Time
	}
	if completed.Valid {
		j.CompletedAt = completed.Time
	}
	return j, nil
}

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
```

- [ ] **Step 5: Run tests — all pass**

Run: `go test ./internal/database/ -run TestAIJobs -v`
Expected: PASS (5 tests).

- [ ] **Step 6: Add interface**

Open `internal/database/iface_ops.go` (or the equivalent `iface_*.go` file matching the repo's per-domain convention — run `ls internal/database/iface_*.go` and pick the one whose domain fits best, else create `iface_aijobs.go`). Add a new interface:

```go
// AIJobsStore is the subset of Store used by internal/ai/aijobs.
type AIJobsStore interface {
	CreateAIJob(job AIJob, payloadJSON []byte) error
	GetAIJob(id string) (AIJob, error)
	GetAIJobByBatchID(batchID string) (AIJob, error)
	GetAIJobPayload(id string) ([]byte, error)
	MarkAIJobSubmitted(id, batchID string) error
	MarkAIJobCompleted(id, status string, successCount, errorCount int, rowErrors []AIJobRowError) error
	MarkAIJobFailed(id, errMsg string) error
	ListAIJobs(typeFilter, statusFilter string, limit, offset int) ([]AIJob, error)
}
```

Verify `*SQLiteStore` satisfies it: `go build ./internal/database/...`.

- [ ] **Step 7: Commit**

```bash
go build ./...
go test ./internal/database/ -run TestAIJobs
git add internal/database/migrations.go internal/database/ai_jobs_store.go internal/database/ai_jobs_store_test.go internal/database/iface_*.go
git commit -m "$(cat <<'EOF'
feat(database): migration 52 — ai_jobs + ai_job_payloads tables

Adds tracking schema for the new internal/ai/aijobs unified batch layer.
Includes the SQLite store methods and tests; PebbleDB path is a no-op
since the batch-tracking flow is SQLite-only (matches deferred_itunes_updates).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1.2 — `internal/ai/aijobs` package skeleton

**Files:**
- Create: `internal/ai/aijobs/doc.go`
- Create: `internal/ai/aijobs/aijobs.go`
- Create: `internal/ai/aijobs/aijobs_test.go`

- [ ] **Step 1: Write the package godoc**

```go
// file: internal/ai/aijobs/doc.go
// version: 1.0.0
// guid: <generate with uuidgen>

// Package aijobs routes bulk-scale LLM chat-completion work through the
// OpenAI Batch API. It sits on top of internal/server.BatchPoller and the
// internal/ai.OpenAIParser batch helpers.
//
// Usage for a feature:
//
//	aijobs.Register("my_feature", func(ctx context.Context, deps aijobs.Deps, itemsJSON []byte, results []aijobs.RowResult) (int, int, []database.AIJobRowError, error) {
//	    // 1. Deserialize items from itemsJSON (feature-specific type)
//	    // 2. For each row in results, match by CustomID to an item
//	    // 3. Apply the result (feature-specific DB mutation), capturing per-row errors
//	    // 4. Return (successCount, errorCount, rowErrors, nil)
//	})
//
//	jobID, err := aijobs.Submit(ctx, deps, aijobs.SubmitRequest{
//	    Type:  "my_feature",
//	    Items: myItems,
//	    Build: func(i int, item MyItem) (aijobs.BatchRequest, error) { ... },
//	})
//
// Synchronous callers stay on internal/ai directly; bulk callers go through Submit.
// The split is enforced by internal/ai.priority_marker_test.go.
package aijobs
```

- [ ] **Step 2: Write failing tests**

```go
// file: internal/ai/aijobs/aijobs_test.go
// version: 1.0.0
// guid: <generate with uuidgen>

package aijobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore is a minimal in-memory AIJobsStore for tests.
type fakeStore struct {
	jobs     map[string]database.AIJob
	payloads map[string][]byte
}

func newFakeStore() *fakeStore {
	return &fakeStore{jobs: map[string]database.AIJob{}, payloads: map[string][]byte{}}
}

func (f *fakeStore) CreateAIJob(j database.AIJob, p []byte) error {
	f.jobs[j.ID] = j
	f.payloads[j.ID] = p
	return nil
}
func (f *fakeStore) GetAIJob(id string) (database.AIJob, error)           { return f.jobs[id], nil }
func (f *fakeStore) GetAIJobByBatchID(b string) (database.AIJob, error) {
	for _, j := range f.jobs {
		if j.BatchID == b {
			return j, nil
		}
	}
	return database.AIJob{}, errors.New("not found")
}
func (f *fakeStore) GetAIJobPayload(id string) ([]byte, error) { return f.payloads[id], nil }
func (f *fakeStore) MarkAIJobSubmitted(id, b string) error {
	j := f.jobs[id]
	j.Status = "submitted"
	j.BatchID = b
	f.jobs[id] = j
	return nil
}
func (f *fakeStore) MarkAIJobCompleted(id, status string, s, e int, re []database.AIJobRowError) error {
	j := f.jobs[id]
	j.Status = status
	j.SuccessCount = s
	j.ErrorCount = e
	if len(re) > 0 {
		b, _ := json.Marshal(re)
		j.RowErrors = string(b)
	}
	f.jobs[id] = j
	return nil
}
func (f *fakeStore) MarkAIJobFailed(id, msg string) error {
	j := f.jobs[id]
	j.Status = "failed"
	j.ErrorMsg = msg
	f.jobs[id] = j
	return nil
}
func (f *fakeStore) ListAIJobs(t, s string, l, o int) ([]database.AIJob, error) {
	var out []database.AIJob
	for _, j := range f.jobs {
		if t != "" && j.Type != t {
			continue
		}
		if s != "" && j.Status != s {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

// fakeBatchClient satisfies BatchClient for tests.
type fakeBatchClient struct {
	uploadCalls   int
	createCalls   int
	lastJSONL     []byte
	lastType      string
	returnBatchID string
	returnErr     error
}

func (f *fakeBatchClient) UploadBatchFile(ctx context.Context, data []byte) (string, error) {
	f.uploadCalls++
	f.lastJSONL = append([]byte(nil), data...)
	return "file_123", f.returnErr
}
func (f *fakeBatchClient) CreateBatchWithMetadata(ctx context.Context, fileID, batchType string) (string, error) {
	f.createCalls++
	f.lastType = batchType
	if f.returnErr != nil {
		return "", f.returnErr
	}
	return f.returnBatchID, nil
}

func TestSubmit_HappyPath(t *testing.T) {
	store := newFakeStore()
	client := &fakeBatchClient{returnBatchID: "batch_xyz"}
	deps := Deps{Store: store, Client: client}

	items := []string{"alpha", "beta", "gamma"}
	jobID, err := Submit(context.Background(), deps, SubmitRequest{
		Type: "test_feature",
		ItemCount: len(items),
		PayloadJSON: mustMarshal(items),
		Build: func(i int) (BatchRequest, error) {
			return BatchRequest{Body: map[string]any{"item": items[i]}, MaxTokens: 100}, nil
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, jobID)

	j, _ := store.GetAIJob(jobID)
	assert.Equal(t, "submitted", j.Status)
	assert.Equal(t, "batch_xyz", j.BatchID)
	assert.Equal(t, 3, j.ItemCount)
	assert.Equal(t, "aijobs", client.lastType) // BatchPoller routes all aijobs under one metadata type
	assert.Equal(t, 1, client.uploadCalls)
	assert.Equal(t, 1, client.createCalls)

	// JSONL must have 3 lines, each with a custom_id and the chat-completions URL
	lines := bytesSplitLines(client.lastJSONL)
	assert.Len(t, lines, 3)
}

func TestSubmit_UploadFailure_MarksRowFailed(t *testing.T) {
	store := newFakeStore()
	client := &fakeBatchClient{returnErr: errors.New("insufficient_quota")}
	deps := Deps{Store: store, Client: client}

	_, err := Submit(context.Background(), deps, SubmitRequest{
		Type: "test_feature", ItemCount: 1, PayloadJSON: []byte("[]"),
		Build: func(i int) (BatchRequest, error) { return BatchRequest{Body: map[string]any{}}, nil },
	})
	require.Error(t, err)

	// A row was created and then marked failed
	jobs, _ := store.ListAIJobs("test_feature", "failed", 10, 0)
	assert.Len(t, jobs, 1)
	assert.Contains(t, jobs[0].ErrorMsg, "insufficient_quota")
}

func TestDispatch_PerRowErrorsIsolated(t *testing.T) {
	store := newFakeStore()
	Register("test_feature", func(ctx context.Context, itemsJSON []byte, results []RowResult) (int, int, []database.AIJobRowError, error) {
		success, fail := 0, 0
		var errs []database.AIJobRowError
		for _, r := range results {
			if r.CustomID == "bad-1" {
				fail++
				errs = append(errs, database.AIJobRowError{CustomID: r.CustomID, Error: "bad row"})
				continue
			}
			success++
		}
		return success, fail, errs, nil
	})
	// Seed a job with payload
	jobID := "01DISP"
	_ = store.CreateAIJob(database.AIJob{ID: jobID, Type: "test_feature", CustomIDPrefix: "01DISP", Status: "submitted", ItemCount: 2, BatchID: "batch_d"}, []byte(`[{"x":1},{"x":2}]`))
	_ = store.MarkAIJobSubmitted(jobID, "batch_d")

	results := []RowResult{
		{CustomID: "good-1", Body: map[string]any{"ok": true}},
		{CustomID: "bad-1", Body: map[string]any{"ok": false}},
	}
	err := Dispatch(context.Background(), store, "batch_d", results)
	require.NoError(t, err)

	j, _ := store.GetAIJob(jobID)
	assert.Equal(t, "completed_with_errors", j.Status)
	assert.Equal(t, 1, j.SuccessCount)
	assert.Equal(t, 1, j.ErrorCount)
	assert.Contains(t, j.RowErrors, "bad-1")
}

func TestDispatch_AllSuccess_MarksCompleted(t *testing.T) {
	store := newFakeStore()
	Register("only_success", func(ctx context.Context, itemsJSON []byte, results []RowResult) (int, int, []database.AIJobRowError, error) {
		return len(results), 0, nil, nil
	})
	_ = store.CreateAIJob(database.AIJob{ID: "01OK", Type: "only_success", CustomIDPrefix: "01OK", Status: "submitted", ItemCount: 1, BatchID: "batch_ok"}, []byte("[]"))
	_ = store.MarkAIJobSubmitted("01OK", "batch_ok")

	err := Dispatch(context.Background(), store, "batch_ok", []RowResult{{CustomID: "x", Body: map[string]any{}}})
	require.NoError(t, err)
	j, _ := store.GetAIJob("01OK")
	assert.Equal(t, "completed", j.Status)
}

func TestDispatch_PanicInCallbackRecovered(t *testing.T) {
	store := newFakeStore()
	Register("panicker", func(ctx context.Context, itemsJSON []byte, results []RowResult) (int, int, []database.AIJobRowError, error) {
		panic("boom")
	})
	_ = store.CreateAIJob(database.AIJob{ID: "01P", Type: "panicker", CustomIDPrefix: "01P", Status: "submitted", ItemCount: 1, BatchID: "batch_p"}, []byte("[]"))
	_ = store.MarkAIJobSubmitted("01P", "batch_p")

	err := Dispatch(context.Background(), store, "batch_p", []RowResult{{CustomID: "x"}})
	require.Error(t, err)
	j, _ := store.GetAIJob("01P")
	assert.Equal(t, "failed", j.Status)
	assert.Contains(t, j.ErrorMsg, "panic")
}

// helpers
func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func bytesSplitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}
```

- [ ] **Step 3: Run — expect fail (types undefined)**

Run: `go test ./internal/ai/aijobs/...`
Expected: FAIL, compilation errors for `Submit`, `Dispatch`, `Register`, `Deps`, `SubmitRequest`, `BatchRequest`, `RowResult`.

- [ ] **Step 4: Implement the package**

```go
// file: internal/ai/aijobs/aijobs.go
// version: 1.0.0
// guid: <generate with uuidgen>

package aijobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/oklog/ulid/v2"
)

// BatchClient is the subset of internal/ai.OpenAIParser methods aijobs uses.
// Defined as an interface so tests can inject fakes without depending on the real client.
type BatchClient interface {
	UploadBatchFile(ctx context.Context, data []byte) (string, error)
	CreateBatchWithMetadata(ctx context.Context, fileID, batchType string) (string, error)
}

// Deps is the runtime dependency set for Submit.
type Deps struct {
	Store  database.AIJobsStore
	Client BatchClient
}

// BatchRequest is one row that will be serialized as JSONL for OpenAI's Batch API.
// Body is the raw /v1/chat/completions request body (messages, model, response_format, etc.).
type BatchRequest struct {
	Body      map[string]any
	MaxTokens int64 // informational only; callers put max_completion_tokens inside Body if needed
}

// SubmitRequest is the caller-facing payload for a new aijobs batch.
type SubmitRequest struct {
	Type        string // feature type, e.g. "dedup_review"
	ItemCount   int    // len(items) — mirrored on the ai_jobs row for visibility
	PayloadJSON []byte // serialized items slice — redelivered to the callback at Dispatch time
	Build       func(i int) (BatchRequest, error)
}

// RowResult is one parsed line from an OpenAI batch output file.
type RowResult struct {
	CustomID string
	Body     map[string]any // the chat-completion response body for this row
	Err      string         // non-empty if OpenAI reported an error for this row
}

// CompletionCallback applies a batch's results. It must:
//   1. Deserialize itemsJSON into its feature-specific item slice
//   2. Match each result to an item by CustomID (the convention: "<prefix>-<index>")
//   3. Apply the result (DB write, etc.), catching per-row errors into the returned slice
//   4. Return (successCount, errorCount, rowErrors, fatalErr). A non-nil fatalErr means the
//      whole batch could not be processed and the job row is marked failed.
type CompletionCallback func(ctx context.Context, itemsJSON []byte, results []RowResult) (successCount, errorCount int, rowErrors []database.AIJobRowError, fatalErr error)

var (
	registry   = map[string]CompletionCallback{}
	registryMu sync.RWMutex
)

// Register associates a type string with its completion callback. Call at package-init
// time from each feature's package (e.g. init() in internal/ai/dedup_review.go).
func Register(typ string, cb CompletionCallback) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[typ] = cb
}

// ClearRegistryForTest resets the registry — test-only helper.
func ClearRegistryForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]CompletionCallback{}
}

// Submit persists a new ai_jobs row, uploads a JSONL batch file, and creates an
// OpenAI batch. Returns the job ID. On upload/create failure the job row is
// marked "failed" and the original error is returned.
func Submit(ctx context.Context, deps Deps, req SubmitRequest) (string, error) {
	if req.ItemCount == 0 {
		return "", fmt.Errorf("aijobs.Submit: no items")
	}
	if req.Build == nil {
		return "", fmt.Errorf("aijobs.Submit: nil Build")
	}

	jobID := ulid.Make().String()
	customIDPrefix := jobID

	// 1. Build the JSONL body up front — if Build returns an error, we haven't
	//    touched the store yet.
	var buf bytes.Buffer
	for i := 0; i < req.ItemCount; i++ {
		br, err := req.Build(i)
		if err != nil {
			return "", fmt.Errorf("aijobs.Submit: build row %d: %w", i, err)
		}
		line := map[string]any{
			"custom_id": fmt.Sprintf("%s-%d", customIDPrefix, i),
			"method":    "POST",
			"url":       "/v1/chat/completions",
			"body":      br.Body,
		}
		b, err := json.Marshal(line)
		if err != nil {
			return "", fmt.Errorf("aijobs.Submit: marshal row %d: %w", i, err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}

	// 2. Insert the pending job row with its payload.
	job := database.AIJob{
		ID:             jobID,
		Type:           req.Type,
		CustomIDPrefix: customIDPrefix,
		Status:         "pending",
		ItemCount:      req.ItemCount,
		CreatedAt:      time.Now(),
	}
	if err := deps.Store.CreateAIJob(job, req.PayloadJSON); err != nil {
		return "", fmt.Errorf("aijobs.Submit: store.CreateAIJob: %w", err)
	}

	// 3. Upload + create batch. On any failure, mark the row failed and return.
	fileID, err := deps.Client.UploadBatchFile(ctx, buf.Bytes())
	if err != nil {
		_ = deps.Store.MarkAIJobFailed(jobID, fmt.Sprintf("upload: %v", err))
		return jobID, err
	}
	batchID, err := deps.Client.CreateBatchWithMetadata(ctx, fileID, "aijobs")
	if err != nil {
		_ = deps.Store.MarkAIJobFailed(jobID, fmt.Sprintf("create batch: %v", err))
		return jobID, err
	}

	if err := deps.Store.MarkAIJobSubmitted(jobID, batchID); err != nil {
		return jobID, fmt.Errorf("aijobs.Submit: mark submitted: %w", err)
	}
	log.Printf("[INFO] aijobs: submitted job %s type=%s batch=%s items=%d", jobID, req.Type, batchID, req.ItemCount)
	return jobID, nil
}

// Dispatch is called by the BatchPoller when an aijobs batch completes.
// It looks up the ai_jobs row, loads the payload, invokes the registered callback,
// and records the outcome.
func Dispatch(ctx context.Context, store database.AIJobsStore, batchID string, results []RowResult) (err error) {
	job, err := store.GetAIJobByBatchID(batchID)
	if err != nil {
		return fmt.Errorf("aijobs.Dispatch: lookup batch %s: %w", batchID, err)
	}

	registryMu.RLock()
	cb, ok := registry[job.Type]
	registryMu.RUnlock()
	if !ok {
		msg := fmt.Sprintf("no callback registered for type %q", job.Type)
		_ = store.MarkAIJobFailed(job.ID, msg)
		return fmt.Errorf("aijobs.Dispatch: %s", msg)
	}

	payload, err := store.GetAIJobPayload(job.ID)
	if err != nil {
		_ = store.MarkAIJobFailed(job.ID, fmt.Sprintf("load payload: %v", err))
		return fmt.Errorf("aijobs.Dispatch: load payload: %w", err)
	}

	// Recover from panics in the callback so one bad feature cannot crash the poller.
	var successCount, errorCount int
	var rowErrors []database.AIJobRowError
	var fatalErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				fatalErr = fmt.Errorf("callback panic: %v", r)
			}
		}()
		successCount, errorCount, rowErrors, fatalErr = cb(ctx, payload, results)
	}()

	if fatalErr != nil {
		_ = store.MarkAIJobFailed(job.ID, fatalErr.Error())
		return fatalErr
	}

	status := "completed"
	if errorCount > 0 {
		status = "completed_with_errors"
	}
	if err := store.MarkAIJobCompleted(job.ID, status, successCount, errorCount, rowErrors); err != nil {
		return fmt.Errorf("aijobs.Dispatch: mark completed: %w", err)
	}
	log.Printf("[INFO] aijobs: dispatched job %s type=%s success=%d errors=%d", job.ID, job.Type, successCount, errorCount)
	return nil
}
```

- [ ] **Step 5: Add the ulid dependency if missing**

Run: `go get github.com/oklog/ulid/v2`
Then: `go mod tidy`

- [ ] **Step 6: Run tests — all pass**

Run: `go test ./internal/ai/aijobs/... -v`
Expected: PASS (5 tests).

- [ ] **Step 7: Build the whole repo**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add internal/ai/aijobs/ go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(ai/aijobs): unified bulk-LLM batch submission + dispatch layer

Submit serializes items to JSONL, uploads via OpenAIParser helpers, creates a
batch with metadata.type="aijobs", and persists an ai_jobs row. Dispatch is
called by BatchPoller on completion; it looks up the row, loads the payload,
invokes the feature-registered callback, and records per-row outcomes.

Panics in callbacks are recovered; the job row is marked failed but the poller
continues.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1.3 — BatchPoller wiring + result parsing

The `BatchPoller` currently hands handlers `(batchID, outputFileID)` and each handler downloads the output file itself. `aijobs.Dispatch` takes `[]RowResult` (pre-parsed), so we need a thin adapter that reads the output file, parses the JSONL lines, and calls `Dispatch`.

**Files:**
- Create: `internal/ai/openai_batch_raw.go` (if `DownloadBatchRaw` doesn't already return the shape we need — check first)
- Modify: `internal/server/batch_poller.go` (register `"aijobs"` handler)
- Modify: `internal/server/server.go` (pass the store into `aijobs.Deps`)

- [ ] **Step 1: Inspect `DownloadBatchRaw`**

Run: `grep -n "DownloadBatchRaw\|func.*Download" internal/ai/*.go`

`DownloadBatchRaw` returns `[]map[string]any` where each map has `custom_id`, `response.body`, and possibly `error`. That's what we need. If the signature is different (check `openai_batch.go`), write a thin adapter that converts to `[]aijobs.RowResult`.

- [ ] **Step 2: Add the adapter to the aijobs handler registration in `batch_poller.go`**

In `registerBatchPollerHandlers` (after the existing `"pipeline"` handler), add:

```go
	// aijobs: unified layer for all bulk-scale LLM work. All such batches
	// carry metadata.type="aijobs"; the per-feature routing happens inside
	// aijobs.Dispatch by looking up the ai_jobs row for this batch_id.
	s.batchPoller.RegisterHandler("aijobs", func(ctx context.Context, batchID, outputFileID string) error {
		if outputFileID == "" {
			return fmt.Errorf("aijobs: no output file for batch %s", batchID)
		}
		raw, err := s.batchPoller.parser.DownloadBatchRaw(ctx, outputFileID)
		if err != nil {
			return fmt.Errorf("aijobs: download batch %s: %w", batchID, err)
		}
		results := make([]aijobs.RowResult, 0, len(raw))
		for _, r := range raw {
			results = append(results, aijobs.RowResult{
				CustomID: r.CustomID,
				Content:  r.Content,
				Err:      r.Error,
			})
		}
		return aijobs.Dispatch(ctx, s.Store().(database.AIJobsStore), batchID, results)
	})
```

Add imports to `batch_poller.go`:

```go
	"github.com/jdfalk/audiobook-organizer/internal/ai/aijobs"
```

Bump the file version header.

- [ ] **Step 3: Verify `OpenAIParser` satisfies `aijobs.BatchClient`**

`aijobs.BatchClient` expects:
- `UploadBatchFile(ctx, data []byte) (string, error)`
- `CreateBatchWithMetadata(ctx, fileID, batchType string) (string, error)`

The existing `OpenAIParser.UploadBatchFile` takes `io.Reader`. Add a thin shim:

In `internal/ai/openai_batch.go`, add (keep the existing `io.Reader` version for other callers):

```go
// UploadBatchFileBytes is a convenience for aijobs which already has a []byte buffer.
func (p *OpenAIParser) UploadBatchFileBytes(ctx context.Context, data []byte) (string, error) {
	return p.UploadBatchFile(ctx, bytes.NewReader(data))
}
```

Import `"bytes"` if missing. Bump the file's version header.

Then add a thin adapter type in `internal/ai/aijobs_adapter.go`:

```go
// file: internal/ai/aijobs_adapter.go
// version: 1.0.0
// guid: <generate with uuidgen>

package ai

import "context"

// AIJobsBatchClient adapts OpenAIParser to the aijobs.BatchClient interface.
// (aijobs.BatchClient's UploadBatchFile signature is []byte rather than io.Reader.)
type AIJobsBatchClient struct {
	Parser *OpenAIParser
}

func (a *AIJobsBatchClient) UploadBatchFile(ctx context.Context, data []byte) (string, error) {
	return a.Parser.UploadBatchFileBytes(ctx, data)
}

func (a *AIJobsBatchClient) CreateBatchWithMetadata(ctx context.Context, fileID, batchType string) (string, error) {
	return a.Parser.CreateBatchWithMetadata(ctx, fileID, batchType)
}
```

- [ ] **Step 4: Build + run existing tests to confirm no regression**

```bash
go build ./...
go test ./internal/ai/... ./internal/server/... -count=1 -short
```
Expected: PASS (or whatever the current baseline is — record and proceed only if the new code isn't the cause of any failure).

- [ ] **Step 5: Commit**

```bash
git add internal/server/batch_poller.go internal/ai/openai_batch.go internal/ai/aijobs_adapter.go
git commit -m "$(cat <<'EOF'
feat(server): register aijobs handler on the BatchPoller

The aijobs handler downloads the batch output file, parses each row into
aijobs.RowResult, and hands the slice to aijobs.Dispatch which looks up
the ai_jobs row and invokes the feature-registered callback.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1.4 — Priority enforcement test + call-site audit

**Files:**
- Create: `internal/ai/priority_marker_test.go`

- [ ] **Step 1: Audit all `Chat.Completions.New` call sites under `internal/ai/`**

Run: `grep -rn "Chat\.Completions\.New" internal/ai/ --include="*.go"`

Record the output. Expected sites (from the design):
- `dedup_review.go:124`
- `metadata_llm_review.go:147`
- `openai_parser.go:136, 238, 329, 391, 544, 678`

For each of `openai_parser.go:544` and `openai_parser.go:678`, **read 50 lines of context** (the enclosing function + its doc comment) and walk the call chain:

```bash
# For each site, identify the enclosing function name, then:
grep -rn "<FuncName>(" --include="*.go" internal/ cmd/ main.go
```

Decide priority:
- If every caller is a maintenance/scheduler/scan loop → `Bulk`
- If any caller is a Gin HTTP handler serving a user-waiting request → consider `Split` (Interactive + Bulk)

Append your findings to the design doc (`docs/superpowers/specs/2026-04-24-aijobs-batch-migration-design.md`) under the "Open Items" section in this exact format:

```markdown
### Phase 1.4 audit results

- `openai_parser.go:544` — function: `<FuncName>`. Callers: `<list>`. Decision: `Bulk` | `Interactive` | `Split`.
- `openai_parser.go:678` — function: `<FuncName>`. Callers: `<list>`. Decision: `Bulk` | `Interactive` | `Split`.
```

- [ ] **Step 2: Write the enforcement test**

```go
// file: internal/ai/priority_marker_test.go
// version: 1.0.0
// guid: <generate with uuidgen>

package ai

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// allowListedSyncCallers enumerates the functions permitted to call
// client.Chat.Completions.New synchronously. Every such function MUST be
// a user-waiting / interactive path. Bulk work goes through internal/ai/aijobs.
//
// To add a caller: add the enclosing func name here AND add the comment
//
//     // PRIORITY: Interactive
//
// on the line immediately above the function declaration.
//
// Each migration in this plan removes an entry from this list until only
// genuinely interactive paths remain.
var allowListedSyncCallers = map[string]bool{
	// Seeded with all current sites; migrations strike them through.
	"reviewDedupBatch":      true, // dedup_review.go:124 — REMOVE in Task 1.5
	"reviewMetadataBatch":   true, // metadata_llm_review.go:147 — REMOVE in Task 2.1 (confirm name during migration)
	"ParsePath":             true, // openai_parser.go:136 — REMOVE in Task 2.2
	"ParseSeries":           true, // openai_parser.go:238 — REMOVE in Task 2.2
	"ParseMetadata":         true, // openai_parser.go:329 — REMOVE in Task 2.2
	"ParseCoverImage":       true, // openai_parser.go:391 — SPLIT in Task 2.3
	"parseOpenAIParser_544": true, // PLACEHOLDER — replace with real func name from Phase 1.4 audit
	"parseOpenAIParser_678": true, // PLACEHOLDER — replace with real func name from Phase 1.4 audit
}

var chatCompletionCallRe = regexp.MustCompile(`client\.Chat\.Completions\.New\(`)

// TestNoUnmarkedChatCompletionCallers fails if any .go file under internal/ai
// calls client.Chat.Completions.New from a function that is neither in the
// allow-list nor marked with "// PRIORITY: Interactive".
func TestNoUnmarkedChatCompletionCallers(t *testing.T) {
	root := "."
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")

		for i, line := range lines {
			if !chatCompletionCallRe.MatchString(line) {
				continue
			}
			// Walk backwards to find the enclosing func decl.
			funcName, priorityMark := findEnclosingFunc(lines, i)
			if funcName == "" {
				offenders = append(offenders, path+": unable to find enclosing func")
				continue
			}
			if allowListedSyncCallers[funcName] {
				continue
			}
			if priorityMark {
				continue
			}
			offenders = append(offenders, path+": "+funcName+" calls Chat.Completions.New but is neither allow-listed nor marked // PRIORITY: Interactive")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("unmarked sync callers:\n  %s", strings.Join(offenders, "\n  "))
	}
}

var funcDeclRe = regexp.MustCompile(`^func\s+(?:\(\s*\w+\s+\*?\w+\s*\)\s+)?(\w+)\s*\(`)

// findEnclosingFunc walks backward from lineIdx to find the `func Foo(...)` line.
// Returns the name and whether the line immediately preceding has the priority marker.
func findEnclosingFunc(lines []string, lineIdx int) (string, bool) {
	for i := lineIdx; i >= 0; i-- {
		m := funcDeclRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		priority := false
		if i > 0 && strings.Contains(lines[i-1], "PRIORITY: Interactive") {
			priority = true
		}
		return m[1], priority
	}
	return "", false
}
```

- [ ] **Step 3: Run the test — expect PASS (everything still allow-listed)**

Run: `go test ./internal/ai/ -run TestNoUnmarkedChatCompletionCallers -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/ai/priority_marker_test.go docs/superpowers/specs/2026-04-24-aijobs-batch-migration-design.md
git commit -m "$(cat <<'EOF'
test(ai): add priority-marker enforcement for Chat.Completions.New callers

Fails CI if any file under internal/ai calls Chat.Completions.New from a
function that is neither in the allow-list nor carries the
// PRIORITY: Interactive marker. Initially every current sync call site is
allow-listed; the aijobs migrations strike them through one by one.

Also records Phase 1.4 audit findings for openai_parser.go:544 and :678.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1.5 — Migrate `dedup_review.go` (reference implementation)

This is the template Phase 2 agents will copy. Be surgical and clean.

**Files:**
- Modify: `internal/ai/dedup_review.go`
- Create: `internal/ai/dedup_review_aijobs_test.go` (new tests alongside existing file)
- Modify: `internal/ai/priority_marker_test.go` (strike out `reviewDedupBatch` from allow-list)
- Modify: the existing maintenance caller of `ReviewDedupPairs` (see Step 4)

- [ ] **Step 1: Find the caller**

Run: `grep -rn "ReviewDedupPairs" --include="*.go" internal/ cmd/ main.go`
Expected: exactly one production caller in the dedup maintenance path (look at the call site to confirm it's the maintenance task that fired the 429 in the log — `internal/dedup/engine.go:1236` area).

Read the caller to understand what it does with the return value synchronously. The sync `ReviewDedupPairs` returns `[]DedupPairVerdict` which the caller persists. When we migrate, the caller can no longer receive the verdicts inline — the callback at completion time must persist them instead.

- [ ] **Step 2: Add the new Bulk entry point + registered callback in `dedup_review.go`**

Replace the existing `ReviewDedupPairs` and `reviewDedupBatch` with a **single** new function `SubmitDedupReviewJob` that takes the deps, builds per-pair chat requests, and calls `aijobs.Submit`. The callback uses the same system prompt, parses the verdicts, and persists them via a new method on whatever store owns dedup verdicts (check `internal/dedup/engine.go` for the current persistence call — likely something like `store.StoreDedupLLMVerdicts` or inline in the caller).

Replacement:

```go
// file: internal/ai/dedup_review.go
// version: 2.0.0 (bump)
// guid: <keep existing>

package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/ai/aijobs"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/openai/openai-go/packages/param"
)

// DedupEntity / DedupPairInput / DedupPairVerdict: unchanged — keep existing types.

// dedupReviewSystemPrompt is the shared prompt for per-pair dedup review.
// Extracted as a package-level const so the Batch API builder and the Interactive
// fallback use identical instructions.
const dedupReviewSystemPrompt = `You are an expert audiobook metadata reviewer. ... (paste the full prompt string currently in reviewDedupBatch unchanged)`

// DedupVerdictSink is the subset of the dedup store's API the callback uses.
// Implemented by *database.SQLiteStore in production.
type DedupVerdictSink interface {
	StoreDedupLLMVerdicts(verdicts []DedupPairVerdict) error
}

// init registers the aijobs completion callback for dedup_review.
func init() {
	aijobs.Register("dedup_review", dedupReviewCallback)
}

// dedupReviewCallbackSink is set by the server wiring layer (see server.go).
// It is a package-level hook so the init() above can register a stable callback
// that looks up the sink at Dispatch time.
var dedupReviewCallbackSink DedupVerdictSink

// SetDedupVerdictSink is called by server startup to inject the sink.
func SetDedupVerdictSink(s DedupVerdictSink) { dedupReviewCallbackSink = s }

// SubmitDedupReviewJob submits a bulk dedup-pair review job via the OpenAI Batch API.
// It returns the job ID; results are applied asynchronously when the batch completes
// via the callback registered in init().
//
// This replaces the former synchronous ReviewDedupPairs. Callers that used
// ReviewDedupPairs must be updated to enqueue the job and let the BatchPoller
// deliver results to the sink.
func SubmitDedupReviewJob(ctx context.Context, deps aijobs.Deps, model string, inputs []DedupPairInput) (string, error) {
	if len(inputs) == 0 {
		return "", fmt.Errorf("SubmitDedupReviewJob: no inputs")
	}

	payloadJSON, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("marshal inputs: %w", err)
	}

	// Group pairs into sub-batches of 25 (the former synchronous sub-batch size).
	// Each sub-batch becomes one row in the JSONL (one chat-completion call).
	const pairsPerRow = 25
	var rows [][]DedupPairInput
	for start := 0; start < len(inputs); start += pairsPerRow {
		end := start + pairsPerRow
		if end > len(inputs) {
			end = len(inputs)
		}
		rows = append(rows, inputs[start:end])
	}

	return aijobs.Submit(ctx, deps, aijobs.SubmitRequest{
		Type:        "dedup_review",
		ItemCount:   len(rows),
		PayloadJSON: payloadJSON,
		Build: func(i int) (aijobs.BatchRequest, error) {
			batch := rows[i]
			batchJSON, err := json.Marshal(batch)
			if err != nil {
				return aijobs.BatchRequest{}, err
			}
			userPrompt := fmt.Sprintf("Review these candidate duplicate pairs:\n\n%s", string(batchJSON))
			return aijobs.BatchRequest{
				Body: map[string]any{
					"model": model,
					"messages": []map[string]any{
						{"role": "system", "content": dedupReviewSystemPrompt},
						{"role": "user", "content": userPrompt},
					},
					"max_completion_tokens": 8000,
					"response_format":       map[string]any{"type": "json_object"},
					"prompt_cache_key":      "audiobook-dedup-pair-review-v1",
				},
				MaxTokens: 8000,
			}, nil
		},
	})
}

// dedupReviewCallback is invoked by aijobs.Dispatch when a dedup_review batch completes.
// Each row's response is a JSON object with a `verdicts` array; we flatten across
// all rows and persist via the injected sink.
func dedupReviewCallback(ctx context.Context, itemsJSON []byte, results []aijobs.RowResult) (int, int, []database.AIJobRowError, error) {
	sink := dedupReviewCallbackSink
	if sink == nil {
		return 0, 0, nil, fmt.Errorf("dedup review: no sink registered")
	}

	var allVerdicts []DedupPairVerdict
	var rowErrors []database.AIJobRowError
	success, failed := 0, 0

	for _, r := range results {
		if r.Err != "" {
			failed++
			rowErrors = append(rowErrors, database.AIJobRowError{CustomID: r.CustomID, Error: r.Err})
			continue
		}
		// r.Content is the raw model output string (the JSON from the assistant).
		var parsed struct {
			Verdicts []DedupPairVerdict `json:"verdicts"`
		}
		if err := json.Unmarshal([]byte(r.Content), &parsed); err != nil {
			failed++
			rowErrors = append(rowErrors, database.AIJobRowError{CustomID: r.CustomID, Error: fmt.Sprintf("parse: %v", err)})
			continue
		}
		allVerdicts = append(allVerdicts, parsed.Verdicts...)
		success++
	}

	if len(allVerdicts) > 0 {
		if err := sink.StoreDedupLLMVerdicts(allVerdicts); err != nil {
			// Sink-level failures are fatal — partial write is worse than none.
			return success, failed, rowErrors, fmt.Errorf("persist verdicts: %w", err)
		}
	}
	return success, failed, rowErrors, nil
}

// Guard: keep param import live in case we later add Interactive paths.
var _ = param.NewOpt[int64]
```

**Important:** Copy the FULL text of the original `systemPrompt` string (lines 82-102 of the current file) into `dedupReviewSystemPrompt` verbatim. Do not paraphrase.

- [ ] **Step 3: Write the new tests alongside**

```go
// file: internal/ai/dedup_review_aijobs_test.go
// version: 1.0.0
// guid: <generate with uuidgen>

package ai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/ai/aijobs"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeVerdictSink struct {
	stored []DedupPairVerdict
}

func (f *fakeVerdictSink) StoreDedupLLMVerdicts(v []DedupPairVerdict) error {
	f.stored = append(f.stored, v...)
	return nil
}

func TestDedupReviewCallback_HappyPath(t *testing.T) {
	sink := &fakeVerdictSink{}
	SetDedupVerdictSink(sink)
	t.Cleanup(func() { SetDedupVerdictSink(nil) })

	results := []aijobs.RowResult{
		{
			CustomID: "j-0",
			Body: map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"content": `{"verdicts":[{"index":1,"is_duplicate":true,"confidence":"high","reason":"same ISBN"}]}`,
					},
				}},
			},
		},
	}
	succ, fail, errs, err := dedupReviewCallback(context.Background(), []byte("[]"), results)
	require.NoError(t, err)
	assert.Equal(t, 1, succ)
	assert.Equal(t, 0, fail)
	assert.Empty(t, errs)
	require.Len(t, sink.stored, 1)
	assert.True(t, sink.stored[0].IsDuplicate)
}

func TestDedupReviewCallback_PerRowErrorsIsolated(t *testing.T) {
	sink := &fakeVerdictSink{}
	SetDedupVerdictSink(sink)
	t.Cleanup(func() { SetDedupVerdictSink(nil) })

	results := []aijobs.RowResult{
		{CustomID: "good", Content: `{"verdicts":[{"index":0,"is_duplicate":true,"confidence":"high","reason":""}]}`},
		{CustomID: "rowerr", Err: "rate_limit_exceeded"},
		{CustomID: "badjson", Content: `not-valid-json-at-content-level`},
	}
	succ, fail, errs, err := dedupReviewCallback(context.Background(), []byte("[]"), results)
	require.NoError(t, err)
	assert.Equal(t, 1, succ)
	assert.Equal(t, 2, fail)
	assert.Len(t, errs, 2)
	assert.Len(t, sink.stored, 1)
}

func TestSubmitDedupReviewJob_SplitsIntoSubBatches(t *testing.T) {
	// 51 pairs → 3 sub-batches of 25, 25, 1.
	inputs := make([]DedupPairInput, 51)
	for i := range inputs {
		inputs[i] = DedupPairInput{Index: i, EntityType: "book", A: DedupEntity{ID: "a"}, B: DedupEntity{ID: "b"}}
	}
	store := newMemAIJobsStore()
	client := &recordingBatchClient{}
	deps := aijobs.Deps{Store: store, Client: client}

	_, err := SubmitDedupReviewJob(context.Background(), deps, "gpt-4o-mini", inputs)
	require.NoError(t, err)
	assert.Equal(t, 1, client.uploads)
	// JSONL should have 3 lines (one per sub-batch).
	lines := splitLines(client.lastData)
	assert.Len(t, lines, 3)
}

// --- shared test helpers used by this and other migration tests ---

type memAIJobsStore struct {
	jobs     map[string]database.AIJob
	payloads map[string][]byte
}

func newMemAIJobsStore() *memAIJobsStore {
	return &memAIJobsStore{jobs: map[string]database.AIJob{}, payloads: map[string][]byte{}}
}
func (m *memAIJobsStore) CreateAIJob(j database.AIJob, p []byte) error {
	m.jobs[j.ID] = j
	m.payloads[j.ID] = p
	return nil
}
func (m *memAIJobsStore) GetAIJob(id string) (database.AIJob, error) { return m.jobs[id], nil }
func (m *memAIJobsStore) GetAIJobByBatchID(b string) (database.AIJob, error) {
	for _, j := range m.jobs {
		if j.BatchID == b {
			return j, nil
		}
	}
	return database.AIJob{}, nil
}
func (m *memAIJobsStore) GetAIJobPayload(id string) ([]byte, error) { return m.payloads[id], nil }
func (m *memAIJobsStore) MarkAIJobSubmitted(id, b string) error {
	j := m.jobs[id]
	j.Status = "submitted"
	j.BatchID = b
	m.jobs[id] = j
	return nil
}
func (m *memAIJobsStore) MarkAIJobCompleted(id, s string, a, e int, r []database.AIJobRowError) error {
	j := m.jobs[id]
	j.Status = s
	j.SuccessCount = a
	j.ErrorCount = e
	if len(r) > 0 {
		b, _ := json.Marshal(r)
		j.RowErrors = string(b)
	}
	m.jobs[id] = j
	return nil
}
func (m *memAIJobsStore) MarkAIJobFailed(id, msg string) error {
	j := m.jobs[id]
	j.Status = "failed"
	j.ErrorMsg = msg
	m.jobs[id] = j
	return nil
}
func (m *memAIJobsStore) ListAIJobs(_, _ string, _, _ int) ([]database.AIJob, error) {
	var out []database.AIJob
	for _, j := range m.jobs {
		out = append(out, j)
	}
	return out, nil
}

type recordingBatchClient struct {
	uploads  int
	creates  int
	lastData []byte
}

func (r *recordingBatchClient) UploadBatchFile(_ context.Context, data []byte) (string, error) {
	r.uploads++
	r.lastData = append([]byte(nil), data...)
	return "file", nil
}
func (r *recordingBatchClient) CreateBatchWithMetadata(_ context.Context, _, _ string) (string, error) {
	r.creates++
	return "batch", nil
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}
```

- [ ] **Step 4: Update the dedup caller**

Find the caller — run: `grep -rn "ReviewDedupPairs\b" --include="*.go" internal/ cmd/ main.go`.

In the caller, replace the synchronous call with `SubmitDedupReviewJob`. The caller's old "apply verdicts" logic moves to a new `StoreDedupLLMVerdicts` method on the dedup store (or reuses an existing persistence method if one is already there — inspect to confirm). The caller now returns after submission; the maintenance task's "completion" signal shifts to "submitted, awaiting batch".

Typical shape of the updated caller:

```go
// BEFORE (simplified):
//   verdicts, err := parser.ReviewDedupPairs(ctx, inputs)
//   if err != nil { return err }
//   return store.StoreDedupLLMVerdicts(verdicts)

// AFTER:
deps := aijobs.Deps{Store: sqliteStore, Client: &ai.AIJobsBatchClient{Parser: parser}}
_, err := ai.SubmitDedupReviewJob(ctx, deps, model, inputs)
if err != nil {
	return fmt.Errorf("submit dedup review: %w", err)
}
return nil // results apply asynchronously via aijobs dispatcher
```

If the existing caller log message says "LLM review complete" or similar, change it to "LLM review job submitted" to reflect the async semantics.

- [ ] **Step 5: Wire the sink at server startup**

In `internal/server/server.go`, wherever the `batchPoller` is created and `registerBatchPollerHandlers` is called, also call:

```go
ai.SetDedupVerdictSink(database.GetGlobalStore()) // type assert if needed
```

The store must implement `ai.DedupVerdictSink`. If `StoreDedupLLMVerdicts` doesn't exist on the store yet, add it as part of this task (trivial wrapper over whatever the dedup engine currently uses to persist verdicts — inspect the old caller to see).

- [ ] **Step 6: Strike `reviewDedupBatch` from allow-list**

Edit `internal/ai/priority_marker_test.go`:

```go
	// "reviewDedupBatch":      true, // MIGRATED in Task 1.5 — now goes through aijobs
```

Delete (or comment out) that line.

- [ ] **Step 7: Run all relevant tests**

```bash
go build ./...
go test ./internal/ai/... ./internal/database/... ./internal/dedup/... ./internal/server/... -count=1 -short
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git commit -am "$(cat <<'EOF'
refactor(ai/dedup_review): migrate to aijobs batch API

Replaces synchronous ReviewDedupPairs / reviewDedupBatch with
SubmitDedupReviewJob (enqueues an aijobs batch) + a registered completion
callback that persists verdicts via a sink injected at server startup.

The maintenance task that hit 429 insufficient_quota on the synchronous
endpoint now submits a Batch API job and returns immediately; results
apply asynchronously when the batch completes (ETA up to 24h).

Removes reviewDedupBatch from the priority-marker allow-list.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Phase 2 — Parallel Migrations (3 Haiku agents)

**For each of Tasks 2.1/2.2/2.3:** treat Task 1.5 (`dedup_review.go`) as the template. The pattern is:

1. Inspect the current sync call site; identify its input type, output type, and persistence target.
2. Extract the system prompt into a package-level const.
3. Add a new `Submit<Feature>Job(ctx, deps, ...)` function that builds per-item (or per-row if the feature already chunks) `BatchRequest`s and calls `aijobs.Submit`.
4. Add a registered callback in `init()` that parses each row's response, persists via a sink interface, and returns `(success, errorCount, rowErrors, fatalErr)`.
5. Wire the sink at server startup in `internal/server/server.go`.
6. Update every caller that used the old sync function. If a caller is genuinely interactive (user waiting on an HTTP request), **do not delete the sync path** — rename it `<Feature>Interactive`, mark it with `// PRIORITY: Interactive`, and keep it.
7. Strike the old function name from `priority_marker_test.go`'s allow-list.
8. Add tests mirroring `dedup_review_aijobs_test.go`.
9. Run `go build ./...` + targeted tests, then commit.

## Task 2.1 — DO NOT MIGRATE `metadata_llm_review.go` (Interactive call site)

**Status: COMPLETED — misclassification corrected.**

`ScoreMetadataCandidates` was initially flagged for migration but is **NOT** a bulk operation. Its production caller is `internal/metafetch/service.go:1463` (`mfs.llmScorer.Score(ctx, query, llmCands)`), which runs during **user-initiated interactive metadata search**. The UI awaits the response; the caller is a synchronous Gin handler waiting on the user. Batch API latency (minutes to hours) makes this path unusable.

**Resolution:**
- Reverted functional changes from commit `b8946847` to restore synchronous `scoreMetadataBatch` behavior.
- Added `// PRIORITY: Interactive` marker on `scoreMetadataBatch` declaration.
- Removed aijobs wiring (`SetAIJobsStore` field, `SetAIJobsStore()` method, server.go call, callback, payload type, cache, polling loop).
- Deleted `internal/ai/metadata_llm_review_aijobs_test.go`.
- Re-added `"scoreMetadataBatch"` to priority-marker test allow-list with comment: "PRIORITY: Interactive — user-waiting metadata search, stays sync".
- Updated design doc migration mapping: row for `metadata_llm_review.go:147` changed from "Bulk | Migrate" to "Interactive | Mark + keep sync" with justification.

No further action needed for this task.

---

## Task 2.2 — Migrate `openai_parser.go` sites `ParsePath`, `ParseSeries`, `ParseMetadata`

**Files:**
- Modify: `internal/ai/openai_parser.go`
- Create: `internal/ai/openai_parser_aijobs_test.go`
- Modify: `internal/ai/priority_marker_test.go`
- Modify: scan/pipeline callers + `internal/server/server.go` sink wiring

These three functions share enough shape to be migrated together but they are distinct features and must each have their own `aijobs.Register` type string and sink.

- [ ] **Step 1: Identify callers for each function**

```bash
grep -rn "ParsePath\b\|ParseSeries\b\|ParseMetadata\b" --include="*.go" internal/ cmd/ main.go
```

Confirm every caller is in a bulk/scan/maintenance path (no Gin handler). If any caller is interactive (HTTP request with user waiting), convert that function to a split: `ParsePathInteractive` (sync, marked) + `SubmitParsePathJob` (batch). The design deems ParsePath/Series/Metadata bulk-only; validate.

- [ ] **Step 2: For each function, do the dedup_review dance**

Three mirror migrations. For each:
- Extract system prompt to a const.
- Add `Submit<Name>Job(ctx, deps, items) (jobID, error)`.
- Register `"parse_path"`, `"parse_series"`, `"parse_metadata"` in `init()`.
- Add a sink interface + setter for each (or one unified `ScanResultsSink` if all three persist to the same target — inspect the old call sites to decide; unified is preferred if the sink methods are cohesive).
- Write callbacks that match the existing sync function's parse+persist behavior.

- [ ] **Step 3: Tests + strike allow-list entries + commit**

Write at least one happy-path + one per-row-error test for each function.

```bash
go build ./...
go test ./internal/ai/... ./internal/server/... ./internal/scanner/... -count=1 -short
git commit -am "$(cat <<'EOF'
refactor(ai/openai_parser): migrate ParsePath/Series/Metadata to aijobs

Three scan-pipeline sync sites now submit aijobs batches. Scan loops no
longer block on the synchronous chat-completions endpoint; results apply
asynchronously via registered callbacks.

Strikes ParsePath, ParseSeries, ParseMetadata from the priority-marker
allow-list.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.3 — Migrate `openai_parser.go:391` (cover image) + sites 544/678

**Files:** same as 2.2.

### 2.3a — `ParseCoverImage` (split)

The design says `ParseCoverImage` has both scan-pipeline (bulk) and single-book-UI (interactive) callers, so it must be split.

- [ ] **Step 1: Confirm the split is needed**

```bash
grep -rn "ParseCoverImage\b" --include="*.go" internal/ cmd/ main.go
```

For each caller, classify Interactive or Bulk by context (Gin handler → Interactive; scan loop → Bulk). If the split assumption from the design is wrong (all callers are Bulk), skip the split — migrate as Bulk only.

- [ ] **Step 2: Create `ParseCoverImageInteractive` (sync) + `SubmitParseCoverImageJob` (batch)**

Keep the current synchronous body verbatim inside `ParseCoverImageInteractive`, prefix the declaration with `// PRIORITY: Interactive`:

```go
// PRIORITY: Interactive
// ParseCoverImageInteractive performs a synchronous cover-image parse for
// single-book UI flows where the user is waiting. Bulk callers must use
// SubmitParseCoverImageJob instead.
func (p *OpenAIParser) ParseCoverImageInteractive(ctx context.Context, ...) (...) {
    // body unchanged
}
```

Then add `SubmitParseCoverImageJob` mirroring the dedup pattern.

- [ ] **Step 3: Update callers by classification**

Gin handlers → call `ParseCoverImageInteractive`. Scan loops → call `SubmitParseCoverImageJob`.

- [ ] **Step 4: Adjust allow-list**

Replace the `ParseCoverImage` entry with `ParseCoverImageInteractive`. The sync caller test should accept the new name because the marker comment is in place.

### 2.3b — Sites 544 & 678 — OUT OF SCOPE

Per the design's Non-Goals and Phase 1.4 audit clarification:
- `openai_parser.go:544` (`reviewAuthorBatch`) — sync fallback for existing `author_dedup` batch flow (not migrated)
- `openai_parser.go:678` (`discoverAuthorBatch`) — sync fallback for existing `author_review` batch flow (not migrated)

Both functions remain allow-listed in the enforcement test. **No code changes needed for these sites.** Existing batch infrastructure (`CreateBatchAuthorDedup`/`CreateBatchAuthorReview`) handles the bulk work; these sync fallbacks stay as-is.

- [ ] **Step 5: Tests, build, commit**

```bash
go build ./...
go test ./internal/ai/... ./internal/server/... -count=1 -short
git commit -am "$(cat <<'EOF'
refactor(ai/openai_parser): split ParseCoverImage (sites 544/678 out-of-scope)

ParseCoverImage split into ParseCoverImageInteractive (kept sync for UI
single-book flows, marked PRIORITY: Interactive) and SubmitParseCoverImageJob
(bulk aijobs batch).

Sites 544 (reviewAuthorBatch) and 678 (discoverAuthorBatch) identified in
Phase 1.4 audit as sync fallbacks for existing author_dedup/author_review
batch flows, which are explicitly out-of-scope per design non-goals.
These functions remain allow-listed and unchanged.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Phase 3 — Observability & Cleanup (serial)

## Task 3.1 — `GET /api/v1/ai-jobs` endpoint

**Files:**
- Create: `internal/server/ai_jobs_handlers.go`
- Create: `internal/server/ai_jobs_handlers_test.go`
- Modify: wherever routes are registered (search `grep -n "ai-scan\|scheduler/tasks" internal/server/*.go` for a sibling route to insert near)

- [ ] **Step 1: Write test first**

```go
// file: internal/server/ai_jobs_handlers_test.go
// version: 1.0.0
// guid: <generate with uuidgen>

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListAIJobsHandler_ReturnsRowsFiltered(t *testing.T) {
	store := newAIJobsTestStore(t)
	t.Cleanup(func() { store.Close() })
	require.NoError(t, store.CreateAIJob(database.AIJob{
		ID: "j1", Type: "dedup_review", CustomIDPrefix: "x", Status: "completed",
		ItemCount: 1, CreatedAt: time.Now(),
	}, []byte("[]")))
	require.NoError(t, store.CreateAIJob(database.AIJob{
		ID: "j2", Type: "metadata_review", CustomIDPrefix: "x", Status: "submitted",
		ItemCount: 1, CreatedAt: time.Now(),
	}, []byte("[]")))

	gin.SetMode(gin.TestMode)
	s := &Server{store: store}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai-jobs?type=dedup_review", nil)
	w := httptest.NewRecorder()
	r := gin.New()
	r.GET("/api/v1/ai-jobs", s.handleListAIJobs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Jobs []database.AIJob `json:"jobs"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Jobs, 1)
	assert.Equal(t, "j1", resp.Jobs[0].ID)
}

// newAIJobsTestStore returns an in-memory SQLite store with migrations applied.
// (Mirror whatever this repo's in-package helper is — check existing server tests for the canonical form.)
func newAIJobsTestStore(t *testing.T) *database.SQLiteStore {
	t.Helper()
	store, err := database.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, database.RunMigrations(store))
	return store
}
```

- [ ] **Step 2: Run — expect fail**

Run: `go test ./internal/server/ -run TestListAIJobsHandler -v`
Expected: FAIL (handler not defined).

- [ ] **Step 3: Implement the handler**

```go
// file: internal/server/ai_jobs_handlers.go
// version: 1.0.0
// guid: <generate with uuidgen>

package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// handleListAIJobs serves GET /api/v1/ai-jobs with optional type/status filters.
// Query params: type, status, limit (default 100, max 500), offset.
func (s *Server) handleListAIJobs(c *gin.Context) {
	typeF := c.Query("type")
	statusF := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}

	store, ok := s.Store().(database.AIJobsStore)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "store does not implement AIJobsStore"})
		return
	}
	jobs, err := store.ListAIJobs(typeF, statusF, limit, offset)
	if err != nil {
		internalError(c, "list ai_jobs", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": jobs})
}
```

Register the route: find the file in `internal/server/` that maps other `/api/v1/...` routes (run `grep -rn "GET.*api/v1/scheduler\|GET.*api/v1/operations" internal/server/`). Add next to a similar admin/observability endpoint:

```go
r.GET("/api/v1/ai-jobs", s.handleListAIJobs)
```

- [ ] **Step 4: Run — pass**

```bash
go test ./internal/server/ -run TestListAIJobsHandler -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit -am "$(cat <<'EOF'
feat(api): GET /api/v1/ai-jobs for aijobs observability

Lists ai_jobs rows with optional type/status filters. Backs the
Diagnostics → AI Jobs panel in the next task.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3.2 — Diagnostics "AI Jobs" panel (frontend)

**Files:**
- Create: `web/src/pages/Diagnostics/AIJobsPanel.tsx`
- Modify: the Diagnostics page component to render the new panel (find via `grep -rn "DiagnosticsPage\|AIBatchAnalysis" web/src/`)

- [ ] **Step 1: Write the panel**

```tsx
// file: web/src/pages/Diagnostics/AIJobsPanel.tsx
// version: 1.0.0
// guid: <generate with uuidgen>

import { useEffect, useState } from "react";
import { Box, Chip, Table, TableBody, TableCell, TableHead, TableRow, Typography } from "@mui/material";

interface AIJob {
  id: string;
  type: string;
  batch_id?: string;
  status: string;
  item_count: number;
  success_count: number;
  error_count: number;
  row_errors?: string;
  error_msg?: string;
  submitted_at?: string;
  completed_at?: string;
  created_at: string;
}

const statusColor = (s: string): "default" | "primary" | "success" | "warning" | "error" => {
  switch (s) {
    case "pending":
    case "submitted":
      return "primary";
    case "completed":
      return "success";
    case "completed_with_errors":
      return "warning";
    case "failed":
    case "expired":
      return "error";
    default:
      return "default";
  }
};

export function AIJobsPanel() {
  const [jobs, setJobs] = useState<AIJob[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const r = await fetch("/api/v1/ai-jobs?limit=50");
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data = await r.json();
        if (!cancelled) setJobs(data.jobs ?? []);
      } catch (e: unknown) {
        if (!cancelled) setError(String(e));
      }
    };
    load();
    const iv = setInterval(load, 15000);
    return () => {
      cancelled = true;
      clearInterval(iv);
    };
  }, []);

  const inFlight = jobs.filter((j) => j.status === "pending" || j.status === "submitted").length;

  return (
    <Box sx={{ mt: 3 }}>
      <Typography variant="h6">AI Jobs</Typography>
      <Typography variant="body2" color="text.secondary" gutterBottom>
        {inFlight} in flight · {jobs.length} recent
      </Typography>
      {error && <Typography color="error">{error}</Typography>}
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Type</TableCell>
            <TableCell>Status</TableCell>
            <TableCell align="right">Items</TableCell>
            <TableCell align="right">OK</TableCell>
            <TableCell align="right">Err</TableCell>
            <TableCell>Submitted</TableCell>
            <TableCell>Completed</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {jobs.map((j) => (
            <TableRow key={j.id}>
              <TableCell>{j.type}</TableCell>
              <TableCell>
                <Chip size="small" color={statusColor(j.status)} label={j.status} />
              </TableCell>
              <TableCell align="right">{j.item_count}</TableCell>
              <TableCell align="right">{j.success_count}</TableCell>
              <TableCell align="right">{j.error_count}</TableCell>
              <TableCell>{j.submitted_at ?? "—"}</TableCell>
              <TableCell>{j.completed_at ?? "—"}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Box>
  );
}
```

- [ ] **Step 2: Render it inside the Diagnostics page**

Find the diagnostics page (`grep -rn "Diagnostics" web/src/pages`). Import and render `<AIJobsPanel />` next to the existing ZIP export / AI batch analysis sections.

- [ ] **Step 3: Run frontend tests + lint**

```bash
cd web && npm test -- AIJobsPanel
npx tsc --noEmit
```

- [ ] **Step 4: Commit**

```bash
git commit -am "$(cat <<'EOF'
feat(web): Diagnostics → AI Jobs panel

Polls /api/v1/ai-jobs every 15s; shows in-flight count and last 50 jobs
with per-job status, item/success/error counts. Supplies the operator-facing
surface the aijobs migration needs for monitoring quota exhaustion.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3.3 — Audit remaining direct `client.Batches.New` callers

**Files:**
- Inspect: `internal/server/ai_scan_pipeline.go`, `internal/server/bench.go`, anywhere else grep finds direct batch calls.

- [ ] **Step 1: Grep**

```bash
grep -rn "client\.Batches\.New\|Batches\.List\|CreateBatchWithMetadata\|UploadBatchFile" --include="*.go" internal/ cmd/ | grep -v aijobs | grep -v _test.go
```

- [ ] **Step 2: For each non-`aijobs` caller, decide:**

- **If it's the existing `author_dedup` / `author_review` / `diagnostics` / `pipeline` flow** — leave it; these already use the Batch API and were explicitly out of scope per the design.
- **If it's `bench.go`** — benchmarking tool; safe to leave raw (it's exercising the API directly on purpose). Add a one-line comment: `// NOTE: bench.go calls the batch API directly to measure raw performance; not routed through aijobs.`
- **If it's anything else** — migrate to `aijobs.Submit` or document why not.

- [ ] **Step 3: Commit any changes**

```bash
git commit -am "$(cat <<'EOF'
chore(server): audit remaining direct Batches.New callers

Confirms author_dedup, author_review, diagnostics, and pipeline flows stay
on their purpose-built batch paths (out of aijobs scope per design).
bench.go stays raw with a clarifying comment.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(If there's nothing to change, skip the commit and move on.)

---

## Task 3.4 — End-to-end integration test

**Files:**
- Create: `internal/ai/aijobs/integration_test.go`

- [ ] **Step 1: Write the test**

```go
// file: internal/ai/aijobs/integration_test.go
// version: 1.0.0
// guid: <generate with uuidgen>

package aijobs_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/ai/aijobs"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SubmitDispatchRoundTrip simulates the full flow:
// Submit → (mock batch "completes") → Dispatch → callback applies results.
func TestIntegration_SubmitDispatchRoundTrip(t *testing.T) {
	store, err := database.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, database.RunMigrations(store))

	aijobs.ClearRegistryForTest()
	applied := 0
	aijobs.Register("int_test", func(ctx context.Context, itemsJSON []byte, results []aijobs.RowResult) (int, int, []database.AIJobRowError, error) {
		for range results {
			applied++
		}
		return len(results), 0, nil, nil
	})

	client := &fakeClient{returnBatchID: "batch_int"}
	deps := aijobs.Deps{Store: store, Client: client}

	items := []map[string]any{{"n": 1}, {"n": 2}}
	payloadJSON, _ := json.Marshal(items)

	jobID, err := aijobs.Submit(context.Background(), deps, aijobs.SubmitRequest{
		Type:        "int_test",
		ItemCount:   len(items),
		PayloadJSON: payloadJSON,
		Build: func(i int) (aijobs.BatchRequest, error) {
			return aijobs.BatchRequest{Body: map[string]any{"i": i}}, nil
		},
	})
	require.NoError(t, err)

	// Simulate the BatchPoller calling Dispatch on completion.
	results := []aijobs.RowResult{
		{CustomID: jobID + "-0", Body: map[string]any{"ok": true}},
		{CustomID: jobID + "-1", Body: map[string]any{"ok": true}},
	}
	require.NoError(t, aijobs.Dispatch(context.Background(), store, "batch_int", results))

	assert.Equal(t, 2, applied)
	j, err := store.GetAIJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, "completed", j.Status)
	assert.Equal(t, 2, j.SuccessCount)
}

type fakeClient struct{ returnBatchID string }

func (f *fakeClient) UploadBatchFile(context.Context, []byte) (string, error) { return "file", nil }
func (f *fakeClient) CreateBatchWithMetadata(context.Context, string, string) (string, error) {
	return f.returnBatchID, nil
}
```

- [ ] **Step 2: Run + commit**

```bash
go test ./internal/ai/aijobs/... -run TestIntegration -v
git commit -am "$(cat <<'EOF'
test(aijobs): end-to-end submit → dispatch integration test

Exercises the full round-trip against an in-memory SQLite store with
migration 52 applied: Submit persists the ai_jobs row and payload, a
simulated batch completion hands RowResults to Dispatch, and the
registered callback applies them. Verifies terminal state is "completed"
and success_count matches.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Final Validation

After Phase 3 completes, run the full check:

- [ ] **All backend tests**

```bash
go build ./...
go test ./... -count=1 -short
```

- [ ] **Priority marker enforcement**

```bash
go test ./internal/ai/ -run TestNoUnmarkedChatCompletionCallers -v
```
Expected: PASS — the allow-list should now contain only genuinely Interactive entries (e.g., `ParseCoverImageInteractive` and any Interactive functions spun out during Phase 2.3b).

- [ ] **No bulk sync callers left**

```bash
grep -rn "Chat\.Completions\.New" --include="*.go" internal/ai/ | grep -v _test.go
```
For each hit, verify the enclosing function either (a) is named `*Interactive` + has the `// PRIORITY: Interactive` marker, or (b) appears in the test's allow-list for a documented reason.

- [ ] **Frontend**

```bash
cd web && npm test && npx tsc --noEmit
```

- [ ] **E2E (optional sanity)**

```bash
make test-e2e
```

- [ ] **Open PR**

```bash
git push -u origin feat/aijobs-batch-migration
gh pr create --title "feat: route bulk LLM work through OpenAI Batch API (aijobs)" --body "$(cat <<'EOF'
## Summary
- New `internal/ai/aijobs` package routes all bulk-scale LLM chat-completion work through the OpenAI Batch API, eliminating 429 `insufficient_quota` failures on maintenance runs.
- Migration 52 adds `ai_jobs` + `ai_job_payloads` tables for unified tracking.
- Caller-intent contract: every `Chat.Completions.New` call site is either migrated to `aijobs.Submit` (Bulk) or marked `// PRIORITY: Interactive` with the enforcement test (`TestNoUnmarkedChatCompletionCallers`) gating CI.
- `GET /api/v1/ai-jobs` + Diagnostics panel for operator visibility.

## Test plan
- [ ] `go test ./... -short` passes
- [ ] `TestNoUnmarkedChatCompletionCallers` passes with minimal allow-list
- [ ] Dedup maintenance task (`dedup_llm_review`) submits a batch instead of 429-ing
- [ ] Diagnostics → AI Jobs panel renders in-flight and recent jobs
- [ ] Interactive cover-image parse still responds synchronously from the UI

Design: `docs/superpowers/specs/2026-04-24-aijobs-batch-migration-design.md`
EOF
)"
```

---

## Self-Review Notes

**Spec coverage:** Every section of the design doc (architecture, caller intent contract, migration mapping, error handling, observability, testing strategy, work decomposition) maps to at least one task.

**Placeholder scan:** The only intentional placeholders are:
- `<generate with uuidgen>` — GUID values (not resolvable statically; the executing agent generates them).
- `parseOpenAIParser_544`, `parseOpenAIParser_678` in the allow-list — explicitly called out as placeholders that Phase 1.4 replaces with real function names from the audit.

**Type consistency:** `AIJob`, `AIJobsStore`, `aijobs.Deps`, `aijobs.SubmitRequest`, `aijobs.BatchRequest`, `aijobs.RowResult`, `aijobs.CompletionCallback` are defined once in Tasks 1.1/1.2 and used with the same names in every later task.

**Scope:** One implementation plan. Phase 2 is parallelizable; Phase 3 is strictly after Phase 2. No sub-project decomposition needed.
