// file: internal/metadata/cover_test.go
// version: 1.0.0
// guid: 5fa1b8c9-d3e4-48f5-95a8-4ac57cde0b12

package metadata

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadCoverArt_Success(t *testing.T) {
	// Serve a fake JPEG
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte{0xFF, 0xD8, 0xFF}) // JPEG magic bytes
	}))
	defer srv.Close()

	dir := t.TempDir()
	path, err := DownloadCoverArt(srv.URL+"/cover.jpg", dir, "book123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(path) != "book123.jpg" {
		t.Errorf("expected book123.jpg, got %s", filepath.Base(path))
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestDownloadCoverArt_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	coversDir := filepath.Join(dir, "covers")
	os.MkdirAll(coversDir, 0755)
	existing := filepath.Join(coversDir, "book123.jpg")
	os.WriteFile(existing, []byte("fake"), 0644)

	path, err := DownloadCoverArt("http://should-not-be-called.example.com/cover.jpg", dir, "book123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != existing {
		t.Errorf("expected existing path %s, got %s", existing, path)
	}
}

func TestDownloadCoverArt_RejectsNonImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	_, err := DownloadCoverArt(srv.URL+"/notimage", dir, "book456")
	if err == nil {
		t.Fatal("expected error for non-image content type")
	}
}

func TestDownloadCoverArt_EmptyInputs(t *testing.T) {
	if _, err := DownloadCoverArt("", "/tmp", "book"); err == nil {
		t.Error("expected error for empty URL")
	}
	if _, err := DownloadCoverArt("http://example.com/img.jpg", "/tmp", ""); err == nil {
		t.Error("expected error for empty bookID")
	}
}

func TestDownloadCoverArt_PNG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{0x89, 0x50, 0x4E, 0x47})
	}))
	defer srv.Close()

	dir := t.TempDir()
	path, err := DownloadCoverArt(srv.URL+"/cover.png", dir, "bookpng")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Ext(path) != ".png" {
		t.Errorf("expected .png extension, got %s", filepath.Ext(path))
	}
}

func TestCoverPathForBook(t *testing.T) {
	dir := t.TempDir()
	coversDir := filepath.Join(dir, "covers")
	os.MkdirAll(coversDir, 0755)

	// No cover exists
	if got := CoverPathForBook(dir, "missing"); got != "" {
		t.Errorf("expected empty, got %s", got)
	}

	// Create one
	os.WriteFile(filepath.Join(coversDir, "found.jpg"), []byte("img"), 0644)
	if got := CoverPathForBook(dir, "found"); got == "" {
		t.Error("expected to find cover")
	}
}

func TestExtensionFromContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want string
	}{
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"image/something", ".jpg"},
	}
	for _, tt := range tests {
		if got := extensionFromContentType(tt.ct); got != tt.want {
			t.Errorf("extensionFromContentType(%q) = %q, want %q", tt.ct, got, tt.want)
		}
	}
}
