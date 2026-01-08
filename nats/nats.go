// Package nats provides a flux.Watcher implementation for NATS KV
// using the native Watch API.
package nats

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

// Watcher watches a NATS KV key for changes using the Watch API.
type Watcher struct {
	kv  jetstream.KeyValue
	key string
}

// Option configures a Watcher.
type Option func(*Watcher)

// New creates a new Watcher for the given NATS KV key.
func New(kv jetstream.KeyValue, key string, opts ...Option) *Watcher {
	w := &Watcher{
		kv:  kv,
		key: key,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Watch begins watching the NATS KV key and returns a channel that emits
// the key's value whenever it changes. The current value is emitted
// immediately to support initial configuration loading.
func (w *Watcher) Watch(ctx context.Context) (<-chan []byte, error) {
	// Start watching the key
	watcher, err := w.kv.Watch(ctx, w.key)
	if err != nil {
		return nil, fmt.Errorf("failed to watch key: %w", err)
	}

	out := make(chan []byte)

	go func() {
		defer close(out)
		defer watcher.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case entry, ok := <-watcher.Updates():
				if !ok {
					return
				}
				// nil entry signals end of initial values
				if entry == nil {
					continue
				}
				// Skip delete operations
				if entry.Operation() == jetstream.KeyValueDelete || entry.Operation() == jetstream.KeyValuePurge {
					continue
				}

				select {
				case out <- entry.Value():
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}
