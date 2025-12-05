package flux

import "context"

// Watcher observes a source for changes and emits raw bytes on a channel.
// Implementations must emit the current value immediately upon Watch() being
// called to support initial configuration loading.
type Watcher interface {
	// Watch begins observing the source and returns a channel that emits
	// raw bytes when changes occur. The channel is closed when the context
	// is canceled or an unrecoverable error occurs.
	//
	// Implementations should emit the current value immediately to support
	// initial configuration loading.
	Watch(ctx context.Context) (<-chan []byte, error)
}
