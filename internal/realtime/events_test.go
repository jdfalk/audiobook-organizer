package realtime
// file: internal/realtime/events_test.go
// version: 1.0.0
// guid: a0b1c2d3-e4f5-6a7b-8c9d-0e1f2a3b4c5d
// last-edited: 2026-01-19

package realtime

import (
	"testing"
	"time"






































































}	}		}			t.Errorf("EventType is empty: %v", et)		if string(et) == "" {	for _, et := range types {		}		EventSystemStatus,		EventOperationLog,		EventOperationStatus,		EventOperationProgress,	types := []EventType{func TestEventTypes(t *testing.T) {}	}		t.Errorf("Expected progress 50, got %v", event.Data["progress"])	if event.Data["progress"] != 50 {	}		t.Errorf("Expected ID 'test-event-1', got '%s'", event.ID)	if event.ID != "test-event-1" {	}		t.Errorf("Expected EventOperationProgress, got %v", event.Type)	if event.Type != EventOperationProgress {		}		Data:      map[string]interface{}{"progress": 50},		Timestamp: time.Now(),		ID:        "test-event-1",		Type:      EventOperationProgress,	event := &Event{func TestEvent_Creation(t *testing.T) {}	}		t.Error("Client is still subscribed to operation-2")	if client.Operations["operation-2"] {		client.Unsubscribe("operation-2")	client.Subscribe("operation-2")	client := NewClient("test-client-3")func TestClient_Unsubscribe(t *testing.T) {}	}		t.Error("Client did not subscribe to operation-1")	if !client.Operations["operation-1"] {		client.Subscribe("operation-1")	client := NewClient("test-client-2")func TestClient_Subscribe(t *testing.T) {}	}		t.Error("Client operations map is nil")	if client.Operations == nil {	}		t.Error("Client channel is nil")	if client.Channel == nil {	}		t.Errorf("Expected ID 'test-client-1', got '%s'", client.ID)	if client.ID != "test-client-1" {	}		t.Fatal("NewClient returned nil")	if client == nil {	client := NewClient("test-client-1")func TestNewClient(t *testing.T) {)