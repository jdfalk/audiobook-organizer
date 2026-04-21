// file: internal/plugins/webhook/plugin_test.go
// version: 1.0.0
// guid: c4d5e6f7-a8b9-0c1d-2e3f-4a5b6c7d8e9f

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
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
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

	// Give the async goroutine a moment to deliver.
	time.Sleep(50 * time.Millisecond)

	var got plugin.Event
	require.NoError(t, json.Unmarshal(received, &got))
	assert.Equal(t, plugin.EventBookImported, got.Type)
	assert.Equal(t, "book-1", got.BookID)
}

func TestDeliver_HMAC_SignaturePresent(t *testing.T) {
	var sigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader = r.Header.Get("X-Audiobook-Signature-256")
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
	time.Sleep(50 * time.Millisecond)

	assert.True(t, len(sigHeader) > 0, "expected HMAC header")
	assert.Contains(t, sigHeader, "sha256=")
}

func TestDeliver_NoSignatureWhenNoSecret(t *testing.T) {
	var sigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader = r.Header.Get("X-Audiobook-Signature-256")
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
	time.Sleep(50 * time.Millisecond)

	assert.Empty(t, sigHeader)
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
