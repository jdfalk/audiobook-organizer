// file: internal/server/handlers/interfaces.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1234-5678-90abcdef0123
// last-edited: 2026-06-02

package handlers

import (
	"context"

	"github.com/falkcorp/audiobook-organizer/internal/plugin"
)

// EventPublisher is the narrow interface for publishing domain events to the plugin bus.
// Used by handlers that trigger side effects visible to plugins.
type EventPublisher interface {
	Publish(ctx context.Context, event plugin.Event)
}

// WriteBackEnqueuer is the narrow interface for enqueuing book write-back jobs.
// Used by handlers that trigger tag-writing after metadata changes.
type WriteBackEnqueuer interface {
	Enqueue(bookID string)
}
