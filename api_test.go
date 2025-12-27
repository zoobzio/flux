package flux

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestConfig is a simple config type for testing.
type TestConfig struct {
	Port    int    `yaml:"port" json:"port"`
	Host    string `yaml:"host" json:"host"`
	Timeout int    `yaml:"timeout" json:"timeout"`
}

// Validate implements the Validator interface.
func (c TestConfig) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Port)
	}
	if c.Host == "" {
		return errors.New("host is required")
	}
	return nil
}

func TestCapacitor_BasicYAML(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg TestConfig) error {
			applied = cfg
			return nil
		},
	).SyncMode().Codec(YAMLCodec{})

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
		func(_ context.Context, _, cfg TestConfig) error {
			applied = cfg
			return nil
		},
	).SyncMode()

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
		func(_ context.Context, _, _ TestConfig) error {
			return nil
		},
	).SyncMode()

	// Invalid: port 0 violates min=1
	ch <- []byte(`{"port": 0, "host": "localhost"}`)

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
		func(_ context.Context, _, _ TestConfig) error {
			return nil
		},
	).SyncMode()

	// Invalid: host is required but missing
	ch <- []byte(`{"port": 8080}`)

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected validation error for missing required field")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCapacitor_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error {
			return nil
		},
	).SyncMode()

	ch <- []byte("not valid json")

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
		func(_ context.Context, _, cfg TestConfig) error {
			applied = cfg
			return nil
		},
	).SyncMode()

	// Valid initial config
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	if capacitor.State() != StateHealthy {
		t.Fatalf("expected healthy, got %s", capacitor.State())
	}

	// Invalid update
	ch <- []byte(`{"port": 0, "host": "localhost"}`) // port 0 invalid
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
		func(_ context.Context, _, cfg TestConfig) error {
			applied = cfg
			return nil
		},
	).SyncMode()

	// Valid → Invalid → Valid
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	ch <- []byte(`{"port": 0, "host": "localhost"}`) // Invalid
	capacitor.Process(ctx)

	if capacitor.State() != StateDegraded {
		t.Fatalf("expected degraded, got %s", capacitor.State())
	}

	ch <- []byte(`{"port": 9090, "host": "newhost"}`) // Valid again
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
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	// Before start, no current
	_, ok := capacitor.Current()
	if ok {
		t.Error("expected no current before start")
	}

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
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
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	// Valid config - no error
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	if capacitor.LastError() != nil {
		t.Errorf("expected no error, got %v", capacitor.LastError())
	}

	// Invalid config
	ch <- []byte(`{"port": 0, "host": "localhost"}`)
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
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	ch <- []byte(`{"port": 9090, "host": "localhost"}`)
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
		func(_ context.Context, _, _ TestConfig) error {
			return errors.New("callback error")
		},
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
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
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

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
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	ctx := context.Background()
	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected error when watcher closes before emitting value")
	}
}

func TestCapacitor_ProcessNotAvailableWithoutSyncMode(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
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
	ch <- []byte(`{"port": 1, "host": "localhost"}`) // Initial value

	var applyCount atomic.Int32
	var lastPort atomic.Int32

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ context.Context, _, cfg TestConfig) error {
			applyCount.Add(1)
			lastPort.Store(int32(cfg.Port)) //nolint:gosec // Port validated to 1-65535
			return nil
		},
	).Debounce(100 * time.Millisecond).Clock(clock)

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
	ch <- []byte(`{"port": 2, "host": "localhost"}`)
	ch <- []byte(`{"port": 3, "host": "localhost"}`)
	ch <- []byte(`{"port": 4, "host": "localhost"}`)

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
	ch <- []byte(`{"port": 1, "host": "localhost"}`) // Initial value

	var applyCount atomic.Int32
	var lastPort atomic.Int32

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ context.Context, _, cfg TestConfig) error {
			applyCount.Add(1)
			lastPort.Store(int32(cfg.Port)) //nolint:gosec // Port validated to 1-65535
			return nil
		},
	).Debounce(100 * time.Millisecond).Clock(clock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := capacitor.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Send change
	ch <- []byte(`{"port": 99, "host": "localhost"}`)
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
	ch <- []byte(`{"port": 1, "host": "localhost"}`)
	ch <- []byte(`{"port": 2, "host": "localhost"}`)
	ch <- []byte(`{"port": 3, "host": "localhost"}`)
	ch <- []byte(`{"port": 4, "host": "localhost"}`)
	ch <- []byte(`{"port": 5, "host": "localhost"}`)

	var applyCount int
	var lastPort int

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg TestConfig) error {
			applyCount++
			lastPort = cfg.Port
			return nil
		},
	).SyncMode()

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

func TestCapacitor_WithCodec_JSON(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg TestConfig) error {
			applied = cfg
			return nil
		},
	).SyncMode().Codec(JSONCodec{})

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

func TestCapacitor_WithCodec_JSONRejectsYAML(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error {
			return nil
		},
	).SyncMode().Codec(JSONCodec{})

	// YAML that is not valid JSON
	ch <- []byte("port: 8080\nhost: localhost")

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected error when YAML sent with JSONCodec")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCapacitor_WithCodec_YAML(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg TestConfig) error {
			applied = cfg
			return nil
		},
	).SyncMode().Codec(YAMLCodec{})

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

func TestCapacitor_WithCodec_YAMLAcceptsJSON(t *testing.T) {
	// YAML parser accepts JSON, so this should work
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg TestConfig) error {
			applied = cfg
			return nil
		},
	).SyncMode().Codec(YAMLCodec{})

	ch <- []byte(`{"port": 9090, "host": "example.com"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 9090 {
		t.Errorf("expected port 9090, got %d", applied.Port)
	}
}

func TestCapacitor_DefaultCodecIsJSON(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied TestConfig
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg TestConfig) error {
			applied = cfg
			return nil
		},
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied.Port != 8080 {
		t.Errorf("expected port 8080, got %d", applied.Port)
	}
}

func TestCapacitor_WithCodec_RejectsInvalidJSON(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error {
			return nil
		},
	).SyncMode().Codec(JSONCodec{})

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
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
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
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

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
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected context error")
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
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Fatal("expected watcher error")
	}
}

func TestCapacitor_ContextCancelWithPendingTimer(t *testing.T) {
	ch := make(chan []byte, 2)
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).Debounce(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send another value to start the debounce timer
	ch <- []byte(`{"port": 9090, "host": "updated"}`)

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
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	var applyCount atomic.Int32
	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error {
			applyCount.Add(1)
			return nil
		},
	).Debounce(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send value and immediately close
	ch <- []byte(`{"port": 9090, "host": "updated"}`)
	close(ch)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Should have processed both (initial + pending on close)
	if applyCount.Load() < 2 {
		t.Errorf("expected at least 2 applies, got %d", applyCount.Load())
	}
}

func TestCapacitor_StartupTimeout(t *testing.T) {
	clock := clockz.NewFakeClock()
	ch := make(chan []byte) // unbuffered, will block forever

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().StartupTimeout(100 * time.Millisecond).Clock(clock)

	errCh := make(chan error, 1)
	go func() {
		errCh <- capacitor.Start(context.Background())
	}()

	// Wait for timeout context to register with the fake clock
	time.Sleep(10 * time.Millisecond)

	// Advance clock past timeout
	clock.Advance(150 * time.Millisecond)
	clock.BlockUntilReady()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if capacitor.State() != StateLoading {
			t.Errorf("expected loading state, got %s", capacitor.State())
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return after timeout")
	}
}

func TestCapacitor_StartupTimeout_SucceedsBeforeTimeout(t *testing.T) {
	clock := clockz.NewFakeClock()
	ch := make(chan []byte, 1)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().StartupTimeout(100 * time.Millisecond).Clock(clock)

	// Send value before starting
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	err := capacitor.Start(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy state, got %s", capacitor.State())
	}
}

func TestCapacitor_NoStartupTimeout_WaitsIndefinitely(t *testing.T) {
	ch := make(chan []byte) // unbuffered, will block

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode() // No startup timeout - should wait indefinitely

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := capacitor.Start(ctx)
	// Should timeout via context, not startup timeout
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

// testMetricsProvider captures metrics calls for testing.
type testMetricsProvider struct {
	stateChanges    []struct{ from, to State }
	processSuccess  []time.Duration
	processFailures []struct {
		stage    string
		duration time.Duration
	}
	changesReceived int
}

func (m *testMetricsProvider) OnStateChange(from, to State) {
	m.stateChanges = append(m.stateChanges, struct{ from, to State }{from, to})
}

func (m *testMetricsProvider) OnProcessSuccess(d time.Duration) {
	m.processSuccess = append(m.processSuccess, d)
}

func (m *testMetricsProvider) OnProcessFailure(stage string, d time.Duration) {
	m.processFailures = append(m.processFailures, struct {
		stage    string
		duration time.Duration
	}{stage, d})
}

func (m *testMetricsProvider) OnChangeReceived() {
	m.changesReceived++
}

func TestCapacitor_Metrics_StateChanges(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 2)
	metrics := &testMetricsProvider{}

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().Metrics(metrics)

	// Valid config
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should have Loading -> Healthy transition
	if len(metrics.stateChanges) != 1 {
		t.Fatalf("expected 1 state change, got %d", len(metrics.stateChanges))
	}
	if metrics.stateChanges[0].from != StateLoading || metrics.stateChanges[0].to != StateHealthy {
		t.Errorf("expected Loading->Healthy, got %s->%s",
			metrics.stateChanges[0].from, metrics.stateChanges[0].to)
	}

	// Invalid config
	ch <- []byte(`{"port": 0, "host": "localhost"}`)
	capacitor.Process(ctx)

	// Should have Healthy -> Degraded transition
	if len(metrics.stateChanges) != 2 {
		t.Fatalf("expected 2 state changes, got %d", len(metrics.stateChanges))
	}
	if metrics.stateChanges[1].from != StateHealthy || metrics.stateChanges[1].to != StateDegraded {
		t.Errorf("expected Healthy->Degraded, got %s->%s",
			metrics.stateChanges[1].from, metrics.stateChanges[1].to)
	}
}

func TestCapacitor_Metrics_ProcessSuccess(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)
	metrics := &testMetricsProvider{}

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().Metrics(metrics)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(metrics.processSuccess) != 1 {
		t.Errorf("expected 1 process success, got %d", len(metrics.processSuccess))
	}
}

func TestCapacitor_Metrics_ProcessFailure(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 2)
	metrics := &testMetricsProvider{}

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().Metrics(metrics)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	// Invalid config - validation failure
	ch <- []byte(`{"port": 0, "host": "localhost"}`)
	capacitor.Process(ctx)

	if len(metrics.processFailures) != 1 {
		t.Fatalf("expected 1 process failure, got %d", len(metrics.processFailures))
	}
	if metrics.processFailures[0].stage != "validate" {
		t.Errorf("expected validate stage, got %s", metrics.processFailures[0].stage)
	}
}

func TestCapacitor_Metrics_UnmarshalFailure(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 2)
	metrics := &testMetricsProvider{}

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().Metrics(metrics)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	// Invalid JSON - unmarshal failure
	ch <- []byte(`{invalid json}`)
	capacitor.Process(ctx)

	if len(metrics.processFailures) != 1 {
		t.Fatalf("expected 1 process failure, got %d", len(metrics.processFailures))
	}
	if metrics.processFailures[0].stage != "unmarshal" {
		t.Errorf("expected unmarshal stage, got %s", metrics.processFailures[0].stage)
	}
}

func TestCapacitor_Metrics_PipelineFailure(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)
	metrics := &testMetricsProvider{}

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error {
			return errors.New("callback failed")
		},
	).SyncMode().Metrics(metrics)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	if len(metrics.processFailures) != 1 {
		t.Fatalf("expected 1 process failure, got %d", len(metrics.processFailures))
	}
	if metrics.processFailures[0].stage != "pipeline" {
		t.Errorf("expected pipeline stage, got %s", metrics.processFailures[0].stage)
	}
}

func TestCapacitor_Metrics_ChangeReceived(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 3)
	metrics := &testMetricsProvider{}

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().Metrics(metrics)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	ch <- []byte(`{"port": 9090, "host": "localhost"}`)
	capacitor.Process(ctx)

	ch <- []byte(`{"port": 9091, "host": "localhost"}`)
	capacitor.Process(ctx)

	if metrics.changesReceived != 3 {
		t.Errorf("expected 3 changes received, got %d", metrics.changesReceived)
	}
}

func TestCapacitor_WithCircuitBreaker_RejectsAfterFailures(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 10)

	var callbackCalls int
	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error {
			callbackCalls++
			return errors.New("always fail")
		},
		WithCircuitBreaker[TestConfig](2, 1*time.Hour), // Open after 2 failures
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx) // First failure
	callbackCalls = 0    // Reset counter after start

	ch <- []byte(`{"port": 9090, "host": "localhost"}`)
	capacitor.Process(ctx) // Second failure - circuit opens

	ch <- []byte(`{"port": 9091, "host": "localhost"}`)
	capacitor.Process(ctx) // Should be rejected by circuit breaker

	// After circuit opens, callback should not be called
	// (the third process should be rejected immediately)
	if callbackCalls > 1 {
		t.Errorf("expected at most 1 callback call after circuit opens, got %d", callbackCalls)
	}
}

func TestCapacitor_OnStop_CalledOnContextCancel(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	stopCh := make(chan State, 1)

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).OnStop(func(s State) {
		stopCh <- s
	})

	ctx, cancel := context.WithCancel(context.Background())

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Cancel context to trigger stop
	cancel()

	select {
	case stopState := <-stopCh:
		if stopState != StateHealthy {
			t.Errorf("expected StateHealthy at stop, got %s", stopState)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected OnStop to be called")
	}
}

func TestCapacitor_OnStop_CalledOnChannelClose(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	stopCh := make(chan struct{}, 1)

	capacitor := New[TestConfig](
		NewChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).OnStop(func(_ State) {
		stopCh <- struct{}{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	close(ch)

	select {
	case <-stopCh:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("expected OnStop to be called when channel closes")
	}
}

func TestCapacitor_ErrorHistory_RecordsErrors(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 5)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().ErrorHistorySize(3)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	// Generate errors
	ch <- []byte(`{"port": 0, "host": "localhost"}`) // Invalid port
	capacitor.Process(ctx)
	ch <- []byte(`{"port": 8080, "host": ""}`) // Missing host
	capacitor.Process(ctx)

	history := capacitor.ErrorHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 errors in history, got %d", len(history))
	}
}

func TestCapacitor_ErrorHistory_ClearsOnSuccess(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 5)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().ErrorHistorySize(3)

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	// Generate error
	ch <- []byte(`{"port": 0, "host": "localhost"}`)
	capacitor.Process(ctx)

	if len(capacitor.ErrorHistory()) != 1 {
		t.Error("expected 1 error before recovery")
	}

	// Success clears history
	ch <- []byte(`{"port": 9090, "host": "localhost"}`)
	capacitor.Process(ctx)

	if capacitor.ErrorHistory() != nil {
		t.Error("expected error history to be cleared on success")
	}
}

func TestCapacitor_ErrorHistory_EvictsOldest(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 10)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode().ErrorHistorySize(2) // Only keep 2

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	// Generate 3 errors
	ch <- []byte(`{"port": 0, "host": "localhost"}`)
	capacitor.Process(ctx)
	ch <- []byte(`{"port": -1, "host": "localhost"}`)
	capacitor.Process(ctx)
	ch <- []byte(`{"port": 99999, "host": "localhost"}`)
	capacitor.Process(ctx)

	history := capacitor.ErrorHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 errors (oldest evicted), got %d", len(history))
	}
}

func TestCapacitor_ErrorHistory_DisabledByDefault(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 3)

	capacitor := New[TestConfig](
		NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	capacitor.Start(ctx)

	ch <- []byte(`{"port": 0, "host": "localhost"}`)
	capacitor.Process(ctx)

	if capacitor.ErrorHistory() != nil {
		t.Error("expected nil error history when disabled")
	}
}
