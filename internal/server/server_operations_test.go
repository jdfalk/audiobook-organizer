// file: internal/server/server_operations_test.go
// version: 1.0.1
// guid: 7f1b2c3d-4e5f-6789-a0b1-c2d3e4f5a6b7
// last-edited: 2026-01-24

//go:build mocks

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	queuemocks "github.com/jdfalk/audiobook-organizer/internal/operations/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStartScan_WithMocks_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Arrange globals
	mockStore := dbmocks.NewMockStore(t)
	database.GlobalStore = mockStore

	mockQueue := queuemocks.NewMockQueue(t)
	operations.GlobalQueue = mockQueue

	// Request body
	folder := "/tmp/folderA"
	body := map[string]interface{}{
		"folder_path": folder,
	}
	buf, _ := json.Marshal(body)

	// Expectations
	returnedOp := &database.Operation{ID: "op-scan-123", Type: "scan"}
	mockStore.EXPECT().CreateOperation(mock.Anything, "scan", &folder).Return(returnedOp, nil).Once()
	mockQueue.EXPECT().Enqueue("op-scan-123", "scan", operations.PriorityNormal, mock.Anything).Return(nil).Once()

	// Server
	srv := &Server{router: gin.New()}
	srv.setupRoutes()

	// Act
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/scan", bytes.NewBuffer(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusAccepted, w.Code)
	var resp database.Operation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, returnedOp.ID, resp.ID)
	assert.Equal(t, "scan", resp.Type)
}

func TestStartScan_QueueNil_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := dbmocks.NewMockStore(t)
	database.GlobalStore = mockStore
	operations.GlobalQueue = nil

	srv := &Server{router: gin.New()}
	srv.setupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/scan", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "operation queue not initialized")
}

func TestStartScan_EnqueueError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := dbmocks.NewMockStore(t)
	database.GlobalStore = mockStore
	mockQueue := queuemocks.NewMockQueue(t)
	operations.GlobalQueue = mockQueue

	returnedOp := &database.Operation{ID: "op-scan-err", Type: "scan"}
	mockStore.EXPECT().CreateOperation(mock.Anything, "scan", (*string)(nil)).Return(returnedOp, nil).Once()
	mockQueue.EXPECT().Enqueue("op-scan-err", "scan", operations.PriorityNormal, mock.Anything).Return(assert.AnError).Once()

	srv := &Server{router: gin.New()}
	srv.setupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/scan", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestStartOrganize_WithMocks_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := dbmocks.NewMockStore(t)
	database.GlobalStore = mockStore
	mockQueue := queuemocks.NewMockQueue(t)
	operations.GlobalQueue = mockQueue

	returnedOp := &database.Operation{ID: "op-org-123", Type: "organize"}
	mockStore.EXPECT().CreateOperation(mock.Anything, "organize", (*string)(nil)).Return(returnedOp, nil).Once()
	mockQueue.EXPECT().Enqueue("op-org-123", "organize", operations.PriorityNormal, mock.Anything).Return(nil).Once()

	srv := &Server{router: gin.New()}
	srv.setupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/organize", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	var resp database.Operation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, returnedOp.ID, resp.ID)
	assert.Equal(t, "organize", resp.Type)
}

func TestStartOrganize_EnqueueError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := dbmocks.NewMockStore(t)
	database.GlobalStore = mockStore
	mockQueue := queuemocks.NewMockQueue(t)
	operations.GlobalQueue = mockQueue

	returnedOp := &database.Operation{ID: "op-org-err", Type: "organize"}
	mockStore.EXPECT().CreateOperation(mock.Anything, "organize", (*string)(nil)).Return(returnedOp, nil).Once()
	mockQueue.EXPECT().Enqueue("op-org-err", "organize", operations.PriorityNormal, mock.Anything).Return(assert.AnError).Once()

	srv := &Server{router: gin.New()}
	srv.setupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/organize", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}
