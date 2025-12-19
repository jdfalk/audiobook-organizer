<!-- file: .github/instructions/general-coding.instructions.md -->
<!-- version: 2.2.0 -->
<!-- guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d -->
<!-- DO NOT EDIT: This file is managed centrally in ghcommon repository -->
<!-- To update: Create an issue/PR in jdfalk/ghcommon -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**"
description: |
  General coding, documentation, and workflow rules for all Copilot/AI agents and VS Code Copilot customization. These rules apply to all files and languages unless overridden by a more specific instructions file. For details, see the main documentation in `.github/copilot-instructions.md`.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# General Coding Instructions

These instructions are the canonical source for all Copilot/AI agent coding,
documentation, and workflow rules in this repository. They are referenced by
language- and task-specific instructions, and are always included by default in
Copilot customization.

- Follow conventional commit message standards and pull request guidelines.
- All language/framework-specific style and workflow rules are now found in
  `.github/instructions/*.instructions.md` files. These are the only canonical
  source for code style, documentation, and workflow rules for each language or
  framework.
- Document all code, classes, functions, and tests extensively, using the
  appropriate style for the language.
- Use the Arrange-Act-Assert pattern for tests.
- Do not duplicate rules; reference this file from more specific instructions.
- For VS Code Copilot customization, this file is included via symlink in
  `.vscode/copilot/`.
- **ALWAYS check before doing:** Before creating files, running operations, or executing scripts, always check current state first. Make all scripts and operations idempotent by checking if the desired state already exists before making changes.
- **USE VS CODE TASKS FIRST:** ALWAYS use VS Code tasks when available instead of manual terminal commands. Tasks provide consistent logging, error handling, and automation. Only fall back to manual commands when no appropriate task exists.
- **🚨 CRITICAL: Always increment file header versions when modifying any file.** When you change any file with a version header, you MUST update the version number according to semantic versioning rules (patch for fixes/typos, minor for features/additions, major for breaking changes). This is mandatory for all files including documentation, templates, workflows, and configuration files.

For more details and the full system, see
[copilot-instructions.md](../copilot-instructions.md).

## Copilot Agent Utility

The repositories use the `copilot-agent-util` command-line tool for enhanced logging, error handling, and automation in VS Code tasks. This Rust-based utility provides consistent output formatting and task management across all repositories.

**If the `copilot-agent-util` tool is not available in your environment:**

- Install it from the source repository: <https://github.com/jdfalk/copilot-agent-util-rust>
- Follow the installation instructions in that repository's README
- The tool is required for proper VS Code task execution and logging

## 🚨 CRITICAL: Use MCP GitHub Tools for Git Operations

**MANDATORY RULE: Always use MCP GitHub tools for git operations instead of manual commands or VS Code tasks.**

When performing git operations (commit, push, pull, file operations), use the MCP GitHub tools:

- `mcp_gitkraken_git_add_or_commit` - Add and commit files
- `mcp_gitkraken_git_push` - Push commits to remote
- `mcp_github_create_or_update_file` - Create or update files in GitHub
- `mcp_github_push_files` - Push multiple files in a single commit

**Benefits:**
- Direct GitHub integration without shell commands
- Consistent error handling and authentication
- Proper logging and audit trail
- Works across all environments

## 🚨 CRITICAL: Use VS Code Tasks First

**MANDATORY RULE: Always attempt to use VS Code tasks before manual commands (except for git operations - use MCP tools for those).**

When performing ANY operation (git, build, test, etc.), follow this priority:

1. **FIRST**: Check if a VS Code task exists for the operation
2. **SECOND**: Use the task via `run_task` tool with appropriate workspace folder
3. **THIRD**: Check task logs in `logs/` folder for results
4. **LAST RESORT**: Use manual terminal commands only if no task exists

### Common Operations and Their Tasks

| Operation | Task Name | Manual Fallback |
|-----------|-----------|-----------------|
| Git add all files | `Git Add All` | `git add .` |
| Git add specific files | `Git Add Selective` | `git add <pattern>` |
| Git commit | `Git Commit` | `git commit -m "message"` |
| Git push | `Git Push` | `git push` |
| Git status | `Git Status` | `git status` |
| Build Go project | `Go Build` | `go build` |
| Run Go tests | `Go Test` | `go test ./...` |
| Protocol buffer generation | `Buf Generate with Output` | `buf generate` |
| Python tests | `Python Test` | `python -m pytest` |
| Rust build | `Rust Build` | `cargo build` |
| Rust tests | `Rust Test` | `cargo test` |

### Task Usage Examples

```bash
# ✅ CORRECT: Use tasks first
run_task("Git Add All", "/path/to/workspace")
run_task("Git Commit", "/path/to/workspace")  # Will prompt for message
run_task("Git Push", "/path/to/workspace")

# ❌ INCORRECT: Manual commands without checking for tasks
git add . && git commit -m "message" && git push
```

### Benefits of Using Tasks

- **Consistent Logging**: All output logged to `logs/` folder with timestamps
- **Error Handling**: Standardized error reporting and debugging information
- **Workspace Awareness**: Tasks run in correct directory with proper context
- **Automation**: Tasks can chain together and include pre/post operations
- **Debugging**: Log files provide complete audit trail for troubleshooting

## Script Language Preference

**MANDATORY RULE: Prefer Python for scripts unless they are incredibly simple.**

When creating automation scripts, configuration tools, or data processing utilities:

1. **FIRST CHOICE**: Python for any script with:
   - API interactions (GitHub, REST APIs, etc.)
   - JSON/YAML processing
   - File manipulation beyond simple copying
   - Error handling and logging
   - Data parsing or transformation
   - More than 20-30 lines of logic

2. **SECOND CHOICE**: Shell scripts (bash/sh) only for:
   - Simple file operations (copy, move, basic checks)
   - Basic git commands
   - Simple environment setup
   - Scripts under 20 lines with minimal logic

3. **CONVERSION REQUIRED**: When existing shell scripts become complex:
   - Convert to Python when adding features
   - Rewrite in Python if error handling is insufficient
   - Migrate when API calls or JSON processing is needed

### Examples

**✅ CORRECT - Use Python for:**

- GitHub API interactions
- Configuration file processing
- Multi-step automation workflows
- Scripts with error handling requirements
- Data validation and transformation

**❌ INCORRECT - Don't use shell for:**

- Complex JSON parsing
- API authentication and error handling
- Multi-repository operations
- Scripts requiring robust error recovery

**✅ ACCEPTABLE - Shell scripts for:**

- Simple `cp`, `mv`, `mkdir` operations
- Basic git commands with minimal logic
- Environment variable setup
- Simple file existence checks

## Required File Header (File Identification)

All source, script, and documentation files MUST begin with a standard header
containing:

- The exact relative file path from the repository root (e.g.,
  `# file: path/to/file.py`)
- The file's semantic version (e.g., `# version: 1.1.0`)
- The file's GUID (e.g., `# guid: 123e4567-e89b-12d3-a456-426614174000`)

**Header format varies by language/file type:**

- **Markdown:**

  ```markdown
  <!-- file: path/to/file.md -->
  <!-- version: 1.1.0 -->
  <!-- guid: 123e4567-e89b-12d3-a456-426614174000 -->
  ```

- **Python:**

  ```python
  #!/usr/bin/env python3
  # file: path/to/file.py
  # version: 1.1.0
  # guid: 123e4567-e89b-12d3-a456-426614174000
  ```

- **Go:**

  ```go
  // file: path/to/file.go
  // version: 1.1.0
  // guid: 123e4567-e89b-12d3-a456-426614174000
  ```

- **JavaScript/TypeScript:**

  ```js
  // file: path/to/file.js
  // version: 1.1.0
  // guid: 123e4567-e89b-12d3-a456-426614174000
  ```

- **Shell (bash/sh):**

  ```bash
  #!/bin/bash
  # file: path/to/script.sh
  # version: 1.1.0
  # guid: 123e4567-e89b-12d3-a456-426614174000
  ```

  (Header must come after the shebang line)
- **CSS:**

  ```css
  /* file: path/to/file.css */
  /* version: 1.1.0 */
  /* guid: 123e4567-e89b-12d3-a456-426614174000 */
  ```

- **R:**

  ```r
  # file: path/to/file.R
  # version: 1.1.0
  # guid: 123e4567-e89b-12d3-a456-426614174000
  ```

  For executable R scripts:

  ```r
  #!/usr/bin/env Rscript
  # file: path/to/script.R
  # version: 1.1.0
  # guid: 123e4567-e89b-12d3-a456-426614174000
  ```

  (Header must come after the shebang line)
- **JSON:**

  ```jsonc
  // file: path/to/file.json
  // version: 1.1.0
  // guid: 123e4567-e89b-12d3-a456-426614174000
  ```

- **TOML:**

  ```toml
  [section]
  # file: path/to/file.toml
  # version: 1.1.0
  # guid: 123e4567-e89b-12d3-a456-426614174000
  ```

  (Header must be inside a section as TOML doesn't support top-level comments)

**All files must include this header in the correct format for their type.**

## Version Update Requirements

**When modifying any file with a version header, ALWAYS update the version
number:**

- **Patch version** (x.y.Z): Bug fixes, typos, minor formatting changes
- **Minor version** (x.Y.z): New features, significant content additions,
  template changes
- **Major version** (X.y.z): Breaking changes, structural overhauls, format
  changes

**Examples:**

- Fix typo: `1.2.3` → `1.2.4`
- Add new section: `1.2.3` → `1.3.0`
- Change template structure: `1.2.3` → `2.0.0`

**This applies to all files with version headers including documentation,
templates, and configuration files.**

## VS Code Tasks Implementation Details

All repositories are configured with standardized VS Code tasks following the priority system outlined above.

### Task Categories

- **Git Operations**: `Git Add All`, `Git Add Selective`, `Git Commit`, `Git Push`, `Git Status`
- **Build Operations**: Repository-specific tasks (e.g., `Go Build`, `Buf Generate`, `Python Test`)
- **Project Operations**: Language/framework-specific tasks

### Task Output and Logging

- **All task output is logged to the `logs/` folder** (gitignored)
- **Always check log files after running tasks** to verify success or diagnose issues
- Log files are named descriptively: `git_commit.log`, `go_build.log`, etc.
- Tasks include success/failure messages at the end of each log

### Task Execution Workflow

1. **Execute**: Use `run_task` tool with task name and workspace folder
2. **Verify**: Check the corresponding log file in `logs/` folder
3. **Debug**: Review log contents if task fails
4. **Retry**: Fix issues and re-run task if needed

This approach provides consistent logging, error handling, and automation across all repositories.
