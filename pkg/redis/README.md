# flux/pkg/redis

Redis watcher for flux using keyspace notifications.

## Installation

```bash
go get github.com/zoobzio/flux/pkg/redis
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/redis/go-redis/v9"
    "github.com/zoobzio/flux"
    fluxredis "github.com/zoobzio/flux/pkg/redis"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    client := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    capacitor := flux.New[Config](
        fluxredis.New(client, "myapp:config"),
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

Redis must have keyspace notifications enabled:

```bash
redis-cli CONFIG SET notify-keyspace-events KEA
```

Or in `redis.conf`:

```
notify-keyspace-events KEA
```
