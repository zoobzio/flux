package flux_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/zoobzio/flux"
	"github.com/zoobzio/pipz"
)

// TestWithErrorHandler tests error handler functionality
func TestWithErrorHandler(t *testing.T) {
	var mainCalled, errorCalled int32
	mainErr := errors.New("main processor error")

	// Main processor that always fails
	mainSync := flux.NewSync("main", func(event flux.Event[string]) error {
		atomic.AddInt32(&mainCalled, 1)
		return mainErr
	})

	// Error handler that processes pipz.Error
	errorHandler := pipz.Effect("error-handler", func(ctx context.Context, err *pipz.Error[flux.Event[string]]) error {
		atomic.AddInt32(&errorCalled, 1)

		// Verify we got the same event
		if err.InputData.ID != "test-1" {
			t.Errorf("error handler got wrong event ID: %s", err.InputData.ID)
		}

		// Verify we got the right error
		if err.Err.Error() != mainErr.Error() {
			t.Errorf("error handler got wrong error: %v", err.Err)
		}

		return nil
	})

	// Combine with error handler
	sync := mainSync.WithErrorHandler(errorHandler)

	ctx := context.Background()
	event := flux.Event[string]{
		ID:      "test-1",
		Data:    "data",
		Context: ctx,
	}

	// Process should return the original error
	_, err := sync.Process(event)
	if err == nil {
		t.Fatal("expected error from main processor")
	}

	// Both should have been called
	if atomic.LoadInt32(&mainCalled) != 1 {
		t.Errorf("expected main called once, got %d", mainCalled)
	}
	if atomic.LoadInt32(&errorCalled) != 1 {
		t.Errorf("expected error handler called once, got %d", errorCalled)
	}
}

// TestErrorHandlerWithSuccess tests that error handler is not called on success
func TestErrorHandlerWithSuccess(t *testing.T) {
	errorHandlerCalled := false

	mainSync := flux.NewSync("main", func(event flux.Event[string]) error {
		return nil // Success
	})

	errorHandler := pipz.Effect("error-handler", func(ctx context.Context, err *pipz.Error[flux.Event[string]]) error {
		errorHandlerCalled = true
		return nil
	})

	sync := mainSync.WithErrorHandler(errorHandler)

	event := flux.Event[string]{
		ID:      "test-2",
		Data:    "data",
		Context: context.Background(),
	}

	_, err := sync.Process(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if errorHandlerCalled {
		t.Error("error handler should not be called on success")
	}
}
