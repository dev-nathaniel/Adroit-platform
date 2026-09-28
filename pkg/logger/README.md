# Logger Package (`pkg/logger`)

The `logger` package wraps Go's standard `log/slog` to provide high-performance, structured JSON and text logging with automatic context attribute injection (such as `request_id`).

---

## 1. Features

* **Native `log/slog`:** Zero external dependencies.
* **Contextual Tracing:** Automatically extracts `request_id` from `context.Context` and appends it to log attributes without manually repeating keys in every log call.
* **Configurable Levels & Formats:** Supports `debug`, `info`, `warn`, `error` levels, and `json` or `text` formats.

---

## 2. Functions & API Reference

* `logger.New(cfg logger.Config) *slog.Logger`  
  Creates and returns a configured `*slog.Logger`.
* `logger.WithRequestID(ctx context.Context, requestID string) context.Context`  
  Returns a context enriched with the platform request ID.
* `logger.GetRequestID(ctx context.Context) string`  
  Retrieves the request ID string from the context.

---

## 3. Usage Example

```go
log := logger.New(logger.Config{
    Level:  "debug",
    Format: "json",
    Output: os.Stdout,
})

ctx := logger.WithRequestID(r.Context(), "req_abc123")
log.InfoContext(ctx, "Deployment initiated", slog.String("service_id", "svc_xyz"))
```

Output:
```json
{"time":"2026-09-28T17:20:00Z","level":"INFO","msg":"Deployment initiated","request_id":"req_abc123","service_id":"svc_xyz"}
```

---

## 4. How to Test

```bash
go test -v ./pkg/logger
```
