# Examples

This directory contains before/after examples demonstrating the value of flux.

## Available Examples

### [Config Reload](./config-reload/)

**The pain**: Config reload requires restart, or live reload with no safety net.

**The fix**: Capacitor enables live config updates with validation and automatic rollback.

```bash
# See the problems
go run ./config-reload/before/

# See the solution
go run ./config-reload/after/
```

### [Feature Flags](./feature-flags/)

**The pain**: Feature flags require restart to change, or live reload with no validation.

**The fix**: Capacitor validates flag combinations before applying, retaining previous flags on failure.

```bash
# See the problems
go run ./feature-flags/before/

# See the solution
go run ./feature-flags/after/
```

## Pattern

Each example follows the same structure:

```
example-name/
├── README.md       # Explains the pain and the fix
├── before/
│   └── main.go     # The typical approach - shows problems
└── after/
    └── main.go     # Using flux - shows improvements
```

## Running Examples

All examples are self-contained and can be run directly:

```bash
# Run any example
go run ./example-name/before/
go run ./example-name/after/
```

## Key Improvements Demonstrated

1. **Validation Before Apply** - Invalid config never reaches your application
2. **Automatic Rollback** - Previous valid config retained on failure
3. **State Visibility** - Clear Healthy/Degraded/Empty states
4. **Observability** - Capitan signals for audit trails and alerting
5. **Debouncing** - Coalesces rapid changes to prevent thrashing
