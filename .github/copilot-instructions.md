<!-- file: .github/copilot-instructions.md -->
<!-- version: 4.0.0 -->
<!-- guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a -->
<!-- last-edited: 2026-06-12 -->

# Audiobook Organizer — Additional Copilot Context

Org-wide coding standards (file headers, Go/TS rules, commit format) are at
**https://github.com/falkcorp/.github** and apply automatically to this repo.

This file contains audiobook-organizer-specific additions only.
For full project context see **CLAUDE.md** at the repo root.

## Project overview

Go 1.24 backend (Gin) + React 18/TypeScript frontend (Material UI).
DB: PebbleDB (primary), NutsDB (activity log). SQLite removed Jun 2026.
Integration: Open Library, AcoustID fingerprinting, OpenAI batch API.

## Key directories

| Directory | Contents |
|---|---|
| `cmd/` | CLI and server entry points |
| `internal/` | Go backend packages |
| `web/src/` | React frontend |
| `docs/specs/` | Design specs |
| `docs/plans/` | Implementation plans |
| `.github/prompts/` | AI agent prompts |

## Critical constraints

- `UpdateBook` does **full column replacement** — always supply all fields.
- iTunes XML path must **never** be scanned by the file scanner.
- Production is Linux (`make deploy`). Do not use macOS-specific commands.
- Worktree discipline: never commit directly to `main`. All changes via PRs.
