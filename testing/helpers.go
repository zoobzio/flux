// Package testing provides test utilities and helpers for flux capacitor testing.
package testing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

// TestConfig is a standard configuration type for testing flux capacitors.
// It implements flux.Validator with configurable validation behavior.
type TestConfig struct {
	Port    int    `yaml:"port" json:"port"`
	Host    string `yaml:"host" json:"host"`
	Timeout int    `yaml:"timeout" json:"timeout"`
}

// Validate implements flux.Validator.
func (c TestConfig) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if c.Host == "" {
		return errors.New("host is required")
	}
	return nil
}

// WaitFor polls a condition until it returns true or timeout is reached.
// Returns true if the condition was met, false if timeout occurred.
func WaitFor(t *testing.T, timeout time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// WaitForState waits until the capacitor reaches the expected state or timeout occurs.
func WaitForState[T flux.Validator](t *testing.T, c *flux.Capacitor[T], expected flux.State, timeout time.Duration) bool {
	t.Helper()
	return WaitFor(t, timeout, func() bool {
		return c.State() == expected
	})
}

// RequireState fails the test immediately if the capacitor is not in the expected state.
func RequireState[T flux.Validator](t *testing.T, c *flux.Capacitor[T], expected flux.State) {
	t.Helper()
	if got := c.State(); got != expected {
		t.Fatalf("expected state %s, got %s", expected, got)
	}
}

// RequireConfig fails the test if Current() returns false or the config doesn't match.
func RequireConfig[T flux.Validator](t *testing.T, c *flux.Capacitor[T], check func(T) bool) {
	t.Helper()
	cfg, ok := c.Current()
	if !ok {
		t.Fatal("expected config to be present, got none")
	}
	if !check(cfg) {
		t.Fatalf("config check failed: %+v", cfg)
	}
}

// NewTestCapacitor creates a capacitor with a sync channel watcher for testing.
// Returns the capacitor and a channel for sending test data.
func NewTestCapacitor(t *testing.T, callback func(context.Context, TestConfig, TestConfig) error) (*flux.Capacitor[TestConfig], chan<- []byte) {
	t.Helper()
	ch := make(chan []byte, 10)
	c := flux.New[TestConfig](
		flux.NewSyncChannelWatcher(ch),
		callback,
	).SyncMode()
	return c, ch
}
