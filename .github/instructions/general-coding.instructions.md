<!-- file: .github/instructions/general-coding.instructions.md -->
<!-- version: 3.0.0 -->
<!-- guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d -->
<!-- last-edited: 2026-01-31 -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**"
description: |
  Universal coding, documentation, and workflow rules for all AI agents. Applies to all files and languages unless overridden by a language-specific instructions file.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# General Coding Instructions

These are the universal rules. Language-specific files in `.github/instructions/` extend these.

## Git Operations

**Priority order:**
1. **MCP GitHub tools** (preferred) — `mcp_github_create_or_update_file`, `mcp_github_push_files`, etc.
2. **Native git** (fallback) — only when MCP tools aren't available.

All commits MUST use conventional commit format: `type(scope): description`.

## Script Language Preference

- **Python** for anything beyond trivial: API calls, JSON/YAML processing, error handling, 20+ lines of logic.
- **Shell** only for simple file ops, basic git commands, env setup, scripts under 20 lines.
- Convert shell → Python when scripts grow complex.

## Check Before Acting

Before creating files or running operations, check current state first. Make all operations idempotent.

## File Headers (MANDATORY)

All source, script, and documentation files MUST begin with a versioned header. Format varies by language:

- **Markdown:** `<!-- file: path -->` / `<!-- version: x.y.z -->` / `<!-- guid: uuid -->` / `<!-- last-edited: date -->`
- **Go/JS/TS:** `// file:` / `// version:` / `// guid:`
- **Python/Shell/R:** `# file:` / `# version:` / `# guid:` (after shebang if present)
- **CSS:** `/* file: */` / `/* version: */` / `/* guid: */`
- **JSON:** `.json` files are **exempt**. `.jsonc` files use `// file:` comments.
- **Protobuf:** `// file:` / `// version:` / `// guid:`

## Version Updates (MANDATORY)

When modifying any file with a version header, increment the version:

- **Patch** (x.y.Z): typos, minor fixes
- **Minor** (x.Y.z): new content, significant additions
- **Major** (X.y.z): structural changes, breaking changes

## .env Files

Keep only `KEY=VALUE` entries (no headers/comments). Store metadata with `JF_FILE_PATH`, `JF_FILE_VERSION`, `JF_FILE_GUID` keys.

## Testing

- Use Arrange-Act-Assert pattern.
- Table-driven tests where applicable.

## Documentation

- Document all code, classes, functions, and tests using the appropriate style for the language.
- Do not duplicate rules across files — reference this file from language-specific instructions.
