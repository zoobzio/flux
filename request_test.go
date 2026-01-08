package flux

import (
	"context"
	"testing"
)

type testConfig struct {
	Value string
}

func (testConfig) Validate() error {
	return nil
}

func TestRequest_Fields(t *testing.T) {
	req := Request[testConfig]{
		Previous: testConfig{Value: "old"},
		Current:  testConfig{Value: "new"},
		Raw:      []byte(`{"value":"new"}`),
	}

	if req.Previous.Value != "old" {
		t.Errorf("Previous.Value = %q, want %q", req.Previous.Value, "old")
	}
	if req.Current.Value != "new" {
		t.Errorf("Current.Value = %q, want %q", req.Current.Value, "new")
	}
	if string(req.Raw) != `{"value":"new"}` {
		t.Errorf("Raw = %q, want %q", string(req.Raw), `{"value":"new"}`)
	}
}

func TestReducer_Type(t *testing.T) {
	var reducer Reducer[testConfig] = func(_ context.Context, _, curr []testConfig) (testConfig, error) {
		if len(curr) == 0 {
			return testConfig{}, nil
		}
		return curr[0], nil
	}

	result, err := reducer(context.Background(), nil, []testConfig{{Value: "test"}})
	if err != nil {
		t.Fatalf("reducer returned error: %v", err)
	}
	if result.Value != "test" {
		t.Errorf("result.Value = %q, want %q", result.Value, "test")
	}
}
