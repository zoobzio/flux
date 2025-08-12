/*
Package flux provides synchronization infrastructure for wrapping callbacks
with rate limiting, circuit breaking, retries, and other resilience patterns.

flux is designed to be embedded within services that need to manage synchronization,
not run as a standalone service. It follows a builder pattern similar to zlog,
allowing services to compose synchronization capabilities around their callbacks.

# Basic Usage

Create a sync wrapper with desired capabilities:

	sync := flux.NewSync("processor", handler).
	    WithRateLimit(100, 10).                    // 100 RPS, burst 10
	    WithCircuitBreaker(5, 30*time.Second).     // Open after 5 failures
	    WithRetry(3).                              // Retry up to 3 times
	    WithTimeout(5*time.Second)                 // 5 second timeout

Process events through the sync pipeline:

	event := flux.Event[Data]{
	    ID:        "123",
	    Data:      data,
	    Source:    "service",
	    Timestamp: time.Now(),
	}

	result, err := sync.Process(event)

# Capabilities

Rate Limiting - Control request rates with configurable burst capacity:

	sync.WithRateLimit(100, 10)        // Wait when limited
	sync.WithRateLimitDrop(100, 10)    // Drop when limited

Circuit Breaking - Prevent cascading failures with automatic recovery:

	sync.WithCircuitBreaker(5, 30*time.Second)

Retry Logic - Automatic retries with different strategies:

	sync.WithRetry(3)                          // Immediate retry
	sync.WithBackoff(5, time.Second)           // Exponential backoff

Timeout Protection - Prevent hanging operations:

	sync.WithTimeout(5*time.Second)

Filtering - Process events conditionally:

	sync.WithFilter(func(ctx context.Context, e Event[T]) bool {
	    return e.Source == "important"
	})

# Stream Processing

For continuous event streams:

	stream := flux.NewStream("events", sync).
	    WithThrottle(50.0).      // 50 events/sec max
	    WithBuffer(1000).        // Buffer up to 1000 events
	    WithFilter(predicate)    // Filter at stream level

Process events from a channel:

	errCh := stream.ProcessWithErrors(ctx, eventChannel)

# Integration

flux is typically integrated into services that need synchronization:

	type Service struct {
	    syncs map[string]*flux.Sync[Record]
	}

	func (s *Service) Initialize() {
	    s.syncs["users"] = flux.NewSync("users", s.handleUser).
	        WithRateLimit(100, 10).
	        WithCircuitBreaker(5, 30*time.Second)
	}

The package is built on top of:
  - pipz: For composable data processing pipelines
  - streamz: For channel-based stream processing
*/
package flux
