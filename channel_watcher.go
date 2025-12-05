package flux

import "context"

// ChannelWatcher wraps an existing byte channel as a Watcher.
// Useful for testing and custom sources that already produce bytes.
type ChannelWatcher struct {
	ch   <-chan []byte
	sync bool
}

// NewChannelWatcher creates a ChannelWatcher that forwards values from the
// given channel through an internal goroutine.
func NewChannelWatcher(ch <-chan []byte) *ChannelWatcher {
	return &ChannelWatcher{ch: ch, sync: false}
}

// NewSyncChannelWatcher creates a ChannelWatcher that returns the source
// channel directly without an intermediate goroutine.
// Use with WithSyncMode() for deterministic testing.
func NewSyncChannelWatcher(ch <-chan []byte) *ChannelWatcher {
	return &ChannelWatcher{ch: ch, sync: true}
}

// Watch returns a channel that emits values from the wrapped channel.
func (w *ChannelWatcher) Watch(ctx context.Context) (<-chan []byte, error) {
	if w.sync {
		return w.ch, nil
	}

	out := make(chan []byte)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-w.ch:
				if !ok {
					return
				}
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
