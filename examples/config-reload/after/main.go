//go:build ignore

// Config Reload - AFTER
// =====================
// Using flux for validated config reload with automatic rollback.
//
// Improvements demonstrated:
// - Config validated BEFORE applying (via struct tags)
// - Invalid config rejected, previous config retained
// - Clear state machine (Healthy/Degraded/Empty)
// - Capitan signals for observability
// - No boilerplate - just struct tags and a callback
//
// Run: go run .

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/flux"
	"gopkg.in/yaml.v3"
)

// Config uses struct tags for both unmarshaling (yaml) and validation (validate).
// Flux automatically:
// 1. Detects YAML/JSON format
// 2. Unmarshals to the struct
// 3. Validates using go-playground/validator
// 4. Only calls callback if everything passes
type Config struct {
	ServerPort     int    `yaml:"server_port" validate:"min=1,max=65535"`
	DatabaseURL    string `yaml:"database_url" validate:"required"`
	MaxConnections int    `yaml:"max_connections" validate:"min=1"`
}

func main() {
	fmt.Println("=== AFTER: Flux Config Reload ===")
	fmt.Println()

	// Set up observability - see all state changes
	capitan.Hook(flux.CapacitorStateChanged, func(_ context.Context, e *capitan.Event) {
		oldState, _ := flux.KeyOldState.From(e)
		newState, _ := flux.KeyNewState.From(e)
		fmt.Printf("[STATE] %s → %s\n", oldState, newState)
	})

	capitan.Hook(flux.CapacitorValidationFailed, func(_ context.Context, e *capitan.Event) {
		errMsg, _ := flux.KeyError.From(e)
		fmt.Printf("[REJECTED] %s\n", errMsg)
	})

	capitan.Hook(flux.CapacitorApplySucceeded, func(_ context.Context, e *capitan.Event) {
		fmt.Println("[APPLIED] Config updated successfully")
	})

	// Create temp config file
	dir, _ := os.MkdirTemp("", "config-demo")
	defer os.RemoveAll(dir)
	configPath := filepath.Join(dir, "config.yaml")

	// Write initial valid config
	writeConfig(configPath, Config{
		ServerPort:     8080,
		DatabaseURL:    "postgres://localhost/app",
		MaxConnections: 100,
	})

	// Create capacitor - just provide watcher and callback!
	// Flux handles unmarshaling and validation automatically via struct tags.
	capacitor := flux.New[Config](
		flux.NewFileWatcher(configPath),

		// Callback: Only called with valid, parsed config
		func(cfg Config) error {
			fmt.Printf("         port=%d, max_conn=%d\n", cfg.ServerPort, cfg.MaxConnections)
			return nil
		},

		flux.WithDebounce(50*time.Millisecond),
	)

	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := capacitor.Start(ctx); err != nil {
		fmt.Printf("Initial load failed: %v\n", err)
	}

	fmt.Printf("\nInitial state: %s\n", capacitor.State())

	// Simulate config changes
	go func() {
		time.Sleep(300 * time.Millisecond)

		// Write INVALID config
		fmt.Println("\n--- Writing invalid config (port: -1) ---")
		writeConfig(configPath, Config{
			ServerPort:     -1, // Invalid! (validate:"min=1")
			DatabaseURL:    "postgres://localhost/app",
			MaxConnections: 50,
		})

		time.Sleep(300 * time.Millisecond)

		// Check state - should be Degraded, not crashed
		fmt.Printf("\nCurrent state: %s\n", capacitor.State())
		if cfg, ok := capacitor.Current(); ok {
			fmt.Printf("Current config still valid: port=%d\n", cfg.ServerPort)
		}

		time.Sleep(300 * time.Millisecond)

		// Write valid config
		fmt.Println("\n--- Writing valid config (port: 9090) ---")
		writeConfig(configPath, Config{
			ServerPort:     9090,
			DatabaseURL:    "postgres://localhost/app",
			MaxConnections: 200,
		})
	}()

	// Wait for demo
	<-ctx.Done()

	// Clean shutdown
	capitan.Shutdown()

	fmt.Println("\n--- Demo complete ---")
	fmt.Println()
	fmt.Println("Improvements demonstrated:")
	fmt.Println("- Invalid config REJECTED (never applied)")
	fmt.Println("- Previous valid config RETAINED during failure")
	fmt.Println("- Clear state: Healthy → Degraded → Healthy")
	fmt.Println("- Application never saw invalid port=-1")
	fmt.Println("- Zero boilerplate: just struct tags!")
}

func writeConfig(path string, cfg Config) {
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(path, data, 0644)
}
