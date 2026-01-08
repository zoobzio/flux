// Package zookeeper provides a flux.Watcher implementation for ZooKeeper
// nodes using the native Watch API.
package zookeeper

import (
	"context"

	"github.com/go-zookeeper/zk"
)

// Watcher watches a ZooKeeper node for changes.
type Watcher struct {
	conn *zk.Conn
	path string
}

// Option configures a Watcher.
type Option func(*Watcher)

// New creates a new Watcher for the given ZooKeeper path.
func New(conn *zk.Conn, path string, opts ...Option) *Watcher {
	w := &Watcher{
		conn: conn,
		path: path,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Watch begins watching the ZooKeeper node and returns a channel that emits
// the node's data whenever it changes. The current value is emitted
// immediately to support initial configuration loading.
func (w *Watcher) Watch(ctx context.Context) (<-chan []byte, error) {
	out := make(chan []byte)

	go func() {
		defer close(out)

		for {
			// Get current value and set watch
			data, _, eventCh, err := w.conn.GetW(w.path)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				// Node doesn't exist yet, watch for creation
				exists, _, eventCh, err := w.conn.ExistsW(w.path)
				if err != nil {
					return
				}
				if !exists {
					select {
					case <-ctx.Done():
						return
					case <-eventCh:
						continue // Re-loop to get the value
					}
				}
				continue
			}

			// Emit current value
			select {
			case out <- data:
			case <-ctx.Done():
				return
			}

			// Wait for change
			select {
			case <-ctx.Done():
				return
			case event := <-eventCh:
				if event.Type == zk.EventNodeDeleted {
					continue
				}
				// Loop back to get new value and set new watch
			}
		}
	}()

	return out, nil
}
