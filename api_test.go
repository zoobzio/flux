package flux_test

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

type testContextKey string

// TestEvent tests the Event type
func TestEvent(t *testing.T) {
	ctx := context.WithValue(context.Background(), testContextKey("test-key"), "test-value")

	event := flux.Event[string]{
		ID:        "test-1",
		Data:      "test-data",
		Source:    "test-source",
		Timestamp: time.Now(),
		Context:   ctx,
		Metadata: map[string]any{
			"key": "value",
		},
	}

	// Verify fields
	if event.ID != "test-1" {
		t.Errorf("expected ID 'test-1', got '%s'", event.ID)
	}

	if event.Data != "test-data" {
		t.Errorf("expected data 'test-data', got '%s'", event.Data)
	}

	if event.Source != "test-source" {
		t.Errorf("expected source 'test-source', got '%s'", event.Source)
	}

	// Verify context propagation
	if val := event.Context.Value(testContextKey("test-key")); val != "test-value" {
		t.Errorf("expected context value 'test-value', got '%v'", val)
	}

	// Verify metadata
	if val, ok := event.Metadata["key"]; !ok || val != "value" {
		t.Errorf("expected metadata key='value', got '%v'", val)
	}
}

// TestSyncProcess tests basic Sync processing
func TestSyncProcess(t *testing.T) {
	called := false
	var receivedEvent flux.Event[string]

	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		called = true
		receivedEvent = event
		return nil
	})

	ctx := context.Background()
	event := flux.Event[string]{
		ID:        "test-1",
		Data:      "test-data",
		Source:    "test",
		Context:   ctx,
		Timestamp: time.Now(),
	}

	result, err := sync.Process(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Error("handler was not called")
	}

	// Verify event was passed correctly
	if receivedEvent.ID != event.ID {
		t.Errorf("event ID mismatch: expected %s, got %s", event.ID, receivedEvent.ID)
	}

	// Verify result is returned unchanged
	if result.ID != event.ID {
		t.Errorf("result ID mismatch: expected %s, got %s", event.ID, result.ID)
	}
}

// TestSyncName tests that Sync implements Name() method
func TestSyncName(t *testing.T) {
	sync := flux.NewSync("test-sync", func(event flux.Event[string]) error {
		return nil
	})

	name := sync.Name()
	if string(name) != "test-sync" {
		t.Errorf("expected name 'test-sync', got '%s'", name)
	}
}
