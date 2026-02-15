<!-- file: docs/audits/2026-02-15-project-audit.md -->
<!-- version: 2.0.0 -->
<!-- guid: d4e5f6a7-b8c9-0123-4567-890abcdef012 -->

# Project Audit — February 15, 2026 (Updated)

**Go coverage**: 81.3% | **E2E**: 134/134 | **Frontend**: 23/23 | **MVP**: ~97%

---

## What Works (Ready for Daily Use)

| Area | Status | Coverage | Notes |
|------|--------|----------|-------|
| Scanner | 100% | 81.1% | Multi-format (M4B, MP3, FLAC, etc.), parallel workers, hash dedup |
| Organizer | 100% | 85.5% | Copy/hardlink/reflink/symlink strategies, configurable naming patterns |
| Database | 100% | 81.2% | SQLite, 16 migrations, full schema incl. users/sessions/playback/stats |
| Metadata fetch | 80% | 88.2% | OpenLibrary integration, provenance tracking |
| Backup/Restore | 100% | 80.6% | Tar+gzip, checksums, auto-cleanup |
| Config system | 95% | 90.1% | 100+ fields, JSON persistence, secret masking |
| Operations queue | 95% | 90.6% | Async background jobs, SSE progress |
| API | 95% | 79.8% | 71 endpoints, pagination, search |
| Frontend | 85% | -- | 8 pages: Dashboard, Library, BookDetail, Settings, Welcome Wizard w/ iTunes |
| Build system | 100% | -- | `make build/run/test/ci` all work, embedded frontend verified |
| CLI | 90% | -- | serve, scan, process, migrate, version |
| iTunes parsing | 100% | 86.5% | Plist parser, track classification, wizard import step |
| Download clients | 100% | -- | Deluge (JSON-RPC), qBittorrent (Web API v2), SABnzbd (REST) fully implemented |
| Multi-author | 100% | -- | book_authors junction table, narrators JSON, cover_url field |
| File watching | 100% | -- | fsnotify watcher with debounce, audio-only filter |
| Cover art | 100% | -- | OpenLibrary fetch, proxy endpoint, local cache |
| User/session mgmt | 100% | -- | ULID IDs, JSON roles, TTL sessions, per-user prefs |
| Playback tracking | 100% | -- | Events, progress upsert, book/user stats |

## What's Been Done Since Last Audit

### Session 11 (this session)
- **Multi-author schema**: Migration 15 added `book_authors` junction table, `narrators_json`, `cover_url` columns
- **Cover art**: Proxy endpoint `/api/v1/covers/proxy`, local `.covers/` cache, OpenLibrary integration
- **Smart import flow**: iTunes import now does: import -> metadata fetch -> author dedup -> organize
- **Welcome wizard**: Added iTunes Library import step (4-step wizard: path, AI key, iTunes, folders)
- **Auto-scan**: fsnotify watcher service with 30s debounce, audio-only filter, SSE notifications
- **Docker**: Verified Dockerfile builds, created `deploy/audiobook-organizer.service` systemd unit
- **All stubs implemented**: 20 SQLite store methods (users, sessions, playback, stats, segments) + 18 download client methods (Deluge, qBittorrent, SABnzbd)

### Key Commits
- `bc5c453` feat(wizard): add iTunes Library import step to welcome wizard
- `bde760a` feat(database): implement user, session, playback, and stats SQLite methods
- `6eac9aa` feat(download): implement Deluge, qBittorrent, and SABnzbd clients
- Plus: multi-author schema, cover art, auto-scan, Docker (from earlier in session)

## What's Broken or Incomplete

### P0 -- Blocks Release

| Issue | Detail |
|-------|--------|
| No authentication | Zero auth. Anyone on network can access everything |
| Exposed API key | `.env` with real OpenAI key committed to git history |
| Manual QA | No documented verification run of all workflows |
| Dockerfile.test Go version | Uses Go 1.23, should be 1.25 |

### P1 -- Needed for Real Use

| Issue | Detail |
|-------|--------|
| OpenAPI spec | Only 5 of 71 endpoints documented |
| Stub frontend pages | Works page and Login page are non-functional stubs |
| Dark mode toggle | ThemeContext exists but no UI toggle |
| Config validation | No validation for port ranges, path existence, etc. |
| Error handling gaps | Fatal errors on HTTPS cert issues, silent failures in some paths |
| Coverage threshold mismatch | CI=0, Makefile=80%, repo-config=60% |
| Node version mismatch | Dockerfile uses node:25, everything else uses 22 |

### P2 -- Nice to Have

| Issue | Detail |
|-------|--------|
| No keyboard shortcuts | P2 backlog |
| No caching layer | Every query hits SQLite directly |
| No full-text search | Uses LIKE queries |
| No log viewing in UI | Stdout only |
| No resume for interrupted scans | Must restart from beginning |
| No docker-compose.yml | Single-command deployment not available |
| Hardcoded QuotaTab data | Shows sample data, not real storage info |

## Key Architecture Notes

- **Global state**: `database.GlobalStore`, `operations.GlobalQueue`, `realtime.GlobalHub`, `config.AppConfig`
- **Frontend embed**: Go binary embeds React via `//go:embed web/dist` (build tag `embed_frontend`)
- **API shape**: `GET /api/v1/audiobooks` -> `{ count, items, limit, offset }`
- **Frontend expects**: `data.items` from API responses
- **iTunes XML**: Uses `itunes.EncodeLocation()` for file:// URL encoding
- **Async ops**: POST returns `{ id: "op-uuid" }`, poll via SSE or GET `/api/v1/operations/{id}`
- **Multi-author**: `book_authors` junction table with role + position; primary = position 0
- **Playback**: Upsert pattern via INSERT ON CONFLICT DO UPDATE
- **Download clients**: JSON-RPC (Deluge), REST+cookies (qBit), REST+apikey (SABnzbd)

## File Counts

```
Go:         30,000+ lines, 28 packages
TypeScript: 8,500+ lines, 15+ components
Tests:      17,000+ lines (220+ unit, 38 integration, 134 E2E)
Endpoints:  71 HTTP routes
Migrations: 16 database migrations
```
