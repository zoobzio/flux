package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	// Create config table and trigger
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value BYTEA NOT NULL
		);

		CREATE OR REPLACE FUNCTION notify_config_change() RETURNS trigger AS $$
		BEGIN
			PERFORM pg_notify('config_changed', NEW.key);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS config_change_trigger ON config;
		CREATE TRIGGER config_change_trigger
			AFTER INSERT OR UPDATE ON config
			FOR EACH ROW EXECUTE FUNCTION notify_config_change();
	`)
	if err != nil {
		t.Fatalf("failed to setup schema: %v", err)
	}

	return pool
}

func TestWatcher_EmitsInitialValue(t *testing.T) {
	pool := setupPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "test-config"
	value := []byte(`{"port": 8080}`)

	_, err := pool.Exec(ctx, "INSERT INTO config (key, value) VALUES ($1, $2)", key, value)
	if err != nil {
		t.Fatalf("failed to insert initial value: %v", err)
	}

	watcher := New(pool, "config_changed", key)
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	select {
	case data := <-ch:
		if string(data) != string(value) {
			t.Errorf("expected %q, got %q", value, data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for initial value")
	}
}

func TestWatcher_EmitsOnChange(t *testing.T) {
	pool := setupPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "test-config"
	initial := []byte(`{"v": 1}`)
	updated := []byte(`{"v": 2}`)

	_, err := pool.Exec(ctx, "INSERT INTO config (key, value) VALUES ($1, $2)", key, initial)
	if err != nil {
		t.Fatalf("failed to insert initial value: %v", err)
	}

	watcher := New(pool, "config_changed", key)
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain initial value
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for initial value")
	}

	// Update value
	_, err = pool.Exec(ctx, "UPDATE config SET value = $1 WHERE key = $2", updated, key)
	if err != nil {
		t.Fatalf("failed to update value: %v", err)
	}

	// Should receive update
	select {
	case data := <-ch:
		if string(data) != string(updated) {
			t.Errorf("expected %q, got %q", updated, data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for update")
	}
}

func TestWatcher_ClosesOnContextCancel(t *testing.T) {
	pool := setupPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	key := "test-config"
	_, err := pool.Exec(ctx, "INSERT INTO config (key, value) VALUES ($1, $2)", key, []byte("value"))
	if err != nil {
		t.Fatalf("failed to insert value: %v", err)
	}

	watcher := New(pool, "config_changed", key)
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain initial
	<-ch

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestWatcher_IgnoresOtherKeys(t *testing.T) {
	pool := setupPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "my-config"
	otherKey := "other-config"

	_, err := pool.Exec(ctx, "INSERT INTO config (key, value) VALUES ($1, $2)", key, []byte(`{"v": 1}`))
	if err != nil {
		t.Fatalf("failed to insert value: %v", err)
	}

	watcher := New(pool, "config_changed", key)
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain initial
	<-ch

	// Update a different key
	_, err = pool.Exec(ctx, "INSERT INTO config (key, value) VALUES ($1, $2)", otherKey, []byte(`{"other": true}`))
	if err != nil {
		t.Fatalf("failed to insert other key: %v", err)
	}

	// Should not receive update for other key
	select {
	case data := <-ch:
		t.Errorf("did not expect update, got %q", data)
	case <-time.After(500 * time.Millisecond):
		// Expected - no update for other key
	}

	// Update our key
	_, err = pool.Exec(ctx, "UPDATE config SET value = $1 WHERE key = $2", []byte(`{"v": 2}`), key)
	if err != nil {
		t.Fatalf("failed to update value: %v", err)
	}

	// Should receive our update
	select {
	case data := <-ch:
		if string(data) != `{"v": 2}` {
			t.Errorf("expected updated value, got %q", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for update")
	}
}
