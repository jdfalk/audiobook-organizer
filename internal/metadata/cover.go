// file: internal/metadata/cover.go
// version: 1.0.0
// guid: 4efaa7b8-e29a-47f3-84f7-39b46bfc9a01

package metadata

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DownloadCoverArt downloads a cover image from coverURL and saves it to
// {destDir}/covers/{bookID}.{ext}. Returns the local file path on success.
// Skips download if the file already exists. Only accepts image/* content types.
func DownloadCoverArt(coverURL string, destDir string, bookID string) (string, error) {
	if coverURL == "" {
		return "", fmt.Errorf("empty cover URL")
	}
	if bookID == "" {
		return "", fmt.Errorf("empty book ID")
	}

	coversDir := filepath.Join(destDir, "covers")

	// Check if cover already exists (any extension)
	matches, _ := filepath.Glob(filepath.Join(coversDir, bookID+".*"))
	for _, m := range matches {
		ext := strings.ToLower(filepath.Ext(m))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".gif" {
			return m, nil
		}
	}

	// Create covers directory
	if err := os.MkdirAll(coversDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create covers directory: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(coverURL)
	if err != nil {
		return "", fmt.Errorf("failed to download cover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cover download returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("unexpected content type: %s", contentType)
	}

	ext := extensionFromContentType(contentType)
	destPath := filepath.Join(coversDir, bookID+ext)

	// Limit to 10 MB
	limitedReader := io.LimitReader(resp.Body, 10*1024*1024)

	f, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create cover file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, limitedReader); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("failed to write cover file: %w", err)
	}

	return destPath, nil
}

// CoverPathForBook returns the local cover file path if it exists, empty string otherwise.
func CoverPathForBook(destDir string, bookID string) string {
	coversDir := filepath.Join(destDir, "covers")
	matches, _ := filepath.Glob(filepath.Join(coversDir, bookID+".*"))
	for _, m := range matches {
		ext := strings.ToLower(filepath.Ext(m))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".gif" {
			return m
		}
	}
	return ""
}

func extensionFromContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "webp"):
		return ".webp"
	default:
		return ".jpg"
	}
}
