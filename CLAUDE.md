<!-- file: CLAUDE.md -->
<!-- version: 4.1.0 -->
<!-- guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f -->
<!-- last-edited: 2026-01-31 -->

# CLAUDE.md

This is an **audiobook organizer** — Go backend + React/TypeScript frontend.
All AI agent instructions live in `.github/`. This file is the entry point.

## Quick Start

- **Architecture & workflows:** [.github/copilot-instructions.md](.github/copilot-instructions.md)
- **Coding standards:** [.github/instructions/](https://github.com/jdfalk/audiobook-organizer/tree/main/.github/instructions/)
- **Prompts:** [.github/prompts/](https://github.com/jdfalk/audiobook-organizer/tree/main/.github/prompts/)
- **Full file index:** [AGENTS.md](AGENTS.md)

## Build & Test Commands

```bash
# Go backend
make test            # go test ./... -v -race
make ci              # test + 80% coverage check
make coverage        # HTML coverage report

# Web frontend (from web/ directory)
npm run dev          # Vite dev server
npm run build        # tsc + Vite build
npm run test         # Vitest unit tests
npm run lint         # ESLint
npm run test:e2e     # Playwright E2E
```

> **Note:** `go.mod` currently says `go 1.24.0`. The Go instructions reference 1.25 features — update go.mod when upgrading.

## Critical Rules

1. **Git:** Use MCP GitHub tools first. Native git as fallback. Conventional commits mandatory.
2. **File headers:** All files need versioned headers. Bump version on every change.
3. **Docs:** Edit files directly. Update version headers. No legacy doc-update scripts.
4. **Scripts:** Python for anything non-trivial. Shell only for simple ops under 20 lines.
