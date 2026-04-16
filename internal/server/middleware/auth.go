// file: internal/server/middleware/auth.go
// version: 1.1.0
// guid: 83c42ecb-1df2-4baf-9890-3f91ab4db6fe

package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

const (
	// SessionCookieName is the auth session cookie used by API clients.
	SessionCookieName = "session_id"
	contextUserKey    = "auth_user"
	contextSessionKey = "auth_session"
)

// SessionTokenFromRequest extracts the session token from Bearer auth or cookie.
func SessionTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		token := strings.TrimSpace(authHeader[len("Bearer "):])
		if token != "" {
			return token
		}
	}
	if cookie, err := r.Cookie(SessionCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

// CurrentUser fetches the authenticated user from Gin context.
func CurrentUser(c *gin.Context) (*database.User, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(contextUserKey)
	if !ok || value == nil {
		return nil, false
	}
	user, ok := value.(*database.User)
	return user, ok && user != nil
}

// CurrentSession fetches the authenticated session from Gin context.
func CurrentSession(c *gin.Context) (*database.Session, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(contextSessionKey)
	if !ok || value == nil {
		return nil, false
	}
	session, ok := value.(*database.Session)
	return session, ok && session != nil
}

// RequireAuth enforces session-based auth when at least one user exists.
func RequireAuth(store database.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store == nil {
			c.Next()
			return
		}

		userCount, err := store.CountUsers()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check auth state"})
			c.Abort()
			return
		}
		if userCount == 0 {
			// First-run bootstrap mode: setup endpoint can create the first admin.
			c.Next()
			return
		}

		token := SessionTokenFromRequest(c.Request)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}

		session, err := store.GetSession(token)
		if err != nil || session == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			c.Abort()
			return
		}
		if session.Revoked || time.Now().After(session.ExpiresAt) {
			_ = store.RevokeSession(token)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
			c.Abort()
			return
		}

		user, err := store.GetUserByID(session.UserID)
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session user"})
			c.Abort()
			return
		}
		if status := strings.ToLower(strings.TrimSpace(user.Status)); status != "" && status != "active" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "inactive user"})
			c.Abort()
			return
		}

		c.Set(contextUserKey, user)
		c.Set(contextSessionKey, session)

		// Also attach user + effective permissions on the request
		// context using the typed helpers from internal/auth so
		// handlers can call auth.Can(ctx, perm) directly. Permissions
		// are the union of every role's permission list — computed
		// on each request. Spec 3.7 calls for caching this on the
		// session blob for perf; that optimization is a follow-up.
		perms := effectivePermissionsFor(store, user)
		ctx := auth.WithUser(c.Request.Context(), user)
		ctx = auth.WithPermissions(ctx, perms)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// effectivePermissionsFor resolves the union of every role's
// Permissions slice for the given user. Unknown roles are skipped.
// Returns nil if the user has no roles (which makes every Can()
// check return false — safe default).
func effectivePermissionsFor(store database.Store, user *database.User) []auth.Permission {
	if user == nil || len(user.Roles) == 0 || store == nil {
		return nil
	}
	seen := make(map[auth.Permission]struct{})
	for _, roleID := range user.Roles {
		role, err := store.GetRoleByID(roleID)
		if err != nil || role == nil {
			continue
		}
		for _, p := range role.Permissions {
			seen[p] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]auth.Permission, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

// RequirePermission returns a middleware that aborts with 403 if the
// calling user doesn't have permission p. Must be chained after
// RequireAuth (which loads the user + permission set into the
// request context).
//
// Exception: if no users exist yet (first-run bootstrap), the check
// is bypassed so the /setup wizard can run unauthenticated.
func RequirePermission(store database.Store, p auth.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		// First-run bypass — RequireAuth uses the same pattern.
		if store != nil {
			if n, err := store.CountUsers(); err == nil && n == 0 {
				c.Next()
				return
			}
		}
		if !auth.Can(c.Request.Context(), p) {
			// Distinguish unauth (no user on ctx) from authz fail
			// (user present but missing perm) so API clients get a
			// meaningful status code.
			if _, ok := auth.UserFromContext(c.Request.Context()); !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			} else {
				c.JSON(http.StatusForbidden, gin.H{"error": "permission denied: " + p})
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAdmin is a middleware that checks the authenticated user has the "admin" role.
// Must be used after RequireAuth in the middleware chain.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}
		for _, role := range user.Roles {
			if role == "admin" {
				c.Next()
				return
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
		c.Abort()
	}
}
