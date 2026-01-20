<!-- file: .github/copilot-instructions.md -->
<!-- version: 2.5.0 -->
<!-- guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a -->
<!-- last-edited: 2026-01-18 -->

# Audiobook Organizer - AI Agent Instructions

This repository is a **full-stack web application** for managing and organizing audiobook collections. It combines a Go backend with a React frontend to provide a comprehensive audiobook management solution.

## 🎯 Communication Protocol

**Error Response Policy**: When errors occur or corrections are needed, skip apologies and respond with "Aye Aye Captain" followed immediately by the corrected solution. Time efficiency is critical—acknowledge, correct, and move forward without unnecessary preamble.

## 🏗️ Repository Architecture

**This is an audiobook management application** built with:

- **Backend (Go 1.25)**: REST API, file operations, metadata extraction, database management
- **Frontend (React + TypeScript)**: Material-UI web interface with real-time updates
- **Database**: SQLite for metadata, PebbleDB for key-value storage
- **Integration**: Open Library API, OpenAI parsing, SSE for real-time updates

### Key Directories

- `cmd/` - CLI and server entry points
- `internal/` - Go backend packages (server, scanner, database, metadata)
- `web/` - React frontend application
- `docs/` - Documentation and testing guides
- `.github/` - CI/CD workflows and configuration

## 🔧 Critical AI Agent Workflows

Use VS Code tasks for non-git operations (build, lint, generate). For git operations, prefer:

1) MCP GitHub tools (preferred), 2) safe-ai-util (fallback), 3) native git (last resort).

### Git Operations (Policy)

- Prefer MCP GitHub tools or safe-ai-util for all git actions (add/commit/push).
- Avoid VS Code git tasks; keep git automation out of editor tasks.
- All commits MUST use conventional commit format: `type(scope): description`.
- See `.github/instructions/commit-messages.instructions.md` for detailed commit message rules.

### Terminal Command Length Limits (CRITICAL)

**MANDATORY RULE: Long terminal commands WILL fail and die.**

Terminal commands with excessive length (either many arguments or very long single lines) will fail with exit code 130 or similar errors. Follow these rules:

**Maximum Safe Limits:**

- **For loops with paths**: No more than 5 paths/arguments
- **Single-line commands**: No more than ~200-300 characters
- **Multi-argument commands**: No more than 5-6 distinct arguments

**Example of TOO LONG (will fail):**

```bash
for pr_dir in /path/one /path/two /path/three /path/four /path/five /path/six /path/seven /path/eight /path/nine /path/ten; do ...
```

**Solution: Use a script in temp_crap repo:**

```bash
# Instead, create a script
cat > /Users/jdfalk/repos/temp_crap/my_script.sh << 'EOF'
#!/bin/bash
for pr_dir in /path/one /path/two /path/three ... /path/twenty; do
    # Command logic here
done
EOF
chmod +x /Users/jdfalk/repos/temp_crap/my_script.sh
/Users/jdfalk/repos/temp_crap/my_script.sh
```

**Why temp_crap:**

- Always available in the workspace
- No approval needed for file creation
- Can handle unlimited command complexity
- Python scripts preferred for anything beyond simple bash

**If you exceed these limits, you WILL break the terminal execution.**

**If you exceed these limits, you WILL break the terminal execution.**

## 📁 File Organization Conventions

**Repository Structure**:

- All files require versioned headers: `<!-- file: path -->`, `<!-- version: x.y.z -->`, `<!-- guid: uuid -->`, `<!-- last-edited: YYYY-MM-DD -->`
- Always increment version numbers on file changes (patch/minor/major semantic versioning)
- Update `last-edited` date whenever making changes

## 🔍 Project-Specific Context

**This is an audiobook management application** - focus on:

1. **Workflow reliability** - Use workflow debugger to identify and fix cross-repo workflow issues
2. **Protobuf tooling** - Buf integration, cycle detection, and cross-repo protobuf synchronization
3. **Configuration propagation** - Ensure changes sync correctly to target repositories
4. **Agent task generation** - Workflow debugger creates structured tasks for AI agents

**Common Operations**:

- Analyze workflow failures: `scripts/workflow-debugger.py`
- Sync to repositories: `scripts/intelligent_sync_to_repos.py`
- Fix protobuf cycles: `tools/protobuf-cycle-fixer.py`Always check `logs/` directory after running VS Code tasks for execution details and debugging information.

For detailed coding rules, see `.github/instructions/general-coding.instructions.md` and language-specific instruction files.
