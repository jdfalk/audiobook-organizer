// file: internal/server/apikey_handlers.go
// version: 2.1.0
// last-edited: 2026-05-01
// guid: a1b2c3d4-e5f6-7890-abcd-ef0123456789

package server

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	servermiddleware "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
)

type createAPIKeyRequest struct {
	Name          string   `json:"name" binding:"required"`
	Description   string   `json:"description"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expires_in_days"`
	UserID        string   `json:"user_id"`
}

type createAPIKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Token     string     `json:"token"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type apiKeyResponse struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	Scopes           []string   `json:"scopes"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP       string     `json:"last_used_ip,omitempty"`
	UseCount         int64      `json:"use_count"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	DeactivatedAt    *time.Time `json:"deactivated_at,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	Identifier       string     `json:"identifier"`
	DaysSinceLastUse *int       `json:"days_since_last_use"`
	NeverUsed        bool       `json:"never_used"`
	Username         string     `json:"username,omitempty"`
}

func buildAPIKeyResponse(key database.APIKey, username string) apiKeyResponse {
	var daysSince *int
	neverUsed := key.LastUsedAt == nil
	if key.LastUsedAt != nil {
		d := int(time.Since(*key.LastUsedAt).Hours() / 24)
		daysSince = &d
	}
	ident := ""
	if len(key.TokenHash) >= 8 {
		ident = "abk_" + key.TokenHash[:8]
	}
	return apiKeyResponse{
		ID:               key.ID,
		UserID:           key.UserID,
		Name:             key.Name,
		Description:      key.Description,
		Scopes:           key.Scopes,
		Status:           key.Status,
		CreatedAt:        key.CreatedAt,
		LastUsedAt:       key.LastUsedAt,
		LastUsedIP:       key.LastUsedIP,
		UseCount:         key.UseCount,
		ExpiresAt:        key.ExpiresAt,
		DeactivatedAt:    key.DeactivatedAt,
		RevokedAt:        key.RevokedAt,
		Identifier:       ident,
		DaysSinceLastUse: daysSince,
		NeverUsed:        neverUsed,
		Username:         username,
	}
}

// isAdminUser reports whether user has the users.manage permission (or admin role).
func isAdminUser(c *gin.Context) bool {
	return auth.Can(c.Request.Context(), auth.PermUsersManage)
}

// POST /api/v1/auth/api-keys
func (s *Server) createAPIKey(c *gin.Context) {
	caller, ok := servermiddleware.CurrentUser(c)
	if !ok || caller == nil {
		httputil.RespondWithUnauthorized(c, "authentication required")
		return
	}

	var req createAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	targetUserID := caller.ID
	if req.UserID != "" && req.UserID != caller.ID {
		if !isAdminUser(c) {
			httputil.RespondWithForbidden(c, "only admins can create keys for other users")
			return
		}
		targetUserID = req.UserID
	}

	// Validate scopes: each scope must be a known permission AND the caller
	// must hold that permission (keys can only narrow, never expand).
	callerPermSet := auth.PermissionsFromContext(c.Request.Context())

	for _, scope := range req.Scopes {
		if !auth.IsKnown(scope) {
			httputil.RespondWithBadRequest(c, "unknown scope: "+scope)
			return
		}
		if callerPermSet != nil {
			if _, has := callerPermSet[auth.Permission(scope)]; !has {
				httputil.RespondWithForbidden(c, "cannot grant scope you don't have: "+scope)
				return
			}
		}
	}

	rawToken, hash, err := database.GenerateAPIKeyToken()
	if err != nil {
		httputil.InternalError(c, "failed to generate token", err)
		return
	}

	key := &database.APIKey{
		UserID:      targetUserID,
		Name:        req.Name,
		Description: req.Description,
		TokenHash:   hash,
		Scopes:      req.Scopes,
		Status:      "active",
	}
	if len(key.Scopes) == 0 {
		key.Scopes = []string{}
	}

	if req.ExpiresInDays > 0 {
		exp := time.Now().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
		key.ExpiresAt = &exp
	}

	created, err := s.Store().CreateAPIKey(key)
	if err != nil {
		httputil.InternalError(c, "failed to create api key", err)
		return
	}

	log.Printf("[APIKEY] created: id=%s user=%s name=%s scopes=%v expires=%v",
		created.ID, targetUserID, created.Name, created.Scopes, created.ExpiresAt)

	resp := createAPIKeyResponse{
		ID:        created.ID,
		Name:      created.Name,
		Token:     rawToken,
		Scopes:    created.Scopes,
		ExpiresAt: created.ExpiresAt,
		CreatedAt: created.CreatedAt,
	}
	httputil.RespondWithCreated(c, resp)
}

// GET /api/v1/auth/api-keys
func (s *Server) listAPIKeys(c *gin.Context) {
	caller, ok := servermiddleware.CurrentUser(c)
	if !ok || caller == nil {
		httputil.RespondWithUnauthorized(c, "authentication required")
		return
	}

	showAll := c.Query("all") == "true" && isAdminUser(c)

	var keys []database.APIKey
	var err error
	if showAll {
		keys, err = s.Store().ListAllAPIKeys()
	} else {
		keys, err = s.Store().ListAPIKeysForUser(caller.ID)
	}
	if err != nil {
		httputil.InternalError(c, "failed to list api keys", err)
		return
	}
	if keys == nil {
		keys = []database.APIKey{}
	}

	// Build username map for admin view.
	userCache := map[string]string{}
	results := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		username := ""
		if showAll {
			if u, cached := userCache[k.UserID]; cached {
				username = u
			} else {
				if user, uerr := s.Store().GetUserByID(k.UserID); uerr == nil && user != nil {
					username = user.Username
				}
				userCache[k.UserID] = username
			}
		}
		results = append(results, buildAPIKeyResponse(k, username))
	}

	httputil.RespondWithOK(c, gin.H{"api_keys": results, "count": len(results)})
}

// GET /api/v1/auth/api-keys/:id
func (s *Server) getAPIKey(c *gin.Context) {
	caller, ok := servermiddleware.CurrentUser(c)
	if !ok || caller == nil {
		httputil.RespondWithUnauthorized(c, "authentication required")
		return
	}

	id := c.Param("id")
	key, err := s.Store().GetAPIKey(id)
	if err != nil {
		httputil.InternalError(c, "failed to get api key", err)
		return
	}
	if key == nil {
		httputil.RespondWithNotFound(c, "api key", id)
		return
	}
	if key.UserID != caller.ID && !isAdminUser(c) {
		httputil.RespondWithForbidden(c, "access denied")
		return
	}

	username := ""
	if isAdminUser(c) {
		if user, uerr := s.Store().GetUserByID(key.UserID); uerr == nil && user != nil {
			username = user.Username
		}
	}

	httputil.RespondWithOK(c, buildAPIKeyResponse(*key, username))
}

// PATCH /api/v1/auth/api-keys/:id
func (s *Server) updateAPIKeyStatus(c *gin.Context) {
	caller, ok := servermiddleware.CurrentUser(c)
	if !ok || caller == nil {
		httputil.RespondWithUnauthorized(c, "authentication required")
		return
	}

	id := c.Param("id")
	key, err := s.Store().GetAPIKey(id)
	if err != nil {
		httputil.InternalError(c, "failed to get api key", err)
		return
	}
	if key == nil {
		httputil.RespondWithNotFound(c, "api key", id)
		return
	}
	if key.UserID != caller.ID && !isAdminUser(c) {
		httputil.RespondWithForbidden(c, "access denied")
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if req.Status == "revoked" {
		httputil.RespondWithBadRequest(c, "use DELETE to revoke a key")
		return
	}
	if req.Status != "active" && req.Status != "inactive" {
		httputil.RespondWithBadRequest(c, "status must be 'active' or 'inactive'")
		return
	}

	if err := s.Store().SetAPIKeyStatus(id, req.Status, time.Now()); err != nil {
		httputil.InternalError(c, "failed to update api key status", err)
		return
	}

	log.Printf("[APIKEY] status change: id=%s user=%s status=%s", id, caller.ID, req.Status)

	updated, err := s.Store().GetAPIKey(id)
	if err != nil || updated == nil {
		httputil.RespondWithOK(c, gin.H{"status": req.Status})
		return
	}
	httputil.RespondWithOK(c, buildAPIKeyResponse(*updated, ""))
}

// DELETE /api/v1/auth/api-keys/:id
func (s *Server) revokeAPIKey(c *gin.Context) {
	caller, ok := servermiddleware.CurrentUser(c)
	if !ok || caller == nil {
		httputil.RespondWithUnauthorized(c, "authentication required")
		return
	}

	id := c.Param("id")
	key, err := s.Store().GetAPIKey(id)
	if err != nil {
		httputil.InternalError(c, "failed to get api key", err)
		return
	}
	if key == nil {
		httputil.RespondWithNotFound(c, "api key", id)
		return
	}
	if key.UserID != caller.ID && !isAdminUser(c) {
		httputil.RespondWithForbidden(c, "access denied")
		return
	}

	if err := s.Store().RevokeAPIKey(id); err != nil {
		httputil.InternalError(c, "failed to revoke api key", err)
		return
	}

	log.Printf("[APIKEY] revoked: id=%s user=%s name=%s", id, caller.ID, key.Name)
	httputil.RespondWithNoContent(c)
}
