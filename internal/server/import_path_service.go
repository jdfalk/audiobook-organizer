// file: internal/server/import_path_service.go
// version: 1.0.0
// guid: d4e5f6g7-h8i9-j0k1-l2m3-n4o5p6q7r8s9

package server

import (
	"fmt"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type ImportPathService struct {
	db database.Store
}

func NewImportPathService(db database.Store) *ImportPathService {
	return &ImportPathService{db: db}
}

// ValidatePath validates that an import path is not empty
func (ips *ImportPathService) ValidatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("import path cannot be empty")
	}
	return nil
}

// CreateImportPath creates a new import path in the database
func (ips *ImportPathService) CreateImportPath(path, name string) (*database.ImportPath, error) {
	if err := ips.ValidatePath(path); err != nil {
		return nil, err
	}

	return ips.db.CreateImportPath(path, name)
}

// UpdateImportPathEnabled updates the enabled status of an import path
func (ips *ImportPathService) UpdateImportPathEnabled(id int, enabled bool) error {
	path, err := ips.db.GetImportPathByID(id)
	if err != nil || path == nil {
		return fmt.Errorf("import path not found")
	}

	path.Enabled = enabled
	return ips.db.UpdateImportPath(id, path)
}

// GetImportPath retrieves an import path by ID
func (ips *ImportPathService) GetImportPath(id int) (*database.ImportPath, error) {
	path, err := ips.db.GetImportPathByID(id)
	if err != nil || path == nil {
		return nil, fmt.Errorf("import path not found")
	}
	return path, nil
}
