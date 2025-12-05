# Benchmark Suite for flux

This directory contains performance benchmarks for the flux package. The benchmarks measure different aspects of configuration synchronization performance.

## Benchmark Categories

### Core Performance (`capacitor_bench_test.go`)
Benchmarks fundamental flux components:
- **Process Pipeline**: Full transform → validate → apply cycle
- **Transform Only**: Transform function overhead
- **Full Pipeline**: End-to-end processing
- **State Transitions**: State change overhead
- **Channel Forwarding**: Watcher to capacitor throughput

## Running Benchmarks

### All Benchmarks
```bash
# Run all benchmarks
go test -v -bench=. ./testing/benchmarks/...

# Run with memory allocation tracking
go test -v -bench=. -benchmem ./testing/benchmarks/...

# Run multiple times for statistical significance
go test -v -bench=. -count=5 ./testing/benchmarks/...
```

### Performance Profiling
```bash
# CPU profiling
go test -bench=BenchmarkCapacitor -cpuprofile=cpu.prof ./testing/benchmarks/...
go tool pprof cpu.prof

# Memory profiling
go test -bench=BenchmarkCapacitor -memprofile=mem.prof ./testing/benchmarks/...
go tool pprof mem.prof
```

## Benchmark Metrics

### Performance Targets
Based on expected performance, flux aims for:
- **Process Pipeline**: < 1µs/op for simple transforms
- **State Transitions**: < 100ns/op
- **Channel Forwarding**: Minimal overhead over raw channel

### Interpreting Results
```
BenchmarkCapacitor_Process-8    1000000    950 ns/op    128 B/op    2 allocs/op
```
- **Name**: BenchmarkCapacitor_Process-8 (8 CPU cores used)
- **Iterations**: 1,000,000 iterations run
- **Time per op**: 950 nanoseconds per operation
- **Memory per op**: 128 bytes allocated per operation
- **Allocations per op**: 2 allocations per operation

### Performance Regression Detection
Use `benchcmp` to compare results over time:
```bash
# Save baseline
go test -bench=. ./testing/benchmarks/... > baseline.txt

# After changes
go test -bench=. ./testing/benchmarks/... > current.txt

# Compare
benchcmp baseline.txt current.txt
```

## Benchmark Environment

### Requirements
- Go 1.23+ for testing features
- Stable system load (avoid running during builds/deployments)
- Consistent CPU frequency (disable CPU scaling if needed)

### Adding New Benchmarks
Follow these patterns when adding benchmarks:

1. **Benchmark naming**: `BenchmarkCapacitor_Specific`
2. **Setup/teardown**: Use `b.ResetTimer()` after setup
3. **Loop structure**: Standard `for i := 0; i < b.N; i++` loop
4. **Memory measurement**: Include `-benchmem` for relevant benchmarks

### Example Benchmark Structure
```go
func BenchmarkCapacitor_Process(b *testing.B) {
    // Setup (not measured)
    ch := make(chan []byte, 1)
    capacitor := flux.New(
        flux.NewSyncChannelWatcher(ch),
        transform,
        validate,
        apply,
        flux.WithSyncMode(),
    )

    b.ResetTimer() // Start timing here
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        ch <- []byte("test data")
        capacitor.Start(context.Background())
    }
}
```

---

The benchmark suite ensures flux maintains excellent performance characteristics.
