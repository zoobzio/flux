package flux_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

// TestBatchSync tests basic batch sync functionality
func TestBatchSync(t *testing.T) {
	var totalProcessed int32

	batchSync := flux.NewBatchSync("test", func(batch []flux.Event[string]) error {
		atomic.AddInt32(&totalProcessed, int32(len(batch)))

		// Verify batch contents
		for i, event := range batch {
			expected := fmt.Sprintf("data-%d", i)
			if event.Data != expected {
				t.Errorf("expected data %s, got %s", expected, event.Data)
			}
		}
		return nil
	})

	// Create a batch of events
	ctx := context.Background()
	batch := make([]flux.Event[string], 5)
	for i := 0; i < 5; i++ {
		batch[i] = flux.Event[string]{
			ID:      fmt.Sprintf("id-%d", i),
			Data:    fmt.Sprintf("data-%d", i),
			Context: ctx,
		}
	}

	_, err := batchSync.Process(batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if atomic.LoadInt32(&totalProcessed) != 5 {
		t.Errorf("expected 5 processed, got %d", totalProcessed)
	}
}

// TestBatchStream tests batch stream processing
func TestBatchStream(t *testing.T) {
	var batchCount int32
	var totalEvents int32

	batchSync := flux.NewBatchSync("test", func(batch []flux.Event[int]) error {
		atomic.AddInt32(&batchCount, 1)
		atomic.AddInt32(&totalEvents, int32(len(batch)))

		t.Logf("Received batch %d with %d events", batchCount, len(batch))
		return nil
	})

	stream := flux.NewBatchStream("test-stream", batchSync).
		WithBatcher(3, 100*time.Millisecond) // Batch size 3 or 100ms

	events := make(chan flux.Event[int])
	ctx := context.Background()

	// Process in background
	done := make(chan error)
	go func() {
		done <- stream.Process(ctx, events)
	}()

	// Send 10 events
	go func() {
		for i := 0; i < 10; i++ {
			events <- flux.Event[int]{
				ID:      fmt.Sprintf("event-%d", i),
				Data:    i,
				Context: ctx,
			}
			// Small delay to test batching
			if i == 5 {
				time.Sleep(150 * time.Millisecond) // Force timeout batch
			}
		}
		close(events)
	}()

	// Wait for completion
	err := <-done
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have received all 10 events
	if atomic.LoadInt32(&totalEvents) != 10 {
		t.Errorf("expected 10 total events, got %d", totalEvents)
	}

	// Should have multiple batches due to size limit and timeout
	batches := atomic.LoadInt32(&batchCount)
	if batches < 2 {
		t.Errorf("expected at least 2 batches, got %d", batches)
	}
}

// TestBatchSyncWithCapabilities tests batch sync with various capabilities
func TestBatchSyncWithCapabilities(t *testing.T) {
	var attempts int32

	batchSync := flux.NewBatchSync("test", func(batch []flux.Event[string]) error {
		attempt := atomic.AddInt32(&attempts, 1)
		// Fail first attempt
		if attempt == 1 {
			return fmt.Errorf("batch processing failed")
		}
		return nil
	}).
		WithRetry(2).
		WithTimeout(100*time.Millisecond).
		WithRateLimit(10, 2)

	ctx := context.Background()
	batch := []flux.Event[string]{
		{ID: "1", Data: "data1", Context: ctx},
		{ID: "2", Data: "data2", Context: ctx},
	}

	_, err := batchSync.Process(batch)
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}

	// Should have retried
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}
