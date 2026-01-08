# flux/file

File watcher for flux using fsnotify.

## Installation

```bash
go get github.com/zoobzio/flux/file
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/zoobzio/flux"
    "github.com/zoobzio/flux/file"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    capacitor := flux.New[Config](
        file.New("/etc/myapp/config.json"),
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

None - uses fsnotify which works on Linux, macOS, and Windows.
