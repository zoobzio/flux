package consul

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/testcontainers/testcontainers-go"
	tcconsul "github.com/testcontainers/testcontainers-go/modules/consul"
)

func setupConsul(t *testing.T) *api.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcconsul.Run(ctx, "consul:1.15")
	if err != nil {
		t.Fatalf("failed to start consul container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	endpoint, err := container.ApiEndpoint(ctx)
	if err != nil {
		t.Fatalf("failed to get endpoint: %v", err)
	}

	client, err := api.NewClient(&api.Config{
		Address: endpoint,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return client
}

func TestWatcher_EmitsInitialValue(t *testing.T) {
	client := setupConsul(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "config/test"
	value := []byte(`{"port": 8080}`)

	_, err := client.KV().Put(&api.KVPair{Key: key, Value: value}, nil)
	if err != nil {
		t.Fatalf("failed to put initial value: %v", err)
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
	client := setupConsul(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "config/test"
	initial := []byte(`{"v": 1}`)
	updated := []byte(`{"v": 2}`)

	_, err := client.KV().Put(&api.KVPair{Key: key, Value: initial}, nil)
	if err != nil {
		t.Fatalf("failed to put initial value: %v", err)
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
	_, err = client.KV().Put(&api.KVPair{Key: key, Value: updated}, nil)
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
	client := setupConsul(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	key := "config/test"
	_, err := client.KV().Put(&api.KVPair{Key: key, Value: []byte("value")}, nil)
	if err != nil {
		t.Fatalf("failed to put value: %v", err)
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
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestWatcher_NonexistentKey(t *testing.T) {
	client := setupConsul(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	watcher := New(client, "nonexistent/key")
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Should not emit anything for nonexistent key, but should start watching
	select {
	case <-ch:
		t.Error("did not expect value for nonexistent key")
	case <-time.After(500 * time.Millisecond):
		// Expected - no initial value
	}

	// Now create the key
	_, err = client.KV().Put(&api.KVPair{Key: "nonexistent/key", Value: []byte("created")}, nil)
	if err != nil {
		t.Fatalf("failed to create key: %v", err)
	}

	// Should receive the new value
	select {
	case data := <-ch:
		if string(data) != "created" {
			t.Errorf("expected 'created', got %q", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for created key")
	}
}
