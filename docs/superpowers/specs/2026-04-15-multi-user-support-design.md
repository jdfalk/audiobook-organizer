<!-- file: docs/superpowers/specs/2026-04-15-multi-user-support-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8a2d5c1e-3b9f-4f60-a7d4-1c8e0f2b9a57 -->

# Multi-User Support — Design

**Status:** Design complete (Apr 15, 2026). Ready for implementation plan.
**Scope item:** TODO.md §3.7.
**Depends on:** 4.4 (DI for `Store`) — design complete, implementation in-flight.
**Brainstorm:** Apr 15, 2026 session.

## Goal

Replace the single-implicit-user model with authenticated multi-user access. All books remain a **shared global library**; users have different permissions expressed via a role matrix. This is primarily about "let my friends view the library safely" — not per-user libraries, not multi-tenancy. Request-based sharing (users asking admin to add something) is reserved for a future spec.

## Locked decisions

### 1. Library scope — shared, not per-user

- All `books`, `authors`, `series`, `book_versions`, operations, tags — global and shared across users
- What differs between users is **permission to see/act on** that data, not *which* data
- Non-admin users can see the entire library (read-only by default)
- Future: `requests.create` / `requests.approve` permissions for user-requested additions (reserved now, implemented later)

### 2. Auth — all three layered into one issuer

| Mechanism | Entry | Result |
|---|---|---|
| Password (local) | Form login | Session cookie |
| OAuth (GitHub, Google) | Provider callback | Session cookie |
| JWT (personal API keys) | `Authorization: Bearer` | Per-request auth, no cookie |

All three converge on the same `*User` in `c.Request.Context()`. Middleware is the single choke point. OAuth providers limited to GitHub + Google initially.

### 3. Permissions model — atoms with role bundles

Permissions are **Go string constants**, not DB rows — validated at route-registration time.

```go
const (
    PermLibraryView         = "library.view"
    PermLibraryEditMetadata = "library.edit_metadata"
    PermLibraryDelete       = "library.delete"
    PermLibraryOrganize     = "library.organize"
    PermScanTrigger         = "scan.trigger"
    PermIntegrationsManage  = "integrations.manage"   // deluge, iTunes, openai, etc.
    PermUsersManage         = "users.manage"
    PermSettingsManage      = "settings.manage"
    PermRequestsCreate      = "requests.create"       // reserved, future
    PermRequestsApprove     = "requests.approve"      // reserved, future
)
```

Seed roles:

| Role | Permissions |
|---|---|
| `admin` | all (including `users.manage`, `integrations.manage`, `settings.manage`) |
| `editor` | everything except `users.manage`, `integrations.manage`, `settings.manage` |
| `viewer` | `library.view` only |

Users can have multiple roles; effective permission set is the union. Custom roles can be created by admins later.

### 4. Enforcement — hybrid middleware + route-level

```go
protected.GET ("/audiobooks",      s.requirePerm(PermLibraryView),         s.listAudiobooks)
protected.POST("/audiobooks/:id",  s.requirePerm(PermLibraryEditMetadata), s.updateAudiobook)
```

- `requirePerm` is a middleware factory: reads user from context, checks permissions, 403 otherwise
- All mutating routes MUST call `requirePerm`; CI grep catches new routes that skip it
- Login / OAuth-callback / health-check routes explicitly bypass via a short public list

### 5. User lifecycle

| Action | Who | Mechanism |
|---|---|---|
| First user created | Server | First-run setup wizard at `/setup` OR `audiobook-organizer user create` CLI |
| Subsequent users | Admin | Users admin page → generates a single-use invite link |
| Self-signup | — | **Disabled**. Invite-only |
| Password reset | Admin | Admin regenerates invite; user re-sets password through it |
| Email-based reset | — | Not in MVP. Infrastructure not worth it |
| Self-edit | User | Can change own password, generate/revoke own API keys; cannot change own roles |
| Lockout | Server | 10 failed logins in 15 min → 15-min lockout window, logged to activity |
| 2FA | — | Not in MVP. Schema leaves room |

### 6. First-run migration

At startup, if no users exist:

1. Server logs `"No users configured. Open http://<host>:<port>/setup or run 'audiobook-organizer user create'."`
2. `/setup` route is enabled only while user count is zero. Accepts username + password + confirm. Creates the user as **admin** and disables `/setup`.
3. All existing DB rows that need a `user_id` (see §10) get backfilled to this user on first login — retroactive provenance for single-user-era data.

### 7. Audit trail — `user_id` on mutating records

Added to:

- `operations` (who launched this op; null → `_system`)
- `operation_changes` (who made each change)
- Activity log entries
- `book_tags` (existing `source` field gains user-id precision when source is `user`)

Scheduler / background jobs attribute to a reserved **`_system`** pseudo-user — a `users` row with `is_system=true`, no permissions, cannot log in, cannot be deleted.

### 8. Integration scope

All external integrations (iTunes, deluge, openlibrary, audible, openai) are **system-level**. Config and trigger gates on `integrations.manage` (admin by default). Viewers can *see* iTunes-sourced books and torrent status but can't configure or trigger anything.

## PebbleDB key-space (matches existing conventions)

### Users

```
user:{userID}                       → User JSON blob
user_by_username:{lcase-username}   → userID
user_by_oauth:{provider}:{subject}  → userID
```

### Roles (permissions inline)

```
role:{roleID}                       → Role JSON: {name, description, permissions: [...]}
```

No `permissions` or `role_permissions` tables — permissions are Go constants, role blobs carry their own permission lists.

### Membership (existence-based, written in batch)

```
user_role:{userID}:{roleID}         → ""
role_member:{roleID}:{userID}       → ""
```

Both written in one Pebble `Batch` to preserve the bidirectional invariant atomically.

### Sessions

```
session:{sessionID}                 → Session JSON: {userID, permissions_snapshot, expires_at, ...}
user_session:{userID}:{sessionID}   → ""
```

Session blob caches the user's permission set at login — no JOIN per request. Invalidated + rewritten on role change. Expired sessions cleaned in the existing maintenance window.

### API keys

```
api_key:{keyID}                     → APIKey JSON: {userID, name, created_at, last_used_at, revoked_at}
user_api_key:{userID}:{keyID}       → ""
```

JWT carries `keyID` as `jti`. Verification: decode → lookup `api_key:{keyID}` → check `revoked_at` is null → load user → auth complete.

### Invites

```
invite:{token}                      → Invite JSON: {username, roleID, created_by, expires_at, used_at}
invite_by_username:{username}       → token
```

Consumption (user first opens invite URL, sets password): delete both keys + create user + write role membership, all in one Pebble batch.

## `Store` interface additions

```go
// Users
GetUserByID(id string) (*User, error)
GetUserByUsername(username string) (*User, error)
GetUserByOAuth(provider, subject string) (*User, error)
CreateUser(*User) (*User, error)
UpdateUser(id string, *User) (*User, error)
DeleteUser(id string) error
ListUsers(limit, offset int) ([]User, int, error)
CountUsers() (int, error)

// Roles
GetRoleByID(id string) (*Role, error)
ListRoles() ([]Role, error)
CreateRole(*Role) (*Role, error)
UpdateRole(id string, *Role) (*Role, error)

// Membership (atomic)
GetUserRoleIDs(userID string) ([]string, error)
SetUserRoles(userID string, roleIDs []string) error
GetRoleMemberIDs(roleID string) ([]string, error)

// Sessions
CreateSession(*Session) error
GetSession(id string) (*Session, error)
DeleteSession(id string) error
DeleteUserSessions(userID string) error       // logout-all
CleanExpiredSessions(before time.Time) (int, error)

// API keys
CreateAPIKey(*APIKey) error
GetAPIKey(id string) (*APIKey, error)
RevokeAPIKey(id string) error
ListUserAPIKeys(userID string) ([]APIKey, error)

// Invites
CreateInvite(*Invite) error
GetInvite(token string) (*Invite, error)
ConsumeInvite(token string, userID string) error   // atomic batch
ListActiveInvites(createdBy string) ([]Invite, error)
```

Implemented on `PebbleStore`, `SQLiteStore`, `MockStore`.

## Request-scoped plumbing (depends on 4.4)

Typed context helpers (pair with 4.4's DI):

```go
type ctxKey int
const (
    userKey ctxKey = iota
    permissionsKey
)

func WithUser(ctx context.Context, u *User) context.Context
func UserFromContext(ctx context.Context) (*User, bool)
func WithPermissions(ctx context.Context, perms []string) context.Context
func PermissionsFromContext(ctx context.Context) []string
func Can(ctx context.Context, perm string) bool
```

Middleware chain: authenticate → load user + permissions into ctx → `requirePerm` checks on routes → handler reads user via `UserFromContext`.

## Why PebbleDB (not MySQL/Postgres) for multi-user

Considered and rejected. Summary:

| SQL feature | Needed for multi-user? |
|---|---|
| Cross-table transactions | Covered by Pebble `Batch` |
| JOIN | No — all reads are point lookups or short prefix scans |
| Foreign key enforcement | App-level via batch writes |
| Row-level locking | No contention here |
| Full-text search | Orthogonal (Bleve, not auth) |
| Replication | Not in scope |

No meaningful loss. The open TODO.md §4.1 Postgres research is about *library* data (books/authors/dedup/reporting), not auth — and if the library ever migrates to Postgres, the multi-user tables come along for free through the `Store` interface.

## MVP scope (first shippable slice)

1. Schema: all PebbleDB keys listed above
2. Password auth + session cookies
3. Login / logout pages, session middleware
4. Roles + permissions + `requirePerm` middleware
5. First-run setup wizard (`/setup`) + CLI alt (`user create`)
6. Users admin page (list / create invite / reset password / deactivate)
7. `user_id` columns + one-time migration to default admin
8. CI grep to flag routes missing `requirePerm`

## Deferred to follow-up PRs

- **OAuth (GitHub + Google)** — separate PR, standard OAuth2 flow against existing middleware
- **JWT personal API keys** — separate PR, adds bearer auth alongside cookies
- **Request feature** (`requests.create`, `requests.approve`) — its own design + PR
- **2FA** — later
- **Email-based password reset** — later; admin-reset is enough for MVP
- **Rate-limiting beyond login lockout** — later, if abuse shows up

## Risks

- **Session invalidation on role change** — permission snapshots in session blobs need to be rewritten. Mitigation: on role change, call `DeleteUserSessions(userID)` to force re-login. Simple but users get kicked out.
- **First-run race** — if two admins hit `/setup` simultaneously. Mitigation: the `/setup` route checks `CountUsers() == 0` inside its handler and uses a mutex; once any user exists, route is disabled.
- **Backfilling `user_id`** — existing rows without a user. Migration script attributes to the first admin. For large operation/activity log histories this is a non-trivial one-time write; runs as a tracked operation.
- **Permission constants drift from role seeds** — if we add a new permission constant but forget to add it to seed roles, existing admins don't have it. Mitigation: seed roles are recomputed at startup (`upsert` behavior) — admin always gets the full set of known permissions.

## Non-goals

- Per-user libraries
- Per-user settings (aside from self-edit of password + API keys)
- Multi-tenancy (one org per server is all we need)
- Delegation / impersonation
- Audit of *reads* (only mutating actions recorded)

## Open implementation questions (deferred)

- OAuth provider whitelist enforced per-install? (probably a config flag, not a compile constant)
- Session expiry default — 30 days? (match typical SaaS)
- API key scopes — are personal API keys scoped to a subset of the user's permissions, or always full? (probably scoped)
- Permission inheritance / negation — not supported, deliberately keep flat
