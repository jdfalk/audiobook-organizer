<!-- file: .github/README.md -->

# Copilot/AI Agent Coding Instructions System

This repository uses a centralized, modular system for Copilot/AI agent coding,
documentation, and workflow instructions, following the latest VS Code Copilot
customization best practices.

## Key Locations

- **General rules**: `.github/instructions/general-coding.instructions.md`
  (applies to all files)
- **Language/task-specific rules**: `.github/instructions/*.instructions.md`
  (with `applyTo` frontmatter)
- **Prompt files**: `.github/prompts/` (for Copilot/AI prompt customization)
- **Agent-specific docs**: `.github/AGENTS.md`, `.github/CLAUDE.md`, etc.
  (pointers to this system)
- **VS Code integration**: `.vscode/copilot/` contains symlinks to canonical
  `.github/` files for Copilot features

## How to Contribute

- Edit or add rules in `.github/instructions/`.
- Add new prompts in `.github/prompts/`.
- Update agent docs to reference this system.
- Do not duplicate rules; always reference the general instructions from
  specific ones.

## For More Details

See [copilot-instructions.md](copilot-instructions.md) for a full system
overview and contributor guide.
