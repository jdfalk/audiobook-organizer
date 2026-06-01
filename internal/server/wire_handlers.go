// file: internal/server/wire_handlers.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-3456-7890-abcdef012345
// last-edited: 2026-06-01

package server

import (
	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/server/handlers"
)

// wireHandlers instantiates Phase-1-migrated handler structs and registers
// their routes. Called from Start() in place of the inline auth/apikey blocks.
// Routes still using s.* methods are migrated in subsequent phases.
func (s *Server) wireHandlers(api *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	authH := handlers.NewAuthHandler(s.Store(), config.AppConfig.EnableAuth)
	apiKeyH := handlers.NewAPIKeyHandler(s.Store())

	authGroup := api.Group("/auth")
	{
		authGroup.GET("/status", authH.GetStatus)
		authGroup.POST("/setup", authH.SetupInitialAdmin)
		authGroup.POST("/login", authH.Login)
		authGroup.POST("/accept-invite", s.handleAcceptInvite)
		authGroup.POST("/bootstrap", s.handleBootstrap)
	}

	authProtected := authGroup.Group("")
	authProtected.Use(authMiddleware)
	{
		authProtected.GET("/me", authH.Me)
		authProtected.PATCH("/me", authH.UpdateMe)
		authProtected.POST("/logout", authH.Logout)
		authProtected.GET("/sessions", authH.ListMySessions)
		authProtected.DELETE("/sessions/:id", authH.RevokeMySession)
		authProtected.PUT("/me/password", authH.ChangePassword)
		authProtected.POST("/temp-tokens", s.perm(permTempLoginMint()), s.createTempLoginToken)

		authProtected.POST("/api-keys", apiKeyH.Create)
		authProtected.GET("/api-keys", apiKeyH.List)
		authProtected.GET("/api-keys/:id", apiKeyH.Get)
		authProtected.PATCH("/api-keys/:id", apiKeyH.UpdateStatus)
		authProtected.DELETE("/api-keys/:id", apiKeyH.Revoke)
	}
}
