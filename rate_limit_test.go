package flux_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

// TestWithRateLimit tests rate limiting functionality
func TestWithRateLimit(t *testing.T) {
	var count int32

	sync := flux.NewSync("test", func(event flux.Event[int]) error {
		atomic.AddInt32(&count, 1)
		return nil
	}).WithRateLimit(10, 1) // 10 per second, burst 1

	ctx := context.Background()
	start := time.Now()

	// Try to process 5 events quickly
	for i := 0; i < 5; i++ {
		event := flux.Event[int]{
			ID:        string(rune(i)),
			Data:      i,
			Context:   ctx,
			Timestamp: time.Now(),
		}

		_, err := sync.Process(event)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}

	elapsed := time.Since(start)
	processed := atomic.LoadInt32(&count)

	// With rate of 10/sec and burst of 1, processing 5 events should take ~400ms
	// (1 immediate, then 4 more at 100ms intervals)
	if elapsed < 300*time.Millisecond {
		t.Errorf("rate limiting too fast: %v", elapsed)
	}

	if processed != 5 {
		t.Errorf("expected 5 events processed, got %d", processed)
	}
}

// TestWithRateLimitDrop tests rate limiting with drop mode
func TestWithRateLimitDrop(t *testing.T) {
	var processed int32

	sync := flux.NewSync("test", func(event flux.Event[int]) error {
		atomic.AddInt32(&processed, 1)
		time.Sleep(50 * time.Millisecond) // Simulate slow processing
		return nil
	}).WithRateLimitDrop(10, 1) // Very low rate to force drops

	ctx := context.Background()

	// Send many events quickly
	for i := 0; i < 10; i++ {
		event := flux.Event[int]{
			ID:      string(rune(i)),
			Data:    i,
			Context: ctx,
		}

		// First event should succeed, others might be dropped
		_, _ = sync.Process(event)
	}

	// Wait for any processing to complete
	time.Sleep(100 * time.Millisecond)

	count := atomic.LoadInt32(&processed)

	// Should have processed fewer than all events due to drops
	if count >= 10 {
		t.Errorf("expected some events to be dropped, but processed %d/10", count)
	}

	if count == 0 {
		t.Error("expected at least one event to be processed")
	}
}
