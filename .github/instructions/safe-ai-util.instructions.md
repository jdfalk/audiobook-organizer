<!-- file: .github/instructions/safe-ai-util.instructions.md -->
<!-- version: 2.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-1234-567890abcdef -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---

applyTo: "\*\*"
description: |
Instructions for using the safe-ai-util Rust utility as the primary tool for development operations. This utility provides superior performance, memory safety, and comprehensive command coverage compared to manual terminal commands. Note: The utility was previously named safe-ai-util and both binary names are supported for backward compatibility.

---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# Safe AI Utility (Rust) - Installation and Command Reference

## 📥 Installation

### Option 1: Download from GitHub Releases (Recommended)

**Download from:** https://github.com/jdfalk/safe-ai-util/releases

Available binaries for:
- **macOS**: `safe-ai-util-macos-arm64` and `safe-ai-util-macos-x86_64`
- **Linux**: `safe-ai-util-linux-arm64` and `safe-ai-util-linux-x86_64`
- **Windows**: `safe-ai-util-windows-x86_64.exe`

```bash
# Example installation on macOS ARM64:
curl -L -o safe-ai-util https://github.com/jdfalk/safe-ai-util/releases/latest/download/safe-ai-util-macos-arm64
chmod +x safe-ai-util
sudo mv safe-ai-util /usr/local/bin/

# For backward compatibility, the safe-ai-util binary name is also available:
# curl -L -o safe-ai-util https://github.com/jdfalk/safe-ai-util/releases/latest/download/safe-ai-util-macos-arm64
```

### Option 2: Build from Source

**Source code:** https://github.com/jdfalk/safe-ai-util

```bash
# Clone and build
git clone https://github.com/jdfalk/safe-ai-util
cd safe-ai-util
cargo build --release
cp target/release/safe-ai-util /usr/local/bin/

# Both binary names are available:
# cp target/release/safe-ai-util /usr/local/bin/
```

### Option 3: Use Local Copy (Development)

Many repositories include a copy in `tools/safe-ai-util/`:

```bash
cd tools/safe-ai-util
cargo build --release
cp target/release/safe-ai-util /usr/local/bin/
```

### Verification

```bash
# Verify installation
safe-ai-util --version

# For backward compatibility, safe-ai-util also works:
# safe-ai-util --version
```

The `safe-ai-util` is a comprehensive Rust-based development utility that provides superior performance, memory safety, and extensive command coverage. **Always prefer this utility over manual commands when available.**

> **Note:** The utility was previously named `safe-ai-util` and both binary names are supported for backward compatibility during the transition period.

## 🚀 Arguments File Support

The utility supports loading arguments from a standard configuration file called `copilot-util-args` or `safe-ai-util-args`. This allows for:

- **Consistent configuration** across all repositories
- **Easy updates** by modifying a single file
- **Environment-specific settings** without changing tasks
- **Complex argument sets** without cluttering VS Code tasks

### Arguments File Location

The utility looks for `safe-ai-util-args` (or `copilot-util-args` for compatibility) in the current working directory or any parent directory (similar to how git finds .git). The file format supports:

```bash
# safe-ai-util-args - Standard configuration file
# Comments are supported with #

# Git configuration
git.default-branch=main
git.auto-push=true
git.commit-template="feat: {message}"

# Editor settings
editor.syntax=auto-detect
editor.tab-width=4
editor.show-line-numbers=true

# Buf/protobuf settings
buf.output-dir=gen
buf.lint-config=.buf.yaml
buf.generate-docs=true

# Global settings
verbose=true
log-level=info
```

### Using Arguments Files

When a `safe-ai-util-args` (or `copilot-util-args`) file is present, the utility automatically loads these settings:

```bash
# The utility automatically finds and uses the args file
safe-ai-util git commit -m "implement feature"
# Uses git.commit-template and other git.* settings from args file

safe-ai-util buf generate
# Uses buf.* settings from args file

safe-ai-util editor file.rs
# Uses editor.* settings from args file
```

## 🚨 PRIORITY ORDER FOR OPERATIONS

**MANDATORY: Follow this exact priority when performing ANY operation:**

1. **FIRST**: Use VS Code tasks (via `run_task` tool) when available
2. **SECOND**: Use `safe-ai-util` Rust utility
3. **LAST RESORT**: Manual terminal commands only if neither above option exists

## Available Commands

### 🔧 Git Operations (Comprehensive)

The utility provides **complete git functionality** with 18+ subcommands and automatic configuration from `safe-ai-util-args`:

```bash
# Git command structure
safe-ai-util git <subcommand> [options] [args]

# Available git subcommands:
safe-ai-util git add [files...]           # Add files to staging
safe-ai-util git commit -m "message"      # Commit changes (uses templates from args file)
safe-ai-util git push                     # Push to remote (respects auto-push setting)
safe-ai-util git pull                     # Pull from remote
safe-ai-util git status                   # Show working tree status
safe-ai-util git branch [name]            # List/create branches
safe-ai-util git checkout <branch>        # Switch branches
safe-ai-util git merge <branch>           # Merge branches
safe-ai-util git rebase <branch>          # Rebase commits
safe-ai-util git reset [options]          # Reset state
safe-ai-util git log [options]            # Show commit history
safe-ai-util git diff [options]           # Show differences
safe-ai-util git stash [command]          # Stash changes
safe-ai-util git remote [command]         # Manage remotes
safe-ai-util git tag [options]            # Manage tags
safe-ai-util git clone <url>              # Clone repository
safe-ai-util git fetch                    # Fetch from remote
safe-ai-util git init                     # Initialize repository
```

**Configuration via copilot-util-args:**

```bash
# Example git configuration in copilot-util-args
git.default-branch=main
git.auto-push=false
git.commit-template="feat: {message}"
git.merge-strategy=no-ff
git.push-default=current
```

**Git Command Examples:**

```bash
# Status and basic operations
safe-ai-util git status
safe-ai-util git add .
safe-ai-util git commit -m "feat: add new feature"
safe-ai-util git push

# Branch operations
safe-ai-util git branch feature-branch
safe-ai-util git checkout feature-branch
safe-ai-util git merge main

# Advanced operations
safe-ai-util git log --oneline -10
safe-ai-util git diff HEAD~1
safe-ai-util git stash push -m "WIP changes"
```

### 📝 Text Processing

#### Sed Stream Editor

Superior Rust implementation of sed with full regex support and configuration from `copilot-util-args`:

```bash
# Sed command structure
safe-ai-util sed [options] [files...]

# Common sed operations:
echo "text" | safe-ai-util sed -e 's/old/new/g'        # Substitute
safe-ai-util sed -i -e 's/old/new/g' file.txt         # In-place edit
safe-ai-util sed -e '/pattern/d' file.txt             # Delete lines
safe-ai-util sed -e '3,5p' -n file.txt                # Print specific lines
```

**Configuration via copilot-util-args:**

```bash
# Example sed configuration in copilot-util-args
sed.backup-suffix=.bak
sed.extended-regexp=true
sed.case-insensitive=false
```

**Sed Options:**

- `-e, --expression <expression>`: Sed expression/script
- `-i, --in-place`: Edit files in place
- `--backup <SUFFIX>`: Backup suffix for in-place editing (overrides args file)
- `-n, --quiet`: Suppress automatic printing
- `-r, --extended-regexp`: Use extended regular expressions

#### AWK Pattern Processing

Complete AWK interpreter with pattern matching, field processing, and configuration support:

```bash
# AWK command structure
safe-ai-util awk 'program' [files...]

# Common AWK operations:
echo "one two three" | safe-ai-util awk '{print $2}'  # Print second field
safe-ai-util awk '/pattern/ {print $0}' file.txt      # Pattern matching
safe-ai-util awk 'BEGIN{sum=0} {sum+=$1} END{print sum}' numbers.txt
```

**Configuration via copilot-util-args:**

```bash
# Example AWK configuration in copilot-util-args
awk.field-separator="\t"
awk.output-separator=" | "
awk.case-insensitive=false
```

**AWK Features:**

- Field processing (`$1`, `$2`, `$NF`, etc.)
- Pattern matching with regex
- Variables and arithmetic operations
- BEGIN and END blocks
- Built-in functions and operators

### ✏️ Custom Editor

Superior Rust-powered terminal editor with advanced features and extensive configuration:

```bash
# Editor command structure
safe-ai-util editor <file> [options]

# Editor options:
-l, --line <NUMBER>      # Start at specific line
-c, --column <NUMBER>    # Start at specific column
-r, --readonly           # Open in read-only mode
-s, --syntax <LANG>      # Syntax highlighting (rust, python, javascript, go)
```

**Configuration via copilot-util-args:**

```bash
# Example editor configuration in copilot-util-args
editor.syntax=auto-detect
editor.tab-width=4
editor.show-line-numbers=true
editor.word-wrap=false
editor.theme=dark
editor.auto-save=true
editor.backup-files=true
```

**Editor Features:**

- **Vi-like keybindings** with multiple modes (normal, insert, command, search, visual)
- **Syntax highlighting** for Rust, Python, JavaScript, Go
- **File operations**: save, save-as, quit, force-quit
- **Search and replace** with regex support
- **Superior performance** with crossterm terminal integration

### � Additional Commands

#### Buf Protocol Buffer Operations

Comprehensive protocol buffer tooling with configuration support:

```bash
# Buf command structure
safe-ai-util buf <subcommand> [options]

# Available buf subcommands:
safe-ai-util buf generate                 # Generate code from proto files
safe-ai-util buf lint                     # Lint proto files
safe-ai-util buf format                   # Format proto files
safe-ai-util buf build                    # Build proto modules
```

**Configuration via copilot-util-args:**

```bash
# Example buf configuration in copilot-util-args
buf.config-file=.buf.yaml
buf.output-dir=gen
buf.generate-docs=true
buf.lint-config=strict
```

#### Exec Command Runner

Execute arbitrary commands with enhanced logging and configuration:

```bash
# Exec command structure
safe-ai-util exec <command> [args...]

# Examples:
safe-ai-util exec go build ./...
safe-ai-util exec npm install
safe-ai-util exec cargo test
```

**Configuration via copilot-util-args:**

```bash
# Example exec configuration in copilot-util-args
exec.log-output=true
exec.timeout=300
exec.capture-env=true
```

## 🔄 Integration with VS Code Tasks

Many repositories have VS Code tasks that use the Rust utility with automatic configuration loading. **Always check for tasks first:**

```bash
# Example task usage (preferred method):
run_task("Git Status", "/path/to/workspace")           # Uses safe-ai-util with args file
run_task("Git Add All", "/path/to/workspace")          # Uses safe-ai-util with args file
run_task("Git Commit", "/path/to/workspace")           # Uses safe-ai-util with args file
run_task("Buf Generate with Output", "/path/to/workspace")  # Uses safe-ai-util with args file
```

### Updating VS Code Tasks for Arguments File Support

Tasks should be updated to use the utility without explicit arguments, allowing the `safe-ai-util-args` (or `copilot-util-args`) file to provide configuration:

```json
{
  "label": "Git Add All",
  "type": "shell",
  "command": "safe-ai-util",
  "args": ["git", "add"],
  "options": {
    "cwd": "${workspaceFolder}"
  }
}
```

The utility will automatically find and load the `safe-ai-util-args` or `copilot-util-args` file from the workspace directory.

## 📋 Usage Priority Examples

### Git Operations

```bash
# ✅ BEST: Use VS Code task (if available)
run_task("Git Status", "/path/to/workspace")

# ✅ GOOD: Use Rust utility directly
safe-ai-util git status

# ❌ AVOID: Manual git command
git status
```

### Text Processing

```bash
# ✅ BEST: Use Rust utility
echo "data" | safe-ai-util sed -e 's/old/new/'
echo "fields" | safe-ai-util awk '{print $2}'

# ❌ AVOID: Manual commands
echo "data" | sed 's/old/new/'
echo "fields" | awk '{print $2}'
```

### File Editing

```bash
# ✅ BEST: Use Rust editor
safe-ai-util editor myfile.rs --syntax rust

# ❌ AVOID: Manual editors
nano myfile.rs
vim myfile.rs
```

## 💡 Benefits of the Rust Utility

1. **Memory Safety**: Rust's borrow checker prevents memory errors
2. **Performance**: Native compiled binary with zero-cost abstractions
3. **Reliability**: Comprehensive error handling and type safety
4. **Consistency**: Unified interface across all development operations
5. **Logging**: Integrated logging for debugging and audit trails
6. **Cross-platform**: Works consistently across all operating systems

## 🚀 Advanced Usage

### Creating and Managing copilot-util-args Files

Create a `copilot-util-args` file in your repository root for consistent configuration:

```bash
# Create standard copilot-util-args file
safe-ai-util editor copilot-util-args

# Or create manually:
cat > copilot-util-args << 'EOF'
# Copilot Agent Utility Configuration
# Auto-loaded when utility is run from this directory or subdirectories

# Git settings
git.default-branch=main
git.auto-push=false
git.commit-template="feat: {message}"

# Editor settings
editor.syntax=auto-detect
editor.tab-width=4
editor.show-line-numbers=true

# Buf/protobuf settings
buf.output-dir=gen
buf.lint-config=.buf.yaml

# Global settings
verbose=true
log-level=info
EOF
```

### Configuration Hierarchy

The utility searches for `copilot-util-args` files in this order:

1. Current working directory
2. Parent directories (up to repository root)
3. User home directory (`~/.copilot-util-args`)
4. Global system directory (`/etc/copilot-util-args`)

Settings from more specific locations override general ones.

### Environment-Specific Configuration

Use different argument files for different environments:

```bash
# Development environment
cp copilot-util-args.dev copilot-util-args

# Production environment
cp copilot-util-args.prod copilot-util-args

# Local overrides
echo "log-level=debug" >> copilot-util-args
```

### Chaining Operations

```bash
# Complex git workflow
safe-ai-util git add .
safe-ai-util git commit -m "feat: implement new feature"
safe-ai-util git push

# Text processing pipeline
safe-ai-util sed -e 's/old/new/g' input.txt | \
safe-ai-util awk '{print $1, $3}' > output.txt
```

### Error Handling

The Rust utility provides superior error messages and handling:

- Clear error descriptions with context
- Proper exit codes for automation
- Detailed logging for debugging
- Safe failure modes without data corruption

## 📚 Command Reference Summary

| Category        | Command                                | Purpose                                           |
| --------------- | -------------------------------------- | ------------------------------------------------- |
| Git             | `safe-ai-util git <subcommand>` | Complete git operations (18+ subcommands)         |
| Text Processing | `safe-ai-util sed <options>`    | Stream editing with regex support                 |
| Text Processing | `safe-ai-util awk '<program>'`  | Pattern processing and field extraction           |
| Editing         | `safe-ai-util editor <file>`    | Superior terminal editor with syntax highlighting |
| Protocol Buffers| `safe-ai-util buf <subcommand>` | Comprehensive protobuf tooling                   |
| Command Execution| `safe-ai-util exec <command>`  | Execute arbitrary commands with enhanced logging  |
| Configuration   | `copilot-util-args` file               | Automatic configuration loading for all commands  |

**Remember: Always use VS Code tasks first, then the Rust utility, and manual commands only as a last resort.**
