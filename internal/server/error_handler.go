// file: internal/server/error_handler.go
// version: 1.1.0
// guid: 5d6e7f8a-9b0c-1d2e-3f4a-5b6c7d8e9f0a
// last-edited: 2026-02-04

package server

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ErrorResponse provides a consistent error response format
type ErrorResponse struct {
	Error  string `json:"error"`
	Code   string `json:"code,omitempty"`
	Status int    `json:"status"`
}

// SuccessResponse provides a consistent success response format
type SuccessResponse struct {
	Data   any    `json:"data,omitempty"`
	Count  int    `json:"count,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// RespondWithError sends a standardized error response and logs the error
func RespondWithError(c *gin.Context, statusCode int, message string, code string) {
	// Log the error with context
	logErrorWithContext(c, statusCode, message)

	c.JSON(statusCode, ErrorResponse{
		Error:  message,
		Code:   code,
		Status: statusCode,
	})
}

// RespondWithBadRequest sends a 400 Bad Request error response
func RespondWithBadRequest(c *gin.Context, message string) {
	RespondWithError(c, http.StatusBadRequest, message, "BAD_REQUEST")
}

// RespondWithValidationError sends a 400 error for validation failures
func RespondWithValidationError(c *gin.Context, field string, reason string) {
	message := "validation error: " + field
	if reason != "" {
		message = message + " (" + reason + ")"
	}
	RespondWithError(c, http.StatusBadRequest, message, "VALIDATION_ERROR")
}

// RespondWithNotFound sends a 404 Not Found error response
func RespondWithNotFound(c *gin.Context, resourceType string, id string) {
	message := resourceType + " not found"
	if id != "" {
		message = message + ": " + id
	}
	RespondWithError(c, http.StatusNotFound, message, "NOT_FOUND")
}

// RespondWithInternalError sends a 500 Internal Server Error response
func RespondWithInternalError(c *gin.Context, message string) {
	RespondWithError(c, http.StatusInternalServerError, message, "INTERNAL_ERROR")
}

// RespondWithConflict sends a 409 Conflict error response
func RespondWithConflict(c *gin.Context, message string) {
	RespondWithError(c, http.StatusConflict, message, "CONFLICT")
}

// RespondWithUnauthorized sends a 401 Unauthorized error response
func RespondWithUnauthorized(c *gin.Context, message string) {
	RespondWithError(c, http.StatusUnauthorized, message, "UNAUTHORIZED")
}

// RespondWithForbidden sends a 403 Forbidden error response
func RespondWithForbidden(c *gin.Context, message string) {
	RespondWithError(c, http.StatusForbidden, message, "FORBIDDEN")
}

// RespondWithSuccess sends a successful response with data
func RespondWithSuccess(c *gin.Context, statusCode int, data any) {
	c.JSON(statusCode, SuccessResponse{
		Data: data,
	})
}

// RespondWithList sends a successful list response with pagination info
func RespondWithList(c *gin.Context, items any, count int, limit int, offset int) {
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"count":  count,
		"limit":  limit,
		"offset": offset,
	})
}

// RespondWithCreated sends a 201 Created response
func RespondWithCreated(c *gin.Context, data any) {
	RespondWithSuccess(c, http.StatusCreated, data)
}

// RespondWithOK sends a 200 OK response
func RespondWithOK(c *gin.Context, data any) {
	RespondWithSuccess(c, http.StatusOK, data)
}

// RespondWithNoContent sends a 204 No Content response
func RespondWithNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// logErrorWithContext logs an error with request context for debugging
func logErrorWithContext(c *gin.Context, statusCode int, message string) {
	method := c.Request.Method
	path := c.Request.URL.Path
	clientIP := c.ClientIP()

	logLevel := "WARNING"
	if statusCode >= 500 {
		logLevel = "ERROR"
	}

	log.Printf("[%s] %s %s %d - %s (from %s)", logLevel, method, path, statusCode, message, clientIP)
}

// HandleBindError handles JSON binding errors with a consistent response
func HandleBindError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	// Try to extract field-specific error information
	errMsg := err.Error()
	if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "binding") {
		RespondWithValidationError(c, "request body", errMsg)
	} else {
		RespondWithBadRequest(c, "invalid request: "+errMsg)
	}
	return true
}

// ParseQueryInt parses an integer query parameter with a default value
func ParseQueryInt(c *gin.Context, key string, defaultValue int) int {
	valueStr := c.DefaultQuery(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

// ParseQueryIntPtr parses an optional integer query parameter, returning nil if not present or invalid
func ParseQueryIntPtr(c *gin.Context, key string) *int {
	valueStr := c.Query(key)
	if valueStr == "" {
		return nil
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return nil
	}
	return &value
}

// ParseQueryBool parses a boolean query parameter with a default value
func ParseQueryBool(c *gin.Context, key string, defaultValue bool) bool {
	valueStr := c.DefaultQuery(key, "")
	if valueStr == "" {
		return defaultValue
	}
	return strings.ToLower(valueStr) == "true" || valueStr == "1"
}

// ParseQueryString returns a query parameter as a string, or empty string if not present
func ParseQueryString(c *gin.Context, key string) string {
	return c.Query(key)
}

// ParsePaginationParams parses common pagination parameters from query string
func ParsePaginationParams(c *gin.Context) PaginationParams {
	limit := ParseQueryInt(c, "limit", 50)
	offset := ParseQueryInt(c, "offset", 0)
	search := ParseQueryString(c, "search")

	// Validate pagination parameters
	if limit < 1 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	return PaginationParams{
		Limit:  limit,
		Offset: offset,
		Search: search,
	}
}

// EnsureNotNil converts nil slices to empty slices to avoid null JSON marshalling
func EnsureNotNil(slice any) any {
	// Handle nil case - return empty slice equivalent
	if slice == nil {
		return []any{}
	}
	return slice
}
