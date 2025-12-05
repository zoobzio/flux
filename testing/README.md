# Testing Infrastructure for flux

This directory contains the comprehensive testing infrastructure for the flux package, designed to support robust development and maintenance of the configuration synchronization system.

## Directory Structure

```
testing/
├── README.md             # This file - testing strategy overview
├── integration/          # Integration tests
│   ├── README.md         # Integration testing documentation
│   └── file_watcher_test.go  # FileWatcher integration tests
└── benchmarks/           # Performance benchmarks
    ├── README.md         # Benchmark documentation
    └── capacitor_bench_test.go  # Core component benchmarks
```

## Testing Strategy

### Unit Tests (Root Package)
- **Location**: Alongside source files (`*_test.go`)
- **Purpose**: Test individual components in isolation
- **Coverage Goal**: 80%+
- **Features**: Uses clockz for deterministic timing

### Integration Tests (`testing/integration/`)
- **Purpose**: Test component interactions with real filesystem
- **Scope**: FileWatcher with actual file operations
- **Focus**: Ensure components work together correctly

### Benchmarks (`testing/benchmarks/`)
- **Purpose**: Measure and track performance characteristics
- **Scope**: Process pipeline, transform, state transitions
- **Focus**: Prevent performance regressions

## Running Tests

### All Tests
```bash
# Run all tests with coverage
go test -v -coverprofile=coverage.out ./...

# Generate coverage report
go tool cover -html=coverage.out
```

### Unit Tests Only
```bash
# Run only unit tests (root package)
go test -v .
```

### Integration Tests Only
```bash
# Run integration tests
go test -v ./testing/integration/...

# With race detection
go test -v -race ./testing/integration/...
```

### Benchmarks Only
```bash
# Run all benchmarks
go test -v -bench=. ./testing/benchmarks/...

# Run with memory allocation tracking
go test -v -bench=. -benchmem ./testing/benchmarks/...

# Run multiple times for statistical significance
go test -v -bench=. -count=5 ./testing/benchmarks/...
```

## Test Dependencies

### Required for Basic Testing
- Go 1.23+ (for testing features)
- clockz for deterministic timing

### Required for Advanced Testing
- Race detector: `go test -race`
- Coverage tools: `go tool cover`

### Optional Performance Tools
- `benchcmp` for comparing benchmark results
- `pprof` for performance profiling

## Best Practices

### Test Organization
1. **Hierarchical naming**: Use descriptive test names that form a hierarchy
2. **Consistent structure**: Follow the same pattern across all test files
3. **Isolated tests**: Each test should be completely independent
4. **Fast tests**: Keep unit tests fast, put slow tests in integration

### Capacitor Testing
1. **Use sync mode for deterministic tests**
2. **Use ChannelWatcher for controlled input**
3. **Verify state transitions**
4. **Test validation and rollback behavior**

### Concurrency Testing
1. **Use `-race` flag regularly**
2. **Test concurrent Start/Current/State operations**
3. **Verify proper cleanup in failure scenarios**

## Continuous Integration

### Pre-commit Checks
```bash
# Run this before committing
make test          # All tests with race detection
make lint          # Code quality checks
make coverage      # Coverage verification
```

### CI Pipeline Requirements
- All tests must pass
- Coverage must remain above 80%
- No race conditions detected
- Benchmarks must not regress significantly

---

This testing infrastructure ensures flux remains reliable, performant, and easy to use.
