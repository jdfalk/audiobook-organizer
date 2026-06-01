// file: internal/server/auth_handlers.go
// version: 2.3.0
// guid: 1457df2f-af76-46cb-a2f4-c9f6f275f93a
// last-edited: 2026-06-01

package server

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/server/handlers"
	servermiddleware "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxFailedLogins      = 10
	lockoutWindowMinutes = 15
)

type failedAttempt struct {
	count   int
	firstAt time.Time
}

var (
	loginLockoutMu sync.Mutex
	loginLockout   = map[string]*failedAttempt{}
)

func isLockedOut(userID string) bool {
	loginLockoutMu.Lock()
	defer loginLockoutMu.Unlock()
	a, ok := loginLockout[userID]
	if !ok {
		return false
	}
	if time.Since(a.firstAt) > lockoutWindowMinutes*time.Minute {
		delete(loginLockout, userID)
		return false
	}
	return a.count >= maxFailedLogins
}

func recordFailedLogin(userID string) {
	loginLockoutMu.Lock()
	defer loginLockoutMu.Unlock()
	a, ok := loginLockout[userID]
	if !ok || time.Since(a.firstAt) > lockoutWindowMinutes*time.Minute {
		loginLockout[userID] = &failedAttempt{count: 1, firstAt: time.Now()}
		return
	}
	a.count++
}

func clearFailedLogins(userID string) {
	loginLockoutMu.Lock()
	defer loginLockoutMu.Unlock()
	delete(loginLockout, userID)
}

const (
	defaultSessionTTL    = 24 * time.Hour
	rememberMeSessionTTL = 7 * 24 * time.Hour
	tempLoginTokenTTL    = 15 * time.Minute
)

func buildAuthUserResponse(user *database.User) handlers.AuthUserResponse {
	return handlers.AuthUserResponse{
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
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	count, err := s.Store().CountUsers()
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to read auth status")
		return
	}
	authEnabled := config.AppConfig.EnableAuth
	requiresAuth := authEnabled && count > 0
	httputil.RespondWithOK(c, gin.H{
		"has_users":       count > 0,
		"auth_enabled":    authEnabled,
		"requires_auth":   requiresAuth,
		"bootstrap_ready": authEnabled && count == 0,
	})
}

func (s *Server) setupInitialAdmin(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	if req.Username == "" || len(req.Password) < 8 {
		httputil.RespondWithBadRequest(c, "username and password (min 8 chars) are required")
		return
	}
	if req.Email == "" {
		req.Email = req.Username + "@local"
	}

	count, err := s.Store().CountUsers()
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to check existing users")
		return
	}
	if count > 0 {
		httputil.RespondWithConflict(c, "initial setup already completed")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to hash password")
		return
	}

	created, err := s.Store().CreateUser(req.Username, req.Email, "bcrypt", string(hash), []string{"admin"}, "active")
	if err != nil {
		httputil.RespondWithBadRequest(c, "failed to create initial user")
		return
	}

	httputil.RespondWithCreated(c, gin.H{
		"message": "admin user created",
		"user":    buildAuthUserResponse(created),
	})
}

func (s *Server) login(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		httputil.RespondWithBadRequest(c, "username and password are required")
		return
	}

	user, err := s.Store().GetUserByUsername(req.Username)
	if err != nil || user == nil {
		httputil.RespondWithUnauthorized(c, "invalid credentials")
		return
	}

	if isLockedOut(user.ID) {
		httputil.RespondWithError(c, 429, "account temporarily locked — try again later", "LOCKOUT")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		recordFailedLogin(user.ID)
		httputil.RespondWithUnauthorized(c, "invalid credentials")
		return
	}
	clearFailedLogins(user.ID)

	ttl := defaultSessionTTL
	if req.RememberMe {
		ttl = rememberMeSessionTTL
	}
	session, err := s.Store().CreateSession(
		user.ID,
		strings.TrimSpace(c.ClientIP()),
		strings.TrimSpace(c.Request.UserAgent()),
		ttl,
	)
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to create session")
		return
	}

	setSessionCookie(c, session.ID, session.ExpiresAt)
	httputil.RespondWithOK(c, gin.H{
		"user":    buildAuthUserResponse(user),
		"session": session,
	})
}

func (s *Server) me(c *gin.Context) {
	user, ok := servermiddleware.CurrentUser(c)
	if !ok {
		httputil.RespondWithUnauthorized(c, "not authenticated")
		return
	}
	httputil.RespondWithOK(c, gin.H{"user": buildAuthUserResponse(user)})
}

// updateMe handles PATCH /api/v1/auth/me.
// Allows the current user to update their own email address.
func (s *Server) updateMe(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	user, ok := servermiddleware.CurrentUser(c)
	if !ok {
		httputil.RespondWithUnauthorized(c, "not authenticated")
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		httputil.RespondWithBadRequest(c, "email is required")
		return
	}

	user.Email = email
	if err := s.Store().UpdateUser(user); err != nil {
		httputil.RespondWithInternalError(c, "failed to update profile")
		return
	}

	httputil.RespondWithOK(c, gin.H{"user": buildAuthUserResponse(user)})
}

func (s *Server) logout(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	session, ok := servermiddleware.CurrentSession(c)
	if ok && session != nil {
		_ = s.Store().RevokeSession(session.ID)
	}
	clearSessionCookie(c)
	httputil.RespondWithOK(c, gin.H{"message": "logged out"})
}

func (s *Server) listMySessions(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	user, ok := servermiddleware.CurrentUser(c)
	if !ok {
		httputil.RespondWithUnauthorized(c, "not authenticated")
		return
	}
	currentSession, _ := servermiddleware.CurrentSession(c)

	sessions, err := s.Store().ListUserSessions(user.ID)
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to list sessions")
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
	httputil.RespondWithOK(c, gin.H{"sessions": response})
}

func (s *Server) revokeMySession(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	user, ok := servermiddleware.CurrentUser(c)
	if !ok {
		httputil.RespondWithUnauthorized(c, "not authenticated")
		return
	}
	currentSession, _ := servermiddleware.CurrentSession(c)

	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		httputil.RespondWithBadRequest(c, "session id required")
		return
	}

	targetSession, err := s.Store().GetSession(sessionID)
	if err != nil || targetSession == nil {
		httputil.RespondWithNotFound(c, "session", sessionID)
		return
	}
	if targetSession.UserID != user.ID {
		httputil.RespondWithForbidden(c, "cannot revoke another user's session")
		return
	}

	if err := s.Store().RevokeSession(sessionID); err != nil {
		httputil.RespondWithInternalError(c, "failed to revoke session")
		return
	}

	if currentSession != nil && currentSession.ID == sessionID {
		clearSessionCookie(c)
	}
	httputil.RespondWithNoContent(c)
}

// changePassword handles PUT /api/v1/auth/me/password.
// Users change their own password by providing their current password and a new one.
// Admins (PermUsersManage) may omit current_password to reset another user's password
// by also providing a user_id field.
func (s *Server) changePassword(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	caller, _ := servermiddleware.CurrentUser(c)
	if caller == nil {
		httputil.RespondWithUnauthorized(c, "not authenticated")
		return
	}

	var req struct {
		UserID          string `json:"user_id"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if len(req.NewPassword) < 8 {
		httputil.RespondWithBadRequest(c, "new password must be at least 8 characters")
		return
	}

	// Determine target user.
	targetID := caller.ID
	isAdminReset := false
	if req.UserID != "" && req.UserID != caller.ID {
		// Only admins may reset another user's password.
		isAdmin := false
		for _, roleRef := range caller.Roles {
			r, _ := s.Store().GetRoleByName(roleRef)
			if r == nil {
				r, _ = s.Store().GetRoleByID(roleRef)
			}
			if r == nil {
				continue
			}
			for _, p := range r.Permissions {
				if p == "users.manage" {
					isAdmin = true
				}
			}
		}
		if !isAdmin {
			httputil.RespondWithForbidden(c, "only admins can reset another user's password")
			return
		}
		targetID = req.UserID
		isAdminReset = true
	}

	target, err := s.Store().GetUserByID(targetID)
	if err != nil || target == nil {
		httputil.RespondWithNotFound(c, "user", targetID)
		return
	}

	// Non-admin users must verify their current password.
	if !isAdminReset {
		if req.CurrentPassword == "" {
			httputil.RespondWithBadRequest(c, "current_password is required")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(target.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			httputil.RespondWithUnauthorized(c, "current password is incorrect")
			return
		}
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to hash password")
		return
	}

	target.PasswordHash = string(newHash)
	target.PasswordHashAlgo = "bcrypt"
	if err := s.Store().UpdateUser(target); err != nil {
		httputil.RespondWithInternalError(c, "failed to update password")
		return
	}

	httputil.RespondWithNoContent(c)
}
