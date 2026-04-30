// file: internal/plugins/webhook/plugin_test.go
// version: 1.0.1
// guid: c4d5e6f7-a8b9-0c1d-2e3f-4a5b6c7d8e9f
// last-edited: 2026-04-30

package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePlugin() *Plugin { return &Plugin{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func TestPlugin_ID(t *testing.T) {
	assert.Equal(t, "webhook", makePlugin().ID())
}

func TestPlugin_Capabilities(t *testing.T) {
	caps := makePlugin().Capabilities()
	require.Len(t, caps, 1)
	assert.Equal(t, plugin.CapEventSubscriber, caps[0])
}

// ---------------------------------------------------------------------------
// Init validation
// ---------------------------------------------------------------------------

func TestInit_MissingURLs(t *testing.T) {
	p := makePlugin()
	err := p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{},
		Events: plugin.NewEventBus(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "urls is required")
}

func TestInit_EmptyURLAfterSplit(t *testing.T) {
	p := makePlugin()
	err := p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": ",,,"},
		Events: plugin.NewEventBus(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid URLs configured")
}

func TestInit_NilEventBus(t *testing.T) {
	p := makePlugin()
	err := p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": "http://example.com"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event bus not provided")
}

func TestInit_SubscribesToAllEventsByDefault(t *testing.T) {
	bus := plugin.NewEventBus()
	p := makePlugin()
	require.NoError(t, p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": "http://example.com"},
		Events: bus,
	}))
	// All event types should be subscribed
	for _, et := range allEventTypes() {
		assert.Equal(t, 1, bus.SubscriberCount(et), "expected subscription for %s", et)
	}
}

func TestInit_SubscribesToSpecificEvents(t *testing.T) {
	bus := plugin.NewEventBus()
	p := makePlugin()
	require.NoError(t, p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{
			"urls":   "http://example.com",
			"events": "book.imported,book.deleted",
		},
		Events: bus,
	}))
	assert.Equal(t, 1, bus.SubscriberCount(plugin.EventBookImported))
	assert.Equal(t, 1, bus.SubscriberCount(plugin.EventBookDeleted))
	assert.Equal(t, 0, bus.SubscriberCount(plugin.EventScanCompleted))
}

// ---------------------------------------------------------------------------
// HealthCheck
// ---------------------------------------------------------------------------

func TestHealthCheck_NotInitialized(t *testing.T) {
	assert.Error(t, makePlugin().HealthCheck())
}

func TestHealthCheck_Initialized(t *testing.T) {
	p := makePlugin()
	_ = p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": "http://example.com"},
		Events: plugin.NewEventBus(),
	})
	assert.NoError(t, p.HealthCheck())
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

func TestShutdown(t *testing.T) {
	p := makePlugin()
	_ = p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": "http://example.com"},
		Events: plugin.NewEventBus(),
	})
	require.NoError(t, p.Shutdown(context.Background()))
	assert.Error(t, p.HealthCheck(), "should be unhealthy after shutdown")
}

// ---------------------------------------------------------------------------
// Event delivery
// ---------------------------------------------------------------------------

func TestDeliver_PostsEventToURL(t *testing.T) {
	received := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := plugin.NewEventBus()
	p := makePlugin()
	require.NoError(t, p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": srv.URL},
		Events: bus,
	}))

	evt := plugin.NewEvent(plugin.EventBookImported, "book-1", map[string]any{"title": "Dune"})
	bus.Publish(context.Background(), evt)

	// Wait for the async goroutine to deliver (with timeout).
	select {
	case body := <-received:
		var got plugin.Event
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, plugin.EventBookImported, got.Type)
		assert.Equal(t, "book-1", got.BookID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP delivery")
	}
}

func TestDeliver_HMAC_SignaturePresent(t *testing.T) {
	sigHeader := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader <- r.Header.Get("X-Audiobook-Signature-256")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := plugin.NewEventBus()
	p := makePlugin()
	require.NoError(t, p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": srv.URL, "secret": "mysecret"},
		Events: bus,
	}))

	bus.Publish(context.Background(), plugin.NewEvent(plugin.EventBookImported, "b1", nil))

	select {
	case sig := <-sigHeader:
		assert.True(t, len(sig) > 0, "expected HMAC header")
		assert.Contains(t, sig, "sha256=")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP delivery")
	}
}

func TestDeliver_NoSignatureWhenNoSecret(t *testing.T) {
	sigHeader := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader <- r.Header.Get("X-Audiobook-Signature-256")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := plugin.NewEventBus()
	p := makePlugin()
	require.NoError(t, p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": srv.URL},
		Events: bus,
	}))

	bus.Publish(context.Background(), plugin.NewEvent(plugin.EventScanCompleted, "", nil))

	select {
	case sig := <-sigHeader:
		assert.Empty(t, sig)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP delivery")
	}
}

func TestDeliver_MultipleURLs(t *testing.T) {
	hits := make(chan struct{}, 10)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- struct{}{}
		w.WriteHeader(http.StatusOK)
	})
	srv1 := httptest.NewServer(handler)
	defer srv1.Close()
	srv2 := httptest.NewServer(handler)
	defer srv2.Close()

	bus := plugin.NewEventBus()
	p := makePlugin()
	require.NoError(t, p.Init(context.Background(), plugin.Deps{
		Config: map[string]string{"urls": srv1.URL + "," + srv2.URL},
		Events: bus,
	}))

	bus.Publish(context.Background(), plugin.NewEvent(plugin.EventBookDeleted, "b1", nil))
	time.Sleep(100 * time.Millisecond)

	assert.Len(t, hits, 2)
}
