package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/flux"
	"github.com/zoobzio/flux/pkg/file"
)

type appConfig struct {
	Feature string `json:"feature" yaml:"feature"`
	Limit   int    `json:"limit" yaml:"limit"`
}

// Validate implements the flux.Validator interface.
func (c appConfig) Validate() error {
	if c.Feature == "" {
		return errors.New("feature is required")
	}
	if c.Limit < 0 {
		return fmt.Errorf("limit must be >= 0, got %d", c.Limit)
	}
	return nil
}

func TestCapacitor_FileWatcher_InitialLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := appConfig{Feature: "test", Limit: 100}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var applied appConfig

	capacitor := flux.New[appConfig](
		file.New(path),
		func(_ context.Context, _, cfg appConfig) error {
			applied = cfg
			return nil
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if capacitor.State() != flux.StateHealthy {
		t.Errorf("expected StateHealthy, got %s", capacitor.State())
	}

	if applied.Feature != "test" || applied.Limit != 100 {
		t.Errorf("unexpected applied config: %+v", applied)
	}
}

func TestCapacitor_FileWatcher_LiveUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := appConfig{Feature: "v1", Limit: 10}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var applyCount atomic.Int32
	var lastApplied atomic.Value

	capacitor := flux.New[appConfig](
		file.New(path),
		func(_ context.Context, _, cfg appConfig) error {
			applyCount.Add(1)
			lastApplied.Store(cfg)
			return nil
		},
		flux.WithDebounce[appConfig](50*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Initial apply
	if applyCount.Load() != 1 {
		t.Errorf("expected 1 apply after start, got %d", applyCount.Load())
	}

	// Update config file
	cfg2 := appConfig{Feature: "v2", Limit: 20}
	data2, err := json.Marshal(cfg2)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data2, 0o600); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Wait for debounced apply
	time.Sleep(200 * time.Millisecond)

	if applyCount.Load() != 2 {
		t.Errorf("expected 2 applies, got %d", applyCount.Load())
	}

	applied := lastApplied.Load().(appConfig)
	if applied.Feature != "v2" || applied.Limit != 20 {
		t.Errorf("unexpected applied config: %+v", applied)
	}
}

func TestCapacitor_FileWatcher_InvalidUpdateRetainsPrevious(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := appConfig{Feature: "valid", Limit: 50}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var lastApplied atomic.Value

	capacitor := flux.New[appConfig](
		file.New(path),
		func(_ context.Context, _, cfg appConfig) error {
			lastApplied.Store(cfg)
			return nil
		},
		flux.WithDebounce[appConfig](50*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Write invalid config (limit -1 violates min=0)
	invalidCfg := appConfig{Feature: "invalid", Limit: -1}
	invalidData, err := json.Marshal(invalidCfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, invalidData, 0o600); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Wait for debounced processing
	time.Sleep(200 * time.Millisecond)

	// Should be degraded
	if capacitor.State() != flux.StateDegraded {
		t.Errorf("expected StateDegraded, got %s", capacitor.State())
	}

	// Previous config should still be current
	current, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config to exist")
	}
	if current.Feature != "valid" || current.Limit != 50 {
		t.Errorf("expected previous config retained, got %+v", current)
	}

	// LastError should be set
	if capacitor.LastError() == nil {
		t.Error("expected LastError to be set")
	}
}

func TestCapacitor_FileWatcher_RecoveryFromDegraded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := appConfig{Feature: "v1", Limit: 10}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	capacitor := flux.New[appConfig](
		file.New(path),
		func(_ context.Context, _, _ appConfig) error {
			return nil
		},
		flux.WithDebounce[appConfig](50*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Write invalid (limit -1 violates min=0)
	invalidCfg := appConfig{Feature: "bad", Limit: -1}
	invalidData, err := json.Marshal(invalidCfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, invalidData, 0o600); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if capacitor.State() != flux.StateDegraded {
		t.Errorf("expected StateDegraded, got %s", capacitor.State())
	}

	// Write valid again
	validCfg := appConfig{Feature: "recovered", Limit: 99}
	validData, err := json.Marshal(validCfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, validData, 0o600); err != nil {
		t.Fatalf("failed to write valid config: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if capacitor.State() != flux.StateHealthy {
		t.Errorf("expected StateHealthy after recovery, got %s", capacitor.State())
	}

	current, _ := capacitor.Current()
	if current.Feature != "recovered" {
		t.Errorf("expected 'recovered', got %s", current.Feature)
	}
}

func TestCapacitor_FileWatcher_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := appConfig{Feature: "valid", Limit: 10}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	capacitor := flux.New[appConfig](
		file.New(path),
		func(_ context.Context, _, _ appConfig) error {
			return nil
		},
		flux.WithDebounce[appConfig](50*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Write malformed JSON
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("failed to write malformed config: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Should be degraded due to unmarshal failure
	if capacitor.State() != flux.StateDegraded {
		t.Errorf("expected StateDegraded, got %s", capacitor.State())
	}

	// Original config retained
	current, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config")
	}
	if current.Feature != "valid" {
		t.Errorf("expected 'valid', got %s", current.Feature)
	}
}
