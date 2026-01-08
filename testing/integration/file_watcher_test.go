package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zoobzio/flux/file"
)

func TestFileWatcher_EmitsInitialContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")

	if err := os.WriteFile(path, []byte("initial"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	watcher := file.New(path)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	select {
	case data := <-out:
		if string(data) != "initial" {
			t.Errorf("expected 'initial', got %q", string(data))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for initial contents")
	}
}

func TestFileWatcher_EmitsOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")

	if err := os.WriteFile(path, []byte("initial"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	watcher := file.New(path)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain initial contents
	select {
	case <-out:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for initial contents")
	}

	// Write new contents
	if err := os.WriteFile(path, []byte("updated"), 0o600); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	// Should receive updated contents
	select {
	case data := <-out:
		if string(data) != "updated" {
			t.Errorf("expected 'updated', got %q", string(data))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for updated contents")
	}
}

func TestFileWatcher_ClosesOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")

	if err := os.WriteFile(path, []byte("initial"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	watcher := file.New(path)

	ctx, cancel := context.WithCancel(context.Background())

	out, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain initial contents
	select {
	case <-out:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for initial contents")
	}

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-out:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}
}

func TestFileWatcher_ErrorOnNonexistentFile(t *testing.T) {
	watcher := file.New("/nonexistent/path/config.txt")

	ctx := context.Background()
	_, err := watcher.Watch(ctx)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestFileWatcher_EventuallySeesLatestValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")

	if err := os.WriteFile(path, []byte("v1"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	watcher := file.New(path)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain initial
	select {
	case <-out:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for initial contents")
	}

	// Write final value
	if err := os.WriteFile(path, []byte("final"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Should eventually see final value (may receive intermediate events)
	var lastSeen string
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case data := <-out:
			lastSeen = string(data)
			if lastSeen == "final" {
				return // Success
			}
		case <-timeout:
			if lastSeen == "" {
				t.Fatal("timeout: received no updates")
			}
			t.Fatalf("timeout: last seen %q, expected 'final'", lastSeen)
		}
	}
}
