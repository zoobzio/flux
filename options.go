package flux

import (
	"context"
	"time"

	"github.com/zoobzio/pipz"
)

// Internal identities for reliability options.
var (
	callbackID       = pipz.NewIdentity("flux:callback", "User callback invocation")
	passthroughID    = pipz.NewIdentity("flux:passthrough", "Composite passthrough")
	retryID          = pipz.NewIdentity("flux:retry", "Retries failed operations")
	backoffID        = pipz.NewIdentity("flux:backoff", "Exponential backoff retry")
	timeoutID        = pipz.NewIdentity("flux:timeout", "Operation timeout")
	fallbackID       = pipz.NewIdentity("flux:fallback", "Fallback alternatives")
	circuitBreakerID = pipz.NewIdentity("flux:circuit-breaker", "Circuit breaker protection")
	errorHandlerID   = pipz.NewIdentity("flux:error-handler", "Error handling")
	middlewareID     = pipz.NewIdentity("flux:middleware", "Middleware sequence")
	rateLimiterID    = pipz.NewIdentity("flux:rate-limiter", "Rate limiting")
)

// Middleware identities for Use* wrapper functions.
var (
	middlewareRetryID    = pipz.NewIdentity("flux:middleware:retry", "Middleware retry")
	middlewareBackoffID  = pipz.NewIdentity("flux:middleware:backoff", "Middleware backoff")
	middlewareTimeoutID  = pipz.NewIdentity("flux:middleware:timeout", "Middleware timeout")
	middlewareFallbackID = pipz.NewIdentity("flux:middleware:fallback", "Middleware fallback")
)

// Option configures the processing pipeline for a Capacitor or CompositeCapacitor.
// Pipeline options wrap the callback with middleware for retry, timeout,
// circuit breaking, and other reliability patterns.
//
// Instance configuration (debounce, sync mode, codec, etc.) is handled via
// chainable methods on the Capacitor/CompositeCapacitor before calling Start().
type Option[T Validator] func(pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]]

// buildPipeline wraps a terminal with pipeline options.
func buildPipeline[T Validator](terminal pipz.Chainable[*Request[T]], opts []Option[T]) pipz.Chainable[*Request[T]] {
	pipeline := terminal
	for _, opt := range opts {
		pipeline = opt(pipeline)
	}
	return pipeline
}

// -----------------------------------------------------------------------------
// Pipeline Options - Wrapping (With*)
// -----------------------------------------------------------------------------
// These options wrap the entire pipeline, providing protection at the boundary.
// Use for resilience patterns that should apply to all processing.

// WithRetry wraps the pipeline with retry logic.
// Failed operations are retried immediately up to maxAttempts times.
// For exponential backoff between retries, use WithBackoff instead.
func WithRetry[T Validator](maxAttempts int) Option[T] {
	return func(p pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
		return pipz.NewRetry(retryID, p, maxAttempts)
	}
}

// WithBackoff wraps the pipeline with exponential backoff retry logic.
// Failed operations are retried with increasing delays: baseDelay, 2*baseDelay, 4*baseDelay, etc.
func WithBackoff[T Validator](maxAttempts int, baseDelay time.Duration) Option[T] {
	return func(p pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
		return pipz.NewBackoff(backoffID, p, maxAttempts, baseDelay)
	}
}

// WithTimeout wraps the pipeline with a timeout.
// If processing takes longer than the specified duration, the operation
// fails with a timeout error.
func WithTimeout[T Validator](d time.Duration) Option[T] {
	return func(p pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
		return pipz.NewTimeout(timeoutID, p, d)
	}
}

// WithFallback wraps the pipeline with fallback processors.
// If the primary pipeline fails, each fallback is tried in order until one succeeds.
func WithFallback[T Validator](fallbacks ...pipz.Chainable[*Request[T]]) Option[T] {
	return func(p pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
		all := append([]pipz.Chainable[*Request[T]]{p}, fallbacks...)
		return pipz.NewFallback(fallbackID, all...)
	}
}

// WithCircuitBreaker wraps the pipeline with circuit breaker protection.
// After 'failures' consecutive failures, the circuit opens and rejects
// further requests until 'recovery' time has passed.
//
// The circuit breaker has three states:
//   - Closed: Normal operation, requests pass through
//   - Open: After threshold failures, requests are rejected immediately
//   - Half-Open: After recovery timeout, one request is allowed to test recovery
//
// Note: Circuit breaker is stateful and protects the entire pipeline.
// There is no Use* equivalent - it only makes sense as a wrapper.
func WithCircuitBreaker[T Validator](failures int, recovery time.Duration) Option[T] {
	return func(p pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
		return pipz.NewCircuitBreaker(circuitBreakerID, p, failures, recovery)
	}
}

// WithErrorHandler adds error observation to the pipeline.
// Errors are passed to the handler for logging, metrics, or alerting,
// but the error still propagates. Use this for observability, not recovery.
//
// Note: There is no Use* equivalent - error handling wraps the pipeline.
func WithErrorHandler[T Validator](handler pipz.Chainable[*pipz.Error[*Request[T]]]) Option[T] {
	return func(p pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
		return pipz.NewHandle(errorHandlerID, p, handler)
	}
}

// WithPipeline wraps the entire processing pipeline with a pipz.Pipeline for
// correlated tracing. Each Process() call generates a unique execution ID,
// while the pipeline ID remains stable (derived from the identity).
//
// Use pipz.ExecutionIDFromContext and pipz.PipelineIDFromContext in middleware
// or signal handlers to extract correlation IDs for observability.
//
// This option should typically be applied last (outermost) to ensure all
// nested processors have access to the correlation context.
//
// Example:
//
//	var configPipelineID = pipz.NewIdentity("myapp:config", "Configuration pipeline")
//
//	capacitor := flux.New[Config](
//	    watcher,
//	    callback,
//	    flux.WithRetry[Config](3),
//	    flux.WithPipeline[Config](configPipelineID),
//	)
func WithPipeline[T Validator](identity pipz.Identity) Option[T] {
	return func(p pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
		return pipz.NewPipeline(identity, p)
	}
}

// -----------------------------------------------------------------------------
// Pipeline Options - Middleware Composition
// -----------------------------------------------------------------------------

// WithMiddleware wraps the pipeline with a sequence of processors.
// Processors execute in order, with the wrapped pipeline (callback) last.
//
// Use the Use* functions to create processors for common patterns,
// or provide custom pipz.Chainable implementations directly.
//
// Example:
//
//	flux.New[Config](
//	    watcher,
//	    callback,
//	    flux.WithMiddleware(
//	        flux.UseEffect[Config]("log", logFn),
//	        flux.UseApply[Config]("enrich", enrichFn),
//	        flux.UseRateLimit[Config](10, 5),
//	    ),
//	    flux.WithCircuitBreaker[Config](5, 30*time.Second),
//	).Debounce(200 * time.Millisecond)
func WithMiddleware[T Validator](processors ...pipz.Chainable[*Request[T]]) Option[T] {
	return func(p pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
		all := make([]pipz.Chainable[*Request[T]], 0, len(processors)+1)
		all = append(all, processors...)
		all = append(all, p)
		return pipz.NewSequence(middlewareID, all...)
	}
}

// -----------------------------------------------------------------------------
// Middleware Processors - Adapters (Use*)
// -----------------------------------------------------------------------------
// These create processors for use inside WithMiddleware.
// They transform or observe the request as it flows through the pipeline.

// UseTransform creates a processor that transforms the request.
// Cannot fail. Use for pure transformations that always succeed.
func UseTransform[T Validator](identity pipz.Identity, fn func(context.Context, *Request[T]) *Request[T]) pipz.Chainable[*Request[T]] {
	return pipz.Transform(identity, fn)
}

// UseApply creates a processor that can transform the request and fail.
// Use for operations like enrichment, validation, or transformation
// that may produce errors.
func UseApply[T Validator](identity pipz.Identity, fn func(context.Context, *Request[T]) (*Request[T], error)) pipz.Chainable[*Request[T]] {
	return pipz.Apply(identity, fn)
}

// UseEffect creates a processor that performs a side effect.
// The request passes through unchanged. Use for logging, metrics,
// or notifications that should not affect the configuration value.
func UseEffect[T Validator](identity pipz.Identity, fn func(context.Context, *Request[T]) error) pipz.Chainable[*Request[T]] {
	return pipz.Effect(identity, fn)
}

// UseMutate creates a processor that conditionally transforms the request.
// The transformer is only applied if the condition returns true.
func UseMutate[T Validator](identity pipz.Identity, transformer func(context.Context, *Request[T]) *Request[T], condition func(context.Context, *Request[T]) bool) pipz.Chainable[*Request[T]] {
	return pipz.Mutate(identity, transformer, condition)
}

// UseEnrich creates a processor that attempts optional enhancement.
// If the enrichment fails, the error is logged but processing continues
// with the original request. Use for non-critical enhancements.
func UseEnrich[T Validator](identity pipz.Identity, fn func(context.Context, *Request[T]) (*Request[T], error)) pipz.Chainable[*Request[T]] {
	return pipz.Enrich(identity, fn)
}

// -----------------------------------------------------------------------------
// Middleware Processors - Wrapping (Use*)
// -----------------------------------------------------------------------------
// These wrap another processor with reliability logic.

// UseRetry wraps a processor with retry logic.
// Failed operations are retried immediately up to maxAttempts times.
func UseRetry[T Validator](maxAttempts int, processor pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
	return pipz.NewRetry(middlewareRetryID, processor, maxAttempts)
}

// UseBackoff wraps a processor with exponential backoff retry logic.
// Failed operations are retried with increasing delays.
func UseBackoff[T Validator](maxAttempts int, baseDelay time.Duration, processor pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
	return pipz.NewBackoff(middlewareBackoffID, processor, maxAttempts, baseDelay)
}

// UseTimeout wraps a processor with a deadline.
// If processing takes longer than the specified duration, the operation fails.
func UseTimeout[T Validator](d time.Duration, processor pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
	return pipz.NewTimeout(middlewareTimeoutID, processor, d)
}

// UseFallback wraps a processor with fallback alternatives.
// If the primary fails, each fallback is tried in order.
func UseFallback[T Validator](primary pipz.Chainable[*Request[T]], fallbacks ...pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
	all := append([]pipz.Chainable[*Request[T]]{primary}, fallbacks...)
	return pipz.NewFallback(middlewareFallbackID, all...)
}

// UseFilter wraps a processor with a condition.
// If the condition returns false, the request passes through unchanged.
func UseFilter[T Validator](identity pipz.Identity, condition func(context.Context, *Request[T]) bool, processor pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
	return pipz.NewFilter(identity, condition, processor)
}

// UseRateLimit wraps a processor with rate limiting.
// Uses a token bucket algorithm with the specified rate (tokens per second)
// and burst size. When tokens are exhausted, requests wait for availability.
func UseRateLimit[T Validator](rate float64, burst int, processor pipz.Chainable[*Request[T]]) pipz.Chainable[*Request[T]] {
	return pipz.NewRateLimiter(rateLimiterID, rate, burst, processor)
}
