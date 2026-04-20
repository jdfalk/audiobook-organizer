// file: internal/plugin/registry.go
// version: 1.0.0

package plugin

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// registry is the global plugin registry.
var globalRegistry = &Registry{
	plugins: make(map[string]Plugin),
	enabled: make(map[string]bool),
}

// Registry manages plugin lifecycle.
type Registry struct {
	plugins   map[string]Plugin
	enabled   map[string]bool
	initOrder []string
	mu        sync.RWMutex
}

// Register adds a plugin to the global registry. Called from init().
func Register(p Plugin) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	if _, exists := globalRegistry.plugins[p.ID()]; exists {
		log.Printf("[WARN] plugin %q already registered, skipping duplicate", p.ID())
		return
	}
	globalRegistry.plugins[p.ID()] = p
	log.Printf("[INFO] plugin registered: %s (%s) v%s", p.Name(), p.ID(), p.Version())
}

// Global returns the global registry.
func Global() *Registry {
	return globalRegistry
}

// Get returns a plugin by ID.
func (r *Registry) Get(id string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

// All returns all registered plugins.
func (r *Registry) All() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	return out
}

// Enable marks a plugin as enabled.
func (r *Registry) Enable(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled[id] = true
}

// Disable marks a plugin as disabled.
func (r *Registry) Disable(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled[id] = false
}

// IsEnabled returns whether a plugin is enabled.
func (r *Registry) IsEnabled(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enabled[id]
}

// ByCapability returns all enabled plugins with a given capability.
func (r *Registry) ByCapability(cap Capability) []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Plugin
	for id, p := range r.plugins {
		if !r.enabled[id] {
			continue
		}
		for _, c := range p.Capabilities() {
			if c == cap {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// InitAll initializes all enabled plugins in registration order.
func (r *Registry) InitAll(ctx context.Context, baseDeps Deps) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.initOrder = nil
	for id, p := range r.plugins {
		if !r.enabled[id] {
			log.Printf("[INFO] plugin %s: disabled, skipping init", id)
			continue
		}

		deps := baseDeps
		deps.Config = baseDeps.Config // each plugin gets its own config in real wiring
		deps.Logger = baseDeps.Logger // scoped logger in real wiring

		if err := p.Init(ctx, deps); err != nil {
			return fmt.Errorf("plugin %s init failed: %w", id, err)
		}
		r.initOrder = append(r.initOrder, id)
		log.Printf("[INFO] plugin %s: initialized", id)
	}
	return nil
}

// ShutdownAll shuts down all initialized plugins in reverse order.
func (r *Registry) ShutdownAll(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := len(r.initOrder) - 1; i >= 0; i-- {
		id := r.initOrder[i]
		if p, ok := r.plugins[id]; ok {
			if err := p.Shutdown(ctx); err != nil {
				log.Printf("[WARN] plugin %s shutdown error: %v", id, err)
			} else {
				log.Printf("[INFO] plugin %s: shut down", id)
			}
		}
	}
	r.initOrder = nil
}

// HealthCheckAll runs health checks on all initialized plugins.
func (r *Registry) HealthCheckAll() map[string]error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string]error, len(r.initOrder))
	for _, id := range r.initOrder {
		if p, ok := r.plugins[id]; ok {
			results[id] = p.HealthCheck()
		}
	}
	return results
}

// ResetForTesting clears the global registry. Test-only.
func ResetForTesting() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.plugins = make(map[string]Plugin)
	globalRegistry.enabled = make(map[string]bool)
	globalRegistry.initOrder = nil
}
