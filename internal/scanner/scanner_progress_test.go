// file: internal/scanner/scanner_progress_test.go
// version: 1.0.0
// guid: 0b4c7f0a-2d7f-4e9b-9a5c-1234567890ab

package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

func TestProcessBooksParallelInvokesProgress(t *testing.T) {
	tmp := t.TempDir()

	// Configure supported extensions for test files
	config.AppConfig.SupportedExtensions = []string{".m4b"}
	config.AppConfig.ConcurrentScans = 2

	books := []Book{
		{FilePath: filepath.Join(tmp, "book1.m4b"), Format: ".m4b"},
		{FilePath: filepath.Join(tmp, "book2.m4b"), Format: ".m4b"},
		{FilePath: filepath.Join(tmp, "book3.m4b"), Format: ".m4b"},
	}

	for _, b := range books {
		if err := os.WriteFile(b.FilePath, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to write temp book: %v", err)
		}
	}

	var progressCalls []int
	var processedPaths []string

	oldSaver := saveBook
	defer func() { saveBook = oldSaver }()
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
