# flux/etcd

etcd watcher for flux using the native Watch API.

## Installation

```bash
go get github.com/zoobzio/flux/etcd
```

## Usage

```go
package main

import (
    "context"
    "log"
    "time"

    clientv3 "go.etcd.io/etcd/client/v3"
    "github.com/zoobzio/flux"
    fluxetcd "github.com/zoobzio/flux/etcd"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    client, err := clientv3.New(clientv3.Config{
        Endpoints:   []string{"localhost:2379"},
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        log.Fatalf("failed to create etcd client: %v", err)
    }
    defer client.Close()

    capacitor := flux.New[Config](
        fluxetcd.New(client, "/myapp/config"),
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

A running etcd cluster. By default connects to `localhost:2379`.
