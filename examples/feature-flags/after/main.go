//go:build ignore

// Feature Flags - AFTER
// =====================
// Using flux for safe live feature flags with validation.
//
// Improvements demonstrated:
// - Flag constraints validated via struct tags
// - Invalid flags rejected, previous retained
// - Audit trail via capitan signals
// - Degraded state visible for alerting
// - No boilerplate - struct tags do the work
//
// Run: go run .

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/flux"
)

// FeatureFlags uses struct tags for unmarshaling (json) and validation (validate).
// For complex validation like flag dependencies, the callback can return an error.
type FeatureFlags struct {
	EnableNewUI       bool `json:"enable_new_ui"`
	EnableNewCart     bool `json:"enable_new_cart"`
	EnableNewCheckout bool `json:"enable_new_checkout"`
	MaxItemsInCart    int  `json:"max_items_in_cart" validate:"min=1"`
}

func main() {
	fmt.Println("=== AFTER: Flux Feature Flags ===")
	fmt.Println()

	// Audit trail - log all flag changes
	capitan.Hook(flux.CapacitorApplySucceeded, func(_ context.Context, e *capitan.Event) {
		fmt.Printf("[AUDIT] Flags updated at %s\n", time.Now().Format("15:04:05"))
	})

	capitan.Hook(flux.CapacitorValidationFailed, func(_ context.Context, e *capitan.Event) {
		errMsg, _ := flux.KeyError.From(e)
		fmt.Printf("[AUDIT] Flags REJECTED: %s\n", errMsg)
	})

	capitan.Hook(flux.CapacitorApplyFailed, func(_ context.Context, e *capitan.Event) {
		errMsg, _ := flux.KeyError.From(e)
		fmt.Printf("[AUDIT] Flags REJECTED (business rule): %s\n", errMsg)
	})

	capitan.Hook(flux.CapacitorStateChanged, func(_ context.Context, e *capitan.Event) {
		newState, _ := flux.KeyNewState.From(e)
		if newState == "degraded" {
			fmt.Println("[ALERT] Flag configuration degraded - using previous flags")
		}
	})

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

	// Create capacitor - struct tags handle unmarshaling and basic validation.
	// The callback can enforce complex business rules and return errors.
	capacitor := flux.New[FeatureFlags](
		flux.NewFileWatcher(flagsPath),

		// Callback: Receives valid, parsed flags
		// Can still enforce business rules by returning errors
		func(f FeatureFlags) error {
			// Business rule: checkout requires cart
			if f.EnableNewCheckout && !f.EnableNewCart {
				return errors.New("enable_new_checkout requires enable_new_cart")
			}

			fmt.Printf("         ui=%v cart=%v checkout=%v max_items=%d\n",
				f.EnableNewUI, f.EnableNewCart, f.EnableNewCheckout, f.MaxItemsInCart)
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

	// Simulate flag changes
	go func() {
		time.Sleep(300 * time.Millisecond)

		// Write INVALID combination
		fmt.Println("\n--- Writing invalid flags (checkout=true, cart=false) ---")
		writeFlags(flagsPath, FeatureFlags{
			EnableNewUI:       true,
			EnableNewCart:     false,
			EnableNewCheckout: true, // Invalid: needs cart! (business rule)
			MaxItemsInCart:    50,
		})

		time.Sleep(300 * time.Millisecond)

		// Check that previous flags are retained
		if f, ok := capacitor.Current(); ok {
			fmt.Printf("\nActive flags (previous): cart=%v checkout=%v max_items=%d\n",
				f.EnableNewCart, f.EnableNewCheckout, f.MaxItemsInCart)
			fmt.Println("Users still have working checkout!")
		}

		time.Sleep(300 * time.Millisecond)

		// Write another invalid - this time struct tag validation
		fmt.Println("\n--- Writing invalid flags (max_items=0) ---")
		writeFlags(flagsPath, FeatureFlags{
			EnableNewUI:       true,
			EnableNewCart:     true,
			EnableNewCheckout: false,
			MaxItemsInCart:    0, // Invalid: min=1 struct tag
		})

		time.Sleep(300 * time.Millisecond)

		// Write valid flags
		fmt.Println("\n--- Writing valid flags ---")
		writeFlags(flagsPath, FeatureFlags{
			EnableNewUI:       true,
			EnableNewCart:     true,
			EnableNewCheckout: true,
			MaxItemsInCart:    100,
		})
	}()

	// Wait for demo
	<-ctx.Done()

	// Final state
	fmt.Printf("\nFinal state: %s\n", capacitor.State())

	capitan.Shutdown()

	fmt.Println("\n--- Demo complete ---")
	fmt.Println()
	fmt.Println("Improvements demonstrated:")
	fmt.Println("- Struct tags for basic validation (max_items >= 1)")
	fmt.Println("- Callback for business rules (checkout requires cart)")
	fmt.Println("- Previous working flags RETAINED on any failure")
	fmt.Println("- Users never saw broken checkout")
	fmt.Println("- Audit trail of all changes")
}

func writeFlags(path string, f FeatureFlags) {
	data, _ := json.MarshalIndent(f, "", "  ")
	os.WriteFile(path, data, 0644)
}
