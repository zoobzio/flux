package flux_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zoobzio/flux"
)

// TestWithFallback tests fallback functionality
func TestWithFallback(t *testing.T) {
	primaryCalled := false
	fallbackCalled := false

	t.Run("primary succeeds", func(t *testing.T) {
		// Reset flags
		primaryCalled = false
		fallbackCalled = false

		primary := flux.NewSync("primary", func(event flux.Event[string]) error {
			primaryCalled = true
			return nil // Success
		})

		fallback := flux.NewSync("fallback", func(event flux.Event[string]) error {
			fallbackCalled = true
			return nil
		})

		sync := primary.WithFallback(fallback)

		event := flux.Event[string]{
			ID:      "test-1",
			Data:    "data",
			Context: context.Background(),
		}

		_, err := sync.Process(event)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !primaryCalled {
			t.Error("primary handler was not called")
		}

		if fallbackCalled {
			t.Error("fallback should not be called when primary succeeds")
		}
	})

	t.Run("primary fails, fallback succeeds", func(t *testing.T) {
		// Reset flags
		primaryCalled = false
		fallbackCalled = false

		primary := flux.NewSync("primary", func(event flux.Event[string]) error {
			primaryCalled = true
			return errors.New("primary failure")
		})

		fallback := flux.NewSync("fallback", func(event flux.Event[string]) error {
			fallbackCalled = true
			return nil // Fallback succeeds
		})

		sync := primary.WithFallback(fallback)

		event := flux.Event[string]{
			ID:      "test-2",
			Data:    "data",
			Context: context.Background(),
		}

		_, err := sync.Process(event)
		if err != nil {
			t.Errorf("expected fallback to succeed, got error: %v", err)
		}

		if !primaryCalled {
			t.Error("primary handler was not called")
		}

		if !fallbackCalled {
			t.Error("fallback handler was not called after primary failure")
		}
	})

	t.Run("both fail", func(t *testing.T) {
		primary := flux.NewSync("primary", func(event flux.Event[string]) error {
			return errors.New("primary failure")
		})

		fallback := flux.NewSync("fallback", func(event flux.Event[string]) error {
			return errors.New("fallback failure")
		})

		sync := primary.WithFallback(fallback)

		event := flux.Event[string]{
			ID:      "test-3",
			Data:    "data",
			Context: context.Background(),
		}

		_, err := sync.Process(event)
		if err == nil {
			t.Error("expected error when both primary and fallback fail")
		}
	})
}
