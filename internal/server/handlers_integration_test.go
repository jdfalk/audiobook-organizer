// file: internal/server/handlers_integration_test.go
// version: 1.0.0
// guid: 3f4a5b6c-7d8e-9f0a-1b2c-3d4e5f6a7b8c

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// setupHandlerTestServer creates a test server with mock database
func setupHandlerTestServer(t *testing.T) *Server {
	// Create a test database instance
	authorID := 1
	mockDB := &database.MockStore{
		GetAllWorksFunc: func() ([]database.Work, error) {
			return []database.Work{
				{ID: "1", Title: "Work 1"},
				{ID: "2", Title: "Work 2"},
			}, nil
		},
		GetAllAuthorsFunc: func() ([]database.Author, error) {
			return []database.Author{
				{ID: 1, Name: "Author 1"},
				{ID: 2, Name: "Author 2"},
			}, nil
		},
		GetAllSeriesFunc: func() ([]database.Series, error) {
			return []database.Series{
				{ID: 1, Name: "Series 1", AuthorID: &authorID},
			}, nil
		},
	}

	// Temporarily set the global store for this test
	oldStore := database.GlobalStore
	database.GlobalStore = mockDB
	t.Cleanup(func() {
		database.GlobalStore = oldStore
	})

	return NewServer()
}

// TestListWorks_Success tests the listWorks handler with successful response
func TestListWorks_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/works", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	server.listWorks(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp WorkListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Count != 2 {
		t.Errorf("expected count 2, got %d", resp.Count)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
}

// TestCreateWork_Success tests the createWork handler with valid input
func TestCreateWork_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	work := database.Work{Title: "New Work"}
	body, _ := json.Marshal(work)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/works", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Mock the CreateWork function
	if store, ok := database.GlobalStore.(*database.MockStore); ok {
		store.CreateWorkFunc = func(w *database.Work) (*database.Work, error) {
			w.ID = "new-123"
			return w, nil
		}
	}

	server.createWork(c)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}

// TestCreateWork_InvalidJSON tests the createWork handler with invalid JSON
func TestCreateWork_InvalidJSON(t *testing.T) {
	server := setupHandlerTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/works", bytes.NewReader([]byte("{invalid")))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	server.createWork(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestGetWork_Success tests the getWork handler with existing work
func TestGetWork_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/works/1", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "1"})

	// Mock the GetWorkByID function
	if store, ok := database.GlobalStore.(*database.MockStore); ok {
		store.GetWorkByIDFunc = func(id string) (*database.Work, error) {
			return &database.Work{ID: id, Title: "Work 1"}, nil
		}
	}

	server.getWork(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestGetWork_NotFound tests the getWork handler with non-existent work
func TestGetWork_NotFound(t *testing.T) {
	server := setupHandlerTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/works/nonexistent", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "nonexistent"})

	// Mock the GetWorkByID function to return nil
	if store, ok := database.GlobalStore.(*database.MockStore); ok {
		store.GetWorkByIDFunc = func(id string) (*database.Work, error) {
			return nil, nil
		}
	}

	server.getWork(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// TestListAuthors_Success tests the listAuthors handler
func TestListAuthors_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/authors", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	server.listAuthors(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp AuthorListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Count != 2 {
		t.Errorf("expected count 2, got %d", resp.Count)
	}
}

// TestListSeries_Success tests the listSeries handler
func TestListSeries_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/series", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	server.listSeries(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp SeriesListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Count)
	}
}

// TestBrowseFilesystem_NoPath tests the browseFilesystem handler without path
func TestBrowseFilesystem_NoPath(t *testing.T) {
	server := setupHandlerTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/browse?path=", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	server.browseFilesystem(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestBatchUpdateAudiobooks_Empty tests batch update with no IDs
func TestBatchUpdateAudiobooks_Empty(t *testing.T) {
	server := setupHandlerTestServer(t)

	req := &BatchUpdateRequest{
		IDs:     []string{},
		Updates: map[string]interface{}{},
	}
	body, _ := json.Marshal(req)

	w := httptest.NewRecorder()
	httpReq := httptest.NewRequest("POST", "/api/audiobooks/batch", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = httpReq

	server.batchUpdateAudiobooks(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp BatchUpdateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
}

// TestDeleteWork_Success tests the deleteWork handler
func TestDeleteWork_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/works/1", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "1"})

	// Mock the DeleteWork function
	if store, ok := database.GlobalStore.(*database.MockStore); ok {
		store.DeleteWorkFunc = func(id string) error {
			return nil
		}
	}

	server.deleteWork(c)

	// c.Status() may return 200 if no response body is written, which is OK
	// The handler sets status correctly via c.Status()
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Errorf("expected status 204 or 200, got %d", w.Code)
	}
}

// TestUpdateWork_InvalidTitle tests updateWork with empty title
func TestUpdateWork_InvalidTitle(t *testing.T) {
	server := setupHandlerTestServer(t)

	work := database.Work{Title: ""}
	body, _ := json.Marshal(work)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/works/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "1"})

	server.updateWork(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
