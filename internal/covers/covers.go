// file: internal/covers/covers.go
// version: 1.0.0
// guid: c3d4e5f6-7890-abcd-ef12-34567890abcd
// last-edited: 2026-05-11
//
// Cover service logic for proxy caching and validation.
// Business logic extracted from internal/server/covers.go.

package covers

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ProxyCoverRequest holds parameters for proxying a cover image.
type ProxyCoverRequest struct {
	URL      string
	CacheDir string
	RootDir  string
}

// ProxyCoverResult holds the result of a proxy operation.
type ProxyCoverResult struct {
	CachePath string
	Error     string
}

// IsAllowedCoverSource validates that a URL is from an approved cover source.
func IsAllowedCoverSource(url string) bool {
	allowed := []string{
		"https://covers.openlibrary.org/",
		"http://covers.openlibrary.org/",
		"https://books.google.com/",
		"http://books.google.com/",
		"https://images-na.ssl-images-amazon.com/",
		"http://images-na.ssl-images-amazon.com/",
		"https://images.amazon.com/",
		"http://images.amazon.com/",
	}
	for _, prefix := range allowed {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}

// GetCachePath computes the cache path for a given cover URL.
func GetCachePath(coverURL, cacheDir string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(coverURL)))
	ext := ".jpg"
	if strings.Contains(coverURL, ".png") {
		ext = ".png"
	}
	return filepath.Join(cacheDir, hash+ext)
}

// FetchAndCacheCover fetches a cover from a URL and caches it.
// Returns the cache path on success or an error string.
func FetchAndCacheCover(coverURL, cacheDir string) (string, string) {
	// Create cache directory
	if err := os.MkdirAll(cacheDir, 0775); err != nil {
		return "", "failed to create cache directory"
	}

	cachePath := GetCachePath(coverURL, cacheDir)

	// Check if already cached
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, ""
	}

	// Fetch from source
	resp, err := http.Get(coverURL) //nolint:gosec // URL is validated by caller
	if err != nil {
		return "", "failed to fetch cover"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "cover source returned error"
	}

	// Write to cache
	f, err := os.Create(cachePath)
	if err != nil {
		return "", "failed to cache cover"
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(cachePath)
		return "", "failed to write cover"
	}
	f.Close()

	return cachePath, ""
}

// FindCoverFile searches for a cover file in standard directories.
func FindCoverFile(filename string, rootDir string) (string, error) {
	dirs := []string{
		filepath.Join(rootDir, ".covers"),
		filepath.Join(rootDir, "covers"),
	}
	for _, dir := range dirs {
		coverPath := filepath.Join(dir, filename)
		if _, err := os.Stat(coverPath); err == nil {
			return coverPath, nil
		}
	}
	return "", os.ErrNotExist
}
