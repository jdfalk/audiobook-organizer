# Plugin System Framework — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the plugin framework (`internal/plugin`) with Plugin interface, EventBus, Registry, and PluginRouter — then migrate Deluge as the first proof-point plugin.

**Architecture:** Framework package defines contracts. Official plugins live in `internal/plugins/<name>/`. Registry manages lifecycle. EventBus replaces ad-hoc hooks. Server wires everything in `NewServer()`.

**Tech Stack:** Go 1.24, Gin HTTP framework, testify for mocks

**Spec:** `docs/superpowers/specs/2026-04-19-plugin-system-design.md`

**Scope:** Framework + Deluge migration only. iTunes migration depends on 4.12 extraction (in progress). Metadata source migration is a follow-up.

---

## File Structure

### New files

```
internal/plugin/
    plugin.go           # Plugin, Capability, Deps interfaces
    registry.go         # Registry struct, Register(), lifecycle methods
    events.go           # EventBus, EventType constants, Event struct, constructors
    router.go           # PluginRouter (scoped Gin wrapper)
    plugin_test.go      # Registry lifecycle tests
    events_test.go      # EventBus pub/sub tests
    router_test.go      # PluginRouter scoping tests

internal/plugins/
    deluge/
        plugin.go       # Deluge plugin implementing Plugin + DownloadClient
        plugin_test.go  # Unit tests with mock store
```

### Modified files

```
internal/server/server.go          # Add EventBus + Registry to Server, wire in NewServer()
internal/server/deluge_integration.go  # Replaced by plugin (delete or thin wrapper)
internal/config/config.go          # Add plugins config section
```

---

## Task 1: Define Plugin interface and Capability types

**Files:**
- Create: `internal/plugin/plugin.go`

- [ ] **Step 1: Create the plugin package with core types**

```go
// file: internal/plugin/plugin.go
package plugin

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// Capability identifies what a plugin can do.
type Capability string

const (
	CapMetadataSource  Capability = "metadata_source"
	CapDownloadClient  Capability = "download_client"
	CapMediaPlayer     Capability = "media_player"
	CapNotifier        Capability = "notifier"
	CapEventSubscriber Capability = "event_subscriber"
)

// Plugin is the base interface every plugin implements.
type Plugin interface {
	ID() string
	Name() string
	Version() string
	Capabilities() []Capability
	Init(ctx context.Context, deps Deps) error
	Shutdown(ctx context.Context) error
	HealthCheck() error
}

// Deps is the dependency bag passed to plugins during Init.
// Plugins use this to interact with the host. They never import
// internal/server.
type Deps struct {
	Store  database.Store
	Events *EventBus
	Config map[string]string
	Logger logger.Logger
	Router PluginRouter
	Queue  operations.Queue
}

// DownloadClient is implemented by plugins that manage a download client.
type DownloadClient interface {
	Plugin
	TestConnection() error
	ListTorrents() ([]TorrentInfo, error)
	MoveStorage(torrentHash, newPath string) error
}

// TorrentInfo describes a torrent known to the download client.
type TorrentInfo struct {
	Hash       string `json:"hash"`
	Name       string `json:"name"`
	SavePath   string `json:"save_path"`
	TotalSize  int64  `json:"total_size"`
	Progress   float64 `json:"progress"`
	State      string `json:"state"`
}

// MediaPlayer is implemented by plugins that sync with a media server.
type MediaPlayer interface {
	Plugin
	SyncLibrary(books []BookInfo) error
	GetPlaybackState(bookID string) (*PlaybackState, error)
	UpdatePlaybackState(bookID string, state PlaybackState) error
}

// BookInfo is a serializable summary of a book for media player sync.
type BookInfo struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Path   string `json:"path"`
}

// PlaybackState represents playback position in a media player.
type PlaybackState struct {
	PositionSeconds float64 `json:"position_seconds"`
	Finished        bool    `json:"finished"`
}

// Notifier is implemented by plugins that send notifications.
type Notifier interface {
	Plugin
	Notify(ctx context.Context, event Event) error
	SupportedEvents() []EventType
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./internal/plugin/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/plugin/plugin.go
git commit -m "feat(plugin): define Plugin interface and capability types"
```

---

## Task 2: Implement EventBus

**Files:**
- Create: `internal/plugin/events.go`
- Create: `internal/plugin/events_test.go`

- [ ] **Step 1: Create EventBus with event types and publish/subscribe**

```go
// file: internal/plugin/events.go
package plugin

import (
	"context"
	"log"
	"sync"
	"time"
)

// EventType identifies a lifecycle event.
type EventType string

const (
	EventBookImported      EventType = "book.imported"
	EventBookDeleted       EventType = "book.deleted"
	EventMetadataApplied   EventType = "metadata.applied"
	EventTagsWritten       EventType = "tags.written"
	EventFileOrganized     EventType = "file.organized"
	EventDedupDetected     EventType = "dedup.detected"
	EventDedupMerged       EventType = "dedup.merged"
	EventCoverChanged      EventType = "cover.changed"
	EventReadStatusChanged EventType = "read_status.changed"
	EventScanCompleted     EventType = "scan.completed"
)

// Event is a JSON-serializable lifecycle event.
type Event struct {
	Type      EventType         `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	BookID    string            `json:"book_id,omitempty"`
	UserID    string            `json:"user_id,omitempty"`
	Data      map[string]any    `json:"data,omitempty"`
}

// NewEvent creates an event with the current timestamp.
func NewEvent(eventType EventType, bookID string, data map[string]any) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now(),
		BookID:    bookID,
		Data:      data,
	}
}

// EventHandler processes a single event.
type EventHandler func(ctx context.Context, event Event) error

// EventBus manages event subscriptions and publishing.
type EventBus struct {
	subscribers map[EventType][]EventHandler
	mu          sync.RWMutex
}

// NewEventBus creates an empty event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[EventType][]EventHandler),
	}
}

// Subscribe registers a handler for an event type.
func (b *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
}

// Publish sends an event to all subscribers. Handlers run in goroutines.
// Errors are logged but do not propagate to the publisher.
func (b *EventBus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	handlers := b.subscribers[event.Type]
	b.mu.RUnlock()

	for _, handler := range handlers {
		h := handler // capture
		go func() {
			if err := h(ctx, event); err != nil {
				log.Printf("[WARN] plugin event handler error for %s: %v", event.Type, err)
			}
		}()
	}
}

// SubscriberCount returns how many handlers are registered for an event type.
func (b *EventBus) SubscriberCount(eventType EventType) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[eventType])
}
```

- [ ] **Step 2: Write EventBus tests**

```go
// file: internal/plugin/events_test.go
package plugin

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBus_SubscribeAndPublish(t *testing.T) {
	bus := NewEventBus()
	var called atomic.Bool

	bus.Subscribe(EventBookImported, func(ctx context.Context, evt Event) error {
		called.Store(true)
		assert.Equal(t, "book-1", evt.BookID)
		return nil
	})

	bus.Publish(context.Background(), NewEvent(EventBookImported, "book-1", nil))
	time.Sleep(50 * time.Millisecond) // handlers run in goroutines
	assert.True(t, called.Load())
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	var count atomic.Int32

	for i := 0; i < 3; i++ {
		bus.Subscribe(EventMetadataApplied, func(ctx context.Context, evt Event) error {
			count.Add(1)
			return nil
		})
	}

	bus.Publish(context.Background(), NewEvent(EventMetadataApplied, "book-1", nil))
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(3), count.Load())
}

func TestEventBus_NoSubscribers(t *testing.T) {
	bus := NewEventBus()
	// Should not panic
	bus.Publish(context.Background(), NewEvent(EventBookDeleted, "book-1", nil))
}

func TestEventBus_HandlerErrorDoesNotPanic(t *testing.T) {
	bus := NewEventBus()
	bus.Subscribe(EventScanCompleted, func(ctx context.Context, evt Event) error {
		return assert.AnError
	})
	// Should not panic
	bus.Publish(context.Background(), NewEvent(EventScanCompleted, "", nil))
	time.Sleep(50 * time.Millisecond)
}

func TestEventBus_SubscriberCount(t *testing.T) {
	bus := NewEventBus()
	assert.Equal(t, 0, bus.SubscriberCount(EventBookImported))
	bus.Subscribe(EventBookImported, func(ctx context.Context, evt Event) error { return nil })
	bus.Subscribe(EventBookImported, func(ctx context.Context, evt Event) error { return nil })
	assert.Equal(t, 2, bus.SubscriberCount(EventBookImported))
}

func TestNewEvent(t *testing.T) {
	evt := NewEvent(EventFileOrganized, "book-1", map[string]any{"old_path": "/a", "new_path": "/b"})
	assert.Equal(t, EventFileOrganized, evt.Type)
	assert.Equal(t, "book-1", evt.BookID)
	assert.Equal(t, "/a", evt.Data["old_path"])
	require.WithinDuration(t, time.Now(), evt.Timestamp, time.Second)
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/plugin/... -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/plugin/events.go internal/plugin/events_test.go
git commit -m "feat(plugin): implement EventBus with pub/sub"
```

---

## Task 3: Implement PluginRouter

**Files:**
- Create: `internal/plugin/router.go`
- Create: `internal/plugin/router_test.go`

- [ ] **Step 1: Create PluginRouter that scopes routes**

```go
// file: internal/plugin/router.go
package plugin

import "github.com/gin-gonic/gin"

// PluginRouter provides scoped HTTP route registration.
// All routes are automatically prefixed with /api/v1/plugins/{pluginID}/.
type PluginRouter interface {
	GET(path string, handler gin.HandlerFunc)
	POST(path string, handler gin.HandlerFunc)
	PUT(path string, handler gin.HandlerFunc)
	DELETE(path string, handler gin.HandlerFunc)
	Group(path string) PluginRouter
}

// ginPluginRouter wraps a gin.RouterGroup to implement PluginRouter.
type ginPluginRouter struct {
	group *gin.RouterGroup
}

// NewPluginRouter creates a scoped router for a plugin.
func NewPluginRouter(parent *gin.RouterGroup, pluginID string) PluginRouter {
	return &ginPluginRouter{group: parent.Group("/" + pluginID)}
}

func (r *ginPluginRouter) GET(path string, handler gin.HandlerFunc)    { r.group.GET(path, handler) }
func (r *ginPluginRouter) POST(path string, handler gin.HandlerFunc)   { r.group.POST(path, handler) }
func (r *ginPluginRouter) PUT(path string, handler gin.HandlerFunc)    { r.group.PUT(path, handler) }
func (r *ginPluginRouter) DELETE(path string, handler gin.HandlerFunc) { r.group.DELETE(path, handler) }

func (r *ginPluginRouter) Group(path string) PluginRouter {
	return &ginPluginRouter{group: r.group.Group(path)}
}
```

- [ ] **Step 2: Write router tests**

```go
// file: internal/plugin/router_test.go
package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPluginRouter_ScopedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	parent := engine.Group("/api/v1/plugins")

	router := NewPluginRouter(parent, "test-plugin")
	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/plugins/test-plugin/status", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPluginRouter_Group(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	parent := engine.Group("/api/v1/plugins")

	router := NewPluginRouter(parent, "my-plugin")
	sub := router.Group("/torrents")
	sub.GET("/list", func(c *gin.Context) {
		c.JSON(200, gin.H{"torrents": []string{}})
	})

	req := httptest.NewRequest("GET", "/api/v1/plugins/my-plugin/torrents/list", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPluginRouter_WrongPath_404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	parent := engine.Group("/api/v1/plugins")

	router := NewPluginRouter(parent, "my-plugin")
	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/plugins/other-plugin/status", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

- [ ] **Step 3: Run tests and commit**

```bash
go test ./internal/plugin/... -v
git add internal/plugin/router.go internal/plugin/router_test.go
git commit -m "feat(plugin): implement PluginRouter with scoped routes"
```

---

## Task 4: Implement Plugin Registry

**Files:**
- Create: `internal/plugin/registry.go`
- Create: `internal/plugin/registry_test.go`

- [ ] **Step 1: Create Registry with register, init, shutdown**

```go
// file: internal/plugin/registry.go
package plugin

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// registry is the global plugin registry.
var (
	globalRegistry = &Registry{
		plugins: make(map[string]Plugin),
		enabled: make(map[string]bool),
	}
)

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
```

- [ ] **Step 2: Write Registry tests**

Test registration, enable/disable, InitAll order, ShutdownAll reverse order, ByCapability filtering, health checks, duplicate registration.

- [ ] **Step 3: Run tests and commit**

```bash
go test ./internal/plugin/... -v
git add internal/plugin/registry.go internal/plugin/registry_test.go
git commit -m "feat(plugin): implement Registry with lifecycle management"
```

---

## Task 5: Migrate Deluge as first plugin

**Files:**
- Create: `internal/plugins/deluge/plugin.go`
- Create: `internal/plugins/deluge/plugin_test.go`
- Modify: `internal/server/server.go` — wire registry + event bus
- Modify: `internal/server/deluge_integration.go` — delegate to plugin

- [ ] **Step 1: Create Deluge plugin**

The Deluge plugin wraps the existing `internal/deluge.Client` and implements `Plugin` + `DownloadClient`:

```go
// file: internal/plugins/deluge/plugin.go
package deluge

import (
	"context"
	"fmt"

	delugeclient "github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
)

type Plugin struct {
	client *delugeclient.Client
	config map[string]string
}

func init() {
	plugin.Register(&Plugin{})
}

func (p *Plugin) ID() string           { return "deluge" }
func (p *Plugin) Name() string         { return "Deluge" }
func (p *Plugin) Version() string      { return "1.0.0" }
func (p *Plugin) Capabilities() []plugin.Capability {
	return []plugin.Capability{plugin.CapDownloadClient}
}

func (p *Plugin) Init(ctx context.Context, deps plugin.Deps) error {
	url := deps.Config["web_url"]
	password := deps.Config["password"]
	if url == "" {
		return fmt.Errorf("deluge: web_url is required")
	}
	p.client = delugeclient.NewClient(url, password)
	p.config = deps.Config
	return nil
}

func (p *Plugin) Shutdown(ctx context.Context) error {
	p.client = nil
	return nil
}

func (p *Plugin) HealthCheck() error {
	if p.client == nil {
		return fmt.Errorf("deluge: not initialized")
	}
	return p.client.TestConnection()
}

func (p *Plugin) TestConnection() error {
	return p.client.TestConnection()
}

func (p *Plugin) ListTorrents() ([]plugin.TorrentInfo, error) {
	torrents, err := p.client.GetTorrents()
	if err != nil {
		return nil, err
	}
	out := make([]plugin.TorrentInfo, len(torrents))
	for i, t := range torrents {
		out[i] = plugin.TorrentInfo{
			Hash:      t.Hash,
			Name:      t.Name,
			SavePath:  t.SavePath,
			TotalSize: t.TotalSize,
			Progress:  t.Progress,
			State:     t.State,
		}
	}
	return out, nil
}

func (p *Plugin) MoveStorage(torrentHash, newPath string) error {
	return p.client.MoveStorage(torrentHash, newPath)
}
```

- [ ] **Step 2: Wire registry and event bus into Server**

In `internal/server/server.go`, add to Server struct:

```go
eventBus       *plugin.EventBus
pluginRegistry *plugin.Registry
```

In `NewServer()`:

```go
server.eventBus = plugin.NewEventBus()
server.pluginRegistry = plugin.Global()

// Enable configured plugins
for id, cfg := range pluginConfigs {
    if cfg.Enabled {
        server.pluginRegistry.Enable(id)
    }
}

// Init all enabled plugins
pluginRoutes := server.router.Group("/api/v1/plugins")
server.pluginRegistry.InitAll(ctx, plugin.Deps{
    Store:  resolvedStore,
    Events: server.eventBus,
    Queue:  server.queue,
    Router: plugin.NewPluginRouter(pluginRoutes, ""), // each plugin gets scoped
})
```

In `Shutdown()`:

```go
server.pluginRegistry.ShutdownAll(ctx)
```

- [ ] **Step 3: Add plugin config section**

In `internal/config/config.go`, add:

```go
type PluginConfig struct {
    Enabled bool              `json:"enabled" yaml:"enabled"`
    Config  map[string]string `json:"config" yaml:"config"`
}
```

And to `Config`:

```go
Plugins map[string]PluginConfig `json:"plugins" yaml:"plugins"`
```

- [ ] **Step 4: Update deluge_integration.go to delegate**

The existing handlers in `deluge_integration.go` should check if the deluge plugin is available and delegate:

```go
func (s *Server) handleDelugeTestConnection(c *gin.Context) {
    plugins := s.pluginRegistry.ByCapability(plugin.CapDownloadClient)
    // find deluge plugin and call TestConnection
}
```

Or keep the existing handlers and have them call through to the plugin.

- [ ] **Step 5: Write plugin test**

```go
// file: internal/plugins/deluge/plugin_test.go
package deluge

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/stretchr/testify/assert"
)

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "deluge", p.ID())
	assert.Equal(t, "Deluge", p.Name())
	assert.Contains(t, p.Capabilities(), plugin.CapDownloadClient)
}

func TestPlugin_InitRequiresURL(t *testing.T) {
	p := &Plugin{}
	err := p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "web_url")
}

func TestPlugin_InitSuccess(t *testing.T) {
	p := &Plugin{}
	err := p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{
			"web_url":  "http://localhost:8112",
			"password": "deluge",
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, p.client)
}

func TestPlugin_HealthCheckNotInitialized(t *testing.T) {
	p := &Plugin{}
	err := p.HealthCheck()
	assert.Error(t, err)
}
```

- [ ] **Step 6: Run tests and commit**

```bash
go build ./...
go test ./internal/plugin/... ./internal/plugins/deluge/... -v
git add -A
git commit -m "feat(plugin): migrate Deluge as first plugin + wire into Server"
```

---

## Task 6: Publish events at key lifecycle points

**Files:**
- Modify: `internal/server/server.go` — pass eventBus to services
- Modify: `internal/server/audiobooks_handlers.go` — publish BookImported/BookDeleted
- Modify: `internal/server/metadata_handlers.go` — publish MetadataApplied
- Modify: Various handler files — add event publishing at key points

- [ ] **Step 1: Add event publishing helper to Server**

```go
func (s *Server) publishEvent(eventType plugin.EventType, bookID string, data map[string]any) {
    if s.eventBus != nil {
        s.eventBus.Publish(s.bgCtx, plugin.NewEvent(eventType, bookID, data))
    }
}
```

- [ ] **Step 2: Publish events at key points**

In handlers and services, add calls:

```go
// After successful book import:
s.publishEvent(plugin.EventBookImported, book.ID, map[string]any{
    "file_path": book.FilePath,
    "source":    "scan",
})

// After metadata applied:
s.publishEvent(plugin.EventMetadataApplied, bookID, map[string]any{
    "source":         metadataSource,
    "changed_fields": changedFields,
})

// After file organized:
s.publishEvent(plugin.EventFileOrganized, bookID, map[string]any{
    "old_path": oldPath,
    "new_path": newPath,
})
```

Add events at 5-8 key lifecycle points. Don't try to cover all 10 event types — add more as plugins need them.

- [ ] **Step 3: Run tests and commit**

```bash
go build ./...
go test ./internal/server/... -short
git add -A
git commit -m "feat(plugin): publish lifecycle events at key points"
```

---

## Task 7: Add plugin health to system status + update TODO

**Files:**
- Modify: `internal/server/system_handlers.go` — include plugin health in status endpoint
- Modify: `TODO.md` — add 4.14 item

- [ ] **Step 1: Add plugin health to system status response**

In `handleSystemStatus`:

```go
// Add to status response:
pluginHealth := make(map[string]string)
if s.pluginRegistry != nil {
    for id, err := range s.pluginRegistry.HealthCheckAll() {
        if err != nil {
            pluginHealth[id] = err.Error()
        } else {
            pluginHealth[id] = "ok"
        }
    }
}
// Include in response JSON
```

- [ ] **Step 2: Update TODO.md**

Add 4.14 as in-progress, add future items for metadata source migration and V2 RPC/webhooks.

- [ ] **Step 3: Run tests and commit**

```bash
go build ./...
go test ./internal/server/... -short -run TestHandler
git add -A
git commit -m "feat(plugin): add plugin health to system status + update TODO"
```

---

## Task 8: Final verification

- [ ] **Step 1: Full build and test**

```bash
go build ./...
go vet ./...
go test ./internal/plugin/... ./internal/plugins/... -v
```

- [ ] **Step 2: Verify plugin registration works end-to-end**

```bash
# Import the deluge plugin in a test and verify registration
go test ./internal/plugin/... -run TestRegistry -v
```

- [ ] **Step 3: Push and create PR**

```bash
git push -u origin feat/plugin-system
gh pr create --title "feat: plugin system framework + Deluge migration (4.14)" \
  --body "V1 plugin framework with:
- Plugin interface + 5 capability types
- EventBus with typed lifecycle events
- Registry with lifecycle management
- PluginRouter for scoped HTTP routes
- Deluge migrated as first official plugin
- Events published at key lifecycle points
- Plugin health in system status endpoint

Spec: docs/superpowers/specs/2026-04-19-plugin-system-design.md"
```
