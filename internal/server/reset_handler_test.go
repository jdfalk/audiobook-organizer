// file: internal/server/reset_handler_test.go
// version: 1.0.0
// guid: 9a0b1c2d-3e4f-5a6b-7c8d-9e0f1a2b3c4d

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// TestResetSystem_Success tests the reset endpoint with successful reset
func TestResetSystem_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	// Mock the Reset function
	if store, ok := database.GlobalStore.(*database.MockStore); ok {
		store.ResetFunc = func() error {
			return nil
		}
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/system/reset", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	server.resetSystem(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Data.Message != "System reset successfully" {
		t.Errorf("expected 'System reset successfully', got %q", resp.Data.Message)
	}
}

// TestResetSystem_Error tests reset when database reset fails
func TestResetSystem_Error(t *testing.T) {
	server := setupHandlerTestServer(t)

	// Mock the Reset function to return an error
	if store, ok := database.GlobalStore.(*database.MockStore); ok {
		store.ResetFunc = func() error {
			return errors.New("test error")
		}
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/system/reset", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	server.resetSystem(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}
