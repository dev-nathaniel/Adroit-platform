# Rate Limiting Package (`pkg/ratelimit`)

The `pkg/ratelimit` package provides high-performance, distributed, and multi-tier rate limiting for the Adroit Platform. It supports in-memory execution for single-instance or local dev workflows, as well as Redis and Redis Cluster backends for synchronized distributed rate limiting across multi-node API Gateway clusters.

---

## 1. Architectural Highlights

- **Multi-Backend Architecture:**
  - **Redis / Redis Cluster (`BackendRedis`):** Atomic distributed rate limiting via Redis Lua scripts with Redis Cluster hash tag routing (`{...}` slots), automatic node discovery, Sentinel support, and sub-millisecond execution.
  - **In-Memory (`BackendMemory`):** Zero-latency, zero-dependency token bucket and sliding window rate limiter with background janitor cleanup routines.
- **Failover & Graceful Degradation (`FallbackToMemory`):**
  - If Redis becomes unreachable or suffers a network partition, the limiter automatically falls back to an in-memory token bucket and throttles warning logs.
  - When Redis connectivity restores, it transparently resumes distributed cluster-wide synchronization without dropping incoming requests.
- **Dual Algorithms:**
  - **Token Bucket (`AlgorithmTokenBucket`):** Refills tokens over time, accommodating bursts up to configured capacity while enforcing long-term rate limits.
  - **Sliding Window (`AlgorithmSlidingWindow`):** Rolling window log using Redis Sorted Sets (`ZSET`), eliminating border spikes across window boundaries.
- **Customer Subscription Tier Quotas:**
  - `TierFree`: 100 req/min (rate: 1.67 rps, burst: 30)
  - `TierPro`: 5,000 req/min (rate: 83.33 rps, burst: 250)
  - `TierEnterprise`: 50,000 req/min (rate: 833.33 rps, burst: 2,000)
  - `TierInternal`: 100,000 req/min (rate: 1,666.67 rps, burst: 5,000)
  - Pluggable tier resolvers extracting credentials from JWT claims, API keys (`X-API-Key`), custom headers (`X-Adroit-Tier`), or client IPs.
- **Redis Cluster Key Sharding:**
  - Uses `{<prefix>:<client_key>}` hash tags so all hash keys map to identical Redis cluster slots, strictly avoiding `CROSSSLOT Keys in request don't hash to the same slot` errors.
- **Standard HTTP Headers:**
  - Injects `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `X-RateLimit-Tier`, and standard `Retry-After` on HTTP `429 Too Many Requests`.

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `RATE_LIMIT_BACKEND` | `string` | `redis` | Storage backend: `redis` or `memory`. |
| `RATE_LIMIT_ALGORITHM` | `string` | `token_bucket` | Algorithm: `token_bucket` or `sliding_window`. |
| `RATE_LIMIT_RPS` | `float64` | `50.0` | Default token replenishment rate per second. |
| `RATE_LIMIT_BURST` | `int` | `100` | Maximum token burst capacity. |
| `RATE_LIMIT_TIERS_ENABLED` | `bool` | `true` | Enable customer subscription tier quotas. |
| `REDIS_ADDR` | `string` | `localhost:6379` | Single Redis host address. |
| `REDIS_ADDRS` | `string` | `localhost:6379` | Comma-separated list of Redis or Redis Cluster nodes. |
| `REDIS_PASSWORD` | `string` | `""` | Redis authentication password. |
| `REDIS_DB` | `int` | `0` | Redis logical database index. |

---

## 3. Core Exported API

### Types & Interfaces

```go
type Limiter interface {
    Allow(ctx context.Context, key string) (*Result, error)
    AllowN(ctx context.Context, key string, n int64) (*Result, error)
    Reset(ctx context.Context, key string) error
    Close() error
}

type Result struct {
    Allowed    bool
    Limit      int64
    Remaining  int64
    ResetAfter time.Duration
    RetryAfter time.Duration
}
```

### Initializing a Limiter

```go
lim, err := ratelimit.New(ratelimit.Config{
    Backend:          ratelimit.BackendRedis,
    Algorithm:        ratelimit.AlgorithmTokenBucket,
    Rate:             100.0,
    Burst:            200,
    RedisAddrs:       []string{"localhost:6379"},
    FallbackToMemory: true,
})
if err != nil {
    log.Fatal(err)
}
defer lim.Close()

res, err := lim.Allow(ctx, "client-ip-or-id")
if !res.Allowed {
    // Rejected: res.RetryAfter
}
```

### Initializing Multi-Tier Limiting

```go
tierLimiter, err := ratelimit.NewTierLimiter(ratelimit.Config{
    Backend:    ratelimit.BackendRedis,
    RedisAddrs: []string{"localhost:6379"},
}, nil)
defer tierLimiter.Close()

res, err := tierLimiter.Allow(ctx, ratelimit.TierPro, "tenant-123")
```

---

## 4. HTTP Middleware Usage

In `pkg/middleware`:

```go
// 1. Single Distributed Limiter
r.Use(middleware.DistributedRateLimit(limiter))

// 2. Multi-Tier Distributed Limiter
r.Use(middleware.TierRateLimit(tierLimiter))
```

---

## 5. How to Run Unit Tests

Execute package unit tests:

```bash
go test -v ./pkg/ratelimit/...
```

Execute tests with coverage:

```bash
go test -cover ./pkg/ratelimit/...
```
