package flux

import "github.com/zoobzio/pipz"

// WithFallback adds a fallback handler for when the primary fails.
//
// If the primary handler returns an error, the fallback handler will be
// attempted with the same event. This is useful for implementing backup
// strategies or degraded functionality.
//
// Example:
//
//	// Try primary handler, fall back to secondary on failure
//	primary := flux.NewSync("primary", primaryHandler)
//	fallback := flux.NewSync("fallback", backupHandler)
//
//	sync := primary.WithFallback(fallback)
func (s *Sync[T]) WithFallback(fallback *Sync[T]) *Sync[T] {
	return &Sync[T]{
		processor: pipz.NewFallback("fallback", s.processor, fallback.processor),
	}
}
