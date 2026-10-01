# API Gateway Service (`cmd/api-gateway`)

The **API Gateway** is the central entrypoint for the Adroit Platform. It handles all inbound management and client traffic from the Web Dashboard, Mobile App, and CLI, applying global cross-cutting concerns (CORS, Request Tracing, Recovery, Structured Logging) and routing traffic to downstream microservices using the `go-chi/chi/v5` router.

---

## 1. Responsibilities

* **Unified Ingress:** Accepts incoming HTTP requests and dispatches them to internal microservices.
* **Routing Engine:** High-performance, idiomatic routing via `go-chi/chi/v5`.
* **Rate Limiting:** Token-bucket rate limiting per client IP with `X-Forwarded-For` support and automatic 429 responses.
* **Request Tracing:** Assigns or propagates `X-Request-ID` across HTTP requests and logs.
* **Resilience:** Catches unhandled panics via recovery middleware to prevent process crashes.
* **CORS:** Cross-Origin Resource Sharing for dashboard and mobile interfaces.
* **Graceful Shutdown:** Intercepts `SIGINT`/`SIGTERM` and drains active connections before termination.

### Rate Limiting Design & Distributed Redis Sync
* **Distributed Synchronization:** Powered by `pkg/ratelimit` and `pkg/middleware`, connecting to Redis or Redis Cluster with automatic node routing (`{...}` hash tags).
* **Graceful Degradation:** If Redis becomes unreachable, the gateway automatically falls back to an in-memory token bucket, preventing cascading failures or blocked traffic.
* **Customer Tier Quotas:** Enforces subscription tiers (`Free`: 100 req/min, `Pro`: 5,000 req/min, `Enterprise`: 50,000 req/min) with dynamic key extraction from API keys (`X-API-Key`), JWT credentials, or client IPs.
* **Standard Headers:** Sets `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `X-RateLimit-Tier`, and standard `Retry-After` on HTTP `429 Too Many Requests`.

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | int | `8080` | Port for the HTTP server to listen on. |
| `ADROIT_ENV` | string | `development` | Deployment environment (`development`, `staging`, `production`). |
| `ADROIT_LOG_LEVEL` | string | `debug` | Logging verbosity (`debug`, `info`, `warn`, `error`). |
| `ADROIT_LOG_FORMAT` | string | `json` | Log format (`json` or `text`). |
| `RATE_LIMIT_BACKEND` | string | `redis` | Storage backend: `redis` or `memory`. |
| `RATE_LIMIT_ALGORITHM` | string | `token_bucket` | Algorithm: `token_bucket` or `sliding_window`. |
| `RATE_LIMIT_RPS` | int | `50` | Allowed sustained requests per second per client IP. |
| `RATE_LIMIT_BURST` | int | `100` | Maximum burst capacity per client IP. |
| `RATE_LIMIT_TIERS_ENABLED` | bool | `true` | Enable customer subscription tier quotas. |
| `REDIS_ADDR` | string | `localhost:6379` | Single Redis host address. |
| `REDIS_ADDRS` | string | `localhost:6379` | Comma-separated list of Redis or Redis Cluster nodes. |
| `REDIS_PASSWORD` | string | `""` | Redis authentication password. |
| `REDIS_DB` | int | `0` | Redis logical database index. |
| `AUTH_SERVICE_URL` | string | `http://localhost:8081` | Downstream Auth Service URL. |
| `USER_SERVICE_URL` | string | `http://localhost:8082` | Downstream User Service URL. |

---

## 3. How to Run Locally

### Option A: Build and Run (Recommended for Windows)
Avoids Windows Application Control / AppLocker blocks on `AppData\Local\Temp`:
```powershell
go build -o bin/api-gateway.exe ./cmd/api-gateway
./bin/api-gateway.exe
```

### Option B: Direct `go run`
If running directly, redirect `GOTMPDIR` out of `AppData\Local\Temp`:
```powershell
$env:GOTMPDIR = "$PWD\.gotmp"
go run ./cmd/api-gateway/main.go
```
The server will start listening on `http://0.0.0.0:8080`.

---

## 4. How to Test

### Run Unit Tests
```bash
go test -v ./cmd/api-gateway ./internal/api-gateway
```

### Run with Coverage
```bash
go test -cover ./cmd/api-gateway ./internal/api-gateway
```

---

## 5. Routes & Endpoints

> [!TIP]
> Complete API specifications are available in **OpenAPI 3.0** format at [`docs/openapi.yaml`](../../docs/openapi.yaml) and as an importable **Postman Collection v2.1.0** at [`docs/adroit-platform.postman_collection.json`](../../docs/adroit-platform.postman_collection.json). See [`docs/README.md`](../../docs/README.md) for details.

| Method | Endpoint | Routing / Target | Auth Required |
| :--- | :--- | :--- | :--- |
| `GET` | `/healthz` | Platform health check and timestamp | No |
| `GET` | `/` | Platform gateway info and router status | No |
| `GET` | `/api/v1/status` | API v1 subsystem status | No |
| `*` | `/api/v1/auth/*` | **Reverse Proxied** to Auth Service (`:8081`) | By endpoint |
| `*` | `/api/v1/users/*` | **Reverse Proxied** to User Service (`:8082`) | By endpoint |
| `*` | `/*` | Custom 404 handler returning standard error envelope | N/A |

### Example cURL Commands

#### 1. Check Health (`GET /healthz`)
```bash
curl -i http://localhost:8080/healthz
```
**Response:**
```json
{
  "success": true,
  "data": {
    "platform": "Adroit PaaS",
    "service": "api-gateway",
    "status": "healthy",
    "timestamp": "2026-09-28T17:20:00Z"
  }
}
```

#### 2. Root Gateway Info (`GET /`)
```bash
curl -i http://localhost:8080/
```
**Response:**
```json
{
  "success": true,
  "data": {
    "platform": "Adroit PaaS",
    "router": "chi/v5",
    "status": "operational",
    "version": "v1.0.0"
  }
}
```

#### 3. Route Not Found (`404`)
```bash
curl -i http://localhost:8080/non-existent
```
**Response:**
```json
{
  "success": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "Route GET /non-existent not found"
  }
}
```
*(Notice the `X-Request-ID` header in the HTTP response headers).*

#### 4. Rate Limit Exceeded (`429`)
```bash
curl -i http://localhost:8080/healthz
```
**Response Headers:**
```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json; charset=utf-8
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
Retry-After: 2
X-Request-ID: d58309a1bf04419999011be17fbfeb67
```
**Response Body:**
```json
{
  "success": false,
  "error": {
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded. Please wait before making more requests.",
    "details": {
      "limit": 100,
      "retry_after_seconds": 2
    }
  }
}
```
