// file: internal/operations/registry/registry.go
// version: 2.2.0
// guid: f6a7b8c9-d0e1-2f3a-4b5c-6d7e8f9a0b1c
// last-edited: 2026-05-06

package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/oklog/ulid/v2"
)

// Registry is the central in-memory and DB-backed object that owns every
// OperationDef, dispatches runs, enforces policies, and routes events.
type Registry struct {
	mu              sync.RWMutex
	defs            map[string]OperationDef
	running         map[string]*runHandle // opID → handle
	pluginRunning   map[string]int        // plugin → count of running ops
	pluginMax       map[string]int        // plugin → max_concurrent (0 = unlimited)
	concurrencyKeys map[string]string     // key → opID of holder
	nextRun         chan *queuedRun
	dispatch        chan struct{}
	store           database.OpsV2Store
	bus             Bus // may be nil; wired in UOS-06
	logger          *slog.Logger
	workers         int
	abandoned       *abandonedTracker

	// Tunable intervals for testing. Zero means use defaults.
	watchdogInterval time.Duration
}

// Options contains optional tunable parameters for a Registry. Zero values
// use sensible defaults. Primarily used in tests to shorten intervals.
type Options struct {
	// WatchdogInterval overrides the 30-second watchdog ticker. Zero = default.
	WatchdogInterval time.Duration
	// AbandonedCap overrides the per-plugin abandoned goroutine cap (default 4).
	AbandonedCap int
	// Bus is the SSE event bus (UOS-06). Nil is safe.
	Bus Bus
}

// New creates a new Registry. workers controls the in-process worker pool size.
// store must implement database.OpsV2Store; the database.Store composite
// interface satisfies this automatically.
// bus may be nil; it will be wired to the real EventHub in UOS-06.
func New(store database.OpsV2Store, logger *slog.Logger, workers int, bus Bus) *Registry {
	return NewWithOptions(store, logger, workers, Options{Bus: bus})
}

// NewWithOptions is like New but accepts optional tunable parameters.
func NewWithOptions(store database.OpsV2Store, logger *slog.Logger, workers int, opts Options) *Registry {
	if workers <= 0 {
		workers = 8
	}
	return &Registry{
		defs:             make(map[string]OperationDef),
		running:          make(map[string]*runHandle),
		pluginRunning:    make(map[string]int),
		pluginMax:        make(map[string]int),
		concurrencyKeys:  make(map[string]string),
		nextRun:          make(chan *queuedRun, workers*2),
		dispatch:         make(chan struct{}, 1),
		store:            store,
		bus:              opts.Bus,
		logger:           logger,
		workers:          workers,
		abandoned:        newAbandonedTracker(opts.AbandonedCap),
		watchdogInterval: opts.WatchdogInterval,
	}
}

// SetBus wires an EventHub to the registry so that operation lifecycle
// events (op.created, op.updated, op.log, op.terminal) are published
// as SSE events. Must be called BEFORE Start(). Safe to call with nil.
func (r *Registry) SetBus(bus Bus) {
	r.mu.Lock()
	r.bus = bus
	r.mu.Unlock()
}

// SetPluginMaxConcurrent configures the per-plugin concurrency cap.
// A value of 0 (the default) means unlimited.
func (r *Registry) SetPluginMaxConcurrent(plugin string, max int) {
	r.mu.Lock()
	r.pluginMax[plugin] = max
	r.mu.Unlock()
}

// Start launches the dispatcher and worker goroutines. Call once at startup.
// resumeAfterStartup is called first (synchronously in a goroutine context)
// to re-queue or drop ops that were in-flight at the last shutdown.
func (r *Registry) Start(ctx context.Context) {
	r.logger.Info("registry: starting", "workers", r.workers)
	// Resume must complete before the dispatcher starts accepting new work.
	r.resumeAfterStartup(ctx)
	go r.runDispatcher(ctx)
	go r.runWatchdog(ctx)
	for i := range r.workers {
		go r.startWorker(ctx, i)
	}
}

// RegisterOp validates and registers an OperationDef.
// Returns an error if:
//   - def.ID is empty
//   - def.Run is nil
//   - def.ResumePolicy == ResumeUnspecified
//   - def.ID is already registered
func (r *Registry) RegisterOp(def OperationDef) error {
	if def.ID == "" {
		return errors.New("registry: OperationDef.ID must not be empty")
	}
	if def.Run == nil {
		return fmt.Errorf("registry: OperationDef.Run must not be nil (id=%s)", def.ID)
	}
	if def.ResumePolicy == ResumeUnspecified {
		return fmt.Errorf("registry: OperationDef.ResumePolicy must not be ResumeUnspecified (id=%s)", def.ID)
	}

	r.mu.Lock()
	if _, exists := r.defs[def.ID]; exists {
		r.mu.Unlock()
		return fmt.Errorf("registry: OperationDef already registered (id=%s)", def.ID)
	}
	r.defs[def.ID] = def
	r.mu.Unlock()

	// Persist to op_definitions_v2. Best-effort; log on error.
	if err := r.upsertDefToDB(def); err != nil {
		r.logger.Warn("registry: failed to upsert op_definitions_v2", "id", def.ID, "error", err)
	}

	r.logger.Info("registry: registered op", "id", def.ID, "plugin", def.Plugin)
	return nil
}

// upsertDefToDB writes the def to op_definitions_v2.
func (r *Registry) upsertDefToDB(def OperationDef) error {
	capsJSON, _ := json.Marshal(def.Capabilities)
	permsJSON, _ := json.Marshal(def.Permissions)
	triggersJSON, _ := json.Marshal(triggersToNames(def.Triggers))
	dependsJSON, _ := json.Marshal(def.DependsOn)
	phasesJSON, _ := json.Marshal(phaseNames(def.Phases))

	var schedCron *string
	if def.Schedule != nil {
		schedCron = def.Schedule
	}

	timeoutSecs := int(def.Timeout.Seconds())

	return r.store.UpsertOpDefinitionV2(database.OpDefinitionV2Row{
		ID:             def.ID,
		Plugin:         def.Plugin,
		DisplayName:    def.DisplayName,
		Description:    def.Description,
		Capabilities:   string(capsJSON),
		Permissions:    string(permsJSON),
		Cancellable:    def.Cancellable,
		Isolate:        def.Isolate,
		ResumePolicy:   resumePolicyName(def.ResumePolicy),
		ScheduleCron:   schedCron,
		Triggers:       string(triggersJSON),
		DependsOn:      string(dependsJSON),
		Phases:         string(phasesJSON),
		TimeoutSeconds: timeoutSecs,
		RegisteredAt:   time.Now().UTC(),
	})
}

// EnqueueOp creates a new queued run for the given def. Returns the ULID of
// the new run.
func (r *Registry) EnqueueOp(ctx context.Context, defID string, params any, opts ...EnqueueOption) (string, error) {
	r.mu.RLock()
	def, ok := r.defs[defID]
	r.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("registry: unknown defID %q", defID)
	}

	// Marshal params.
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return "", fmt.Errorf("registry: marshal params: %w", err)
		}
		rawParams = b
	} else {
		rawParams = json.RawMessage("{}")
	}

	// Apply options.
	eopts := &EnqueueOptions{}
	for _, opt := range opts {
		opt(eopts)
	}

	// Priority: option overrides def default.
	priority := def.DefaultPriority
	if eopts.Priority != nil {
		priority = *eopts.Priority
	}

	// Generate ULID.
	opID := ulid.Make().String()

	now := time.Now().UTC()

	var parentID *string
	if eopts.ParentID != "" {
		parentID = &eopts.ParentID
	}
	var actorUserID *string
	if eopts.ActorUserID != "" {
		actorUserID = &eopts.ActorUserID
	}
	var parentSpanID *string
	if eopts.ParentSpanID != "" {
		parentSpanID = &eopts.ParentSpanID
	}
	traceID := eopts.TraceID
	if traceID == "" {
		traceID = ulid.Make().String()
	}
	spanID := eopts.SpanID
	if spanID == "" {
		spanID = ulid.Make().String()
	}

	row := database.OperationV2Row{
		ID:           opID,
		DefID:        def.ID,
		Plugin:       def.Plugin,
		ParentID:     parentID,
		ActorUserID:  actorUserID,
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		Status:       "queued",
		Priority:     int(priority),
		Params:       string(rawParams),
		QueuedAt:     now,
	}

	if err := r.store.InsertOperationV2(row); err != nil {
		return "", fmt.Errorf("registry: insert operation_v2: %w", err)
	}

	r.logger.Info("registry: enqueued op", "op_id", opID, "def_id", defID, "priority", priority)

	// Signal the dispatcher.
	r.pingDispatch()

	return opID, nil
}

// Cancel cancels an operation by id.
// If the op is queued, it is marked canceled in the DB.
// If the op is running, its context is canceled.
func (r *Registry) Cancel(opID string) error {
	r.mu.Lock()
	h, running := r.running[opID]
	r.mu.Unlock()

	if running {
		r.logger.Info("registry: canceling running op", "op_id", opID)
		h.cancel()
		return nil
	}

	// Try to mark it canceled if it's still queued.
	updated, err := r.store.SetOperationV2StatusIfQueued(opID, "canceled")
	if err != nil {
		return fmt.Errorf("registry: cancel op %s: %w", opID, err)
	}
	if updated {
		r.logger.Info("registry: canceled queued op", "op_id", opID)
	}
	return nil
}

// AbandonedCount returns the current number of abandoned goroutines for a
// plugin. Used by tests and metrics; the dispatcher uses isBlocked internally.
func (r *Registry) AbandonedCount(plugin string) int {
	return r.abandoned.countFor(plugin)
}

// ActiveDefs returns all registered OperationDefs.
func (r *Registry) ActiveDefs() []OperationDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]OperationDef, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out
}

// Shutdown drains the worker pool. On timeout it marks remaining running ops
// as interrupted per their ResumePolicy and returns.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.logger.Info("registry: shutting down")

	// Gather running ops.
	r.mu.Lock()
	handles := make([]*runHandle, 0, len(r.running))
	for _, h := range r.running {
		handles = append(handles, h)
	}
	r.mu.Unlock()

	// Cancel all running ops.
	for _, h := range handles {
		h.cancel()
	}

	// Wait until context expires or all workers drain.
	done := make(chan struct{})
	go func() {
		// Poll until no running ops remain.
		for {
			r.mu.RLock()
			n := len(r.running)
			r.mu.RUnlock()
			if n == 0 {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
		close(done)
	}()

	select {
	case <-done:
		r.logger.Info("registry: all workers drained")
		return nil
	case <-ctx.Done():
		// Mark remaining as interrupted.
		r.mu.Lock()
		for opID, h := range r.running {
			h.abandoned = true
			status := interruptedStatus(h.resumePolicy)
			now := time.Now().UTC()
			_ = r.store.UpdateOperationV2Status(opID, status, nil, &now, nil)
		}
		r.mu.Unlock()
		r.logger.Warn("registry: shutdown timeout; marked remaining ops as interrupted")
		return ctx.Err()
	}
}

// writeStrike appends a strike record to op_strikes_v2 and logs it.
func (r *Registry) writeStrike(opID, defID, plugin, kind, message string) {
	details := fmt.Sprintf(`{"plugin":%q,"message":%q}`, plugin, message)
	row := database.OpStrikeV2Row{
		DefID:       defID,
		OperationID: opID,
		Kind:        kind,
		Details:     details,
		OccurredAt:  time.Now().UTC(),
	}
	if err := r.store.InsertOpStrikeV2(row); err != nil {
		r.logger.Warn("registry: failed to write strike", "op_id", opID, "kind", kind, "error", err)
	}
	r.logger.Warn("registry: strike recorded", "op_id", opID, "def_id", defID, "kind", kind, "message", message)
}

// pingDispatch sends a non-blocking signal to the dispatch channel.
func (r *Registry) pingDispatch() {
	select {
	case r.dispatch <- struct{}{}:
	default:
	}
}

// releaseRunHandle removes a handle from the running map and releases
// the concurrency key if held.
func (r *Registry) releaseRunHandle(opID string) {
	r.mu.Lock()
	h, ok := r.running[opID]
	if ok {
		delete(r.running, opID)
		if h.plugin != "" {
			r.pluginRunning[h.plugin]--
			if r.pluginRunning[h.plugin] < 0 {
				r.pluginRunning[h.plugin] = 0
			}
		}
		if h.concurrencyKey != "" {
			if holder, held := r.concurrencyKeys[h.concurrencyKey]; held && holder == opID {
				delete(r.concurrencyKeys, h.concurrencyKey)
			}
		}
	}
	r.mu.Unlock()
	r.pingDispatch()
}

// --- Helpers ---

func resumePolicyName(p ResumePolicy) string {
	switch p {
	case ResumeRestart:
		return "restart"
	case ResumeRequeue:
		return "requeue"
	case ResumeDrop:
		return "drop"
	case ResumeAsk:
		return "ask"
	default:
		return "unspecified"
	}
}

func interruptedStatus(p ResumePolicy) string {
	switch p {
	case ResumeDrop:
		return "interrupted_dropped"
	default:
		return "interrupted_quiesced"
	}
}

func triggersToNames(subs []EventSubscription) []string {
	names := make([]string, len(subs))
	for i, s := range subs {
		names[i] = s.EventName
	}
	return names
}

func phaseNames(phases []Phase) []string {
	names := make([]string, len(phases))
	for i, p := range phases {
		names[i] = p.Name
	}
	return names
}
