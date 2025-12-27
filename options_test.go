package flux

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/pipz"
)

// Test identities for options tests.
var (
	testDoubleID = pipz.NewIdentity("test:double", "Test double processor")
	testTripleID           = pipz.NewIdentity("test:triple", "Test triple processor")
	testLogID              = pipz.NewIdentity("test:log", "Test log effect")
	testMarkID             = pipz.NewIdentity("test:mark", "Test mark processor")
	testFallbackID         = pipz.NewIdentity("test:fallback", "Test fallback processor")
	testErrorObserverID    = pipz.NewIdentity("test:error-observer", "Test error observer")
	testDoubleIfPositiveID = pipz.NewIdentity("test:double-if-positive", "Test conditional double")
	testFailingEnrichID    = pipz.NewIdentity("test:failing-enrichment", "Test failing enrichment")
	testFlakyID            = pipz.NewIdentity("test:flaky", "Test flaky processor")
	testSlowID             = pipz.NewIdentity("test:slow", "Test slow processor")
	testPrimaryID          = pipz.NewIdentity("test:primary", "Test primary processor")
	testOnlyLargeID        = pipz.NewIdentity("test:only-large", "Test only large filter")
)

// OptionTestConfig is a test config for option tests.
type OptionTestConfig struct {
	Value int `json:"value"`
}

func (c OptionTestConfig) Validate() error {
	if c.Value < 0 {
		return errors.New("value must be non-negative")
	}
	return nil
}

func TestWithRetry_RetriesOnFailure(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var attempts int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			attempts++
			if attempts < 3 {
				return errors.New("transient failure")
			}
			return nil
		},
		WithRetry[OptionTestConfig](3),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}
}

func TestWithRetry_ExhaustsRetries(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var attempts int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			attempts++
			return errors.New("persistent failure")
		},
		WithRetry[OptionTestConfig](3),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithTimeout_EnforcesDeadline(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(ctx context.Context, _, _ OptionTestConfig) error {
			// Simulate slow operation
			select {
			case <-time.After(1 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		WithTimeout[OptionTestConfig](50*time.Millisecond),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWithMiddleware_UseApply_TransformsConfig(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var finalValue int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseApply[OptionTestConfig](testDoubleID, func(_ context.Context, req *Request[OptionTestConfig]) (*Request[OptionTestConfig], error) {
				req.Current.Value *= 2
				return req, nil
			}),
		),
	).SyncMode()

	ch <- []byte(`{"value": 21}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalValue != 42 {
		t.Errorf("expected transformed value 42, got %d", finalValue)
	}
}

func TestWithMiddleware_UseEffect_ExecutesSideEffect(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var effectCalled bool
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			return nil
		},
		WithMiddleware(
			UseEffect[OptionTestConfig](testLogID, func(_ context.Context, _ *Request[OptionTestConfig]) error {
				effectCalled = true
				return nil
			}),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !effectCalled {
		t.Error("expected effect to be called")
	}
}

func TestWithMiddleware_UseTransform_TransformsWithoutError(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var finalValue int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseTransform[OptionTestConfig](testTripleID, func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
				req.Current.Value *= 3
				return req
			}),
		),
	).SyncMode()

	ch <- []byte(`{"value": 14}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalValue != 42 {
		t.Errorf("expected transformed value 42, got %d", finalValue)
	}
}

func TestWithMiddleware_MultipleProcessors_ExecuteInOrder(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var finalValue int
	// Processors execute in order: double first, then triple
	// So: double(7) = 14, triple(14) = 42
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseTransform[OptionTestConfig](testDoubleID, func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
				req.Current.Value *= 2
				return req
			}),
			UseTransform[OptionTestConfig](testTripleID, func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
				req.Current.Value *= 3
				return req
			}),
		),
	).SyncMode()

	ch <- []byte(`{"value": 7}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// double(7) = 14, triple(14) = 42
	if finalValue != 42 {
		t.Errorf("expected transformed value 42, got %d", finalValue)
	}
}

func TestWithCircuitBreaker_OpensAfterFailures(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 10)

	var callbackCalls int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			callbackCalls++
			return errors.New("always fail")
		},
		WithCircuitBreaker[OptionTestConfig](2, 1*time.Hour),
	).SyncMode()

	ch <- []byte(`{"value": 1}`)
	capacitor.Start(ctx) // First failure
	callbackCalls = 0    // Reset

	ch <- []byte(`{"value": 2}`)
	capacitor.Process(ctx) // Second failure - circuit opens

	ch <- []byte(`{"value": 3}`)
	capacitor.Process(ctx) // Should be rejected

	ch <- []byte(`{"value": 4}`)
	capacitor.Process(ctx) // Should be rejected

	// After circuit opens, callback should not be called
	if callbackCalls > 1 {
		t.Errorf("expected at most 1 callback call after circuit opens, got %d", callbackCalls)
	}
}

func TestPipelineAndInstanceConfig(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var transformCalled bool
	var appliedValue int

	// Pipeline options in constructor, instance config via chainable methods
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			appliedValue = cfg.Value
			return nil
		},
		WithMiddleware( // pipeline option
			UseTransform[OptionTestConfig](testMarkID, func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
				transformCalled = true
				return req
			}),
		),
	).SyncMode().Debounce(50 * time.Millisecond)

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !transformCalled {
		t.Error("expected transform to be called")
	}
	if appliedValue != 42 {
		t.Errorf("expected value 42, got %d", appliedValue)
	}
}

func TestWithBackoff_RetriesWithDelay(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var attempts int32
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			atomic.AddInt32(&attempts, 1)
			if atomic.LoadInt32(&attempts) < 2 {
				return errors.New("transient failure")
			}
			return nil
		},
		WithBackoff[OptionTestConfig](3, 1*time.Millisecond),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("expected success after backoff retries, got %v", err)
	}
	if atomic.LoadInt32(&attempts) < 2 {
		t.Errorf("expected at least 2 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestUseRateLimit_ThrottlesRequests(t *testing.T) {
	// Rate limiting test - verify it doesn't block the first request
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			return nil
		},
		WithMiddleware(
			UseRateLimit[OptionTestConfig](100, 10, // 100 per second, burst of 10
				UseTransform[OptionTestConfig](testMarkID, func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
					return req
				}),
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	start := time.Now()
	err := capacitor.Start(ctx)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First request should be immediate (within burst)
	if duration > 100*time.Millisecond {
		t.Errorf("expected immediate processing, took %v", duration)
	}
}

func TestWithFallback_UsesFallbackOnFailure(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var callbackCalled bool
	var fallbackCalled bool

	// WithFallback wraps the pipeline (including callback) as primary.
	// If the primary fails, fallback processors are tried.
	fallback := UseApply[OptionTestConfig](testFallbackID, func(_ context.Context, req *Request[OptionTestConfig]) (*Request[OptionTestConfig], error) {
		fallbackCalled = true
		req.Current.Value = 99
		return req, nil
	})

	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			callbackCalled = true
			return errors.New("callback failed")
		},
		WithFallback(fallback),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !callbackCalled {
		t.Error("expected primary callback to be called first")
	}
	if !fallbackCalled {
		t.Error("expected fallback to be called after primary failed")
	}
	current, _ := capacitor.Current()
	if current.Value != 99 {
		t.Errorf("expected fallback value 99, got %d", current.Value)
	}
}

func TestWithErrorHandler_ObservesErrors(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 2)

	var observedError string
	errorHandler := pipz.Effect(testErrorObserverID, func(_ context.Context, err *pipz.Error[*Request[OptionTestConfig]]) error {
		observedError = err.Err.Error()
		return nil
	})

	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			return errors.New("callback failed")
		},
		WithErrorHandler[OptionTestConfig](errorHandler),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	_ = capacitor.Start(ctx)

	if observedError != "callback failed" {
		t.Errorf("expected observed error 'callback failed', got %q", observedError)
	}
}

func TestUseMutate_ConditionalTransform(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var finalValue int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseMutate[OptionTestConfig](testDoubleIfPositiveID,
				func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
					req.Current.Value *= 2
					return req
				},
				func(_ context.Context, req *Request[OptionTestConfig]) bool {
					return req.Current.Value > 0
				},
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 21}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalValue != 42 {
		t.Errorf("expected 42, got %d", finalValue)
	}
}

func TestUseMutate_SkipsWhenConditionFalse(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var finalValue int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseMutate[OptionTestConfig](testDoubleIfPositiveID,
				func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
					req.Current.Value *= 2
					return req
				},
				func(_ context.Context, req *Request[OptionTestConfig]) bool {
					return req.Current.Value > 100 // condition not met
				},
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalValue != 42 {
		t.Errorf("expected unchanged 42, got %d", finalValue)
	}
}

func TestUseEnrich_ContinuesOnFailure(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var finalValue int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseEnrich[OptionTestConfig](testFailingEnrichID, func(_ context.Context, req *Request[OptionTestConfig]) (*Request[OptionTestConfig], error) {
				return req, errors.New("enrichment failed")
			}),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should continue with original value despite enrichment failure
	if finalValue != 42 {
		t.Errorf("expected 42, got %d", finalValue)
	}
}

func TestUseRetry_InlineRetry(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var attempts int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			return nil
		},
		WithMiddleware(
			UseRetry[OptionTestConfig](3,
				UseApply[OptionTestConfig](testFlakyID, func(_ context.Context, req *Request[OptionTestConfig]) (*Request[OptionTestConfig], error) {
					attempts++
					if attempts < 3 {
						return req, errors.New("transient")
					}
					return req, nil
				}),
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestUseTimeout_InlineTimeout(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			return nil
		},
		WithMiddleware(
			UseTimeout[OptionTestConfig](50*time.Millisecond,
				UseApply[OptionTestConfig](testSlowID, func(ctx context.Context, req *Request[OptionTestConfig]) (*Request[OptionTestConfig], error) {
					select {
					case <-time.After(1 * time.Second):
						return req, nil
					case <-ctx.Done():
						return req, ctx.Err()
					}
				}),
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestUseFallback_InlineFallback(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var finalValue int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseFallback[OptionTestConfig](
				UseApply[OptionTestConfig](testPrimaryID, func(_ context.Context, req *Request[OptionTestConfig]) (*Request[OptionTestConfig], error) {
					return req, errors.New("primary failed")
				}),
				UseApply[OptionTestConfig](testFallbackID, func(_ context.Context, req *Request[OptionTestConfig]) (*Request[OptionTestConfig], error) {
					req.Current.Value = 99
					return req, nil
				}),
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalValue != 99 {
		t.Errorf("expected fallback value 99, got %d", finalValue)
	}
}

func TestUseFilter_SkipsWhenConditionFalse(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var transformCalled bool
	var finalValue int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseFilter[OptionTestConfig](testOnlyLargeID,
				func(_ context.Context, req *Request[OptionTestConfig]) bool {
					return req.Current.Value > 100
				},
				UseTransform[OptionTestConfig](testDoubleID, func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
					transformCalled = true
					req.Current.Value *= 2
					return req
				}),
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transformCalled {
		t.Error("expected transform to be skipped")
	}
	if finalValue != 42 {
		t.Errorf("expected unchanged 42, got %d", finalValue)
	}
}

func TestUseFilter_ExecutesWhenConditionTrue(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var finalValue int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg OptionTestConfig) error {
			finalValue = cfg.Value
			return nil
		},
		WithMiddleware(
			UseFilter[OptionTestConfig](testOnlyLargeID,
				func(_ context.Context, req *Request[OptionTestConfig]) bool {
					return req.Current.Value > 10
				},
				UseTransform[OptionTestConfig](testDoubleID, func(_ context.Context, req *Request[OptionTestConfig]) *Request[OptionTestConfig] {
					req.Current.Value *= 2
					return req
				}),
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 21}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalValue != 42 {
		t.Errorf("expected 42, got %d", finalValue)
	}
}

func TestUseBackoff_RetriesWithExponentialDelay(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var attempts int
	capacitor := New[OptionTestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ OptionTestConfig) error {
			return nil
		},
		WithMiddleware(
			UseBackoff[OptionTestConfig](3, 1*time.Millisecond,
				UseApply[OptionTestConfig](testFlakyID, func(_ context.Context, req *Request[OptionTestConfig]) (*Request[OptionTestConfig], error) {
					attempts++
					if attempts < 2 {
						return req, errors.New("transient")
					}
					return req, nil
				}),
			),
		),
	).SyncMode()

	ch <- []byte(`{"value": 42}`)
	err := capacitor.Start(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts)
	}
}
