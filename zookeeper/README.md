# flux/zookeeper

ZooKeeper watcher for flux using the native Watch API.

## Installation

```bash
go get github.com/zoobzio/flux/zookeeper
```

## Usage

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/go-zookeeper/zk"
    "github.com/zoobzio/flux"
    fluxzk "github.com/zoobzio/flux/zookeeper"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    conn, _, err := zk.Connect([]string{"localhost:2181"}, 5*time.Second)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    capacitor := flux.New[Config](
        fluxzk.New(conn, "/config/myapp"),
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

## Setup

Create the ZooKeeper node before use:

```bash
zkCli.sh
create /config ""
create /config/myapp '{"port": 8080, "host": "localhost"}'
```
