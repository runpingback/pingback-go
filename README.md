# pingback-go

Go SDK for [Pingback](https://pingback.lol) — reliable cron jobs and background tasks.

## Installation

```bash
go get github.com/champ3oy/pingback-go
```

## Quick Start

```go
package main

import (
    "encoding/json"
    "net/http"
    "os"

    pingback "github.com/champ3oy/pingback-go"
)

func main() {
    pb := pingback.New(
        os.Getenv("PINGBACK_API_KEY"),
        os.Getenv("PINGBACK_CRON_SECRET"),
    )

    pb.Cron("cleanup", "0 3 * * *", func(ctx *pingback.Context) (any, error) {
        expired := removeExpiredSessions()
        ctx.Log("Removed sessions", "count", expired)
        return map[string]int{"removed": expired}, nil
    }, pingback.WithRetries(2))

    pb.Task("send-email", func(ctx *pingback.Context) (any, error) {
        var p EmailPayload
        json.Unmarshal(ctx.Payload, &p)
        sendEmail(p)
        ctx.Log("Sent email", "to", p.To)
        return nil, nil
    }, pingback.WithTimeout("15s"))

    http.Handle("/api/pingback", pb.Handler())
    http.ListenAndServe(":8080", nil)
}
```

## Defining Functions

### Cron Jobs

```go
pb.Cron("daily-report", "0 9 * * *", func(ctx *pingback.Context) (any, error) {
    report := generateReport()
    ctx.Log("Report generated", "rows", report.RowCount)
    return report, nil
}, pingback.WithRetries(3), pingback.WithTimeout("60s"))
```

### Background Tasks

```go
pb.Task("process-upload", func(ctx *pingback.Context) (any, error) {
    var p UploadPayload
    json.Unmarshal(ctx.Payload, &p)
    result, err := processFile(p.FileID)
    if err != nil {
        ctx.Error("Processing failed", "fileId", p.FileID, "error", err.Error())
        return nil, err
    }
    ctx.Log("Processed file", "fileId", p.FileID)
    return result, nil
}, pingback.WithRetries(2), pingback.WithTimeout("5m"))
```

### Fan-Out

```go
pb.Cron("send-emails", "*/15 * * * *", func(ctx *pingback.Context) (any, error) {
    pending := getPendingEmails()
    for _, email := range pending {
        ctx.Task("send-email", map[string]string{"id": email.ID})
    }
    ctx.Log("Dispatched emails", "count", len(pending))
    return map[string]int{"dispatched": len(pending)}, nil
})
```

## Programmatic Triggering

```go
pb := pingback.New(os.Getenv("PINGBACK_API_KEY"), os.Getenv("PINGBACK_CRON_SECRET"))

execID, err := pb.Trigger(context.Background(), "send-email", map[string]string{
    "to": "user@example.com",
})
```

## Structured Logging

```go
ctx.Log("message")                         // info
ctx.Log("message", "key", "value")         // info with metadata
ctx.Warn("slow query", "ms", 2500)         // warning
ctx.Error("failed", "code", "E001")        // error
ctx.Debug("cache stats", "hits", 847)      // debug
```

## Configuration

```go
pb := pingback.New(
    apiKey,
    cronSecret,
    pingback.WithPlatformURL("https://api.pingback.lol"),  // default
    pingback.WithBaseURL("https://myapp.com"),              // your app's public URL
)
```

### Function Options

```go
pingback.WithRetries(3)          // retry up to 3 times (default: 0)
pingback.WithTimeout("30s")      // timeout after 30 seconds
pingback.WithConcurrency(5)      // allow 5 concurrent runs (default: 1)
```

## Environment Variables

```
PINGBACK_API_KEY=pb_live_...        # From your Pingback project settings
PINGBACK_CRON_SECRET=...            # From your Pingback project settings
```

## How It Works

1. You define cron jobs and tasks with `pb.Cron()` and `pb.Task()`
2. Mount the handler with `http.Handle("/api/pingback", pb.Handler())`
3. On first request, the SDK registers your functions with the Pingback platform
4. The platform sends signed HTTP requests to your handler when jobs are due
5. The handler verifies the HMAC signature, executes the function, and returns results
6. Fan-out tasks are dispatched independently by the platform
