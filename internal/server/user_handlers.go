// file: internal/server/user_handlers.go
// version: 1.0.0
// guid: 2d0e1f8a-3b9c-4a70-b8c5-3d7e0f1b9a99
//
// User management admin endpoints (spec 3.7 task 7). All routes
// gated on `users.manage` permission via s.perm().

package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"golang.org/x/crypto/bcrypt"
)

// handleListUsers returns all users (admin only).
// GET /api/v1/users
func (s *Server) handleListUsers(c *gin.Context) {
	users, err := s.Store().ListUsers()
	if err != nil {
		internalError(c, "failed to list users", err)
		return
	}
	if users == nil {
		users = []database.User{}
	}
	safe := make([]gin.H, 0, len(users))
	for _, u := range users {
		safe = append(safe, gin.H{
			"id": u.ID, "username": u.Username, "email": u.Email,
			"roles": u.Roles, "status": u.Status,
			"created_at": u.CreatedAt, "updated_at": u.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"users": safe, "count": len(safe)})
}

// handleCreateInvite generates an invite token.
// POST /api/v1/users/invite
func (s *Server) handleCreateInvite(c *gin.Context) {
	var req struct {
		Username  string `json:"username" binding:"required"`
		RoleID    string `json:"role_id" binding:"required"`
		ExpiresIn int    `json:"expires_in"` // hours, default 72
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token := generateToken()
	expiresIn := 72 * time.Hour
	if req.ExpiresIn > 0 {
		expiresIn = time.Duration(req.ExpiresIn) * time.Hour
	}

	invite := &database.Invite{
		Token:     token,
		Username:  strings.TrimSpace(req.Username),
		RoleID:    req.RoleID,
		ExpiresAt: time.Now().Add(expiresIn),
	}
	created, err := s.Store().CreateInvite(invite)
	if err != nil {
		internalError(c, "failed to create invite", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"invite": created, "token": token})
}

// handleListInvites returns all active (non-expired, non-used) invites.
// GET /api/v1/users/invites
func (s *Server) handleListInvites(c *gin.Context) {
	invites, err := s.Store().ListActiveInvites()
	if err != nil {
		internalError(c, "failed to list invites", err)
		return
	}
	if invites == nil {
		invites = []database.Invite{}
	}
	c.JSON(http.StatusOK, gin.H{"invites": invites, "count": len(invites)})
}

// handleDeleteInvite revokes an invite by token.
// DELETE /api/v1/users/invites/:token
func (s *Server) handleDeleteInvite(c *gin.Context) {
	token := c.Param("token")
	if err := s.Store().DeleteInvite(token); err != nil {
		internalError(c, "failed to delete invite", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": token})
}

// handleAcceptInvite consumes an invite token and creates a user.
// POST /api/v1/auth/accept-invite
func (s *Server) handleAcceptInvite(c *gin.Context) {
	var req struct {
		Token    string `json:"token" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(c, "hash password", err)
		return
	}

	user, err := s.Store().ConsumeInvite(req.Token, "bcrypt", string(hash))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sess, err := s.Store().CreateSession(user.ID, c.ClientIP(), c.Request.UserAgent(), 30*24*time.Hour)
	if err != nil {
		internalError(c, "create session", err)
		return
	}

	setSessionCookie(c, sess.ID, sess.ExpiresAt)
	c.JSON(http.StatusCreated, gin.H{"user": user, "session": sess})
}

// handleDeactivateUser soft-deactivates a user (sets status=locked).
// POST /api/v1/users/:id/deactivate
func (s *Server) handleDeactivateUser(c *gin.Context) {
	id := c.Param("id")
	user, err := s.Store().GetUserByID(id)
	if err != nil {
		internalError(c, "get user", err)
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	user.Status = "locked"
	if err := s.Store().UpdateUser(user); err != nil {
		internalError(c, "deactivate user", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// handleReactivateUser reactivates a locked user.
// POST /api/v1/users/:id/reactivate
func (s *Server) handleReactivateUser(c *gin.Context) {
	id := c.Param("id")
	user, err := s.Store().GetUserByID(id)
	if err != nil {
		internalError(c, "get user", err)
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	user.Status = "active"
	if err := s.Store().UpdateUser(user); err != nil {
		internalError(c, "reactivate user", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// handleResetPassword generates a new invite for an existing user
// so they can set a new password.
// POST /api/v1/users/:id/reset-password
func (s *Server) handleResetPassword(c *gin.Context) {
	id := c.Param("id")
	user, err := s.Store().GetUserByID(id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	token := generateToken()
	invite := &database.Invite{
		Token:     token,
		Username:  user.Username,
		RoleID:    "",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if _, err := s.Store().CreateInvite(invite); err != nil {
		internalError(c, "create reset invite", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "expires_at": invite.ExpiresAt})
}

// registerUserAdminRoutes wires user management endpoints.
func (s *Server) registerUserAdminRoutes(protected *gin.RouterGroup) {
	users := protected.Group("/users")
	{
		users.GET("", s.perm("users.manage"), s.handleListUsers)
		users.POST("/invite", s.perm("users.manage"), s.handleCreateInvite)
		users.GET("/invites", s.perm("users.manage"), s.handleListInvites)
		users.DELETE("/invites/:token", s.perm("users.manage"), s.handleDeleteInvite)
		users.POST("/:id/deactivate", s.perm("users.manage"), s.handleDeactivateUser)
		users.POST("/:id/reactivate", s.perm("users.manage"), s.handleReactivateUser)
		users.POST("/:id/reset-password", s.perm("users.manage"), s.handleResetPassword)
	}
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
