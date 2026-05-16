// file: internal/dedup/lifecycle.go
// version: 1.0.0

// Lifecycle methods on *dedup.Engine that the serviceregistry container
// picks up via interface satisfaction. PostInit subscribes to lifecycle
// events on the plugin event bus so the engine reacts to book imports
// from any source (filesystem watcher, iTunes importer, manual upload)
// without a server-bound closure callback.
//
// Replaces server.fireDedupOnImport, which was wired via a closure in
// itunesservice.Deps.OnBookCreated (removed in PLUGIN-DECOUPLE).

package dedup

import (
	"context"
	"log"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// PostInit wires the dedup engine into the rest of the container. Called
// by Container.PostInit after Build completes. Three steps:
//
//  1. Subscribe to plugin.EventBookImported via the eventbus so any source
//     (iTunes import, filesystem watcher, manual upload) triggers a dedup
//     check on the new book.
//  2. Pull chromem-go ANN store (optional, soft) and wire it via
//     SetChromemStore. Launch the chromem hydration goroutine on the
//     engine's bg-context.
//  3. Pull AIJobsStore (interface assertion against the main store) and
//     wire it for async dedup review batches. Register the engine as the
//     dedup verdict applier for batch callbacks.
//
// Safe to call when the engine is nil (Build returns nil when API key
// isn't configured — typed-nil receiver allowed by Go method dispatch on
// pointer types only when method has a nil-check, which we do).
func (de *Engine) PostInit(ctx context.Context, c *serviceregistry.Container) error {
	if de == nil {
		return nil
	}
	// Initialise the engine's own bg-context for subscriber goroutines.
	// The event bus's Publish runs each handler in its own goroutine with
	// the publisher's ctx; we ignore that ctx in favor of our own so a
	// canceled publisher (e.g. test-only request ctx) doesn't kill our
	// dedup check mid-flight.
	de.bgMu.Lock()
	if de.bgCtx == nil {
		de.bgCtx, de.bgCancel = context.WithCancel(context.Background())
	}
	bgCtx := de.bgCtx
	de.bgMu.Unlock()

	// Step 1 — event bus subscription
	if bus, ok := serviceregistry.TryGet[*plugin.EventBus](c, "eventbus"); ok && bus != nil {
		bus.Subscribe(plugin.EventBookImported, de.onBookImported)
		log.Printf("[dedup] PostInit: subscribed to EventBookImported")
	} else {
		log.Printf("[dedup] PostInit: eventbus not available, skipping dedup-on-import subscription")
	}

	// Step 2 — chromem store + hydrate
	if chromemStore, ok := serviceregistry.TryGet[*database.ChromemEmbeddingStore](c, "chromemstore"); ok && chromemStore != nil {
		de.SetChromemStore(chromemStore)
		log.Println("[INFO] chromem-go ANN store active for dedup Layer 2")
		// Hydrate asynchronously on the engine's bg-context.
		go func() {
			hCtx, cancel := context.WithTimeout(bgCtx, 30*time.Minute)
			defer cancel()
			books, authors, err := de.HydrateChromem(hCtx)
			if err != nil {
				log.Printf("[WARN] chromem hydrate finished with error: %v (books=%d authors=%d)", err, books, authors)
				return
			}
			log.Printf("[INFO] chromem hydrate complete: books=%d authors=%d", books, authors)
		}()
	}

	// Step 3 — aijobs store + verdict applier
	if jobs, ok := serviceregistry.TryGet[database.AIJobsStore](c, "aijobsstore"); ok && jobs != nil {
		de.SetAIJobsStore(jobs)
		ai.SetDedupVerdictApplier(de)
		log.Println("[INFO] Dedup async review (aijobs) wired")
	}

	return nil
}

// onBookImported is the EventHandler bound to EventBookImported. It runs
// in a goroutine spawned by EventBus.Publish; we ignore the publisher's
// ctx in favor of the engine's bg-context so external cancellation
// (e.g. an HTTP request that just finished) doesn't cut the dedup check
// off mid-stream.
func (de *Engine) onBookImported(_ context.Context, evt plugin.Event) error {
	if evt.BookID == "" {
		return nil
	}
	de.bgMu.RLock()
	bgCtx := de.bgCtx
	de.bgMu.RUnlock()
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	if _, err := de.CheckBook(bgCtx, evt.BookID); err != nil {
		log.Printf("[WARN] dedup-on-import CheckBook(%s): %v", evt.BookID, err)
	}
	return nil
}

// Stop releases the engine's background-context resources. Called by
// Container.Stop. Safe to call multiple times.
func (de *Engine) Stop(ctx context.Context) error {
	if de == nil {
		return nil
	}
	de.bgMu.Lock()
	defer de.bgMu.Unlock()
	if de.bgCancel != nil {
		de.bgCancel()
		de.bgCancel = nil
		de.bgCtx = nil
	}
	return nil
}
