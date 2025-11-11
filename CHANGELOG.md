## [Unreleased]

### Added

  - Created `internal/server/server_test.go` with 20+ test functions covering all endpoints
  - Created `scripts/test-api-endpoints.py` for manual endpoint testing with performance metrics
  - Added end-to-end workflow tests and response time benchmarks
  - Created `docs/api-testing-summary.md` documenting test results and discovered issues
  - Created `scripts/TEST-README.md` with complete testing documentation
  - Tests identified 10 critical bugs before manual testing (null arrays, ID format issues, validation gaps)
## [Unreleased]

### Added / Changed

- Extended Book metadata fields: work_id, narrator, edition, language, publisher, isbn10, isbn13 (with SQLite migration & CRUD support)
- API tests for extended metadata (round‑trip + update semantics)
- Hardened audiobook update handler error checking (nil-safe not found handling)
- Metadata extraction scaffolding for future multi-format support (tag reader integration prep)
- Work entity: basic model, SQLite schema, Pebble+SQLite store methods, and REST API endpoints (list/get/create/update/delete, list books by work)

### Upcoming

- Audio tag reading for MP3 (ID3v2), M4B/M4A (iTunes atoms), FLAC/OGG (Vorbis comments), AAC
- Safe in-place metadata writing with backup/rollback
- Work entity (model + CRUD + association to Book via `work_id`)
- Manual endpoint regression run post ULID + metadata changes
- Git LFS sample audiobook fixtures for integration tests
  - POST `/api/filesystem/exclude` - Create .jabexclude files
