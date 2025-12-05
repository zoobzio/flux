package benchmarks

import (
	"context"
	"fmt"
	"testing"

	"github.com/zoobzio/flux"
)

type benchConfig struct {
	Value int    `yaml:"value" json:"value" validate:"min=0"`
	Name  string `yaml:"name" json:"name" validate:"required"`
}

func BenchmarkCapacitor_ProcessSingle(b *testing.B) {
	ch := make(chan []byte, b.N+1)
	ch <- []byte("value: 0\nname: initial")
	for i := 1; i <= b.N; i++ {
		ch <- []byte(fmt.Sprintf("value: %d\nname: test", i))
	}

	capacitor := flux.New[benchConfig](
		flux.NewSyncChannelWatcher(ch),
		func(_ benchConfig) error { return nil },
		flux.WithSyncMode(),
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
	ch <- []byte("value: 0\nname: initial")
	for i := 1; i <= b.N; i++ {
		ch <- []byte(fmt.Sprintf("value: %d\nname: test", i))
	}

	var lastApplied int

	capacitor := flux.New[benchConfig](
		flux.NewSyncChannelWatcher(ch),
		func(cfg benchConfig) error {
			lastApplied = cfg.Value
			return nil
		},
		flux.WithSyncMode(),
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
	ch <- []byte("value: 1\nname: valid") // Initial valid

	// Alternate valid/invalid
	for i := 0; i < b.N; i++ {
		ch <- []byte("value: -1\nname: invalid") // Invalid (value < 0)
		ch <- []byte("value: 1\nname: valid")    // Valid
	}

	capacitor := flux.New[benchConfig](
		flux.NewSyncChannelWatcher(ch),
		func(_ benchConfig) error { return nil },
		flux.WithSyncMode(),
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
