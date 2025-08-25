# file: scripts/copilot-firewall/README.md

# version: 1.0.0

# guid: a1b2c3d4-5e6f-7g8h-9i0j-1k2l3m4n5o6p

# GitHub Copilot Firewall Allowlist Manager

An interactive Python tool for managing the `COPILOT_AGENT_FIREWALL_ALLOW_LIST_ADDITIONS`
environment variable across multiple GitHub repositories.

## Features

- 🎯 **Interactive Selection**: Choose specific repositories with checkboxes
- 🌟 **Bulk Operations**: Select all repositories at once
- 🔍 **Smart Filtering**: Filter repositories by name or description
- 📊 **Rich Display**: Beautiful table display with repository metadata
- 🔒 **Safety Features**: Dry-run mode and confirmation prompts
- 🚀 **Fast**: Efficient GitHub CLI integration

## Installation

### Prerequisites

1. **GitHub CLI**: Install from https://cli.github.com/manual/installation
2. **Python 3.8+**: Required for the script
3. **Authentication**: Run `gh auth login` to authenticate with GitHub

### Install the Tool

```bash
# Navigate to the copilot-firewall directory
cd scripts/copilot-firewall

# Install in development mode
pip install -e .

# Or install dependencies only
pip install inquirer rich
```

## Usage

### Basic Usage

```bash
# Run with default settings (jdfalk organization)
copilot-firewall

# Or run directly with Python
python -m copilot_firewall.main
```

### Advanced Options

```bash
# Specify different organization
copilot-firewall --org your-org

# Dry run (see what would happen without making changes)
copilot-firewall --dry-run

# List repositories only
copilot-firewall --list-only

# Limit number of repositories fetched
copilot-firewall --limit 50

# Get help
copilot-firewall --help
```

### Interactive Modes

When you run the tool, you'll see several options:

1. **🎯 Select specific repositories**: Choose individual repos with checkboxes
2. **🌟 Select all repositories**: Apply to all repos at once
3. **🔍 Filter and select repositories**: Filter by name/description, then select
4. **❌ Cancel operation**: Exit without making changes

## What It Does

The tool sets the `COPILOT_AGENT_FIREWALL_ALLOW_LIST_ADDITIONS` environment variable for GitHub
Actions in your selected repositories. This variable contains a comprehensive list of allowed
domains for GitHub Copilot agents.

### Included Domains

The allowlist includes essential domains for development:

- **Documentation**: docs.github.com, developer.mozilla.org, docs.microsoft.com
- **Package Registries**: npmjs.com, pypi.org, rubygems.org, crates.io
- **Cloud Providers**: aws.amazon.com, azure.microsoft.com, cloud.google.com
- **Development Tools**: git-scm.com, docker.com, kubernetes.io
- **Standards Organizations**: w3.org, ietf.org, ecma-international.org
- **Learning Resources**: freecodecamp.org, css-tricks.com, baeldung.com
- \*\*And many more...

## Examples

### Select Specific Repositories

```bash
$ copilot-firewall
GitHub Copilot Firewall Allowlist Manager
Organization: jdfalk

Fetching repositories...
┌─────────────────────┬────────────┬──────────────────────────────┬──────────────┐
│ Repository          │ Visibility │ Description                  │ Last Updated │
├─────────────────────┼────────────┼──────────────────────────────┼──────────────┤
│ auto-formatter      │ 🌍 Public  │ GitHub Action for automated │ 2025-06-29   │
│ gcommon             │ 🌍 Public  │ Common Go utilities          │ 2025-06-29   │
│ subtitle-manager    │ 🌍 Public  │ Subtitle file manager        │ 2025-06-29   │
└─────────────────────┴────────────┴──────────────────────────────┴──────────────┘

What would you like to do?
> 🎯 Select specific repositories
  🌟 Select all repositories
  🔍 Filter and select repositories
  ❌ Cancel operation
```

### Dry Run Mode

```bash
$ copilot-firewall --dry-run
# Shows what would be done without actually setting variables
```

### Filter Repositories

```bash
$ copilot-firewall
# Choose "🔍 Filter and select repositories"
# Enter filter term: "subtitle"
# Shows only repositories matching "subtitle"
```

## Development

### Project Structure

```
copilot-firewall/
├── pyproject.toml              # Project configuration
├── README.md                   # This file
├── copilot_firewall/
│   ├── __init__.py            # Package initialization
│   └── main.py                # Main application logic
└── tests/                     # Test files (future)
```

### Code Quality

The project uses modern Python tooling:

- **Black**: Code formatting
- **isort**: Import sorting
- **mypy**: Type checking
- **ruff**: Fast linting
- **pytest**: Testing framework

### Running Development Commands

```bash
# Format code
black copilot_firewall/
isort copilot_firewall/

# Type checking
mypy copilot_firewall/

# Linting
ruff check copilot_firewall/

# Run tests (when available)
pytest
```

## Security

This tool only sets repository-level environment variables that are publicly visible in GitHub
Actions. It does not handle secrets or sensitive information.

The GitHub CLI must be authenticated with appropriate permissions to modify repository settings.

## Troubleshooting

### Common Issues

1. **GitHub CLI not found**

   ```
   GitHub CLI (gh) is not installed. Please install it first.
   ```

   Solution: Install GitHub CLI from https://cli.github.com/

2. **Not authenticated**

   ```
   You are not logged in to GitHub CLI. Please run 'gh auth login' first.
   ```

   Solution: Run `gh auth login` and follow the prompts

3. **No repositories found**

   ```
   No repositories found or you don't have access to the namespace.
   ```

   Solution: Check the organization name and your access permissions

4. **Permission denied**
   ```
   Error setting variable for repository-name: exit status 1
   ```
   Solution: Ensure you have admin or write access to the repository

### Getting Help

- Use `copilot-firewall --help` for command-line options
- Check GitHub CLI authentication with `gh auth status`
- Verify repository access with `gh repo list your-org`

## License

This project is part of the ghcommon repository and follows the same license terms.
