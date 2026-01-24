//go:build mocks

// file: internal/server/server_coverage_improvements_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-01-23

// This file contains additional tests to improve server package coverage
// from 66% to 80%+, focusing on error paths, edge cases, and validation.

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/stretchr/testify/assert"
)

// TestCountAudiobooksError tests the error path when CountBooks fails
func TestCountAudiobooksError(t *testing.T) {
	// Set gin to test mode
	gin.SetMode(gin.TestMode)

	// Create mock store
	mockStore := mocks.NewMockStore(t)

	// Set up expectation for error scenario
	mockStore.EXPECT().
		CountBooks().
		Return(0, errors.New("database connection failed")).
		Once()

	// Set global store for the test
	oldStore := database.GlobalStore
	database.GlobalStore = mockStore
	defer func() { database.GlobalStore = oldStore }()

	// Create server
	srv := NewServer()

	// Make request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/count", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["error"], "database connection failed")

	mockStore.AssertExpectations(t)
}

// TestGetAudiobookError tests the error path when GetBookByID fails
func TestGetAudiobookError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := mocks.NewMockStore(t)

	// Set up expectation for database error
	mockStore.EXPECT().
		GetBookByID("test-id-123").
		Return(nil, errors.New("database read error")).
		Once()

	oldStore := database.GlobalStore
	database.GlobalStore = mockStore
	defer func() { database.GlobalStore = oldStore }()

	srv := NewServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/test-id-123", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["error"], "database read error")

	mockStore.AssertExpectations(t)
}

// TestGetAudiobookNotFound tests the case when a book doesn't exist
func TestGetAudiobookNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := mocks.NewMockStore(t)

	// Return nil book to simulate not found
	mockStore.EXPECT().
		GetBookByID("nonexistent-id").
		Return(nil, nil).
		Once()

	oldStore := database.GlobalStore
	database.GlobalStore = mockStore
	defer func() { database.GlobalStore = oldStore }()

	srv := NewServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/nonexistent-id", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockStore.AssertExpectations(t)
}

// TestBatchUpdateAudiobooksErrors tests error cases in batch updates
func TestBatchUpdateAudiobooksErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid JSON",
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid",
		},
		{
			name: "empty audiobooks list",
			requestBody: map[string]interface{}{
				"audiobooks": []interface{}{},
			},
			expectedStatus: http.StatusOK, // Empty list is valid but returns success
			expectedError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := mocks.NewMockStore(t)

			oldStore := database.GlobalStore
			database.GlobalStore = mockStore
			defer func() { database.GlobalStore = oldStore }()

			srv := NewServer()

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				assert.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response["error"], tt.expectedError)
			}
		})
	}
}

// TestExportMetadataError tests error handling in metadata export
func TestExportMetadataError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := mocks.NewMockStore(t)

	// Simulate database error
	mockStore.EXPECT().
		GetAllBooks(0, 0).
		Return(nil, errors.New("export failed: database error")).
		Once()

	oldStore := database.GlobalStore
	database.GlobalStore = mockStore
	defer func() { database.GlobalStore = oldStore }()

	srv := NewServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metadata/export", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockStore.AssertExpectations(t)
}

// TestBulkFetchMetadataErrors tests error cases in bulk metadata fetch
func TestBulkFetchMetadataErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid JSON",
			requestBody:    "not json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid request",
		},
		{
			name: "empty book IDs",
			requestBody: map[string]interface{}{
				"book_ids": []string{},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "no book IDs provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := mocks.NewMockStore(t)

			oldStore := database.GlobalStore
			database.GlobalStore = mockStore
			defer func() { database.GlobalStore = oldStore }()

			srv := NewServer()

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				assert.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/metadata/bulk-fetch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCancelOperationErrors tests error handling in operation cancellation
func TestCancelOperationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test case 1: Database not initialized
	t.Run("database not initialized", func(t *testing.T) {
		oldStore := database.GlobalStore
		database.GlobalStore = nil
		defer func() { database.GlobalStore = oldStore }()

		srv := NewServer()

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/operations/test-id", nil)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error"], "database not initialized")
	})

	// Test case 2: Operation queue not initialized
	t.Run("operation queue not initialized", func(t *testing.T) {
		mockStore := mocks.NewMockStore(t)

		oldStore := database.GlobalStore
		oldQueue := operations.GlobalQueue
		database.GlobalStore = mockStore
		operations.GlobalQueue = nil
		defer func() {
			database.GlobalStore = oldStore
			operations.GlobalQueue = oldQueue
		}()

		srv := NewServer()

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/operations/test-id", nil)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error"], "operation queue not initialized")
	})
}

// TestCreateWorkErrors tests error cases in work creation
func TestCreateWorkErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name:           "invalid JSON",
			requestBody:    "invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing required field",
			requestBody: map[string]interface{}{
				"type": "", // empty type
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := mocks.NewMockStore(t)

			oldStore := database.GlobalStore
			database.GlobalStore = mockStore
			defer func() { database.GlobalStore = oldStore }()

			srv := NewServer()

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				assert.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/works", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
