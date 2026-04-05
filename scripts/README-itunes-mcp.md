# iTunes MCP Server

An MCP (Model Context Protocol) server that exposes the iTunes COM API on the
Windows machine (`unimatrixzero.local`) for remote control from Claude Code.

## Overview

The server runs as a PowerShell script over stdio, speaking JSON-RPC 2.0 with
MCP framing (Content-Length headers, like LSP). Claude Code connects to it via
SSH and can then query tracks, verify files, open libraries, and run tests.

## Setup

### Prerequisites

- **Windows machine** with iTunes for Windows installed
- **SSH server** running on the Windows machine (OpenSSH for Windows)
- **PowerShell 5.1+** (built into Windows)
- SSH key-based auth configured from your Mac to `jdfalk@unimatrixzero.local`

### 1. Copy the script to the Windows machine

The script lives at `W:\audiobook-organizer\scripts\itunes-mcp-server.ps1`.
If the repo is already cloned/synced there, no action needed.

### 2. Configure Claude Code

The MCP server is already configured in `.mcp.json` at the project root.
Claude Code will detect it automatically. To add it manually instead, run:

```bash
claude mcp add itunes -- ssh jdfalk@unimatrixzero.local powershell -ExecutionPolicy Bypass -File "W:\\audiobook-organizer\\scripts\\itunes-mcp-server.ps1"
```

### 3. Verify connectivity

```bash
ssh jdfalk@unimatrixzero.local "powershell -Command 'echo ok'"
```

## Available Tools

| Tool | Description |
|------|-------------|
| `itunes_open_library(path)` | Set registry and launch iTunes with specified library folder |
| `itunes_close()` | Quit iTunes via COM and release resources |
| `itunes_get_track_count()` | Return total track count |
| `itunes_get_tracks(offset, limit)` | Paginated track list (Name, Artist, Album, Location, Duration, PersistentID) |
| `itunes_verify_files(limit)` | Check if track file locations exist on disk |
| `itunes_get_library_info()` | Library path, iTunes version, track/playlist counts |
| `itunes_search(query, limit)` | Search tracks by name |
| `itunes_run_test(test_folder)` | Run a single test case, return results |

## Usage Examples

Once configured, Claude Code can use the tools directly:

- "Open the production iTunes library and check how many tracks it has"
- "Search iTunes for tracks by 'Brandon Sanderson'"
- "Verify that all track file paths are valid"
- "Run the test case in W:\audiobook-organizer\.itunes-writeback\tests\basic-3-books"

## Protocol Details

- **Transport**: stdio over SSH
- **Framing**: Content-Length headers (MCP/LSP style)
- **Protocol**: JSON-RPC 2.0 with MCP method names (`initialize`, `tools/list`, `tools/call`)
- **Logging**: All diagnostic output goes to stderr (does not interfere with protocol)

## Troubleshooting

**"iTunes COM object failed"**: iTunes may not be installed, or another instance
is running. The server will try to kill existing instances before launching.

**SSH timeout**: Ensure the Windows SSH server is running:
`Get-Service sshd | Start-Service`

**Registry access**: The script writes to `HKCU:\Software\Apple Computer, Inc.\iTunes`
to set the library path. No admin rights needed for HKCU.

**Large libraries**: Operations on 88K+ tracks may take a while. Use `limit`
parameters to paginate. The `itunes_verify_files` tool caps invalid path details
at 100 entries to keep response sizes manageable.
