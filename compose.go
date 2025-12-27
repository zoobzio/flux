package flux

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/clockz"
	"github.com/zoobzio/pipz"
)

// SourceError represents an error from a specific source in a CompositeCapacitor.
type SourceError struct {
	Index int
	Error error
}

// CompositeCapacitor watches multiple sources, unmarshals and validates each,
// and delivers the slice of parsed values to a reducer for merging.
type CompositeCapacitor[T Validator] struct {
	sources        []Watcher
	reducer        Reducer[T]
	pipeline       pipz.Chainable[*Request[T]]
	debounce       time.Duration
	startupTimeout time.Duration
	syncMode       bool
	clock          clockz.Clock
	codec          Codec
	metrics        MetricsProvider
	onStop         func(State)

	state        atomic.Int32
	current      atomic.Pointer[T]
	lastError    atomic.Pointer[error]
	errorHistory *errorRing
	sourceErrors atomic.Pointer[[]SourceError]

	mu      sync.Mutex
	started bool

	// For sync mode
	sourceChans []<-chan []byte
	latest      [][]byte
	ready       []bool

	// Track parsed values for old/new comparison
	latestParsed atomic.Pointer[[]T]
}

// Compose creates a CompositeCapacitor for multiple sources.
//
// Each source emits raw bytes when it changes.
// Bytes from each source are automatically unmarshaled to type T using the configured codec.
// Each parsed value is validated by calling T.Validate().
// When all sources are ready, the reducer receives the previous and new slices of parsed values
// in the same order as the sources. On initial load, prev will be nil.
// The reducer merges them and returns the final configuration.
//
// Pipeline options (With*) configure the processing pipeline. Instance
// configuration uses chainable methods before calling Start().
//
// Example:
//
//	type Config struct {
//	    Port    int `json:"port"`
//	    Timeout int `json:"timeout"`
//	}
//
//	func (c Config) Validate() error {
//	    if c.Port < 1 || c.Port > 65535 {
//	        return errors.New("port must be between 1 and 65535")
//	    }
//	    return nil
//	}
//
//	capacitor := flux.Compose[Config](
//	    func(ctx context.Context, prev, curr []Config) (Config, error) {
//	        merged := curr[0]  // defaults
//	        if curr[1].Port != 0 {
//	            merged.Port = curr[1].Port  // file overrides
//	        }
//	        return merged, nil
//	    },
//	    []flux.Watcher{defaultsWatcher, fileWatcher},
//	).Debounce(200 * time.Millisecond)
func Compose[T Validator](
	reducer Reducer[T],
	sources []Watcher,
	opts ...Option[T],
) *CompositeCapacitor[T] {
	// Create a passthrough terminal for CompositeCapacitor
	// The actual reducer is called in process() before the pipeline
	terminal := pipz.Transform(passthroughID, func(_ context.Context, req *Request[T]) *Request[T] {
		return req
	})
	pipeline := buildPipeline(terminal, opts)

	c := &CompositeCapacitor[T]{
		sources:      sources,
		reducer:      reducer,
		pipeline:     pipeline,
		debounce:     DefaultDebounce,
		clock:        clockz.RealClock,
		codec:        JSONCodec{},
		errorHistory: newErrorRing(0),
		latest:       make([][]byte, len(sources)),
		ready:        make([]bool, len(sources)),
	}
	c.state.Store(int32(StateLoading))

	return c
}

// -----------------------------------------------------------------------------
// Chainable Instance Configuration
// -----------------------------------------------------------------------------

// Debounce sets the debounce duration for change processing.
// Changes arriving within this duration are coalesced into a single update.
// Default: 100ms. Must be called before Start().
func (c *CompositeCapacitor[T]) Debounce(d time.Duration) *CompositeCapacitor[T] {
	c.debounce = d
	return c
}

// SyncMode enables synchronous processing for testing.
// In sync mode, changes are processed immediately without debouncing
// or async goroutines, making tests deterministic. Must be called before Start().
func (c *CompositeCapacitor[T]) SyncMode() *CompositeCapacitor[T] {
	c.syncMode = true
	return c
}

// Clock sets a custom clock for time operations.
// Use this with clockz.FakeClock for deterministic debounce testing.
// Must be called before Start().
func (c *CompositeCapacitor[T]) Clock(clock clockz.Clock) *CompositeCapacitor[T] {
	c.clock = clock
	return c
}

// Codec sets the codec for deserializing configuration data.
// Default: JSONCodec. Must be called before Start().
func (c *CompositeCapacitor[T]) Codec(codec Codec) *CompositeCapacitor[T] {
	c.codec = codec
	return c
}

// StartupTimeout sets the maximum duration to wait for the initial
// configuration value from each source. If any source fails to emit
// within this duration, Start() returns an error.
// Default: no timeout (wait indefinitely). Must be called before Start().
func (c *CompositeCapacitor[T]) StartupTimeout(d time.Duration) *CompositeCapacitor[T] {
	c.startupTimeout = d
	return c
}

// Metrics sets a metrics provider for observability integration.
// The provider receives callbacks on state changes, processing success/failure,
// and change events. Must be called before Start().
func (c *CompositeCapacitor[T]) Metrics(provider MetricsProvider) *CompositeCapacitor[T] {
	c.metrics = provider
	return c
}

// OnStop sets a callback that is invoked when the capacitor stops watching.
// The callback receives the final state of the capacitor. This is useful for
// graceful shutdown scenarios where cleanup is needed. Must be called before Start().
func (c *CompositeCapacitor[T]) OnStop(fn func(State)) *CompositeCapacitor[T] {
	c.onStop = fn
	return c
}

// ErrorHistorySize sets the number of recent errors to retain.
// When set, ErrorHistory() returns up to this many recent errors.
// Use 0 (default) to only retain the most recent error via LastError().
// Must be called before Start().
func (c *CompositeCapacitor[T]) ErrorHistorySize(n int) *CompositeCapacitor[T] {
	c.errorHistory = newErrorRing(n)
	return c
}

// State returns the current state of the CompositeCapacitor.
func (c *CompositeCapacitor[T]) State() State {
	return State(c.state.Load())
}

// Current returns the last successfully merged configuration.
// Unlike the single-source Capacitor, this returns the value produced by
// the reducer function, representing the merged result of all sources.
func (c *CompositeCapacitor[T]) Current() (T, bool) {
	ptr := c.current.Load()
	if ptr == nil {
		var zero T
		return zero, false
	}
	return *ptr, true
}

// LastError returns the last error encountered.
func (c *CompositeCapacitor[T]) LastError() error {
	ptr := c.lastError.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// ErrorHistory returns the recent error history, oldest first.
// Returns nil if error history is not enabled (see WithErrorHistory).
func (c *CompositeCapacitor[T]) ErrorHistory() []error {
	return c.errorHistory.all()
}

// SourceErrors returns errors from individual sources, if any.
// This provides granular insight into which sources are failing.
func (c *CompositeCapacitor[T]) SourceErrors() []SourceError {
	ptr := c.sourceErrors.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// Start begins watching all sources.
func (c *CompositeCapacitor[T]) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return fmt.Errorf("capacitor already started")
	}
	c.started = true
	c.mu.Unlock()

	if len(c.sources) == 0 {
		return fmt.Errorf("compose requires at least one source")
	}

	capitan.Emit(ctx, CapacitorStarted,
		KeyDebounce.Field(c.debounce),
	)

	// Start all source watchers
	c.sourceChans = make([]<-chan []byte, len(c.sources))
	for i, src := range c.sources {
		ch, err := src.Watch(ctx)
		if err != nil {
			return fmt.Errorf("failed to start source %d: %w", i, err)
		}
		c.sourceChans[i] = ch
	}

	// Wait for initial value from each source
	// Wrap context with startup timeout if configured
	startupCtx := ctx
	if c.startupTimeout > 0 {
		var cancel context.CancelFunc
		startupCtx, cancel = c.clock.WithTimeout(ctx, c.startupTimeout)
		defer cancel()
	}

	for i, ch := range c.sourceChans {
		select {
		case <-startupCtx.Done():
			if c.startupTimeout > 0 && startupCtx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("startup timeout: source %d did not emit initial value within %v", i, c.startupTimeout)
			}
			return startupCtx.Err()
		case raw, ok := <-ch:
			if !ok {
				return fmt.Errorf("source %d closed before emitting initial value", i)
			}
			c.latest[i] = raw
			c.ready[i] = true
		}
	}

	capitan.Emit(ctx, CapacitorChangeReceived)
	if c.metrics != nil {
		c.metrics.OnChangeReceived()
	}

	// Process initial merged value
	initialErr := c.process(ctx)

	if c.syncMode {
		return initialErr
	}

	// Continue watching asynchronously
	go c.watch(ctx)

	return initialErr
}

// Process manually processes pending changes in sync mode.
func (c *CompositeCapacitor[T]) Process(ctx context.Context) bool {
	if !c.syncMode {
		return false
	}

	// Check each source for new value (non-blocking)
	changed := false
	for i, ch := range c.sourceChans {
		select {
		case raw, ok := <-ch:
			if !ok {
				continue
			}
			c.latest[i] = raw
			changed = true
		default:
		}
	}

	if changed {
		capitan.Emit(ctx, CapacitorChangeReceived)
		if c.metrics != nil {
			c.metrics.OnChangeReceived()
		}
		_ = c.process(ctx) //nolint:errcheck // Errors stored via setError
		return true
	}
	return false
}

// process unmarshals each source, validates, calls reducer, and runs pipeline.
func (c *CompositeCapacitor[T]) process(ctx context.Context) error {
	start := c.clock.Now()
	oldState := c.State()

	// Unmarshal and validate each source, collecting any errors
	results := make([]T, len(c.latest))
	var sourceErrs []SourceError

	for i, raw := range c.latest {
		var result T
		if err := c.codec.Unmarshal(raw, &result); err != nil {
			sourceErrs = append(sourceErrs, SourceError{Index: i, Error: err})
			c.setError(err)
			c.setSourceErrors(sourceErrs)
			c.transitionState(ctx, oldState, c.failureState())
			capitan.Emit(ctx, CapacitorTransformFailed,
				KeyError.Field(fmt.Sprintf("source %d: %s", i, err.Error())),
			)
			if c.metrics != nil {
				c.metrics.OnProcessFailure("unmarshal", c.clock.Since(start))
			}
			return fmt.Errorf("unmarshal source %d failed: %w", i, err)
		}

		if err := result.Validate(); err != nil {
			sourceErrs = append(sourceErrs, SourceError{Index: i, Error: err})
			c.setError(err)
			c.setSourceErrors(sourceErrs)
			c.transitionState(ctx, oldState, c.failureState())
			capitan.Emit(ctx, CapacitorValidationFailed,
				KeyError.Field(fmt.Sprintf("source %d: %s", i, err.Error())),
			)
			if c.metrics != nil {
				c.metrics.OnProcessFailure("validate", c.clock.Since(start))
			}
			return fmt.Errorf("validation source %d failed: %w", i, err)
		}

		results[i] = result
	}

	// Get previous parsed values for reducer (nil on first call)
	var prev []T
	if ptr := c.latestParsed.Load(); ptr != nil {
		prev = *ptr
	}

	// Reduce all parsed values to final merged config
	merged, err := c.reducer(ctx, prev, results)
	if err != nil {
		c.setError(err)
		c.transitionState(ctx, oldState, c.failureState())
		capitan.Emit(ctx, CapacitorApplyFailed,
			KeyError.Field(err.Error()),
		)
		if c.metrics != nil {
			c.metrics.OnProcessFailure("reducer", c.clock.Since(start))
		}
		return fmt.Errorf("reducer failed: %w", err)
	}

	// Get previous merged value for pipeline
	var prevMerged T
	if ptr := c.current.Load(); ptr != nil {
		prevMerged = *ptr
	}

	// Build request and process through pipeline
	req := &Request[T]{Previous: prevMerged, Current: merged}
	processed, err := c.pipeline.Process(ctx, req)
	if err != nil {
		c.setError(err)
		c.transitionState(ctx, oldState, c.failureState())
		capitan.Emit(ctx, CapacitorApplyFailed,
			KeyError.Field(err.Error()),
		)
		if c.metrics != nil {
			c.metrics.OnProcessFailure("pipeline", c.clock.Since(start))
		}
		return fmt.Errorf("pipeline failed: %w", err)
	}

	// Success - store merged result
	c.current.Store(&processed.Current)
	c.latestParsed.Store(&results)
	c.lastError.Store(nil)
	c.errorHistory.clear()
	c.sourceErrors.Store(nil)
	c.transitionState(ctx, oldState, StateHealthy)
	capitan.Emit(ctx, CapacitorApplySucceeded)
	if c.metrics != nil {
		c.metrics.OnProcessSuccess(c.clock.Since(start))
	}

	return nil
}

func (c *CompositeCapacitor[T]) failureState() State {
	if c.current.Load() == nil {
		return StateEmpty
	}
	return StateDegraded
}

func (c *CompositeCapacitor[T]) transitionState(ctx context.Context, oldState, newState State) {
	if oldState == newState {
		return
	}
	c.state.Store(int32(newState))
	capitan.Emit(ctx, CapacitorStateChanged,
		KeyOldState.Field(oldState.String()),
		KeyNewState.Field(newState.String()),
	)
	if c.metrics != nil {
		c.metrics.OnStateChange(oldState, newState)
	}
}

func (c *CompositeCapacitor[T]) setError(err error) {
	e := err
	c.lastError.Store(&e)
	c.errorHistory.push(err)
}

func (c *CompositeCapacitor[T]) setSourceErrors(errs []SourceError) {
	c.sourceErrors.Store(&errs)
}

// watch processes changes from all sources with debouncing.
func (c *CompositeCapacitor[T]) watch(ctx context.Context) {
	defer func() {
		finalState := c.State()
		capitan.Emit(ctx, CapacitorStopped,
			KeyState.Field(finalState.String()),
		)
		if c.onStop != nil {
			c.onStop(finalState)
		}
	}()

	// Fan-in channel: source goroutines signal when data arrives
	changed := make(chan int, len(c.sourceChans))

	// Start a goroutine for each source
	var wg sync.WaitGroup
	wg.Add(len(c.sourceChans))

	for i, ch := range c.sourceChans {
		go func(idx int, ch <-chan []byte) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case raw, ok := <-ch:
					if !ok {
						return
					}
					c.latest[idx] = raw
					select {
					case changed <- idx:
					case <-ctx.Done():
						return
					}
				}
			}
		}(i, ch)
	}

	// Single goroutine handles debouncing and processing
	go func() {
		var (
			timer      clockz.Timer
			timerC     <-chan time.Time
			hasPending bool
		)

		for {
			select {
			case <-ctx.Done():
				if timer != nil {
					timer.Stop()
				}
				return

			case <-changed:
				capitan.Emit(ctx, CapacitorChangeReceived)
				if c.metrics != nil {
					c.metrics.OnChangeReceived()
				}
				hasPending = true

				// Reset or start debounce timer
				if timer == nil {
					timer = c.clock.NewTimer(c.debounce)
					timerC = timer.C()
				} else {
					if !timer.Stop() {
						select {
						case <-timerC:
						default:
						}
					}
					timer.Reset(c.debounce)
				}

			case <-timerC:
				if hasPending {
					_ = c.process(ctx) //nolint:errcheck // Errors stored via setError
					hasPending = false
				}
			}
		}
	}()

	wg.Wait()
	close(changed)
}
