//go:build mocks

package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	queuemocks "github.com/jdfalk/audiobook-organizer/internal/operations/mocks"
	"github.com/stretchr/testify/assert"
)

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
			name:        "queue cancel error",
			operationID: "test-op-456",
			mockStoreSetup: func(m *dbmocks.MockStore) {
				// No database expectations needed
			},
			mockQueueSetup: func(m *queuemocks.MockQueue) {
				m.EXPECT().Cancel("test-op-456").
					Return(errors.New("operation test-op-456 not found")).Once()
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Contains(t, w.Body.String(), "operation test-op-456 not found")
			},
		},
		{
			name:        "nil queue error",
			operationID: "test-op-789",
			mockStoreSetup: func(m *dbmocks.MockStore) {
				// No database expectations needed
			},
			mockQueueSetup: nil, // Don't set up queue - leave it nil
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Contains(t, w.Body.String(), "queue not initialized")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock store
			mockStore := dbmocks.NewMockStore(t)
			tt.mockStoreSetup(mockStore)
			database.GlobalStore = mockStore

			// Create and setup mock queue (or leave nil for nil test)
			if tt.mockQueueSetup != nil {
				mockQueue := queuemocks.NewMockQueue(t)
				tt.mockQueueSetup(mockQueue)
				operations.GlobalQueue = mockQueue
			} else {
				operations.GlobalQueue = nil
			}

			// Create server
			server := &Server{
				router: gin.New(),
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

func TestGetOperationsWithQueueMock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		mockStoreSetup func(*dbmocks.MockStore)
		mockQueueSetup func(*queuemocks.MockQueue)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "successfully get active operations",
			mockStoreSetup: func(m *dbmocks.MockStore) {
				// listActiveOperations calls GetOperationByID for each operation
				m.EXPECT().GetOperationByID("op1").Return(&database.Operation{
					ID:       "op1",
					Type:     "scan",
					Status:   "running",
					Progress: 5,
					Total:    10,
					Message:  "scanning files",
				}, nil).Once()
				m.EXPECT().GetOperationByID("op2").Return(&database.Operation{
					ID:       "op2",
					Type:     "organize",
					Status:   "queued",
					Progress: 0,
					Total:    0,
					Message:  "",
				}, nil).Once()
			},
			mockQueueSetup: func(m *queuemocks.MockQueue) {
				m.EXPECT().ActiveOperations().Return([]operations.ActiveOperation{
					{ID: "op1", Type: "scan"},
					{ID: "op2", Type: "organize"},
				}).Once()
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := w.Body.String()
				assert.Contains(t, body, "op1")
				assert.Contains(t, body, "op2")
				assert.Contains(t, body, "scan")
				assert.Contains(t, body, "organize")
			},
		},
		{
			name: "empty active operations list",
			mockStoreSetup: func(m *dbmocks.MockStore) {
				// No database expectations needed
			},
			mockQueueSetup: func(m *queuemocks.MockQueue) {
				m.EXPECT().ActiveOperations().Return([]operations.ActiveOperation{}).Once()
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Contains(t, w.Body.String(), "[]")
			},
		},
		{
			name: "nil queue returns empty array",
			mockStoreSetup: func(m *dbmocks.MockStore) {
				// No database expectations needed
			},
			mockQueueSetup: nil, // Don't set up queue
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Contains(t, w.Body.String(), "[]")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock store
			mockStore := dbmocks.NewMockStore(t)
			tt.mockStoreSetup(mockStore)
			database.GlobalStore = mockStore

			// Create and setup mock queue (or leave nil)
			if tt.mockQueueSetup != nil {
				mockQueue := queuemocks.NewMockQueue(t)
				tt.mockQueueSetup(mockQueue)
				operations.GlobalQueue = mockQueue
			} else {
				operations.GlobalQueue = nil
			}

			// Create server
			server := &Server{
				router: gin.New(),
			}
			server.setupRoutes()

			// Make request
			req := httptest.NewRequest("GET", "/api/v1/operations/active", nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)
			tt.checkResponse(t, w)
		})
	}
}
