- [x] ✅ **Backend**: Database migration system with version tracking
- [x] ✅ **Backend**: Complete audiobook CRUD API (create, read, update, delete, batch)
- [x] ✅ **Backend**: Authors and series API endpoints
- [x] ✅ **Backend**: Library folder management API
- [x] ✅ **Backend**: Operation tracking and logs API
- [x] ✅ **Backend**: HTTP server with configurable timeouts
- [x] ✅ **Backend**: Safe file operations wrapper (copy-first, checksums, rollback)
- [x] ✅ **Backend**: File system browsing API (.jabexclude, stats, permissions)
- [x] ✅ **Backend**: Async operation queue system with priority handling
- [x] ✅ **Backend**: WebSocket/SSE for real-time operation updates
- [x] ✅ **Backend**: Database backup and restore functionality
- [x] ✅ **Backend**: Enhanced metadata API (batch updates, history, validation)

- [x] ✅ **Frontend**: React app setup with TypeScript and Material-UI
- [x] ✅ **Frontend**: Dashboard with library statistics and navigation
- [x] ✅ **Frontend**: Library page with import path management
- [x] ✅ **Frontend**: System page with Logs, Storage, Quotas, System Info tabs
- [x] ✅ **Frontend**: Settings page with comprehensive configuration options
- [x] ✅ **Frontend**: Smart folder/file naming patterns with live examples

- [x] ✅ **Backend**: Media info extraction from audio files
  - ✅ Created mediainfo package with Extract() function
  - ✅ Supports MP3, M4A/M4B, FLAC, OGG formats
  - ✅ Extracts bitrate, codec, sample rate, channels, bit depth
  - ✅ Quality string generation and tier comparison
- [ ] 🟡 **Backend**: Version management API (link versions, set primary, manage version groups)
- [ ] 🟡 **Backend**: Import paths CRUD API (list, add, remove, scan)
- [ ] 🟡 **Backend**: System info API (storage, quotas, system stats)
- [ ] 🟡 **Backend**: Logs API with filtering (level, source, search, pagination)
- [ ] 🟡 **Backend**: Settings API (save/load configuration)
- [x] **Backend - Database migration for media info and version fields**
  - ✅ Created migration005 adding all 9 fields to books table
  - ✅ Handles duplicate column detection gracefully
  - ✅ Creates indices for version_group_id and is_primary_version
- [ ] 🟡 **Backend**: Metadata source integration (Audible, Goodreads, Open Library, Google Books)

- [ ] 🟡 **Frontend**: Library browser with grid/list views and version selection
- [ ] 🟡 **Frontend**: Metadata editor with inline editing and version management
- [ ] 🟡 **Frontend**: Multiple version display and management UI
- [ ] 🟡 **Frontend**: Connect all pages to backend APIs

- [ ] 🟡 **General**: Configure GitHub workflows
- [ ] 🟡 **Testing**: Unit and integration test framework
- [ ] 🟡 **Docs**: OpenAPI/Swagger documentation

- [ ] 🟡 **General**: Implement library organization with hard links, reflinks,
      or copies (auto mode tries reflink → hardlink → copy)
