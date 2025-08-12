# flux

A Go library providing synchronization infrastructure for wrapping callbacks with rate limiting, circuit breaking, retries, and other resilience patterns.

## Overview

flux follows a builder pattern similar to zlog, allowing services to compose synchronization capabilities around their callbacks. It is designed to be embedded within services that need to manage synchronization, not run as a standalone service.

## Features

- **Rate Limiting**: Control request rates with configurable burst capacity
- **Circuit Breaking**: Prevent cascading failures with automatic recovery
- **Retry Logic**: Automatic retries with configurable attempts and backoff
- **Timeout Protection**: Prevent hanging operations
- **Error Handling**: Custom error pipelines for logging and recovery
- **Filtering**: Process events conditionally
- **Stream Processing**: Handle continuous event streams
- **Batch Processing**: Process events in batches for efficiency
- **Type Safety**: Full generic type support
- **Composable**: Chain multiple capabilities using builder pattern

## Installation

```bash
go get github.com/zoobzio/flux
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/zoobzio/flux"
)

func main() {
    // Create a sync wrapper with desired capabilities
    sync := flux.NewSync("user-processor", processUser).
        WithRateLimit(100, 10).                    // 100 RPS, burst 10
        WithCircuitBreaker(5, 30*time.Second).     // Open after 5 failures
        WithRetry(3).                              // Retry up to 3 times
        WithTimeout(5*time.Second)                 // 5 second timeout
    
    // Process events through the sync pipeline
    event := flux.Event[User]{
        ID:        "user-123",
        Data:      user,
        Source:    "user-service",
        Timestamp: time.Now(),
    }
    
    result, err := sync.Process(ctx, event)
    if err != nil {
        log.Printf("Processing failed: %v", err)
    }
}

func processUser(ctx context.Context, event flux.Event[User]) error {
    // Your processing logic here
    return updateDatabase(ctx, event.Data)
}
```

## Usage Examples

### Basic Synchronization

```go
// Simple sync with error handling
sync := flux.NewSync("handler", func(ctx context.Context, event flux.Event[Data]) error {
    log.Printf("Processing: %s", event.ID)
    return process(event.Data)
})
```

### Rate Limiting

```go
// Limit to 100 requests per second with burst of 10
sync := flux.NewSync("api", handler).
    WithRateLimit(100, 10)

// Drop events instead of waiting when rate limited
sync := flux.NewSync("analytics", handler).
    WithRateLimitDrop(1000, 100)
```

### Circuit Breaking

```go
// Open circuit after 5 consecutive failures, try recovery after 30s
sync := flux.NewSync("external-api", handler).
    WithCircuitBreaker(5, 30*time.Second)
```

### Retry Strategies

```go
// Simple retry - immediate retry up to 3 times
sync := flux.NewSync("db", handler).
    WithRetry(3)

// Exponential backoff - delays double each retry
sync := flux.NewSync("api", handler).
    WithBackoff(5, time.Second) // 1s, 2s, 4s, 8s, 16s
```

### Filtering

```go
// Only process specific events
sync := flux.NewSync("filtered", handler).
    WithFilter(func(ctx context.Context, e flux.Event[Data]) bool {
        return e.Source == "important" && 
               time.Since(e.Timestamp) < 5*time.Minute
    })
```

### Composition

```go
// Combine multiple capabilities
sync := flux.NewSync("resilient", handler).
    WithRateLimit(50, 5).
    WithCircuitBreaker(10, time.Minute).
    WithRetry(3).
    WithTimeout(10*time.Second)
```

### Error Handling

```go
// Create error handler using pipz
errorHandler := pipz.Effect("error-handler", func(ctx context.Context, err *pipz.Error[flux.Event[Data]]) error {
    log.Printf("Failed event %s: %v", err.InputData.ID, err.Err)
    return sendToDeadLetter(err.InputData)
})

// Attach to sync
sync := flux.NewSync("main", handler).
    WithErrorHandler(errorHandler)
```

## Stream Processing

For continuous event streams:

```go
// Create a stream processor
stream := flux.NewStream("events", sync).
    WithThrottle(50.0).      // 50 events/sec max
    WithBuffer(1000).        // Buffer up to 1000 events
    WithFilter(func(e flux.Event[Data]) bool {
        return e.Priority == "high"
    }).
    WithErrorStrategy(flux.ErrorChannel)  // Send errors to channel

// Process events from channel
events := make(chan flux.Event[Data])
go stream.Process(ctx, events)

// Monitor errors
for err := range stream.ErrorChannel() {
    log.Printf("Stream error: %v", err)
}
```

## Batch Processing

For efficient batch operations:

```go
// Create batch handler
batchSync := flux.NewBatchSync("bulk-insert", func(ctx context.Context, batch []flux.Event[User]) error {
    // Extract users from events
    users := make([]User, len(batch))
    for i, event := range batch {
        users[i] = event.Data
    }
    return db.BulkInsert(ctx, users)
}).
    WithRetry(3).
    WithTimeout(30*time.Second)

// Create batch stream
stream := flux.NewBatchStream("user-stream", batchSync).
    WithBatcher(100, time.Second)  // Batch up to 100 or every second

// Process events
stream.Process(ctx, userEvents)
```

## Integration Pattern

flux is designed to be embedded in services that need synchronization:

```go
type Service struct {
    syncs map[string]*flux.Sync[Record]
}

func (s *Service) Initialize() {
    // Configure different sync patterns for different use cases
    s.syncs["users"] = flux.NewSync("users", s.handleUser).
        WithRateLimit(100, 10).
        WithCircuitBreaker(5, 30*time.Second)
    
    s.syncs["orders"] = flux.NewSync("orders", s.handleOrder).
        WithRateLimit(50, 5).
        WithBackoff(5, time.Second).
        WithConcurrency(10)
}

func (s *Service) ProcessUpdate(table string, record Record) error {
    sync := s.syncs[table]
    event := flux.Event[Record]{
        ID:        record.ID,
        Data:      record,
        Source:    table,
        Timestamp: time.Now(),
    }
    
    _, err := sync.Process(context.Background(), event)
    return err
}
```

## Built on

- [pipz](https://github.com/zoobzio/pipz) - For composable data processing pipelines
- [streamz](https://github.com/zoobzio/streamz) - For channel-based stream processing

## License

[Add your license here]