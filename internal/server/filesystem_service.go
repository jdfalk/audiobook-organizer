// file: internal/server/filesystem_service.go
// version: 1.1.0
// guid: b8c9d0e1-f2a3-4b5c-6d7e-8f9a0b1c2d3e

package server

import (
	"fmt"
	"os"
	"path/filepath"
)

type FilesystemService struct{}

func NewFilesystemService() *FilesystemService {
	return &FilesystemService{}
}

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"mod_time,omitempty"`
	Excluded bool   `json:"excluded"`
}

type BrowseResult struct {
	Path     string                 `json:"path"`
	Items    []FileInfo             `json:"items"`
	Count    int                    `json:"count"`
	DiskInfo map[string]any `json:"disk_info"`
}

func (fs *FilesystemService) BrowseDirectory(path string) (*BrowseResult, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	items := []FileInfo{}
	for _, entry := range entries {
		fullPath := filepath.Join(absPath, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		excluded := false
		if entry.IsDir() {
			jabExcludePath := filepath.Join(fullPath, ".jabexclude")
			if _, err := os.Stat(jabExcludePath); err == nil {
				excluded = true
			}
		}

		item := FileInfo{
			Name:     entry.Name(),
			Path:     fullPath,
			IsDir:    entry.IsDir(),
			Excluded: excluded,
		}

		if !entry.IsDir() {
			item.Size = info.Size()
			item.ModTime = info.ModTime().Unix()
		}

		items = append(items, item)
	}

	diskInfo := map[string]any{}
	if stat, err := os.Stat(absPath); err == nil {
		diskInfo = map[string]any{
			"exists":   true,
			"readable": stat.Mode().Perm()&0400 != 0,
			"writable": stat.Mode().Perm()&0200 != 0,
		}
	}

	return &BrowseResult{
		Path:     absPath,
		Items:    items,
		Count:    len(items),
		DiskInfo: diskInfo,
	}, nil
}

func (fs *FilesystemService) CreateExclusion(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path not found or inaccessible: %w", err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("path must be a directory")
	}

	excludeFile := filepath.Join(absPath, ".jabexclude")
	return os.WriteFile(excludeFile, []byte(""), 0644)
}

func (fs *FilesystemService) RemoveExclusion(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	excludeFile := filepath.Join(absPath, ".jabexclude")
	if err := os.Remove(excludeFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("exclusion not found")
		}
		return fmt.Errorf("failed to remove exclusion: %w", err)
	}
	return nil
}
