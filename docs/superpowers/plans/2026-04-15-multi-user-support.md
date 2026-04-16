# Multi-User Support — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Spec:** `docs/superpowers/specs/2026-04-15-multi-user-support-design.md`
**Depends on:** 4.4 DI (Server.store field must exist for request-scoped user injection)
**Unblocks:** 3.6 Read/unread (per-user state), 3.4 Playlists (per-user ownership)

---

### Task 1: Schema — users, roles, sessions, invites (1 PR)

**Files:**
- Modify: `internal/database/store.go` — add `User`, `Role`, `Session`, `APIKey`, `Invite` structs + Store interface methods
- Modify: `internal/database/pebble_store.go` — implement all new Store methods with PebbleDB keys per the spec
- Modify: `internal/database/mock_store.go` — add mock implementations
- Create: `internal/database/user_store_test.go`

PebbleDB keys:
```
user:{id}, user_by_username:{lcase}, user_by_oauth:{provider}:{subject}
role:{id} (inline permissions array)
user_role:{userID}:{roleID}, role_member:{roleID}:{userID}
session:{id}, user_session:{userID}:{sessionID}
api_key:{id}, user_api_key:{userID}:{keyID}
invite:{token}, invite_by_username:{username}
```

- [ ] Implement all CRUD methods: CreateUser, GetUserByID, GetUserByUsername, GetUserByOAuth, UpdateUser, DeleteUser, ListUsers, CountUsers
- [ ] Role methods: GetRoleByID, ListRoles, CreateRole, UpdateRole
- [ ] Membership: GetUserRoleIDs, SetUserRoles (atomic batch), GetRoleMemberIDs
- [ ] Session CRUD + CleanExpiredSessions
- [ ] APIKey CRUD + RevokeAPIKey
- [ ] Invite CRUD + ConsumeInvite (atomic batch: delete invite + create user + write role)
- [ ] Test each method
- [ ] Seed roles: `admin` (all perms), `editor` (all except users/integrations/settings manage), `viewer` (library.view only)

---

### Task 2: Permission constants + context helpers (1 PR)

**Files:**
- Create: `internal/auth/permissions.go` — permission string constants
- Create: `internal/auth/context.go` — `WithUser`, `UserFromContext`, `WithPermissions`, `PermissionsFromContext`, `Can`
- Create: `internal/auth/context_test.go`

- [ ] Define all permission constants from spec: `library.view`, `library.edit_metadata`, `library.delete`, `library.organize`, `scan.trigger`, `integrations.manage`, `users.manage`, `settings.manage`, `playlists.create`, `requests.create`, `requests.approve`
- [ ] Typed context helpers using unexported `ctxKey` type
- [ ] `Can(ctx, perm) bool` — check if calling user has the permission
- [ ] Test roundtrip: set user + perms on context, read back

---

### Task 3: Auth middleware + session management (1 PR)

**Files:**
- Create: `internal/server/auth_middleware.go` — `authenticate()` gin middleware, `requirePerm(perm)` route-level middleware factory
- Modify: `internal/server/server.go` — wire middleware into router
- Create: `internal/server/session_service.go` — login, logout, session create/validate/cleanup
- Create: `internal/server/auth_middleware_test.go`

- [ ] `authenticate()`: read session cookie → load session from store → load user → set user + permissions on `c.Request.Context()`. If no valid session → continue without user (public routes)
- [ ] `requirePerm(perm)`: read user from context → check `Can(ctx, perm)` → 401 if no user, 403 if no permission
- [ ] Session cookie: `ao_session` httpOnly secure SameSite=Lax, random 32-byte hex value
- [ ] Session expiry: 30 days default, refresh `last_seen_at` on every request
- [ ] Login: verify bcrypt password → create session → set cookie
- [ ] Logout: delete session + clear cookie
- [ ] Test: authenticated request succeeds; unauthenticated 401; insufficient permission 403

---

### Task 4: Password auth — login/logout endpoints + login page (1 PR)

**Files:**
- Create: `internal/server/login_handlers.go` — `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/me`
- Create: `web/src/pages/Login.tsx` (may already exist — modify)
- Modify: `internal/server/server.go` — register public routes for login, protected for logout/me

- [ ] `POST /auth/login` — username + password → bcrypt verify → session → cookie → 200
- [ ] `POST /auth/logout` — delete session → clear cookie → 200
- [ ] `GET /auth/me` — return current user + permissions (or 401)
- [ ] Failed login tracking: increment `failed_login_count`, check lockout (10 fails / 15 min)
- [ ] Frontend Login page with username + password form, error display, redirect after success
- [ ] Test: login flow, lockout after 10 failures

---

### Task 5: First-run setup wizard (1 PR)

**Files:**
- Create: `internal/server/setup_handlers.go` — `GET /api/v1/setup/status`, `POST /api/v1/setup/create-admin`
- Create: `web/src/pages/Setup.tsx` — one-page wizard: username + password + confirm
- Modify: `internal/server/server.go` — `/setup` routes enabled only when CountUsers() == 0

- [ ] `GET /setup/status` → `{needs_setup: true/false}`
- [ ] `POST /setup/create-admin` — create user with `admin` role, create session, redirect to app
- [ ] Guard: if any users exist, return 403
- [ ] Mutex to prevent race if two browsers hit `/setup` simultaneously
- [ ] Frontend: auto-redirect to /setup on first visit when `needs_setup == true`

---

### Task 6: Wire `requirePerm` on all existing routes (1 PR)

**Files:**
- Modify: `internal/server/server.go` — add `s.requirePerm(...)` to every route in `setupRoutes`

- [ ] Group routes by permission level:
  - `library.view`: GET endpoints for books, authors, series, operations, activity
  - `library.edit_metadata`: PUT/POST/DELETE on books, metadata apply, batch ops
  - `library.delete`: DELETE endpoints
  - `library.organize`: organize, reconcile
  - `scan.trigger`: scan, import
  - `integrations.manage`: iTunes, deluge, openai config
  - `settings.manage`: config endpoints
  - `users.manage`: user CRUD (task 7)
- [ ] CI grep script: `grep -rn 'protected\.\(GET\|POST\|PUT\|DELETE\|PATCH\)' server.go | grep -v requirePerm` → must return zero lines

---

### Task 7: User management admin page (1 PR)

**Files:**
- Create: `internal/server/user_handlers.go` — CRUD for users, invite generation, password reset
- Create: `web/src/pages/Users.tsx` — admin page: list users, create invite, deactivate, reset password
- Modify: `web/src/components/layout/Sidebar.tsx` — add Users link (admin only)
- Modify: `internal/server/server.go` — routes for user management, gated on `users.manage`

- [ ] `GET /api/v1/users` — list (admin only)
- [ ] `POST /api/v1/users/invite` — generate invite {username, role_id, expires_in} → returns token URL
- [ ] `POST /api/v1/users/:id/reset-password` — regenerate invite for existing user
- [ ] `DELETE /api/v1/users/:id` — soft-deactivate (mark locked_until = far future)
- [ ] `GET /api/v1/invites` — list active invites
- [ ] Invite consumption endpoint: `POST /api/v1/auth/accept-invite` — token + password → create user
- [ ] Frontend: Users page with table, "Create Invite" dialog, action buttons

---

### Task 8: user_id backfill + audit trail (1 PR)

**Files:**
- Modify: `internal/database/pebble_store.go` — add `user_id` field to `Operation`, `OperationChange`, activity log entry structs
- Create backfill tracked operation: walk all existing ops/changes/activity, set `user_id = admin_user_id`
- Create: `_system` pseudo-user row at startup if not present

- [ ] Add `user_id` to operation + change + activity-log key schemas
- [ ] `_system` user: `{is_system: true, username: "_system"}`, no password, no permissions, cannot log in
- [ ] Backfill: one-time tracked op (idempotent) — attribute existing data to the admin user created in task 5
- [ ] Going forward: every operation/change/activity write includes the calling user's ID from context

---

### Task 9 (deferred): OAuth + JWT API keys

Not in MVP. Separate PRs after MVP ships:

**9a: OAuth (GitHub + Google)**
- Create: `internal/server/oauth_handlers.go` — OAuth2 flow, callback, find-or-create user
- Add `golang.org/x/oauth2` dependency
- Config: OAuth client IDs + secrets in settings

**9b: JWT personal API keys**
- Create: `internal/server/apikey_handlers.go` — generate, list, revoke
- Modify: `internal/server/auth_middleware.go` — recognize `Authorization: Bearer <jwt>`, decode, lookup `api_key:{jti}`, verify not revoked, load user

---

### Estimated effort

| Task | Size | Depends on |
|---|---|---|
| 1 (schema) | L | 4.4 task 1 |
| 2 (permissions) | S | — |
| 3 (middleware) | M | 1+2 |
| 4 (login) | M | 3 |
| 5 (setup wizard) | M | 4 |
| 6 (wire routes) | M | 3 |
| 7 (admin page) | L | 4+6 |
| 8 (backfill) | M | 5+7 |
| 9a (OAuth) | M | deferred |
| 9b (JWT) | M | deferred |
| **Total** | ~8 MVP PRs, L overall | |
