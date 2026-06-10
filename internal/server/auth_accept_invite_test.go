// file: internal/server/auth_accept_invite_test.go
// version: 1.0.0
// guid: b3c4d5e6-f7a8-9b0c-1d2e-3f4a5b6c7d8e
// last-edited: 2026-06-09

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestHandleAcceptInvite_EmptyBody tests that an empty POST body returns 400 with a clear message.
func TestHandleAcceptInvite_EmptyBody(t *testing.T) {
	srv := &Server{router: gin.New()}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/accept-invite", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	srv.handleAcceptInvite(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "EOF") {
		t.Errorf("response body must not contain raw 'EOF' string: %s", body)
	}
	if !strings.Contains(body, "request body required") {
		t.Errorf("response body should contain 'request body required': %s", body)
	}
}

// TestHandleAcceptInvite_MissingContentLength tests HTTP/2 streaming bodies (no Content-Length).
func TestHandleAcceptInvite_NoContentLength(t *testing.T) {
	srv := &Server{router: gin.New()}

	// Create a request with no Content-Length (simulates HTTP/2 streaming)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/accept-invite", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1 // Indicates unknown/chunked

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	srv.handleAcceptInvite(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "EOF") {
		t.Errorf("response body must not contain raw 'EOF' string: %s", body)
	}
}

// TestHandleAcceptInvite_MissingFields tests that missing required fields return a clear error.
func TestHandleAcceptInvite_MissingFields(t *testing.T) {
	srv := &Server{router: gin.New()}

	reqBody := map[string]string{"token": ""}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/accept-invite", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	srv.handleAcceptInvite(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "EOF") {
		t.Errorf("response body must not contain raw 'EOF' string: %s", body)
	}
}
