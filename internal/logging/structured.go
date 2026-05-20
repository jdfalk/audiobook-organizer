// file: internal/logging/structured.go
// version: 1.0.0

package logging

import (
	"context"
	"log/slog"
	"encoding/json"
)

// opAttrs builds a list of slog attributes from an OpContext.
// Returns empty list if op is nil.
func opAttrs(op *OpContext) []any {
	if op == nil {
		return []any{}
	}
	attrs := []any{
		"opID", op.ID,
		"opType", op.Type,
		"opStatus", op.Status,
	}
	if len(op.Entities) > 0 {
		entitiesJSON, _ := json.Marshal(op.Entities)
		attrs = append(attrs, "entities", string(entitiesJSON))
	}
	return attrs
}

// Info logs a message with operation context from ctx automatically included.
// Additional attrs are appended after operation attributes.
func Info(ctx context.Context, msg string, attrs ...any) {
	op := OpFromContext(ctx)
	args := append(opAttrs(op), attrs...)
	slog.Info(msg, args...)
}

// Warn logs a warning with operation context from ctx automatically included.
func Warn(ctx context.Context, msg string, attrs ...any) {
	op := OpFromContext(ctx)
	args := append(opAttrs(op), attrs...)
	slog.Warn(msg, args...)
}

// Error logs an error with operation context from ctx automatically included.
func Error(ctx context.Context, msg string, attrs ...any) {
	op := OpFromContext(ctx)
	args := append(opAttrs(op), attrs...)
	slog.Error(msg, args...)
}

// Debug logs a debug message with operation context from ctx automatically included.
func Debug(ctx context.Context, msg string, attrs ...any) {
	op := OpFromContext(ctx)
	args := append(opAttrs(op), attrs...)
	slog.Debug(msg, args...)
}
