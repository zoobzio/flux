# flux/pkg/firestore

Firestore watcher for flux using realtime listeners.

## Installation

```bash
go get github.com/zoobzio/flux/pkg/firestore
```

## Usage

```go
package main

import (
    "context"
    "log"

    "cloud.google.com/go/firestore"
    "github.com/zoobzio/flux"
    fluxfs "github.com/zoobzio/flux/pkg/firestore"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    client, err := firestore.NewClient(ctx, "my-project")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    capacitor := flux.New[Config](
        fluxfs.New(client, "config", "myapp"),
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

## Options

### WithField

Watch a specific field within the document:

```go
fluxfs.New(client, "config", "myapp", fluxfs.WithField("settings"))
```

## Document Structure

By default, the watcher expects the document to have a `data` field containing the JSON configuration:

```json
{
  "data": "{\"port\": 8080, \"host\": \"localhost\"}"
}
```

Use the helper functions to create/update documents:

```go
fluxfs.CreateDocument(ctx, client, "config", "myapp", []byte(`{"port": 8080}`))
fluxfs.UpdateDocument(ctx, client, "config", "myapp", []byte(`{"port": 9090}`))
```
