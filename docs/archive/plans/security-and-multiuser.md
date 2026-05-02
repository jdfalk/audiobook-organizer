<!-- file: docs/plans/security-and-multiuser.md -->
<!-- version: 2.0.0 -->
<!-- guid: d9e0f1a2-b3c4-5d6e-7f8a-9b0c1d2e3f4a -->
<!-- last-edited: 2026-01-31 -->

# Security and Multi-User

## Overview

Security hardening for the current single-user deployment, plus the full
multi-user architecture for when that becomes a requirement. All items are
post-MVP / future.

---

## Security Hardening (Near-Term)

### Content Security Policy

Add CSP headers to all responses that serve the React SPA. The middleware
lives in `internal/server/server.go` alongside the existing `corsMiddleware()`
(line 816). It must be added to the router in `NewServer()` (line 454–469)
immediately after `corsMiddleware()`.

**File: `internal/server/server.go`** — add the following middleware function
at the package level (place it right after `corsMiddleware`):

```go
// cspMiddleware sets a Content-Security-Policy header on every response.
//
// Directive rationale for this application:
//   default-src 'self'       — blocks everything not explicitly allowed.
//   script-src  'self'       — React bundle served from same origin; no CDN scripts.
//                 'unsafe-inline' — MUI's JSS runtime injects <style> tags that
//                                   browsers treat as inline scripts in some edge cases;
//                                   remove once MUI migrates fully to emotion/static CSS.
//   style-src   'self' 'unsafe-inline' — MUI emotion runtime generates inline styles.
//   img-src     'self' data: https: — cover art may be fetched from HTTPS origins or
//                                      embedded as data: URIs.
//   font-src    'self' https:       — system fonts and potential CDN fallbacks.
//   connect-src 'self'              — fetch/XHR to same origin only (API calls).
//                                     Add specific external origins here if metadata
//                                     providers are ever proxied client-side.
//   frame-ancestors 'none'          — this app is never embedded in an iframe.
func cspMiddleware() gin.HandlerFunc {
	policy := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: https:",
		"font-src 'self' https:",
		"connect-src 'self'",
		"frame-ancestors 'none'",
	}, "; ")

	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", policy)
		// X-Content-Type-Options prevents MIME sniffing of uploaded files.
		c.Header("X-Content-Type-Options", "nosniff")
		// X-Frame-Options is a legacy header; keep for older browsers.
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	}
}
```

**Wire it in `NewServer()`** — add the call after `corsMiddleware()`:

```go
func NewServer() *Server {
	router := gin.Default()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(cspMiddleware())   // ← insert here

	metrics.Register()
	server := &Server{router: router}
	server.setupRoutes()
	return server
}
```

---

### Path Traversal Defense

The current `browseFilesystem` handler (line 1880 of `internal/server/server.go`)
calls `filepath.Abs(path)` but does not reject paths that escape a
designated root. An attacker can pass `path=../../../../etc` and the handler
will happily list `/etc`.

**File: `internal/server/server.go`** — add a validation helper and replace
the existing path handling in `browseFilesystem`.

```go
// validateAndCanonicalizeFilePath resolves a user-supplied path to an absolute,
// cleaned path and verifies it does not escape the allowed root directories.
//
// allowedRoots is the set of top-level directories the user is permitted to
// browse.  For this application that is: config.AppConfig.RootDir and every
// path in the import paths list.  If allowedRoots is empty the check is
// skipped (permissive single-user mode).
//
// Returns the canonical absolute path on success, or an error describing why
// the path was rejected.
func validateAndCanonicalizeFilePath(rawPath string, allowedRoots []string) (string, error) {
	if strings.TrimSpace(rawPath) == "" {
		return "", fmt.Errorf("path must not be empty")
	}

	// 1. Resolve to absolute, cleaning all `.` and `..` segments.
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	canonical := filepath.Clean(abs)

	// 2. Reject if the canonical path contains residual traversal markers.
	//    filepath.Clean removes them, but a second check guards against
	//    symlink-based tricks on some OS configurations.
	if strings.Contains(canonical, "..") {
		return "", fmt.Errorf("path traversal detected: resolved path contains '..'")
	}

	// 3. If allowedRoots is provided, verify the canonical path is under
	//    at least one allowed root.
	if len(allowedRoots) > 0 {
		allowed := false
		for _, root := range allowedRoots {
			cleanRoot := filepath.Clean(root)
			// Use HasPrefix with a trailing separator to avoid matching
			// /data/books-backup when root is /data/books.
			if canonical == cleanRoot || strings.HasPrefix(canonical, cleanRoot+string(filepath.Separator)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("path %q is outside all allowed directories", canonical)
		}
	}

	return canonical, nil
}
```

**Replace the path handling in `browseFilesystem`** (line 1881–1892):

```go
func (s *Server) browseFilesystem(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter is required"})
		return
	}

	// Build allowed roots: RootDir + all import paths
	allowedRoots := []string{}
	if config.AppConfig.RootDir != "" {
		allowedRoots = append(allowedRoots, config.AppConfig.RootDir)
	}
	if database.GlobalStore != nil {
		if importPaths, err := database.GlobalStore.GetAllImportPaths(); err == nil {
			for _, ip := range importPaths {
				allowedRoots = append(allowedRoots, ip.Path)
			}
		}
	}
	// Also allow the user's home directory (used by the wizard browser)
	if home, err := os.UserHomeDir(); err == nil {
		allowedRoots = append(allowedRoots, home)
	}

	absPath, err := validateAndCanonicalizeFilePath(path, allowedRoots)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ... rest of the handler unchanged (ReadDir on absPath, etc.)
}
```

**Test cases** (place in `internal/server/server_path_traversal_test.go`):

```go
func TestValidateAndCanonicalizeFilePath(t *testing.T) {
	roots := []string{"/data/audiobooks", "/data/imports"}

	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
	}{
		{"valid path under root",        "/data/audiobooks/fiction",       false, ""},
		{"exact root",                   "/data/audiobooks",              false, ""},
		{"traversal to /etc",            "/data/audiobooks/../../etc",    true,  "outside all allowed"},
		{"traversal with encoding trick","/data/audiobooks/..%2F..%2Fetc", true, ""},  // URL-decoded before reaching handler
		{"empty path",                   "",                              true,  "must not be empty"},
		{"path under second root",       "/data/imports/sub",             false, ""},
		{"sibling directory attack",     "/data/audiobooks-secret",       true,  "outside all allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateAndCanonicalizeFilePath(tt.input, roots)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.errSubstr != "" && err != nil && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("error %q does not contain %q", err, tt.errSubstr)
			}
		})
	}
}
```

---

### Audit Log

Record every configuration change with the old value, new value, and
timestamp. Store entries in PebbleDB using a time-sortable key pattern.

**File: `internal/server/audit.go`** — create this file.

```go
// file: internal/server/audit.go
package server

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

// AuditEntry represents a single auditable event.
type AuditEntry struct {
	ID        string    `json:"id"`         // ULID for time-sortable ordering
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`     // e.g. "config.update"
	Field     string    `json:"field"`      // the specific config key changed
	OldValue  string    `json:"old_value"`  // JSON-encoded previous value
	NewValue  string    `json:"new_value"`  // JSON-encoded new value
	ActorIP   string    `json:"actor_ip"`   // client IP (single-user; extended for multi-user)
}

// PebbleDB key pattern:
//   audit:<ULID>
//
// ULID encodes a timestamp in its first 48 bits, so lexicographic iteration
// over keys with prefix "audit:" yields entries in chronological order.
// This is the same pattern used for operation:<id> keys (see pebble_store.go).

func auditKey(id string) []byte {
	return []byte(fmt.Sprintf("audit:%s", id))
}

// WriteAuditEntry persists an audit entry to PebbleDB via the global store's
// SetSetting interface.  We use SetSetting with a synthetic key because the
// Store interface does not yet have a dedicated audit method.  A dedicated
// method (AddAuditEntry) should be added to the Store interface and
// implemented in pebble_store.go following the same pattern as
// AddOperationLog.
func WriteAuditEntry(entry AuditEntry) error {
	if database.GlobalStore == nil {
		return fmt.Errorf("database not initialized")
	}
	if entry.ID == "" {
		entry.ID = ulid.Make().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	// Store as a setting with key "audit:<ULID>" and value = JSON.
	// typ="audit", isSecret=false.
	return database.GlobalStore.SetSetting(
		fmt.Sprintf("audit:%s", entry.ID),
		string(data),
		"audit",
		false,
	)
}

// readAuditEntries retrieves audit entries.  The Store.GetAllSettings method
// returns all settings; filter by key prefix "audit:" and deserialize.
func readAuditEntries(limit, offset int) ([]AuditEntry, int, error) {
	if database.GlobalStore == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	allSettings, err := database.GlobalStore.GetAllSettings()
	if err != nil {
		return nil, 0, err
	}

	var entries []AuditEntry
	for _, s := range allSettings {
		if len(s.Key) < 6 || s.Key[:6] != "audit:" {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal([]byte(s.Value), &entry); err != nil {
			continue // skip malformed entries
		}
		entries = append(entries, entry)
	}

	total := len(entries)
	// Paginate
	if offset >= total {
		return []AuditEntry{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return entries[offset:end], total, nil
}
```

**File: `internal/server/server.go`** — hook audit logging into
`updateConfig`. After the existing field-by-field update block (around line
3084–3210), before the `SaveConfigToDatabase` call, emit an audit entry for
each changed field. Add this block:

```go
// --- Audit logging: emit one entry per updated field ---
clientIP := c.ClientIP()
for _, field := range updated {
	entry := AuditEntry{
		Action:  "config.update",
		Field:   field,
		ActorIP: clientIP,
		// OldValue / NewValue: populated by comparing config snapshots.
		// For a minimal first cut, log the new value only.
		NewValue: fmt.Sprintf("%v", updates[field]),
	}
	if err := WriteAuditEntry(entry); err != nil {
		log.Printf("[WARN] failed to write audit entry for %s: %v", field, err)
		// Non-fatal: the config update still succeeds.
	}
}
```

For full old-vs-new tracking, snapshot `config.AppConfig` into a
`map[string]interface{}` **before** the update loop begins, then compare
after. This is the recommended next step.

**API endpoint** — add a route in `setupRoutes()`:

```go
api.GET("/audit", s.listAuditEntries)
```

Handler:

```go
func (s *Server) listAuditEntries(c *gin.Context) {
	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	entries, total, err := readAuditEntries(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}
```

---

### Secret Scanning

Reject config update payloads that contain values matching common API-key
patterns.  This prevents accidental commit of secrets into config when a
developer pastes a test key.

**File: `internal/server/secret_scan.go`** — create this file.

```go
// file: internal/server/secret_scan.go
package server

import (
	"fmt"
	"regexp"
	"strings"
)

// secretPattern defines a compiled regex and a human-readable label.
type secretPattern struct {
	label   string
	pattern *regexp.Regexp
}

// secretPatterns covers the most common API-key and credential formats.
var secretPatterns = []secretPattern{
	{
		label:   "OpenAI API key",
		// sk-... keys are 51 chars; also match the newer org-scoped prefix.
		pattern: regexp.MustCompile(`\bsk-(?:org-)?[A-Za-z0-9]{48,}\b`),
	},
	{
		label:   "AWS access key ID",
		pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	},
	{
		label:   "Generic high-entropy token (base64, 40+ chars)",
		// Matches long base64 strings that look like bearer tokens.
		// Threshold of 40 chars avoids false positives on normal strings.
		pattern: regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`),
	},
	{
		label:   "Private key PEM header",
		pattern: regexp.MustCompile(`-----BEGIN\s+(RSA |EC )?PRIVATE KEY-----`),
	},
}

// ScanForSecrets inspects every string value in the update payload.
// Returns a list of human-readable findings.  An empty slice means clean.
//
// The caller (updateConfig handler) should reject the request when findings
// are non-empty and return a 400 with the findings in the response body.
func ScanForSecrets(updates map[string]interface{}) []string {
	var findings []string

	// Walk the map recursively; only string leaf values are scanned.
	var walk func(path string, v interface{})
	walk = func(path string, v interface{}) {
		switch val := v.(type) {
		case string:
			// Skip known-safe fields that legitimately hold key-like values.
			// The openai_api_key field is whitelisted because it is the
			// intentional destination for that secret.
			if path == "openai_api_key" || strings.HasSuffix(path, ".goodreads") {
				return
			}
			for _, sp := range secretPatterns {
				if sp.pattern.MatchString(val) {
					findings = append(findings,
						fmt.Sprintf("field %q appears to contain a %s", path, sp.label))
				}
			}
		case map[string]interface{}:
			for k, child := range val {
				childPath := path + "." + k
				if path == "" {
					childPath = k
				}
				walk(childPath, child)
			}
		case []interface{}:
			for i, child := range val {
				walk(fmt.Sprintf("%s[%d]", path, i), child)
			}
		}
	}

	walk("", updates)
	return findings
}
```

**Hook into `updateConfig`** — insert this check at the very top of the
handler, right after `ShouldBindJSON` succeeds (after line 3081):

```go
// Secret scanning: reject payloads that look like they contain leaked secrets.
if findings := ScanForSecrets(updates); len(findings) > 0 {
	c.JSON(http.StatusBadRequest, gin.H{
		"error":    "config update rejected: potential secret detected in payload",
		"findings": findings,
	})
	return
}
```

---

### Dependency Vulnerability Updates

- Automated PR creation for dependencies with known CVEs

---

## Multi-User Architecture (Future)

### User Management

The `Store` interface (`internal/database/store.go` lines 102–106) already
defines `CreateUser`, `GetUserByID`, `GetUserByUsername`, `GetUserByEmail`,
and `UpdateUser`.  The `types/index.ts` frontend type already has a `User`
interface.  The database layer is ready; the missing pieces are the HTTP
middleware and frontend integration.

**Go User model** (already in database package, referenced by Store):

```go
// User is already defined in the database package.  For reference:
type User struct {
    ID             string   `json:"id"`              // ULID
    Username       string   `json:"username"`
    Email          string   `json:"email"`
    PasswordHash   string   `json:"-"`               // never serialized to API responses
    PasswordHashAlgo string `json:"password_hash_algo"` // e.g. "bcrypt"
    Roles          []string `json:"roles"`           // ["admin"], ["user"], ["readonly"]
    Status         string   `json:"status"`          // "active" | "suspended"
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}
```

### Authentication — JWT Middleware

**File: `internal/server/auth.go`** — create this file when multi-user mode is
enabled.

```go
// file: internal/server/auth.go
package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// jwtSecret should be loaded from config or generated at startup and stored
// in the database settings table.  Never hardcode.
var jwtSecret []byte

// JWTClaims is the payload embedded in the token.
type JWTClaims struct {
	UserID    string   `json:"user_id"`
	Username  string   `json:"username"`
	Roles     []string `json:"roles"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
}

// signJWT produces a minimal HS256 JWT.  In production, replace with a
// well-tested library (e.g. golang-jwt/jwt).  This is a reference
// implementation showing the structure.
func signJWT(claims JWTClaims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"alg":"HS256","typ":"JWT"}`),
	)
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)

	signingInput := header + "." + payloadB64
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

// jwtAuthMiddleware extracts and validates the JWT from the Authorization
// header.  It sets gin context keys "user_id", "user_roles" for downstream
// handlers.  If the token is missing or invalid it aborts with 401.
//
// This middleware is NOT added to the router when multi-user mode is disabled.
// The router setup in setupRoutes() should conditionally wrap the api group:
//
//     if config.AppConfig.EnableMultiUser {
//         api.Use(jwtAuthMiddleware())
//     }
func jwtAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing Authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid Authorization header format"})
			return
		}
		token := parts[1]

		// Split token into segments
		segments := strings.Split(token, ".")
		if len(segments) != 3 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "malformed token"})
			return
		}

		// Verify signature
		signingInput := segments[0] + "." + segments[1]
		mac := hmac.New(sha256.New, jwtSecret)
		mac.Write([]byte(signingInput))
		expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(segments[2]), []byte(expectedSig)) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token signature"})
			return
		}

		// Decode claims
		payloadBytes, err := base64.RawURLEncoding.DecodeString(segments[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "malformed token payload"})
			return
		}
		var claims JWTClaims
		if err := json.Unmarshal(payloadBytes, &claims); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "malformed token claims"})
			return
		}

		// Check expiration
		if time.Now().Unix() > claims.ExpiresAt {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
			return
		}

		// Attach to context for downstream RBAC checks
		c.Set("user_id", claims.UserID)
		c.Set("user_roles", claims.Roles)
		c.Next()
	}
}
```

### RBAC — Role-Based Access Control on Handlers

Use a decorator (middleware factory) pattern so individual handlers can
declare the minimum role required without modifying their own logic.

```go
// requireRole returns a gin middleware that checks the "user_roles" context
// value (set by jwtAuthMiddleware) and aborts with 403 if the required role
// is not present.
//
// Usage in route registration:
//
//     api.PUT("/config", requireRole("admin"), s.updateConfig)
//     api.GET("/audiobooks", requireRole("user"), s.listAudiobooks)
//
// Role hierarchy (lowest to highest): readonly < user < admin
var roleHierarchy = map[string]int{
	"readonly": 1,
	"user":     2,
	"admin":    3,
}

func requireRole(minRole string) gin.HandlerFunc {
	minLevel, ok := roleHierarchy[minRole]
	if !ok {
		panic(fmt.Sprintf("unknown role: %s", minRole))
	}

	return func(c *gin.Context) {
		rolesRaw, exists := c.Get("user_roles")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no authenticated user"})
			return
		}
		roles, _ := rolesRaw.([]string)

		for _, r := range roles {
			if level, ok := roleHierarchy[r]; ok && level >= minLevel {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":         "insufficient permissions",
			"required_role": minRole,
			"user_roles":    roles,
		})
	}
}
```

**Route registration pattern** when multi-user is enabled:

```go
if config.AppConfig.EnableMultiUser {
	api.Use(jwtAuthMiddleware())
}

// Read-only endpoints: any authenticated user
api.GET("/audiobooks", requireRole("user"), s.listAudiobooks)
api.GET("/audiobooks/:id", requireRole("user"), s.getAudiobook)

// Mutating endpoints: admin only
api.PUT("/config", requireRole("admin"), s.updateConfig)
api.DELETE("/audiobooks/:id", requireRole("admin"), s.deleteAudiobook)
api.POST("/operations/scan", requireRole("admin"), s.startScan)
```

The `requireRole` middleware is a no-op when multi-user is disabled (the
middleware is simply not registered on the router group).

### SSL/TLS

- HTTPS support with certificate management (already partially implemented —
  see `ServerConfig.TLSCertFile` / `TLSKeyFile` in `internal/server/server.go`
  lines 448–449 and the TLS setup block at lines 486–522)
- Let's Encrypt integration for automatic certificate provisioning
- Self-signed certificate generation for local/air-gapped deployments
- Configurable cipher suites and TLS versions

---

## Dependencies

- Multi-user depends on a stable single-user product being shipped first
- SSL/TLS can be implemented independently of multi-user (partial support
  already exists in the server config)
- Security hardening items (CSP, path traversal, audit log, secret scanning)
  are independent of each other and of multi-user

## References

- Server and route registration: `internal/server/server.go`
- Middleware chain entry point: `NewServer()` in `internal/server/server.go`
- Config struct: `internal/config/config.go`
- Database Store interface: `internal/database/store.go`
- PebbleDB key patterns: `internal/database/pebble_store.go` (prefix conventions:
  `book:<id>`, `operation:<id>`, `author:<id>`, `work:<id>`, `import_path:<id>`)
- Filesystem browse handler: `internal/server/server.go` (browseFilesystem)
- Config update handler: `internal/server/server.go` (updateConfig)
