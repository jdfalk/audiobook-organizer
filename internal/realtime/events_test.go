// file: internal/realtime/events_test.go
// version: 1.0.0
// guid: a0b1c2d3-e4f5-6a7b-8c9d-0e1f2a3b4c5d
// last-edited: 2026-01-19

package realtime

import (
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("test-client-1")
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.ID != "test-client-1" {
		t.Errorf("Expected ID 'test-client-1', got '%s'", client.ID)
	}
	if client.Channel == nil {
		t.Error("Client channel is nil")
	}
	if client.Operations == nil {
		t.Error("Client operations map is nil")
	}
}

func TestClient_Subscribe(t *testing.T) {
	client := NewClient("test-client-2")
	client.Subscribe("operation-1")
	if !client.Operations["operation-1"] {
		t.Error("Client did not subscribe to operation-1")
	}
}

func TestClient_Unsubscribe(t *testing.T) {
	client := NewClient("test-client-3")
	client.Subscribe("operation-2")
	client.Unsubscribe("operation-2")
	if client.Operations["operation-2"] {
		t.Error("Client is still subscribed to operation-2")
	}
}

func TestEvent_Creation(t *testing.T) {
	event := &Event{
		Type:      EventOperationProgress,
		ID:        "test-event-1",
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"progress": 50},
	}
	if event.Type != EventOperationProgress {
		t.Errorf("Expected EventOperationProgress, got %v", event.Type)
	}
	if event.ID != "test-event-1" {
		t.Errorf("Expected ID 'test-event-1', got '%s'", event.ID)
	}
	if event.Data["progress"] != 50 {
		t.Errorf("Expected progress 50, got %v", event.Data["progress"])
	}
}

func TestEventTypes(t *testing.T) {
	types := []EventType{
		EventOperationProgress,
		EventOperationStatus,
		EventOperationLog,
		EventSystemStatus,
	}
	for _, et := range types {
		if string(et) == "" {
			t.Errorf("EventType is empty: %v", et)
		}
	}
}
