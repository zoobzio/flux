# Config Reload

**The pain**: Manual file watching with no validation, no rollback, and crashes on bad config.

**The fix**: Capacitor validates before applying and retains the previous config on failure.

## Run Both

```bash
# See the problems
go run ./before/

# See the solution
go run ./after/
```

## Before: The Typical Approach

```go
watcher.Add(configPath)
for event := range watcher.Events {
    data, _ := os.ReadFile(configPath)
    yaml.Unmarshal(data, &config)
    app.SetConfig(config) // Hope it's valid!
}
```

**Problems:**
- Invalid config crashes or corrupts state
- No rollback - bad config stays applied
- Parse errors leave config in unknown state
- No visibility into what happened
- Hard to test file watching logic

## After: Validated Reload with Rollback

```go
capacitor := flux.New(
    flux.NewFileWatcher(configPath),
    parseConfig,
    validateConfig,
    applyConfig,
)
capacitor.Start(ctx)
```

**Wins:**
- Invalid config rejected before apply
- Previous config retained on failure
- State machine tracks health (Healthy/Degraded/Empty)
- Capitan signals for observability
- Sync mode for deterministic testing

## The Demo

The example writes a config file, then simulates:
1. Initial valid config → Applied
2. Invalid config (negative port) → Rejected, previous retained
3. Valid config again → Applied, recovered

Watch the state transitions and see how flux protects your application.
