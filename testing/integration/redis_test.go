package integration

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/zoobzio/flux"
	fluxredis "github.com/zoobzio/flux/pkg/redis"
)

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("failed to get endpoint: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: endpoint,
	})

	// Enable keyspace notifications
	if err := client.ConfigSet(ctx, "notify-keyspace-events", "KEA").Err(); err != nil {
		t.Fatalf("failed to enable keyspace notifications: %v", err)
	}

	return client
}

func TestCapacitor_Redis_InitialLoad(t *testing.T) {
	client := setupRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "config:test"
	cfg := appConfig{Feature: "redis-test", Limit: 200}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := client.Set(ctx, key, data, 0).Err(); err != nil {
		t.Fatalf("failed to set initial value: %v", err)
	}

	var applied appConfig

	capacitor := flux.New[appConfig](
		fluxredis.New(client, key),
		func(_ context.Context, _, cfg appConfig) error {
			applied = cfg
			return nil
		},
	)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if capacitor.State() != flux.StateHealthy {
		t.Errorf("expected StateHealthy, got %s", capacitor.State())
	}

	if applied.Feature != "redis-test" || applied.Limit != 200 {
		t.Errorf("unexpected applied config: %+v", applied)
	}
}

func TestCapacitor_Redis_LiveUpdate(t *testing.T) {
	client := setupRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "config:live"
	cfg := appConfig{Feature: "v1", Limit: 10}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := client.Set(ctx, key, data, 0).Err(); err != nil {
		t.Fatalf("failed to set initial value: %v", err)
	}

	var applyCount atomic.Int32
	var lastApplied atomic.Value

	capacitor := flux.New[appConfig](
		fluxredis.New(client, key),
		func(_ context.Context, _, cfg appConfig) error {
			applyCount.Add(1)
			lastApplied.Store(cfg)
			return nil
		},
	).Debounce(50 * time.Millisecond)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if applyCount.Load() != 1 {
		t.Errorf("expected 1 apply after start, got %d", applyCount.Load())
	}

	// Update config in Redis
	cfg2 := appConfig{Feature: "v2", Limit: 20}
	data2, err := json.Marshal(cfg2)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := client.Set(ctx, key, data2, 0).Err(); err != nil {
		t.Fatalf("failed to update value: %v", err)
	}

	// Wait for notification and debounced apply
	time.Sleep(500 * time.Millisecond)

	if applyCount.Load() != 2 {
		t.Errorf("expected 2 applies, got %d", applyCount.Load())
	}

	applied := lastApplied.Load().(appConfig)
	if applied.Feature != "v2" || applied.Limit != 20 {
		t.Errorf("unexpected applied config: %+v", applied)
	}
}

func TestCapacitor_Redis_InvalidUpdateRetainsPrevious(t *testing.T) {
	client := setupRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "config:retain"
	cfg := appConfig{Feature: "valid", Limit: 50}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := client.Set(ctx, key, data, 0).Err(); err != nil {
		t.Fatalf("failed to set initial value: %v", err)
	}

	capacitor := flux.New[appConfig](
		fluxredis.New(client, key),
		func(_ context.Context, _, _ appConfig) error {
			return nil
		},
	).Debounce(50 * time.Millisecond)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Write invalid config (limit -1 violates min=0)
	invalidCfg := appConfig{Feature: "invalid", Limit: -1}
	invalidData, err := json.Marshal(invalidCfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := client.Set(ctx, key, invalidData, 0).Err(); err != nil {
		t.Fatalf("failed to set invalid value: %v", err)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	if capacitor.State() != flux.StateDegraded {
		t.Errorf("expected StateDegraded, got %s", capacitor.State())
	}

	// Previous config should still be current
	current, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config to exist")
	}
	if current.Feature != "valid" || current.Limit != 50 {
		t.Errorf("expected previous config retained, got %+v", current)
	}

	if capacitor.LastError() == nil {
		t.Error("expected LastError to be set")
	}
}

func TestCapacitor_Redis_RecoveryFromDegraded(t *testing.T) {
	client := setupRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key := "config:recovery"
	cfg := appConfig{Feature: "v1", Limit: 10}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := client.Set(ctx, key, data, 0).Err(); err != nil {
		t.Fatalf("failed to set initial value: %v", err)
	}

	capacitor := flux.New[appConfig](
		fluxredis.New(client, key),
		func(_ context.Context, _, _ appConfig) error {
			return nil
		},
	).Debounce(50 * time.Millisecond)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Write invalid
	invalidCfg := appConfig{Feature: "bad", Limit: -1}
	invalidData, err := json.Marshal(invalidCfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := client.Set(ctx, key, invalidData, 0).Err(); err != nil {
		t.Fatalf("failed to set invalid value: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	if capacitor.State() != flux.StateDegraded {
		t.Errorf("expected StateDegraded, got %s", capacitor.State())
	}

	// Write valid again
	validCfg := appConfig{Feature: "recovered", Limit: 99}
	validData, err := json.Marshal(validCfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := client.Set(ctx, key, validData, 0).Err(); err != nil {
		t.Fatalf("failed to set valid value: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	if capacitor.State() != flux.StateHealthy {
		t.Errorf("expected StateHealthy after recovery, got %s", capacitor.State())
	}

	current, _ := capacitor.Current()
	if current.Feature != "recovered" {
		t.Errorf("expected 'recovered', got %s", current.Feature)
	}
}
