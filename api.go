// Package flux provides reactive configuration synchronization primitives.
//
// The core type is Capacitor, which watches external sources for changes,
// deserializes and validates the data, and delivers it to application code
// with automatic rollback on failure.
//
// # Capacitor
//
// A Capacitor monitors a source for changes and processes them through a
// pipeline:
//
//	Source → Deserialize → Validate → Pipeline → Store
//
// If any step fails, the previous valid configuration is retained and
// the Capacitor enters a degraded state while continuing to watch for
// valid updates.
//
// # Validation
//
// Configuration types must implement the Validator interface:
//
//	type Validator interface {
//	    Validate() error
//	}
//
// This gives full control over validation logic. For simple cases, you can
// delegate to a validation library like go-playground/validator within your
// Validate method.
//
// # State Machine
//
// Capacitor maintains one of four states:
//
//   - Loading: Initial state, no config yet
//   - Healthy: Valid config applied
//   - Degraded: Last change failed, previous config still active
//   - Empty: Initial load failed, no valid config ever obtained
//
// # Watchers
//
// The Watcher interface abstracts change sources. The core package provides
// ChannelWatcher for testing. Additional watchers are available in pkg/:
//
//   - pkg/file: File watcher using fsnotify
//   - pkg/redis: Redis keyspace notifications
//   - pkg/consul: Consul blocking queries
//   - pkg/etcd: etcd Watch API
//   - pkg/nats: NATS JetStream KV
//   - pkg/kubernetes: ConfigMap/Secret watch
//   - pkg/zookeeper: ZooKeeper node watch
//   - pkg/firestore: Firestore realtime listeners
//
// # Example
//
//	type AppConfig struct {
//	    Port int    `json:"port"`
//	    Host string `json:"host"`
//	}
//
//	func (c AppConfig) Validate() error {
//	    if c.Port < 1 || c.Port > 65535 {
//	        return errors.New("port must be between 1 and 65535")
//	    }
//	    if c.Host == "" {
//	        return errors.New("host is required")
//	    }
//	    return nil
//	}
//
//	capacitor := flux.New[AppConfig](
//	    file.New("/etc/myapp/config.json"),
//	    func(ctx context.Context, prev, curr AppConfig) error {
//	        log.Printf("config changed: %+v -> %+v", prev, curr)
//	        return app.Reconfigure(curr)
//	    },
//	)
//
//	if err := capacitor.Start(ctx); err != nil {
//	    log.Printf("initial config failed: %v", err)
//	}
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

// DefaultDebounce is the default debounce duration for change processing.
const DefaultDebounce = 100 * time.Millisecond

// Validator is the interface that configuration types must implement.
// This allows users to define their own validation logic.
type Validator interface {
	Validate() error
}

// Watcher observes a source for changes and emits raw bytes on a channel.
// Implementations must emit the current value immediately upon Watch() being
// called to support initial configuration loading.
type Watcher interface {
	// Watch begins observing the source and returns a channel that emits
	// raw bytes when changes occur. The channel is closed when the context
	// is canceled or an unrecoverable error occurs.
	//
	// Implementations should emit the current value immediately to support
	// initial configuration loading.
	Watch(ctx context.Context) (<-chan []byte, error)
}

// Capacitor watches a source for changes, unmarshals and validates the data,
// and delivers it to application code with automatic rollback on failure.
type Capacitor[T Validator] struct {
	watcher        Watcher
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

	mu      sync.Mutex
	started bool

	// For sync mode: channel to receive changes
	changes <-chan []byte
}

// New creates a Capacitor that watches a source for configuration changes.
//
// The watcher emits raw bytes when the source changes. Bytes are automatically
// unmarshaled to type T using the configured codec. The struct is validated by
// calling T.Validate(). On success, the callback is invoked with previous and
// current values.
//
// Pipeline options (With*) configure the processing pipeline. Instance
// configuration uses chainable methods before calling Start().
//
// Example:
//
//	capacitor := flux.New[Config](
//	    file.New("config.json"),
//	    func(ctx context.Context, prev, curr Config) error {
//	        log.Printf("config changed: port %d -> %d", prev.Port, curr.Port)
//	        return nil
//	    },
//	    flux.WithRetry(3),
//	).Debounce(200 * time.Millisecond)
func New[T Validator](
	watcher Watcher,
	fn func(ctx context.Context, prev, curr T) error,
	opts ...Option[T],
) *Capacitor[T] {
	terminal := pipz.Effect(callbackID, func(ctx context.Context, req *Request[T]) error {
		return fn(ctx, req.Previous, req.Current)
	})
	pipeline := buildPipeline(terminal, opts)

	c := &Capacitor[T]{
		watcher:      watcher,
		pipeline:     pipeline,
		debounce:     DefaultDebounce,
		clock:        clockz.RealClock,
		codec:        JSONCodec{},
		errorHistory: newErrorRing(0),
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
func (c *Capacitor[T]) Debounce(d time.Duration) *Capacitor[T] {
	c.debounce = d
	return c
}

// SyncMode enables synchronous processing for testing.
// In sync mode, changes are processed immediately without debouncing
// or async goroutines, making tests deterministic. Must be called before Start().
func (c *Capacitor[T]) SyncMode() *Capacitor[T] {
	c.syncMode = true
	return c
}

// Clock sets a custom clock for time operations.
// Use this with clockz.FakeClock for deterministic debounce testing.
// Must be called before Start().
func (c *Capacitor[T]) Clock(clock clockz.Clock) *Capacitor[T] {
	c.clock = clock
	return c
}

// Codec sets the codec for deserializing configuration data.
// Default: JSONCodec. Must be called before Start().
func (c *Capacitor[T]) Codec(codec Codec) *Capacitor[T] {
	c.codec = codec
	return c
}

// StartupTimeout sets the maximum duration to wait for the initial
// configuration value from the watcher. If the watcher fails to emit
// within this duration, Start() returns an error.
// Default: no timeout (wait indefinitely). Must be called before Start().
func (c *Capacitor[T]) StartupTimeout(d time.Duration) *Capacitor[T] {
	c.startupTimeout = d
	return c
}

// Metrics sets a metrics provider for observability integration.
// The provider receives callbacks on state changes, processing success/failure,
// and change events. Must be called before Start().
func (c *Capacitor[T]) Metrics(provider MetricsProvider) *Capacitor[T] {
	c.metrics = provider
	return c
}

// OnStop sets a callback that is invoked when the capacitor stops watching.
// The callback receives the final state of the capacitor. This is useful for
// graceful shutdown scenarios where cleanup is needed. Must be called before Start().
func (c *Capacitor[T]) OnStop(fn func(State)) *Capacitor[T] {
	c.onStop = fn
	return c
}

// ErrorHistorySize sets the number of recent errors to retain.
// When set, ErrorHistory() returns up to this many recent errors.
// Use 0 (default) to only retain the most recent error via LastError().
// Must be called before Start().
func (c *Capacitor[T]) ErrorHistorySize(n int) *Capacitor[T] {
	c.errorHistory = newErrorRing(n)
	return c
}

// State returns the current state of the Capacitor.
func (c *Capacitor[T]) State() State {
	return State(c.state.Load())
}

// Current returns the current valid configuration and true, or the zero value
// and false if no valid configuration has been applied.
func (c *Capacitor[T]) Current() (T, bool) {
	ptr := c.current.Load()
	if ptr == nil {
		var zero T
		return zero, false
	}
	return *ptr, true
}

// LastError returns the last error encountered, or nil if no error occurred.
func (c *Capacitor[T]) LastError() error {
	ptr := c.lastError.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// ErrorHistory returns the recent error history, oldest first.
// Returns nil if error history is not enabled (see WithErrorHistory).
func (c *Capacitor[T]) ErrorHistory() []error {
	return c.errorHistory.all()
}

// Start begins watching for changes. It blocks until the first configuration
// is processed (success or failure), then continues watching asynchronously.
//
// If the initial configuration fails, Start returns the error but continues
// watching in the background for valid updates.
//
// In sync mode, Start only processes the initial value. Use Process() to
// manually trigger processing of subsequent values.
//
// Start can only be called once. Subsequent calls return an error.
func (c *Capacitor[T]) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return fmt.Errorf("capacitor already started")
	}
	c.started = true
	c.mu.Unlock()

	capitan.Emit(ctx, CapacitorStarted,
		KeyDebounce.Field(c.debounce),
	)

	changes, err := c.watcher.Watch(ctx)
	if err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	// Wait for first value and process synchronously
	var initialErr error

	// Wrap context with startup timeout if configured
	startupCtx := ctx
	if c.startupTimeout > 0 {
		var cancel context.CancelFunc
		startupCtx, cancel = c.clock.WithTimeout(ctx, c.startupTimeout)
		defer cancel()
	}

	select {
	case <-startupCtx.Done():
		if c.startupTimeout > 0 && startupCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("startup timeout: watcher did not emit initial value within %v", c.startupTimeout)
		}
		return startupCtx.Err()
	case raw, ok := <-changes:
		if !ok {
			return fmt.Errorf("watcher closed before emitting initial value")
		}
		capitan.Emit(ctx, CapacitorChangeReceived)
		if c.metrics != nil {
			c.metrics.OnChangeReceived()
		}
		initialErr = c.process(ctx, raw)
	}

	if c.syncMode {
		// In sync mode, store channel for manual processing
		c.changes = changes
		return initialErr
	}

	// Continue watching asynchronously
	go c.watch(ctx, changes)

	return initialErr
}

// Process reads and processes the next value from the watcher.
// This is only available in sync mode and is used for deterministic testing.
// Returns false if no value is available or the channel is closed.
func (c *Capacitor[T]) Process(ctx context.Context) bool {
	if !c.syncMode {
		return false
	}

	select {
	case raw, ok := <-c.changes:
		if !ok {
			return false
		}
		capitan.Emit(ctx, CapacitorChangeReceived)
		if c.metrics != nil {
			c.metrics.OnChangeReceived()
		}
		_ = c.process(ctx, raw) //nolint:errcheck // Errors stored via setError
		return true
	default:
		return false
	}
}

// process unmarshals, validates, and delivers a single configuration update.
func (c *Capacitor[T]) process(ctx context.Context, raw []byte) error {
	start := c.clock.Now()
	oldState := c.State()

	// Unmarshal
	var result T
	if err := c.codec.Unmarshal(raw, &result); err != nil {
		c.setError(err)
		c.transitionState(ctx, oldState, c.failureState())
		capitan.Emit(ctx, CapacitorTransformFailed,
			KeyError.Field(err.Error()),
		)
		if c.metrics != nil {
			c.metrics.OnProcessFailure("unmarshal", c.clock.Since(start))
		}
		return fmt.Errorf("unmarshal failed: %w", err)
	}

	// Validate
	if err := result.Validate(); err != nil {
		c.setError(err)
		c.transitionState(ctx, oldState, c.failureState())
		capitan.Emit(ctx, CapacitorValidationFailed,
			KeyError.Field(err.Error()),
		)
		if c.metrics != nil {
			c.metrics.OnProcessFailure("validate", c.clock.Since(start))
		}
		return fmt.Errorf("validation failed: %w", err)
	}

	// Get previous value for pipeline (zero value if none)
	var prev T
	if ptr := c.current.Load(); ptr != nil {
		prev = *ptr
	}

	// Build request and process through pipeline
	req := &Request[T]{Previous: prev, Current: result, Raw: raw}
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

	// Success - store result and clear error history
	c.current.Store(&processed.Current)
	c.lastError.Store(nil)
	c.errorHistory.clear()
	c.transitionState(ctx, oldState, StateHealthy)
	capitan.Emit(ctx, CapacitorApplySucceeded)
	if c.metrics != nil {
		c.metrics.OnProcessSuccess(c.clock.Since(start))
	}

	return nil
}

// failureState returns the appropriate failure state based on whether
// a valid configuration has ever been applied.
func (c *Capacitor[T]) failureState() State {
	if c.current.Load() == nil {
		return StateEmpty
	}
	return StateDegraded
}

// transitionState updates the state and emits a state change event if changed.
func (c *Capacitor[T]) transitionState(ctx context.Context, oldState, newState State) {
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

// setError stores an error atomically and adds it to the error history.
func (c *Capacitor[T]) setError(err error) {
	e := err
	c.lastError.Store(&e)
	c.errorHistory.push(err)
}

// watch processes changes from the watcher channel with debouncing.
func (c *Capacitor[T]) watch(ctx context.Context, changes <-chan []byte) {
	defer func() {
		finalState := c.State()
		capitan.Emit(ctx, CapacitorStopped,
			KeyState.Field(finalState.String()),
		)
		if c.onStop != nil {
			c.onStop(finalState)
		}
	}()

	var (
		timer      clockz.Timer
		pending    []byte
		hasPending bool
	)

	for {
		// Get timer channel or nil if no timer
		var timerC <-chan time.Time
		if timer != nil {
			timerC = timer.C()
		}

		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return

		case raw, ok := <-changes:
			if !ok {
				// Channel closed, process any pending change
				if hasPending {
					_ = c.process(ctx, pending) //nolint:errcheck // Errors stored via setError
				}
				return
			}

			capitan.Emit(ctx, CapacitorChangeReceived)
			if c.metrics != nil {
				c.metrics.OnChangeReceived()
			}
			pending = raw
			hasPending = true

			// Reset or start debounce timer
			if timer == nil {
				timer = c.clock.NewTimer(c.debounce)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C():
					default:
					}
				}
				timer.Reset(c.debounce)
			}

		case <-timerC:
			if hasPending {
				_ = c.process(ctx, pending) //nolint:errcheck // Errors stored via setError
				hasPending = false
			}
		}
	}
}
