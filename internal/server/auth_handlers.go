// file: internal/server/auth_handlers.go
// version: 1.1.0
// guid: 1457df2f-af76-46cb-a2f4-c9f6f275f93a

package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	servermiddleware "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
	"golang.org/x/crypto/bcrypt"
)

const defaultSessionTTL = 24 * time.Hour

type authUserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Roles     []string  `json:"roles"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func buildAuthUserResponse(user *database.User) authUserResponse {
	return authUserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		Roles:     user.Roles,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	}
}

func isHTTPSRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}

func setSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	if c == nil {
		return
	}
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     servermiddleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isHTTPSRequest(c),
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     servermiddleware.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPSRequest(c),
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) getAuthStatus(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	count, err := database.GlobalStore.CountUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read auth status"})
		return
	}
	authEnabled := config.AppConfig.EnableAuth
	requiresAuth := authEnabled && count > 0
	c.JSON(http.StatusOK, gin.H{
		"has_users":       count > 0,
		"auth_enabled":    authEnabled,
		"requires_auth":   requiresAuth,
		"bootstrap_ready": authEnabled && count == 0,
	})
}

func (s *Server) setupInitialAdmin(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	if req.Username == "" || len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password (min 8 chars) are required"})
		return
	}
	if req.Email == "" {
		req.Email = req.Username + "@local"
	}

	count, err := database.GlobalStore.CountUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check existing users"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "initial setup already completed"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	created, err := database.GlobalStore.CreateUser(req.Username, req.Email, "bcrypt", string(hash), []string{"admin"}, "active")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to create initial user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "admin user created",
		"user":    buildAuthUserResponse(created),
	})
}

func (s *Server) login(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	user, err := database.GlobalStore.GetUserByUsername(req.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	session, err := database.GlobalStore.CreateSession(
		user.ID,
		strings.TrimSpace(c.ClientIP()),
		strings.TrimSpace(c.Request.UserAgent()),
		defaultSessionTTL,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	setSessionCookie(c, session.ID, session.ExpiresAt)
	c.JSON(http.StatusOK, gin.H{
		"user":    buildAuthUserResponse(user),
		"session": session,
	})
}

func (s *Server) me(c *gin.Context) {
	user, ok := servermiddleware.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": buildAuthUserResponse(user)})
}

func (s *Server) logout(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	session, ok := servermiddleware.CurrentSession(c)
	if ok && session != nil {
		_ = database.GlobalStore.RevokeSession(session.ID)
	}
	clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (s *Server) listMySessions(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	user, ok := servermiddleware.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	currentSession, _ := servermiddleware.CurrentSession(c)

	sessions, err := database.GlobalStore.ListUserSessions(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	type sessionView struct {
		database.Session
		Current bool `json:"current"`
	}
	response := make([]sessionView, 0, len(sessions))
	for _, session := range sessions {
		response = append(response, sessionView{
			Session: session,
			Current: currentSession != nil && session.ID == currentSession.ID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"sessions": response})
}

func (s *Server) revokeMySession(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	user, ok := servermiddleware.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	currentSession, _ := servermiddleware.CurrentSession(c)

	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id required"})
		return
	}

	targetSession, err := database.GlobalStore.GetSession(sessionID)
	if err != nil || targetSession == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if targetSession.UserID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot revoke another user's session"})
		return
	}

	if err := database.GlobalStore.RevokeSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke session"})
		return
	}

	if currentSession != nil && currentSession.ID == sessionID {
		clearSessionCookie(c)
	}
	c.Status(http.StatusNoContent)
}
