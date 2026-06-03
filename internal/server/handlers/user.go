// file: internal/server/handlers/user.go
// version: 1.1.0
// guid: b2c3d4e5-f6a7-8901-bcde-ef0123456789
// last-edited: 2026-06-02

// Package handlers contains extracted HTTP handler types for the audiobook
// organizer server. UserHandler covers user management and invite endpoints.

package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"golang.org/x/crypto/bcrypt"
)

// UserStore is the narrow persistence interface required by UserHandler.
type UserStore interface {
	ListUsers() ([]database.User, error)
	CreateInvite(invite *database.Invite) (*database.Invite, error)
	ListActiveInvites() ([]database.Invite, error)
	DeleteInvite(token string) error
	ConsumeInvite(token, algo, hash string) (*database.User, error)
	CreateSession(userID, ip, ua string, ttl time.Duration) (*database.Session, error)
	GetUserByID(id string) (*database.User, error)
	UpdateUser(user *database.User) error
}

// UserHandler handles user-management and invite HTTP endpoints.
type UserHandler struct {
	store UserStore
}

// NewUserHandler constructs a UserHandler backed by the given UserStore.
func NewUserHandler(store UserStore) *UserHandler {
	return &UserHandler{store: store}
}

// generateToken produces a cryptographically random 32-byte hex string.
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ListUsers handles GET /api/v1/users — returns all users (admin only).
func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.store.ListUsers()
	if err != nil {
		httputil.InternalError(c, "failed to list users", err)
		return
	}
	safe := make([]gin.H, 0, len(users))
	for _, u := range users {
		safe = append(safe, gin.H{
			"id": u.ID, "username": u.Username, "email": u.Email,
			"roles": u.Roles, "status": u.Status,
			"created_at": u.CreatedAt, "updated_at": u.UpdatedAt,
		})
	}
	httputil.RespondWithOK(c, gin.H{"users": safe, "count": len(safe)})
}

// CreateInvite handles POST /api/v1/users/invite — generates an invite token.
func (h *UserHandler) CreateInvite(c *gin.Context) {
	var req struct {
		Username  string `json:"username" binding:"required"`
		RoleID    string `json:"role_id" binding:"required"`
		ExpiresIn int    `json:"expires_in"` // hours, default 72
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
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
	created, err := h.store.CreateInvite(invite)
	if err != nil {
		httputil.InternalError(c, "failed to create invite", err)
		return
	}
	httputil.RespondWithCreated(c, gin.H{"invite": created, "token": token})
}

// ListInvites handles GET /api/v1/users/invites — returns active invites.
func (h *UserHandler) ListInvites(c *gin.Context) {
	invites, err := h.store.ListActiveInvites()
	if err != nil {
		httputil.InternalError(c, "failed to list invites", err)
		return
	}
	if invites == nil {
		invites = []database.Invite{}
	}
	httputil.RespondWithOK(c, gin.H{"invites": invites, "count": len(invites)})
}

// DeleteInvite handles DELETE /api/v1/users/invites/:token — revokes an invite.
func (h *UserHandler) DeleteInvite(c *gin.Context) {
	token := c.Param("token")
	if err := h.store.DeleteInvite(token); err != nil {
		httputil.InternalError(c, "failed to delete invite", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"deleted": token})
}

// AcceptInvite handles POST /api/v1/auth/accept-invite — consumes an invite and creates a user session.
func (h *UserHandler) AcceptInvite(c *gin.Context) {
	var req struct {
		Token    string `json:"token" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if len(req.Password) < 8 {
		httputil.RespondWithBadRequest(c, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httputil.InternalError(c, "hash password", err)
		return
	}

	user, err := h.store.ConsumeInvite(req.Token, "bcrypt", string(hash))
	if err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	sess, err := h.store.CreateSession(user.ID, c.ClientIP(), c.Request.UserAgent(), 30*24*time.Hour)
	if err != nil {
		httputil.InternalError(c, "create session", err)
		return
	}

	// SetSessionCookie is exported from handlers/auth.go; user.go is in the
	// same package so we can call it directly.
	SetSessionCookie(c, sess.ID, sess.ExpiresAt)
	// Session token is delivered via the HttpOnly cookie only — never in the
	// JSON body (would expose the bearer token to page JS, defeating HttpOnly).
	httputil.RespondWithCreated(c, gin.H{"user": user, "expires_at": sess.ExpiresAt})
}

// DeactivateUser handles POST /api/v1/users/:id/deactivate — soft-deactivates a user.
func (h *UserHandler) DeactivateUser(c *gin.Context) {
	id := c.Param("id")
	user, err := h.store.GetUserByID(id)
	if err != nil {
		httputil.InternalError(c, "get user", err)
		return
	}
	if user == nil {
		httputil.RespondWithNotFound(c, "user", id)
		return
	}

	user.Status = "locked"
	if err := h.store.UpdateUser(user); err != nil {
		httputil.InternalError(c, "deactivate user", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"user": user})
}

// ReactivateUser handles POST /api/v1/users/:id/reactivate — reactivates a locked user.
func (h *UserHandler) ReactivateUser(c *gin.Context) {
	id := c.Param("id")
	user, err := h.store.GetUserByID(id)
	if err != nil {
		httputil.InternalError(c, "get user", err)
		return
	}
	if user == nil {
		httputil.RespondWithNotFound(c, "user", id)
		return
	}

	user.Status = "active"
	if err := h.store.UpdateUser(user); err != nil {
		httputil.InternalError(c, "reactivate user", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"user": user})
}

// ResetPassword handles POST /api/v1/users/:id/reset-password — generates a
// reset invite so the user can set a new password.
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id := c.Param("id")
	user, err := h.store.GetUserByID(id)
	if err != nil || user == nil {
		httputil.RespondWithNotFound(c, "user", id)
		return
	}

	token := generateToken()
	invite := &database.Invite{
		Token:     token,
		Username:  user.Username,
		RoleID:    "",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if _, err := h.store.CreateInvite(invite); err != nil {
		httputil.InternalError(c, "create reset invite", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"token": token, "expires_at": invite.ExpiresAt})
}
