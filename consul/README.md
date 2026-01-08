# flux/consul

Consul KV watcher for flux using blocking queries.

## Installation

```bash
go get github.com/zoobzio/flux/consul
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/hashicorp/consul/api"
    "github.com/zoobzio/flux"
    fluxconsul "github.com/zoobzio/flux/consul"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    client, err := api.NewClient(api.DefaultConfig())
    if err != nil {
        log.Fatalf("failed to create consul client: %v", err)
    }

    capacitor := flux.New[Config](
        fluxconsul.New(client, "myapp/config"),
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

A running Consul agent. By default connects to `localhost:8500`.

Configure via environment variables:

```bash
export CONSUL_HTTP_ADDR=consul.example.com:8500
export CONSUL_HTTP_TOKEN=your-acl-token
```
