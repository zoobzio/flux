package etcd

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcetcd "github.com/testcontainers/testcontainers-go/modules/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func setupEtcd(t *testing.T) *clientv3.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcetcd.Run(ctx, "gcr.io/etcd-development/etcd:v3.5.21")
	if err != nil {
		t.Fatalf("failed to start etcd container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	endpoint, err := container.ClientEndpoint(ctx)
	if err != nil {
		t.Fatalf("failed to get endpoint: %v", err)
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
	})

	return client
}

func TestWatcher_EmitsInitialValue(t *testing.T) {
	client := setupEtcd(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "/config/test"
	value := []byte(`{"port": 8080}`)

	_, err := client.Put(ctx, key, string(value))
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
	client := setupEtcd(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "/config/test"
	initial := []byte(`{"v": 1}`)
	updated := []byte(`{"v": 2}`)

	_, err := client.Put(ctx, key, string(initial))
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
	_, err = client.Put(ctx, key, string(updated))
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
	client := setupEtcd(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	key := "/config/test"
	_, err := client.Put(ctx, key, "value")
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
	client := setupEtcd(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	watcher := New(client, "/nonexistent/key")
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Should not emit anything for nonexistent key
	select {
	case <-ch:
		t.Error("did not expect value for nonexistent key")
	case <-time.After(500 * time.Millisecond):
		// Expected - no initial value
	}

	// Now create the key
	_, err = client.Put(ctx, "/nonexistent/key", "created")
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
