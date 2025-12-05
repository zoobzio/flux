---
title: Flux Documentation
description: Reactive configuration synchronization for Go with automatic rollback and state management.
author: Flux Team
published: 2025-12-03
tags: [Documentation, Overview, Getting Started]
---

# Flux Documentation

Welcome to flux - reactive configuration synchronization for Go.

## Start Here

- **[Getting Started](./2.tutorials/1.quickstart.md)** - Sync your first config file in 5 minutes
- **[Core Concepts](./1.learn/2.core-concepts.md)** - Understand capacitors, watchers, and state
- **[API Reference](./4.reference/1.api.md)** - Complete API documentation

## Learn

- **[Introduction](./1.learn/1.introduction.md)** - Why flux exists and what problems it solves
- **[Core Concepts](./1.learn/2.core-concepts.md)** - Mental models and fundamentals
- **[Architecture](./1.learn/3.architecture.md)** - State machine design and data flow

## Tutorials

- **[Quickstart](./2.tutorials/1.quickstart.md)** - Your first config sync in 5 minutes
- **[File Config Sync](./2.tutorials/2.file-config-sync.md)** - Complete file-based configuration example

## Guides

- **[Testing](./3.guides/1.testing.md)** - Testing patterns with sync mode and clockz
- **[Custom Watchers](./3.guides/2.custom-watchers.md)** - Implementing custom data sources
- **[State Management](./3.guides/3.state-management.md)** - Handling state transitions and rollback

## Reference

- **[API Reference](./4.reference/1.api.md)** - Complete API documentation
- **[Signals](./4.reference/2.signals.md)** - Capitan signal definitions

## Quick Links

- [GitHub Repository](https://github.com/zoobzio/flux)
- [Go Package Documentation](https://pkg.go.dev/github.com/zoobzio/flux)

## Features

### Reactive Configuration

Watch external sources and automatically apply changes. Flux handles unmarshaling and validation via struct tags:

```go
type Config struct {
    Port int `yaml:"port" validate:"min=1,max=65535"`
}

capacitor := flux.New[Config](
    flux.NewFileWatcher("/etc/myapp/config.yaml"),
    func(cfg Config) error {
        return app.SetConfig(cfg)
    },
)

capacitor.Start(ctx)
```

### Automatic Rollback

Failed updates preserve the last known good configuration:

```go
// If validation or callback fails:
// - State transitions to Degraded
// - Previous config remains active
// - Error is captured for inspection
if capacitor.State() == flux.StateDegraded {
    log.Printf("Config error: %v", capacitor.LastError())
    cfg, _ := capacitor.Current() // Still returns last valid config
}
```

### State Machine

Clear lifecycle states for operational visibility:

```
Loading → Healthy (valid config applied)
        → Empty   (initial load failed)

Healthy → Degraded (update failed, previous config retained)
        → Healthy  (new valid config applied)

Degraded → Healthy (recovery with valid config)
         → Degraded (another failure)
```

### Automatic Validation

Struct tags power both unmarshaling and validation:

```go
type Config struct {
    Port     int    `yaml:"port" validate:"min=1,max=65535"`
    Host     string `yaml:"host" validate:"required"`
    Timeout  int    `yaml:"timeout" validate:"gte=0"`
}
```

Flux uses [go-playground/validator](https://github.com/go-playground/validator) for validation.

### Debounced Updates

Rapid changes are coalesced to prevent thrashing:

```go
capacitor := flux.New[Config](
    watcher,
    callback,
    flux.WithDebounce(100*time.Millisecond),
)
```

### Observability

Built-in capitan signals for monitoring:

```go
// Listen to state changes
capitan.Hook(flux.CapacitorStateChanged, func(ctx context.Context, e *capitan.Event) {
    oldState, _ := flux.KeyOldState.From(e)
    newState, _ := flux.KeyNewState.From(e)
    log.Printf("State: %s → %s", oldState, newState)
})
```

### Testable Design

Sync mode and fake clock for deterministic testing:

```go
capacitor := flux.New[Config](
    flux.NewSyncChannelWatcher(ch),
    callback,
    flux.WithSyncMode(),
)

capacitor.Start(ctx)
capacitor.Process(ctx) // Manual, deterministic processing
```
