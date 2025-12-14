package flux

import (
	"context"

	"github.com/zoobzio/pipz"
)

// Request carries configuration data through the processing pipeline.
// It provides access to both the previous and current configuration values,
// allowing pipeline stages to make decisions based on what changed.
type Request[T Validator] struct {
	// Previous is the last successfully applied configuration.
	// On initial load, this will be the zero value of T.
	Previous T

	// Current is the newly parsed and validated configuration.
	// Pipeline stages may modify this value before it is stored.
	Current T

	// Raw contains the original bytes received from the watcher.
	// This is useful for debugging or logging purposes.
	Raw []byte
}

// Terminal is the final processing stage in a Capacitor pipeline.
// It receives the Request after all middleware has processed it.
type Terminal[T Validator] pipz.Chainable[*Request[T]]

// Reducer merges multiple configuration sources into a single configuration.
// It receives the previous merged values (nil on first call) and the current
// parsed values from each source in the same order as the sources were provided.
type Reducer[T Validator] func(ctx context.Context, prev, curr []T) (T, error)
