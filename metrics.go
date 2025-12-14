package flux

import "time"

// MetricsProvider allows integration with metrics systems like Prometheus, StatsD, etc.
// Implement this interface to receive callbacks on key capacitor events.
type MetricsProvider interface {
	// OnStateChange is called when the capacitor transitions between states.
	OnStateChange(from, to State)

	// OnProcessSuccess is called when a configuration is successfully processed.
	// Duration is the time taken to process (unmarshal, validate, callback).
	OnProcessSuccess(duration time.Duration)

	// OnProcessFailure is called when processing fails at any stage.
	// Stage indicates where the failure occurred: "unmarshal", "validate", or "callback".
	OnProcessFailure(stage string, duration time.Duration)

	// OnChangeReceived is called when raw data is received from the watcher.
	OnChangeReceived()
}

// NoOpMetricsProvider is a no-op implementation of MetricsProvider.
// Use this as an embedded type to implement only the methods you need.
type NoOpMetricsProvider struct{}

func (NoOpMetricsProvider) OnStateChange(_, _ State)                   {}
func (NoOpMetricsProvider) OnProcessSuccess(_ time.Duration)           {}
func (NoOpMetricsProvider) OnProcessFailure(_ string, _ time.Duration) {}
func (NoOpMetricsProvider) OnChangeReceived()                          {}
