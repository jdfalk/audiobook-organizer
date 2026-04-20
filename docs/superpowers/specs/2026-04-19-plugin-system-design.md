# Plugin System Design

**Date:** 2026-04-19
**Status:** Approved
**Backlog item:** New (add as 4.14)

## Problem

Integrations (iTunes, Deluge, metadata sources) are hard-wired into the server
with ad-hoc patterns — callback hooks, global variables, direct constructor
wiring. Adding new integrations requires modifying core server code. Users
cannot add custom integrations without recompiling.

## Goals

- **Unified integration model** — one pattern for all integrations instead of
  per-integration wiring
- **User extensibility** — users can eventually add custom plugins (media
  players, notifiers, download clients) without modifying core code
- **Testability** — plugins are isolated units with mock-friendly interfaces
- **Gradual migration** — existing integrations migrate one at a time without
  breaking changes

## Non-goals (V1)

- RPC/subprocess plugins (V2)
- Webhook event delivery (V2)
- Plugin marketplace or discovery
- Hot-reload of plugins at runtime
- UI for plugin management (use config file for now)

## Architecture

Two-tier design:

1. **V1 (this spec):** Compile-time plugins. Official integrations are Go
   packages that implement well-defined interfaces. Included in the binary by
   default, excludable via build tags. A central registry manages lifecycle.
   An event bus replaces ad-hoc callback hooks.

2. **V2 (future):** RPC + webhook plugins. External binaries communicate over
   gRPC (HashiCorp go-plugin pattern). Webhook subscribers receive event
   POSTs. Same interfaces — the transport is transparent.

V1 interfaces are designed to be transport-agnostic so V2 works without
redesigning the contracts.

## Plugin Base Interface

Every plugin implements:

```go
package plugin

type Plugin interface {
    ID() string
    Name() string
    Version() string
    Capabilities() []Capability
    Init(ctx context.Context, deps Deps) error
    Shutdown(ctx context.Context) error
    HealthCheck() error
}

type Capability string

const (
    CapMetadataSource  Capability = "metadata_source"
    CapDownloadClient  Capability = "download_client"
    CapMediaPlayer     Capability = "media_player"
    CapNotifier        Capability = "notifier"
    CapEventSubscriber Capability = "event_subscriber"
    CapStorageProvider Capability = "storage_provider" // future
)
```

## Plugin Dependencies

Plugins receive a `Deps` struct during `Init`. This is their only way to
interact with the host. They never import `internal/server`.

```go
type Deps struct {
    Store   database.Store       // database access
    Events  *EventBus            // subscribe to lifecycle events
    Config  map[string]string    // plugin-specific config values
    Logger  logger.Logger        // scoped logger
    Router  PluginRouter         // HTTP routes under /api/v1/plugins/{id}/
    Queue   operations.Queue     // enqueue async operations
}

type PluginRouter interface {
    GET(path string, handler gin.HandlerFunc)
    POST(path string, handler gin.HandlerFunc)
    PUT(path string, handler gin.HandlerFunc)
    DELETE(path string, handler gin.HandlerFunc)
    Group(path string) PluginRouter
}
```

`PluginRouter` scopes all routes to `/api/v1/plugins/{pluginID}/` automatically.
Plugins cannot register routes outside their scope.

## Capability Interfaces

### MetadataSource (existing, formalized)

```go
type MetadataSource interface {
    Plugin
    SearchByTitle(title string) ([]BookMetadata, error)
    SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error)
}

// Optional enrichment:
type ContextualSearcher interface {
    SearchByContext(ctx *SearchContext) ([]BookMetadata, error)
}
```

### DownloadClient (new)

```go
type DownloadClient interface {
    Plugin
    TestConnection() error
    ListTorrents() ([]TorrentInfo, error)
    MoveStorage(torrentHash, newPath string) error
}
```

### MediaPlayer (new)

```go
type MediaPlayer interface {
    Plugin
    SyncLibrary(books []BookInfo) error
    GetPlaybackState(bookID string) (*PlaybackState, error)
    UpdatePlaybackState(bookID string, state PlaybackState) error
}
```

### Notifier (new)

```go
type Notifier interface {
    Plugin
    Notify(event Event) error
    SupportedEvents() []EventType
}
```

### EventSubscriber (new)

```go
type EventSubscriber interface {
    Plugin
    HandleEvent(ctx context.Context, event Event) error
    SubscribedEvents() []EventType
}
```

## Event System

### Event Types

```go
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
```

### Event Struct

All events are JSON-serializable (required for V2 webhook/RPC transport):

```go
type Event struct {
    Type      EventType         `json:"type"`
    Timestamp time.Time         `json:"timestamp"`
    BookID    string            `json:"book_id,omitempty"`
    UserID    string            `json:"user_id,omitempty"`
    Data      map[string]any    `json:"data"`
}
```

`Data` carries event-specific fields. Typed helper constructors build events
with validated fields:

```go
func NewBookImportedEvent(bookID, filePath, source string) Event
func NewMetadataAppliedEvent(bookID, metadataSource string, changedFields []string) Event
// etc.
```

### EventBus

```go
type EventBus struct {
    subscribers map[EventType][]EventHandler
    mu          sync.RWMutex
}

type EventHandler func(ctx context.Context, event Event) error

func (b *EventBus) Subscribe(eventType EventType, handler EventHandler)
func (b *EventBus) Publish(ctx context.Context, event Event)
```

`Publish` is fire-and-forget — handlers run in goroutines. Errors are logged
but don't propagate to the publisher. This prevents a misbehaving plugin from
blocking core operations.

### Migration from callback hooks

The existing hooks become internal event subscribers:

| Current hook | Replaced by |
|-------------|-------------|
| `ScanHooks.OnBookScanned` | `EventBookImported` subscriber |
| `ScanHooks.OnImportDedup` | `EventBookImported` subscriber (dedup engine subscribes) |
| `OrganizeHooks.OnCollision` | `EventFileOrganized` subscriber (with collision data) |
| `ActivityLogger.RecordActivity` | `EventBus` subscriber that writes to activity store |

This migration is optional for V1 — the hooks can coexist with the event bus.
Full migration happens when the hook-using code is ready.

## Plugin Registry

```go
type Registry struct {
    plugins  map[string]Plugin
    enabled  map[string]bool
    initOrder []string
    mu       sync.RWMutex
}

func Register(p Plugin)                              // called from init()
func (r *Registry) Enable(id string)
func (r *Registry) Disable(id string)
func (r *Registry) InitAll(ctx context.Context, baseDeps Deps) error
func (r *Registry) ShutdownAll(ctx context.Context) error
func (r *Registry) Get(id string) (Plugin, bool)
func (r *Registry) ByCapability(cap Capability) []Plugin
func (r *Registry) HealthCheckAll() map[string]error
```

### Registration

Official plugins register via `init()` in their package. The main binary
imports them:

```go
// cmd/main.go or internal/plugins/all.go:
import (
    _ "github.com/jdfalk/audiobook-organizer/internal/plugins/itunes"
    _ "github.com/jdfalk/audiobook-organizer/internal/plugins/deluge"
    _ "github.com/jdfalk/audiobook-organizer/internal/plugins/openlibrary"
    _ "github.com/jdfalk/audiobook-organizer/internal/plugins/audible"
    // ...
)
```

Build tags exclude optional plugins:

```go
//go:build !no_itunes

package itunes

func init() {
    plugin.Register(&Plugin{})
}
```

### Lifecycle

1. **Register** — `init()` adds plugin to global registry
2. **Configure** — `NewServer()` reads plugin config, calls `registry.Enable(id)` for configured plugins
3. **Init** — `registry.InitAll(ctx, deps)` calls `Init` on each enabled plugin in order
4. **Run** — plugins handle events, serve routes, respond to capability interface calls
5. **Health** — periodic `HealthCheckAll()`, surfaced in `/api/v1/system/status`
6. **Shutdown** — `registry.ShutdownAll(ctx)` in reverse init order

## Configuration

Plugin config lives in the existing config system:

```yaml
plugins:
  itunes:
    enabled: true
    library_path: "/mnt/bigdata/books/itunes/iTunes Library.xml"
    path_mappings:
      - from: "Z:\\"
        to: "/mnt/bigdata/"
  deluge:
    enabled: true
    web_url: "http://localhost:8112"
    password: "deluge"
  slack:
    enabled: false
    webhook_url: ""
    events: ["book.imported", "dedup.merged"]
```

Each plugin receives only its own config section as `map[string]string` via
`Deps.Config`. Plugins never read other plugins' config.

## Package Layout

```
internal/plugin/
    plugin.go           # Plugin interface, Capability, Deps
    registry.go         # Registry, Register(), lifecycle
    events.go           # EventBus, EventType, Event
    router.go           # PluginRouter (scoped Gin wrapper)

internal/plugins/       # official plugin implementations
    itunes/             # iTunes integration (migrated from server)
    deluge/             # Deluge download client
    openlibrary/        # OpenLibrary metadata source
    audible/            # Audible metadata source
    google_books/       # Google Books metadata source
    audnexus/           # Audnexus metadata source
    hardcover/          # Hardcover metadata source
    wikipedia/          # Wikipedia metadata source
```

`internal/plugin/` (singular) is the framework.
`internal/plugins/` (plural) contains implementations.

## V1 Scope

1. Define the `internal/plugin` package (Plugin, Deps, Registry, EventBus)
2. Migrate iTunes as the first official plugin (depends on 4.12 extraction)
3. Migrate Deluge as a second plugin (simpler, validates the pattern)
4. Migrate 2-3 metadata sources (OpenLibrary, Audible) to validate MetadataSource capability
5. Wire the EventBus into `NewServer()` and publish events at key lifecycle points
6. Add plugin health to `/api/v1/system/status`

## V2 Scope (future, not this spec)

- gRPC transport for subprocess plugins
- Webhook delivery for EventSubscriber
- Plugin discovery/loading from a directory
- UI settings panel for plugin configuration
- Plugin dependency declarations

## V2 Design Constraint

All V1 interfaces must be transport-agnostic:
- Event structs are JSON-serializable (no Go channels, no pointers to internal types)
- Capability interface methods use serializable params and returns
- `Deps` fields are interfaces (wrappable with RPC stubs)
- No assumption of shared memory between host and plugin

## Testing

- `internal/plugin` package: unit tests for Registry lifecycle, EventBus pub/sub,
  PluginRouter scoping
- Each official plugin: unit tests with MockStore (same pattern as existing
  service tests)
- Integration test: register 2-3 test plugins, verify Init/Shutdown/event
  delivery order
