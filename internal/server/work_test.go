// file: internal/server/work_test.go
// version: 1.0.0
// guid: 6e3b2a5d-1c4f-4f7a-92e1-2b3c4d5e6f7a

package server

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestWorkCRUD(t *testing.T) {
    server, cleanup := setupTestServer(t)
    defer cleanup()

    // Create Work
    createPayload := map[string]any{
        "title": "The Hobbit",
    }
    body, _ := json.Marshal(createPayload)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/works", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

    var created database.Work
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
    require.NotEmpty(t, created.ID)
    assert.Equal(t, "The Hobbit", created.Title)

    // List Works
    req = httptest.NewRequest(http.MethodGet, "/api/v1/works", nil)
    w = httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    require.Equal(t, http.StatusOK, w.Code)
    var listResp map[string]any
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
    assert.Equal(t, float64(1), listResp["count"]) // JSON numbers default to float64

    // Get Work by ID
    req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/works/%s", created.ID), nil)
    w = httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    require.Equal(t, http.StatusOK, w.Code)

    var fetched database.Work
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fetched))
    assert.Equal(t, created.ID, fetched.ID)
    assert.Equal(t, "The Hobbit", fetched.Title)

    // Update Work title
    updatePayload := map[string]any{
        "title": "The Hobbit (Revised)",
    }
    body, _ = json.Marshal(updatePayload)
    req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/works/%s", created.ID), bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w = httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    require.Equal(t, http.StatusOK, w.Code)

    var updated database.Work
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
    assert.Equal(t, "The Hobbit (Revised)", updated.Title)

    // Add a book referencing this Work, then list books by Work
    // Create directly via store to keep test focused on Work endpoints
    b := &database.Book{
        Title:    "The Hobbit (Unabridged)",
        FilePath: "/tmp/hobbit.m4b",
        Format:   "m4b",
    }
    wid := created.ID
    b.WorkID = &wid
    createdBook, err := database.GlobalStore.CreateBook(b)
    require.NoError(t, err)
    require.NotNil(t, createdBook)

    req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/works/%s/books", created.ID), nil)
    w = httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    require.Equal(t, http.StatusOK, w.Code)

    var booksResp map[string]any
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &booksResp))
    assert.Equal(t, float64(1), booksResp["count"]) // one book linked

    // Delete Work
    req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/works/%s", created.ID), nil)
    w = httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    require.Equal(t, http.StatusNoContent, w.Code)
}
