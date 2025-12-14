package integration

import (
	"testing"
	"time"
)

// waitFor polls a condition until it returns true or timeout is reached.
// Uses short polling intervals for fast tests with reliable results.
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) bool {
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
