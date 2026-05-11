// file: internal/covers/covers_test.go
// version: 1.0.0
// guid: e5f6a7b8-9012-cdef-0123-456789abcdef
// last-edited: 2026-05-11

package covers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsAllowedCoverSource(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		allowed bool
	}{
		{
			name:    "openlibrary https",
			url:     "https://covers.openlibrary.org/b/id/123-M.jpg",
			allowed: true,
		},
		{
			name:    "openlibrary http",
			url:     "http://covers.openlibrary.org/b/id/123-M.jpg",
			allowed: true,
		},
		{
			name:    "google books https",
			url:     "https://books.google.com/books/content?id=xyz",
			allowed: true,
		},
		{
			name:    "amazon images https",
			url:     "https://images.amazon.com/images/P/B123456789.jpg",
			allowed: true,
		},
		{
			name:    "amazon ssl images https",
			url:     "https://images-na.ssl-images-amazon.com/images/P/B123456789.jpg",
			allowed: true,
		},
		{
			name:    "unauthorized domain",
			url:     "https://evil.com/cover.jpg",
			allowed: false,
		},
		{
			name:    "empty url",
			url:     "",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAllowedCoverSource(tt.url)
			if result != tt.allowed {
				t.Errorf("IsAllowedCoverSource(%q) = %v, want %v", tt.url, result, tt.allowed)
			}
		})
	}
}

func TestGetCachePath(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		cacheDir string
		wantExt  string
	}{
		{
			name:     "jpg image",
			url:      "https://covers.openlibrary.org/b/id/123-M.jpg",
			cacheDir: "/cache",
			wantExt:  ".jpg",
		},
		{
			name:     "png image",
			url:      "https://books.google.com/books/content?id=xyz.png",
			cacheDir: "/cache",
			wantExt:  ".png",
		},
		{
			name:     "no extension defaults to jpg",
			url:      "https://example.com/cover",
			cacheDir: "/tmp",
			wantExt:  ".jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCachePath(tt.url, tt.cacheDir)
			if !strings.HasPrefix(result, tt.cacheDir) {
				t.Errorf("GetCachePath result %q does not start with cacheDir %q", result, tt.cacheDir)
			}
			if !strings.HasSuffix(result, tt.wantExt) {
				t.Errorf("GetCachePath result %q does not have extension %q", result, tt.wantExt)
			}
		})
	}
}

func TestFindCoverFile(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	coversDir := filepath.Join(tmpDir, "covers")
	coversCacheDir := filepath.Join(tmpDir, ".covers")

	if err := os.MkdirAll(coversDir, 0755); err != nil {
		t.Fatalf("failed to create covers dir: %v", err)
	}
	if err := os.MkdirAll(coversCacheDir, 0755); err != nil {
		t.Fatalf("failed to create .covers dir: %v", err)
	}

	// Create a test cover file in main covers dir
	coverPath := filepath.Join(coversDir, "book123.jpg")
	if err := os.WriteFile(coverPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test cover: %v", err)
	}

	// Create a test cover file in cache dir
	cacheCoverPath := filepath.Join(coversCacheDir, "cached.jpg")
	if err := os.WriteFile(cacheCoverPath, []byte("cached"), 0644); err != nil {
		t.Fatalf("failed to create cached cover: %v", err)
	}

	tests := []struct {
		name      string
		filename  string
		wantFound bool
		wantDir   string
	}{
		{
			name:      "find in covers directory",
			filename:  "book123.jpg",
			wantFound: true,
			wantDir:   "covers",
		},
		{
			name:      "find in .covers cache directory",
			filename:  "cached.jpg",
			wantFound: true,
			wantDir:   ".covers",
		},
		{
			name:      "not found",
			filename:  "nonexistent.jpg",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FindCoverFile(tt.filename, tmpDir)
			if tt.wantFound {
				if err != nil {
					t.Errorf("FindCoverFile(%q) unexpected error: %v", tt.filename, err)
				}
				if !strings.Contains(result, tt.wantDir) {
					t.Errorf("FindCoverFile(%q) result %q should contain directory %q", tt.filename, result, tt.wantDir)
				}
			} else {
				if err == nil {
					t.Errorf("FindCoverFile(%q) expected error, got result: %q", tt.filename, result)
				}
			}
		})
	}
}
