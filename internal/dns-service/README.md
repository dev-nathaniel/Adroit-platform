# Adroit Ingress & DNS Service (`dns-service`)

The **Ingress & DNS Service** manages platform edge reverse proxy routing, custom domain registration, DNS challenge verification (CNAME/TXT), on-demand SSL/TLS certificate lifecycle, and dynamic synchronization with **Caddy** via its Admin API.

---

## 1. System Architecture & Ingress Flow

```
   [ Client: api.mycompany.com / app.adroit.run ]
                         │
                         ▼
             [ Caddy Edge Reverse Proxy ]
        (Port 80/443 with on-demand ACME TLS)
                         │
       (Dynamic JSON routes via Admin API :2019)
                         │
                         ▼
        ┌───────────────────────────────────┐
        │      DNS & Ingress Service        │
        │          (Port 8088)              │
        └─────────────────┬─────────────────┘
                          │
      ┌───────────────────┼───────────────────┐
      ▼                   ▼                   ▼
[ PostgreSQL ]     [ DNS Resolver ]   [ NATS JetStream ]
- custom_domains   - net.LookupCNAME  - adroit.deploy.healthy
- ingress_routes   - net.LookupTXT    - adroit.domain.verified
                   - CNAME/TXT engine - adroit.route.updated
```

---

## 2. Key Capabilities

1. **Custom Domain Registration & Verification**:
   - Allows users to bind arbitrary custom domains (`api.mycompany.com`) to platform services.
   - Generates cryptographically secure challenge tokens (`adr_verify_<hex>`).
   - Supports dual verification mechanisms:
     - **CNAME Challenge**: Validates that the custom domain points directly to the platform ingress CNAME (default: `cname.adroit.run`).
     - **TXT Challenge**: Validates that an `_adroit-challenge.<domain>` TXT record contains the service's verification token.
   - Built-in development bypass for `.localhost` and `.test` domains.

2. **Dynamic Edge Reverse Proxy (Caddy Admin API)**:
   - Synchronizes routing rules directly with Caddy's dynamic JSON Admin API (`http://localhost:2019`).
   - Uses Caddy `@id` identifiers for idempotent, zero-downtime additions, updates, and removals of reverse proxy rules.
   - Automatically provisions upstream dial targets pointing to active Nomad allocations or service ports.
   - Includes a thread-safe `SimulatedIngressDriver` for offline development and hermetic unit testing.

3. **Platform Subdomain Ingress (`*.adroit.run`)**:
   - Automatically provisions default routing for `<service_slug>.adroit.run` pointing to active deployments.

4. **Event-Driven Traffic Switching**:
   - Subscribes to `adroit.deploy.healthy`: As soon as a Nomad allocation passes health checks, the DNS service updates upstream proxies for all active domains tied to that service.
   - Subscribes to `adroit.service.deleted`: Cleans up all associated edge routes and database records.

---

## 3. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | int | `8088` | HTTP API listen port |
| `DATABASE_URL` | string | `postgres://adroit:adroit_secret@localhost:5432/adroit_platform?sslmode=disable` | PostgreSQL connection DSN |
| `PLATFORM_DOMAIN` | string | `adroit.run` | Base domain suffix for platform services |
| `CANONICAL_CNAME` | string | `cname.adroit.run` | Target host required for CNAME verification |
| `CADDY_ADMIN_URL` | string | `http://localhost:2019` | URL for Caddy's dynamic Admin API |
| `INGRESS_DRIVER` | string | `auto` | Ingress driver: `auto`, `caddy`, or `simulated` |
| `NATS_URL` | string | `nats://localhost:4222` | NATS JetStream cluster connection URL |
| `JWT_SECRET` | string | `adroit-super-secret-...` | Shared HMAC-SHA256 secret for authentication |

---

## 4. Route Table

| Method | Path | Auth Required | Description |
| :--- | :--- | :---: | :--- |
| `GET` | `/healthz` | No | Health check probe |
| `POST` | `/services/{id}/domains` | Yes | Register custom domain for a service |
| `GET` | `/services/{id}/domains` | Yes | List all domains attached to a service |
| `GET` | `/domains/{id}` | Yes | Get domain details and DNS setup instructions |
| `POST` | `/domains/{id}/verify` | Yes | Trigger immediate DNS challenge check |
| `POST` | `/domains/{id}/sync` | Yes | Re-synchronize Caddy edge proxy rule |
| `DELETE` | `/domains/{id}` | Yes | Delete domain and deprovision Caddy route |
| `POST` | `/services/{id}/sync-routes` | Yes | Force synchronize all routes for a service |
| `GET` | `/ingress/routes` | Yes | List all active edge ingress routes across services |

---

## 5. Testing

Run unit and integration tests:
```bash
go test -v ./internal/dns-service/...
go test -v ./pkg/ingress/...
```
