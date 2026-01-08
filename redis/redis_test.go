package redis

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("failed to get endpoint: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: endpoint,
	})

	// Enable keyspace notifications
	if err := client.ConfigSet(ctx, "notify-keyspace-events", "KEA").Err(); err != nil {
		t.Fatalf("failed to enable keyspace notifications: %v", err)
	}

	return client
}

func TestWatcher_EmitsInitialValue(t *testing.T) {
	client := setupRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := "config:test"
	value := []byte(`{"port": 8080}`)

	if err := client.Set(ctx, key, value, 0).Err(); err != nil {
		t.Fatalf("failed to set initial value: %v", err)
	}

	watcher := New(client, key)
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
	client := setupRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := "config:test"
	initial := []byte(`{"v": 1}`)
	updated := []byte(`{"v": 2}`)

	if err := client.Set(ctx, key, initial, 0).Err(); err != nil {
		t.Fatalf("failed to set initial value: %v", err)
	}

	watcher := New(client, key)
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
	if err := client.Set(ctx, key, updated, 0).Err(); err != nil {
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
	client := setupRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	key := "config:test"
	if err := client.Set(ctx, key, []byte("value"), 0).Err(); err != nil {
		t.Fatalf("failed to set value: %v", err)
	}

	watcher := New(client, key)
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
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}
