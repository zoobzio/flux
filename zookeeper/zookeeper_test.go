package zookeeper

import (
	"context"
	"testing"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupZookeeper(t *testing.T) *zk.Conn {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "zookeeper:3.9",
			ExposedPorts: []string{"2181/tcp"},
			WaitingFor:   wait.ForListeningPort("2181/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("failed to start zookeeper container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}

	port, err := container.MappedPort(ctx, "2181/tcp")
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}

	conn, _, err := zk.Connect([]string{host + ":" + port.Port()}, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	return conn
}

func TestWatcher_EmitsInitialValue(t *testing.T) {
	conn := setupZookeeper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := "/config/test"
	value := []byte(`{"port": 8080}`)

	// Create parent path
	_, err := conn.Create("/config", nil, 0, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		t.Fatalf("failed to create parent: %v", err)
	}

	_, err = conn.Create(path, value, 0, zk.WorldACL(zk.PermAll))
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	watcher := New(conn, path)
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
	conn := setupZookeeper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := "/config/test"
	initial := []byte(`{"v": 1}`)
	updated := []byte(`{"v": 2}`)

	// Create parent path
	_, err := conn.Create("/config", nil, 0, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		t.Fatalf("failed to create parent: %v", err)
	}

	_, err = conn.Create(path, initial, 0, zk.WorldACL(zk.PermAll))
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	watcher := New(conn, path)
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
	_, err = conn.Set(path, updated, -1)
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
	conn := setupZookeeper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	path := "/config/test"

	// Create parent path
	_, err := conn.Create("/config", nil, 0, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		t.Fatalf("failed to create parent: %v", err)
	}

	_, err = conn.Create(path, []byte("value"), 0, zk.WorldACL(zk.PermAll))
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	watcher := New(conn, path)
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

func TestWatcher_WaitsForNodeCreation(t *testing.T) {
	conn := setupZookeeper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := "/config/delayed"
	value := []byte(`{"delayed": true}`)

	// Create parent path
	_, err := conn.Create("/config", nil, 0, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		t.Fatalf("failed to create parent: %v", err)
	}

	// Start watching before node exists
	watcher := New(conn, path)
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Create node after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, err := conn.Create(path, value, 0, zk.WorldACL(zk.PermAll))
		if err != nil {
			t.Errorf("failed to create node: %v", err)
		}
	}()

	// Should receive value once node is created
	select {
	case data := <-ch:
		if string(data) != string(value) {
			t.Errorf("expected %q, got %q", value, data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for node creation")
	}
}

func TestWatcher_HandlesNodeDeletion(t *testing.T) {
	conn := setupZookeeper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := "/config/deletable"
	initial := []byte(`{"v": 1}`)
	recreated := []byte(`{"v": 2}`)

	// Create parent path
	_, err := conn.Create("/config", nil, 0, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		t.Fatalf("failed to create parent: %v", err)
	}

	_, err = conn.Create(path, initial, 0, zk.WorldACL(zk.PermAll))
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	watcher := New(conn, path)
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

	// Delete node
	err = conn.Delete(path, -1)
	if err != nil {
		t.Fatalf("failed to delete node: %v", err)
	}

	// Recreate node with new value
	time.Sleep(100 * time.Millisecond)
	_, err = conn.Create(path, recreated, 0, zk.WorldACL(zk.PermAll))
	if err != nil {
		t.Fatalf("failed to recreate node: %v", err)
	}

	// Should receive new value after recreation
	select {
	case data := <-ch:
		if string(data) != string(recreated) {
			t.Errorf("expected %q, got %q", recreated, data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for recreated value")
	}
}

func TestWatcher_ContextCancelDuringWaitForNode(t *testing.T) {
	conn := setupZookeeper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	path := "/config/never-created"

	// Create parent path
	_, err := conn.Create("/config", nil, 0, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		t.Fatalf("failed to create parent: %v", err)
	}

	// Start watching non-existent node
	watcher := New(conn, path)
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Cancel context while waiting for node creation
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Channel should close without receiving a value
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to close without value")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}
