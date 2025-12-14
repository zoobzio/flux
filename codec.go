package flux

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// Codec defines the deserialization contract for configuration data.
// Implement this interface to use alternative formats like TOML, HCL, or custom binary formats.
type Codec interface {
	// Unmarshal deserializes bytes into a value.
	Unmarshal(data []byte, v any) error

	// ContentType returns the MIME type for observability and debugging.
	ContentType() string
}

// JSONCodec implements Codec using encoding/json.
type JSONCodec struct{}

// Unmarshal deserializes JSON bytes into v.
func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// ContentType returns the JSON MIME type.
func (JSONCodec) ContentType() string {
	return "application/json"
}

// Ensure JSONCodec implements Codec.
var _ Codec = JSONCodec{}

// YAMLCodec implements Codec using gopkg.in/yaml.v3.
type YAMLCodec struct{}

// Unmarshal deserializes YAML bytes into v.
func (YAMLCodec) Unmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

// ContentType returns the YAML MIME type.
func (YAMLCodec) ContentType() string {
	return "application/x-yaml"
}

// Ensure YAMLCodec implements Codec.
var _ Codec = YAMLCodec{}
