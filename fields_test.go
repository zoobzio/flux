package flux

import (
	"testing"
	"time"
)

func TestKeyState(t *testing.T) {
	field := KeyState.Field("healthy")
	if field.Key().Name() != "state" {
		t.Errorf("expected key 'state', got %q", field.Key().Name())
	}
}

func TestKeyOldState(t *testing.T) {
	field := KeyOldState.Field("loading")
	if field.Key().Name() != "old_state" {
		t.Errorf("expected key 'old_state', got %q", field.Key().Name())
	}
}

func TestKeyNewState(t *testing.T) {
	field := KeyNewState.Field("healthy")
	if field.Key().Name() != "new_state" {
		t.Errorf("expected key 'new_state', got %q", field.Key().Name())
	}
}

func TestKeyError(t *testing.T) {
	field := KeyError.Field("something went wrong")
	if field.Key().Name() != "error" {
		t.Errorf("expected key 'error', got %q", field.Key().Name())
	}
}

func TestKeyDebounce(t *testing.T) {
	field := KeyDebounce.Field(100 * time.Millisecond)
	if field.Key().Name() != "debounce" {
		t.Errorf("expected key 'debounce', got %q", field.Key().Name())
	}
}
