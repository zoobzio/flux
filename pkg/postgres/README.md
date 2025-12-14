# flux/pkg/postgres

PostgreSQL watcher for flux using LISTEN/NOTIFY.

## Installation

```bash
go get github.com/zoobzio/flux/pkg/postgres
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/zoobzio/flux"
    fluxpg "github.com/zoobzio/flux/pkg/postgres"
)

type Config struct {
    Port int    `json:"port" validate:"min=1,max=65535"`
    Host string `json:"host" validate:"required"`
}

func main() {
    ctx := context.Background()

    pool, err := pgxpool.New(ctx, "postgres://user:pass@localhost:5432/mydb")
    if err != nil {
        log.Fatalf("failed to create pool: %v", err)
    }
    defer pool.Close()

    capacitor := flux.New[Config](
        fluxpg.New(pool, "config_changed", "myapp"),
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

### Schema

Create a config table:

```sql
CREATE TABLE config (
    key TEXT PRIMARY KEY,
    value BYTEA NOT NULL
);
```

### Trigger

Set up a trigger to send notifications on changes:

```sql
CREATE OR REPLACE FUNCTION notify_config_change() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('config_changed', NEW.key);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER config_change_trigger
    AFTER INSERT OR UPDATE ON config
    FOR EACH ROW EXECUTE FUNCTION notify_config_change();
```

## Options

### WithTable

Use a custom table name (defaults to "config"):

```go
fluxpg.New(pool, "config_changed", "myapp", fluxpg.WithTable("app_settings"))
```
