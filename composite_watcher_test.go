package flux

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// ComposeConfig is a test config for Compose tests.
type ComposeConfig struct {
	Port    int    `yaml:"port" json:"port" validate:"min=1,max=65535"`
	Host    string `yaml:"host" json:"host" validate:"required"`
	Timeout int    `yaml:"timeout" json:"timeout"`
}

func TestCompose_MergesTwoSources(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	var applied []ComposeConfig
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			applied = configs
			return configs[1], nil // return second as merged result
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	// Send to both sources
	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: override")

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

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
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
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	// Defaults
	ch1 <- []byte("port: 8080\nhost: localhost\ntimeout: 30")
	// Overrides (only port)
	ch2 <- []byte("port: 9090\nhost: override\ntimeout: 0")

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

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	// First valid, second invalid (port 0)
	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 0\nhost: localhost") // Invalid: port 0

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
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			applied = configs
			return configs[2], nil // return last as merged
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
		NewSyncChannelWatcher(ch3),
	)

	ch1 <- []byte("port: 1\nhost: one")
	ch2 <- []byte("port: 2\nhost: two")
	ch3 <- []byte("port: 3\nhost: three")

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
	capacitor := ComposeWithOptions[ComposeConfig](
		func(_ []ComposeConfig) (ComposeConfig, error) {
			return ComposeConfig{}, nil
		},
		[]Option{WithSyncMode()},
		// No sources
	)

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Error("expected error for empty sources")
	}
}

func TestCompose_SingleSource(t *testing.T) {
	ctx := context.Background()
	ch := make(chan []byte, 1)

	var applied []ComposeConfig
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			applied = configs
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch),
	)

	ch <- []byte("port: 8080\nhost: single")

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
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			lastApplied = configs
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	// Initial values
	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Update only first source
	ch1 <- []byte("port: 7070\nhost: updated")
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
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			lastApplied = configs
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	// Valid initial values
	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if capacitor.State() != StateHealthy {
		t.Fatalf("expected healthy, got %s", capacitor.State())
	}

	// Update first source with invalid value
	ch1 <- []byte("port: 0\nhost: localhost") // Invalid port
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
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			applied = configs
			return configs[1], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	// Mixed: first YAML, second JSON
	ch1 <- []byte("port: 8080\nhost: yaml-source")
	ch2 <- []byte(`{"port": 9090, "host": "json-source"}`)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if applied[0].Host != "yaml-source" {
		t.Errorf("expected yaml-source, got %s", applied[0].Host)
	}
	if applied[1].Host != "json-source" {
		t.Errorf("expected json-source, got %s", applied[1].Host)
	}
}

func TestCompose_Wrapper(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	// Use Compose directly (not ComposeWithOptions)
	capacitor := Compose[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	// Start will block waiting for debounce in async mode,
	// but initial processing is synchronous
	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if capacitor.State() != StateHealthy {
		t.Errorf("expected healthy, got %s", capacitor.State())
	}
}

func TestCompose_LastError(t *testing.T) {
	ctx := context.Background()
	ch1 := make(chan []byte, 2)
	ch2 := make(chan []byte, 1)

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	// Valid initial values
	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// LastError should be nil after success
	if capacitor.LastError() != nil {
		t.Errorf("expected nil error after success, got %v", capacitor.LastError())
	}

	// Update with invalid value
	ch1 <- []byte("port: 0\nhost: localhost") // Invalid port
	capacitor.Process(ctx)

	// LastError should now be set
	if capacitor.LastError() == nil {
		t.Error("expected error after validation failure")
	}
}

func TestCompose_CurrentBeforeStart(t *testing.T) {
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(make(chan []byte)),
	)

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

	capacitor := ComposeWithOptions[ComposeConfig](
		func(_ []ComposeConfig) (ComposeConfig, error) {
			return ComposeConfig{}, fmt.Errorf("reducer failed intentionally")
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

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
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			lastPort.Store(int32(configs[0].Port)) //nolint:gosec // Port validated to 1-65535
			return configs[0], nil
		},
		[]Option{WithDebounce(10 * time.Millisecond)}, // Short debounce for test
		NewChannelWatcher(ch1),
		NewChannelWatcher(ch2),
	)

	// Initial values
	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if lastPort.Load() != 8080 {
		t.Errorf("expected initial port 8080, got %d", lastPort.Load())
	}

	// Send update through first channel
	ch1 <- []byte("port: 7070\nhost: updated")

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

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithDebounce(10 * time.Millisecond)},
		NewChannelWatcher(ch1),
		NewChannelWatcher(ch2),
	)

	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

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

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

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

	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		nil, // No options, async mode
		NewChannelWatcher(ch1),
		NewChannelWatcher(ch2),
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

	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

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

	ch1 <- []byte("port: 8080\nhost: localhost")
	close(ch2) // Close before sending

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when source closes during init")
	}
}

func TestCompose_UnmarshalError(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("not valid yaml: {{{}}")

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

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
	ch <- []byte("port: 8080\nhost: localhost")

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch),
		&errorWatcher{err: fmt.Errorf("watcher failed")},
	)

	err := capacitor.Start(context.Background())
	if err == nil {
		t.Fatal("expected watcher error")
	}
}

func TestCompose_CannotStartTwice(t *testing.T) {
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)

	ch1 <- []byte("port: 8080\nhost: localhost")
	ch2 <- []byte("port: 9090\nhost: other")

	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			return configs[0], nil
		},
		[]Option{WithSyncMode()},
		NewSyncChannelWatcher(ch1),
		NewSyncChannelWatcher(ch2),
	)

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

	ch1 <- []byte("port: 1\nhost: localhost")
	ch2 <- []byte("port: 2\nhost: other")

	var lastPort atomic.Int32
	capacitor := ComposeWithOptions[ComposeConfig](
		func(configs []ComposeConfig) (ComposeConfig, error) {
			lastPort.Store(int32(configs[0].Port)) //nolint:gosec // Port validated to 1-65535
			return configs[0], nil
		},
		[]Option{WithDebounce(10 * time.Millisecond)},
		NewChannelWatcher(ch1),
		NewChannelWatcher(ch2),
	)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send multiple rapid updates to trigger debounce timer reset
	for i := 3; i <= 5; i++ {
		ch1 <- []byte(fmt.Sprintf("port: %d\nhost: localhost", i))
		time.Sleep(5 * time.Millisecond) // Less than debounce
	}

	// Wait for final debounce
	time.Sleep(50 * time.Millisecond)

	// Should have coalesced to final value
	if lastPort.Load() != 5 {
		t.Errorf("expected final port 5, got %d", lastPort.Load())
	}
}
