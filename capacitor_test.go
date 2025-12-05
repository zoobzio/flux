package flux

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestConfig is a simple config type for testing with validation tags.
type TestConfig struct {
	Port    int    `yaml:"port" json:"port" validate:"min=1,max=65535"`
	Host    string `yaml:"host" json:"host" validate:"required"`
	Timeout int    `yaml:"timeout" json:"timeout"`
}

func TestCapacitor_BasicYAML(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
	)

	ch <- []byte("port: 8080\nhost: localhost\ntimeout: 30")

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 8080 {
		t.Errorf("expected port 8080, got %d", applied.Port)
	}
	if applied.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", applied.Host)
	}
	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}
}

func TestCapacitor_BasicJSON(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
	)

	ch <- []byte(`{"port": 9090, "host": "example.com", "timeout": 60}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 9090 {
		t.Errorf("expected port 9090, got %d", applied.Port)
	}
	if applied.Host != "example.com" {
		t.Errorf("expected host example.com, got %s", applied.Host)
	}
}

func TestCapacitor_ValidationFailsMinMax(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error {
			return nil
		},
		WithSyncMode(),
	)

	// Invalid: port 0 violates min=1
	ch <- []byte("port: 0\nhost: localhost")

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected validation error")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCapacitor_ValidationFailsMissingRequired(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error {
			return nil
		},
		WithSyncMode(),
	)

	// Invalid: host is required but missing
	ch <- []byte("port: 8080")

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected validation error for missing required field")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCapacitor_InvalidYAML(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error {
			return nil
		},
		WithSyncMode(),
	)

	ch <- []byte("not: valid: yaml: {{{}}")

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCapacitor_RollbackOnValidationFailure(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
	)

	// Valid initial config
	ch <- []byte("port: 8080\nhost: localhost")
	capacitor.Start(ctx)

	if capacitor.State() != StateHealthy {
		t.Fatalf("expected healthy, got %s", capacitor.State())
	}

	// Invalid update
	ch <- []byte("port: 0\nhost: localhost") // port 0 invalid
	capacitor.Process(ctx)

	// Should be degraded, not empty
	if capacitor.State() != StateDegraded {
		t.Errorf("expected degraded, got %s", capacitor.State())
	}

	// Previous config should still be current
	current, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config")
	}
	if current.Port != 8080 {
		t.Errorf("expected port 8080 retained, got %d", current.Port)
	}

	// Applied should still be old value (callback not called on failure)
	if applied.Port != 8080 {
		t.Errorf("expected applied to still be 8080, got %d", applied.Port)
	}
}

func TestCapacitor_RecoverFromDegraded(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
	)

	// Valid → Invalid → Valid
	ch <- []byte("port: 8080\nhost: localhost")
	capacitor.Start(ctx)

	ch <- []byte("port: 0\nhost: localhost") // Invalid
	capacitor.Process(ctx)

	if capacitor.State() != StateDegraded {
		t.Fatalf("expected degraded, got %s", capacitor.State())
	}

	ch <- []byte("port: 9090\nhost: newhost") // Valid again
	capacitor.Process(ctx)

	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}

	if applied.Port != 9090 {
		t.Errorf("expected port 9090, got %d", applied.Port)
	}
}

func TestCapacitor_Current(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		WithSyncMode(),
	)

	// Before start, no current
	_, ok := capacitor.Current()
	if ok {
		t.Error("expected no current before start")
	}

	ch <- []byte("port: 8080\nhost: localhost")
	capacitor.Start(ctx)

	current, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current after start")
	}
	if current.Port != 8080 {
		t.Errorf("expected port 8080, got %d", current.Port)
	}
}

func TestCapacitor_LastError(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		WithSyncMode(),
	)

	// Valid config - no error
	ch <- []byte("port: 8080\nhost: localhost")
	capacitor.Start(ctx)

	if capacitor.LastError() != nil {
		t.Errorf("expected no error, got %v", capacitor.LastError())
	}

	// Invalid config
	ch <- []byte("port: 0\nhost: localhost")
	capacitor.Process(ctx)

	if capacitor.LastError() == nil {
		t.Error("expected error after validation failure")
	}
}

func TestCapacitor_CannotStartTwice(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		WithSyncMode(),
	)

	ch <- []byte("port: 8080\nhost: localhost")
	capacitor.Start(ctx)

	ch <- []byte("port: 9090\nhost: localhost")
	err := capacitor.Start(ctx)
	if err == nil {
		t.Error("expected error on second start")
	}
}

func TestCapacitor_CallbackError(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error {
			return errors.New("callback error")
		},
		WithSyncMode(),
	)

	ch <- []byte("port: 8080\nhost: localhost")
	err := capacitor.Start(ctx)

	if err == nil {
		t.Fatal("expected callback error")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCapacitor_ContextCancellationBeforeValue(t *testing.T) {
	ch := make(chan []byte) // unbuffered, will block

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		WithSyncMode(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := capacitor.Start(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCapacitor_WatcherClosedBeforeStart(t *testing.T) {
	ch := make(chan []byte)
	close(ch)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		WithSyncMode(),
	)

	ctx := context.Background()
	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected error when watcher closes before emitting value")
	}
}

func TestCapacitor_ProcessNotAvailableWithoutSyncMode(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte("port: 8080\nhost: localhost")

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		// No WithSyncMode()
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := capacitor.Start(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Process should return false when not in sync mode
	if capacitor.Process(ctx) {
		t.Error("expected Process to return false when not in sync mode")
	}
}

func TestCapacitor_Debounce_CoalescesRapidChanges(t *testing.T) {
	clock := clockz.NewFakeClock()
	ch := make(chan []byte, 10)
	ch <- []byte("port: 1\nhost: localhost") // Initial value

	var applyCount atomic.Int32
	var lastPort atomic.Int32

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(cfg TestConfig) error {
			applyCount.Add(1)
			lastPort.Store(int32(cfg.Port)) //nolint:gosec // Port validated to 1-65535
			return nil
		},
		WithDebounce(100*time.Millisecond),
		WithClock(clock),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := capacitor.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Initial value applied immediately (no debounce on first)
	if applyCount.Load() != 1 {
		t.Errorf("expected 1 apply after start, got %d", applyCount.Load())
	}

	// Send rapid changes
	ch <- []byte("port: 2\nhost: localhost")
	ch <- []byte("port: 3\nhost: localhost")
	ch <- []byte("port: 4\nhost: localhost")

	// Allow goroutine to receive changes
	time.Sleep(10 * time.Millisecond)

	// No additional applies yet - debounce timer hasn't fired
	if applyCount.Load() != 1 {
		t.Errorf("expected still 1 apply (debouncing), got %d", applyCount.Load())
	}

	// Advance clock past debounce duration
	clock.Advance(150 * time.Millisecond)
	clock.BlockUntilReady()

	// Allow goroutine to process timer
	time.Sleep(10 * time.Millisecond)

	// Should have applied only the latest value
	if applyCount.Load() != 2 {
		t.Errorf("expected 2 applies after debounce, got %d", applyCount.Load())
	}
	if lastPort.Load() != 4 {
		t.Errorf("expected last port 4, got %d", lastPort.Load())
	}
}

func TestCapacitor_Debounce_ProcessesPendingOnClose(t *testing.T) {
	clock := clockz.NewFakeClock()
	ch := make(chan []byte, 10)
	ch <- []byte("port: 1\nhost: localhost") // Initial value

	var applyCount atomic.Int32
	var lastPort atomic.Int32

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(cfg TestConfig) error {
			applyCount.Add(1)
			lastPort.Store(int32(cfg.Port)) //nolint:gosec // Port validated to 1-65535
			return nil
		},
		WithDebounce(100*time.Millisecond),
		WithClock(clock),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := capacitor.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Send change
	ch <- []byte("port: 99\nhost: localhost")
	time.Sleep(10 * time.Millisecond)

	// Close channel before debounce fires
	close(ch)
	time.Sleep(10 * time.Millisecond)

	// Pending change should be processed immediately on close
	if applyCount.Load() != 2 {
		t.Errorf("expected 2 applies after close, got %d", applyCount.Load())
	}
	if lastPort.Load() != 99 {
		t.Errorf("expected last port 99, got %d", lastPort.Load())
	}
}

func TestCapacitor_ProcessCount(t *testing.T) {
	ch := make(chan []byte, 5)
	ch <- []byte("port: 1\nhost: localhost")
	ch <- []byte("port: 2\nhost: localhost")
	ch <- []byte("port: 3\nhost: localhost")
	ch <- []byte("port: 4\nhost: localhost")
	ch <- []byte("port: 5\nhost: localhost")

	var applyCount int
	var lastPort int

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applyCount++
			lastPort = cfg.Port
			return nil
		},
		WithSyncMode(),
	)

	ctx := context.Background()
	err := capacitor.Start(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Initial value processed
	if applyCount != 1 {
		t.Errorf("expected 1 apply after start, got %d", applyCount)
	}

	// Process remaining values one by one
	for i := 2; i <= 5; i++ {
		if !capacitor.Process(ctx) {
			t.Fatalf("expected Process to return true for value %d", i)
		}
		if applyCount != i {
			t.Errorf("expected %d applies, got %d", i, applyCount)
		}
		if lastPort != i {
			t.Errorf("expected last port %d, got %d", i, lastPort)
		}
	}

	// No more values
	if capacitor.Process(ctx) {
		t.Error("expected Process to return false when no values")
	}
}

func TestCapacitor_WithJSON_AcceptsJSON(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
		WithJSON(),
	)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 8080 {
		t.Errorf("expected port 8080, got %d", applied.Port)
	}
	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}
}

func TestCapacitor_WithJSON_RejectsYAML(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error {
			return nil
		},
		WithSyncMode(),
		WithJSON(),
	)

	// YAML that is not valid JSON
	ch <- []byte("port: 8080\nhost: localhost")

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected error when YAML sent with WithJSON()")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCapacitor_WithYAML_AcceptsYAML(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
		WithYAML(),
	)

	ch <- []byte("port: 8080\nhost: localhost")

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 8080 {
		t.Errorf("expected port 8080, got %d", applied.Port)
	}
	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}
}

func TestCapacitor_WithYAML_AcceptsJSON(t *testing.T) {
	// YAML parser accepts JSON, so this should work
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
		WithYAML(),
	)

	ch <- []byte(`{"port": 9090, "host": "example.com"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 9090 {
		t.Errorf("expected port 9090, got %d", applied.Port)
	}
}

func TestCapacitor_FormatAuto_DetectsJSON(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
		// No format option = auto-detect
	)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 8080 {
		t.Errorf("expected port 8080, got %d", applied.Port)
	}
}

func TestCapacitor_FormatAuto_DetectsYAML(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(cfg TestConfig) error {
			applied = cfg
			return nil
		},
		WithSyncMode(),
		// No format option = auto-detect
	)

	ch <- []byte("port: 8080\nhost: localhost")

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 8080 {
		t.Errorf("expected port 8080, got %d", applied.Port)
	}
}

func TestCapacitor_WithJSON_RejectsInvalidJSON(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error {
			return nil
		},
		WithSyncMode(),
		WithJSON(),
	)

	ch <- []byte(`{"port": 8080, "host": "localhost"`) // Missing closing brace

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCapacitor_ProcessNotInSyncMode(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte("port: 8080\nhost: localhost")

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		// No WithSyncMode - async mode
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Process should return false when not in sync mode
	if capacitor.Process(ctx) {
		t.Error("expected Process to return false in async mode")
	}
}

func TestCapacitor_ProcessChannelClosed(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte("port: 8080\nhost: localhost")

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		WithSyncMode(),
	)

	ctx := context.Background()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Close channel
	close(ch)

	// Process should return false when channel is closed
	if capacitor.Process(ctx) {
		t.Error("expected Process to return false when channel closed")
	}
}

func TestCapacitor_StartContextCancelledDuringWatch(t *testing.T) {
	ch := make(chan []byte) // unbuffered, will block

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		WithSyncMode(),
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestState_StringUnknown(t *testing.T) {
	unknown := State(99)
	if unknown.String() != "unknown" {
		t.Errorf("expected 'unknown', got %s", unknown.String())
	}
}

// errorWatcher is a Watcher that returns an error on Watch.
type errorWatcher struct {
	err error
}

func (w *errorWatcher) Watch(_ context.Context) (<-chan []byte, error) {
	return nil, w.err
}

func TestCapacitor_WatcherError(t *testing.T) {
	capacitor := New[TestConfig](
		&errorWatcher{err: errors.New("watcher failed")},
		func(_ TestConfig) error { return nil },
		WithSyncMode(),
	)

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Fatal("expected watcher error")
	}
}

func TestCapacitor_ContextCancelWithPendingTimer(t *testing.T) {
	ch := make(chan []byte, 2)
	ch <- []byte("port: 8080\nhost: localhost")

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ TestConfig) error { return nil },
		WithDebounce(100*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send another value to start the debounce timer
	ch <- []byte("port: 9090\nhost: updated")

	// Give time for the value to be received
	time.Sleep(10 * time.Millisecond)

	// Cancel while timer is pending
	cancel()

	// Give time for goroutine to exit
	time.Sleep(20 * time.Millisecond)

	// No panic means success
}

func TestCapacitor_WatchChannelClosedWithPending(t *testing.T) {
	ch := make(chan []byte, 2)
	ch <- []byte("port: 8080\nhost: localhost")

	var applyCount atomic.Int32
	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ TestConfig) error {
			applyCount.Add(1)
			return nil
		},
		WithDebounce(50*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send value and immediately close
	ch <- []byte("port: 9090\nhost: updated")
	close(ch)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Should have processed both (initial + pending on close)
	if applyCount.Load() < 2 {
		t.Errorf("expected at least 2 applies, got %d", applyCount.Load())
	}
}
