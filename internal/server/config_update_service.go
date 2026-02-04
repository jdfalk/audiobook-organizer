// file: internal/server/config_update_service.go
// version: 1.0.0
// guid: f6g7h8i9-j0k1-l2m3-n4o5-p6q7r8s9t0u1

package server

import (
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

type ConfigUpdateService struct{}

func NewConfigUpdateService() *ConfigUpdateService {
	return &ConfigUpdateService{}
}

// ValidateUpdate checks if the update payload has required fields
func (cus *ConfigUpdateService) ValidateUpdate(payload map[string]any) error {
	if len(payload) == 0 {
		return fmt.Errorf("no configuration updates provided")
	}
	return nil
}

// ExtractStringField extracts a string value from payload
func (cus *ConfigUpdateService) ExtractStringField(payload map[string]any, key string) (string, bool) {
	val, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// ExtractBoolField extracts a bool value from payload
func (cus *ConfigUpdateService) ExtractBoolField(payload map[string]any, key string) (bool, bool) {
	val, ok := payload[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// ExtractIntField extracts an int value from payload (handling JSON float64)
func (cus *ConfigUpdateService) ExtractIntField(payload map[string]any, key string) (int, bool) {
	val, ok := payload[key]
	if !ok {
		return 0, false
	}
	f, ok := val.(float64)
	return int(f), ok
}

// ApplyUpdates applies all updates from payload to AppConfig
func (cus *ConfigUpdateService) ApplyUpdates(payload map[string]any) {
	if rootDir, ok := cus.ExtractStringField(payload, "root_dir"); ok {
		config.AppConfig.RootDir = rootDir
	}

	if autoOrganize, ok := cus.ExtractBoolField(payload, "auto_organize"); ok {
		config.AppConfig.AutoOrganize = autoOrganize
	}

	if concurrentScans, ok := cus.ExtractIntField(payload, "concurrent_scans"); ok {
		config.AppConfig.ConcurrentScans = concurrentScans
	}

	if excludePatterns, ok := payload["exclude_patterns"].([]any); ok {
		patterns := make([]string, len(excludePatterns))
		for i, p := range excludePatterns {
			if s, ok := p.(string); ok {
				patterns[i] = s
			}
		}
		config.AppConfig.ExcludePatterns = patterns
	}

	if supportedExtensions, ok := payload["supported_extensions"].([]any); ok {
		extensions := make([]string, len(supportedExtensions))
		for i, e := range supportedExtensions {
			if s, ok := e.(string); ok {
				extensions[i] = s
			}
		}
		config.AppConfig.SupportedExtensions = extensions
	}
}

// MaskSecrets removes sensitive fields from config for response
func (cus *ConfigUpdateService) MaskSecrets(cfg *config.Config) map[string]any {
	result := map[string]any{
		"root_dir":              cfg.RootDir,
		"auto_organize":         cfg.AutoOrganize,
		"concurrent_scans":      cfg.ConcurrentScans,
		"exclude_patterns":      cfg.ExcludePatterns,
		"supported_extensions":  cfg.SupportedExtensions,
	}
	return result
}
