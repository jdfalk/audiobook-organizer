// file: internal/server/metadata_fields_test.go
// version: 1.0.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtendedBookMetadata verifies that new optional metadata fields
// (WorkID, Narrator, Edition, Language, Publisher, ISBN10, ISBN13)
// can be created, retrieved, and updated correctly through the API.
func TestExtendedBookMetadata(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Test data with extended metadata
	narrator := "Roy Dotrice"
	edition := "Unabridged"
	language := "English"
	publisher := "Random House Audio"
	isbn10 := "0553897845"
	isbn13 := "978-0553897845"

	// Create a book directly in the database with extended metadata
	book := &database.Book{
		Title:     "A Game of Thrones",
		FilePath:  "/audiobooks/got.m4b",
		Format:    "m4b",
		Narrator:  &narrator,
		Edition:   &edition,
		Language:  &language,
		Publisher: &publisher,
		ISBN10:    &isbn10,
		ISBN13:    &isbn13,
	}

	createdBook, err := database.GlobalStore.CreateBook(book)
	require.NoError(t, err, "Failed to create book in database")
	require.NotEmpty(t, createdBook.ID, "Book ID should not be empty")
	bookID := createdBook.ID

	// Verify all metadata fields were stored
	assert.Equal(t, narrator, *createdBook.Narrator)
	assert.Equal(t, edition, *createdBook.Edition)
	assert.Equal(t, language, *createdBook.Language)
	assert.Equal(t, publisher, *createdBook.Publisher)
	assert.Equal(t, isbn10, *createdBook.ISBN10)
	assert.Equal(t, isbn13, *createdBook.ISBN13)

	// Retrieve the book via API and verify metadata persisted
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/"+bookID, nil)
	getW := httptest.NewRecorder()
	server.router.ServeHTTP(getW, getReq)

	assert.Equal(t, http.StatusOK, getW.Code)
	var getResp map[string]interface{}
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &getResp))

	assert.Equal(t, narrator, getResp["narrator"])
	assert.Equal(t, edition, getResp["edition"])
	assert.Equal(t, language, getResp["language"])
	assert.Equal(t, publisher, getResp["publisher"])
	assert.Equal(t, isbn10, getResp["isbn10"])
	assert.Equal(t, isbn13, getResp["isbn13"])

	// Update some metadata fields (must send all fields to avoid clearing)
	updatedNarrator := "Stephen Fry"
	updatedLanguage := "English (UK)"
	updatePayload := map[string]interface{}{
		"title":     "A Game of Thrones",
		"file_path": "/audiobooks/got.m4b",
		"format":    "m4b",
		"narrator":  updatedNarrator,
		"language":  updatedLanguage,
		"edition":   edition,
		"publisher": publisher,
		"isbn10":    isbn10,
		"isbn13":    isbn13,
	}
	updateBody, _ := json.Marshal(updatePayload)

	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/audiobooks/"+bookID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	server.router.ServeHTTP(updateW, updateReq)

	assert.Equal(t, http.StatusOK, updateW.Code)
	var updateResp map[string]interface{}
	require.NoError(t, json.Unmarshal(updateW.Body.Bytes(), &updateResp))

	// Verify updated fields
	assert.Equal(t, updatedNarrator, updateResp["narrator"])
	assert.Equal(t, updatedLanguage, updateResp["language"])
	// Verify unchanged fields remain
	assert.Equal(t, edition, updateResp["edition"])
	assert.Equal(t, publisher, updateResp["publisher"])
	assert.Equal(t, isbn10, updateResp["isbn10"])
	assert.Equal(t, isbn13, updateResp["isbn13"])
}

// TestBookWithoutExtendedMetadata verifies that books created without
// the new optional fields work correctly (backward compatibility).
func TestBookWithoutExtendedMetadata(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a book directly in database without any extended metadata (legacy behavior)
	book := &database.Book{
		Title:    "The Hobbit",
		FilePath: "/audiobooks/hobbit.mp3",
		Format:   "mp3",
	}

	createdBook, err := database.GlobalStore.CreateBook(book)
	require.NoError(t, err, "Failed to create book in database")
	require.NotEmpty(t, createdBook.ID, "Book ID should not be empty")
	bookID := createdBook.ID

	// Verify book was created without optional fields
	assert.Nil(t, createdBook.Narrator)
	assert.Nil(t, createdBook.Edition)
	assert.Nil(t, createdBook.Language)
	assert.Nil(t, createdBook.Publisher)
	assert.Nil(t, createdBook.ISBN10)
	assert.Nil(t, createdBook.ISBN13)

	// Retrieve via API and verify fields are absent
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/"+bookID, nil)
	getW := httptest.NewRecorder()
	server.router.ServeHTTP(getW, getReq)

	assert.Equal(t, http.StatusOK, getW.Code)
	var getResp map[string]interface{}
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &getResp))

	// API should not include nil fields in JSON response
	_, hasNarrator := getResp["narrator"]
	_, hasEdition := getResp["edition"]
	_, hasLanguage := getResp["language"]
	assert.False(t, hasNarrator, "Narrator field should not be present")
	assert.False(t, hasEdition, "Edition field should not be present")
	assert.False(t, hasLanguage, "Language field should not be present")
}
