# flux

[![CI Status](https://github.com/zoobzio/flux/workflows/CI/badge.svg)](https://github.com/zoobzio/flux/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/flux/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/flux)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/flux)](https://goreportcard.com/report/github.com/zoobzio/flux)
[![CodeQL](https://github.com/zoobzio/flux/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/flux/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/flux.svg)](https://pkg.go.dev/github.com/zoobzio/flux)
[![License](https://img.shields.io/github/license/zoobzio/flux)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zoobzio/flux)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/flux)](https://github.com/zoobzio/flux/releases)

Reactive configuration synchronization for Go.

Watch external sources, validate changes, and apply them safely with automatic rollback on failure.

## Watch, Validate, Apply

A capacitor watches a source, validates incoming data, and calls you back when it changes.

```go
type Config struct {
    Port int    `json:"port"`
    Host string `json:"host"`
}

func (c Config) Validate() error {
    if c.Port < 1 || c.Port > 65535 {
        return errors.New("invalid port")
    }
    return nil
}

capacitor := flux.New[Config](
    file.New("/etc/app/config.json"),
    func(ctx context.Context, prev, curr Config) error {
        log.Printf("Port changed: %d -> %d", prev.Port, curr.Port)
        return reconfigureServer(curr)
    },
)
```

Start once, react forever. Invalid config is rejected, previous valid config is retained.

```go
if err := capacitor.Start(ctx); err != nil {
    log.Fatal("Initial load failed:", err)
}

// State machine tracks health
capacitor.State()     // Loading -> Healthy -> Degraded -> Empty

// Current config always available
if cfg, ok := capacitor.Current(); ok {
    server.Listen(cfg.Host, cfg.Port)
}

// Last error for observability
if err := capacitor.LastError(); err != nil {
    metrics.RecordConfigError(err)
}
```

Multiple sources? Compose them with a reducer.

```go
capacitor := flux.Compose[Config](
    func(ctx context.Context, prev, curr []Config) (Config, error) {
        // Merge defaults, file config, and environment overrides
        return mergeConfigs(curr[0], curr[1], curr[2]), nil
    },
    file.New("/etc/app/defaults.json"),
    file.New("/etc/app/config.json"),
    env.New("APP_"),
)
```

Same guarantees: validate first, reject invalid, retain previous, call back with valid.

## Install

```bash
go get github.com/zoobzio/flux
```

Requires Go 1.24+.

## Quick Start

```go
package main

import (
    "context"
    "errors"
    "log"

    "github.com/zoobzio/flux"
    "github.com/zoobzio/flux/file"
)

type Config struct {
    Port int    `json:"port"`
    Host string `json:"host"`
}

func (c Config) Validate() error {
    if c.Port < 1 || c.Port > 65535 {
        return errors.New("port must be between 1 and 65535")
    }
    if c.Host == "" {
        return errors.New("host is required")
    }
    return nil
}

func main() {
    ctx := context.Background()

    capacitor := flux.New[Config](
        file.New("/etc/myapp/config.json"),
        func(ctx context.Context, prev, curr Config) error {
            log.Printf("Config updated: %+v", curr)
            return nil
        },
    )

    if err := capacitor.Start(ctx); err != nil {
        log.Fatalf("Initial load failed: %v", err)
    }

    log.Printf("State: %s", capacitor.State())

    if cfg, ok := capacitor.Current(); ok {
        log.Printf("Listening on %s:%d", cfg.Host, cfg.Port)
    }

    // Capacitor watches in background until context is cancelled
    <-ctx.Done()
}
```

## Capabilities

| Feature | Description | Docs |
|---------|-------------|------|
| State Machine | Loading, Healthy, Degraded, Empty with clear transitions | [Concepts](docs/2.learn/2.concepts.md) |
| Multi-Source Composition | Merge configs from multiple watchers with custom reducers | [Multi-Source](docs/4.cookbook/2.multi-source.md) |
| Pluggable Providers | File, Redis, Consul, etcd, NATS, Kubernetes, ZooKeeper, Firestore | [Providers](docs/3.guides/2.providers.md) |
| Validation Pipeline | Type-safe validation with automatic rejection and rollback | [Architecture](docs/2.learn/3.architecture.md) |
| Debouncing | Configurable delay to batch rapid changes | [Best Practices](docs/3.guides/4.best-practices.md) |
| Signal Observability | State changes and errors via [capitan](https://github.com/zoobzio/capitan) | [Fields](docs/5.reference/2.fields.md) |
| Testing Utilities | Sync mode and channel watchers for deterministic tests | [Testing](docs/3.guides/1.testing.md) |

## Why flux?

- **Safe by default** — Invalid config rejected, previous retained, callback only sees valid data
- **Four-state machine** — Loading, Healthy, Degraded, Empty with clear transitions
- **Multi-source composition** — Merge configs from files, Redis, Kubernetes, environment
- **Pluggable providers** — File, Redis, Consul, etcd, NATS, Kubernetes, ZooKeeper, Firestore
- **Observable** — [capitan](https://github.com/zoobzio/capitan) signals for state changes and failures
- **Testable** — Sync mode and channel watchers for deterministic tests

## Configuration as a Service

Flux enables a pattern: **define once, update anywhere, validate always**.

Your configuration lives in external sources — files, Redis, Kubernetes ConfigMaps. Flux watches, validates, and delivers changes. Your application just reacts.

```go
// In your infrastructure
configMap := &corev1.ConfigMap{
    Data: map[string]string{
        "config": `{"port": 8080, "host": "0.0.0.0"}`,
    },
}

// In your application
capacitor := flux.New[Config](
    kubernetes.New(client, "default", "app-config", "config"),
    func(ctx context.Context, prev, curr Config) error {
        return server.Reconfigure(curr)
    },
)
```

Update the ConfigMap, flux delivers the change. Invalid update? Rejected. Previous config retained. Your application never sees bad data.

## Documentation

- [Overview](docs/1.overview.md) — Design philosophy and architecture

### Learn

- [Quickstart](docs/2.learn/1.quickstart.md) — Get started in minutes
- [Core Concepts](docs/2.learn/2.concepts.md) — Capacitor, watchers, state machine, validation
- [Architecture](docs/2.learn/3.architecture.md) — Processing pipeline, debouncing, error handling

### Guides

- [Testing](docs/3.guides/1.testing.md) — Sync mode, channel watchers, deterministic tests
- [Providers](docs/3.guides/2.providers.md) — Configuring file, Redis, Kubernetes, and other watchers
- [State Management](docs/3.guides/3.state.md) — State transitions, error recovery, circuit breakers
- [Best Practices](docs/3.guides/4.best-practices.md) — Validation design, graceful degradation, observability

### Cookbook

- [File Config](docs/4.cookbook/1.file-config.md) — Hot-reloading configuration files
- [Multi-Source](docs/4.cookbook/2.multi-source.md) — Merging defaults, files, and environment
- [Custom Watcher](docs/4.cookbook/3.custom-watcher.md) — Building watchers for custom sources

### Reference

- [API Reference](docs/5.reference/1.api.md) — Complete function and type documentation
- [Fields Reference](docs/5.reference/2.fields.md) — Capitan signal fields
- [Providers Reference](docs/5.reference/3.providers.md) — All provider packages and options

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. Run `make help` for available commands.

## License

MIT License — see [LICENSE](LICENSE) for details.
