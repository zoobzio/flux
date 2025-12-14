// Package postgres provides a flux.Watcher implementation for PostgreSQL
// using LISTEN/NOTIFY with a backing table.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Watcher watches a PostgreSQL table row for changes using LISTEN/NOTIFY.
// Requires a trigger to be set up on the table that sends notifications.
//
// Example trigger setup:
//
//	CREATE OR REPLACE FUNCTION notify_config_change() RETURNS trigger AS $$
//	BEGIN
//	    PERFORM pg_notify('config_changed', NEW.key);
//	    RETURN NEW;
//	END;
//	$$ LANGUAGE plpgsql;
//
//	CREATE TRIGGER config_change_trigger
//	    AFTER INSERT OR UPDATE ON config
//	    FOR EACH ROW EXECUTE FUNCTION notify_config_change();
type Watcher struct {
	pool    *pgxpool.Pool
	channel string
	key     string
	table   string
}

// Option configures a Watcher.
type Option func(*Watcher)

// WithTable sets the table name to query for values.
// Defaults to "config".
func WithTable(table string) Option {
	return func(w *Watcher) {
		w.table = table
	}
}

// New creates a new Watcher for the given notification channel and key.
// The channel should match the channel used in pg_notify.
// The key identifies which row to fetch from the config table.
func New(pool *pgxpool.Pool, channel, key string, opts ...Option) *Watcher {
	w := &Watcher{
		pool:    pool,
		channel: channel,
		key:     key,
		table:   "config",
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Watch begins watching for PostgreSQL notifications and returns a channel
// that emits the row's value whenever it changes. The current value is
// emitted immediately to support initial configuration loading.
func (w *Watcher) Watch(ctx context.Context) (<-chan []byte, error) {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}

	// Start listening
	_, err = conn.Exec(ctx, fmt.Sprintf("LISTEN %s", w.channel))
	if err != nil {
		conn.Release()
		return nil, fmt.Errorf("failed to listen on channel %s: %w", w.channel, err)
	}

	out := make(chan []byte)

	go func() {
		defer close(out)
		defer conn.Release()

		// Emit initial value
		value, err := w.fetchValue(ctx)
		if err == nil && value != nil {
			select {
			case out <- value:
			case <-ctx.Done():
				return
			}
		}

		// Wait for notifications
		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}

			// Check if notification is for our key
			if notification.Payload != w.key {
				continue
			}

			// Fetch updated value
			value, err := w.fetchValue(ctx)
			if err != nil {
				continue
			}
			if value == nil {
				continue
			}

			select {
			case out <- value:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// fetchValue retrieves the current value from the config table.
func (w *Watcher) fetchValue(ctx context.Context) ([]byte, error) {
	var value []byte
	query := fmt.Sprintf("SELECT value FROM %s WHERE key = $1", w.table)
	err := w.pool.QueryRow(ctx, query, w.key).Scan(&value)
	if err != nil {
		return nil, err
	}
	return value, nil
}
