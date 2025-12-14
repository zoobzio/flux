# flux/pkg/nats

NATS watcher for flux using JetStream KV Watch API.

## Installation

```bash
go get github.com/zoobzio/flux/pkg/nats
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
    "github.com/zoobzio/flux"
    fluxnats "github.com/zoobzio/flux/pkg/nats"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    nc, err := nats.Connect("nats://localhost:4222")
    if err != nil {
        log.Fatal(err)
    }

    js, err := jetstream.New(nc)
    if err != nil {
        log.Fatal(err)
    }

    kv, err := js.KeyValue(ctx, "config")
    if err != nil {
        log.Fatal(err)
    }

    capacitor := flux.New[Config](
        fluxnats.New(kv, "myapp"),
        func(prev, curr Config) error {
            log.Printf("config updated: %+v", curr)
            return nil
        },
    )

    if err := capacitor.Start(ctx); err != nil {
        log.Fatalf("failed to start: %v", err)
    }

    // Block forever
    select {}
}
```

## Requirements

NATS server must have JetStream enabled:

```bash
nats-server --jetstream
```

Create a KV bucket before use:

```bash
nats kv add config
nats kv put config myapp '{"port": 8080, "host": "localhost"}'
```
