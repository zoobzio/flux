package flux

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/zoobzio/capitan"
	"github.com/zoobzio/clockz"
	"gopkg.in/yaml.v3"
)

// DefaultDebounce is the default debounce duration for change processing.
const DefaultDebounce = 100 * time.Millisecond

// validate is the shared validator instance.
var validate = validator.New()

// Capacitor watches a source for changes, unmarshals and validates the data,
// and delivers it to application code with automatic rollback on failure.
type Capacitor[T any] struct {
	watcher  Watcher
	callback func(T) error
	debounce time.Duration
	syncMode bool
	clock    clockz.Clock
	format   Format

	state     atomic.Int32
	current   atomic.Pointer[T]
	lastError atomic.Pointer[error]

	mu      sync.Mutex
	started bool

	// For sync mode: channel to receive changes
	changes <-chan []byte
}

// Format specifies the expected data format.
type Format int

const (
	// FormatAuto detects format from content (default).
	FormatAuto Format = iota
	// FormatJSON expects JSON format.
	FormatJSON
	// FormatYAML expects YAML format.
	FormatYAML
)

// config holds configuration options for a Capacitor.
type config struct {
	debounce time.Duration
	syncMode bool
	clock    clockz.Clock
	format   Format
}

// Option configures a Capacitor.
type Option func(*config)

// WithDebounce sets the debounce duration for change processing.
// Changes arriving within this duration are coalesced into a single update.
func WithDebounce(d time.Duration) Option {
	return func(c *config) {
		c.debounce = d
	}
}

// WithSyncMode enables synchronous processing for testing.
// In sync mode, changes are processed immediately without debouncing
// or async goroutines, making tests deterministic.
func WithSyncMode() Option {
	return func(c *config) {
		c.syncMode = true
	}
}

// WithClock sets a custom clock for time operations.
// Use this with clockz.FakeClock for deterministic debounce testing.
func WithClock(clock clockz.Clock) Option {
	return func(c *config) {
		c.clock = clock
	}
}

// WithJSON enforces JSON format for incoming data.
// If set, data that is not valid JSON will fail with an error.
// Without this option, format is auto-detected.
func WithJSON() Option {
	return func(c *config) {
		c.format = FormatJSON
	}
}

// WithYAML enforces YAML format for incoming data.
// If set, data is always parsed as YAML (which also accepts JSON).
// Without this option, format is auto-detected.
func WithYAML() Option {
	return func(c *config) {
		c.format = FormatYAML
	}
}

// New creates a new Capacitor for a single source.
//
// The watcher emits raw bytes when the source changes.
// Bytes are automatically unmarshaled to type R (via yaml/json struct tags).
// The struct is validated using go-playground/validator tags.
// On success, the callback receives the ready-to-use value.
//
// Example:
//
//	type Config struct {
//	    Port int `yaml:"port" validate:"min=1,max=65535"`
//	}
//
//	capacitor := flux.New[Config](
//	    flux.NewFileWatcher("config.yaml"),
//	    func(cfg Config) error {
//	        return app.SetConfig(cfg)
//	    },
//	)
func New[T any](
	watcher Watcher,
	callback func(T) error,
	opts ...Option,
) *Capacitor[T] {
	cfg := &config{
		debounce: DefaultDebounce,
		clock:    clockz.RealClock,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	c := &Capacitor[T]{
		watcher:  watcher,
		callback: callback,
		debounce: cfg.debounce,
		syncMode: cfg.syncMode,
		clock:    cfg.clock,
		format:   cfg.format,
	}
	c.state.Store(int32(StateLoading))

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
	select {
	case <-ctx.Done():
		return ctx.Err()
	case raw, ok := <-changes:
		if !ok {
			return fmt.Errorf("watcher closed before emitting initial value")
		}
		capitan.Emit(ctx, CapacitorChangeReceived)
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
		_ = c.process(ctx, raw) //nolint:errcheck // Errors stored via setError
		return true
	default:
		return false
	}
}

// process unmarshals, validates, and delivers a single configuration update.
func (c *Capacitor[T]) process(ctx context.Context, raw []byte) error {
	oldState := c.State()

	// Unmarshal
	var result T
	if err := unmarshal(raw, &result, c.format); err != nil {
		c.setError(err)
		c.transitionState(ctx, oldState, c.failureState())
		capitan.Emit(ctx, CapacitorTransformFailed,
			KeyError.Field(err.Error()),
		)
		return fmt.Errorf("unmarshal failed: %w", err)
	}

	// Validate
	if err := validate.Struct(result); err != nil {
		c.setError(err)
		c.transitionState(ctx, oldState, c.failureState())
		capitan.Emit(ctx, CapacitorValidationFailed,
			KeyError.Field(err.Error()),
		)
		return fmt.Errorf("validation failed: %w", err)
	}

	// Callback
	if err := c.callback(result); err != nil {
		c.setError(err)
		c.transitionState(ctx, oldState, c.failureState())
		capitan.Emit(ctx, CapacitorApplyFailed,
			KeyError.Field(err.Error()),
		)
		return fmt.Errorf("callback failed: %w", err)
	}

	// Success
	c.current.Store(&result)
	c.lastError.Store(nil)
	c.transitionState(ctx, oldState, StateHealthy)
	capitan.Emit(ctx, CapacitorApplySucceeded)

	return nil
}

// unmarshal parses bytes according to the specified format.
// If format is FormatAuto, it detects the format from content.
func unmarshal(data []byte, v any, format Format) error {
	switch format {
	case FormatJSON:
		if err := json.Unmarshal(data, v); err != nil {
			return fmt.Errorf("expected JSON: %w", err)
		}
		return nil

	case FormatYAML:
		return yaml.Unmarshal(data, v)

	default: // FormatAuto
		// Trim whitespace for detection
		trimmed := bytes.TrimSpace(data)

		// Detect JSON by leading character
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			return json.Unmarshal(data, v)
		}

		// Default to YAML (which also handles plain JSON)
		return yaml.Unmarshal(data, v)
	}
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
}

// setError stores an error atomically.
func (c *Capacitor[T]) setError(err error) {
	e := err
	c.lastError.Store(&e)
}

// watch processes changes from the watcher channel with debouncing.
func (c *Capacitor[T]) watch(ctx context.Context, changes <-chan []byte) {
	defer func() {
		capitan.Emit(ctx, CapacitorStopped,
			KeyState.Field(c.State().String()),
		)
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
