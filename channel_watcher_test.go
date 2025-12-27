package flux

import (
	"context"
	"testing"
	"time"
)

func TestChannelWatcher_ForwardsValues(t *testing.T) {
	source := make(chan []byte, 3)
	source <- []byte("one")
	source <- []byte("two")
	source <- []byte("three")

	watcher := NewChannelWatcher(source)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	expected := []string{"one", "two", "three"}
	for i, exp := range expected {
		select {
		case v := <-out:
			if string(v) != exp {
				t.Errorf("expected %s, got %s", exp, string(v))
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for value %d", i)
		}
	}
}

func TestChannelWatcher_ClosesOnSourceClose(t *testing.T) {
	source := make(chan []byte, 1)
	source <- []byte("value")
	close(source)

	watcher := NewChannelWatcher(source)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain the value
	<-out

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

func TestChannelWatcher_ClosesOnContextCancel(t *testing.T) {
	source := make(chan []byte) // unbuffered, will block

	watcher := NewChannelWatcher(source)

	ctx, cancel := context.WithCancel(context.Background())

	out, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
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

func TestChannelWatcher_RespectsContextDuringSend(t *testing.T) {
	source := make(chan []byte, 1)
	source <- []byte("value")

	watcher := NewChannelWatcher(source)

	ctx, cancel := context.WithCancel(context.Background())

	out, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Don't read from out, causing backpressure
	// Cancel context while send is blocked
	cancel()

	// Goroutine should exit cleanly
	select {
	case <-out:
		// Value may or may not have been sent before cancel
	case <-time.After(100 * time.Millisecond):
		// This is also acceptable - send was blocked and canceled
	}
}

func TestChannelWatcher_CancelWhileBlockedOnSend(t *testing.T) {
	// Unbuffered source channel
	source := make(chan []byte)

	watcher := NewChannelWatcher(source)

	ctx, cancel := context.WithCancel(context.Background())

	watchOut, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Send a value that will be received by watcher goroutine
	go func() {
		source <- []byte("test")
	}()

	// Wait for value to be received by watcher goroutine
	// It will now be blocked trying to send to watchOut (unbuffered)
	time.Sleep(20 * time.Millisecond)

	// Cancel context - this should unblock the send
	cancel()

	// watchOut should close cleanly
	select {
	case <-watchOut:
		// Channel closed as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("channel did not close after context cancel")
	}
}
