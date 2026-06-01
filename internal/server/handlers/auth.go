// file: internal/server/handlers/auth.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-012345678901
// last-edited: 2026-06-01

package handlers

import "time"

// AuthUserResponse is the JSON shape returned after login and token refresh.
type AuthUserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Roles     []string  `json:"roles"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
