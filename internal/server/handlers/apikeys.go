// file: internal/server/handlers/apikeys.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f01234567890
// last-edited: 2026-06-01

package handlers

import "time"

// CreateAPIKeyRequest is the JSON body for POST /api/v1/api-keys.
type CreateAPIKeyRequest struct {
	Name          string   `json:"name" binding:"required"`
	Description   string   `json:"description"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expires_in_days"`
	UserID        string   `json:"user_id"`
}

// CreateAPIKeyResponse is returned after successfully creating an API key.
type CreateAPIKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Token     string     `json:"token"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// APIKeyResponse is the JSON shape for a single API key (list and detail endpoints).
type APIKeyResponse struct {
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
