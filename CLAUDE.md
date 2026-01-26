<!-- file: CLAUDE.md -->
<!-- version: 3.0.0 -->
<!-- guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f -->
<!-- last-edited: 2026-01-25 -->

# CLAUDE.md

> **NOTE:** This file is a pointer. All Claude/AI agent and workflow
> instructions are centralized in the `.github/instructions/` and
> `.github/prompts/` directories.

## 🎯 Quick Reference

**Main Documentation:**

- [Copilot Instructions](.github/copilot-instructions.md) - Primary AI agent
  configuration
- [Instructions Directory](.github/instructions/) - All coding standards and
  language-specific rules
- [Prompts Directory](.github/prompts/) - Specialized prompts for specific tasks

**For complete list of all instruction files, see [AGENTS.md](AGENTS.md)**

## 🚨 CRITICAL: Documentation Update Protocol

This repository uses direct-edit documentation workflow:

- Edit documentation directly in target files
- Always update version headers when making changes
- Do not use legacy doc-update scripts (create-doc-update.sh,
  doc_update_manager.py)
- Follow semantic versioning for version numbers

## 🔧 Git Operations Policy

**Preferred order for git operations:**

1. **MCP GitHub tools** (preferred) - Use when available
2. **safe-ai-util** (fallback) - Provides safety checks and logging
3. **Native git** (last resort) - Use only when other options unavailable

**Use VS Code tasks for non-git operations only** (build, lint, test, generate).

## 📋 Key Instruction Categories

### Workflow & Process

- Commit messages (conventional commits format)
- Pull request descriptions
- Code review guidelines
- Test generation standards
- Security best practices

### Language-Specific Rules

- Go, Python, TypeScript, JavaScript, Rust, Shell
- Protobuf, Markdown, JSON, HTML/CSS
- GitHub Actions workflows

### Specialized Prompts

- Code review, documentation generation
- Bug reports, feature requests
- Merge conflict resolution
- Test generation

> For all Claude, Copilot, or workflow tasks, **refer to the files in
> `.github/instructions/` and `.github/prompts/`**.
