//go:build ignore

// Config Reload - BEFORE
// ======================
// The typical approach: manual file watching with no validation or rollback.
//
// Problems demonstrated:
// - Invalid config crashes or corrupts application state
// - No rollback - once applied, bad config stays
// - Parse errors leave config in unknown state
// - No visibility into config health
//
// Run: go run .

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ServerPort     int    `yaml:"server_port"`
	DatabaseURL    string `yaml:"database_url"`
	MaxConnections int    `yaml:"max_connections"`
}

var currentConfig Config

func main() {
	fmt.Println("=== BEFORE: Manual Config Reload ===")
	fmt.Println()

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

	// Start watching
	watcher, _ := fsnotify.NewWatcher()
	defer watcher.Close()
	watcher.Add(configPath)

	// Load initial config
	loadConfig(configPath)
	fmt.Printf("Initial config: port=%d, max_conn=%d\n", currentConfig.ServerPort, currentConfig.MaxConnections)

	// Simulate config changes in background
	go func() {
		time.Sleep(500 * time.Millisecond)

		// Write INVALID config - negative port
		fmt.Println("\n--- Writing invalid config (port: -1) ---")
		writeConfig(configPath, Config{
			ServerPort:     -1, // Invalid!
			DatabaseURL:    "postgres://localhost/app",
			MaxConnections: 50,
		})

		time.Sleep(500 * time.Millisecond)

		// Write valid config again
		fmt.Println("\n--- Writing valid config (port: 9090) ---")
		writeConfig(configPath, Config{
			ServerPort:     9090,
			DatabaseURL:    "postgres://localhost/app",
			MaxConnections: 200,
		})
	}()

	// Watch for changes
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Write != 0 {
				loadConfig(configPath)
				// Problem: We applied the config without validating!
				fmt.Printf("Config applied: port=%d, max_conn=%d\n", currentConfig.ServerPort, currentConfig.MaxConnections)

				// Problem: Now we check validity AFTER applying
				if currentConfig.ServerPort < 0 {
					fmt.Println("WARNING: Invalid port! But config is already applied...")
					fmt.Println("         Application may be in broken state!")
				}
			}
		case err := <-watcher.Errors:
			fmt.Printf("Watcher error: %v\n", err)
		case <-timeout:
			fmt.Println("\n--- Demo complete ---")
			fmt.Println()
			fmt.Println("Problems demonstrated:")
			fmt.Println("- Invalid config was applied (port: -1)")
			fmt.Println("- No automatic rollback to previous valid config")
			fmt.Println("- Application state corrupted until next valid config")
			fmt.Println("- No clear visibility into config health")
			return
		}
	}
}

func loadConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Read error: %v\n", err)
		return
	}

	// Problem: We just overwrite the config, even if parse fails partially
	if err := yaml.Unmarshal(data, &currentConfig); err != nil {
		fmt.Printf("Parse error: %v\n", err)
		// Config may be partially updated!
	}
}

func writeConfig(path string, cfg Config) {
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(path, data, 0644)
}
