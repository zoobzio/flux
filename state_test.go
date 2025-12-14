package flux

import "testing"

func TestState_String_Loading(t *testing.T) {
	if s := StateLoading.String(); s != "loading" {
		t.Errorf("expected 'loading', got %q", s)
	}
}

func TestState_String_Healthy(t *testing.T) {
	if s := StateHealthy.String(); s != "healthy" {
		t.Errorf("expected 'healthy', got %q", s)
	}
}

func TestState_String_Degraded(t *testing.T) {
	if s := StateDegraded.String(); s != "degraded" {
		t.Errorf("expected 'degraded', got %q", s)
	}
}

func TestState_String_Empty(t *testing.T) {
	if s := StateEmpty.String(); s != "empty" {
		t.Errorf("expected 'empty', got %q", s)
	}
}

func TestState_String_Unknown(t *testing.T) {
	unknown := State(999)
	if s := unknown.String(); s != "unknown" {
		t.Errorf("expected 'unknown', got %q", s)
	}
}

func TestState_Values(t *testing.T) {
	// Verify iota ordering
	if StateLoading != 0 {
		t.Errorf("expected StateLoading=0, got %d", StateLoading)
	}
	if StateHealthy != 1 {
		t.Errorf("expected StateHealthy=1, got %d", StateHealthy)
	}
	if StateDegraded != 2 {
		t.Errorf("expected StateDegraded=2, got %d", StateDegraded)
	}
	if StateEmpty != 3 {
		t.Errorf("expected StateEmpty=3, got %d", StateEmpty)
	}
}
