package flux_test

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/flux"
)

// TestWithFilter tests filtering functionality
func TestWithFilter(t *testing.T) {
	var processed []string

	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		processed = append(processed, event.Data)
		return nil
	}).WithFilter(func(e flux.Event[string]) bool {
		return e.Source == "allowed"
	})

	ctx := context.Background()

	// Process events from different sources
	events := []flux.Event[string]{
		{ID: "1", Data: "data1", Source: "allowed", Context: ctx},
		{ID: "2", Data: "data2", Source: "blocked", Context: ctx},
		{ID: "3", Data: "data3", Source: "allowed", Context: ctx},
		{ID: "4", Data: "data4", Source: "other", Context: ctx},
	}

	for _, event := range events {
		_, _ = sync.Process(event)
	}

	// Should only process events from "allowed" source
	if len(processed) != 2 {
		t.Errorf("expected 2 processed events, got %d", len(processed))
	}

	if processed[0] != "data1" || processed[1] != "data3" {
		t.Errorf("unexpected processed data: %v", processed)
	}
}

type tenantContextKey string

// TestFilterWithContext tests filtering based on context values
func TestFilterWithContext(t *testing.T) {
	callCount := 0

	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		callCount++
		return nil
	}).WithFilter(func(e flux.Event[string]) bool {
		tenantID := e.Context.Value(tenantContextKey("tenant-id"))
		return tenantID == "premium"
	})

	// Test with premium tenant
	premiumCtx := context.WithValue(context.Background(), tenantContextKey("tenant-id"), "premium")
	premiumEvent := flux.Event[string]{
		ID:      "1",
		Data:    "premium-data",
		Context: premiumCtx,
	}

	_, _ = sync.Process(premiumEvent)

	// Test with basic tenant
	basicCtx := context.WithValue(context.Background(), tenantContextKey("tenant-id"), "basic")
	basicEvent := flux.Event[string]{
		ID:      "2",
		Data:    "basic-data",
		Context: basicCtx,
	}

	_, _ = sync.Process(basicEvent)

	// Only premium event should be processed
	if callCount != 1 {
		t.Errorf("expected 1 call for premium tenant, got %d", callCount)
	}
}

// TestFilterByTimestamp tests filtering based on event age
func TestFilterByTimestamp(t *testing.T) {
	processed := 0

	sync := flux.NewSync("test", func(event flux.Event[string]) error {
		processed++
		return nil
	}).WithFilter(func(e flux.Event[string]) bool {
		return time.Since(e.Timestamp) < 1*time.Minute
	})

	ctx := context.Background()

	// Recent event
	recentEvent := flux.Event[string]{
		ID:        "1",
		Data:      "recent",
		Context:   ctx,
		Timestamp: time.Now(),
	}

	// Old event
	oldEvent := flux.Event[string]{
		ID:        "2",
		Data:      "old",
		Context:   ctx,
		Timestamp: time.Now().Add(-2 * time.Hour),
	}

	_, _ = sync.Process(recentEvent)
	_, _ = sync.Process(oldEvent)

	if processed != 1 {
		t.Errorf("expected only recent event to be processed, got %d", processed)
	}
}
