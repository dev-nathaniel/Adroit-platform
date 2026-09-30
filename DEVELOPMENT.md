# Adroit Platform — Development & Implementation Tracker

> **Purpose:** Living documentation tracking all architectural implementations, decisions, package layouts, and progress so development remains seamless and transparent.

---

## 1. Directory Structure Standards

```
Adroit-platform/
├── ARCHITECTURE.md                  # Comprehensive architectural specification
├── DEVELOPMENT.md                   # This tracker: progress, conventions, decisions
├── .gitignore                       # Standard Go, OS, and local env exclusions
├── .env.example                     # Environment template for local and container setups
├── go.mod                           # Go module definition
├── go.sum                           # Go module checksums
│
├── cmd/                             # Service main entry points
│   ├── api-gateway/                 # Central API Gateway entrypoint
│   ├── auth-service/                # Auth Service entrypoint
│   ├── billing-service/             # Billing & Metering Service entrypoint
│   ├── build-service/               # Build Service entrypoint
│   ├── dns-service/                 # DNS & Custom Domain Service entrypoint
│   ├── docker-registry-service/     # Docker Registry Service entrypoint
│   ├── git-integration-service/     # Git Integration Service entrypoint
│   ├── log-service/                 # Logging & Streaming Service entrypoint
│   ├── managed-database-service/    # Managed Database Service entrypoint
│   ├── migrate/                     # Standalone Production Database Migration CLI
│   ├── notification-service/        # Notification & Webhook Service entrypoint
│   ├── orchestration-service/       # Nomad Orchestration Service entrypoint
│   ├── project-service/             # Project & Environment Service entrypoint
│   ├── runtime-service/             # Runtime & Scaling Service entrypoint
│   └── user-service/                # User & Organization Service entrypoint
│
├── internal/                        # Private service business logic & handlers
│   ├── api-gateway/                 # Reverse proxy, auth middleware, route dispatching
│   ├── auth-service/                # OAuth, JWT, session verification
│   ├── billing-service/             # Stripe, usage tracking, quota enforcement
│   ├── build-service/               # Build pipeline triggers, BuildKit Nomad job specs
│   ├── dns-service/                 # DNS challenge validation, Caddy route management
│   ├── docker-registry-service/     # Harbor API integration & credentials
│   ├── git-integration-service/     # GitHub/GitLab webhook processing & repository access
│   ├── log-service/                 # Log collection & WebSocket/SSE log streamers
│   ├── managed-database-service/    # Postgres/Redis database provisioning logic
│   ├── notification-service/        # Webhook, email, and alert dispatchers
│   ├── orchestration-service/       # Nomad client, job lifecycle, rolling updates
│   ├── project-service/             # Project, environment variables, services config
│   ├── runtime-service/             # Instance monitoring, scaling rules, health status
│   └── user-service/                # Account profiles, teams, organizations
│
├── pkg/                             # Shared, reusable platform libraries
│   ├── config/                      # Environment configuration loading & validation
│   ├── logger/                      # Structured JSON logger (built on log/slog)
│   ├── response/                    # Standardized HTTP JSON envelope & error handling
│   ├── middleware/                  # HTTP middlewares (Request ID, Logging, CORS, Recovery)
│   ├── events/                      # NATS JetStream event definitions & payload envelopes
│   └── types/                       # Shared domain models & enumerations
│
├── docs/                            # API Specifications & Developer Tooling
│   ├── openapi.yaml                 # OpenAPI 3.0.3 specification
│   ├── adroit-platform.postman_collection.json # Postman Collection v2.1.0 with auto-token chaining
│   └── README.md                    # Usage guide for Swagger UI and Postman
│
└── deployments/                     # Infrastructure as Code & Orchestration
    ├── docker-compose.dev.yml       # Local development stack (Postgres, Redis, NATS, Consul)
    └── nomad/                       # Nomad HCL job definitions
```

---

## 2. Platform Conventions & Standards

1. **Structured Logging:**
   - Always use `pkg/logger` (based on Go's `log/slog`) with structured key-value attributes.
   - Every request has a unique `request_id` propagated across logs and HTTP response headers (`X-Request-ID`).

2. **Unified API Response Envelope:**
   - Success:
     ```json
     {
       "success": true,
       "data": { ... },
       "meta": { "page": 1, "per_page": 20, "total": 100 }
     }
     ```
   - Failure:
     ```json
     {
       "success": false,
       "error": {
         "code": "RESOURCE_NOT_FOUND",
         "message": "Project with id proj_123 was not found",
         "details": null
       }
     }
     ```

3. **Event-Driven Messaging (NATS JetStream):**
   - Stream Name: `ADROIT_EVENTS`
   - Subject Hierarchy:
     - `adroit.project.> ` (e.g. `adroit.project.created`, `adroit.project.deleted`)
     - `adroit.build.> ` (e.g. `adroit.build.requested`, `adroit.build.started`, `adroit.build.completed`, `adroit.build.failed`)
     - `adroit.deploy.> ` (e.g. `adroit.deploy.scheduled`, `adroit.deploy.running`, `adroit.deploy.healthy`, `adroit.deploy.failed`)
     - `adroit.database.> ` (e.g. `adroit.database.provisioned`, `adroit.database.deprovisioned`)

4. **12-Factor Configuration:**
   - All services read configurations from environment variables with sensible defaults for local development.

5. **Testing Standards:**
   - Unit tests accompany all shared packages and service handlers.
   - Run tests with: `go test -v ./...`
   - Run tests with race detection: `go test -race ./...`
   - Run tests with coverage breakdown: `go test -cover ./...`

6. **Module Documentation (Mandatory READMEs):**
   - Every service (`cmd/<service>`, `internal/<service>`) and shared module (`pkg/<module>`) must include its own `README.md`.
   - Each `README.md` must document:
     - Purpose and responsibilities of the module.
     - Configuration and environment variables.
     - How to run locally.
     - How to execute unit tests.
     - Full route table (HTTP methods, paths, auth requirements, payloads).
     - Core exported functions and API references.

7. **Rate Limiting Architecture & Evolution:**
   - **Why not `chi.Throttle`:** Chi's built-in throttle only caps global concurrent in-flight requests server-wide; it does not track requests per second, time windows, or client IPs.
   - **Current Pattern (In-Memory Token Bucket):** Implemented in `pkg/middleware/ratelimit.go` using a thread-safe token bucket algorithm with per-IP tracking and background cleanup. Optimal for local development and single-instance deployments (zero external latency, zero dependencies).
   - **[TODO] Multi-Node Distributed Limiter (Redis Cluster):** In multi-node production deployments where multiple API Gateway replicas run behind the Edge Load Balancer, rate limits will be synchronized globally across nodes via the **Redis Cluster** (already provisioned in `deployments/docker-compose.dev.yml`). This also enables customer-tier rate quotas (e.g. Free: 100 req/min, Pro: 5,000 req/min).

8. **Database Migrations Architecture:**
   - **Engine:** Powered by `golang-migrate` with embedded SQL filesystem (`deployments/migrations/migrations.go`) and PostgreSQL advisory locking.
   - **Local Development:** `cmd/adroit-dev` automatically checks PostgreSQL and applies pending migrations on boot. Direct inspection and rollbacks are available via `go run ./cmd/migrate [up|down|version|force]`.
   - **Production:** Packaged into a minimal container via `deployments/Dockerfile.migrate` and executed as an isolated pre-deployment batch job in Nomad (`deployments/nomad/adroit-migrate.nomad.hcl`) before rolling out microservice updates.

---

## 3. Implementation Progress Tracker

| Milestone | Component | Status | Details |
| :--- | :--- | :--- | :--- |
| **Phase 1: Foundation** | Project Architecture Docs | Completed | Master `ARCHITECTURE.md` specification created. |
| | Progress & Decisions Tracker | Completed | Living `DEVELOPMENT.md` tracking all implementations. |
| | Core Directory Layout | Completed | Standard `cmd/`, `internal/`, `pkg/`, `deployments/` scaffolded for all 14 services. |
| | Shared `pkg/logger` | Completed | Zero-dependency structured JSON/text logging via `log/slog` with request ID tracing. |
| | Shared `pkg/response` | Completed | Standardized API JSON response envelope, error codes, and pagination metadata. |
| | Shared `pkg/middleware` | Completed | RequestID, Slog, Panic Recovery, CORS, RequireAuth, and Token Bucket Rate Limiter. |
| | Shared `pkg/types` | Completed | Universal domain models (Project, Service, Build, Deployment, Database, User). |
| | Shared `pkg/events` | Completed | NATS JetStream typed event envelopes and domain subject definitions. |
| | Shared `pkg/config` | Completed | 12-factor configuration loader with typed getters and fallback defaults. |
| | Local Backing Services | Completed | `docker-compose.dev.yml` for Postgres, Redis, NATS (JetStream), Consul, Caddy. |
| | API Gateway Entrypoint | Completed | `cmd/api-gateway/main.go` implemented and verified compiling with Chi router. |
| **Phase 2: Core Microservices** | PostgreSQL Schema & Migrations | Completed | Migration DDL (`001_init_schema.{up,down}.sql`), embedded FS, standalone `cmd/migrate` runner (golang-migrate), Nomad batch job, and auto-preflight in `adroit-dev`. |
| | Identity & Crypto Primitives | Completed | `pkg/identity` for bcrypt password hashing (cost 12) and HMAC-SHA256 JWT tokens. |
| | Auth Service | Completed | `internal/auth-service` & `cmd/auth-service`: Register, Login, Me, auto-workspace creation, rate limiting. |
| | User & Organization Service | Completed | `internal/user-service` & `cmd/user-service`: User profiles, multi-tenant orgs, team memberships & RBAC. |
| | Notification Service & Event Backbone | Completed | `internal/notification-service`, `cmd/notification-service`, `pkg/events`: NATS JetStream & InMem EventBus, invite email rendering, SMTP & Console providers, signed webhooks with HMAC-SHA256 & backoff. |
| | API Specifications & Postman | Completed | OpenAPI 3.0.3 (`docs/openapi.yaml`) & Postman Collection v2.1.0 (`docs/adroit-platform.postman_collection.json`) with auto-token chaining. |
| | All-in-One Dev Runner | Completed | `cmd/adroit-dev`: One-command runner launching Gateway, Auth, User, Notification, and Project services concurrently. |
| | Project & Service Catalog | Completed | `internal/project-service` & `cmd/project-service`: Projects, standalone services (`project_id=nil`), moving services into/out of projects (`/services/{id}/move`), hierarchical environment variables, and NATS JetStream event publishing. |
| | Git Integration & Webhooks | Completed | `internal/git-integration-service` & `cmd/git-integration-service`: GitHub & GitLab push/ping webhook ingress, HMAC-SHA256 & token verification, canonical URL normalization (HTTPS, SSH, git://), branch matching, automated build record creation, NATS JetStream event dispatching (`adroit.build.requested`), provider credential linking, and repository/branch discovery. |
| | Build Service & Worker | Completed | `internal/build-service`, `cmd/build-service`, and `pkg/builder`: Pluggable builder engine (Local/Dev BuildKit simulator & Nomad batch runner), build lifecycle API (`POST/GET /services/{id}/builds`, `GET /builds/{id}`, `POST /builds/{id}/cancel`, `GET /builds/{id}/logs`), NATS JetStream integration (`adroit.build.requested` subscriber, emitting `adroit.build.started`, `adroit.build.completed`, `adroit.build.failed`, and orchestration handoff `adroit.deploy.requested`), migration 006 (`build_logs` & `error_message`), and API Gateway / dev runner supervisor integration on port 8086. |
| | Orchestration & Runtime | Up Next | Nomad driver, allocations, rollout strategies, health monitoring. |
| | Ingress & Caddy Routing | Pending | Dynamic edge reverse proxy routing via Consul. |
| | Distributed Rate Limiting | **TODO** | Upgrade rate limiting to Redis Cluster backend for multi-node gateway sync. |
