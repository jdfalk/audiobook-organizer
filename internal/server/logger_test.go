// file: internal/server/logger_test.go
// version: 1.1.1
// guid: 2e3f4a5b-6c7d-8e9f-0a1b-2c3d4e5f6a7b

package server

import (
	"errors"
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

// TestOperationLogger_LogStart tests that LogStart doesn't panic
func TestOperationLogger_LogStart(t *testing.T) {
	logger := NewOperationLogger("testHandler", "GET", "/api/test", "req-123")
	logger.LogStart() // Should not panic

	logger.SetResourceID("res-456")
	logger.LogStart() // Should not panic with resource ID
}

// TestOperationLogger_LogSuccess tests that LogSuccess doesn't panic
func TestOperationLogger_LogSuccess(t *testing.T) {
	logger := NewOperationLogger("testHandler", "GET", "/api/test", "req-123")
	logger.LogSuccess(200) // Should not panic

	logger.SetResourceID("res-456")
	logger.LogSuccess(201) // Should not panic with resource ID
}

// TestOperationLogger_LogError tests that LogError doesn't panic
func TestOperationLogger_LogError(t *testing.T) {
	logger := NewOperationLogger("testHandler", "GET", "/api/test", "req-123")
	logger.LogError(500, errors.New("test error")) // Should not panic

	logger.SetResourceID("res-456")
	logger.LogError(404, errors.New("not found")) // Should not panic with resource ID
}

// TestOperationLogger_LogDebug tests that LogDebug doesn't panic
func TestOperationLogger_LogDebug(t *testing.T) {
	logger := NewOperationLogger("testHandler", "GET", "/api/test", "req-123")
	logger.LogDebug("debug message") // Should not panic
}

// TestOperationLogger_LogWarning tests that LogWarning doesn't panic
func TestOperationLogger_LogWarning(t *testing.T) {
	logger := NewOperationLogger("testHandler", "GET", "/api/test", "req-123")
	logger.LogWarning("warning message") // Should not panic
}

// TestServiceLogger_LogOperation tests that LogOperation doesn't panic
func TestServiceLogger_LogOperation(t *testing.T) {
	logger := NewServiceLogger("TestService", "req-123")
	logger.LogOperation("testOp", nil) // Should not panic without details

	details := map[string]any{"key": "value", "count": 42}
	logger.LogOperation("testOp", details) // Should not panic with details
}

// TestServiceLogger_LogError tests that LogError doesn't panic
func TestServiceLogger_LogError(t *testing.T) {
	logger := NewServiceLogger("TestService", "req-123")
	logger.LogError("testOp", errors.New("test error")) // Should not panic
}

// TestServiceLogger_LogDebug tests that LogDebug doesn't panic
func TestServiceLogger_LogDebug(t *testing.T) {
	logger := NewServiceLogger("TestService", "req-123")
	logger.LogDebug("testOp", "debug message") // Should not panic
}

// TestRequestLogger_LogRequest tests that LogRequest doesn't panic
func TestRequestLogger_LogRequest(t *testing.T) {
	logger := NewRequestLogger("req-123", "192.168.1.1", "Mozilla/5.0", "GET", "/api/test")
	logger.LogRequest() // Should not panic
}

// TestRequestLogger_LogResponse tests that LogResponse doesn't panic
func TestRequestLogger_LogResponse(t *testing.T) {
	logger := NewRequestLogger("req-123", "192.168.1.1", "Mozilla/5.0", "GET", "/api/test")
	logger.LogResponse(200, 1024) // Should not panic
}

// TestLogMetric tests that LogMetric doesn't panic
func TestLogMetric(t *testing.T) {
	LogMetric("test_metric", 123.45, "ms") // Should not panic
}

// TestLogDatabaseOperation tests that LogDatabaseOperation doesn't panic
func TestLogDatabaseOperation(t *testing.T) {
	LogDatabaseOperation("SELECT", "audiobooks", time.Millisecond*10, 5, nil) // Should not panic
	LogDatabaseOperation("INSERT", "audiobooks", time.Millisecond*5, 0, errors.New("test error")) // Should not panic
}

// TestLogServiceCacheHit tests that LogServiceCacheHit doesn't panic
func TestLogServiceCacheHit(t *testing.T) {
	LogServiceCacheHit("TestService", "test-key") // Should not panic
}

// TestLogServiceCacheMiss tests that LogServiceCacheMiss doesn't panic
func TestLogServiceCacheMiss(t *testing.T) {
	LogServiceCacheMiss("TestService", "test-key") // Should not panic
}

// TestLogSlowQuery tests that LogSlowQuery doesn't panic
func TestLogSlowQuery(t *testing.T) {
	LogSlowQuery("SELECT * FROM audiobooks", time.Millisecond*10, time.Second) // Below threshold
	LogSlowQuery("SELECT * FROM audiobooks", time.Second*2, time.Second) // Above threshold
}

// TestLogValidationError tests that LogValidationError doesn't panic
func TestLogValidationError(t *testing.T) {
	LogValidationError("TestHandler", "email", "invalid format", "req-123") // Should not panic
}

// TestLogAuthorizationFailure tests that LogAuthorizationFailure doesn't panic
func TestLogAuthorizationFailure(t *testing.T) {
	LogAuthorizationFailure("user-123", "audiobook-456", "delete", "req-789") // Should not panic
}

// TestLogAuditEvent tests that LogAuditEvent doesn't panic
func TestLogAuditEvent(t *testing.T) {
	LogAuditEvent("DELETE", "user-123", "audiobook-456", "delete", "test details") // Should not panic
}
