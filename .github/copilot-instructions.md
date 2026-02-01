<!-- file: .github/copilot-instructions.md -->
<!-- version: 3.0.0 -->
<!-- guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a -->
<!-- last-edited: 2026-01-31 -->

# Audiobook Organizer - AI Agent Instructions

Full-stack web application for managing and organizing audiobook collections.

## Repository Architecture

- **Backend (Go 1.25):** REST API, file operations, metadata extraction, database management
- **Frontend (React + TypeScript):** Material-UI web interface with real-time updates via SSE
- **Database:** SQLite for metadata, PebbleDB for key-value storage
- **Integration:** Open Library API, OpenAI parsing

### Key Directories

| Directory | Contents |
|-----------|----------|
| `cmd/` | CLI and server entry points |
| `internal/` | Go backend packages (server, scanner, database, metadata) |
| `web/` | React frontend application |
| `docs/` | Documentation |
| `.github/` | CI/CD workflows, instructions, prompts |

## Git Operations Policy

1. **MCP GitHub tools** (preferred) — use for all git operations when available.
2. **Native git** (fallback) — when MCP tools aren't available.

All commits MUST use conventional commit format: `type(scope): description`.
See `.github/instructions/commit-messages.instructions.md` for full rules.

## File Organization

All files require versioned headers: `<!-- file: path -->`, `<!-- version: x.y.z -->`, `<!-- guid: uuid -->`, `<!-- last-edited: YYYY-MM-DD -->`.

Always increment version numbers on changes (semantic versioning). Update `last-edited` date.

## Instruction Files

- **Coding standards:** `.github/instructions/*.instructions.md`
- **Specialized prompts:** `.github/prompts/*.prompt.md`
- **This file:** Primary context for repository architecture and workflow.

For detailed coding rules, see `.github/instructions/general-coding.instructions.md` and the language-specific instruction files.
