// Package redis provides a flux.Watcher implementation for Redis keys
// using keyspace notifications.
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Watcher watches a Redis key for changes using keyspace notifications.
// Requires Redis to have keyspace notifications enabled:
//
//	CONFIG SET notify-keyspace-events KEA
//
// Or in redis.conf:
//
//	notify-keyspace-events KEA
type Watcher struct {
	client *redis.Client
	key    string
}

// Option configures a Watcher.
type Option func(*Watcher)

// New creates a new Watcher for the given Redis key.
func New(client *redis.Client, key string, opts ...Option) *Watcher {
	w := &Watcher{
		client: client,
		key:    key,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Watch begins watching the Redis key and returns a channel that emits
// the key's value whenever it changes. The current value is emitted
// immediately to support initial configuration loading.
func (w *Watcher) Watch(ctx context.Context) (<-chan []byte, error) {
	// Subscribe to keyspace notifications for this key
	channel := fmt.Sprintf("__keyspace@0__:%s", w.key)
	pubsub := w.client.Subscribe(ctx, channel)

	// Verify subscription worked
	_, err := pubsub.Receive(ctx)
	if err != nil {
		pubsub.Close()
		return nil, fmt.Errorf("failed to subscribe to keyspace notifications: %w", err)
	}

	out := make(chan []byte)

	go func() {
		defer close(out)
		defer pubsub.Close()

		// Emit initial value
		val, err := w.client.Get(ctx, w.key).Bytes()
		if err != nil && err != redis.Nil {
			return
		}
		if err != redis.Nil {
			select {
			case out <- val:
			case <-ctx.Done():
				return
			}
		}

		// Watch for changes
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}

				// Only react to set/hset operations
				switch msg.Payload {
				case "set", "hset", "mset", "setex", "psetex", "setnx":
					val, err := w.client.Get(ctx, w.key).Bytes()
					if err != nil {
						continue
					}
					select {
					case out <- val:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out, nil
}
