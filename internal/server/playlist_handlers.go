// file: internal/server/playlist_handlers.go
// version: 2.0.1
// guid: 7a3d5f2e-8c4b-4a70-b8c5-3d7e0f1b9a79
//
// HTTP endpoints for user-created playlists (spec 3.4 task 3).
// Supports:
//   - Static playlists: user-curated ordered book lists
//   - Smart playlists: DSL queries evaluated on demand via
//     EvaluateSmartPlaylist (delegates to Bleve + per-user filter)
//
// Create/update/delete are gated on `playlists.create` once 3.7
// permission wiring ships. GET paths require `library.view`.
// The `callingUserID` shim from reading_handlers.go supplies the
// user context.

package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/search"
)

// playlistCreateReq is the payload for POST /api/v1/playlists.
type playlistCreateReq struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type" binding:"required"` // static|smart
	BookIDs     []string `json:"book_ids,omitempty"`
	Query       string   `json:"query,omitempty"`
	SortJSON    string   `json:"sort_json,omitempty"`
	Limit       int      `json:"limit,omitempty"`
}

// playlistUpdateReq mirrors playlistCreateReq but all fields are
// optional — only set ones are applied.
type playlistUpdateReq struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	BookIDs     *[]string `json:"book_ids,omitempty"`
	Query       *string   `json:"query,omitempty"`
	SortJSON    *string   `json:"sort_json,omitempty"`
	Limit       *int      `json:"limit,omitempty"`
}

type playlistBooksAddReq struct {
	BookIDs []string `json:"book_ids" binding:"required"`
}

type playlistReorderReq struct {
	BookIDs []string `json:"book_ids" binding:"required"`
}

// handleCreatePlaylist — POST /api/v1/playlists
func (s *Server) handleCreatePlaylist(c *gin.Context) {
	var req playlistCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithBadRequest(c, err.Error())
		return
	}
	if err := validatePlaylistCreate(&req); err != nil {
		RespondWithBadRequest(c, err.Error())
		return
	}

	pl := &database.UserPlaylist{
		Name:            strings.TrimSpace(req.Name),
		Description:     req.Description,
		Type:            req.Type,
		BookIDs:         req.BookIDs,
		Query:           req.Query,
		SortJSON:        req.SortJSON,
		Limit:           req.Limit,
		CreatedByUserID: callingUserID(c),
		Dirty:           true, // new playlists need iTunes sync
	}
	created, err := s.Store().CreateUserPlaylist(pl)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
			RespondWithConflict(c, err.Error())
			return
		}
		internalError(c, "failed to create playlist", err)
		return
	}
	RespondWithCreated(c, created)
}

// handleListPlaylists — GET /api/v1/playlists?type=static|smart&limit=N&offset=M
func (s *Server) handleListPlaylists(c *gin.Context) {
	plType := c.Query("type")
	if plType != "" &&
		plType != database.UserPlaylistTypeStatic &&
		plType != database.UserPlaylistTypeSmart {
		RespondWithBadRequest(c, "type must be static, smart, or empty")
		return
	}
	limit, offset := paginationFromQuery(c)
	lists, total, err := s.Store().ListUserPlaylists(plType, limit, offset)
	if err != nil {
		internalError(c, "failed to list playlists", err)
		return
	}
	RespondWithList(c, lists, total, limit, offset)
}

// handleGetPlaylist — GET /api/v1/playlists/:id
// For static: returns playlist + the stored BookIDs.
// For smart: evaluates the query and returns the live book list
// alongside the playlist metadata. Caches evaluation into
// MaterializedBookIDs for the iTunes push worker.
func (s *Server) handleGetPlaylist(c *gin.Context) {
	id := c.Param("id")
	pl, err := s.Store().GetUserPlaylist(id)
	if err != nil {
		internalError(c, "failed to load playlist", err)
		return
	}
	if pl == nil {
		RespondWithNotFound(c, "playlist", id)
		return
	}

	resp := gin.H{"playlist": pl}
	switch pl.Type {
	case database.UserPlaylistTypeStatic:
		resp["book_ids"] = pl.BookIDs
	case database.UserPlaylistTypeSmart:
		bookIDs, evalErr := EvaluateSmartPlaylist(
			s.Store(), s.SearchIndex(),
			pl.Query, pl.SortJSON, pl.Limit,
			callingUserID(c),
		)
		if evalErr != nil {
			// Surface as 503 when the index is unavailable — this is
			// a transient condition during startup. Actual query
			// errors are 400 (user's smart-playlist DSL is busted).
			if evalErr == ErrSearchIndexUnavailable {
				RespondWithError(c, 503, evalErr.Error(), "SERVICE_UNAVAILABLE")
				return
			}
			RespondWithBadRequest(c, evalErr.Error())
			return
		}
		resp["book_ids"] = bookIDs
		// Cache for iTunes sync worker. Persist only if changed.
		if !stringSlicesEqual(pl.MaterializedBookIDs, bookIDs) {
			pl.MaterializedBookIDs = bookIDs
			_ = s.Store().UpdateUserPlaylist(pl)
		}
	}
	RespondWithOK(c, resp)
}

// handleUpdatePlaylist — PUT /api/v1/playlists/:id
func (s *Server) handleUpdatePlaylist(c *gin.Context) {
	id := c.Param("id")
	pl, err := s.Store().GetUserPlaylist(id)
	if err != nil {
		internalError(c, "failed to load playlist", err)
		return
	}
	if pl == nil {
		RespondWithNotFound(c, "playlist", id)
		return
	}

	var req playlistUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithBadRequest(c, err.Error())
		return
	}
	if req.Name != nil {
		pl.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		pl.Description = *req.Description
	}
	if req.BookIDs != nil {
		if pl.Type != database.UserPlaylistTypeStatic {
			RespondWithBadRequest(c, "book_ids only valid for static playlists")
			return
		}
		pl.BookIDs = *req.BookIDs
	}
	if req.Query != nil {
		if pl.Type != database.UserPlaylistTypeSmart {
			RespondWithBadRequest(c, "query only valid for smart playlists")
			return
		}
		if _, err := search.ParseQuery(*req.Query); err != nil {
			RespondWithBadRequest(c, "invalid query: "+err.Error())
			return
		}
		pl.Query = *req.Query
	}
	if req.SortJSON != nil {
		pl.SortJSON = *req.SortJSON
	}
	if req.Limit != nil {
		pl.Limit = *req.Limit
	}
	pl.Dirty = true
	if err := s.Store().UpdateUserPlaylist(pl); err != nil {
		internalError(c, "failed to update playlist", err)
		return
	}
	RespondWithOK(c, pl)
}

// handleDeletePlaylist — DELETE /api/v1/playlists/:id
func (s *Server) handleDeletePlaylist(c *gin.Context) {
	id := c.Param("id")
	if err := s.Store().DeleteUserPlaylist(id); err != nil {
		internalError(c, "failed to delete playlist", err)
		return
	}
	RespondWithOK(c, gin.H{"deleted": id})
}

// handleAddBooksToPlaylist — POST /api/v1/playlists/:id/books
// Appends book IDs to a static playlist, de-duplicating against
// existing entries. No-op on smart playlists.
func (s *Server) handleAddBooksToPlaylist(c *gin.Context) {
	id := c.Param("id")
	var req playlistBooksAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithBadRequest(c, err.Error())
		return
	}
	pl, err := s.Store().GetUserPlaylist(id)
	if err != nil {
		internalError(c, "failed to load playlist", err)
		return
	}
	if pl == nil {
		RespondWithNotFound(c, "playlist", id)
		return
	}
	if pl.Type != database.UserPlaylistTypeStatic {
		RespondWithBadRequest(c, "cannot add books to smart playlist")
		return
	}
	existing := make(map[string]bool, len(pl.BookIDs))
	for _, bid := range pl.BookIDs {
		existing[bid] = true
	}
	for _, bid := range req.BookIDs {
		if bid == "" || existing[bid] {
			continue
		}
		pl.BookIDs = append(pl.BookIDs, bid)
		existing[bid] = true
	}
	pl.Dirty = true
	if err := s.Store().UpdateUserPlaylist(pl); err != nil {
		internalError(c, "failed to add books", err)
		return
	}
	RespondWithOK(c, pl)
}

// handleRemoveBookFromPlaylist — DELETE /api/v1/playlists/:id/books/:bookID
func (s *Server) handleRemoveBookFromPlaylist(c *gin.Context) {
	id := c.Param("id")
	bookID := c.Param("bookID")
	pl, err := s.Store().GetUserPlaylist(id)
	if err != nil {
		internalError(c, "failed to load playlist", err)
		return
	}
	if pl == nil {
		RespondWithNotFound(c, "playlist", id)
		return
	}
	if pl.Type != database.UserPlaylistTypeStatic {
		RespondWithBadRequest(c, "cannot remove books from smart playlist")
		return
	}
	filtered := pl.BookIDs[:0]
	for _, b := range pl.BookIDs {
		if b != bookID {
			filtered = append(filtered, b)
		}
	}
	pl.BookIDs = filtered
	pl.Dirty = true
	if err := s.Store().UpdateUserPlaylist(pl); err != nil {
		internalError(c, "failed to remove book", err)
		return
	}
	RespondWithOK(c, pl)
}

// handleReorderPlaylist — POST /api/v1/playlists/:id/reorder
// Replaces book order. Rejects if the payload changes the set of
// books (use add/remove endpoints for that).
func (s *Server) handleReorderPlaylist(c *gin.Context) {
	id := c.Param("id")
	var req playlistReorderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithBadRequest(c, err.Error())
		return
	}
	pl, err := s.Store().GetUserPlaylist(id)
	if err != nil {
		internalError(c, "failed to load playlist", err)
		return
	}
	if pl == nil {
		RespondWithNotFound(c, "playlist", id)
		return
	}
	if pl.Type != database.UserPlaylistTypeStatic {
		RespondWithBadRequest(c, "cannot reorder smart playlist")
		return
	}
	if !sameBookSet(pl.BookIDs, req.BookIDs) {
		RespondWithBadRequest(c, "reorder must keep the same book set")
		return
	}
	pl.BookIDs = req.BookIDs
	pl.Dirty = true
	if err := s.Store().UpdateUserPlaylist(pl); err != nil {
		internalError(c, "failed to reorder", err)
		return
	}
	RespondWithOK(c, pl)
}

// handleMaterializePlaylist — POST /api/v1/playlists/:id/materialize
// Evaluates a smart playlist and creates a new static playlist
// from the snapshot. The source smart playlist is left unchanged.
func (s *Server) handleMaterializePlaylist(c *gin.Context) {
	id := c.Param("id")
	src, err := s.Store().GetUserPlaylist(id)
	if err != nil {
		internalError(c, "failed to load playlist", err)
		return
	}
	if src == nil {
		RespondWithNotFound(c, "playlist", id)
		return
	}
	if src.Type != database.UserPlaylistTypeSmart {
		RespondWithBadRequest(c, "only smart playlists can be materialized")
		return
	}
	bookIDs, evalErr := EvaluateSmartPlaylist(
		s.Store(), s.SearchIndex(),
		src.Query, src.SortJSON, src.Limit,
		callingUserID(c),
	)
	if evalErr != nil {
		if evalErr == ErrSearchIndexUnavailable {
			RespondWithError(c, 503, evalErr.Error(), "SERVICE_UNAVAILABLE")
			return
		}
		RespondWithBadRequest(c, evalErr.Error())
		return
	}

	snapshot := &database.UserPlaylist{
		Name:            fmt.Sprintf("%s (snapshot %s)", src.Name, time.Now().Format("2006-01-02")),
		Description:     fmt.Sprintf("Materialized from smart playlist %q at %s", src.Name, time.Now().Format(time.RFC3339)),
		Type:            database.UserPlaylistTypeStatic,
		BookIDs:         bookIDs,
		CreatedByUserID: callingUserID(c),
		Dirty:           true,
	}
	created, err := s.Store().CreateUserPlaylist(snapshot)
	if err != nil {
		// Name collision is the common case — retry with a counter.
		for i := 2; i < 10 && err != nil; i++ {
			snapshot.Name = fmt.Sprintf("%s (snapshot %s #%d)", src.Name, time.Now().Format("2006-01-02"), i)
			created, err = s.Store().CreateUserPlaylist(snapshot)
		}
		if err != nil {
			internalError(c, "failed to materialize", err)
			return
		}
	}
	RespondWithCreated(c, created)
}

// validatePlaylistCreate checks required fields and type-specific
// shape of a playlistCreateReq.
func validatePlaylistCreate(req *playlistCreateReq) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch req.Type {
	case database.UserPlaylistTypeStatic:
		if req.Query != "" {
			return fmt.Errorf("static playlist must not have a query")
		}
	case database.UserPlaylistTypeSmart:
		if len(req.BookIDs) > 0 {
			return fmt.Errorf("smart playlist must not have explicit book_ids")
		}
		if strings.TrimSpace(req.Query) == "" {
			return fmt.Errorf("smart playlist requires a query")
		}
		if _, err := search.ParseQuery(req.Query); err != nil {
			return fmt.Errorf("invalid query: %w", err)
		}
	default:
		return fmt.Errorf("type must be static or smart")
	}
	return nil
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sameBookSet reports whether a and b contain the same elements,
// ignoring order.
func sameBookSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}

// registerPlaylistRoutes wires playlist endpoints onto the protected
// router group.
func (s *Server) registerPlaylistRoutes(protected *gin.RouterGroup) {
	protected.GET("/playlists", s.handleListPlaylists)
	protected.POST("/playlists", s.handleCreatePlaylist)
	protected.GET("/playlists/:id", s.handleGetPlaylist)
	protected.PUT("/playlists/:id", s.handleUpdatePlaylist)
	protected.DELETE("/playlists/:id", s.handleDeletePlaylist)
	protected.POST("/playlists/:id/books", s.handleAddBooksToPlaylist)
	protected.DELETE("/playlists/:id/books/:bookID", s.handleRemoveBookFromPlaylist)
	protected.POST("/playlists/:id/reorder", s.handleReorderPlaylist)
	protected.POST("/playlists/:id/materialize", s.handleMaterializePlaylist)
}
