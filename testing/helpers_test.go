package testing

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

func TestTestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TestConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  TestConfig{Port: 8080, Host: "localhost", Timeout: 30},
			wantErr: false,
		},
		{
			name:    "port too low",
			config:  TestConfig{Port: 0, Host: "localhost"},
			wantErr: true,
		},
		{
			name:    "port too high",
			config:  TestConfig{Port: 70000, Host: "localhost"},
			wantErr: true,
		},
		{
			name:    "empty host",
			config:  TestConfig{Port: 8080, Host: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWaitFor(t *testing.T) {
	t.Run("condition met immediately", func(t *testing.T) {
		result := WaitFor(t, 100*time.Millisecond, func() bool {
			return true
		})
		if !result {
			t.Error("expected WaitFor to return true")
		}
	})

	t.Run("condition never met", func(t *testing.T) {
		result := WaitFor(t, 50*time.Millisecond, func() bool {
			return false
		})
		if result {
			t.Error("expected WaitFor to return false on timeout")
		}
	})

	t.Run("condition met after delay", func(t *testing.T) {
		start := time.Now()
		met := false
		go func() {
			time.Sleep(30 * time.Millisecond)
			met = true
		}()
		result := WaitFor(t, 100*time.Millisecond, func() bool {
			return met
		})
		if !result {
			t.Error("expected WaitFor to return true")
		}
		if time.Since(start) < 30*time.Millisecond {
			t.Error("condition should have taken at least 30ms")
		}
	})
}

func TestWaitForState(t *testing.T) {
	ch := make(chan []byte, 1)
	c := flux.New[TestConfig](
		flux.NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !WaitForState(t, c, flux.StateHealthy, 100*time.Millisecond) {
		t.Error("expected capacitor to reach healthy state")
	}
}

func TestRequireState(t *testing.T) {
	ch := make(chan []byte, 1)
	c := flux.New[TestConfig](
		flux.NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should not panic/fail for correct state.
	RequireState(t, c, flux.StateHealthy)
}

func TestRequireConfig(t *testing.T) {
	ch := make(chan []byte, 1)
	c := flux.New[TestConfig](
		flux.NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ TestConfig) error { return nil },
	).SyncMode()

	ch <- []byte(`{"port": 8080, "host": "localhost"}`)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	RequireConfig(t, c, func(cfg TestConfig) bool {
		return cfg.Port == 8080 && cfg.Host == "localhost"
	})
}

func TestNewTestCapacitor(t *testing.T) {
	var received TestConfig
	c, ch := NewTestCapacitor(t, func(_ context.Context, _, curr TestConfig) error {
		received = curr
		return nil
	})

	ch <- []byte(`{"port": 9090, "host": "example.com"}`)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if received.Port != 9090 {
		t.Errorf("expected port 9090, got %d", received.Port)
	}
	if received.Host != "example.com" {
		t.Errorf("expected host example.com, got %s", received.Host)
	}
}
