//go:build mocks

package server

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
)

// TestGetAudiobookTagsErrors tests error scenarios for getAudiobookTags (59.3% → target 70%+)
func TestGetAudiobookTagsErrors(t *testing.T) {
	tests := []struct {
		name       string
		bookID     string
		mockSetup  func(*mocks.MockStore)
		statusCode int
	}{
		{
			name:   "database error",
			bookID: "01HQWKV1234567890ABCDEFGHJK",
			mockSetup: func(m *mocks.MockStore) {
				m.EXPECT().GetBookByID("01HQWKV1234567890ABCDEFGHJK").Return(nil, errors.New("database connection error")).Once()
			},
			statusCode: http.StatusInternalServerError,
		},
		{
			name:   "book not found",
			bookID: "01HQWKV9999999999999999999",
			mockSetup: func(m *mocks.MockStore) {
				m.EXPECT().GetBookByID("01HQWKV9999999999999999999").Return(nil, nil).Once()
			},
			statusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			mockStore := mocks.NewMockStore(t)
			tt.mockSetup(mockStore)

			oldStore := database.GlobalStore
			database.GlobalStore = mockStore
			defer func() { database.GlobalStore = oldStore }()

			srv := NewServer()

			req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/"+tt.bookID+"/tags", nil)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
			mockStore.AssertExpectations(t)
		})
	}
}

// TestAddBlockedHashErrors tests error scenarios for addBlockedHash (60.0% → target 70%+)
func TestAddBlockedHashErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		statusCode int
		errMsg     string
	}{
		{
			name:       "invalid JSON",
			body:       `{"invalid": json}`,
			statusCode: http.StatusBadRequest,
			errMsg:     "invalid",
		},
		{
			name:       "missing hash field",
			body:       `{"reason": "duplicate"}`,
			statusCode: http.StatusBadRequest,
			errMsg:     "Hash",
		},
		{
			name:       "missing reason field",
			body:       `{"hash": "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"}`,
			statusCode: http.StatusBadRequest,
			errMsg:     "Reason",
		},
		{
			name:       "hash too short",
			body:       `{"hash": "abc123", "reason": "duplicate"}`,
			statusCode: http.StatusBadRequest,
			errMsg:     "64 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			mockStore := mocks.NewMockStore(t)

			oldStore := database.GlobalStore
			database.GlobalStore = mockStore
			defer func() { database.GlobalStore = oldStore }()

			srv := NewServer()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/blocked-hashes", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
			if tt.errMsg != "" {
				assert.Contains(t, w.Body.String(), tt.errMsg)
			}
			mockStore.AssertExpectations(t)
		})
	}
}

// TestAddBlockedHashDatabaseError tests database failure in addBlockedHash
func TestAddBlockedHashDatabaseError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockStore := mocks.NewMockStore(t)

	validHash := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	mockStore.EXPECT().
		AddBlockedHash(validHash, "duplicate file").
		Return(errors.New("database error: constraint violation")).
		Once()

	oldStore := database.GlobalStore
	database.GlobalStore = mockStore
	defer func() { database.GlobalStore = oldStore }()

	srv := NewServer()

	body := `{"hash": "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", "reason": "duplicate file"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/blocked-hashes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockStore.AssertExpectations(t)
}

// TestDeleteWorkErrors tests error scenarios for deleteWork (63.6% → target 75%+)
func TestDeleteWorkErrors(t *testing.T) {
	tests := []struct {
		name       string
		workID     string
		mockSetup  func(*mocks.MockStore)
		statusCode int
	}{
		{
			name:   "database error",
			workID: "01HQWKV1234567890ABCDEFGHJK",
			mockSetup: func(m *mocks.MockStore) {
				m.EXPECT().DeleteWork("01HQWKV1234567890ABCDEFGHJK").Return(errors.New("database connection error")).Once()
			},
			statusCode: http.StatusInternalServerError,
		},
		{
			name:   "work not found",
			workID: "01HQWKV9999999999999999999",
			mockSetup: func(m *mocks.MockStore) {
				m.EXPECT().DeleteWork("01HQWKV9999999999999999999").Return(errors.New("work not found")).Once()
			},
			statusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			mockStore := mocks.NewMockStore(t)
			tt.mockSetup(mockStore)

			oldStore := database.GlobalStore
			database.GlobalStore = mockStore
			defer func() { database.GlobalStore = oldStore }()

			srv := NewServer()

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/works/"+tt.workID, nil)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
			mockStore.AssertExpectations(t)
		})
	}
}
