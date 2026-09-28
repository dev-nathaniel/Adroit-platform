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

### Rate Limiting Design & Roadmap
* **Current Implementation:** In-memory Token Bucket (`pkg/middleware/ratelimit.go`). Provides zero-latency, zero-dependency per-IP throttling for single-instance / local development.
* **Why not `chi.Throttle`:** Chi's built-in throttle only caps global concurrent in-flight requests server-wide; it does not track requests per second, time windows, or client IPs.
* **[TODO] Redis Cluster Distributed Limiting:** When scaling the API Gateway to multiple nodes behind the Edge Load Balancer, the rate limiter will transition to the Redis Cluster to synchronize rate buckets across all nodes and support tier-based quotas (e.g. Free vs Pro tier).

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | int | `8080` | Port for the HTTP server to listen on. |
| `ADROIT_ENV` | string | `development` | Deployment environment (`development`, `staging`, `production`). |
| `ADROIT_LOG_LEVEL` | string | `debug` | Logging verbosity (`debug`, `info`, `warn`, `error`). |
| `ADROIT_LOG_FORMAT` | string | `json` | Log format (`json` or `text`). |
| `RATE_LIMIT_RPS` | int | `50` | Allowed sustained requests per second per client IP. |
| `RATE_LIMIT_BURST` | int | `100` | Maximum burst capacity per client IP. |

---

## 3. How to Run Locally

### Start Server
```bash
go run ./cmd/api-gateway/main.go
```
The server will start listening on `http://0.0.0.0:8080`.

---

## 4. How to Test

### Run Unit Tests
```bash
go test -v ./cmd/api-gateway
```

### Run with Coverage
```bash
go test -cover ./cmd/api-gateway
```

---

## 5. Routes & Endpoints

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `GET` | `/healthz` | Platform health check and timestamp | No |
| `GET` | `/` | Platform gateway info and router status | No |
| `GET` | `/api/v1/status` | API v1 subsystem status | No |
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
