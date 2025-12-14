package file

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	watcher := New("/path/to/config.json")
	if watcher == nil {
		t.Fatal("expected non-nil watcher")
	}
	if watcher.path != "/path/to/config.json" {
		t.Errorf("expected path '/path/to/config.json', got %q", watcher.path)
	}
}

func TestWatcher_Watch_EmitsInitialContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	content := []byte(`{"key": "value"}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	watcher := New(path)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case data := <-ch:
		if !bytes.Equal(data, content) {
			t.Errorf("expected %q, got %q", content, data)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for initial content")
	}
}

func TestWatcher_Watch_NonexistentFile(t *testing.T) {
	watcher := New("/nonexistent/path/config.json")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := watcher.Watch(ctx)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestWatcher_Watch_ClosesOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	watcher := New(path)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Drain initial content
	<-ch

	cancel()

	// Channel should close
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to close after context cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel to close")
	}
}

func TestWatcher_Watch_EmitsOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte(`{"v": 1}`), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	watcher := New(path)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Drain initial content
	<-ch

	// Write new content
	if err := os.WriteFile(path, []byte(`{"v": 2}`), 0o600); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	// Should receive update
	select {
	case data := <-ch:
		if string(data) != `{"v": 2}` {
			t.Errorf("expected updated content, got %q", data)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for file update")
	}
}
