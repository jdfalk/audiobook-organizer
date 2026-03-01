<!-- file: TODO.md -->
<!-- version: 3.0.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-03-01 -->

# Project TODO

> All detailed plans live in [`docs/plans/`](docs/plans/). This file is a
> priority-ordered index: one-line summary per item, link to the full plan.

---

## 🎯 MVP Status — February 16, 2026

**~99% complete** · All P0/P1/P2 items done · Release pipeline finalized · All critical bugs fixed

**Remaining for MVP**: manual QA sign-off

---

## 🚨 P0 — Critical Path to MVP

| Item | Plan |
| --- | --- |
| ~~Rotate exposed key + scrub `.env` from git history~~ ✅ Never committed, .gitignore correct | [Roadmap to 100%](docs/roadmap-to-100-percent.md) |
| ~~Complete OpenAPI coverage for all implemented endpoints~~ ✅ v1.1.0, 80+ paths | [Roadmap to 100%](docs/roadmap-to-100-percent.md) |
| ~~iTunes Library Import — Phases 2–4~~ ✅ Complete | [iTunes Integration](docs/plans/itunes-integration.md) |
| Manual QA & validation across all core workflows | [MVP Critical Path](docs/plans/mvp-critical-path.md) |
| ~~Release pipeline fixes (token permissions, GoReleaser, changelog)~~ ✅ ghcommon v1.10.3, GoReleaser prerelease, Makefile targets | [Release & DevOps](docs/plans/release-packaging-and-devops.md) |
| ~~Raise Go test coverage from 73.8% to 80%~~ ✅ 81.3% (38 integration tests) | [Session 10 Plan](docs/archive/SESSION_10_INTEGRATION_TEST_PLAN.md) |
| ~~Expand Playwright E2E to critical workflows~~ ✅ 134/134 passing | [MVP Critical Path](docs/plans/mvp-critical-path.md) |

---

## 🔴 P1 — High Priority

| Item | Plan |
| --- | --- |
| ~~Fix metadata fetch fallback (fails for translated/subtitled titles)~~ ✅ 5-step cascade with subtitle stripping + author-only search | [Metadata System](docs/plans/metadata-system.md) |
| ~~Design & implement multiple authors/narrators support~~ ✅ Narrator entity, BookNarrator junction, API endpoints, tests | [Metadata System](docs/plans/metadata-system.md) |
| ~~Metadata quality — raw tags, provenance display, expanded edit dialog~~ ✅ Field-states API, provenance indicators, lock icons | [Metadata System](docs/plans/metadata-system.md) |
| ~~Delete/purge flow refinement in Book Detail~~ ✅ Confirmation checkbox, block-hash explanation, deletion timestamp | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| ~~CI/CD health monitoring (detect action output drift)~~ ✅ Version checks, output logging, auto-issue creation | [Release & DevOps](docs/plans/release-packaging-and-devops.md) |
| Capture manual verification notes from P0 QA | [MVP Critical Path](docs/plans/mvp-critical-path.md) |

---

## 🟡 P2 — Medium Priority

| Item | Plan |
| --- | --- |
| ~~Persist operation logs + log view UX improvements~~ ✅ Migration 21, SQLite persistence, queue wiring | [Observability](docs/plans/observability-and-monitoring.md) |
| ~~SSE system status heartbeats (live metrics without polling)~~ ✅ Complete | [Observability](docs/plans/observability-and-monitoring.md) |
| ~~Parallel scanning with goroutine pool~~ ✅ Complete | [Performance & Reliability](docs/plans/performance-and-reliability.md) |
| ~~Caching layer for frequent book queries~~ ✅ 30s/10s TTL cache with invalidation | [Performance & Reliability](docs/plans/performance-and-reliability.md) |
| ~~Debounced library size recomputation via fsnotify~~ ✅ Recursive watcher with 5s debounce, audio file filtering | [Performance & Reliability](docs/plans/performance-and-reliability.md) |
| ~~Global notification/toast system~~ ✅ Toast provider, auto-dismiss for success/info, persist for error/warning | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| ~~Dark mode with persisted preference~~ ✅ Complete | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| ~~Keyboard shortcuts~~ ✅ / or Ctrl+K for search, g+l library, g+s settings, ? help | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| ~~Welcome wizard (first-run setup)~~ ✅ Complete | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| ~~Developer guide (architecture, data flow, deployment)~~ ✅ docs/developer-guide.md | [MVP Critical Path](docs/plans/mvp-critical-path.md) |
| ~~NPM cache fix (CRITICAL-002)~~ ✅ Added npm cache to vulnerability-scan.yml | [Release & DevOps](docs/plans/release-packaging-and-devops.md) |
| ~~ghcommon pre-release & tagging strategy (CRITICAL-004)~~ ✅ Complete | [Release & DevOps](docs/plans/release-packaging-and-devops.md) |

---

## Metadata Sources

- [x] Hardcover.app integration
- [x] Google Books API key support (config, persistence, credentials map, env var)
- [x] Cover art: automatic download to local disk
- [x] Cover art: embed in audio file metadata tags (ffmpeg/metaflac, graceful fallback)

## Search Quality

- [x] Title cleaning: bracket stripping for "[Series] Title" format
- [x] Title cleaning: part/volume/chapter pattern removal
- [x] Title cleaning: subtitle stripping (colon, dash, em-dash separators)
- [x] Author-only fallback search with best-title-match scoring
- [x] Fuzzy search / Levenshtein distance matching
- [x] Search result ranking/scoring (0-100 fuzzy score, best match first)

## Infrastructure

- [x] PebbleDB format version logging on startup
- [x] PebbleDB v2 upgrade (v2.1.4, all imports migrated to v2 path)
- [x] Docker deployment (multi-stage build, docker-compose, Makefile targets)
- [x] launchd/systemd service files (macOS plist + Linux systemd unit with install scripts)
- [x] File watching / auto-scan for new audiobooks (fsnotify watcher with debounce)

## Data Quality

- [x] Metadata undo/history (MetadataChangeRecord, UI component)
- [x] Directory-as-filepath bug in tag extraction — fixed, falls back to filename parsing
- [x] Basic auth for web UI (constant-time compare, health/static exemptions)

---

## 🔥 P0 — UI & Metadata Overhaul (February 2026)

See [UI & Metadata Overhaul Design](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) for full details.

### Phase 1A: Fix Multiple Authors & AI Parse

| Item | Status | Plan |
| --- | --- | --- |
| Migration 22: backfill `book_authors`/`book_narrators` from `&`-delimited names | 🟢 | [Phase 1A](docs/plans/2026-02-28-phase1a-fix-multiple-authors.md) |
| Route AI parse through AudiobookService (multi-author split, history, narrator join) | 🟢 | [Phase 1A](docs/plans/2026-02-28-phase1a-fix-multiple-authors.md) |
| AI parse sends full folder path + existing metadata + file count (not just filename) | 🟢 | [Phase 1A](docs/plans/2026-02-28-phase1a-fix-multiple-authors.md) |

### Phase 1B: Fix Fetch Metadata Matching

| Item | Status | Plan |
| --- | --- | --- |
| Penalize box sets/collections/omnibus in result scoring | 🟢 | [Phase 1B](docs/plans/2026-02-28-phase1b-fix-fetch-metadata-matching.md) |
| Precision+recall F1 scoring instead of word-overlap only | 🟢 | [Phase 1B](docs/plans/2026-02-28-phase1b-fix-fetch-metadata-matching.md) |
| Series position filter (reject mismatched positions) | 🟢 | [Phase 1B](docs/plans/2026-02-28-phase1b-fix-fetch-metadata-matching.md) |
| Minimum quality threshold (reject scores below 0.35) | 🟢 | [Phase 1B](docs/plans/2026-02-28-phase1b-fix-fetch-metadata-matching.md) |
| Rich metadata bonus (prefer results with description, cover, narrator) | 🟢 | [Phase 1B](docs/plans/2026-02-28-phase1b-fix-fetch-metadata-matching.md) |

### Phase 1C: Fix History & Timestamps

| Item | Status | Plan |
| --- | --- | --- |
| `metadata_updated_at` column — only changes on metadata edits | 🟢 | [Phase 1C](docs/plans/2026-02-28-phase1c-fix-history-and-timestamps.md) |
| `last_written_at` column — set when files are written | 🟢 | [Phase 1C](docs/plans/2026-02-28-phase1c-fix-history-and-timestamps.md) |
| Change detection in `UpdateBook` (compare old vs new before updating timestamps) | 🟢 | [Phase 1C](docs/plans/2026-02-28-phase1c-fix-history-and-timestamps.md) |
| Field extractor loop records history entries for all manual edits | 🟢 | [Phase 1C](docs/plans/2026-02-28-phase1c-fix-history-and-timestamps.md) |

### Phase 2: Save to Files

| Item | Status | Plan |
| --- | --- | --- |
| `POST /api/v1/audiobooks/:id/write-back` endpoint | 🟢 | [Phase 2](docs/plans/2026-02-28-phase2-save-to-files-button.md) |
| Track number support in WriteMetadataToFile (M4B, MP3, FLAC) | 🟢 | [Phase 2](docs/plans/2026-02-28-phase2-save-to-files-button.md) |
| Per-segment numbering: `001 - Title.mp3`, track X/Y tags | 🟢 | [Phase 2](docs/plans/2026-02-28-phase2-save-to-files-button.md) |
| "Save to Files" button with confirmation dialog | 🟢 | [Phase 2](docs/plans/2026-02-28-phase2-save-to-files-button.md) |

### Phase 3: Multi-file Tab Layout

| Item | Status | Plan |
| --- | --- | --- |
| `GET /api/v1/audiobooks/:id/segments/:segmentId/tags` endpoint | 🟢 | [Phase 3](docs/plans/2026-02-28-phase3-multifile-tab-layout.md) |
| FileSelector component (chips for ≤20 files, dropdown for >20) | 🟢 | [Phase 3](docs/plans/2026-02-28-phase3-multifile-tab-layout.md) |
| Scoped Info/Tags/Compare tabs for selected file | 🟢 | [Phase 3](docs/plans/2026-02-28-phase3-multifile-tab-layout.md) |
| Fix Tags tab: show actual embedded media info (codec, bitrate, etc.) for single-file books | 🟢 | [Phase 3](docs/plans/2026-02-28-phase3-multifile-tab-layout.md) |

### Phase 4B: Manual Metadata Matching UI

| Item | Status | Plan |
| --- | --- | --- |
| Show top 10 scored results to user, let them pick or search | 🟢 | [Phase 4B](docs/plans/2026-02-28-phase4b-manual-metadata-matching.md) |
| Search from UI (title/author/ISBN) | 🟢 | [Phase 4B](docs/plans/2026-02-28-phase4b-manual-metadata-matching.md) |
| "No match" option that marks book as manually reviewed | 🟢 | [Phase 4B](docs/plans/2026-02-28-phase4b-manual-metadata-matching.md) |
| Field-level apply (Advanced mode with checkboxes) | 🟢 | [Phase 4B](docs/plans/2026-02-28-phase4b-manual-metadata-matching.md) |

### Phase 4: Multi-file Metadata Read

| Item | Status | Plan |
| --- | --- | --- |
| Folder path parser (author/series/title/narrator from directory hierarchy) | 🟢 | [Phase 4](docs/plans/2026-02-28-phase4-multifile-metadata-read.md) |
| Combined metadata assembly (folder + first file tags + filename pattern) | 🟢 | [Phase 4](docs/plans/2026-02-28-phase4-multifile-metadata-read.md) |
| Scanner integration for generic part-numbered files | 🟢 | [Phase 4](docs/plans/2026-02-28-phase4-multifile-metadata-read.md) |

---

## 🐛 Active Bugs

| Bug | Status | Plan |
| --- | --- | --- |
| ~~Directory-as-filepath in tag extraction (metadata.go:105)~~ | ✅ Fixed 2026-02-26 | [Database & Data Quality](docs/plans/database-and-data-quality.md) |
| ~~Search bar broken: after no results, typing new text and hitting Enter doesn't re-search~~ | ✅ Fixed 2026-03-01 | SearchBar was hidden by empty state |
| ~~BookDetail UX overhaul: button bar, merged tabs, mismatch readability, resolve buttons~~ | ✅ Fixed 2026-03-01 | [Design](docs/plans/2026-03-01-bookdetail-ux-overhaul-design.md) |
| ~~Author display showing author_id instead of name~~ | ✅ Fixed 2026-03-01 | Removed author_id from fallback chain |
| ~~Cover art not reloading after metadata fetch~~ | ✅ Fixed 2026-03-01 | Reset coverError on cover_url change |
| ~~Metadata change history not recording for manual edits~~ | ✅ Fixed 2026-03-01 | Added recordChangeHistory in updateAudiobook |
| ~~App.test.tsx missing getAppVersion mock~~ | ✅ Fixed 2026-03-01 | Pre-existing test gap |

Previously fixed:

| Bug | Status | Plan |
| --- | --- | --- |
| ~~Import path `total_size` returning negative values~~ | ✅ Fixed 2026-02-01 | [Database & Data Quality](docs/plans/database-and-data-quality.md) |
| ~~Corrupted organize paths with unresolved placeholders~~ | ✅ Fixed 2026-02-01 | [Library Org & Transcoding](docs/plans/library-organization-and-transcoding.md) |

---

## 🔮 Post-MVP — Feature Backlog

### iTunes Feature Parity — Metadata

| Item | Plan |
| --- | --- |
| Genre/category taxonomy | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Rating (1-5 stars) | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Copyright field | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Explicit/clean flag | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Chapter marks display (M4B/MP4) | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Per-chapter artwork | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Grouping field (link related books beyond series) | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Sort fields (sort-title, sort-author, sort-narrator) | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Comments/notes field | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |

### iTunes Feature Parity — Library Management

| Item | Plan |
| --- | --- |
| Smart collections / saved filters | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Bulk metadata editing (multi-select) | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Duplicate detection | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Missing file detection | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Storage usage dashboard | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Column-customizable list view (iTunes-style sortable table) | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Keyboard navigation (arrow keys, spacebar) | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Import/export library metadata (backup/restore without files) | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Mark as read/unread status | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Reading progress tracking | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Improved cover art display/editing | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Search with filters/facets | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |
| Sorting with more fields and saved orders | [Overhaul](docs/plans/2026-02-28-ui-metadata-overhaul-design.md) |

### Other

| Item | Plan |
| --- | --- |
| Anthology detection & review queue | [Anthology Handling](docs/plans/2026-01-31-anthology-handling-design.md) |
| ~~iTunes ITL binary read/write (Go port of titl)~~ ✅ Parser, location updater, playlists, track insertion | [iTunes Integration](docs/plans/itunes-integration.md) |
| ~~iTunes ITL write-back in organize workflow~~ ✅ Auto-updates .itl after file moves, with backup/validate/restore | [iTunes Integration](docs/plans/itunes-integration.md) |
| iTunes bidirectional sync (playcount management + sync) | [iTunes Integration](docs/plans/itunes-integration.md) |
| Release group & provenance tracking | [Metadata System](docs/plans/metadata-system.md) |
| Download client integration (Deluge, SABnzbd, qBittorrent) | [Download Clients](docs/plans/download-client-integration.md) |
| Advanced naming templates (Sonarr-style) | [Library Org & Transcoding](docs/plans/library-organization-and-transcoding.md) |
| **MP3→chapterized M4B conversion (CRITICAL post-launch)** | [Library Org & Transcoding](docs/plans/library-organization-and-transcoding.md) |
| Audio transcoding (general, chapters, cover art) | [Library Org & Transcoding](docs/plans/library-organization-and-transcoding.md) |
| Web download & export | [Library Org & Transcoding](docs/plans/library-organization-and-transcoding.md) |
| Security hardening (CSP, path traversal, audit log) | [Security & Multi-User](docs/plans/security-and-multiuser.md) |
| Multi-user architecture (auth, RBAC, SSL/TLS) | [Security & Multi-User](docs/plans/security-and-multiuser.md) |
| API enhancements (PATCH, webhooks, rate limiting, ETag) | [API & Integrations](docs/plans/api-and-ecosystem-integrations.md) |
| Ecosystem integrations (Calibre, OPDS, Plex/Jellyfin) | [API & Integrations](docs/plans/api-and-ecosystem-integrations.md) |
| Database quality (dedup, orphan detection, full-text search) | [Database & Data Quality](docs/plans/database-and-data-quality.md) |
| Backup enhancements (incremental, scheduled, integrity) | [Database & Data Quality](docs/plans/database-and-data-quality.md) |
| Observability (metrics endpoint, health checks, error aggregation) | [Observability](docs/plans/observability-and-monitoring.md) |
| Performance & reliability (resume scans, circuit breakers, retry) | [Performance & Reliability](docs/plans/performance-and-reliability.md) |
| Frontend components (timeline, quality chart, folder tree, log tail) | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| Accessibility (screen readers, high contrast, focus management) | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| Internationalization (i18n) | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| Mobile / PWA | [Frontend & UX](docs/plans/frontend-ux-and-accessibility.md) |
| Docker multi-arch, Helm chart, binary distribution | [Release & DevOps](docs/plans/release-packaging-and-devops.md) |
| Anthology configurable queue behavior (very low priority) | [Anthology Handling](docs/plans/2026-01-31-anthology-handling-design.md) |

---

## 🤖 CI/CD Workflow Actionization

Managed via background agent queue. See [Release & DevOps](docs/plans/release-packaging-and-devops.md) for details.

1. Plan security workflow actionization
2. Audit remaining workflows for action conversion
3. Validate new composite actions CI/CD pipelines
4. Verify action tags and releases
5. Update reusable workflows to use new actions
