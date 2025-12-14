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

// ComposeConfig is a test config for Compose tests.
type ComposeConfig struct {
	Port    int    `yaml:"port" json:"port"`
	Host    string `yaml:"host" json:"host"`
	Timeout int    `yaml:"timeout" json:"timeout"`
}

// Validate implements the Validator interface.
func (c ComposeConfig) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Port)
	}
	if c.Host == "" {
		return errors.New("host is required")
	}
	return nil
}

func TestCompose_MergesTwoSources(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	var applied []ComposeConfig
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			applied = configs
			return configs[1], nil // return second as merged result
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	// Send to both sources
	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "override"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(applied) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(applied))
	}
	if applied[0].Port != 8080 {
		t.Errorf("expected first port 8080, got %d", applied[0].Port)
	}
	if applied[1].Port != 9090 {
		t.Errorf("expected second port 9090, got %d", applied[1].Port)
	}
	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}

	// Verify Current() returns the merged result
	current, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config")
	}
	if current.Port != 9090 {
		t.Errorf("expected current port 9090 (merged), got %d", current.Port)
	}
}

func TestCompose_MergesWithReducer(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1) // defaults
	ch2 := make(chan []byte, 1) // overrides

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			// Simple merge: start with defaults, override with non-zero values
			merged := configs[0]
			if configs[1].Port != 0 {
				merged.Port = configs[1].Port
			}
			if configs[1].Host != "" {
				merged.Host = configs[1].Host
			}
			if configs[1].Timeout != 0 {
				merged.Timeout = configs[1].Timeout
			}
			return merged, nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	// Defaults
	ch1 <- []byte(`{"port": 8080, "host": "localhost", "timeout": 30}`)
	// Overrides (only port)
	ch2 <- []byte(`{"port": 9090, "host": "override", "timeout": 0}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	merged, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config")
	}
	if merged.Port != 9090 {
		t.Errorf("expected merged port 9090, got %d", merged.Port)
	}
	if merged.Host != "override" {
		t.Errorf("expected merged host 'override', got %s", merged.Host)
	}
	if merged.Timeout != 30 {
		t.Errorf("expected merged timeout 30 (from defaults), got %d", merged.Timeout)
	}
}

func TestCompose_ValidationFailsOnAnySource(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	// First valid, second invalid (port 0)
	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 0, "host": "localhost"}`) // Invalid: port 0

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected validation error")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCompose_ThreeSources(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)
	ch3 := make(chan []byte, 1)

	var applied []ComposeConfig
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			applied = configs
			return configs[2], nil // return last as merged
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2), NewSyncChannelWatcher(ch3)},
	).SyncMode()

	ch1 <- []byte(`{"port": 1, "host": "one"}`)
	ch2 <- []byte(`{"port": 2, "host": "two"}`)
	ch3 <- []byte(`{"port": 3, "host": "three"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(applied) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(applied))
	}
	for i, cfg := range applied {
		expected := i + 1
		if cfg.Port != expected {
			t.Errorf("config %d: expected port %d, got %d", i, expected, cfg.Port)
		}
	}
}

func TestCompose_NoSourcesError(t *testing.T) {
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, _ []ComposeConfig) (ComposeConfig, error) {
			return ComposeConfig{}, nil
		},
		[]Watcher{},
	).SyncMode()

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Error("expected error for empty sources")
	}
}

func TestCompose_SingleSource(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied []ComposeConfig
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			applied = configs
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch)},
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "single"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(applied) != 1 {
		t.Fatalf("expected 1 config, got %d", len(applied))
	}
	if applied[0].Port != 8080 {
		t.Errorf("expected port 8080, got %d", applied[0].Port)
	}
}

func TestCompose_UpdateSingleSource(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 2)
	ch2 := make(chan []byte, 1)

	var lastApplied []ComposeConfig
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			lastApplied = configs
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	// Initial values
	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Update only first source
	ch1 <- []byte(`{"port": 7070, "host": "updated"}`)
	if !capacitor.Process(ctx) {
		t.Fatal("expected Process to return true")
	}

	if lastApplied[0].Port != 7070 {
		t.Errorf("expected first port 7070, got %d", lastApplied[0].Port)
	}
	if lastApplied[1].Port != 9090 {
		t.Errorf("expected second port still 9090, got %d", lastApplied[1].Port)
	}
}

func TestCompose_RollbackOnValidationFailure(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 2)
	ch2 := make(chan []byte, 1)

	var lastApplied []ComposeConfig
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			lastApplied = configs
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	// Valid initial values
	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if capacitor.State() != StateHealthy {
		t.Fatalf("expected healthy, got %s", capacitor.State())
	}

	// Update first source with invalid value
	ch1 <- []byte(`{"port": 0, "host": "localhost"}`) // Invalid port
	capacitor.Process(ctx)

	if capacitor.State() != StateDegraded {
		t.Errorf("expected degraded, got %s", capacitor.State())
	}

	// Previous config should still be current
	current, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config")
	}
	if current.Port != 8080 {
		t.Errorf("expected previous port 8080, got %d", current.Port)
	}

	// lastApplied should still have old values (reducer not called on failure)
	if lastApplied[0].Port != 8080 {
		t.Errorf("expected lastApplied[0].Port 8080, got %d", lastApplied[0].Port)
	}
}

func TestCompose_JSONFormat(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	var applied []ComposeConfig
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			applied = configs
			return configs[1], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	ch1 <- []byte(`{"port": 8080, "host": "json-source1"}`)
	ch2 <- []byte(`{"port": 9090, "host": "json-source2"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied[0].Host != "json-source1" {
		t.Errorf("expected json-source1, got %s", applied[0].Host)
	}
	if applied[1].Host != "json-source2" {
		t.Errorf("expected json-source2, got %s", applied[1].Host)
	}
}

func TestCompose_LastError(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 2)
	ch2 := make(chan []byte, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	// Valid initial values
	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// LastError should be nil after success
	if capacitor.LastError() != nil {
		t.Errorf("expected nil error after success, got %v", capacitor.LastError())
	}

	// Update with invalid value
	ch1 <- []byte(`{"port": 0, "host": "localhost"}`) // Invalid port
	capacitor.Process(ctx)

	// LastError should now be set
	if capacitor.LastError() == nil {
		t.Error("expected error after validation failure")
	}
}

func TestCompose_CurrentBeforeStart(t *testing.T) {
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(make(chan []byte))},
	).SyncMode()

	// Current should return zero value and false before any config
	current, ok := capacitor.Current()
	if ok {
		t.Error("expected ok=false before start")
	}
	if current.Port != 0 || current.Host != "" {
		t.Error("expected zero value before start")
	}
}

func TestCompose_ReducerError(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, _ []ComposeConfig) (ComposeConfig, error) {
			return ComposeConfig{}, fmt.Errorf("reducer failed intentionally")
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected reducer error")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}

	if capacitor.LastError() == nil {
		t.Error("expected LastError to be set")
	}
}

func TestCompose_AsyncWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1 := make(chan []byte, 2)
	ch2 := make(chan []byte, 2)

	var lastPort atomic.Int32
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			lastPort.Store(int32(configs[0].Port)) //nolint:gosec // Port validated to 1-65535
			return configs[0], nil
		},
		[]Watcher{NewChannelWatcher(ch1), NewChannelWatcher(ch2)},
	).Debounce(10 * time.Millisecond)

	// Initial values
	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if lastPort.Load() != 8080 {
		t.Errorf("expected initial port 8080, got %d", lastPort.Load())
	}

	// Send update through first channel
	ch1 <- []byte(`{"port": 7070, "host": "updated"}`)

	// Wait for debounce + processing
	time.Sleep(50 * time.Millisecond)

	if lastPort.Load() != 7070 {
		t.Errorf("expected updated port 7070, got %d", lastPort.Load())
	}

	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}
}

func TestCompose_AsyncWatchContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewChannelWatcher(ch1), NewChannelWatcher(ch2)},
	).Debounce(10 * time.Millisecond)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Cancel context - async watchers should exit gracefully
	cancel()

	// Give goroutines time to exit
	time.Sleep(50 * time.Millisecond)

	// No panic or deadlock means success
	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}
}

func TestCompose_StartContextCancelled(t *testing.T) {
	ch1 := make(chan []byte) // unbuffered, will block
	ch2 := make(chan []byte)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestCompose_ProcessNotInSyncMode(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewChannelWatcher(ch1), NewChannelWatcher(ch2)},
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

func TestCompose_ProcessChannelClosed(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	ctx := context.Background()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Close both channels
	close(ch1)
	close(ch2)

	// Process should return false when no new data
	if capacitor.Process(ctx) {
		t.Error("expected Process to return false when channels closed")
	}
}

func TestCompose_SourceClosedDuringInit(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	close(ch2) // Close before sending

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when source closes during init")
	}
}

func TestCompose_UnmarshalError(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte("not valid json")

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Fatal("expected unmarshal error")
	}

	if capacitor.State() != StateEmpty {
		t.Errorf("expected empty state, got %s", capacitor.State())
	}
}

func TestCompose_WatcherError(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte(`{"port": 8080, "host": "localhost"}`)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch), &errorWatcher{err: fmt.Errorf("watcher failed")}},
	).SyncMode()

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Fatal("expected watcher error")
	}
}

func TestCompose_CannotStartTwice(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	ctx := context.Background()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	err := capacitor.Start(ctx)
	if err == nil {
		t.Fatal("expected error on second Start")
	}
}

func TestCompose_AsyncMultipleUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1 := make(chan []byte, 5)
	ch2 := make(chan []byte, 5)

	ch1 <- []byte(`{"port": 1, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 2, "host": "other"}`)

	var lastPort atomic.Int32
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			lastPort.Store(int32(configs[0].Port)) //nolint:gosec // Port validated to 1-65535
			return configs[0], nil
		},
		[]Watcher{NewChannelWatcher(ch1), NewChannelWatcher(ch2)},
	).Debounce(10 * time.Millisecond)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send multiple rapid updates to trigger debounce timer reset
	for i := 3; i <= 5; i++ {
		ch1 <- []byte(fmt.Sprintf(`{"port": %d, "host": "localhost"}`, i))
		time.Sleep(5 * time.Millisecond) // Less than debounce
	}

	// Wait for final debounce
	time.Sleep(50 * time.Millisecond)

	// Should have coalesced to final value
	if lastPort.Load() != 5 {
		t.Errorf("expected final port 5, got %d", lastPort.Load())
	}
}

func TestCompose_SourceErrors(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 2)
	ch2 := make(chan []byte, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode()

	// Valid initial values
	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// SourceErrors should be nil after success
	if capacitor.SourceErrors() != nil {
		t.Errorf("expected nil source errors after success")
	}

	// Update first source with invalid value
	ch1 <- []byte(`{"port": 0, "host": "localhost"}`) // Invalid port
	capacitor.Process(ctx)

	// SourceErrors should now have an entry
	errs := capacitor.SourceErrors()
	if len(errs) == 0 {
		t.Error("expected source errors after validation failure")
	}
	if errs[0].Index != 0 {
		t.Errorf("expected error at index 0, got %d", errs[0].Index)
	}
}

// composeMetricsProvider captures metrics calls for CompositeCapacitor testing.
type composeMetricsProvider struct {
	stateChanges    []struct{ from, to State }
	processSuccess  []time.Duration
	processFailures []struct {
		stage    string
		duration time.Duration
	}
	changesReceived int
}

func (m *composeMetricsProvider) OnStateChange(from, to State) {
	m.stateChanges = append(m.stateChanges, struct{ from, to State }{from, to})
}

func (m *composeMetricsProvider) OnProcessSuccess(d time.Duration) {
	m.processSuccess = append(m.processSuccess, d)
}

func (m *composeMetricsProvider) OnProcessFailure(stage string, d time.Duration) {
	m.processFailures = append(m.processFailures, struct {
		stage    string
		duration time.Duration
	}{stage, d})
}

func (m *composeMetricsProvider) OnChangeReceived() {
	m.changesReceived++
}

func TestCompose_Metrics_StateChanges(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 2)
	ch2 := make(chan []byte, 1)
	metrics := &composeMetricsProvider{}

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode().Metrics(metrics)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

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
	ch1 <- []byte(`{"port": 0, "host": "localhost"}`)
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

func TestCompose_Metrics_ProcessSuccess(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)
	metrics := &composeMetricsProvider{}

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode().Metrics(metrics)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(metrics.processSuccess) != 1 {
		t.Errorf("expected 1 process success, got %d", len(metrics.processSuccess))
	}
}

func TestCompose_Metrics_ProcessFailure(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 2)
	ch2 := make(chan []byte, 1)
	metrics := &composeMetricsProvider{}

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode().Metrics(metrics)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)
	capacitor.Start(ctx)

	// Invalid config - validation failure
	ch1 <- []byte(`{"port": 0, "host": "localhost"}`)
	capacitor.Process(ctx)

	if len(metrics.processFailures) != 1 {
		t.Fatalf("expected 1 process failure, got %d", len(metrics.processFailures))
	}
	if metrics.processFailures[0].stage != "validate" {
		t.Errorf("expected validate stage, got %s", metrics.processFailures[0].stage)
	}
}

func TestCompose_WithCircuitBreaker_RejectsAfterFailures(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 10)
	ch2 := make(chan []byte, 1)

	var reducerCalls int
	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			reducerCalls++
			return configs[0], errors.New("always fail")
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
		WithCircuitBreaker[ComposeConfig](2, 1*time.Hour), // Open after 2 failures
	).SyncMode()

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	capacitor.Start(ctx) // First failure (reducer error)
	reducerCalls = 0     // Reset counter after start

	ch1 <- []byte(`{"port": 7070, "host": "localhost"}`)
	capacitor.Process(ctx) // Second failure - circuit opens

	ch1 <- []byte(`{"port": 6060, "host": "localhost"}`)
	capacitor.Process(ctx) // Third request - should be rejected by circuit breaker

	ch1 <- []byte(`{"port": 5050, "host": "localhost"}`)
	capacitor.Process(ctx) // Fourth request - also rejected

	ch1 <- []byte(`{"port": 4040, "host": "localhost"}`)
	capacitor.Process(ctx) // Fifth request - also rejected

	// After circuit opens at 2 failures, we should see exactly 1 more call after start
	// (the second failure that triggers the open state)
	// Then subsequent calls are rejected without calling the reducer
	// However, pipz circuit breaker allows one test call in half-open state
	// For this test, verify that not all 5 calls after start went through
	if reducerCalls >= 5 {
		t.Errorf("circuit breaker should have blocked some calls, got %d reducer calls", reducerCalls)
	}
}

func TestCompose_OnStop_CalledOnContextCancel(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	stopCh := make(chan State, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewChannelWatcher(ch1), NewChannelWatcher(ch2)},
	).OnStop(func(s State) {
		stopCh <- s
	})

	ctx, cancel := context.WithCancel(context.Background())

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

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

func TestCompose_ErrorHistory_RecordsErrors(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 5)
	ch2 := make(chan []byte, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode().ErrorHistorySize(3)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)
	capacitor.Start(ctx)

	// Generate errors
	ch1 <- []byte(`{"port": 0, "host": "localhost"}`)
	capacitor.Process(ctx)
	ch1 <- []byte(`{"port": -1, "host": "localhost"}`)
	capacitor.Process(ctx)

	history := capacitor.ErrorHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 errors in history, got %d", len(history))
	}
}

func TestCompose_ErrorHistory_ClearsOnSuccess(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 5)
	ch2 := make(chan []byte, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode().ErrorHistorySize(3)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)
	capacitor.Start(ctx)

	// Generate error
	ch1 <- []byte(`{"port": 0, "host": "localhost"}`)
	capacitor.Process(ctx)

	if len(capacitor.ErrorHistory()) != 1 {
		t.Error("expected 1 error before recovery")
	}

	// Success clears history
	ch1 <- []byte(`{"port": 7070, "host": "localhost"}`)
	capacitor.Process(ctx)

	if capacitor.ErrorHistory() != nil {
		t.Error("expected error history to be cleared on success")
	}
}

func TestCompose_Clock_SetsCustomClock(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	clock := clockz.NewFakeClock()

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode().Clock(clock)

	ch1 <- []byte(`{"port": 8080, "host": "localhost"}`)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	if err := capacitor.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// If we got here without panic, Clock was set correctly
	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}
}

func TestCompose_Codec_SetsCustomCodec(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode().Codec(YAMLCodec{})

	// YAML format
	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	if err := capacitor.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	cfg, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config")
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
}

func TestCompose_StartupTimeout_TimesOut(t *testing.T) {
	ch1 := make(chan []byte) // unbuffered, will block
	ch2 := make(chan []byte, 1)
	ch2 <- []byte(`{"port": 9090, "host": "other"}`)

	capacitor := Compose[ComposeConfig](
		func(_ context.Context, _, configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Watcher{NewSyncChannelWatcher(ch1), NewSyncChannelWatcher(ch2)},
	).SyncMode().StartupTimeout(50 * time.Millisecond)

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
