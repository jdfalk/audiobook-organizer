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

	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// PostInit wires the dedup engine into the plugin event bus. Called by
// Container.PostInit after Build completes. The engine subscribes to
// EventBookImported and runs CheckBook in its own goroutine with the
// engine's bg-context (NOT the event publisher's ctx — see Q1 brainstorm
// decision in deferred-work doc).
//
// Safe to call when the engine is nil (Build returns nil when API key
// isn't configured — typed-nil receiver allowed by Go method dispatch on
// pointer types only when method has a nil-check, which we do).
func (de *Engine) PostInit(ctx context.Context, c *serviceregistry.Container) error {
	if de == nil {
		return nil
	}
	bus, ok := serviceregistry.TryGet[*plugin.EventBus](c, "eventbus")
	if !ok || bus == nil {
		log.Printf("[dedup] PostInit: eventbus not available, skipping dedup-on-import subscription")
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
	de.bgMu.Unlock()

	bus.Subscribe(plugin.EventBookImported, de.onBookImported)
	log.Printf("[dedup] PostInit: subscribed to EventBookImported")
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

