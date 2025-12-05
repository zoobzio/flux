package flux

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/clockz"
)

// CompositeCapacitor watches multiple sources, unmarshals and validates each,
// and delivers the slice of parsed values to a reducer for merging.
type CompositeCapacitor[T any] struct {
	sources  []Watcher
	reducer  func([]T) (T, error)
	debounce time.Duration
	syncMode bool
	clock    clockz.Clock
	format   Format

	state     atomic.Int32
	current   atomic.Pointer[T]
	lastError atomic.Pointer[error]

	mu      sync.Mutex
	started bool

	// For sync mode
	sourceChans []<-chan []byte
	latest      [][]byte
	ready       []bool
}

// Compose creates a CompositeCapacitor for multiple sources.
//
// Each source emits raw bytes when it changes.
// Bytes from each source are automatically unmarshaled to type T.
// Each parsed value is validated using go-playground/validator tags.
// When all sources are ready, the reducer receives a slice of all parsed values
// in the same order as the sources. The reducer merges them and returns the
// final configuration, which is stored and accessible via Current().
//
// Example:
//
//	type Config struct {
//	    Port    int `yaml:"port" validate:"min=1,max=65535"`
//	    Timeout int `yaml:"timeout"`
//	}
//
//	capacitor := flux.Compose[Config](
//	    func(configs []Config) (Config, error) {
//	        merged := configs[0]  // defaults
//	        if configs[1].Port != 0 {
//	            merged.Port = configs[1].Port  // file overrides
//	        }
//	        return merged, nil
//	    },
//	    defaultsWatcher,
//	    fileWatcher,
//	)
func Compose[T any](
	reducer func([]T) (T, error),
	sources ...Watcher,
) *CompositeCapacitor[T] {
	return ComposeWithOptions[T](reducer, nil, sources...)
}

// ComposeWithOptions creates a CompositeCapacitor with custom options.
func ComposeWithOptions[T any](
	reducer func([]T) (T, error),
	opts []Option,
	sources ...Watcher,
) *CompositeCapacitor[T] {
	cfg := &config{
		debounce: DefaultDebounce,
		clock:    clockz.RealClock,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	c := &CompositeCapacitor[T]{
		sources:  sources,
		reducer:  reducer,
		debounce: cfg.debounce,
		syncMode: cfg.syncMode,
		clock:    cfg.clock,
		format:   cfg.format,
		latest:   make([][]byte, len(sources)),
		ready:    make([]bool, len(sources)),
	}
	c.state.Store(int32(StateLoading))

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
	for i, ch := range c.sourceChans {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case raw, ok := <-ch:
			if !ok {
				return fmt.Errorf("source %d closed before emitting initial value", i)
			}
			c.latest[i] = raw
			c.ready[i] = true
		}
	}

	capitan.Emit(ctx, CapacitorChangeReceived)

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
		_ = c.process(ctx) //nolint:errcheck // Errors stored via setError
		return true
	}
	return false
}

// process unmarshals each source, validates, and calls callback.
func (c *CompositeCapacitor[T]) process(ctx context.Context) error {
	oldState := c.State()

	// Unmarshal and validate each source
	results := make([]T, len(c.latest))
	for i, raw := range c.latest {
		var result T
		if err := unmarshal(raw, &result, c.format); err != nil {
			c.setError(err)
			c.transitionState(ctx, oldState, c.failureState())
			capitan.Emit(ctx, CapacitorTransformFailed,
				KeyError.Field(fmt.Sprintf("source %d: %s", i, err.Error())),
			)
			return fmt.Errorf("unmarshal source %d failed: %w", i, err)
		}

		if err := validate.Struct(result); err != nil {
			c.setError(err)
			c.transitionState(ctx, oldState, c.failureState())
			capitan.Emit(ctx, CapacitorValidationFailed,
				KeyError.Field(fmt.Sprintf("source %d: %s", i, err.Error())),
			)
			return fmt.Errorf("validation source %d failed: %w", i, err)
		}

		results[i] = result
	}

	// Reduce all parsed values to final merged config
	merged, err := c.reducer(results)
	if err != nil {
		c.setError(err)
		c.transitionState(ctx, oldState, c.failureState())
		capitan.Emit(ctx, CapacitorApplyFailed,
			KeyError.Field(err.Error()),
		)
		return fmt.Errorf("reducer failed: %w", err)
	}

	// Success - store merged result
	c.current.Store(&merged)
	c.lastError.Store(nil)
	c.transitionState(ctx, oldState, StateHealthy)
	capitan.Emit(ctx, CapacitorApplySucceeded)

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
}

func (c *CompositeCapacitor[T]) setError(err error) {
	e := err
	c.lastError.Store(&e)
}

// watch processes changes from all sources with debouncing.
func (c *CompositeCapacitor[T]) watch(ctx context.Context) {
	defer func() {
		capitan.Emit(ctx, CapacitorStopped,
			KeyState.Field(c.State().String()),
		)
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
