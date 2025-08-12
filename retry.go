package flux

import (
	"time"

	"github.com/zoobzio/pipz"
)

// WithRetry adds retry capability to the sync.
//
// Failed operations will be retried up to the specified number of attempts.
// Retries are immediate without delay. For exponential backoff between
// attempts, use WithBackoff instead.
//
// The same event is passed to each retry attempt. Retries stop immediately
// if the context is canceled.
//
// Example:
//
//	// Retry up to 3 times on failure
//	sync := flux.NewSync("database", handler).
//	    WithRetry(3)
func (s *Sync[T]) WithRetry(attempts int) *Sync[T] {
	return &Sync[T]{
		processor: pipz.NewRetry("retry", s.processor, attempts),
	}
}

// WithBackoff adds exponential backoff retry to the sync.
//
// Failed operations will be retried with exponentially increasing delays
// between attempts. The delay starts at baseDelay and doubles with each
// retry (capped at 1 minute).
//
// This is preferred over WithRetry for operations that might fail due to
// temporary overload or rate limiting.
//
// Example:
//
//	// Retry 5 times with exponential backoff starting at 1 second
//	sync := flux.NewSync("api", handler).
//	    WithBackoff(5, time.Second)
func (s *Sync[T]) WithBackoff(attempts int, baseDelay time.Duration) *Sync[T] {
	return &Sync[T]{
		processor: pipz.NewBackoff("backoff", s.processor, attempts, baseDelay),
	}
}
