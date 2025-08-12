package flux

import "github.com/zoobzio/pipz"

// WithRateLimit adds rate limiting capability to the sync.
//
// The sync will limit processing to the specified rate per second with
// the given burst capacity. By default, it will wait for available capacity.
// Use SetMode("drop") on the rate limiter to drop events instead of waiting.
//
// Parameters:
//   - rps: Requests per second (sustained rate)
//   - burst: Maximum burst size above the sustained rate
//
// Example:
//
//	// Limit to 100 requests per second with burst of 10
//	sync := flux.NewSync("api", handler).
//	    WithRateLimit(100, 10)
//
//	// Drop events when rate limited instead of waiting
//	sync := flux.NewSync("api", handler).
//	    WithRateLimitDrop(100, 10)
func (s *Sync[T]) WithRateLimit(rps float64, burst int) *Sync[T] {
	limiter := pipz.NewRateLimiter[Event[T]]("rate-limit", rps, burst)
	return &Sync[T]{
		processor: pipz.NewSequence("rate-limited", limiter, s.processor),
	}
}

// WithRateLimitDrop adds rate limiting that drops events when limited.
//
// Similar to WithRateLimit but configured to drop events immediately
// when the rate limit is exceeded instead of waiting for capacity.
func (s *Sync[T]) WithRateLimitDrop(rps float64, burst int) *Sync[T] {
	limiter := pipz.NewRateLimiter[Event[T]]("rate-limit", rps, burst).SetMode("drop")
	return &Sync[T]{
		processor: pipz.NewSequence("rate-limited", limiter, s.processor),
	}
}
