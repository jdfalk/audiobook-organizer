// file: internal/realtime/events.go
// version: 1.1.1
// guid: 9e8d7f6a-5c4b-3a21-0f9e-8d7c6b5a4392

package realtime

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// EventType defines the type of real-time event
type EventType string

const (
	EventOperationProgress EventType = "operation.progress"
	EventOperationStatus   EventType = "operation.status"
	EventOperationLog      EventType = "operation.log"
	EventSystemStatus      EventType = "system.status"
)

// Event represents a real-time event to send to clients
type Event struct {
	Type      EventType              `json:"type"`
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// Client represents a connected SSE client
type Client struct {
	ID         string
	Channel    chan *Event
	Operations map[string]bool // Operations this client is interested in
	mu         sync.RWMutex
}

// NewClient creates a new SSE client
func NewClient(id string) *Client {
	return &Client{
		ID:         id,
		Channel:    make(chan *Event, 100),
		Operations: make(map[string]bool),
	}
}

// Subscribe subscribes the client to an operation
func (c *Client) Subscribe(operationID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Operations[operationID] = true
	log.Printf("Client %s subscribed to operation %s", c.ID, operationID)
}

// Unsubscribe unsubscribes the client from an operation
func (c *Client) Unsubscribe(operationID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Operations, operationID)
	log.Printf("Client %s unsubscribed from operation %s", c.ID, operationID)
}

// IsSubscribed checks if client is subscribed to an operation
func (c *Client) IsSubscribed(operationID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Operations[operationID]
}

// EventHub manages SSE connections and event distribution
type EventHub struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// NewEventHub creates a new event hub
func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[string]*Client),
	}
}

// RegisterClient registers a new client
func (h *EventHub) RegisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client.ID] = client
	log.Printf("Client %s registered, total clients: %d", client.ID, len(h.clients))
}

// UnregisterClient removes a client
func (h *EventHub) UnregisterClient(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if client, exists := h.clients[clientID]; exists {
		close(client.Channel)
		delete(h.clients, clientID)
		log.Printf("Client %s unregistered, remaining clients: %d", clientID, len(h.clients))
	}
}

// Broadcast sends an event to all subscribed clients
func (h *EventHub) Broadcast(event *Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, client := range h.clients {
		// Send to clients if:
		// 1. Event has no ID (system-wide events), OR
		// 2. Client has no subscriptions (wants all events), OR
		// 3. Client is subscribed to this specific operation
		if event.ID == "" || len(client.Operations) == 0 || client.IsSubscribed(event.ID) {
			select {
			case client.Channel <- event:
				count++
			default:
				log.Printf("Warning: Client %s channel full, dropping event", client.ID)
			}
		}
	}

	if count > 0 {
		log.Printf("Broadcasted event %s to %d clients", event.Type, count)
	}
}

// SendOperationProgress sends an operation progress event
func (h *EventHub) SendOperationProgress(operationID string, current, total int, message string) {
	event := &Event{
		Type:      EventOperationProgress,
		ID:        operationID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"operation_id": operationID,
			"current":      current,
			"total":        total,
			"message":      message,
			"percentage":   calculatePercentage(current, total),
		},
	}
	h.Broadcast(event)
}

// SendOperationStatus sends an operation status change event
func (h *EventHub) SendOperationStatus(operationID, status string, details map[string]interface{}) {
	event := &Event{
		Type:      EventOperationStatus,
		ID:        operationID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"operation_id": operationID,
			"status":       status,
			"details":      details,
		},
	}
	h.Broadcast(event)
}

// SendOperationLog sends an operation log event
func (h *EventHub) SendOperationLog(operationID, level, message string, details *string) {
	data := map[string]interface{}{
		"operation_id": operationID,
		"level":        level,
		"message":      message,
	}
	if details != nil {
		data["details"] = *details
	}

	event := &Event{
		Type:      EventOperationLog,
		ID:        operationID,
		Timestamp: time.Now(),
		Data:      data,
	}
	h.Broadcast(event)
}

// SendSystemStatus sends a system status event
func (h *EventHub) SendSystemStatus(data map[string]interface{}) {
	event := &Event{
		Type:      EventSystemStatus,
		ID:        "",
		Timestamp: time.Now(),
		Data:      data,
	}
	h.Broadcast(event)
}

// GetClientCount returns the number of connected clients
func (h *EventHub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// HandleSSE handles Server-Sent Events connection
func (h *EventHub) HandleSSE(c *gin.Context) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")

	// Create client
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	client := NewClient(clientID)

	// Subscribe to operations if specified
	if operationID := c.Query("operation"); operationID != "" {
		client.Subscribe(operationID)
	}

	// Register client
	h.RegisterClient(client)
	defer h.UnregisterClient(clientID)

	// Send initial connection event
	initialEvent := &Event{
		Type:      "connection.established",
		ID:        "",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"client_id": clientID,
		},
	}

	if data, err := json.Marshal(initialEvent); err == nil {
		_, _ = c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", data)))
		c.Writer.Flush()
	}

	// Keep connection alive and stream events
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			log.Printf("Client %s connection closed", clientID)
			return
		case event := <-client.Channel:
			// Marshal event to JSON
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("Error marshaling event: %v", err)
				continue
			}

			// Write SSE format: data: {json}\n\n
			_, err = c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", data)))
			if err != nil {
				log.Printf("Error writing to client %s: %v", clientID, err)
				return
			}

			// Flush immediately
			c.Writer.Flush()
		case <-ticker.C:
			// Send heartbeat
			heartbeat := map[string]interface{}{
				"type":      "heartbeat",
				"timestamp": time.Now(),
			}
			if data, err := json.Marshal(heartbeat); err == nil {
				_, _ = c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", data)))
				c.Writer.Flush()
			}
		}
	}
}

// calculatePercentage calculates percentage with bounds checking
func calculatePercentage(current, total int) int {
	if total <= 0 {
		return 0
	}
	percentage := (current * 100) / total
	if percentage > 100 {
		return 100
	}
	return percentage
}

// Global event hub instance
var GlobalHub *EventHub

// InitializeEventHub initializes the global event hub
func InitializeEventHub() {
	if GlobalHub != nil {
		log.Println("Warning: event hub already initialized")
		return
	}
	GlobalHub = NewEventHub()
	log.Println("Event hub initialized")
}
