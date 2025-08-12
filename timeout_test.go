package flux_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

// TestWithTimeout tests timeout functionality
func TestWithTimeout(t *testing.T) {
	t.Run("timeout exceeded", func(t *testing.T) {
		sync := flux.NewSync("test", func(event flux.Event[string]) error {
			// Sleep longer than timeout
			time.Sleep(200 * time.Millisecond)
			return nil
		}).WithTimeout(100 * time.Millisecond)

		event := flux.Event[string]{
			ID:      "test-1",
			Data:    "data",
			Context: context.Background(),
		}

		start := time.Now()
		_, err := sync.Process(event)
		elapsed := time.Since(start)

		if err == nil {
			t.Error("expected timeout error")
		}

		if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline exceeded") {
			t.Errorf("expected timeout error, got: %v", err)
		}

		// Should timeout at ~100ms, not wait full 200ms
		if elapsed > 150*time.Millisecond {
			t.Errorf("timeout took too long: %v", elapsed)
		}
	})

	t.Run("completes before timeout", func(t *testing.T) {
		sync := flux.NewSync("test", func(event flux.Event[string]) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		}).WithTimeout(200 * time.Millisecond)

		event := flux.Event[string]{
			ID:      "test-2",
			Data:    "data",
			Context: context.Background(),
		}

		_, err := sync.Process(event)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestTimeoutWithContext tests timeout interaction with context deadlines
func TestTimeoutWithContext(t *testing.T) {
	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}).WithTimeout(200 * time.Millisecond)

	// Context with shorter deadline
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	event := flux.Event[string]{
		ID:      "test-3",
		Data:    "data",
		Context: ctx,
	}

	start := time.Now()
	_, err := sync.Process(event)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected timeout error")
	}

	// Should respect the shorter context deadline
	if elapsed > 80*time.Millisecond {
		t.Errorf("should have timed out at context deadline, took: %v", elapsed)
	}
}
