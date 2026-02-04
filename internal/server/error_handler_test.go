// file: internal/server/error_handler_test.go
// version: 1.0.0
// guid: 6e7f8a9b-0c1d-2e3f-4a5b-6c7d8e9f0a1b

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRespondWithBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	RespondWithBadRequest(c, "test error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	if !contains(w.Body.String(), "test error") {
		t.Errorf("expected error message in response, got %q", w.Body.String())
	}
}

func TestRespondWithNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	RespondWithNotFound(c, "book", "123")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	if !contains(w.Body.String(), "not found") {
		t.Errorf("expected 'not found' in response, got %q", w.Body.String())
	}
}

func TestRespondWithInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	RespondWithInternalError(c, "database error")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestRespondWithCreated(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	data := map[string]string{"id": "123"}
	RespondWithCreated(c, data)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}

func TestParseQueryInt(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?limit=25", nil)

	value := ParseQueryInt(c, "limit", 50)
	if value != 25 {
		t.Errorf("expected 25, got %d", value)
	}

	value = ParseQueryInt(c, "offset", 0)
	if value != 0 {
		t.Errorf("expected 0, got %d", value)
	}
}

func TestParseQueryIntPtr(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?author_id=42", nil)

	value := ParseQueryIntPtr(c, "author_id")
	if value == nil || *value != 42 {
		t.Errorf("expected pointer to 42, got %v", value)
	}

	value = ParseQueryIntPtr(c, "missing")
	if value != nil {
		t.Errorf("expected nil for missing key, got %v", value)
	}
}

func TestParseQueryBool(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?flag=true&other=1", nil)

	value := ParseQueryBool(c, "flag", false)
	if !value {
		t.Errorf("expected true, got false")
	}

	value = ParseQueryBool(c, "other", false)
	if !value {
		t.Errorf("expected true for '1', got false")
	}

	value = ParseQueryBool(c, "missing", true)
	if !value {
		t.Errorf("expected default true, got false")
	}
}

func TestParseQueryString(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?search=test", nil)

	value := ParseQueryString(c, "search")
	if value != "test" {
		t.Errorf("expected 'test', got %q", value)
	}

	value = ParseQueryString(c, "missing")
	if value != "" {
		t.Errorf("expected empty string, got %q", value)
	}
}

// Helper function to check if substring exists
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
