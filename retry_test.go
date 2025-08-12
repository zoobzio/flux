package flux_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

// TestWithRetry tests retry functionality
func TestWithRetry(t *testing.T) {
	var attempts int32
	failUntil := int32(2)

	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt <= failUntil {
			return errors.New("simulated failure")
		}
		return nil
	}).WithRetry(3) // Retry up to 3 times

	ctx := context.Background()
	event := flux.Event[string]{
		ID:      "test-1",
		Data:    "data",
		Context: ctx,
	}

	// Should succeed on the 3rd attempt
	_, err := sync.Process(event)
	if err != nil {
		t.Errorf("expected success after retries, got: %v", err)
	}

	totalAttempts := atomic.LoadInt32(&attempts)
	if totalAttempts != 3 {
		t.Errorf("expected 3 attempts, got %d", totalAttempts)
	}
}

// TestWithRetryExhausted tests when retries are exhausted
func TestWithRetryExhausted(t *testing.T) {
	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		return errors.New("always fails")
	}).WithRetry(2)

	event := flux.Event[string]{
		ID:      "test-1",
		Data:    "data",
		Context: context.Background(),
	}

	_, err := sync.Process(event)
	if err == nil {
		t.Error("expected error after exhausted retries")
	}
}

// TestWithBackoff tests exponential backoff retry
func TestWithBackoff(t *testing.T) {
	var attempts int32
	var timestamps []time.Time

	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		atomic.AddInt32(&attempts, 1)
		timestamps = append(timestamps, time.Now())

		if len(timestamps) < 3 {
			return errors.New("fail to test backoff")
		}
		return nil
	}).WithBackoff(3, 50*time.Millisecond)

	event := flux.Event[string]{
		ID:      "test-1",
		Data:    "data",
		Context: context.Background(),
	}

	start := time.Now()
	_, err := sync.Process(event)
	if err != nil {
		t.Errorf("expected success after backoff retries, got: %v", err)
	}

	// Verify timing - should have delays of ~50ms, ~100ms
	if len(timestamps) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(timestamps))
	}

	// First retry after ~50ms
	delay1 := timestamps[1].Sub(timestamps[0])
	if delay1 < 40*time.Millisecond || delay1 > 70*time.Millisecond {
		t.Errorf("first retry delay out of range: %v", delay1)
	}

	// Second retry after ~100ms (exponential)
	delay2 := timestamps[2].Sub(timestamps[1])
	if delay2 < 90*time.Millisecond || delay2 > 120*time.Millisecond {
		t.Errorf("second retry delay out of range: %v", delay2)
	}

	// Total time should be at least 150ms
	totalTime := time.Since(start)
	if totalTime < 140*time.Millisecond {
		t.Errorf("total time too short for backoff: %v", totalTime)
	}
}
