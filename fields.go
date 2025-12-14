package flux

import "github.com/zoobzio/capitan"

// Field keys for Capacitor events.
var (
	// KeyState is the current state of the Capacitor.
	KeyState = capitan.NewStringKey("state")

	// KeyOldState is the previous state before a transition.
	KeyOldState = capitan.NewStringKey("old_state")

	// KeyNewState is the new state after a transition.
	KeyNewState = capitan.NewStringKey("new_state")

	// KeyError is the error message when an operation fails.
	KeyError = capitan.NewStringKey("error")

	// KeyDebounce is the configured debounce duration.
	KeyDebounce = capitan.NewDurationKey("debounce")
)
