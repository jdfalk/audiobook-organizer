// file: internal/server/audiobook_update_service_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestAudiobookUpdateService_ValidateRequest_EmptyID(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	_, err := service.ValidateRequest("", map[string]interface{}{})

	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestAudiobookUpdateService_ValidateRequest_NoUpdates(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	_, err := service.ValidateRequest("book1", map[string]interface{}{})

	if err == nil {
		t.Error("expected error for empty updates")
	}
}

func TestAudiobookUpdateService_ExtractStringField(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	payload := map[string]interface{}{
		"title": "New Title",
	}

	result, ok := service.ExtractStringField(payload, "title")

	if !ok || result != "New Title" {
		t.Errorf("expected 'New Title', got %q (ok=%v)", result, ok)
	}
}

func TestAudiobookUpdateService_ExtractIntField(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	payload := map[string]interface{}{
		"author_id": float64(42),
	}

	result, ok := service.ExtractIntField(payload, "author_id")

	if !ok || result != 42 {
		t.Errorf("expected 42, got %d (ok=%v)", result, ok)
	}
}

func TestAudiobookUpdateService_ExtractStringField_NotFound(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	payload := map[string]interface{}{}

	_, ok := service.ExtractStringField(payload, "missing")

	if ok {
		t.Error("expected ok=false for missing field")
	}
}

func TestAudiobookUpdateService_ExtractOverrides_Success(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	payload := map[string]interface{}{
		"overrides": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}

	result, ok := service.ExtractOverrides(payload)

	if !ok || len(result) != 2 {
		t.Errorf("expected 2 overrides, got %d (ok=%v)", len(result), ok)
	}
}

func TestAudiobookUpdateService_ApplyUpdatesToBook(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	book := &database.Book{
		ID:    "book1",
		Title: "Original Title",
	}

	updates := map[string]interface{}{
		"title": "Updated Title",
	}

	service.ApplyUpdatesToBook(book, updates)

	if book.Title != "Updated Title" {
		t.Errorf("expected 'Updated Title', got %q", book.Title)
	}
}

func TestAudiobookUpdateService_ApplyUpdatesToBook_MultipleFields(t *testing.T) {
	service := NewAudiobookUpdateService(&database.MockStore{})

	book := &database.Book{
		ID:    "book1",
		Title: "Original Title",
	}

	authorID := 10
	updates := map[string]interface{}{
		"title":     "Updated Title",
		"author_id": float64(authorID),
	}

	service.ApplyUpdatesToBook(book, updates)

	if book.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %q", book.Title)
	}
	if book.AuthorID == nil || *book.AuthorID != authorID {
		t.Errorf("expected author_id %d, got %v", authorID, book.AuthorID)
	}
}
