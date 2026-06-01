// file: internal/server/handlers/playlists.go
// version: 1.0.0
// guid: a7b8c9d0-e1f2-3456-abcd-456789012345
// last-edited: 2026-06-01

package handlers

// PlaylistCreateReq is the payload for POST /api/v1/playlists.
type PlaylistCreateReq struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type" binding:"required"` // static|smart
	BookIDs     []string `json:"book_ids,omitempty"`
	Query       string   `json:"query,omitempty"`
	SortJSON    string   `json:"sort_json,omitempty"`
	Limit       int      `json:"limit,omitempty"`
}

// PlaylistUpdateReq mirrors PlaylistCreateReq but all fields are
// optional — only set ones are applied.
type PlaylistUpdateReq struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	BookIDs     *[]string `json:"book_ids,omitempty"`
	Query       *string   `json:"query,omitempty"`
	SortJSON    *string   `json:"sort_json,omitempty"`
	Limit       *int      `json:"limit,omitempty"`
}

// PlaylistBooksAddReq is the payload for POST /api/v1/playlists/:id/books.
type PlaylistBooksAddReq struct {
	BookIDs []string `json:"book_ids" binding:"required"`
}

// PlaylistReorderReq is the payload for PUT /api/v1/playlists/:id/books/order.
type PlaylistReorderReq struct {
	BookIDs []string `json:"book_ids" binding:"required"`
}
