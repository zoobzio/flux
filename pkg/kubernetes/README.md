# flux/pkg/kubernetes

Kubernetes watcher for flux using ConfigMap and Secret Watch API.

## Installation

```bash
go get github.com/zoobzio/flux/pkg/kubernetes
```

## Usage

### ConfigMap

```go
package main

import (
    "context"
    "log"

    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "github.com/zoobzio/flux"
    fluxk8s "github.com/zoobzio/flux/pkg/kubernetes"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    config, err := rest.InClusterConfig()
    if err != nil {
        log.Fatal(err)
    }

    client, err := kubernetes.NewForConfig(config)
    if err != nil {
        log.Fatal(err)
    }

    capacitor := flux.New[Config](
        fluxk8s.New(client, "default", "myapp-config", "config.json"),
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

### Secret

```go
capacitor := flux.New[Config](
    fluxk8s.New(client, "default", "myapp-secret", "config.json",
        fluxk8s.WithResourceType(fluxk8s.Secret),
    ),
    func(prev, curr Config) error {
        log.Printf("config updated: %+v", curr)
        return nil
    },
)
```

## ConfigMap Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: myapp-config
  namespace: default
data:
  config.json: |
    {"port": 8080, "host": "localhost"}
```
