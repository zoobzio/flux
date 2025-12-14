// Package etcd provides a flux.Watcher implementation for etcd keys
// using the native Watch API.
package etcd

import (
	"context"
	"fmt"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Watcher watches an etcd key for changes using the Watch API.
type Watcher struct {
	client *clientv3.Client
	key    string
}

// Option configures a Watcher.
type Option func(*Watcher)

// New creates a new Watcher for the given etcd key.
func New(client *clientv3.Client, key string, opts ...Option) *Watcher {
	w := &Watcher{
		client: client,
		key:    key,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Watch begins watching the etcd key and returns a channel that emits
// the key's value whenever it changes. The current value is emitted
// immediately to support initial configuration loading.
func (w *Watcher) Watch(ctx context.Context) (<-chan []byte, error) {
	// Get initial value
	resp, err := w.client.Get(ctx, w.key)
	if err != nil {
		return nil, fmt.Errorf("failed to get initial value: %w", err)
	}

	out := make(chan []byte)

	go func() {
		defer close(out)

		// Emit initial value if key exists
		if len(resp.Kvs) > 0 {
			select {
			case out <- resp.Kvs[0].Value:
			case <-ctx.Done():
				return
			}
		}

		// Watch for changes starting from current revision
		watchChan := w.client.Watch(ctx, w.key, clientv3.WithRev(resp.Header.Revision+1))

		for {
			select {
			case <-ctx.Done():
				return
			case watchResp, ok := <-watchChan:
				if !ok {
					return
				}
				if watchResp.Err() != nil {
					continue
				}

				for _, event := range watchResp.Events {
					// Only emit on PUT events (not DELETE)
					if event.Type == clientv3.EventTypePut {
						select {
						case out <- event.Kv.Value:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()

	return out, nil
}
