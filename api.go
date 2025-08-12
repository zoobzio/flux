// Package flux provides synchronization infrastructure for wrapping callbacks
// with rate limiting, circuit breaking, retries, and other resilience patterns.
//
// flux follows a builder pattern similar to zlog, allowing services to compose
// synchronization capabilities around their callbacks. It is designed to be
// embedded within services that need to manage synchronization, not run as
// a standalone service.
//
// Basic usage:
//
//	// Create a sync wrapper with desired capabilities
//	sync := flux.NewSync("user-processor", processUser).
//	    WithRateLimit(100, 10).                    // 100 RPS, burst 10
//	    WithCircuitBreaker(5, 30*time.Second).     // Open after 5 failures
//	    WithRetry(3).                              // Retry up to 3 times
//	    WithTimeout(5*time.Second)                 // 5 second timeout
//
//	// Process events through the sync pipeline
//	event := flux.Event[User]{
//	    ID:        "user-123",
//	    Data:      user,
//	    Source:    "user-service",
//	    Timestamp: time.Now(),
//	}
//	result, err := sync.Process(event)
//
// The package provides:
//   - Sync: Core synchronization wrapper with builder methods
//   - Event: Standard event wrapper for synchronized data
//   - Stream: Continuous event processing with streaming capabilities
//
// All synchronization is built on top of pipz for composability and
// streamz for channel-based streaming operations.
package flux

import (
	"context"
	"time"

	"github.com/zoobzio/pipz"
)

// Event is the standard wrapper for all synchronized data.
// It provides metadata about the data being processed while
// remaining agnostic to the actual data type.
//
// Each event carries its own context for proper observability,
// tracing, and deadline propagation through the pipeline.
type Event[T any] struct {
	// ID uniquely identifies this event
	ID string

	// Data is the actual payload being synchronized
	Data T

	// Source identifies where this event originated
	Source string

	// Timestamp records when this event was created
	Timestamp time.Time

	// Context carries request-scoped values like traces, deadlines, and metrics
	Context context.Context

	// Metadata allows for additional context
	Metadata map[string]any
}

// Sync wraps user callbacks with synchronization primitives.
// It implements pipz.Chainable to allow composition with other
// pipz processors and connectors.
//
// Sync is immutable - each builder method returns a new Sync
// instance with the added capability. This allows for safe
// concurrent use and clear composition.
type Sync[T any] struct {
	processor pipz.Chainable[Event[T]]
}

// Process delegates to the underlying processor pipeline.
// The event's context is used for processing, ensuring proper
// propagation of traces, deadlines, and other request-scoped values.
//
// If the event doesn't have a context, context.Background() is used.
func (s Sync[T]) Process(event Event[T]) (Event[T], error) {
	ctx := event.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return s.processor.Process(ctx, event)
}

// Name returns the name of the underlying processor.
// This satisfies the pipz.Chainable interface.
func (s Sync[T]) Name() pipz.Name {
	return s.processor.Name()
}
