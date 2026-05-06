// file: internal/operations/registry/bus.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a
// last-edited: 2026-05-06

package registry

import (
	"context"
	"sync"
)

// Event is a single operations event published on the EventHub.
type Event struct {
	Name    string // e.g. "op.created", "op.updated", "op.log", "op.terminal"
	Payload any    // arbitrary JSON-serialisable value
}

// subscriber holds a buffered channel for one SSE client.
type subscriber struct {
	ch chan Event
}

// EventHub is the in-process SSE event bus for the operations system.
// It implements the Bus interface and can fan-out to multiple SSE clients.
//
// EventHub is safe for concurrent use. The zero value is not usable;
// use NewEventHub to construct one.
type EventHub struct {
	mu          sync.RWMutex
	subscribers map[uint64]*subscriber
	nextID      uint64
}

// NewEventHub constructs an EventHub ready for use.
func NewEventHub() *EventHub {
	return &EventHub{
		subscribers: make(map[uint64]*subscriber),
	}
}

// Publish sends an event to every subscriber. Subscribers whose channels
// are full (slow clients) are skipped — the event is dropped rather than
// blocking the publisher. Publish is nil-safe; calling it on a nil *EventHub
// is a no-op.
func (h *EventHub) Publish(_ context.Context, eventName string, payload any) error {
	if h == nil {
		return nil
	}
	ev := Event{Name: eventName, Payload: payload}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sub := range h.subscribers {
		select {
		case sub.ch <- ev:
		default:
			// Subscriber is too slow; drop the event for this client.
		}
	}
	return nil
}

// Subscribe registers a new subscriber and returns a read-only channel and
// an unsubscribe function. The caller MUST call the returned function when
// the SSE connection is closed to avoid leaking goroutines and channels.
//
// The channel is buffered (size 64). If the caller does not drain it quickly
// enough, events are dropped (see Publish).
func (h *EventHub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.nextID
	h.nextID++

	sub := &subscriber{ch: make(chan Event, 64)}
	h.subscribers[id] = sub

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subscribers, id)
		// Drain and close so any blocked range-over-chan in the SSE handler exits.
		for len(sub.ch) > 0 {
			<-sub.ch
		}
		close(sub.ch)
	}
	return sub.ch, unsubscribe
}

// SubscriberCount returns the current number of active subscribers. Useful
// for health-check metrics and tests.
func (h *EventHub) SubscriberCount() int {
	if h == nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}
