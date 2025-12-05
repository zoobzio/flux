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

## The Problem

Configuration reload is tricky:

```go
// Option A: Static config - requires restart
var config = loadConfig()

// Option B: Live reload - no safety net
func reloadConfig() {
    data, _ := os.ReadFile("config.yaml")
    yaml.Unmarshal(data, &config) // Hope it's valid!
}
```

**Problems:**
- Invalid config corrupts application state
- No rollback when bad config breaks the app
- No visibility into configuration health

## The Solution

Flux provides `Capacitor` - a reactive primitive that watches, validates, and applies configuration changes safely:

```go
type Config struct {
    Port int `yaml:"port" validate:"min=1,max=65535"`
}

capacitor := flux.New[Config](
    flux.NewFileWatcher("config.yaml"),
    func(cfg Config) error {
        // Only called with valid, parsed config
        return app.SetConfig(cfg)
    },
)

capacitor.Start(ctx)
```

**Wins:**
- Automatic YAML/JSON unmarshaling via struct tags
- Automatic validation via [go-playground/validator](https://github.com/go-playground/validator) tags
- Invalid config rejected, previous retained
- Clear state machine (Healthy/Degraded/Empty)
- Capitan signals for observability

## Quick Start

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/zoobzio/flux"
)

type Config struct {
    Port        int    `yaml:"port" json:"port" validate:"min=1,max=65535"`
    DatabaseURL string `yaml:"database_url" json:"database_url" validate:"required"`
    MaxConns    int    `yaml:"max_conns" json:"max_conns" validate:"min=1"`
}

func main() {
    capacitor := flux.New[Config](
        flux.NewFileWatcher("/etc/myapp/config.yaml"),

        // Callback: Only called with valid, parsed config
        func(cfg Config) error {
            // Update your application
            log.Printf("Config applied: port=%d", cfg.Port)
            return nil
        },

        flux.WithDebounce(100*time.Millisecond),
    )

    ctx := context.Background()
    if err := capacitor.Start(ctx); err != nil {
        log.Printf("Initial load failed: %v", err)
    }

    // Check state anytime
    log.Println(capacitor.State()) // "healthy", "degraded", or "empty"

    // Get current config
    if cfg, ok := capacitor.Current(); ok {
        log.Printf("Port: %d", cfg.Port)
    }
}
```

Flux automatically:
1. Detects YAML or JSON format (by `{` or `[` prefix)
2. Unmarshals to your struct via `yaml`/`json` tags
3. Validates using `validate` struct tags
4. Only calls your callback if everything passes

To enforce a specific format instead of auto-detection:

```go
flux.WithJSON()  // Only accept JSON - YAML will fail
flux.WithYAML()  // Always parse as YAML (also accepts JSON)
```

## State Machine

```
┌─────────┐   valid    ┌─────────┐
│ Loading │──────────▶│ Healthy │◀──┐
└─────────┘            └─────────┘   │
     │                      │        │
     │ invalid              │ invalid│ valid
     ▼                      ▼        │
┌─────────┐            ┌─────────┐───┘
│  Empty  │            │ Degraded│
└─────────┘            └─────────┘
```

- **Loading** - Initial state, no config yet
- **Healthy** - Valid config applied
- **Degraded** - Last change failed, previous config still active
- **Empty** - Initial load failed, no valid config ever obtained

## Multi-Source Composition

Combine multiple configuration sources with `Compose`:

```go
type Config struct {
    Port    int    `yaml:"port" validate:"min=1,max=65535"`
    Timeout int    `yaml:"timeout" validate:"min=0"`
    Debug   bool   `yaml:"debug"`
}

capacitor := flux.Compose[Config](
    // Reducer receives all parsed configs - you merge and return result
    func(configs []Config) (Config, error) {
        merged := configs[0]  // Start with defaults

        // Override with file config
        if configs[1].Port != 0 {
            merged.Port = configs[1].Port
        }
        if configs[1].Timeout != 0 {
            merged.Timeout = configs[1].Timeout
        }

        // Env config has highest priority
        if configs[2].Port != 0 {
            merged.Port = configs[2].Port
        }
        merged.Debug = configs[2].Debug

        return merged, nil
    },
    defaultsWatcher,  // Lowest priority
    fileWatcher,      // Medium priority
    envWatcher,       // Highest priority
)
```

Each source emits raw bytes. Flux parses and validates each one independently, then passes the slice of parsed configs to your reducer. The merged result is stored and accessible via `Current()`.

## Observability

Flux emits [capitan](https://github.com/zoobzio/capitan) signals for monitoring:

```go
capitan.Hook(flux.CapacitorStateChanged, func(_ context.Context, e *capitan.Event) {
    oldState, _ := flux.KeyOldState.From(e)
    newState, _ := flux.KeyNewState.From(e)
    log.Printf("Config state: %s → %s", oldState, newState)
})

capitan.Hook(flux.CapacitorValidationFailed, func(_ context.Context, e *capitan.Event) {
    errMsg, _ := flux.KeyError.From(e)
    log.Printf("Config rejected: %s", errMsg)
})
```

Available signals:
- `CapacitorStarted` - Watching started
- `CapacitorStopped` - Watching stopped
- `CapacitorStateChanged` - State transition
- `CapacitorChangeReceived` - Raw change from watcher
- `CapacitorTransformFailed` - Unmarshal error
- `CapacitorValidationFailed` - Validation error
- `CapacitorApplyFailed` - Callback error
- `CapacitorApplySucceeded` - Config applied

## Installation

```bash
go get github.com/zoobzio/flux
```

Requirements: Go 1.23+

## Custom Watchers

Implement the `Watcher` interface for custom sources:

```go
type Watcher interface {
    Watch(ctx context.Context) (<-chan []byte, error)
}
```

Example polling watcher:

```go
type PollingWatcher struct {
    fetch    func(context.Context) ([]byte, error)
    interval time.Duration
}

func (w *PollingWatcher) Watch(ctx context.Context) (<-chan []byte, error) {
    ch := make(chan []byte)
    go func() {
        defer close(ch)
        ticker := time.NewTicker(w.interval)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                if data, err := w.fetch(ctx); err == nil {
                    ch <- data
                }
            }
        }
    }()
    return ch, nil
}
```

## Testing

Flux provides `ChannelWatcher` and sync mode for deterministic testing:

```go
func TestConfig(t *testing.T) {
    ch := make(chan []byte, 1)
    watcher := flux.NewSyncChannelWatcher(ch)

    var applied TestConfig
    capacitor := flux.New[TestConfig](
        watcher,
        func(cfg TestConfig) error {
            applied = cfg
            return nil
        },
        flux.WithSyncMode(), // Synchronous processing
    )

    // Send config and verify immediately
    ch <- []byte("port: 8080\nhost: localhost")
    capacitor.Start(context.Background())

    assert.Equal(t, 8080, applied.Port)
    assert.Equal(t, flux.StateHealthy, capacitor.State())
}
```

## Validation

Flux uses [go-playground/validator](https://github.com/go-playground/validator) for struct tag validation. Common tags:

```go
type Config struct {
    Port     int    `validate:"min=1,max=65535"`      // Range
    Host     string `validate:"required"`              // Required
    URL      string `validate:"url"`                   // URL format
    Email    string `validate:"email"`                 // Email format
    Timeout  int    `validate:"gte=0,lte=300"`        // Greater/less than
    LogLevel string `validate:"oneof=debug info warn"` // Enum
}
```

For complex validation (e.g., field dependencies), return an error from your callback:

```go
capacitor := flux.New[Config](
    watcher,
    func(cfg Config) error {
        // Business rule: checkout requires cart
        if cfg.EnableCheckout && !cfg.EnableCart {
            return errors.New("checkout requires cart")
        }
        return app.SetConfig(cfg)
    },
)
```

## Contributing

Contributions welcome! Please ensure:
- Tests pass: `go test ./...`
- Code is formatted: `go fmt ./...`
- No lint errors: `golangci-lint run`

## License

MIT License - see [LICENSE](LICENSE) file for details.
