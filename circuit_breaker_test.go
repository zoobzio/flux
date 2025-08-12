package flux_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

// TestWithCircuitBreaker tests circuit breaker functionality
func TestWithCircuitBreaker(t *testing.T) {
	var shouldFail atomic.Bool
	shouldFail.Store(true)

	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		if shouldFail.Load() {
			return errors.New("simulated failure")
		}
		return nil
	}).WithCircuitBreaker(2, 100*time.Millisecond) // Open after 2 failures

	ctx := context.Background()
	event := flux.Event[string]{
		ID:      "test-1",
		Data:    "data",
		Context: ctx,
	}

	// First two calls should fail and open the circuit
	for i := 0; i < 2; i++ {
		_, err := sync.Process(event)
		if err == nil {
			t.Error("expected error")
		}
	}

	// Circuit should be open now
	_, err := sync.Process(event)
	if err == nil {
		t.Error("expected circuit breaker open error")
	}

	if !strings.Contains(err.Error(), "circuit breaker is open") {
		t.Errorf("expected circuit breaker error, got: %v", err)
	}

	// Allow handler to succeed for recovery
	shouldFail.Store(false)

	// Wait for circuit to go to half-open
	time.Sleep(150 * time.Millisecond)

	// This should succeed and close the circuit
	_, err = sync.Process(event)
	if err != nil {
		t.Errorf("expected success after circuit recovery, got: %v", err)
	}
}
