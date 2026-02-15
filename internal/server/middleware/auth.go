// file: internal/server/middleware/auth.go
// version: 1.0.0
// guid: 83c42ecb-1df2-4baf-9890-3f91ab4db6fe

package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
		c.Next()
	}
}
