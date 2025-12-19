<!-- file: .github/copilot-instructions.md -->
<!-- version: 2.3.2 -->
<!-- guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a -->

# GitHub Common Workflows Repository - AI Agent Instructions

This repository serves as the **central infrastructure hub** for reusable GitHub Actions workflows, scripts, and configurations across multiple repositories. It implements a sophisticated modular instruction system and provides automation tools for multi-repository management.

## 🏗️ Repository Architecture

**This is a workflow infrastructure repository**, not a typical application codebase. Key architectural components:

- **Reusable Workflows**: `.github/workflows/reusable-*.yml` - Called by other repositories
- **Script Library**: `scripts/` - Python automation tools for cross-repo operations
- **Instruction System**: `.github/instructions/` - Modular AI agent rules with language targeting
- **Workflow Debugging**: `scripts/workflow-debugger.py` - Analyzes failures and generates fix tasks
- **Multi-Repo Sync**: `scripts/intelligent_sync_to_repos.py` - Propagates changes to target repos

## 🔧 Critical AI Agent Workflows

Use VS Code tasks for non-git operations (build, lint, generate). For git operations, prefer:
1) MCP GitHub tools (preferred), 2) safe-ai-util (fallback), 3) native git (last resort).

Use specialized subagents when possible: CI Workflow Doctor, Dependency Auditor, Documentation Curator, Git Hygiene Guardian, Lint & Format Conductor, Protobuf Builder, Protobuf Cycle Resolver, and others in `.github/prompts/` for targeted expertise.

### Protobuf Operations (Core Focus)
```bash
# Use tasks, not manual buf commands
"Buf Generate with Output" - Generates protobuf code with logging
"Buf Lint with Output" - Lints protobuf files with comprehensive output
```
- This repo heavily focuses on protobuf tooling and cross-repo protobuf management
- Use `tools/protobuf-cycle-fixer.py` for import cycle resolution
- Protobuf changes trigger the `protobuf-generation.yml` workflow

### Git Operations (Policy)
- Prefer MCP GitHub tools or safe-ai-util for all git actions (add/commit/push).
- Avoid VS Code git tasks; keep git automation out of editor tasks.
- All commits MUST use conventional commit format: `type(scope): description`.
- See `.github/instructions/commit-messages.instructions.md` for detailed commit message rules.

## 🎯 Multi-Repository Management Patterns

**This repository manages configurations for multiple target repositories:**

### Sync Operations
```bash
# Primary sync script for propagating changes
python scripts/intelligent_sync_to_repos.py --target-repos "repo1,repo2" --dry-run
```
- Syncs `.github/instructions/`, `.github/prompts/`, and workflows to target repos
- Creates VS Code Copilot symlinks: `.vscode/copilot/` → `.github/instructions/`
- Handles repository-specific file exclusions and maintains file headers

### Workflow Debugging & Auto-Fix
```bash
python scripts/workflow-debugger.py --org jdfalk --scan-all --fix-tasks
```
- Analyzes workflow failures across repositories
- Generates JSON fix tasks for Copilot agents at `workflow-debug-output/fix-tasks/`
- Categorizes failures: permissions, dependencies, syntax, infrastructure
- Outputs actionable remediation steps with code examples

## 📁 File Organization Conventions

**Modular Instruction System** (referenced by general instructions):
- `general-coding.instructions.md` - Base rules for all languages
- `{language}.instructions.md` - Language-specific extensions with `applyTo: "**/*.{ext}"` frontmatter
- Instructions are synced to target repos and symlinked for VS Code Copilot integration

**Repository-Specific Patterns**:
- All files require versioned headers: `<!-- file: path -->`, `<!-- version: x.y.z -->`, `<!-- guid: uuid -->`
- Always increment version numbers on file changes (patch/minor/major semantic versioning)
- Use `copilot-util-args` file for storing command arguments between task executions

## 🔍 Project-Specific Context

**This is an infrastructure repository** - focus on:
1. **Workflow reliability** - Use workflow debugger to identify and fix cross-repo workflow issues
2. **Protobuf tooling** - Buf integration, cycle detection, and cross-repo protobuf synchronization
3. **Configuration propagation** - Ensure changes sync correctly to target repositories
4. **Agent task generation** - Workflow debugger creates structured tasks for AI agents

**Common Operations**:
- Analyze workflow failures: `scripts/workflow-debugger.py`
- Sync to repositories: `scripts/intelligent_sync_to_repos.py`
- Fix protobuf cycles: `tools/protobuf-cycle-fixer.py`Always check `logs/` directory after running VS Code tasks for execution details and debugging information.

For detailed coding rules, see `.github/instructions/general-coding.instructions.md` and language-specific instruction files.
