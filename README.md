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

## Two Primitives

```go
// Single source - watch, validate, apply
capacitor := flux.New[Config](watcher, callback)

// Multiple sources - watch all, merge with reducer
capacitor := flux.Compose[Config](reducer, watchers)
```

Both provide the same guarantees: invalid config is rejected, previous valid config is retained, and your callback only receives validated data.

## Installation

```bash
go get github.com/zoobzio/flux
```

Requires Go 1.23+.

## Quick Start

```go
package main

import (
    "context"
    "errors"
    "log"

    "github.com/zoobzio/flux"
    "github.com/zoobzio/flux/pkg/file"
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
    capacitor := flux.New[Config](
        file.New("/etc/myapp/config.json"),
        func(prev, curr Config) error {
            log.Printf("Config changed: port %d -> %d", prev.Port, curr.Port)
            return nil
        },
    )

    if err := capacitor.Start(context.Background()); err != nil {
        log.Fatalf("Initial load failed: %v", err)
    }

    log.Printf("State: %s", capacitor.State()) // healthy, degraded, or empty

    if cfg, ok := capacitor.Current(); ok {
        log.Printf("Current port: %d", cfg.Port)
    }

    // Capacitor continues watching in background until context is cancelled
    select {}
}
```

## Why flux?

- **Safe by default** — Invalid config rejected, previous retained, callback only sees valid data
- **Four-state machine** — Loading, Healthy, Degraded, Empty with clear transitions
- **Multi-source composition** — Merge configs from files, Redis, Kubernetes, environment
- **Pluggable providers** — File, Redis, Consul, etcd, NATS, Kubernetes, ZooKeeper, Firestore
- **Observable** — [capitan](https://github.com/zoobzio/capitan) signals for state changes, failures, metrics
- **Testable** — Sync mode and channel watchers for deterministic tests

## Documentation

Full documentation is available in the [docs/](docs/) directory:

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

Contributions welcome! Please ensure:
- Tests pass: `go test ./...`
- Code is formatted: `go fmt ./...`
- No lint errors: `golangci-lint run`

## License

MIT License — see [LICENSE](LICENSE) for details.
