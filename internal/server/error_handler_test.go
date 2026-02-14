// file: internal/server/error_handler_test.go
// version: 1.2.1
// guid: 6e7f8a9b-0c1d-2e3f-4a5b-6c7d8e9f0a1b
// last-edited: 2026-02-13

package server

import (
	"fmt"
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

func TestParsePaginationParams(t *testing.T) {
	tests := []struct {
		name       string
		queryStr   string
		wantLimit  int
		wantOffset int
		wantSearch string
	}{
		{
			name:       "default values",
			queryStr:   "/",
			wantLimit:  50,
			wantOffset: 0,
			wantSearch: "",
		},
		{
			name:       "custom limit and offset",
			queryStr:   "/?limit=100&offset=20",
			wantLimit:  100,
			wantOffset: 20,
			wantSearch: "",
		},
		{
			name:       "with search term",
			queryStr:   "/?limit=25&offset=0&search=test",
			wantLimit:  25,
			wantOffset: 0,
			wantSearch: "test",
		},
		{
			name:       "limit exceeds max",
			queryStr:   "/?limit=2000",
			wantLimit:  1000,
			wantOffset: 0,
			wantSearch: "",
		},
		{
			name:       "negative offset",
			queryStr:   "/?offset=-5",
			wantLimit:  50,
			wantOffset: 0,
			wantSearch: "",
		},
		{
			name:       "zero limit",
			queryStr:   "/?limit=0",
			wantLimit:  50,
			wantOffset: 0,
			wantSearch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", tt.queryStr, nil)

			params := ParsePaginationParams(c)

			if params.Limit != tt.wantLimit {
				t.Errorf("limit: got %d, want %d", params.Limit, tt.wantLimit)
			}
			if params.Offset != tt.wantOffset {
				t.Errorf("offset: got %d, want %d", params.Offset, tt.wantOffset)
			}
			if params.Search != tt.wantSearch {
				t.Errorf("search: got %q, want %q", params.Search, tt.wantSearch)
			}
		})
	}
}

func TestRespondWithValidationError(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		reason string
		want   string
	}{
		{"with reason", "email", "invalid format", "validation error: email (invalid format)"},
		{"without reason", "name", "", "validation error: name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			RespondWithValidationError(c, tt.field, tt.reason)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
			if !contains(w.Body.String(), tt.want) {
				t.Errorf("expected %q in body, got %q", tt.want, w.Body.String())
			}
		})
	}
}

func TestRespondWithConflict(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	RespondWithConflict(c, "duplicate entry")
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
	if !contains(w.Body.String(), "CONFLICT") {
		t.Errorf("expected CONFLICT code in body, got %q", w.Body.String())
	}
}

func TestRespondWithUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	RespondWithUnauthorized(c, "invalid token")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if !contains(w.Body.String(), "UNAUTHORIZED") {
		t.Errorf("expected UNAUTHORIZED code in body, got %q", w.Body.String())
	}
}

func TestRespondWithForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	RespondWithForbidden(c, "access denied")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !contains(w.Body.String(), "FORBIDDEN") {
		t.Errorf("expected FORBIDDEN code in body, got %q", w.Body.String())
	}
}

func TestRespondWithSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	RespondWithSuccess(c, http.StatusOK, map[string]string{"key": "value"})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !contains(w.Body.String(), "value") {
		t.Errorf("expected 'value' in body, got %q", w.Body.String())
	}
}

func TestRespondWithList(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	items := []string{"a", "b"}
	RespondWithList(c, items, 2, 50, 0)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "\"count\":2") {
		t.Errorf("expected count:2 in body, got %q", body)
	}
	if !contains(body, "\"limit\":50") {
		t.Errorf("expected limit:50 in body, got %q", body)
	}
}

func TestRespondWithOK(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	RespondWithOK(c, "hello")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRespondWithNoContent(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/", nil)
	RespondWithNoContent(c)
	// gin.Context.Status writes the header; verify via Writer.Status()
	if c.Writer.Status() != http.StatusNoContent {
		t.Errorf("expected 204, got %d", c.Writer.Status())
	}
}

func TestHandleBindError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantBool bool
		wantCode int
	}{
		{"nil error returns false", nil, false, 200},
		{"required error", fmt.Errorf("field required"), true, 400},
		{"binding error", fmt.Errorf("binding failed"), true, 400},
		{"generic error", fmt.Errorf("bad json"), true, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", nil)
			got := HandleBindError(c, tt.err)
			if got != tt.wantBool {
				t.Errorf("expected %v, got %v", tt.wantBool, got)
			}
			if tt.wantBool && w.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, w.Code)
			}
		})
	}
}

func TestRespondWithError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)
	RespondWithError(c, http.StatusTeapot, "I'm a teapot", "TEAPOT")
	if w.Code != http.StatusTeapot {
		t.Errorf("expected 418, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "TEAPOT") {
		t.Errorf("expected TEAPOT in body, got %q", body)
	}
	if !contains(body, "I'm a teapot") {
		t.Errorf("expected error message in body, got %q", body)
	}
}

func TestEnsureNotNil(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantNil  bool
		wantType string
	}{
		{
			name:     "nil slice",
			input:    nil,
			wantNil:  false,
			wantType: "[]interface {}",
		},
		{
			name:     "non-nil slice",
			input:    []string{"a", "b"},
			wantNil:  false,
			wantType: "[]string",
		},
		{
			name:     "empty slice",
			input:    []int{},
			wantNil:  false,
			wantType: "[]int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureNotNil(tt.input)

			if result == nil && !tt.wantNil {
				t.Errorf("got nil, want non-nil")
			}

			if result != nil && tt.wantNil {
				t.Errorf("got non-nil, want nil")
			}
		})
	}
}
