---
name: server-bootstrap
description: Initialize server authentication and retrieve API key. SSH to the audiobook-organizer server, restart the service, extract the bootstrap token from journalctl, exchange it for an API key via POST /api/v1/auth/bootstrap, and write the key to .claude/.api-token (shared across worktrees, auto-cleanup after 8 hours). Use when starting fresh or when the API key has expired.
---

# Server Bootstrap

Initializes authentication for the audiobook-organizer server and stores the API key for use across all worktrees. The key is written to `.claude/.api-token` with a timestamp; cleanup happens automatically after 8 hours.

## Quick Start

When you need an API key (first time, or after restart):

```
Server IP: <user will prompt>
```

The skill will:
1. SSH to the server and restart `audiobook-organizer.service`
2. Extract the bootstrap token from journalctl logs (10-minute window)
3. POST to `/api/v1/auth/bootstrap` to exchange token for API key
4. Write key + expiry to `.claude/.api-token` (shared, .gitignored)
5. Schedule cleanup after 8 hours (non-blocking background process)

## The Token File

The `.api-token` file format:
```
api_key=abbs_xxxxx
key_id=...
username=admin
expires_at=<unix-timestamp-8h-from-now>
```

Other worktrees read this file to get the shared API key. The cleanup process removes the file after 8 hours.

## Bootstrap Token Exchange

The bootstrap token (from logs) is one-time-use and valid for 10 minutes. The POST request:

```bash
curl -X POST http://<server>:8484/api/v1/auth/bootstrap \
  -H "Content-Type: application/json" \
  -d '{"token":"abbs_...", "key_name":"workspace-key"}'
```

Returns:
```json
{
  "api_key": "abbs_xxxxx",
  "key_id": "...",
  "user_id": "...",
  "username": "admin",
  "scopes": ["all"]
}
```

See [references/bootstrap-api.md](references/bootstrap-api.md) for full API details.

## Troubleshooting

- **"Token expired"**: Server restart required. The bootstrap token has a fixed 10-minute TTL from service startup.
- **SSH connection fails**: Check server IP and network connectivity.
- **Rate limited**: If you fail the token exchange 5 times in an hour, you'll be rate-limited. Wait or restart the service for a fresh token.

## When to Use This Skill

- Starting a new worktree for the first time
- After a server restart (old token no longer valid)
- When you get 401 Unauthorized on API calls
- After the automatic 8-hour cleanup of the token file
