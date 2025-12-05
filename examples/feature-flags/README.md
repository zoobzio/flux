# Feature Flags

**The pain**: Feature flags require restart to change, or live reload with no safety net.

**The fix**: Capacitor enables live flag updates with validation and rollback.

## Run Both

```bash
# See the problems
go run ./before/

# See the solution
go run ./after/
```

## Before: The Typical Approach

```go
// Option A: Static flags - require restart
var enableNewUI = os.Getenv("ENABLE_NEW_UI") == "true"

// Option B: Live reload - no validation
func reloadFlags() {
    data, _ := os.ReadFile("flags.json")
    json.Unmarshal(data, &flags) // Hope it's valid!
}
```

**Problems:**
- Static flags require deployment to change
- Live reload can apply invalid flag combinations
- No rollback when flags break the app
- Hard to audit what changed when

## After: Safe Live Flags

```go
capacitor := flux.New(
    flux.NewFileWatcher("flags.json"),
    parseFlags,
    validateFlags,  // Check flag combinations
    applyFlags,
)
```

**Wins:**
- Change flags without restart
- Invalid combinations rejected
- Previous flags retained on failure
- Capitan signals for audit trail

## The Demo

The example shows:
1. Initial flags → Applied
2. Invalid flag combination → Rejected, previous retained
3. Valid update → Applied

Feature flags often have dependencies (e.g., `new_checkout` requires `new_cart`). Flux validates these before applying.
