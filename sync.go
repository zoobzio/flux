package flux

import (
	"context"

	"github.com/zoobzio/pipz"
)

// NewSync creates a synchronization wrapper for a user callback.
//
// The name parameter identifies the sync in error messages and debugging output.
// The handler function is called for each event processed through this sync.
//
// The handler receives an event containing both data and context. The context
// carries request-scoped values like OpenTelemetry spans, request IDs, and
// deadlines. This ensures proper observability and tracing through the pipeline.
//
// Example:
//
//	sync := flux.NewSync("user-updates", func(event flux.Event[User]) error {
//	    // Context is available from the event
//	    span := trace.SpanFromContext(event.Context)
//	    span.SetAttributes(attribute.String("user.id", event.ID))
//
//	    // Process the user update
//	    fmt.Printf("Processing user %s from %s\n", event.ID, event.Source)
//	    return updateDatabase(event.Context, event.Data)
//	})
func NewSync[T any](name string, handler func(Event[T]) error) *Sync[T] {
	// Wrap to adapt to pipz.Effect signature
	wrapper := func(ctx context.Context, event Event[T]) error {
		// Ensure event has context
		if event.Context == nil {
			event.Context = ctx
		}
		return handler(event)
	}

	return &Sync[T]{
		processor: pipz.Effect[Event[T]](name, wrapper),
	}
}
