package flux_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

// TestNewStream tests basic stream creation
func TestNewStream(t *testing.T) {
	processed := 0
	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		processed++
		return nil
	})

	stream := flux.NewStream("test-stream", sync)

	events := make(chan flux.Event[string], 3)
	events <- flux.Event[string]{ID: "1", Data: "one", Context: context.Background()}
	events <- flux.Event[string]{ID: "2", Data: "two", Context: context.Background()}
	events <- flux.Event[string]{ID: "3", Data: "three", Context: context.Background()}
	close(events)

	err := stream.Process(context.Background(), events)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if processed != 3 {
		t.Errorf("expected 3 events processed, got %d", processed)
	}
}

// TestStreamWithThrottle tests stream throttling
func TestStreamWithThrottle(t *testing.T) {
	processed := 0
	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		processed++
		return nil
	})

	// Throttle to 10 events per second
	stream := flux.NewStream("throttled", sync).
		WithThrottle(10.0)

	events := make(chan flux.Event[string], 5)
	for i := 0; i < 5; i++ {
		events <- flux.Event[string]{
			ID:      string(rune(i)),
			Data:    "data",
			Context: context.Background(),
		}
	}
	close(events)

	start := time.Now()
	err := stream.Process(context.Background(), events)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// With 10/sec rate, 5 events should take ~400ms
	if elapsed < 300*time.Millisecond {
		t.Errorf("throttling too fast: %v", elapsed)
	}
}

// TestStreamErrorStrategies tests different error handling strategies
func TestStreamErrorStrategies(t *testing.T) {
	t.Run("ErrorContinue", func(t *testing.T) {
		processed := 0
		sync := flux.NewSync("test", func(event flux.Event[int]) error {
			if event.Data%2 == 0 {
				return errors.New("even number")
			}
			processed++
			return nil
		})

		stream := flux.NewStream("test", sync).
			WithErrorStrategy(flux.ErrorContinue)

		events := make(chan flux.Event[int], 5)
		for i := 1; i <= 5; i++ {
			events <- flux.Event[int]{ID: string(rune(i)), Data: i, Context: context.Background()}
		}
		close(events)

		err := stream.Process(context.Background(), events)
		if err != nil {
			t.Errorf("ErrorContinue should not return error: %v", err)
		}

		// Should process odd numbers only (1, 3, 5)
		if processed != 3 {
			t.Errorf("expected 3 processed, got %d", processed)
		}
	})

	t.Run("ErrorStop", func(t *testing.T) {
		processed := 0
		sync := flux.NewSync("test", func(event flux.Event[int]) error {
			processed++
			if event.Data == 3 {
				return errors.New("stop at 3")
			}
			return nil
		})

		stream := flux.NewStream("test", sync).
			WithErrorStrategy(flux.ErrorStop)

		events := make(chan flux.Event[int], 5)
		for i := 1; i <= 5; i++ {
			events <- flux.Event[int]{ID: string(rune(i)), Data: i, Context: context.Background()}
		}
		close(events)

		err := stream.Process(context.Background(), events)
		if err == nil {
			t.Error("ErrorStop should return error")
		}

		// Should stop at 3
		if processed != 3 {
			t.Errorf("expected processing to stop at 3, processed %d", processed)
		}
	})
}

// TestStreamWithFilter tests stream-level filtering
func TestStreamWithFilter(t *testing.T) {
	processed := 0
	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		processed++
		return nil
	})

	stream := flux.NewStream("filtered", sync).
		WithFilter(func(e flux.Event[string]) bool {
			return e.Source == "keep"
		})

	events := make(chan flux.Event[string], 4)
	events <- flux.Event[string]{ID: "1", Data: "a", Source: "keep", Context: context.Background()}
	events <- flux.Event[string]{ID: "2", Data: "b", Source: "skip", Context: context.Background()}
	events <- flux.Event[string]{ID: "3", Data: "c", Source: "keep", Context: context.Background()}
	events <- flux.Event[string]{ID: "4", Data: "d", Source: "skip", Context: context.Background()}
	close(events)

	err := stream.Process(context.Background(), events)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if processed != 2 {
		t.Errorf("expected 2 events to pass filter, got %d", processed)
	}
}
