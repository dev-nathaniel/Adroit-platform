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


---

## 3. Implementation Progress Tracker

| Milestone | Component | Status | Details |
| :--- | :--- | :--- | :--- |
| **Phase 1: Foundation** | Project Architecture Docs | Completed | Master `ARCHITECTURE.md` specification created. |
| | Progress & Decisions Tracker | Completed | Living `DEVELOPMENT.md` tracking all implementations. |
| | Core Directory Layout | Completed | Standard `cmd/`, `internal/`, `pkg/`, `deployments/` scaffolded for all 14 services. |
| | Shared `pkg/logger` | Completed | Zero-dependency structured JSON/text logging via `log/slog` with request ID tracing. |
| | Shared `pkg/response` | Completed | Standardized API JSON response envelope, error codes, and pagination metadata. |
| | Shared `pkg/middleware` | Completed | RequestID injection, Slog logging, Panic Recovery, and CORS middleware. |
| | Shared `pkg/types` | Completed | Universal domain models (Project, Service, Build, Deployment, Database, User). |
| | Shared `pkg/events` | Completed | NATS JetStream typed event envelopes and domain subject definitions. |
| | Shared `pkg/config` | Completed | 12-factor configuration loader with typed getters and fallback defaults. |
| | Local Backing Services | Completed | `docker-compose.dev.yml` for Postgres, Redis, NATS (JetStream), Consul, Caddy. |
| | API Gateway Entrypoint | Completed | `cmd/api-gateway/main.go` implemented and verified compiling. |
| **Phase 2: Core Microservices** | User & Auth Service | Up Next | Next priority. |
| | Project & Service Catalog | Pending | Project/service configuration and env vars. |
| | Build Service & Worker | Pending | BuildKit integration and Nomad batch job runner. |
| | Orchestration & Runtime | Pending | Nomad driver, allocations, health monitoring. |
| | Ingress & Caddy Routing | Pending | Dynamic edge reverse proxy routing via Consul. |
