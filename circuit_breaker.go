package flux

import (
	"time"

	"github.com/zoobzio/pipz"
)

// WithCircuitBreaker adds circuit breaker protection to the sync.
//
// The circuit breaker prevents cascading failures by stopping requests
// to failing handlers. After the threshold number of consecutive failures,
// the circuit opens and all requests fail immediately. After the timeout
// period, the circuit enters half-open state to test if the handler has
// recovered.
//
// Parameters:
//   - threshold: Consecutive failures before opening circuit
//   - timeout: How long to wait before attempting recovery
//
// Example:
//
//	// Open circuit after 5 consecutive failures, try recovery after 30s
//	sync := flux.NewSync("external-api", handler).
//	    WithCircuitBreaker(5, 30*time.Second)
func (s *Sync[T]) WithCircuitBreaker(threshold int, timeout time.Duration) *Sync[T] {
	return &Sync[T]{
		processor: pipz.NewCircuitBreaker("circuit-breaker", s.processor, threshold, timeout),
	}
}
