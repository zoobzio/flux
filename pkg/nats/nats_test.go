package nats

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

func setupNATS(t *testing.T) jetstream.KeyValue {
	t.Helper()
	ctx := context.Background()

	container, err := tcnats.Run(ctx, "nats:2.10-alpine", tcnats.WithArgument("--jetstream"))
	if err != nil {
		t.Fatalf("failed to start nats container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get endpoint: %v", err)
	}

	nc, err := nats.Connect(endpoint)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
	})

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("failed to create jetstream: %v", err)
	}

	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "config",
	})
	if err != nil {
		t.Fatalf("failed to create kv bucket: %v", err)
	}

	return kv
}

func TestWatcher_EmitsInitialValue(t *testing.T) {
	kv := setupNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "test-config"
	value := []byte(`{"port": 8080}`)

	_, err := kv.Put(ctx, key, value)
	if err != nil {
		t.Fatalf("failed to put initial value: %v", err)
	}

	watcher := New(kv, key)
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
	kv := setupNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "test-config"
	initial := []byte(`{"v": 1}`)
	updated := []byte(`{"v": 2}`)

	_, err := kv.Put(ctx, key, initial)
	if err != nil {
		t.Fatalf("failed to put initial value: %v", err)
	}

	watcher := New(kv, key)
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
	_, err = kv.Put(ctx, key, updated)
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
	kv := setupNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	key := "test-config"
	_, err := kv.Put(ctx, key, []byte("value"))
	if err != nil {
		t.Fatalf("failed to put value: %v", err)
	}

	watcher := New(kv, key)
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
