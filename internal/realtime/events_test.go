// file: internal/realtime/events_test.go
// version: 1.0.0
// guid: 6f7a8b9c-0d1e-2f3a-4b5c-6d7e8f9a0b1c

package realtime

import (
	"fmt"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	clientID := "test-client-123"
	client := NewClient(clientID)
	
	if client.ID != clientID {
		t.Errorf("Expected client ID %s, got %s", clientID, client.ID)
	}
	if client.Channel == nil {
		t.Error("Expected non-nil channel")
	}
	if client.Operations == nil {
		t.Error("Expected non-nil operations map")
	}
	if len(client.Operations) != 0 {
		t.Error("Expected empty operations map initially")
	}
}

func TestClient_Subscribe(t *testing.T) {
	client := NewClient("test-client")
	operationID := "op-123"
	
	client.Subscribe(operationID)
	
	if !client.IsSubscribed(operationID) {
		t.Errorf("Client should be subscribed to operation %s", operationID)
	}
}

func TestClient_Unsubscribe(t *testing.T) {
	client := NewClient("test-client")
	operationID := "op-123"
	
	client.Subscribe(operationID)
	client.Unsubscribe(operationID)
	
	if client.IsSubscribed(operationID) {
		t.Errorf("Client should not be subscribed to operation %s after unsubscribe", operationID)
	}
}

func TestClient_IsSubscribed(t *testing.T) {
	client := NewClient("test-client")
	
	// Test not subscribed initially
	if client.IsSubscribed("op-1") {
		t.Error("Client should not be subscribed initially")
	}
	
	// Subscribe and test
	client.Subscribe("op-1")
	if !client.IsSubscribed("op-1") {
		t.Error("Client should be subscribed after Subscribe()")
	}
	
	// Test different operation
	if client.IsSubscribed("op-2") {
		t.Error("Client should not be subscribed to different operation")
	}
}

func TestNewEventHub(t *testing.T) {
	hub := NewEventHub()
	
	if hub.clients == nil {
		t.Error("Expected non-nil clients map")
	}
	if len(hub.clients) != 0 {
		t.Error("Expected empty clients map initially")
	}
}

func TestEventHub_RegisterClient(t *testing.T) {
	hub := NewEventHub()
	client := NewClient("client-1")
	
	hub.RegisterClient(client)
	
	if hub.GetClientCount() != 1 {
		t.Errorf("Expected 1 client, got %d", hub.GetClientCount())
	}
}

func TestEventHub_UnregisterClient(t *testing.T) {
	hub := NewEventHub()
	client := NewClient("client-1")
	
	hub.RegisterClient(client)
	hub.UnregisterClient(client.ID)
	
	if hub.GetClientCount() != 0 {
		t.Errorf("Expected 0 clients, got %d", hub.GetClientCount())
	}
}

func TestEventHub_GetClientCount(t *testing.T) {
	hub := NewEventHub()
	
	if hub.GetClientCount() != 0 {
		t.Error("Expected 0 clients initially")
	}
	
	// Add clients
	for i := 0; i < 5; i++ {
		client := NewClient(fmt.Sprintf("client-%d", i))
		hub.RegisterClient(client)
	}
	
	if hub.GetClientCount() != 5 {
		t.Errorf("Expected 5 clients, got %d", hub.GetClientCount())
	}
}

func TestEventHub_Broadcast_SystemWideEvent(t *testing.T) {
	hub := NewEventHub()
	
	// Register clients
	client1 := NewClient("client-1")
	client2 := NewClient("client-2")
	hub.RegisterClient(client1)
	hub.RegisterClient(client2)
	
	// Create system-wide event (no ID)
	event := &Event{
		Type:      EventSystemStatus,
		ID:        "",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"status": "running",
		},
	}
	
	// Broadcast
	hub.Broadcast(event)
	
	// Both clients should receive the event
	select {
	case receivedEvent := <-client1.Channel:
		if receivedEvent.Type != EventSystemStatus {
			t.Error("Client 1 received wrong event type")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Client 1 did not receive event")
	}
	
	select {
	case receivedEvent := <-client2.Channel:
		if receivedEvent.Type != EventSystemStatus {
			t.Error("Client 2 received wrong event type")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Client 2 did not receive event")
	}
}

func TestEventHub_Broadcast_OperationSpecificEvent(t *testing.T) {
	hub := NewEventHub()
	
	// Register clients with different subscriptions
	client1 := NewClient("client-1")
	client1.Subscribe("op-1")
	
	client2 := NewClient("client-2")
	client2.Subscribe("op-2")
	
	hub.RegisterClient(client1)
	hub.RegisterClient(client2)
	
	// Broadcast event for op-1
	event := &Event{
		Type:      EventOperationProgress,
		ID:        "op-1",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"progress": 50,
		},
	}
	
	hub.Broadcast(event)
	
	// Client1 should receive, client2 should not
	select {
	case <-client1.Channel:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Client 1 did not receive operation-specific event")
	}
	
	select {
	case <-client2.Channel:
		t.Error("Client 2 should not receive event for different operation")
	case <-time.After(50 * time.Millisecond):
		// Expected - no event received
	}
}

func TestEventHub_SendOperationProgress(t *testing.T) {
	hub := NewEventHub()
	client := NewClient("client-1")
	client.Subscribe("op-123")
	hub.RegisterClient(client)
	
	hub.SendOperationProgress("op-123", 50, 100, "Processing...")
	
	select {
	case event := <-client.Channel:
		if event.Type != EventOperationProgress {
			t.Error("Received wrong event type")
		}
		if event.ID != "op-123" {
			t.Error("Received wrong operation ID")
		}
		if event.Data["current"] != 50 {
			t.Error("Wrong current value")
		}
		if event.Data["total"] != 100 {
			t.Error("Wrong total value")
		}
		if event.Data["percentage"] != 50 {
			t.Error("Wrong percentage calculation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive operation progress event")
	}
}

func TestEventHub_SendOperationStatus(t *testing.T) {
	hub := NewEventHub()
	client := NewClient("client-1")
	client.Subscribe("op-123")
	hub.RegisterClient(client)
	
	details := map[string]interface{}{
		"files_processed": 42,
	}
	hub.SendOperationStatus("op-123", "completed", details)
	
	select {
	case event := <-client.Channel:
		if event.Type != EventOperationStatus {
			t.Error("Received wrong event type")
		}
		if event.Data["status"] != "completed" {
			t.Error("Wrong status")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive operation status event")
	}
}

func TestEventHub_SendOperationLog(t *testing.T) {
	hub := NewEventHub()
	client := NewClient("client-1")
	client.Subscribe("op-123")
	hub.RegisterClient(client)
	
	message := "Processing file"
	detailsStr := "file.mp3"
	hub.SendOperationLog("op-123", "info", message, &detailsStr)
	
	select {
	case event := <-client.Channel:
		if event.Type != EventOperationLog {
			t.Error("Received wrong event type")
		}
		if event.Data["level"] != "info" {
			t.Error("Wrong log level")
		}
		if event.Data["message"] != message {
			t.Error("Wrong message")
		}
		if event.Data["details"] != detailsStr {
			t.Error("Wrong details")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive operation log event")
	}
}

func TestEventHub_SendSystemStatus(t *testing.T) {
	hub := NewEventHub()
	client := NewClient("client-1")
	hub.RegisterClient(client)
	
	data := map[string]interface{}{
		"uptime": "10m",
	}
	hub.SendSystemStatus(data)
	
	select {
	case event := <-client.Channel:
		if event.Type != EventSystemStatus {
			t.Error("Received wrong event type")
		}
		if event.ID != "" {
			t.Error("System status event should have empty ID")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive system status event")
	}
}

func TestCalculatePercentage(t *testing.T) {
	tests := []struct {
		current  int
		total    int
		expected int
	}{
		{0, 100, 0},
		{50, 100, 50},
		{100, 100, 100},
		{150, 100, 100}, // Should cap at 100
		{0, 0, 0},       // Edge case: total is 0
		{10, 0, 0},      // Edge case: total is 0
		{-5, 100, -5},   // Negative current (edge case)
	}
	
	for _, tt := range tests {
		result := calculatePercentage(tt.current, tt.total)
		if result != tt.expected {
			t.Errorf("calculatePercentage(%d, %d) = %d, want %d", 
				tt.current, tt.total, result, tt.expected)
		}
	}
}

func TestInitializeEventHub(t *testing.T) {
	// Save old global hub
	oldHub := GlobalHub
	defer func() { GlobalHub = oldHub }()
	
	// Reset and initialize
	GlobalHub = nil
	InitializeEventHub()
	
	if GlobalHub == nil {
		t.Error("Expected GlobalHub to be initialized")
	}
	
	// Test idempotency
	prevHub := GlobalHub
	InitializeEventHub()
	if GlobalHub != prevHub {
		t.Error("InitializeEventHub should be idempotent")
	}
}
