package flux

import "github.com/zoobzio/capitan"

// Capacitor lifecycle signals.
var (
	// CapacitorStarted is emitted when a Capacitor begins watching.
	CapacitorStarted = capitan.NewSignal(
		"flux.capacitor.started",
		"Capacitor watching started",
	)

	// CapacitorStopped is emitted when a Capacitor stops watching.
	CapacitorStopped = capitan.NewSignal(
		"flux.capacitor.stopped",
		"Capacitor watching stopped",
	)

	// CapacitorStateChanged is emitted when a Capacitor transitions between states.
	CapacitorStateChanged = capitan.NewSignal(
		"flux.capacitor.state.changed",
		"Capacitor state transition",
	)
)

// Change processing signals.
var (
	// CapacitorChangeReceived is emitted when raw data is received from the watcher.
	CapacitorChangeReceived = capitan.NewSignal(
		"flux.capacitor.change.received",
		"Raw change received from watcher",
	)

	// CapacitorTransformFailed is emitted when the transform function fails.
	CapacitorTransformFailed = capitan.NewSignal(
		"flux.capacitor.transform.failed",
		"Transform function failed",
	)

	// CapacitorValidationFailed is emitted when validation fails.
	CapacitorValidationFailed = capitan.NewSignal(
		"flux.capacitor.validation.failed",
		"Validation failed",
	)

	// CapacitorApplyFailed is emitted when the apply function fails.
	CapacitorApplyFailed = capitan.NewSignal(
		"flux.capacitor.apply.failed",
		"Apply function failed",
	)

	// CapacitorApplySucceeded is emitted when configuration is successfully applied.
	CapacitorApplySucceeded = capitan.NewSignal(
		"flux.capacitor.apply.succeeded",
		"Config applied successfully",
	)
)
