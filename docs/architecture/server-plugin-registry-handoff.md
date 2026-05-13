<!-- file: docs/architecture/server-plugin-registry-handoff.md -->
<!-- version: 1.0.0 -->
<!-- last-edited: 2026-05-13 -->

# SERVER-PLUGIN-REG — Handoff Brief

> **Use this as the opening prompt for a fresh AI session continuing the
> plugin migration.** It is intentionally short — read the three companion
> docs only as you reach the tickets that need them.

## Repo context

Go + React audiobook organizer. The host (`internal/server`) historically
built every service in a 968-line `NewServer`. We're partway through a
migration to a per-instance service container (`internal/serviceregistry`)
where domain packages self-register via `init()` and `NewServer` just
asks the container to build them.

Production data lives on `172.16.2.30` (PebbleDB at
`/var/lib/audiobook-organizer/audiobooks.pebble`). **Always use
`make deploy` after merging server-side changes** — `Makefile.local`
builds, scp's, and restarts via systemd.

## State as of 2026-05-13

- All 7 waves of the original SERVER-PLUGIN-REG plan landed.
- ~39 services registered across 5 named groups: `core`, `ai`, `scheduler`,
  `plugins`, `activity`. Audit with
  `grep -rn 'Groups.*"core"' internal/ --include="*.go"`.
- `NewServer` is **462 lines** (target: ≤50). Down from 968 historic / 577
  at session start.
- 0 open PRs. `go vet ./...` clean. `staticcheck ./...` clean. `go build`
  clean.
- Pre-existing test failures to ignore: **SERVER-THIN-8** —
  `TestITunesImport_*`, `TestE2E_ITunesImportOrganizeWriteBack`,
  `TestOrganizeService_ViaHTTP`, `TestAddImportPathAutoScan`,
  `TestStartScanOperation`, `TestStartOrganizeOperation`. These timeout
  at 15s and have been broken on `main` since before the migration started.

## Key design decisions (don't relitigate)

1. **Event bus dispatch is async + panic-isolated + decoupled contexts.**
   Subscribers get the bus's lifecycle ctx, not the publisher's.
   `plugin.EventBus.Publish` already wraps each subscriber goroutine in
   `recover()`.
2. **NewServer uses explicit phases**: `Resolve → Build → PostInit` happen
   in `NewServer`; `Start/Stop` belong to `Server.Start`/`Server.Shutdown`.
3. **`database.GetGlobalStore` → `GetGlobalStoreForTest` rename** is the
   bridge state; full deletion (`TEST-GLOBAL-STORE-MIGRATION`) waits on
   `SERVER-GLOBAL-STORE-AUDIT`.
4. **`Container.IncludeGroup(names ...string)`** is the production
   inclusion API. Each `ServiceDef` has a `Groups []string` field. Explicit
   `Container.Include(name)` stays for tests and ad-hoc additions.

## Companion docs (in `docs/architecture/`)

- `server-plugin-registry-design.md` — original design rationale
- `server-plugin-registry-plan.md` — original 7-wave plan
- **`server-plugin-registry-deferred-work.md`** — the live work plan;
  read this before starting any ticket

## What's left

In dependency order. Estimated sizes assume one focused engineer.

### 1. PLUGIN-DECOUPLE-SERVER-CLOSURES (remaining work) — M
Partially done in PR #887 + #888. **Remaining**: full container
registration of `itunesservice.Service` so W3.1 (writebackbatcher),
W5.1 (maintenance plugin), W5.2 (itunes plugin) can be un-stubbed.

Blockers to address:
- `itunesservice.Deps.ActivityFn` — closure over `server.activityService`.
  Fix: register the closure in itunesservice's `register.go` Build using
  `serviceregistry.Get[*activity.Service](c, "activity")` (in container).
- `itunesservice.Deps.Realtime` — `*realtime.EventHub`. Needs `realtime`
  package register.go (small).
- `itunesservice.Deps.Logger` — `logger.Logger`. Needs a "logger" service
  Override or registration (tiny).
- `maintenance.ServerDeps` — multi-field struct of `*Server` references.
  Refactor to pull each field individually via container at Build time.

After this: delete inline `itunesservice.New(...)` from `NewServer` (~25
lines) and the inline `maintenanceplugin.New(server).Register(...)` block.

### 2. PROMOTE-STUB-REGISTRATIONS — S
Once #1 lands, change three stub Builds in
`internal/itunes/service/register.go`,
`internal/plugins/maintenance/register.go`,
`internal/plugins/itunes/register.go` to return real instances pulled
from the container.

### 3. SERVER-LIFECYCLE-FLIP (remaining) — L
Wire `Container.Start(ctx)` into `Server.Start` and `Container.Stop(ctx)`
into `Server.Shutdown`. **Per-service blockers** (each is its own small
PR):
- `updatescheduler` — register.go uses hard-coded `"dev"` version. Fix
  by adding an `Override("appversion", appVersion)` from the server.
- `searchindex` — Container's `Start` would open Bleve; conflicts with
  the inline open in `server_lifecycle.go`. Pick one path.
- `activitywriter` — inline `aw := activity.NewWriter(...)` in NewServer
  creates a parallel writer. Delete the inline; pull from container.

After this: delete the inline `server.updateScheduler.Start()` +
`s.opRegistry.Start(s.bgCtx)` + `s.opRegistry.Shutdown(regCtx)` calls.

### 4. W7.1 NEWSERVER-TRIM — M
Natural output of the above. Target: `NewServer ≤ 50 lines`. The
remaining ~410 lines after step #1 are inline plugin registration,
hook wiring (`scanner.SetScanHooks`, `organizeService.SetOrganizeHooks`,
maintenance.InjectStore, etc), and route registration. Move what's
movable into PostInit on the relevant services.

### 5. SERVER-GLOBAL-STORE-AUDIT — XL (parallel-safe)
~120 production callers of `database.GetGlobalStore()`. Migrate
per-package, one PR at a time. Order (smallest first):
1. `cmd/seed.go`, `cmd/dedup_bench.go`, `cmd/diagnostics.go`
2. `internal/itunes/rebuild.go`, `internal/backup/backup.go`,
   `internal/database/store.go`
3. `internal/server/server_*.go` (~30 across files)
4. `internal/metafetch/helpers.go`, `internal/organizer/organizer.go`
5. `internal/audiobooks/helpers.go`
6. `internal/scanner/scanner.go` (35 — biggest, save for last)

Pattern: change function signature to accept `store database.Store`
explicitly; propagate to callers. Tests keep working via the existing
`GetGlobalStore`/`SetGlobalStore` (rename to `*ForTest` after the audit
is done).

### 6. TEST-GLOBAL-STORE-MIGRATION — M
After #5: migrate the ~289 test sites from `SetGlobalStoreForTest(mock)`
to per-test container construction
(`serviceregistry.NewContainer().Override("store", mockStore)`). Then
delete the test globals entirely.

## How to ship work in this repo

```bash
# Branch + make changes
git checkout -b refactor/<ticket-slug>
# ... edits + tests ...
go vet ./...
go build ./...
go test ./internal/<affected>/... -short -race -timeout=60s

# Commit + push + PR + admin-merge
git push -u origin refactor/<ticket-slug>
gh pr create --base main --head refactor/<ticket-slug> --title "..." --body "..."
gh pr merge <N> --rebase --admin --delete-branch
git checkout main && git pull --ff-only origin main
make deploy  # if server-side changes; runs from primary checkout
```

This repo enforces **rebase/FF only** — never use `--squash`. The user
admin-merges with `--admin` regularly; you may do the same.

## User preferences (learned this session)

- **Terse > verbose.** Don't pad responses with insights/recaps unless
  asked. Short status updates between tool calls only.
- **No flattery or affirmations.** Skip "great choice!", "perfect!", etc.
- **Honest about deferred work.** Don't claim "feature complete" if real
  blockers remain — document the blocker, propose a path, move on.
- **Push forward.** When given a goal like "until done", keep working
  through tickets in dependency order. Don't pause for confirmation on
  each one.
- **PR descriptions earn their length.** Spell out per-file changes, why
  each is safe (often a grep that confirms zero callers), what was kept
  inline and why. The user reviews these.
- **Stub-registrations are documented bridges**, not failures. If a real
  blocker exists, note it in the file header + open a follow-up ticket.

## Gotchas

1. **Test-mode `MockStore` doesn't expect `UpsertOpDefinitionV2`.** If you
   move a `Plugin.Register(opRegistry)` into PostInit, make sure the
   plugin's Build returns nil when `cfg.RootDir == ""` (test path) —
   otherwise PostInit triggers the unexpected mock call. PR #888 has
   the canonical guard pattern.
2. **macOS `t.TempDir()` returns `/var/folders/...` which is a symlink
   to `/private/var/folders/...`.** Any path-validation that uses
   `filepath.EvalSymlinks` on one side and not the other will fail.
   `internal/security/pathvalidation/SecureJoinResolved` has the fix
   (PR #863) — reuse the `resolveExistingPrefix` pattern.
3. **`internal/database` cannot import `internal/config`** (cycle via
   `persistence.go`). Services that need both — like the AI cluster
   services that need `config.Config.OpenAIAPIKey` and want to live near
   `database` types — get registered from `internal/server/registry_wire.go`
   instead. See the `embeddingstore`/`chromemstore`/`aijobsstore`/`dedup`
   registrations there.
4. **A child agent in a `/parallel-sweep` can wreck a worktree** with
   `git add -A`. Every dispatch must spell out: "stage ONLY <explicit
   paths>; verify `git status --short` shows only those before commit."
   PR #851 was a 260-file disaster from this exact oversight.
5. **`make ci` runs full server tests** which include SERVER-THIN-8
   pre-existing failures. For per-PR validation,
   `go test ./internal/server/ -short -race -timeout=180s -run "TestHandler_|TestNewServer|TestRegister|TestServer"`
   is the green subset.

## Quick verification commands

```bash
# All-in-one health check
go vet ./... && go build ./... && staticcheck ./...

# Service registry sanity
go test ./internal/serviceregistry/... -count=1 -race -timeout=60s

# What's in each group right now
for g in core ai scheduler plugins activity; do
  echo "=== $g ==="
  grep -rn "Groups.*\"$g\"" internal/ --include="*.go"
done

# NewServer size (target: <=50)
awk '/^func NewServer/,/^}/' internal/server/server.go | wc -l

# GetGlobalStore audit (target: 0 production callers)
grep -rn "GetGlobalStore()" --include="*.go" internal/ cmd/ | grep -v "_test\." | wc -l
```

## Active goal

Continue through the deferred-work tickets in dependency order until the
migration is operationally complete: itunesservice + maintenance plugin
fully container-built, lifecycle flip done, NewServer trimmed, GlobalStore
audit complete. Pick up at PLUGIN-DECOUPLE-SERVER-CLOSURES (remaining
work) per the order above.
