# Admin CLI + Multi-User Auth + RBAC Design

**Date:** 2026-03-07
**Status:** Draft — design only, not yet implemented

## Overview

Three interconnected features:

1. **Admin CLI mode** — a `admin` subcommand in the binary that connects to a remote server and sends commands via the REST API
2. **Multi-user support** — full user management with registration, profiles, and per-user API tokens
3. **RBAC** — resource:action permission model with custom roles

## 1. RBAC Permission Model

### Permissions

Permissions are `resource:action` strings. The wildcard `*` grants all permissions.

| Permission | Description |
|---|---|
| `books:read` | View audiobooks, metadata, history |
| `books:write` | Edit metadata, apply changes, delete |
| `scans:trigger` | Start scans, organize operations |
| `config:read` | View server configuration |
| `config:write` | Modify server configuration |
| `users:manage` | Create/edit/delete users and roles |
| `tokens:manage` | Create/revoke API tokens (own tokens always allowed) |
| `system:admin` | System-level operations (backup, restore, reset, logs) |
| `logs:read` | View server logs |

### Default Roles

| Role | Permissions |
|---|---|
| **admin** | `*` (all) |
| **user** | `books:read`, `books:write`, `scans:trigger`, `config:read`, `logs:read` |
| **viewer** | `books:read`, `config:read` |

### Custom Roles

Admins can create custom roles with any combination of permissions. Roles are stored in the database.

### DB Schema: Roles

```sql
CREATE TABLE roles (
    id          TEXT PRIMARY KEY,  -- ULID
    name        TEXT UNIQUE NOT NULL,
    permissions TEXT NOT NULL,     -- JSON array of permission strings
    built_in    BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMP,
    updated_at  TIMESTAMP
);

CREATE TABLE user_roles (
    user_id TEXT NOT NULL REFERENCES users(id),
    role_id TEXT NOT NULL REFERENCES roles(id),
    PRIMARY KEY (user_id, role_id)
);
```

### Permission Checking

```go
// HasPermission checks if a user has a specific permission via any of their roles.
func HasPermission(user *database.User, permission string) bool {
    for _, role := range user.Roles {
        if role has "*" or role has permission {
            return true
        }
    }
    return false
}
```

Middleware applies permission checks per-route using a `RequirePermission("books:write")` middleware function.

## 2. Per-User API Tokens

### Design

- Each user can create multiple named API tokens (e.g., "CLI on laptop", "automation script")
- Tokens are 32-byte random values, base64url-encoded (43 chars)
- Only the SHA-256 hash is stored in DB (token shown once at creation, never again)
- First 8 characters stored as `token_prefix` for identification in listings
- Tokens inherit the creating user's roles/permissions directly (no sub-scoping)
- Sent via `Authorization: Bearer <token>` header
- Optional expiry date; tokens can be revoked individually

### DB Schema: API Tokens

```sql
CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,  -- ULID
    user_id      TEXT NOT NULL REFERENCES users(id),
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL,     -- SHA-256 hash of the token
    token_prefix TEXT NOT NULL,     -- First 8 chars for display
    expires_at   TIMESTAMP,        -- NULL = never expires
    last_used_at TIMESTAMP,
    created_at   TIMESTAMP NOT NULL,
    revoked_at   TIMESTAMP         -- NULL = active
);
```

### Auth Flow

The existing auth middleware is extended:

1. Check `Authorization: Bearer <token>` header first
2. If present, SHA-256 hash the token, look up in `api_tokens` table
3. If valid (not revoked, not expired), load the associated user + roles
4. If no bearer token, fall back to existing session cookie auth
5. If neither, check basic auth if enabled
6. Rate limiting applies per-user (whether via token or session)

### API Endpoints for Token Management

```
POST   /api/v1/tokens          — Create new token (returns token value once)
GET    /api/v1/tokens          — List own tokens (prefix, name, last_used, expiry)
DELETE /api/v1/tokens/:id      — Revoke a token
```

Admin users can also:
```
GET    /api/v1/admin/tokens           — List all tokens across users
DELETE /api/v1/admin/tokens/:id       — Revoke any token
```

## 3. Admin CLI Mode

### Usage

```bash
# Connect to a remote server
audiobook-organizer admin --server https://myserver:8484 --token <api-token>

# Subcommands
audiobook-organizer admin config get                    # Get full config
audiobook-organizer admin config set key=value ...      # Set config values
audiobook-organizer admin status                        # System status
audiobook-organizer admin scan --path /audiobooks       # Trigger scan
audiobook-organizer admin books list                    # List audiobooks
audiobook-organizer admin books get <id>                # Get book details
audiobook-organizer admin books update <id> --field val # Update book
audiobook-organizer admin users list                    # List users
audiobook-organizer admin users create <username>       # Create user
audiobook-organizer admin roles list                    # List roles
audiobook-organizer admin roles create <name> --perms   # Create role
audiobook-organizer admin logs --level info --tail 100  # View logs
audiobook-organizer admin backup create                 # Create backup
audiobook-organizer admin token create --name "my cli"  # Create API token
```

### Connection Config

The CLI reads connection details from (in priority order):
1. Command-line flags (`--server`, `--token`)
2. Environment variables (`AUDIOBOOK_SERVER`, `AUDIOBOOK_TOKEN`)
3. Config file (`~/.audiobook-organizer.yaml` → `admin.server`, `admin.token`)

### Implementation

The admin CLI is a thin HTTP client wrapper:
- Each subcommand maps to a REST API call
- Responses are formatted as human-readable tables/JSON
- `--output json` flag for machine-readable output
- `--output table` (default) for human-readable output
- Exit codes: 0 = success, 1 = error, 2 = auth failure

### New Package

```
internal/admin/
    client.go       — HTTP client with auth, TLS, retries
    commands.go     — Cobra command tree for admin subcommands
    formatter.go    — Output formatting (table, JSON)
cmd/admin.go        — Registers admin command with root
```

## 4. Multi-User Enhancements

### Current State

The server already has:
- User model in DB (`users` table with id, username, email, password_hash, roles, status)
- Session-based auth (login, logout, session management)
- Basic auth option
- Auth middleware that can be enabled/disabled

### What's Needed

- **User management API** (admin-only):
  ```
  GET    /api/v1/admin/users          — List all users
  POST   /api/v1/admin/users          — Create user
  GET    /api/v1/admin/users/:id      — Get user details
  PUT    /api/v1/admin/users/:id      — Update user (roles, status, etc.)
  DELETE /api/v1/admin/users/:id      — Delete user
  ```

- **Role management API** (admin-only):
  ```
  GET    /api/v1/admin/roles          — List all roles
  POST   /api/v1/admin/roles          — Create custom role
  PUT    /api/v1/admin/roles/:id      — Update role permissions
  DELETE /api/v1/admin/roles/:id      — Delete custom role (not built-in)
  ```

- **Self-service endpoints** (any authenticated user):
  ```
  PUT    /api/v1/me/password          — Change own password
  GET    /api/v1/me/tokens            — List own API tokens
  POST   /api/v1/me/tokens            — Create own API token
  DELETE /api/v1/me/tokens/:id        — Revoke own API token
  ```

- **Permission middleware** — wrap existing routes with `RequirePermission()` checks
- **Seed default roles** on first startup / migration

## 5. Implementation Order

This is the suggested implementation sequence. Each phase is independently useful.

### Phase 1: RBAC Foundation
- Add `roles` and `user_roles` tables to DB schema
- Seed default roles (admin, user, viewer)
- Add `HasPermission()` helper
- Add `RequirePermission()` middleware
- Apply permission checks to existing API routes

### Phase 2: API Tokens
- Add `api_tokens` table
- Extend auth middleware for Bearer token support
- Add token CRUD endpoints
- Add token management to web UI settings page

### Phase 3: User & Role Management APIs
- Add admin user CRUD endpoints
- Add admin role CRUD endpoints
- Add self-service password change + token management
- Add user management page to web UI

### Phase 4: Admin CLI
- Create `internal/admin/` package with HTTP client
- Add `admin` cobra subcommand with all sub-commands
- Config/status/scan/books/users/roles/logs/backup commands
- Table and JSON output formatters

### Phase 5: Web UI Integration
- User management page (admin only)
- Role management page (admin only)
- API token management in user profile
- Permission-aware UI (hide/disable features user can't access)

## 6. Security Considerations

- API tokens are SHA-256 hashed before storage (never stored in plaintext)
- Token creation response includes the raw token exactly once
- Rate limiting per-user applies to both session and token auth
- Failed auth attempts are logged with IP
- Admin operations are audit-logged
- RBAC checks happen at middleware level (not just UI hiding)
- Built-in roles cannot be deleted or have permissions reduced below minimum
- The first user created is always assigned the admin role (existing behavior preserved)
