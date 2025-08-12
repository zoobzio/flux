package flux

import (
	"context"
	"fmt"
	"time"

	"github.com/zoobzio/streamz"
)

// ErrorStrategy defines how stream processing handles errors.
type ErrorStrategy string

const (
	// ErrorContinue continues processing after errors (default)
	ErrorContinue ErrorStrategy = "continue"
	// ErrorStop stops processing on first error
	ErrorStop ErrorStrategy = "stop"
	// ErrorChannel sends errors to error channel and continues
	ErrorChannel ErrorStrategy = "channel"
)

// Stream provides continuous event processing with synchronization capabilities.
// It combines streamz processors for channel operations with pipz-based Sync
// for resilient event handling.
//
// Stream is designed for scenarios where events arrive continuously through
// channels and need to be processed with synchronization primitives like
// rate limiting, buffering, and batching.
type Stream[T any] struct {
	name          string
	sync          *Sync[T]
	pipeline      []streamz.Processor[Event[T], Event[T]]
	errorStrategy ErrorStrategy
	errorChan     chan error
}

// NewStream creates a new stream processor with the given sync handler.
//
// The sync parameter should be configured with all desired capabilities
// (rate limiting, circuit breaking, etc.) before creating the stream.
//
// Example:
//
//	// Create a sync with capabilities
//	sync := flux.NewSync("processor", handler).
//	    WithRateLimit(100, 10).
//	    WithRetry(3)
//
//	// Create a stream that will process events through the sync
//	stream := flux.NewStream("event-stream", sync)
func NewStream[T any](name string, sync *Sync[T]) *Stream[T] {
	return &Stream[T]{
		name:          name,
		sync:          sync,
		pipeline:      []streamz.Processor[Event[T], Event[T]]{},
		errorStrategy: ErrorContinue, // Default
	}
}

// WithThrottle adds stream-level rate limiting.
//
// This throttles the stream before events reach the sync processor.
// It's useful for smoothing out bursts at the stream level, separate
// from any rate limiting configured on the sync itself.
//
// Example:
//
//	stream := flux.NewStream("events", sync).
//	    WithThrottle(50.0)  // 50 events per second max
func (s *Stream[T]) WithThrottle(eventsPerSecond float64) *Stream[T] {
	s.pipeline = append(s.pipeline, streamz.NewThrottle[Event[T]](eventsPerSecond))
	return s
}

// WithBuffer adds buffering to handle bursts.
//
// Events will be buffered up to the specified size. When the buffer
// is full, the behavior depends on the configured strategy.
//
// Example:
//
//	stream := flux.NewStream("events", sync).
//	    WithBuffer(1000)  // Buffer up to 1000 events
func (s *Stream[T]) WithBuffer(size int) *Stream[T] {
	s.pipeline = append(s.pipeline, streamz.NewBuffer[Event[T]](size))
	return s
}

// WithBatch is deprecated. Use BatchStream for batch processing.
//
// Batch processing requires a different handler signature ([]Event[T] instead of Event[T]),
// so it's implemented as a separate type. Use NewBatchStream with NewBatchSync:
//
//	batchSync := flux.NewBatchSync("batch", batchHandler)
//	batchStream := flux.NewBatchStream("stream", batchSync).
//	    WithBatcher(100, time.Second)
//
// Deprecated: Use BatchStream instead.
func (s *Stream[T]) WithBatch(maxSize int, maxLatency time.Duration) *Stream[T] {
	panic("WithBatch is deprecated - use BatchStream with BatchSync for batch processing")
}

// WithFilter adds stream-level filtering.
//
// This filters events before they reach the sync processor, which can
// improve performance by avoiding unnecessary processing.
//
// Example:
//
//	stream := flux.NewStream("events", sync).
//	    WithFilter(func(e Event[T]) bool {
//	        return e.Source == "important"
//	    })
func (s *Stream[T]) WithFilter(predicate func(Event[T]) bool) *Stream[T] {
	s.pipeline = append(s.pipeline, streamz.NewFilter[Event[T]]("filter", predicate))
	return s
}

// WithErrorStrategy sets how the stream handles processing errors.
//
// Available strategies:
//   - ErrorContinue: Log and continue processing (default)
//   - ErrorStop: Stop processing on first error
//   - ErrorChannel: Send errors to channel and continue
//
// Example:
//
//	stream := flux.NewStream("events", sync).
//	    WithErrorStrategy(flux.ErrorStop)
func (s *Stream[T]) WithErrorStrategy(strategy ErrorStrategy) *Stream[T] {
	s.errorStrategy = strategy
	if strategy == ErrorChannel && s.errorChan == nil {
		s.errorChan = make(chan error, 100) // Buffered channel
	}
	return s
}

// ErrorChannel returns the error channel when using ErrorChannel strategy.
// Returns nil if not using ErrorChannel strategy.
func (s *Stream[T]) ErrorChannel() <-chan error {
	return s.errorChan
}

// Process starts processing events from the input channel.
//
// This method blocks until the input channel is closed or the context
// is canceled. Events are processed through the configured stream
// pipeline and then through the sync processor.
//
// The method returns when all events have been processed or an error
// occurs that cannot be handled by the configured error handling.
//
// Example:
//
//	ctx := context.Background()
//	events := make(chan flux.Event[Data])
//
//	// Start processing in a goroutine
//	go func() {
//	    if err := stream.Process(ctx, events); err != nil {
//	        log.Printf("Stream processing failed: %v", err)
//	    }
//	}()
//
//	// Send events
//	events <- flux.Event[Data]{...}
func (s *Stream[T]) Process(ctx context.Context, input <-chan Event[T]) error {
	// Apply stream processors in sequence
	current := input
	for _, proc := range s.pipeline {
		current = proc.Process(ctx, current)
	}

	// Process each event through the sync pipeline
	for event := range current {
		select {
		case <-ctx.Done():
			return fmt.Errorf("stream processing canceled: %w", ctx.Err())
		default:
			// Use event's context for processing
			_, err := s.sync.Process(event)
			if err != nil {
				switch s.errorStrategy {
				case ErrorStop:
					return fmt.Errorf("stream stopped due to error: %w", err)
				case ErrorChannel:
					if s.errorChan != nil {
						select {
						case s.errorChan <- err:
						default:
							// Channel full, drop error
						}
					}
				case ErrorContinue:
					// Continue processing
				}
			}
		}
	}

	// Close error channel if we own it
	if s.errorStrategy == ErrorChannel && s.errorChan != nil {
		close(s.errorChan)
	}

	return nil
}

// ProcessWithErrors starts processing events and returns an error channel.
//
// DEPRECATED: Use Process() with WithErrorStrategy(ErrorChannel) instead.
//
// This method is kept for backward compatibility but the preferred approach is:
//
//	stream := flux.NewStream("events", sync).
//	    WithErrorStrategy(flux.ErrorChannel)
//
//	go stream.Process(ctx, events)
//	errCh := stream.ErrorChannel()
func (s *Stream[T]) ProcessWithErrors(ctx context.Context, input <-chan Event[T]) <-chan error {
	// Set error strategy to channel
	oldStrategy := s.errorStrategy
	s.WithErrorStrategy(ErrorChannel)

	// Start processing in background
	go func() {
		_ = s.Process(ctx, input)
		// Restore original strategy
		s.errorStrategy = oldStrategy
	}()

	return s.errorChan
}

// Name returns the name of this stream processor.
func (s *Stream[T]) Name() string {
	return s.name
}
