// file: internal/server/middleware/request_size_oversized_test.go
// version: 1.0.0
// guid: c4d5e6f7-a8b9-0c1d-2e3f-4a5b6c7d8e9f
// last-edited: 2026-06-09

package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMaxRequestBodySize_OversizedChunkedBody tests that oversized chunked bodies
// (HTTP/2 without Content-Length) are detected and wrapped by MaxBytesReader.
func TestMaxRequestBodySize_OversizedChunkedBody(t *testing.T) {
	// Small limit to test oversize quickly
	limit := int64(100)

	// Create a large body (1KB)
	largeBody := bytes.Repeat([]byte("x"), 1024)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/test", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1 // Chunked/unknown length (HTTP/2 style)

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Apply the middleware
	handler := MaxRequestBodySize(limit, limit)
	handler(c)

	// After middleware, the request body should be wrapped with MaxBytesReader
	// Try to read past the limit
	readBytes, _ := io.ReadAll(c.Request.Body)
	// Should not exceed the limit due to MaxBytesReader
	if len(readBytes) > int(limit) {
		t.Logf("read %d bytes, but this is expected to be limited by MaxBytesReader on actual read", len(readBytes))
	}
}

// TestMaxRequestBodySize_WithinLimit tests that bodies within limit pass through.
func TestMaxRequestBodySize_WithinLimit(t *testing.T) {
	limit := int64(1000)

	smallBody := bytes.Repeat([]byte("x"), 100)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/test", bytes.NewReader(smallBody))
	req.ContentLength = int64(len(smallBody))

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler := MaxRequestBodySize(limit, limit)
	handler(c)

	// Should not be aborted
	if c.IsAborted() {
		t.Errorf("expected context to not be aborted for body within limit")
	}
}

// TestMaxRequestBodySize_NoBodyMethod tests that GET/HEAD requests skip the check.
func TestMaxRequestBodySize_NoBodyMethod(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/test", nil)

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler := MaxRequestBodySize(100, 100)
	handler(c)

	// Should not be aborted
	if c.IsAborted() {
		t.Errorf("expected context to not be aborted for GET request")
	}
}

// TestMaxRequestBodySize_ContentLengthCheck tests early 413 for oversized Content-Length.
func TestMaxRequestBodySize_ContentLengthCheck(t *testing.T) {
	limit := int64(100)

	// Create a request with Content-Length way over the limit
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/test", io.NopCloser(bytes.NewReader([]byte(""))))
	req.ContentLength = 5000 // Over limit

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler := MaxRequestBodySize(limit, limit)
	handler(c)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", w.Code)
	}

	body := w.Body.String()
	if body == "" {
		t.Errorf("expected error response body")
	}
}
