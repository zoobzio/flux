// Package consul provides a flux.Watcher implementation for Consul KV
// using blocking queries.
package consul

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul/api"
)

// Watcher watches a Consul KV key for changes using blocking queries.
type Watcher struct {
	client *api.Client
	key    string
}

// Option configures a Watcher.
type Option func(*Watcher)

// New creates a new Watcher for the given Consul KV key.
func New(client *api.Client, key string, opts ...Option) *Watcher {
	w := &Watcher{
		client: client,
		key:    key,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Watch begins watching the Consul KV key and returns a channel that emits
// the key's value whenever it changes. The current value is emitted
// immediately to support initial configuration loading.
func (w *Watcher) Watch(ctx context.Context) (<-chan []byte, error) {
	kv := w.client.KV()

	// Get initial value and index
	pair, meta, err := kv.Get(w.key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get initial value: %w", err)
	}

	out := make(chan []byte)

	go func() {
		defer close(out)

		lastIndex := meta.LastIndex

		// Emit initial value if key exists
		if pair != nil {
			select {
			case out <- pair.Value:
			case <-ctx.Done():
				return
			}
		}

		// Watch for changes using blocking queries
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			opts := &api.QueryOptions{
				WaitIndex: lastIndex,
			}
			opts = opts.WithContext(ctx)

			pair, meta, err := kv.Get(w.key, opts)
			if err != nil {
				// Context cancelled
				if ctx.Err() != nil {
					return
				}
				// Other error - continue watching
				continue
			}

			// Only emit if index changed and key exists
			if meta.LastIndex > lastIndex && pair != nil {
				lastIndex = meta.LastIndex
				select {
				case out <- pair.Value:
				case <-ctx.Done():
					return
				}
			} else if meta.LastIndex > lastIndex {
				// Index changed but key deleted - update index
				lastIndex = meta.LastIndex
			}
		}
	}()

	return out, nil
}
