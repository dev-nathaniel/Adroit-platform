# Ingress & Edge Proxy Driver Package (`pkg/ingress`)

This package provides pluggable reverse proxy driver interfaces, a live **Caddy Admin API** HTTP client, and an in-memory **Simulated Driver** for managing dynamic ingress routes in the Adroit PaaS platform.

## Key Types & Interfaces

- `Driver`: Universal interface (`UpsertRoute`, `DeleteRoute`, `GetRoute`, `ListRoutes`, `CheckHealth`).
- `CaddyClient`: Live JSON Admin API client utilizing Caddy `@id` identifiers for idempotent, zero-downtime updates.
- `SimulatedIngressDriver`: In-memory thread-safe mock driver for unit testing and offline development.
- `GenerateRouteID`: Normalizes host patterns into canonical route identifiers.

## Usage Example

```go
driver := ingress.NewCaddyClient("http://localhost:2019", "adroit_edge")

err := driver.UpsertRoute(ctx, ingress.RouteSpec{
    ID:          ingress.GenerateRouteID("api.customer.com"),
    HostPattern: "api.customer.com",
    Upstreams:   []ingress.UpstreamTarget{{Dial: "127.0.0.1:8080"}},
    EnableTLS:   true,
})
```
