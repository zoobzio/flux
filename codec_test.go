package flux

import "testing"

type codecTestConfig struct {
	Name  string `json:"name" yaml:"name"`
	Value int    `json:"value" yaml:"value"`
}

func TestJSONCodec_Unmarshal(t *testing.T) {
	codec := JSONCodec{}

	data := []byte(`{"name": "test", "value": 42}`)
	var cfg codecTestConfig

	if err := codec.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Name != "test" {
		t.Errorf("expected name 'test', got %q", cfg.Name)
	}
	if cfg.Value != 42 {
		t.Errorf("expected value 42, got %d", cfg.Value)
	}
}

func TestJSONCodec_UnmarshalInvalid(t *testing.T) {
	codec := JSONCodec{}

	data := []byte(`{not valid json}`)
	var cfg codecTestConfig

	if err := codec.Unmarshal(data, &cfg); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestJSONCodec_ContentType(t *testing.T) {
	codec := JSONCodec{}

	if ct := codec.ContentType(); ct != "application/json" {
		t.Errorf("expected 'application/json', got %q", ct)
	}
}

func TestYAMLCodec_Unmarshal(t *testing.T) {
	codec := YAMLCodec{}

	data := []byte("name: test\nvalue: 42")
	var cfg codecTestConfig

	if err := codec.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Name != "test" {
		t.Errorf("expected name 'test', got %q", cfg.Name)
	}
	if cfg.Value != 42 {
		t.Errorf("expected value 42, got %d", cfg.Value)
	}
}

func TestYAMLCodec_UnmarshalJSON(t *testing.T) {
	codec := YAMLCodec{}

	// YAML codec should also accept JSON (YAML is a superset of JSON)
	data := []byte(`{"name": "json-compat", "value": 99}`)
	var cfg codecTestConfig

	if err := codec.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Name != "json-compat" {
		t.Errorf("expected name 'json-compat', got %q", cfg.Name)
	}
	if cfg.Value != 99 {
		t.Errorf("expected value 99, got %d", cfg.Value)
	}
}

func TestYAMLCodec_UnmarshalInvalid(t *testing.T) {
	codec := YAMLCodec{}

	data := []byte("name: [unclosed")
	var cfg codecTestConfig

	if err := codec.Unmarshal(data, &cfg); err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestYAMLCodec_ContentType(t *testing.T) {
	codec := YAMLCodec{}

	if ct := codec.ContentType(); ct != "application/x-yaml" {
		t.Errorf("expected 'application/x-yaml', got %q", ct)
	}
}
