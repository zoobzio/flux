package flux

import "github.com/zoobzio/pipz"

// WithErrorHandler adds an error processing pipeline.
//
// When the main processor returns an error, the error handler pipeline
// will be executed with error details. This is useful for logging,
// alerting, or sending to dead letter queues.
//
// The error handler receives a *pipz.Error[Event[T]] containing the
// original event and error information. It still returns the original
// error from the main processor.
//
// Example:
//
//	// Create error handler that processes pipz.Error
//	errorHandler := pipz.Effect("error-logger", func(ctx context.Context, err *pipz.Error[flux.Event[Data]]) error {
//	    log.Printf("Failed to process event %s: %v", err.InputData.ID, err.Err)
//	    // Send to dead letter queue
//	    return sendToDeadLetter(err.InputData)
//	})
//
//	sync := flux.NewSync("main", processEvent).
//	    WithErrorHandler(errorHandler)
func (s *Sync[T]) WithErrorHandler(errorHandler pipz.Chainable[*pipz.Error[Event[T]]]) *Sync[T] {
	return &Sync[T]{
		processor: pipz.NewHandle("error-handler", s.processor, errorHandler),
	}
}
