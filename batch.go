package flux

import (
	"context"
	"time"

	"github.com/zoobzio/pipz"
	"github.com/zoobzio/streamz"
)

// BatchSync wraps batch callbacks with synchronization primitives.
// Similar to Sync but handles batches of events instead of individual events.
type BatchSync[T any] struct {
	processor pipz.Chainable[[]Event[T]]
}

// Process delegates to the underlying processor pipeline.
// Uses the context from the first event in the batch, or background if empty.
func (bs BatchSync[T]) Process(batch []Event[T]) ([]Event[T], error) {
	ctx := context.Background()
	if len(batch) > 0 && batch[0].Context != nil {
		ctx = batch[0].Context
	}
	return bs.processor.Process(ctx, batch)
}

// Name returns the name of the underlying processor.
func (bs BatchSync[T]) Name() pipz.Name {
	return bs.processor.Name()
}

// NewBatchSync creates a synchronization wrapper for a batch callback.
//
// The handler processes batches of events instead of individual events.
// This is useful for operations that are more efficient in batches,
// such as bulk database inserts or batch API calls.
//
// Each event in the batch carries its own context, allowing for proper
// tracing of individual items even when processed in bulk.
//
// Example:
//
//	batchSync := flux.NewBatchSync("batch-insert", func(batch []flux.Event[User]) error {
//	    // Use context from first event for batch operation
//	    ctx := batch[0].Context
//
//	    // Process entire batch
//	    users := make([]User, len(batch))
//	    for i, event := range batch {
//	        users[i] = event.Data
//	    }
//	    return db.BulkInsert(ctx, users)
//	})
func NewBatchSync[T any](name string, handler func([]Event[T]) error) *BatchSync[T] {
	// Wrap to adapt to pipz.Effect signature
	wrapper := func(ctx context.Context, batch []Event[T]) error {
		// Ensure all events have context
		for i := range batch {
			if batch[i].Context == nil {
				batch[i].Context = ctx
			}
		}
		return handler(batch)
	}

	return &BatchSync[T]{
		processor: pipz.Effect[[]Event[T]](name, wrapper),
	}
}

// WithRateLimit adds rate limiting to the batch processor.
// The rate limit applies to batches, not individual events.
func (bs *BatchSync[T]) WithRateLimit(batchesPerSecond float64, burst int) *BatchSync[T] {
	limiter := pipz.NewRateLimiter[[]Event[T]]("batch-rate-limit", batchesPerSecond, burst)
	return &BatchSync[T]{
		processor: pipz.NewSequence("rate-limited", limiter, bs.processor),
	}
}

// WithRetry adds retry capability to the batch processor.
func (bs *BatchSync[T]) WithRetry(attempts int) *BatchSync[T] {
	return &BatchSync[T]{
		processor: pipz.NewRetry("batch-retry", bs.processor, attempts),
	}
}

// WithTimeout adds timeout protection to batch processing.
func (bs *BatchSync[T]) WithTimeout(duration time.Duration) *BatchSync[T] {
	return &BatchSync[T]{
		processor: pipz.NewTimeout("batch-timeout", bs.processor, duration),
	}
}

// WithErrorHandler adds an error processing pipeline for failed batches.
//
// The error handler receives a *pipz.Error[[]Event[T]] containing the
// failed batch and error details.
func (bs *BatchSync[T]) WithErrorHandler(errorHandler pipz.Chainable[*pipz.Error[[]Event[T]]]) *BatchSync[T] {
	return &BatchSync[T]{
		processor: pipz.NewHandle("batch-error-handler", bs.processor, errorHandler),
	}
}

// BatchStream handles stream processing with batching.
// It converts streams of individual events into batches for processing.
type BatchStream[T any] struct {
	name     string
	sync     *BatchSync[T]
	batcher  streamz.Processor[Event[T], []Event[T]]
	pipeline []streamz.Processor[[]Event[T], []Event[T]]
}

// NewBatchStream creates a stream processor that batches events.
//
// The sync parameter should be a BatchSync configured to handle
// batches of events rather than individual events.
//
// Example:
//
//	batchSync := flux.NewBatchSync("bulk-insert", bulkHandler).
//	    WithRetry(3).
//	    WithTimeout(30*time.Second)
//
//	stream := flux.NewBatchStream("events", batchSync).
//	    WithBatcher(100, time.Second)  // Batch up to 100 or every second
func NewBatchStream[T any](name string, sync *BatchSync[T]) *BatchStream[T] {
	return &BatchStream[T]{
		name:     name,
		sync:     sync,
		pipeline: []streamz.Processor[[]Event[T], []Event[T]]{},
	}
}

// WithBatcher configures the batching parameters.
//
// Events will be collected into batches based on size and time limits.
// A batch is emitted when either limit is reached.
//
// Parameters:
//   - maxSize: Maximum number of events per batch
//   - maxLatency: Maximum time to wait before emitting a partial batch
//
// Example:
//
//	stream := flux.NewBatchStream("events", batchSync).
//	    WithBatcher(100, time.Second)  // Emit when 100 events or 1 second
func (bs *BatchStream[T]) WithBatcher(maxSize int, maxLatency time.Duration) *BatchStream[T] {
	bs.batcher = streamz.NewBatcher[Event[T]](streamz.BatchConfig{
		MaxSize:    maxSize,
		MaxLatency: maxLatency,
	})
	return bs
}

// Process starts processing events from the input channel in batches.
//
// Events are collected into batches according to the batcher configuration
// and then processed through the batch sync pipeline.
//
// The method blocks until the input channel is closed or the context
// is canceled.
func (bs *BatchStream[T]) Process(ctx context.Context, input <-chan Event[T]) error {
	if bs.batcher == nil {
		// Default batcher if not configured
		bs.batcher = streamz.NewBatcher[Event[T]](streamz.BatchConfig{
			MaxSize:    100,
			MaxLatency: time.Second,
		})
	}

	// Apply batcher to convert T -> []T
	batched := bs.batcher.Process(ctx, input)

	// Apply any additional batch processors
	current := batched
	for _, proc := range bs.pipeline {
		current = proc.Process(ctx, current)
	}

	// Process each batch through the sync pipeline
	for batch := range current {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, err := bs.sync.Process(batch)
			if err != nil {
				// For now, return on error
				// Could add error strategies like regular Stream
				return err
			}
		}
	}

	return nil
}

// Name returns the name of this batch stream processor.
func (bs *BatchStream[T]) Name() string {
	return bs.name
}
