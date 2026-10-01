# Middleware Package (`pkg/middleware`)

The `middleware` package provides production-grade HTTP middlewares compatible with Go standard library `http.Handler` and Chi router chains.

---

## 1. Available Middlewares

### `middleware.RequestID`
* **Signature:** `func(http.Handler) http.Handler`
* **Function:** Extracts incoming `X-Request-ID` from request headers or generates a cryptographically random 16-byte hex token. Injects the ID into the HTTP context and mirrors it to the response header `X-Request-ID`.

### `middleware.Logging(log *slog.Logger)`
* **Signature:** `func(*slog.Logger) func(http.Handler) http.Handler`
* **Function:** Logs each incoming HTTP request using structured JSON. Records `request_id`, HTTP `method`, `path`, `status` code, elapsed `duration`, payload metrics (`bytes_in` request payload size, `bytes_out` response body size), `remote_addr`, and `user_agent`.

### `middleware.Recovery(log *slog.Logger)`
* **Signature:** `func(*slog.Logger) func(http.Handler) http.Handler`
* **Function:** Recovers from unhandled panics inside handlers, logs the panic details with the request path, and safely writes a `500 Internal Server Error` JSON response to the client instead of crashing the process.

### `middleware.CORS(allowedOrigins string)`
* **Signature:** `func(string) func(http.Handler) http.Handler`
* **Function:** Configures Cross-Origin Resource Sharing headers (`Access-Control-Allow-Origin`, `Methods`, `Headers`, `Credentials`), and handles `OPTIONS` preflight requests immediately with `204 No Content`.

### `middleware.RateLimit(rl *RateLimiter)`
* **Signature:** `func(*RateLimiter) func(http.Handler) http.Handler`
* **Function:** In-memory token bucket rate limiter per client IP. Inspects `X-Forwarded-For` and `X-Real-IP` behind reverse proxies (e.g. Caddy/Load Balancer). Injects `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and when quota is exhausted, injects `Retry-After` and returns a `429 Too Many Requests` JSON error. Includes background janitor to purge stale inactive client records.

### `middleware.DistributedRateLimit(lim ratelimit.Limiter, keyExtractor ...func(r *http.Request) string)`
* **Signature:** `func(ratelimit.Limiter, ...func(*http.Request) string) func(http.Handler) http.Handler`
* **Function:** Distributed rate limiter backed by Redis Cluster or Memory (`pkg/ratelimit`). Evaluates atomic Lua scripts in Redis with Cluster `{...}` hash tags and automatic in-memory fallback. Sets `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and `Retry-After`.

### `middleware.TierRateLimit(tl *ratelimit.TierLimiter, resolver ...func(r *http.Request) (string, ratelimit.Tier))`
* **Signature:** `func(*ratelimit.TierLimiter, ...func(*http.Request) (string, ratelimit.Tier)) func(http.Handler) http.Handler`
* **Function:** Multi-tier distributed customer rate limiter enforcing Free, Pro, Enterprise, and Internal quotas with automatic key and credential resolution (`X-API-Key`, JWT claims, `X-Adroit-Tier`, or client IP). Sets `X-RateLimit-Tier`, `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and `Retry-After`.

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
