package flux

import (
	"context"

	"github.com/zoobzio/pipz"
)

// WithFilter adds conditional processing to the sync.
//
// Events will only be processed if they pass the predicate function.
// Events that don't match are silently skipped without calling the handler.
// This is useful for creating syncs that only process specific types of events.
//
// The predicate receives the full event and should return true to process
// or false to skip. Filtering happens before any other processing.
//
// Example:
//
//	// Only process events from specific source
//	sync := flux.NewSync("filtered", handler).
//	    WithFilter(func(e flux.Event[Data]) bool {
//	        return e.Source == "important-source"
//	    })
//
//	// Only process recent events
//	sync := flux.NewSync("recent", handler).
//	    WithFilter(func(e flux.Event[Data]) bool {
//	        return time.Since(e.Timestamp) < 5*time.Minute
//	    })
//
//	// Filter based on context values
//	sync := flux.NewSync("tenant", handler).
//	    WithFilter(func(e flux.Event[Data]) bool {
//	        tenantID := e.Context.Value("tenant-id")
//	        return tenantID == "premium-tenant"
//	    })
func (s *Sync[T]) WithFilter(predicate func(Event[T]) bool) *Sync[T] {
	// Wrap to include context
	wrapper := func(ctx context.Context, event Event[T]) bool {
		if event.Context == nil {
			event.Context = ctx
		}
		return predicate(event)
	}

	return &Sync[T]{
		processor: pipz.NewFilter("filter", wrapper, s.processor),
	}
}
