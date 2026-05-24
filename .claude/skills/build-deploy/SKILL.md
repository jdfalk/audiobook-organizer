---
name: build-deploy
description: Build and deploy the audiobook-organizer using Makefile targets. Handles full builds (frontend + backend), API-only builds, test runs, and deployment. Always uses `make` targets instead of manual commands; auto-installs frontend dependencies before builds. Use when asked to build, run, test, deploy, or for frontend dev server.
---

# Build & Deploy

Wraps the Makefile build system, ensuring consistent builds and preventing common mistakes (forgetting frontend location, missing npm install, using wrong targets).

## Quick Start

Common commands:

```bash
make build          # Full build: frontend + Go backend (embedded UI)
make build-api      # Backend only (faster iteration)
make run            # Full build then serve (http://localhost:8080)
make run-api        # Backend-only build then serve
make test-short     # Quick tests (skip slow prop tests)
make test-all       # All tests (backend full + frontend)
make web-dev        # Vite dev server (http://localhost:5173)
make deploy         # Full build + deploy to production
make deploy-debug   # Build with debug symbols then deploy
```

## Key Points

**Frontend location:** `repo_root/web/` (Vite project)

**Dependencies:** `make build` and `make web-build` automatically run `npm install` in `web/` before building. This ensures:
- No "missing dependency" surprises
- Minor version updates don't break the build
- Fast path: `npm install` exits instantly if package.json hasn't changed

**Embedded UI:** Default `make build` embeds the React frontend into the Go binary (tag `embed_frontend`). This creates a single self-contained binary served on `:8080`. For frontend development, use `make web-dev` (separate Vite server on `:5173`).

## Build Variants

| Target | Frontend | Backend | Speed | Use Case |
|--------|----------|---------|-------|----------|
| `make build` | ✓ Build | ✓ Embed | Slower | Deployment, distribution |
| `make build-api` | ✗ Skip | ✓ Build | Faster | Local API iteration |
| `make run` | ✓ Build | ✓ Run | — | One-step dev |
| `make run-api` | ✗ Skip | ✓ Run | — | Backend-only dev |
| `make web-dev` | ✓ Live | ✗ Skip | — | Frontend dev (Vite HMR) |

## Testing

```bash
make test           # Full backend tests (~15 min, includes property tests)
make test-short     # Fast backend tests (~1 min, skips property tests)
make test-all       # Backend (full) + frontend tests
make test-all-short # Backend (-short) + frontend tests
make test-frontend  # Frontend tests only
make test-e2e       # Playwright end-to-end tests
```

## Deployment

```bash
make deploy         # Build + deploy to production (requires Makefile.local)
make deploy-debug   # Build with debug symbols + deploy
```

Deploy targets are defined in `Makefile.local` (not in the main Makefile). They handle:
- Go build with versioning
- Frontend build
- Upload to production server
- Service restart

See [references/makefile-targets.md](references/makefile-targets.md) for full target list and options.

## Frontend Development

For React/TypeScript work, use the Vite dev server:

```bash
make web-dev
```

Then:
- Open http://localhost:5173
- Edit `web/src/**/*.tsx` → changes appear instantly (HMR)
- Backend API available at http://localhost:8080 (adjust in vite.config.ts if needed)

To test the embedded UI (after `make build`):

```bash
./audiobook-organizer
# Open http://localhost:8080
```

## When to Use This Skill

- `"Build the backend"` → `make build-api`
- `"Run the app"` → `make run` (or `make web-dev` if frontend-focused)
- `"Run tests"` → `make test-short` (or `make test-all` for full)
- `"Deploy"` → `make deploy`
- `"I need the frontend"` → `make web-dev` (live editing)
