# Integration Tests for flux

This directory contains integration tests that verify how flux components work together with real filesystem operations.

## Test Categories

### FileWatcher Tests (`file_watcher_test.go`)
Tests FileWatcher with actual file operations:
- File creation and initial read
- File modification detection
- Context cancellation
- Eventual consistency with rapid writes

## Running Integration Tests

### All Integration Tests
```bash
go test -v ./testing/integration/...
```

### With Race Detection
```bash
go test -v -race ./testing/integration/...
```

### With Extended Timeout
```bash
# Run with extended timeout for stress tests
go test -v -timeout=5m ./testing/integration/...
```

## Test Patterns

### FileWatcher Testing
Integration tests for FileWatcher use real temporary files:

```go
func TestFileWatcher_Integration(t *testing.T) {
    // Create temp file
    dir := t.TempDir()
    path := filepath.Join(dir, "config.yaml")
    os.WriteFile(path, []byte("initial"), 0644)

    // Create watcher
    watcher := flux.NewFileWatcher(path)
    ch, _ := watcher.Watch(ctx)

    // Receive initial content
    data := <-ch
    assert.Equal(t, "initial", string(data))

    // Modify file
    os.WriteFile(path, []byte("updated"), 0644)

    // Receive update
    data = <-ch
    assert.Equal(t, "updated", string(data))
}
```

### Eventual Consistency Testing
For tests involving filesystem events, test the final state rather than intermediate states:

```go
func TestFileWatcher_EventuallySeesLatestValue(t *testing.T) {
    // Write multiple times rapidly
    for i := 0; i < 10; i++ {
        os.WriteFile(path, []byte(fmt.Sprintf("value-%d", i)), 0644)
    }

    // Wait for final value
    var lastValue string
    timeout := time.After(2 * time.Second)
    for {
        select {
        case data := <-ch:
            lastValue = string(data)
            if lastValue == "value-9" {
                return // Success
            }
        case <-timeout:
            t.Fatalf("expected value-9, got %s", lastValue)
        }
    }
}
```

## Guidelines

### When to Add Integration Tests
- Testing FileWatcher with real filesystem
- Testing end-to-end Capacitor with FileWatcher
- Verifying filesystem event handling

### Test Structure
1. **Setup**: Create temp files and watchers
2. **Execute**: Perform file operations
3. **Verify**: Check received events or final state
4. **Cleanup**: Handled by t.TempDir()

### Best Practices
- Use `t.TempDir()` for automatic cleanup
- Test eventual consistency, not exact event sequences
- Include timeout handling
- Avoid time.Sleep for synchronization

## Performance Considerations

- Integration tests should complete in seconds, not minutes
- Use realistic but small file sizes
- Focus on correctness rather than throughput

## Debugging Integration Tests

### Verbose Output
```bash
go test -v -run TestFileWatcher ./testing/integration/...
```

### With Tracing
```bash
go test -v -trace=trace.out ./testing/integration/...
go tool trace trace.out
```

### Race Detection Details
```bash
go test -v -race ./testing/integration/...
```

---

Integration tests ensure that flux components work correctly together with real filesystem operations.
