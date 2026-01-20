// file: internal/scanner/scanner_test.go
// version: 1.0.0
// guid: 5c1a2b3c-4d5e-6f7a-8b9c-0d1e2f3a4b5c

package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

func withTempBooks(t *testing.T, names []string) []Book {
	t.Helper()
	dir := t.TempDir()
	books := make([]Book, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to write temp book %s: %v", name, err)
		}
		books = append(books, Book{FilePath: path, Format: filepath.Ext(path)})
	}
	return books
}

func TestScanDirectoryParallelFiltersExtensions(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() { config.AppConfig.SupportedExtensions = oldExts })
	config.AppConfig.SupportedExtensions = []string{".m4b", ".mp3"}

	dir := t.TempDir()
	allowed := filepath.Join(dir, "keep.m4b")
	other := filepath.Join(dir, "skip.txt")
	if err := os.WriteFile(allowed, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write allowed: %v", err)
	}
	if err := os.WriteFile(other, []byte("noop"), 0o644); err != nil {
		t.Fatalf("write other: %v", err)
	}

	books, err := ScanDirectoryParallel(dir, 2)
	if err != nil {
		t.Fatalf("ScanDirectoryParallel error: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 audiobook, got %d", len(books))
	}
	if books[0].FilePath != allowed {
		t.Fatalf("unexpected file path: %s", books[0].FilePath)
	}
}

func TestProcessBooksParallelInvokesProgress(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	oldWorkers := config.AppConfig.ConcurrentScans
	t.Cleanup(func() {
		config.AppConfig.SupportedExtensions = oldExts
		config.AppConfig.ConcurrentScans = oldWorkers
	})
	config.AppConfig.SupportedExtensions = []string{".m4b"}
	config.AppConfig.ConcurrentScans = 2

	books := withTempBooks(t, []string{"book1.m4b", "book2.m4b", "book3.m4b"})

	var progressCalls []int
	var processedPaths []string

	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(book *Book) error { return nil }

	progressFn := func(processed int, total int, bookPath string) {
		progressCalls = append(progressCalls, processed)
		processedPaths = append(processedPaths, filepath.Base(bookPath))
		if total != len(books) {
			t.Errorf("expected total %d, got %d", len(books), total)
		}
	}

	if err := ProcessBooksParallel(context.Background(), books, 2, progressFn); err != nil {
		t.Fatalf("ProcessBooksParallel returned error: %v", err)
	}

	if len(progressCalls) != len(books) {
		t.Fatalf("expected %d progress calls, got %d", len(books), len(progressCalls))
	}

	for i, processed := range progressCalls {
		expected := i + 1
		if processed != expected {
			t.Fatalf("progress call %d expected %d processed, got %d", i, expected, processed)
		}
	}

	for _, book := range books {
		found := false
		for _, seen := range processedPaths {
			if filepath.Base(book.FilePath) == seen {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("book %s not observed in progress callback", filepath.Base(book.FilePath))
		}
	}
}

func TestExtractInfoFromPathParsesTitleAndAuthor(t *testing.T) {
	b := &Book{FilePath: filepath.Join("/tmp", "Jane Doe", "My Story - Jane Doe.m4b")}
	extractInfoFromPath(b)
	if b.Author != "Jane Doe" {
		t.Fatalf("expected author 'Jane Doe', got '%s'", b.Author)
	}
	if b.Title == "" {
		t.Fatalf("expected title to be set")
	}
}

func TestParseFilenameForAuthor(t *testing.T) {
	title, author := parseFilenameForAuthor("The Stand - Stephen King")
	if title != "The Stand" || author != "Stephen King" {
		t.Fatalf("unexpected parse result: title=%s author=%s", title, author)
	}

	title, author = parseFilenameForAuthor("No Author Here")
	if title != "" || author != "" {
		t.Fatalf("expected empty parse result for non-standard filename")
	}
}

func TestLooksLikePersonName(t *testing.T) {
	cases := map[string]bool{
		"Jane Doe":   true,
		"J. K. R":    true,
		"volume one": false,
		"12345":      false,
	}
	for name, expected := range cases {
		if looksLikePersonName(name) != expected {
			t.Fatalf("looksLikePersonName(%s) expected %v", name, expected)
		}
	}
}

func TestComputeFileHash(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hash.m4b")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := ComputeFileHash(path)
	if err != nil {
		t.Fatalf("ComputeFileHash error: %v", err)
	}
	expected := sha256.Sum256(content)
	if got != hex.EncodeToString(expected[:]) {
		t.Fatalf("unexpected hash: got %s want %s", got, hex.EncodeToString(expected[:]))
	}
}

func TestScanDirectory(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() { config.AppConfig.SupportedExtensions = oldExts })
	config.AppConfig.SupportedExtensions = []string{".m4b", ".mp3"}

	dir := t.TempDir()
	allowed := filepath.Join(dir, "test.m4b")
	if err := os.WriteFile(allowed, []byte("test"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	books, err := ScanDirectory(dir)
	if err != nil {
		t.Fatalf("ScanDirectory error: %v", err)
	}
	if len(books) != 1 {
		t.Errorf("expected 1 book, got %d", len(books))
	}
}

func TestExtractAuthorFromDirectory(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "author in directory",
			filePath: "/media/audiobooks/John Smith/Book Title.m4b",
			want:     "John Smith",
		},
		{
			name:     "author-title pattern",
			filePath: "/media/Stephen King - The Stand/book.m4b",
			want:     "Stephen King",
		},
		{
			name:     "skip common directories",
			filePath: "/media/audiobooks/book.m4b",
			want:     "",
		},
		{
			name:     "skip downloads directory",
			filePath: "/downloads/book.m4b",
			want:     "",
		},
		{
			name:     "translator pattern",
			filePath: "/media/Haruki Murakami - translator - Jay Rubin/1Q84.m4b",
			want:     "Haruki Murakami",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAuthorFromDirectory(tt.filePath)
			if got != tt.want {
				t.Errorf("extractAuthorFromDirectory(%q) = %q, want %q", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestIsValidAuthor(t *testing.T) {
	tests := []struct {
		author string
		valid  bool
	}{
		{"Stephen King", true},
		{"J.K. Rowling", true},
		{"", false},
		{"book1", false},
		{"chapter 5", false},
		{"Part One", false},
		{"Volume 2", false},
		{"123", false},
		{"Disc 1", false},
	}

	for _, tt := range tests {
		t.Run(tt.author, func(t *testing.T) {
			got := isValidAuthor(tt.author)
			if got != tt.valid {
				t.Errorf("isValidAuthor(%q) = %v, want %v", tt.author, got, tt.valid)
			}
		})
	}
}

func TestExtractInfoFromPathVariants(t *testing.T) {
	tests := []struct {
		name       string
		filePath   string
		wantTitle  string
		wantAuthor string
		wantSeries string
	}{
		{
			name:       "numbered chapter stripped",
			filePath:   "/media/Book Name-01 Chapter 01.m4b",
			wantTitle:  "Book Name",
			wantAuthor: "",
		},
		{
			name:       "track number prefix",
			filePath:   "/media/01 Book Title.m4b",
			wantTitle:  "Book Title",
			wantAuthor: "",
		},
		{
			name:      "simple filename",
			filePath:  "/media/SimpleBook.m4b",
			wantTitle: "SimpleBook",
		},
		{
			name:       "author-title pattern with author name",
			filePath:   "/media/The Stand - Stephen King.m4b",
			wantTitle:  "The Stand",
			wantAuthor: "Stephen King",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := &Book{FilePath: tt.filePath}
			extractInfoFromPath(book)

			if tt.wantTitle != "" && book.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", book.Title, tt.wantTitle)
			}
			if tt.wantAuthor != "" && book.Author != tt.wantAuthor {
				t.Errorf("author = %q, want %q", book.Author, tt.wantAuthor)
			}
			if tt.wantSeries != "" && book.Series != tt.wantSeries {
				t.Errorf("series = %q, want %q", book.Series, tt.wantSeries)
			}
		})
	}
}

func TestProcessBooksParallelCancellation(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() { config.AppConfig.SupportedExtensions = oldExts })
	config.AppConfig.SupportedExtensions = []string{".m4b"}

	books := withTempBooks(t, []string{"book1.m4b", "book2.m4b", "book3.m4b"})

	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(book *Book) error { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := ProcessBooksParallel(ctx, books, 2, nil)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestScanDirectoryParallelNegativeWorkers(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() { config.AppConfig.SupportedExtensions = oldExts })
	config.AppConfig.SupportedExtensions = []string{".m4b"}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.m4b")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	books, err := ScanDirectoryParallel(dir, -1)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(books) != 1 {
		t.Errorf("expected 1 book, got %d", len(books))
	}
}

func TestProcessBooksParallelNegativeWorkers(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() { config.AppConfig.SupportedExtensions = oldExts })
	config.AppConfig.SupportedExtensions = []string{".m4b"}

	books := withTempBooks(t, []string{"book1.m4b"})

	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(book *Book) error { return nil }

	// Should handle negative workers gracefully (defaults to 1)
	err := ProcessBooksParallel(context.Background(), books, -1, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestComputeFileHashError(t *testing.T) {
	_, err := ComputeFileHash("/nonexistent/file.m4b")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("stringPtrValue", func(t *testing.T) {
		s := "test"
		ptr := stringPtrValue(s)
		if ptr == nil || *ptr != s {
			t.Error("stringPtrValue failed")
		}
	})

	t.Run("stringPtr", func(t *testing.T) {
		ptr := stringPtr("test")
		if ptr == nil || *ptr != "test" {
			t.Error("stringPtr failed")
		}
	})

	t.Run("intPtr", func(t *testing.T) {
		ptr := intPtr(42)
		if ptr == nil || *ptr != 42 {
			t.Error("intPtr failed")
		}
	})

	t.Run("nullablePtr empty", func(t *testing.T) {
		ptr := nullablePtr("")
		if ptr != nil {
			t.Error("nullablePtr should return nil for empty string")
		}
	})

	t.Run("nullablePtr whitespace", func(t *testing.T) {
		ptr := nullablePtr("   ")
		if ptr != nil {
			t.Error("nullablePtr should return nil for whitespace")
		}
	})

	t.Run("nullablePtr value", func(t *testing.T) {
		ptr := nullablePtr("test")
		if ptr == nil || *ptr != "test" {
			t.Error("nullablePtr failed for non-empty string")
		}
	})
}

func TestIdentifySeriesUsingExternalAPIs(t *testing.T) {
	// This is a placeholder function that returns nil
	err := identifySeriesUsingExternalAPIs([]Book{{Title: "Test"}})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProcessBooks(t *testing.T) {
	oldExts := config.AppConfig.SupportedExtensions
	oldWorkers := config.AppConfig.ConcurrentScans
	t.Cleanup(func() {
		config.AppConfig.SupportedExtensions = oldExts
		config.AppConfig.ConcurrentScans = oldWorkers
	})
	config.AppConfig.SupportedExtensions = []string{".m4b"}
	config.AppConfig.ConcurrentScans = 1

	books := withTempBooks(t, []string{"book1.m4b"})

	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(book *Book) error { return nil }

	err := ProcessBooks(books)
	if err != nil {
		t.Errorf("ProcessBooks error: %v", err)
	}
}
