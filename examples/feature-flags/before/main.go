//go:build ignore

// Feature Flags - BEFORE
// ======================
// The typical approach: static flags or unsafe live reload.
//
// Problems demonstrated:
// - Invalid flag combinations applied without validation
// - No rollback when bad flags break functionality
// - No audit trail of flag changes
//
// Run: go run .

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type FeatureFlags struct {
	EnableNewUI       bool `json:"enable_new_ui"`
	EnableNewCart     bool `json:"enable_new_cart"`
	EnableNewCheckout bool `json:"enable_new_checkout"`
	MaxItemsInCart    int  `json:"max_items_in_cart"`
}

var flags FeatureFlags

func main() {
	fmt.Println("=== BEFORE: Unsafe Feature Flags ===")
	fmt.Println()

	// Create temp flags file
	dir, _ := os.MkdirTemp("", "flags-demo")
	defer os.RemoveAll(dir)
	flagsPath := filepath.Join(dir, "flags.json")

	// Write initial valid flags
	writeFlags(flagsPath, FeatureFlags{
		EnableNewUI:       true,
		EnableNewCart:     true,
		EnableNewCheckout: false,
		MaxItemsInCart:    50,
	})

	// Start watching
	watcher, _ := fsnotify.NewWatcher()
	defer watcher.Close()
	watcher.Add(flagsPath)

	// Load initial
	loadFlags(flagsPath)
	printFlags("Initial")

	// Simulate flag changes
	go func() {
		time.Sleep(500 * time.Millisecond)

		// Write INVALID combination: new_checkout requires new_cart
		fmt.Println("\n--- Writing invalid flags (checkout=true, cart=false) ---")
		writeFlags(flagsPath, FeatureFlags{
			EnableNewUI:       true,
			EnableNewCart:     false, // Disabled!
			EnableNewCheckout: true,  // But this needs cart!
			MaxItemsInCart:    -10,   // Also invalid
		})

		time.Sleep(500 * time.Millisecond)

		// Write valid flags
		fmt.Println("\n--- Writing valid flags ---")
		writeFlags(flagsPath, FeatureFlags{
			EnableNewUI:       true,
			EnableNewCart:     true,
			EnableNewCheckout: true,
			MaxItemsInCart:    100,
		})
	}()

	// Watch for changes
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Write != 0 {
				loadFlags(flagsPath)
				printFlags("Applied")

				// Problem: Check AFTER applying
				if flags.EnableNewCheckout && !flags.EnableNewCart {
					fmt.Println("BUG: Checkout enabled without cart!")
					fmt.Println("     Users will see errors at checkout...")
				}
				if flags.MaxItemsInCart < 0 {
					fmt.Println("BUG: Negative max items!")
					fmt.Println("     Cart will reject all items...")
				}
			}
		case <-timeout:
			fmt.Println("\n--- Demo complete ---")
			fmt.Println()
			fmt.Println("Problems demonstrated:")
			fmt.Println("- Invalid flag combination was applied")
			fmt.Println("- Users experienced broken checkout")
			fmt.Println("- No automatic rollback to working flags")
			fmt.Println("- No audit trail of what changed")
			return
		}
	}
}

func loadFlags(path string) {
	data, _ := os.ReadFile(path)
	json.Unmarshal(data, &flags) // No validation!
}

func writeFlags(path string, f FeatureFlags) {
	data, _ := json.MarshalIndent(f, "", "  ")
	os.WriteFile(path, data, 0644)
}

func printFlags(label string) {
	fmt.Printf("%s: ui=%v cart=%v checkout=%v max_items=%d\n",
		label, flags.EnableNewUI, flags.EnableNewCart, flags.EnableNewCheckout, flags.MaxItemsInCart)
}
