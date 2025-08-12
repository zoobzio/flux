package flux_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zoobzio/flux"
)

// TestNewSync tests basic sync creation and functionality
func TestNewSync(t *testing.T) {
	handlerCalled := false
	testErr := errors.New("test error")

	t.Run("successful handler", func(t *testing.T) {
		sync := flux.NewSync("success", func(event flux.Event[string]) error {
			handlerCalled = true
			if event.Data != "test" {
				t.Errorf("expected data 'test', got '%s'", event.Data)
			}
			return nil
		})

		event := flux.Event[string]{
			ID:      "1",
			Data:    "test",
			Context: context.Background(),
		}

		_, err := sync.Process(event)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !handlerCalled {
			t.Error("handler was not called")
		}
	})

	t.Run("failing handler", func(t *testing.T) {
		sync := flux.NewSync("failure", func(event flux.Event[string]) error {
			return testErr
		})

		event := flux.Event[string]{
			ID:      "2",
			Data:    "test",
			Context: context.Background(),
		}

		_, err := sync.Process(event)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("nil context handling", func(t *testing.T) {
		var receivedContext context.Context

		sync := flux.NewSync("nil-context", func(event flux.Event[string]) error {
			receivedContext = event.Context
			return nil
		})

		event := flux.Event[string]{
			ID:      "3",
			Data:    "test",
			Context: nil, // Explicitly nil
		}

		_, err := sync.Process(event)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if receivedContext == nil {
			t.Error("expected context to be set, got nil")
		}
	})
}

type syncTestContextKey string

// TestSyncContextPropagation tests that context is properly propagated
func TestSyncContextPropagation(t *testing.T) {
	key := syncTestContextKey("test-key")
	value := "test-value"

	sync := flux.NewSync("context-test", func(event flux.Event[string]) error {
		// Verify context value is accessible
		if val := event.Context.Value(key); val != value {
			t.Errorf("expected context value '%s', got '%v'", value, val)
		}
		return nil
	})

	ctx := context.WithValue(context.Background(), key, value)
	event := flux.Event[string]{
		ID:      "ctx-1",
		Data:    "test",
		Context: ctx,
	}

	_, err := sync.Process(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
