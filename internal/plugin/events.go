// file: internal/plugin/events.go
// version: 1.0.0

package plugin

import (
	"context"
	"log"
	"sync"
	"time"
)

// EventType identifies a lifecycle event.
type EventType string

const (
	EventBookImported      EventType = "book.imported"
	EventBookDeleted       EventType = "book.deleted"
	EventMetadataApplied   EventType = "metadata.applied"
	EventTagsWritten       EventType = "tags.written"
	EventFileOrganized     EventType = "file.organized"
	EventDedupDetected     EventType = "dedup.detected"
	EventDedupMerged       EventType = "dedup.merged"
	EventCoverChanged      EventType = "cover.changed"
	EventReadStatusChanged EventType = "read_status.changed"
	EventScanCompleted     EventType = "scan.completed"
)

// Event is a JSON-serializable lifecycle event.
type Event struct {
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	BookID    string         `json:"book_id,omitempty"`
	UserID    string         `json:"user_id,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// NewEvent creates an event with the current timestamp.
func NewEvent(eventType EventType, bookID string, data map[string]any) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now(),
		BookID:    bookID,
		Data:      data,
	}
}

// EventHandler processes a single event.
type EventHandler func(ctx context.Context, event Event) error

// EventBus manages event subscriptions and publishing.
type EventBus struct {
	subscribers map[EventType][]EventHandler
	mu          sync.RWMutex
}

// NewEventBus creates an empty event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[EventType][]EventHandler),
	}
}

// Subscribe registers a handler for an event type.
func (b *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
}

// Publish sends an event to all subscribers. Handlers run in goroutines.
// Errors are logged but do not propagate to the publisher.
func (b *EventBus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	handlers := b.subscribers[event.Type]
	b.mu.RUnlock()

	for _, handler := range handlers {
		h := handler // capture
		go func() {
			if err := h(ctx, event); err != nil {
				log.Printf("[WARN] plugin event handler error for %s: %v", event.Type, err)
			}
		}()
	}
}

// SubscriberCount returns how many handlers are registered for an event type.
func (b *EventBus) SubscriberCount(eventType EventType) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[eventType])
}
