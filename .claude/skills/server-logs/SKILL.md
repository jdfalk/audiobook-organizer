---
name: server-logs
description: Pull logs from the audiobook-organizer service and provide web UI login instructions. Requires running server-bootstrap first to get .api-token file. Reads journalctl via SSH (supports tail, filter, streaming), extracts login credentials, and provides instructions for accessing the web dashboard. Use when troubleshooting server issues, checking service status, or needing to log into the web UI.
---

# Server Logs

Retrieves service logs and authentication credentials, with support for filtering, tailing, and streaming. Integrates with the `server-bootstrap` skill to provide credentials.

## Quick Start

Assumes `.api-token` file exists (created by `server-bootstrap` skill):

```bash
# Get last 50 log lines
./scripts/fetch-logs.sh status

# Stream logs (Ctrl+C to stop)
./scripts/fetch-logs.sh stream

# Show only errors
./scripts/fetch-logs.sh errors

# Get login credentials
./scripts/fetch-logs.sh login
```

## What You Get

### Service Logs
Lines from `journalctl -u audiobook-organizer.service` with timestamps, levels (DEBUG/INFO/WARN/ERROR), and messages.

**Log format:**
```
2026-05-23T21:52:40.893-04:00 level=INFO msg="Some message" key1=value1 key2=value2
```

### Login Credentials
From `.api-token` file:
- **Server IP**: `172.16.2.30`
- **API Port**: `8080`
- **API Key**: `abbs_xxxxx...`
- **Web UI URL**: `http://172.16.2.30:8080`
- **Username**: `admin` (if using password auth)

## Log Filtering Options

| Command | Effect |
|---------|--------|
| `status` | Last 50 lines |
| `errors` | Lines with level=ERROR |
| `warnings` | Lines with level=WARN or level=ERROR |
| `stream` | Live stream (tail -f) |
| `since:1h` | Last hour |
| `since:10m` | Last 10 minutes |
| `login` | Show credentials + web UI instructions |

## Streaming vs. Snapshot

- **Snapshot** (`status`, `errors`): Show recent lines and exit (useful for checking current state)
- **Stream** (`stream`): Keep showing new lines as they appear (Ctrl+C to exit)

Use snapshots for quick checks, streams for watching a deployment or debugging.

## Using the Credentials

Once you have login info from `./scripts/fetch-logs.sh login`:

1. **Web UI (http://172.16.2.30:8080)**
   - Open in browser
   - Log in with username/password OR API key
   - View dashboard, library, activity log, etc.

2. **API Calls**
   ```bash
   curl -H "Authorization: Bearer abbs_xxxxx" \
     http://172.16.2.30:8080/api/v1/audiobooks
   ```

3. **SSH to Server**
   ```bash
   ssh root@172.16.2.30
   sudo systemctl status audiobook-organizer.service
   sudo systemctl restart audiobook-organizer.service
   ```

## Common Tasks

### Check if service is running
```bash
./scripts/fetch-logs.sh status
# Look for recent entries; no errors = healthy
```

### Watch for errors during a deployment
```bash
./scripts/fetch-logs.sh stream
# Ctrl+C when done
```

### Find a specific error
```bash
./scripts/fetch-logs.sh errors
# Grep output for keywords
```

### Get credentials for API call
```bash
./scripts/fetch-logs.sh login
# Copy the API key
```

## Log Levels

- **DEBUG**: Detailed diagnostic info (usually hidden)
- **INFO**: Normal operation messages
- **WARN**: Warnings (unusual but handled)
- **ERROR**: Errors (problems that need attention)

By default, `errors` filter shows WARN and ERROR. Use `warnings` to include both.

## Troubleshooting

### "SSH connection failed"
- Check server IP in `.api-token`
- Check network connectivity
- Verify SSH key or credentials

### "No logs found / empty output"
- Service may be stopped (check `status`)
- Or no activity in requested time window
- Try `stream` to wait for new logs

### ".api-token not found"
- Run `server-bootstrap` skill first to create it
- Token expires after 8 hours; run bootstrap again if expired

## See Also

- `server-bootstrap` — Create `.api-token` file
- `build-deploy` — Deploy new version before checking logs
- Web dashboard — Open http://`<server>`:8080 for full UI
