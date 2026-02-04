// file: internal/server/logger_test.go
// version: 1.0.0
// guid: 2e3f4a5b-6c7d-8e9f-0a1b-2c3d4e5f6a7b

package server

import (
	"testing"
	"time"
)

func TestNewOperationLogger(t *testing.T) {
	logger := NewOperationLogger("listAudiobooks", "GET", "/api/audiobooks", "req-123")

	if logger.handler != "listAudiobooks" {
		t.Errorf("expected handler 'listAudiobooks', got %q", logger.handler)
	}
	if logger.method != "GET" {
		t.Errorf("expected method 'GET', got %q", logger.method)
	}
	if logger.path != "/api/audiobooks" {
		t.Errorf("expected path '/api/audiobooks', got %q", logger.path)
	}
	if logger.requestID != "req-123" {
		t.Errorf("expected requestID 'req-123', got %q", logger.requestID)
	}
}

func TestOperationLogger_SetResourceID(t *testing.T) {
	logger := NewOperationLogger("getAudiobook", "GET", "/api/audiobooks/:id", "req-123")
	logger.SetResourceID("book-456")

	if logger.resourceID != "book-456" {
		t.Errorf("expected resourceID 'book-456', got %q", logger.resourceID)
	}
}

func TestOperationLogger_AddDetail(t *testing.T) {
	logger := NewOperationLogger("updateAudiobook", "PUT", "/api/audiobooks/:id", "req-123")
	logger.AddDetail("title", "New Title")
	logger.AddDetail("author_id", 42)

	if logger.details["title"] != "New Title" {
		t.Errorf("expected detail 'title' to be 'New Title', got %v", logger.details["title"])
	}
	if logger.details["author_id"] != 42 {
		t.Errorf("expected detail 'author_id' to be 42, got %v", logger.details["author_id"])
	}
}

func TestNewServiceLogger(t *testing.T) {
	logger := NewServiceLogger("AudiobookService", "req-123")

	if logger.serviceName != "AudiobookService" {
		t.Errorf("expected serviceName 'AudiobookService', got %q", logger.serviceName)
	}
	if logger.requestID != "req-123" {
		t.Errorf("expected requestID 'req-123', got %q", logger.requestID)
	}
}

func TestNewRequestLogger(t *testing.T) {
	logger := NewRequestLogger("req-123", "192.168.1.1", "Mozilla/5.0", "GET", "/api/audiobooks")

	if logger.requestID != "req-123" {
		t.Errorf("expected requestID 'req-123', got %q", logger.requestID)
	}
	if logger.clientIP != "192.168.1.1" {
		t.Errorf("expected clientIP '192.168.1.1', got %q", logger.clientIP)
	}
	if logger.method != "GET" {
		t.Errorf("expected method 'GET', got %q", logger.method)
	}
}

func TestRequestLogger_Duration(t *testing.T) {
	logger := NewRequestLogger("req-123", "127.0.0.1", "test-agent", "GET", "/test")

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	duration := time.Since(logger.startTime)
	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}
}

func TestNewLogger(t *testing.T) {
	logger := NewLogger(InfoLevel)

	if logger.minLevel != InfoLevel {
		t.Errorf("expected minLevel InfoLevel, got %v", logger.minLevel)
	}
}

func TestStructuredLog(t *testing.T) {
	now := time.Now()
	log := &StructuredLog{
		Timestamp:  now,
		Level:      "INFO",
		Message:    "test message",
		Handler:    "testHandler",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		Duration:   100 * time.Millisecond,
	}

	if log.Level != "INFO" {
		t.Errorf("expected level 'INFO', got %q", log.Level)
	}
	if log.StatusCode != 200 {
		t.Errorf("expected statusCode 200, got %d", log.StatusCode)
	}
	if log.Duration != 100*time.Millisecond {
		t.Errorf("expected duration 100ms, got %v", log.Duration)
	}
}

func TestOperationLogger_TimingAccuracy(t *testing.T) {
	logger := NewOperationLogger("slowOp", "POST", "/test", "req-123")

	// Simulate operation
	time.Sleep(50 * time.Millisecond)

	elapsed := time.Since(logger.startTime)
	if elapsed < 50*time.Millisecond {
		t.Errorf("expected elapsed >= 50ms, got %v", elapsed)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected elapsed < 100ms, got %v", elapsed)
	}
}
