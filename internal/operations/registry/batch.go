// file: internal/operations/registry/batch.go
// version: 1.0.0
// guid: e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b
// last-edited: 2026-06-13

// batch.go implements M3: coalescing burst enqueues of a Batchable op type into
// one OperationV2Row via a debounce timer.
//
// Design overview
//
//	EnqueueOp (registry.go) calls batchManager.Add(defID, subject) for Batchable
//	ops instead of inserting a row directly. Add:
//	  1. Journals the subject via store.AddToBatchBucket (idempotent).
//	  2. Arms (or re-arms) the per-op-type debounce timer.
//
//	Timer fire (via time.AfterFunc):
//	  1. Checks shuttingDown — bails without touching the store if true.
//	  2. Checks the generation counter — bails if a newer Add has overtaken it.
//	  3. Snapshot-and-release: under batchMu, copies subjects + clears in-mem
//	     map + increments gen; then releases batchMu.
//	  4. With no lock held: evaluates AllSatisfied(def.Requires) per subject.
//	     Ready subjects are inserted as one batched OperationV2Row; unready
//	     subjects stay in the journal (ClearBatchBucket is called only for ready
//	     subjects).
//	  5. Pings the dispatcher.
//
// Concurrency
//
//	batchMu guards bucket + timer state.
//	r.mu is NOT held while batchMu is held (no nesting → no deadlock).
//	Timer fire releases batchMu before touching the store or r.mu.
//
// Shutdown
//
//	On Stop/cancelFn, all timers are stopped and buckets are left journaled.
//	The fire closure checks r.shuttingDown.Load() and bails before any DB call.
//	This matches the "pebble: closed" safety pattern already in the registry.
//
// Per-enqueue WithRequires on Batchable ops
//
//	The store can only journal a subject (OpSubject), not per-call requirements.
//	Requirement gating at dispatch therefore uses def.Requires only.
//	M4 consumers (e.g. dedup.check-book) must declare requirements on the
//	OperationDef, not via per-enqueue WithRequires options.

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/oklog/ulid/v2"
)

const (
	defaultBatchWindow  = 5 * time.Second
	defaultBatchMaxWait = 60 * time.Second
)

// batchedSubjectsParams is the params shape written into an OperationV2Row for
// a batched op. The op's Run function receives this and must iterate subjects.
type batchedSubjectsParams struct {
	Subjects []database.OpSubject `json:"subjects"`
}

// batchBucket tracks the in-memory state for a single op-type's pending window.
type batchBucket struct {
	// subjects is the current in-memory set of pending subjects. Mirrors the
	// journal; both are cleared together on dispatch.
	subjects map[string]database.OpSubject // key = "type:id"

	// firstArrival is when the first subject arrived in the current window.
	// Used to enforce BatchMaxWait.
	firstArrival time.Time

	// timer is the active debounce timer. May be nil if no window is open.
	timer *time.Timer

	// gen is incremented every time the timer is armed. The fire closure
	// captures its gen at arm time and bails if r.batchBuckets[opType].gen
	// has moved on — preventing stale fires from dispatching.
	gen uint64
}

// batchManager owns all per-op-type buckets and the batchMu mutex.
// It is embedded in the Registry.
type batchManager struct {
	mu      sync.Mutex
	buckets map[string]*batchBucket // opType → bucket
}

func newBatchManager() *batchManager {
	return &batchManager{
		buckets: make(map[string]*batchBucket),
	}
}

// Add adds subject to the in-memory bucket for opType, journals it, and
// (re-)arms the debounce timer. Called by EnqueueOp for Batchable defs.
//
// bw and bmw are BatchWindow and BatchMaxWait from the OperationDef (already
// defaulted to non-zero values by the caller).
func (r *Registry) batchAdd(opType string, sub database.OpSubject, bw, bmw time.Duration) {
	// Journal first (idempotent) — safe outside batchMu because the store is
	// independently thread-safe. If journaling fails we still proceed; worst
	// case a crash loses this subject (acceptable: the add API is fire-and-forget
	// for callers that pass ("", nil) on batchable ops).
	if err := r.store.AddToBatchBucket(opType, sub); err != nil {
		r.logger.Warn("batch: AddToBatchBucket failed", "op_type", opType,
			"subject_type", sub.Type, "subject_id", sub.ID, "error", err)
	}

	r.batch.mu.Lock()

	b, ok := r.batch.buckets[opType]
	if !ok {
		b = &batchBucket{
			subjects: make(map[string]database.OpSubject),
		}
		r.batch.buckets[opType] = b
	}

	key := sub.Type + ":" + sub.ID
	if _, exists := b.subjects[key]; !exists {
		b.subjects[key] = sub
	}

	now := time.Now()
	if b.firstArrival.IsZero() {
		b.firstArrival = now
	}

	// Compute when to fire: min(now+bw, firstArrival+bmw).
	windowDeadline := now.Add(bw)
	maxWaitDeadline := b.firstArrival.Add(bmw)
	fireAt := windowDeadline
	if maxWaitDeadline.Before(fireAt) {
		fireAt = maxWaitDeadline
	}
	delay := fireAt.Sub(now)
	if delay < 0 {
		delay = 0
	}

	// Stop the old timer (if any) to prevent double-fire. Drain its channel
	// only if it hadn't already fired to avoid a goroutine leak.
	if b.timer != nil {
		b.timer.Stop()
	}

	// Increment generation so previous fire closures that may have already
	// been scheduled (before Stop()) will self-abort.
	b.gen++
	capturedGen := b.gen

	b.timer = time.AfterFunc(delay, func() {
		r.batchFire(opType, capturedGen)
	})

	r.batch.mu.Unlock()
}

// batchFire is called by time.AfterFunc when the debounce window expires.
// It dispatches ready subjects and leaves unready ones in the journal.
func (r *Registry) batchFire(opType string, capturedGen uint64) {
	// Bail immediately if shutting down — we must not touch the store after
	// the DB is closed (mirrors the "pebble: closed" safety guard in worker.go).
	if r.shuttingDown.Load() {
		return
	}

	// --- Phase 1: snapshot under batchMu, then release ---
	r.batch.mu.Lock()

	b, ok := r.batch.buckets[opType]
	if !ok {
		r.batch.mu.Unlock()
		return
	}
	if b.gen != capturedGen {
		// A newer Add has overtaken this fire — the timer was reset.
		r.batch.mu.Unlock()
		return
	}

	// Snapshot subjects and reset the bucket.
	snapshot := make([]database.OpSubject, 0, len(b.subjects))
	for _, sub := range b.subjects {
		snapshot = append(snapshot, sub)
	}
	b.subjects = make(map[string]database.OpSubject)
	b.firstArrival = time.Time{} // reset for next window
	b.timer = nil

	r.batch.mu.Unlock()

	// --- Phase 2: per-subject requirement evaluation (no lock held) ---
	if len(snapshot) == 0 {
		return
	}

	r.mu.RLock()
	def, defOK := r.defs[opType]
	r.mu.RUnlock()
	if !defOK {
		r.logger.Warn("batch: fire: op def not found; dropping subjects",
			"op_type", opType, "count", len(snapshot))
		return
	}

	var readySubs []database.OpSubject
	var unreadySubs []database.OpSubject

	if len(def.Requires) == 0 {
		// No requirements — all subjects are ready.
		readySubs = snapshot
	} else {
		for _, sub := range snapshot {
			regSub := Subject{Type: sub.Type, ID: sub.ID}
			ok, reason, err := AllSatisfied(OpsV2DepAdapter{r.store}, def.Requires, regSub)
			if err != nil {
				r.logger.Warn("batch: fire: AllSatisfied error; keeping subject bucketed",
					"op_type", opType, "subject_type", sub.Type, "subject_id", sub.ID, "error", err)
				unreadySubs = append(unreadySubs, sub)
				continue
			}
			if ok {
				readySubs = append(readySubs, sub)
			} else {
				r.logger.Debug("batch: fire: subject not ready; staying bucketed",
					"op_type", opType, "subject_type", sub.Type, "subject_id", sub.ID, "reason", reason)
				unreadySubs = append(unreadySubs, sub)
			}
		}
	}

	// Re-bucket unready subjects (leave them in journal — they were never cleared).
	// Their in-memory state was lost in the snapshot, so we re-add them.
	// The journal entry already exists (idempotent), but we need the in-mem bucket.
	if len(unreadySubs) > 0 {
		bw, bmw := effectiveBatchWindows(def)
		for _, sub := range unreadySubs {
			r.batchAdd(opType, sub, bw, bmw)
		}
	}

	if len(readySubs) == 0 {
		r.logger.Info("batch: fire: no ready subjects; all re-bucketed",
			"op_type", opType, "unready", len(unreadySubs))
		return
	}

	// --- Phase 3: dispatch one OperationV2Row for all ready subjects ---
	if err := r.batchDispatch(def, readySubs); err != nil {
		r.logger.Warn("batch: fire: dispatch failed; subjects will be lost from journal",
			"op_type", opType, "ready", len(readySubs), "error", err)
		return
	}

	// Clear dispatched subjects from the journal.
	if err := r.store.ClearBatchBucket(opType, readySubs); err != nil {
		r.logger.Warn("batch: fire: ClearBatchBucket failed (journal may have stale entries)",
			"op_type", opType, "error", err)
	}

	r.logger.Info("batch: dispatched",
		"op_type", opType, "ready", len(readySubs), "re_bucketed", len(unreadySubs))
}

// batchDispatch inserts one OperationV2Row for the given ready subjects.
// The row's params are {"subjects":[{"type":...,"id":...},...]} — the op's Run
// function is expected to iterate this list.
func (r *Registry) batchDispatch(def OperationDef, subs []database.OpSubject) error {
	params := batchedSubjectsParams{Subjects: subs}
	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("batchDispatch: marshal params: %w", err)
	}

	opID := ulid.Make().String()
	now := time.Now().UTC()
	traceID := ulid.Make().String()
	spanID := ulid.Make().String()

	row := database.OperationV2Row{
		ID:       opID,
		DefID:    def.ID,
		Plugin:   def.Plugin,
		TraceID:  traceID,
		SpanID:   spanID,
		Status:   "queued",
		Priority: int(def.DefaultPriority),
		Params:   string(rawParams),
		QueuedAt: now,
	}

	if err := r.store.InsertOperationV2(row); err != nil {
		return fmt.Errorf("batchDispatch: InsertOperationV2: %w", err)
	}

	r.logger.Info("batch: inserted batched op",
		"op_id", opID, "def_id", def.ID, "subject_count", len(subs))
	r.publishOpCreated(row, false)
	r.pingDispatch()
	return nil
}

// batchReloadOnStart is called during Start() to reload non-empty bucket journals
// and re-arm timers (or dispatch immediately if past max-wait).
// Must be called after the internal context is created but before goroutines start
// processing — called synchronously in Start().
func (r *Registry) batchReloadOnStart(ctx context.Context) {
	r.mu.RLock()
	defs := make([]OperationDef, 0, len(r.defs))
	for _, d := range r.defs {
		if d.Batchable {
			defs = append(defs, d)
		}
	}
	r.mu.RUnlock()

	for _, def := range defs {
		entries, err := r.store.ListBatchBucket(def.ID)
		if err != nil {
			r.logger.Warn("batch: reload: ListBatchBucket failed",
				"op_type", def.ID, "error", err)
			continue
		}
		if len(entries) == 0 {
			continue
		}

		bw, bmw := effectiveBatchWindows(def)
		now := time.Now()

		// Find the earliest AddedAt across all entries to determine firstArrival.
		var earliestNano int64
		for _, e := range entries {
			if earliestNano == 0 || e.AddedAt < earliestNano {
				earliestNano = e.AddedAt
			}
		}
		firstArrival := time.Unix(0, earliestNano)

		// If we're already past max-wait, fire immediately (delay=0).
		maxWaitDeadline := firstArrival.Add(bmw)
		var delay time.Duration
		if now.After(maxWaitDeadline) {
			delay = 0
		} else {
			// Use a fresh window from now, capped by max-wait.
			windowDeadline := now.Add(bw)
			if maxWaitDeadline.Before(windowDeadline) {
				windowDeadline = maxWaitDeadline
			}
			delay = windowDeadline.Sub(now)
			if delay < 0 {
				delay = 0
			}
		}

		r.batch.mu.Lock()
		b, ok := r.batch.buckets[def.ID]
		if !ok {
			b = &batchBucket{subjects: make(map[string]database.OpSubject)}
			r.batch.buckets[def.ID] = b
		}
		for _, e := range entries {
			key := e.Sub.Type + ":" + e.Sub.ID
			b.subjects[key] = e.Sub
		}
		b.firstArrival = firstArrival
		b.gen++
		capturedGen := b.gen
		b.timer = time.AfterFunc(delay, func() {
			r.batchFire(def.ID, capturedGen)
		})
		r.batch.mu.Unlock()

		r.logger.Info("batch: reload: armed bucket",
			"op_type", def.ID, "subjects", len(entries), "delay_ms", delay.Milliseconds())
	}
}

// batchStopAllTimers cancels all pending debounce timers without dispatching.
// Called by Shutdown() to avoid timer goroutines firing after the DB is closed.
// Subjects remain in the journal for the next Start() to reload.
func (r *Registry) batchStopAllTimers() {
	r.batch.mu.Lock()
	defer r.batch.mu.Unlock()
	for _, b := range r.batch.buckets {
		if b.timer != nil {
			b.timer.Stop()
			b.timer = nil
		}
	}
}

// effectiveBatchWindows returns the effective BatchWindow and BatchMaxWait for
// def, applying defaults when the def fields are zero.
func effectiveBatchWindows(def OperationDef) (bw, bmw time.Duration) {
	bw = def.BatchWindow
	if bw <= 0 {
		bw = defaultBatchWindow
	}
	bmw = def.BatchMaxWait
	if bmw <= 0 {
		bmw = defaultBatchMaxWait
	}
	return bw, bmw
}
