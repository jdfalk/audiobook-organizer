// file: internal/server/config_update_service.go
// version: 3.0.0
// guid: f6g7h8i9-j0k1-l2m3-n4o5-p6q7r8s9t0u1

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ConfigUpdateService handles applying and persisting config changes.
type ConfigUpdateService struct {
	db database.Store
}

// NewConfigUpdateService creates a new ConfigUpdateService.
func NewConfigUpdateService(db database.Store) *ConfigUpdateService {
	return &ConfigUpdateService{db: db}
}

// ValidateUpdate checks that the payload is non-empty.
func (cus *ConfigUpdateService) ValidateUpdate(payload map[string]any) error {
	if len(payload) == 0 {
		return fmt.Errorf("no configuration updates provided")
	}
	return nil
}

// MaskSecrets returns a copy of cfg with all secret fields masked for API responses.
func (cus *ConfigUpdateService) MaskSecrets(cfg config.Config) config.Config {
	masked := cfg
	if masked.OpenAIAPIKey != "" {
		masked.OpenAIAPIKey = database.MaskSecret(masked.OpenAIAPIKey)
	}
	if masked.GoogleBooksAPIKey != "" {
		masked.GoogleBooksAPIKey = database.MaskSecret(masked.GoogleBooksAPIKey)
	}
	if masked.HardcoverAPIToken != "" {
		masked.HardcoverAPIToken = database.MaskSecret(masked.HardcoverAPIToken)
	}
	if masked.BasicAuthPassword != "" {
		masked.BasicAuthPassword = database.MaskSecret(masked.BasicAuthPassword)
	}
	return masked
}

// secretFieldKeys are extracted and applied explicitly, then removed before the
// JSON round-trip so they are never stored in plaintext in the config blob.
var secretFieldKeys = []string{
	"openai_api_key",
	"google_books_api_key",
	"hardcover_api_token",
	"basic_auth_password",
}

// immutableFieldKeys cannot be changed at runtime and are rejected if present.
var immutableFieldKeys = []string{"database_type", "enable_sqlite"}

// UpdateConfig applies a config update payload to AppConfig and persists it.
//
// Architecture: non-secret fields are applied via JSON round-trip onto AppConfig.
// json.Unmarshal only overwrites keys present in the JSON, so absent keys leave
// AppConfig unchanged. This means any new field added to config.Config is
// automatically handled here with no registration required.
func (cus *ConfigUpdateService) UpdateConfig(payload map[string]any) (int, map[string]any) {
	if cus.db == nil {
		return http.StatusInternalServerError, map[string]any{"error": "database not initialized"}
	}
	if payload == nil {
		return http.StatusBadRequest, map[string]any{"error": "configuration payload is required"}
	}

	// Reject immutable fields
	for _, field := range immutableFieldKeys {
		if _, ok := payload[field]; ok {
			return http.StatusBadRequest, map[string]any{"error": field + " cannot be changed at runtime"}
		}
	}

	// Apply secrets explicitly — they need masking/debug logging and must not
	// flow through the JSON round-trip to avoid plaintext exposure.
	if val, ok := payloadString(payload, "openai_api_key"); ok {
		log.Printf("[DEBUG] UpdateConfig: updating OpenAI API key (len=%d)", len(val))
		config.AppConfig.OpenAIAPIKey = val
	}
	if val, ok := payloadString(payload, "google_books_api_key"); ok {
		config.AppConfig.GoogleBooksAPIKey = val
	}
	if val, ok := payloadString(payload, "hardcover_api_token"); ok {
		config.AppConfig.HardcoverAPIToken = val
	}
	if val, ok := payloadString(payload, "basic_auth_password"); ok {
		config.AppConfig.BasicAuthPassword = val
	}

	// Build filtered payload without secrets (already applied above)
	filtered := make(map[string]any, len(payload))
	for k, v := range payload {
		filtered[k] = v
	}
	for _, k := range secretFieldKeys {
		delete(filtered, k)
	}

	// Apply all remaining fields via JSON round-trip.
	// Any field in config.Config with a matching json tag is set automatically.
	payloadJSON, err := json.Marshal(filtered)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"error": "failed to encode payload: " + err.Error()}
	}
	if err := json.Unmarshal(payloadJSON, &config.AppConfig); err != nil {
		return http.StatusBadRequest, map[string]any{"error": "failed to apply config: " + err.Error()}
	}

	// Post-process: trim root_dir whitespace, derive setup_complete
	config.AppConfig.RootDir = strings.TrimSpace(config.AppConfig.RootDir)
	config.AppConfig.SetupComplete = config.AppConfig.RootDir != ""

	if err := config.SaveConfigToDatabase(cus.db); err != nil {
		log.Printf("ERROR: failed to persist config: %v", err)
		return http.StatusInternalServerError, map[string]any{
			"error":   "failed to save configuration",
			"details": err.Error(),
		}
	}

	log.Printf("Configuration saved successfully")

	return http.StatusOK, map[string]any{
		"message": "configuration updated and saved to database",
		"config":  cus.MaskSecrets(config.AppConfig),
	}
}

// payloadString extracts a string value from the payload if present and non-empty.
func payloadString(payload map[string]any, key string) (string, bool) {
	v, ok := payload[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// ApplyUpdates applies config updates and persists them.
// Deprecated: prefer UpdateConfig directly.
func (cus *ConfigUpdateService) ApplyUpdates(payload map[string]any) error {
	status, resp := cus.UpdateConfig(payload)
	if status >= 400 {
		if errMsg, ok := resp["error"].(string); ok {
			return fmt.Errorf("%s", errMsg)
		}
		return fmt.Errorf("config update failed with status %d", status)
	}
	return nil
}
