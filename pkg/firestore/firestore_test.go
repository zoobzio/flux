package firestore

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func setupFirestore(t *testing.T) *firestore.Client {
	t.Helper()
	ctx := context.Background()

	container, err := gcloud.RunFirestore(ctx, "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators",
		gcloud.WithProjectID("test-project"),
	)
	if err != nil {
		t.Fatalf("failed to start firestore container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	conn, err := grpc.NewClient(container.URI,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to create grpc connection: %v", err)
	}

	client, err := firestore.NewClient(ctx, "test-project",
		option.WithGRPCConn(conn),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
	})

	return client
}

func TestWatcher_EmitsInitialValue(t *testing.T) {
	client := setupFirestore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collection := "config"
	document := "app"
	value := []byte(`{"port": 8080}`)

	err := CreateDocument(ctx, client, collection, document, value)
	if err != nil {
		t.Fatalf("failed to create document: %v", err)
	}

	watcher := New(client, collection, document)
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	select {
	case data := <-ch:
		if string(data) != string(value) {
			t.Errorf("expected %q, got %q", value, data)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for initial value")
	}
}

func TestWatcher_EmitsOnChange(t *testing.T) {
	client := setupFirestore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collection := "config"
	document := "app"
	initial := []byte(`{"v": 1}`)
	updated := []byte(`{"v": 2}`)

	err := CreateDocument(ctx, client, collection, document, initial)
	if err != nil {
		t.Fatalf("failed to create document: %v", err)
	}

	watcher := New(client, collection, document)
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain initial value
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for initial value")
	}

	// Update value
	err = UpdateDocument(ctx, client, collection, document, updated)
	if err != nil {
		t.Fatalf("failed to update document: %v", err)
	}

	// Should receive update
	select {
	case data := <-ch:
		if string(data) != string(updated) {
			t.Errorf("expected %q, got %q", updated, data)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for update")
	}
}

func TestWatcher_ClosesOnContextCancel(t *testing.T) {
	client := setupFirestore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	collection := "config"
	document := "app"

	err := CreateDocument(ctx, client, collection, document, []byte("value"))
	if err != nil {
		t.Fatalf("failed to create document: %v", err)
	}

	watcher := New(client, collection, document)
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
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}
