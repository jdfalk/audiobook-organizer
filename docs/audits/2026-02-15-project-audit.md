<!-- file: docs/audits/2026-02-15-project-audit.md -->
<!-- version: 1.0.0 -->
<!-- guid: d4e5f6a7-b8c9-0123-4567-890abcdef012 -->

# Project Audit — February 15, 2026

**Go coverage**: 81.3% | **E2E**: 134/134 | **Frontend**: 23/23 | **MVP**: ~93%

---

## What Works (Ready for Daily Use)

| Area | Status | Coverage | Notes |
|------|--------|----------|-------|
| Scanner | 100% | 81.1% | Multi-format (M4B, MP3, FLAC, etc.), parallel workers, hash dedup |
| Organizer | 100% | 85.5% | Copy/hardlink/reflink/symlink strategies, configurable naming patterns |
| Database | 95% | 81.2% | SQLite, 14 migrations, full schema |
| Metadata fetch | 80% | 88.2% | OpenLibrary integration, provenance tracking |
| Backup/Restore | 100% | 80.6% | Tar+gzip, checksums, auto-cleanup |
| Config system | 95% | 90.1% | 100+ fields, JSON persistence, secret masking |
| Operations queue | 95% | 90.6% | Async background jobs, SSE progress |
| API | 95% | 79.8% | 71 endpoints, pagination, search |
| Frontend | 80% | — | 8 pages: Dashboard, Library, BookDetail, Settings, etc. |
| Build system | 100% | — | `make build/run/test/ci` all work |
| CLI | 90% | — | serve, scan, process, migrate, version |
| iTunes parsing | 100% | 86.5% | Plist parser, track classification |
| Download clients | Interfaces only | 100% | Deluge/qBit/SABnzbd stubs, no real integration |

## What's Broken or Incomplete

### P0 — Blocks MVP

| Issue | Detail |
|-------|--------|
| ~~Test coverage 80%~~ | ✅ Done (81.3%) |
| Manual QA | No documented verification run of all workflows |
| Release pipeline | GoReleaser token permissions, changelog generation |

### P1 — Needed for Real Use

| Issue | Detail |
|-------|--------|
| No authentication | Zero auth. Anyone on network can access everything |
| No deployment packaging | No Dockerfile, no docker-compose, no systemd service |
| iTunes UI incomplete | Settings tab is a stub (parsing/API work, UI doesn't) |
| Welcome wizard | Component exists but config persistence is partial |
| Single author only | Can't store multiple authors/narrators per book |
| OpenAPI spec | Only 5 of 71 endpoints documented |

### P2 — Nice to Have

| Issue | Detail |
|-------|--------|
| No dark mode | P2 backlog |
| No keyboard shortcuts | P2 backlog |
| No caching layer | Every query hits SQLite directly |
| No full-text search | Uses LIKE queries |
| No log viewing in UI | Stdout only |
| No resume for interrupted scans | Must restart from beginning |

## What's Missing for "Daily Driver"

For **single-user personal use** (you, on your machine):

1. **Deployment** — Need at minimum a way to run as a background service
   - Docker compose (easiest) or launchd plist (macOS)
   - Auto-start on boot

2. **File watching** — Currently must manually trigger scans
   - fsnotify or polling on import directories
   - Auto-scan when new files appear

3. **Notifications** — No way to know when operations complete unless watching UI
   - Desktop notifications or webhook support

4. **Cover art** — Grid view shows no book covers
   - OpenLibrary has cover URLs, but they're not displayed

5. **Bulk import workflow** — Drop a folder of audiobooks and have it "just work"
   - Scan → deduplicate → fetch metadata → organize → done

## Priority Path to Daily Driver

### Phase 1: Make It Runnable (1-2 days)
- [ ] Create Dockerfile + docker-compose.yml
- [ ] Or: create macOS launchd plist for background service
- [ ] Verify `make build && ./audiobook-organizer serve` works end-to-end with real library

### Phase 2: Make It Useful (3-5 days)
- [ ] Test with your real audiobook collection
- [ ] Fix any bugs found during real-world testing
- [ ] Add auto-scan on import directory (fsnotify watcher)
- [ ] Wire up cover art display in library grid

### Phase 3: Make It Reliable (1 week)
- [ ] Add basic auth (even just API key or basic HTTP auth)
- [ ] Scheduled backup (cron or internal timer)
- [ ] Complete manual QA checklist
- [ ] Fix release pipeline for binary distribution

### Phase 4: Polish (ongoing)
- [ ] Dark mode
- [ ] Multiple authors/narrators
- [ ] OpenAPI spec completion
- [ ] Desktop notifications
- [ ] Performance tuning for large libraries (10k+ books)

## Key Architecture Notes

- **Global state**: `database.GlobalStore`, `operations.GlobalQueue`, `realtime.GlobalHub`, `config.AppConfig`
- **Frontend embed**: Go binary embeds React via `//go:embed web/dist` (build tag `embed_frontend`)
- **API shape**: `GET /api/v1/audiobooks` → `{ count, items, limit, offset }`
- **Frontend expects**: `data.items` from API responses
- **iTunes XML**: Uses `itunes.EncodeLocation()` for file:// URL encoding
- **Async ops**: POST returns `{ id: "op-uuid" }`, poll via SSE or GET `/api/v1/operations/{id}`

## File Counts

```
Go:         26,000+ lines, 26 packages
TypeScript: 8,000+ lines, 15+ components
Tests:      15,000+ lines (200+ unit, 38 integration, 134 E2E)
Endpoints:  71 HTTP routes
Migrations: 14 database migrations
```
