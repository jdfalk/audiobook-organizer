// file: internal/server/logger.go
// version: 1.0.0
// guid: 1d2e3f4a-5b6c-7d8e-9f0a-1b2c3d4e5f6a

package server

import (
	"fmt"
	"log"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// Logger provides structured logging for handlers and services
type Logger struct {
	minLevel LogLevel
}

// NewLogger creates a new logger instance
func NewLogger(minLevel LogLevel) *Logger {
	return &Logger{minLevel: minLevel}
}

// StructuredLog represents a structured log entry with contextual data
type StructuredLog struct {
	Timestamp   time.Time
	Level       string
	Message     string
	Handler     string
	Method      string
	Path        string
	StatusCode  int
	Duration    time.Duration
	Error       string
	UserID      string
	RequestID   string
	ResourceID  string
	Details     map[string]any
}

// OperationLogger tracks the lifecycle of a handler operation
type OperationLogger struct {
	handler    string
	method     string
	path       string
	startTime  time.Time
	requestID  string
	resourceID string
	details    map[string]any
}

// NewOperationLogger creates a new operation logger
func NewOperationLogger(handler, method, path, requestID string) *OperationLogger {
	return &OperationLogger{
		handler:   handler,
		method:    method,
		path:      path,
		startTime: time.Now(),
		requestID: requestID,
		details:   make(map[string]any),
	}
}

// SetResourceID sets the resource ID being operated on
func (ol *OperationLogger) SetResourceID(id string) {
	ol.resourceID = id
}

// AddDetail adds a contextual detail to the operation log
func (ol *OperationLogger) AddDetail(key string, value any) {
	ol.details[key] = value
}

// LogStart logs the start of the operation
func (ol *OperationLogger) LogStart() {
	msg := fmt.Sprintf("[START] %s %s", ol.method, ol.path)
	if ol.resourceID != "" {
		msg = fmt.Sprintf("%s (resource: %s)", msg, ol.resourceID)
	}
	log.Printf("[INFO] %s [request-id: %s]", msg, ol.requestID)
}

// LogSuccess logs the successful completion of the operation
func (ol *OperationLogger) LogSuccess(statusCode int) {
	duration := time.Since(ol.startTime)
	msg := fmt.Sprintf("[SUCCESS] %s %s (%d) in %v",
		ol.method, ol.path, statusCode, duration)
	if ol.resourceID != "" {
		msg = fmt.Sprintf("%s (resource: %s)", msg, ol.resourceID)
	}
	log.Printf("[INFO] %s [request-id: %s]", msg, ol.requestID)
}

// LogError logs an error that occurred during the operation
func (ol *OperationLogger) LogError(statusCode int, err error) {
	duration := time.Since(ol.startTime)
	msg := fmt.Sprintf("[ERROR] %s %s (%d) in %v: %v",
		ol.method, ol.path, statusCode, duration, err)
	if ol.resourceID != "" {
		msg = fmt.Sprintf("%s (resource: %s)", msg, ol.resourceID)
	}
	log.Printf("[ERROR] %s [request-id: %s]", msg, ol.requestID)
}

// LogDebug logs a debug message
func (ol *OperationLogger) LogDebug(message string) {
	log.Printf("[DEBUG] %s: %s [request-id: %s]", ol.handler, message, ol.requestID)
}

// LogWarning logs a warning message
func (ol *OperationLogger) LogWarning(message string) {
	log.Printf("[WARN] %s: %s [request-id: %s]", ol.handler, message, ol.requestID)
}

// ServiceLogger provides logging for service layer operations
type ServiceLogger struct {
	serviceName string
	requestID   string
}

// NewServiceLogger creates a new service logger
func NewServiceLogger(serviceName, requestID string) *ServiceLogger {
	return &ServiceLogger{
		serviceName: serviceName,
		requestID:   requestID,
	}
}

// LogOperation logs the execution of a service operation
func (sl *ServiceLogger) LogOperation(operation string, details map[string]any) {
	detailStr := ""
	if len(details) > 0 {
		detailStr = fmt.Sprintf(" %v", details)
	}
	log.Printf("[SERVICE] %s.%s%s [request-id: %s]",
		sl.serviceName, operation, detailStr, sl.requestID)
}

// LogError logs an error from the service
func (sl *ServiceLogger) LogError(operation string, err error) {
	log.Printf("[SERVICE-ERROR] %s.%s: %v [request-id: %s]",
		sl.serviceName, operation, err, sl.requestID)
}

// LogDebug logs a debug message from the service
func (sl *ServiceLogger) LogDebug(operation string, message string) {
	log.Printf("[SERVICE-DEBUG] %s.%s: %s [request-id: %s]",
		sl.serviceName, operation, message, sl.requestID)
}

// RequestLogger provides request-level logging
type RequestLogger struct {
	requestID  string
	clientIP   string
	userAgent  string
	method     string
	path       string
	startTime  time.Time
}

// NewRequestLogger creates a new request logger
func NewRequestLogger(requestID, clientIP, userAgent, method, path string) *RequestLogger {
	return &RequestLogger{
		requestID: requestID,
		clientIP:  clientIP,
		userAgent: userAgent,
		method:    method,
		path:      path,
		startTime: time.Now(),
	}
}

// LogRequest logs the received request
func (rl *RequestLogger) LogRequest() {
	log.Printf("[REQUEST] %s %s from %s [request-id: %s] [agent: %s]",
		rl.method, rl.path, rl.clientIP, rl.requestID, rl.userAgent)
}

// LogResponse logs the response sent
func (rl *RequestLogger) LogResponse(statusCode int, responseSize int) {
	duration := time.Since(rl.startTime)
	log.Printf("[RESPONSE] %s %s -> %d (%d bytes) in %v [request-id: %s]",
		rl.method, rl.path, statusCode, responseSize, duration, rl.requestID)
}

// LogMetric logs a performance metric
func LogMetric(name string, value float64, unit string) {
	log.Printf("[METRIC] %s: %.2f %s", name, value, unit)
}

// LogDatabaseOperation logs a database operation with its performance
func LogDatabaseOperation(operation string, table string, duration time.Duration, rowsAffected int, err error) {
	if err != nil {
		log.Printf("[DB-ERROR] %s on %s failed in %v: %v", operation, table, duration, err)
		return
	}
	log.Printf("[DB] %s on %s completed in %v (%d rows)", operation, table, duration, rowsAffected)
}

// LogServiceCacheHit logs a service cache hit
func LogServiceCacheHit(serviceName string, key string) {
	log.Printf("[CACHE-HIT] %s: %s", serviceName, key)
}

// LogServiceCacheMiss logs a service cache miss
func LogServiceCacheMiss(serviceName string, key string) {
	log.Printf("[CACHE-MISS] %s: %s", serviceName, key)
}

// LogSlowQuery logs when a database query takes too long
func LogSlowQuery(query string, duration time.Duration, threshold time.Duration) {
	if duration > threshold {
		log.Printf("[SLOW-QUERY] Query took %v (threshold: %v): %s",
			duration, threshold, query)
	}
}

// LogValidationError logs a validation error with context
func LogValidationError(handler string, field string, reason string, requestID string) {
	log.Printf("[VALIDATION-ERROR] %s field %q: %s [request-id: %s]",
		handler, field, reason, requestID)
}

// LogAuthorizationFailure logs when authorization fails
func LogAuthorizationFailure(userID string, resource string, action string, requestID string) {
	log.Printf("[AUTH-FAILURE] User %s attempted %s on %s [request-id: %s]",
		userID, action, resource, requestID)
}

// LogAuditEvent logs an important audit event
func LogAuditEvent(eventType string, userID string, resourceID string, action string, details string) {
	log.Printf("[AUDIT] %s by user %s on %s: %s - %s",
		eventType, userID, resourceID, action, details)
}
