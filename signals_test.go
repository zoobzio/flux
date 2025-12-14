package flux

import "testing"

func TestCapacitorStarted(t *testing.T) {
	if CapacitorStarted.Name() != "flux.capacitor.started" {
		t.Errorf("expected name 'flux.capacitor.started', got %q", CapacitorStarted.Name())
	}
}

func TestCapacitorStopped(t *testing.T) {
	if CapacitorStopped.Name() != "flux.capacitor.stopped" {
		t.Errorf("expected name 'flux.capacitor.stopped', got %q", CapacitorStopped.Name())
	}
}

func TestCapacitorStateChanged(t *testing.T) {
	if CapacitorStateChanged.Name() != "flux.capacitor.state.changed" {
		t.Errorf("expected name 'flux.capacitor.state.changed', got %q", CapacitorStateChanged.Name())
	}
}

func TestCapacitorChangeReceived(t *testing.T) {
	if CapacitorChangeReceived.Name() != "flux.capacitor.change.received" {
		t.Errorf("expected name 'flux.capacitor.change.received', got %q", CapacitorChangeReceived.Name())
	}
}

func TestCapacitorTransformFailed(t *testing.T) {
	if CapacitorTransformFailed.Name() != "flux.capacitor.transform.failed" {
		t.Errorf("expected name 'flux.capacitor.transform.failed', got %q", CapacitorTransformFailed.Name())
	}
}

func TestCapacitorValidationFailed(t *testing.T) {
	if CapacitorValidationFailed.Name() != "flux.capacitor.validation.failed" {
		t.Errorf("expected name 'flux.capacitor.validation.failed', got %q", CapacitorValidationFailed.Name())
	}
}

func TestCapacitorApplyFailed(t *testing.T) {
	if CapacitorApplyFailed.Name() != "flux.capacitor.apply.failed" {
		t.Errorf("expected name 'flux.capacitor.apply.failed', got %q", CapacitorApplyFailed.Name())
	}
}

func TestCapacitorApplySucceeded(t *testing.T) {
	if CapacitorApplySucceeded.Name() != "flux.capacitor.apply.succeeded" {
		t.Errorf("expected name 'flux.capacitor.apply.succeeded', got %q", CapacitorApplySucceeded.Name())
	}
}
