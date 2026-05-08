package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	queuemocks "github.com/jdfalk/audiobook-organizer/internal/operations/mocks"
	"github.com/stretchr/testify/assert"
)

// TestCancelOperationWithQueueMock verifies the DELETE /operations/:id handler
// uses the queue's Cancel method, falling back to DB update when the queue
// returns an error or is nil.
func TestCancelOperationWithQueueMock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		operationID    string
		mockStoreSetup func(*dbmocks.MockStore)
		mockQueueSetup func(*queuemocks.MockQueue)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:        "successfully cancel operation",
			operationID: "test-op-123",
			mockStoreSetup: func(m *dbmocks.MockStore) {
				// No database expectations needed for this test
			},
			mockQueueSetup: func(m *queuemocks.MockQueue) {
				m.EXPECT().Cancel("test-op-123").Return(nil).Once()
			},
			expectedStatus: http.StatusNoContent,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				// 204 No Content has no body
				assert.Empty(t, w.Body.String())
			},
		},
		{
			name:        "queue cancel error falls back to DB",
			operationID: "test-op-456",
			mockStoreSetup: func(m *dbmocks.MockStore) {
				m.EXPECT().UpdateOperationStatus("test-op-456", "canceled", 0, 0, "force canceled (stale operation)").
					Return(nil).Once()
			},
			mockQueueSetup: func(m *queuemocks.MockQueue) {
				m.EXPECT().Cancel("test-op-456").
					Return(errors.New("operation test-op-456 not found")).Once()
			},
			expectedStatus: http.StatusNoContent,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Empty(t, w.Body.String())
			},
		},
		{
			name:        "nil queue falls back to DB update",
			operationID: "test-op-789",
			mockStoreSetup: func(m *dbmocks.MockStore) {
				m.EXPECT().UpdateOperationStatus("test-op-789", "canceled", 0, 0, "force canceled (stale operation)").
					Return(nil).Once()
			},
			mockQueueSetup: nil, // Don't set up queue - leave it nil
			expectedStatus: http.StatusNoContent,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Empty(t, w.Body.String())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock store
			mockStore := dbmocks.NewMockStore(t)
			tt.mockStoreSetup(mockStore)
			database.SetGlobalStore(mockStore)

			// Create server
			server := &Server{
				router: gin.New(),
			}

			// Create and setup mock queue (or leave nil for nil test)
			if tt.mockQueueSetup != nil {
				mockQueue := queuemocks.NewMockQueue(t)
				tt.mockQueueSetup(mockQueue)
				server.queue = mockQueue
			} else {
				server.queue = nil
			}
			server.setupRoutes()

			// Make request
			req := httptest.NewRequest("DELETE", "/api/v1/operations/"+tt.operationID, nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)
			tt.checkResponse(t, w)
		})
	}
}

// TestGetOperationsWithQueueMock verifies GET /operations/active returns 410 Gone
// since UOS-14 removed this endpoint. Use GET /operations/timeline instead.
func TestGetOperationsWithQueueMock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		mockQueueSetup func(*queuemocks.MockQueue)
	}{
		{
			name:           "with queue - returns 410 gone",
			mockQueueSetup: nil,
		},
		{
			name:           "nil queue - returns 410 gone",
			mockQueueSetup: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := dbmocks.NewMockStore(t)
			database.SetGlobalStore(mockStore)

			server := &Server{
				router: gin.New(),
			}

			if tt.mockQueueSetup != nil {
				mockQueue := queuemocks.NewMockQueue(t)
				tt.mockQueueSetup(mockQueue)
				server.queue = mockQueue
			} else {
				server.queue = nil
			}
			server.setupRoutes()

			// UOS-14: /operations/active is removed; must return 410 Gone
			req := httptest.NewRequest("GET", "/api/v1/operations/active", nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusGone, w.Code)
			assert.Contains(t, w.Body.String(), "gone")
		})
	}
}
