package flux

import (
	"time"

	"github.com/zoobzio/pipz"
)

// WithTimeout adds timeout protection to the sync.
//
// Each event processing will be limited to the specified duration.
// If processing takes longer, it will be canceled and return a timeout error.
// This protects against handlers that hang or take too long.
//
// The timeout applies to the entire processing pipeline, including any
// retries or other capabilities added after the timeout.
//
// Example:
//
//	// Timeout after 5 seconds
//	sync := flux.NewSync("slow-api", handler).
//	    WithTimeout(5 * time.Second)
func (s *Sync[T]) WithTimeout(duration time.Duration) *Sync[T] {
	return &Sync[T]{
		processor: pipz.NewTimeout("timeout", s.processor, duration),
	}
}
