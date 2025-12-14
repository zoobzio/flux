package flux

import (
	"testing"
	"time"
)

func TestNoOpMetricsProvider_DoesNotPanic(_ *testing.T) {
	var m NoOpMetricsProvider

	// These should not panic
	m.OnStateChange(StateLoading, StateHealthy)
	m.OnProcessSuccess(100 * time.Millisecond)
	m.OnProcessFailure("validate", 50*time.Millisecond)
	m.OnChangeReceived()
}
