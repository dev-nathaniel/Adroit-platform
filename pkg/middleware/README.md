# Middleware Package (`pkg/middleware`)

The `middleware` package provides production-grade HTTP middlewares compatible with Go standard library `http.Handler` and Chi router chains.

---

## 1. Available Middlewares

### `middleware.RequestID`
* **Signature:** `func(http.Handler) http.Handler`
* **Function:** Extracts incoming `X-Request-ID` from request headers or generates a cryptographically random 16-byte hex token. Injects the ID into the HTTP context and mirrors it to the response header `X-Request-ID`.

### `middleware.Logging(log *slog.Logger)`
* **Signature:** `func(*slog.Logger) func(http.Handler) http.Handler`
* **Function:** Logs each incoming HTTP request using structured JSON. Records `request_id`, HTTP `method`, `path`, `status` code, elapsed `duration`, `remote_addr`, and `user_agent`.

### `middleware.Recovery(log *slog.Logger)`
* **Signature:** `func(*slog.Logger) func(http.Handler) http.Handler`
* **Function:** Recovers from unhandled panics inside handlers, logs the panic details with the request path, and safely writes a `500 Internal Server Error` JSON response to the client instead of crashing the process.

### `middleware.CORS(allowedOrigins string)`
* **Signature:** `func(string) func(http.Handler) http.Handler`
* **Function:** Configures Cross-Origin Resource Sharing headers (`Access-Control-Allow-Origin`, `Methods`, `Headers`, `Credentials`), and handles `OPTIONS` preflight requests immediately with `204 No Content`.

---

## 2. Usage with Chi Router

```go
r := chi.NewRouter()

r.Use(middleware.RequestID)
r.Use(middleware.Recovery(log))
r.Use(middleware.Logging(log))
r.Use(middleware.CORS("*"))
```

---

## 3. How to Test

```bash
go test -v ./pkg/middleware
```
