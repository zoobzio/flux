package benchmarks

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/zoobzio/flux"
)

type benchConfig struct {
	Value int    `yaml:"value" json:"value"`
	Name  string `yaml:"name" json:"name"`
}

// Validate implements the flux.Validator interface.
func (c benchConfig) Validate() error {
	if c.Value < 0 {
		return fmt.Errorf("value must be >= 0, got %d", c.Value)
	}
	if c.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

func BenchmarkCapacitor_ProcessSingle(b *testing.B) {
	ch := make(chan []byte, b.N+1)
	ch <- []byte(`{"value": 0, "name": "initial"}`)
	for i := 1; i <= b.N; i++ {
		ch <- []byte(fmt.Sprintf(`{"value": %d, "name": "test"}`, i))
	}

	capacitor := flux.New[benchConfig](
		flux.NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ benchConfig) error { return nil },
		flux.WithSyncMode[benchConfig](),
	)

	ctx := context.Background()
	if err := capacitor.Start(ctx); err != nil {
		b.Fatalf("Start() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		capacitor.Process(ctx)
	}
}

func BenchmarkCapacitor_FullPipeline(b *testing.B) {
	ch := make(chan []byte, b.N+1)
	ch <- []byte(`{"value": 0, "name": "initial"}`)
	for i := 1; i <= b.N; i++ {
		ch <- []byte(fmt.Sprintf(`{"value": %d, "name": "test"}`, i))
	}

	var lastApplied int

	capacitor := flux.New[benchConfig](
		flux.NewSyncChannelWatcher(ch),
		func(_ context.Context, _, cfg benchConfig) error {
			lastApplied = cfg.Value
			return nil
		},
		flux.WithSyncMode[benchConfig](),
	)

	ctx := context.Background()
	if err := capacitor.Start(ctx); err != nil {
		b.Fatalf("Start() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		capacitor.Process(ctx)
	}

	// Prevent compiler optimization
	if lastApplied < 0 {
		b.Fatal("unexpected")
	}
}

func BenchmarkCapacitor_StateTransitions(b *testing.B) {
	ch := make(chan []byte, b.N*2+1)
	ch <- []byte(`{"value": 1, "name": "valid"}`) // Initial valid

	// Alternate valid/invalid
	for i := 0; i < b.N; i++ {
		ch <- []byte(`{"value": -1, "name": "invalid"}`) // Invalid (value < 0)
		ch <- []byte(`{"value": 1, "name": "valid"}`)    // Valid
	}

	capacitor := flux.New[benchConfig](
		flux.NewSyncChannelWatcher(ch),
		func(_ context.Context, _, _ benchConfig) error { return nil },
		flux.WithSyncMode[benchConfig](),
	)

	ctx := context.Background()
	if err := capacitor.Start(ctx); err != nil {
		b.Fatalf("Start() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		capacitor.Process(ctx) // Invalid -> Degraded
		capacitor.Process(ctx) // Valid -> Healthy
	}
}

func BenchmarkChannelWatcher_Forwarding(b *testing.B) {
	source := make(chan []byte, b.N)
	for i := 0; i < b.N; i++ {
		source <- []byte(fmt.Sprintf("value: %d", i))
	}

	watcher := flux.NewChannelWatcher(source)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := watcher.Watch(ctx)
	if err != nil {
		b.Fatalf("Watch() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-out
	}
}
