## [Unreleased]

### Added

- MVP specification document with complete feature requirements and architecture
- Detailed 14-week implementation plan for web interface development
- Current progress analysis showing 35-40% MVP completion
- Executive summary document for project overview
- Enhanced README with MVP development roadmap
- Document library organization options
- Setup standard ghcommon workflows and scripts
- **Database migration system** with version tracking and sequential migration support
- **Complete audiobook CRUD API** with update, delete, and batch update operations
- **Authors and Series API endpoints** for listing and managing metadata
- **HTTP server enhancements** with configurable timeouts and better error handling
- **Library folder management** with full CRUD operations
- **Operation tracking** with create, status, cancel, and logs retrieval
- **Async operation queue system** with priority handling and background workers
- **Real-time updates via Server-Sent Events (SSE)** for operation progress and logs
- **WebSocket/SSE integration** with automatic client connection management

### Planning

- Identified web interface requirements: React + Material Design + TypeScript
- Planned backend enhancements: Go 1.25, REST API, WebSocket support
- Defined safe file operations with copy-first and backup strategies
- Outlined comprehensive format support for major audiobook formats
- Created detailed phase breakdown for 14-week development timeline

### Backend (Go)

- Implemented migration system in `internal/database/migrations.go`
  - Version tracking via user preferences
  - Sequential migration application with rollback support
  - Migration history tracking
  - Auto-run on database initialization
- Enhanced server routes (`internal/server/server.go` v1.4.0)
  - PUT `/api/v1/audiobooks/:id` - Update audiobook metadata
  - DELETE `/api/v1/audiobooks/:id` - Delete audiobook
  - POST `/api/v1/audiobooks/batch` - Batch update multiple audiobooks
  - GET `/api/v1/authors` - List all authors
  - GET `/api/v1/series` - List all series
  - GET `/api/filesystem/browse` - Browse server directories
  - POST `/api/filesystem/exclude` - Create .jabexclude files
  - DELETE `/api/filesystem/exclude` - Remove .jabexclude files
- Enhanced health check endpoint with database metrics
- Library folder CRUD complete (list, add, remove)
- Operation endpoints complete (scan, organize, status, cancel, logs)
- Server timeout configuration via CLI flags
- **Safe file operations** (`internal/fileops/safe_operations.go`)
  - Copy-first logic with automatic backups
  - SHA256 checksum verification
  - Atomic operations with rollback support
  - Configurable backup retention
  - SafeMove and SafeCopy utilities
- **Operation queue system** (`internal/operations/queue.go`)
  - Async operation execution with configurable workers
  - Priority-based queue (low, normal, high)
  - Cancellation support with context propagation
  - Progress reporting interface for operations
  - Automatic status updates to database
  - Integration with real-time event hub
- **Real-time event system** (`internal/realtime/events.go`)
  - Server-Sent Events (SSE) endpoint at `/api/events`
  - Operation progress streaming
  - Operation status changes (queued, running, completed, failed, canceled)
  - Operation log streaming with levels (info, warn, error)
  - Client subscription management
  - Automatic heartbeat for connection keepalive
  - Event types: operation.progress, operation.status, operation.log, system.status
- Enhanced server initialization (`cmd/root.go` v1.4.0)
  - Configurable worker count via `--workers` flag
  - Event hub initialization on server start
  - Graceful queue shutdown with timeout
  - Updated serve command with background operation support

