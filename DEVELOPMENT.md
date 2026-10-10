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
   - **Multi-Backend Architecture (`pkg/ratelimit`):** Provides a unified `Limiter` and `TierLimiter` interface with dual algorithms: Token Bucket and Sliding Window (ZSET).
   - **Multi-Node Distributed Limiter (Redis Cluster):** Synchronizes rate limits globally across all API Gateway replicas via Redis / Redis Cluster using atomic Lua scripts. Keys are partitioned using Redis Cluster `{...}` hash tags to guarantee safe slot routing and avoid cross-slot errors.
   - **Resilience & Automatic Fallback:** If Redis is unreachable or partitioned, the limiter automatically falls back to an in-memory token bucket and rate-limits warning logs, ensuring API availability is never compromised.
   - **Customer Tier Quotas:** Enforces subscription tiers (`Free`: 100 req/min, `Pro`: 5,000 req/min, `Enterprise`: 50,000 req/min, `Internal`: 100,000 req/min) with credential resolvers for API keys (`X-API-Key`), JWT claims, `X-Adroit-Tier`, or client IP.
   - **HTTP Headers:** Standard RFC 6585 compliance with `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `X-RateLimit-Tier`, and `Retry-After`.

8. **Database Migrations Architecture:**
   - **Engine:** Powered by `golang-migrate` with embedded SQL filesystem (`deployments/migrations/migrations.go`) and PostgreSQL advisory locking.
   - **Local Development:** `cmd/adroit-dev` automatically checks PostgreSQL and applies pending migrations on boot. Direct inspection and rollbacks are available via `go run ./cmd/migrate [up|down|version|force]`.
   - **Production:** Packaged into a minimal container via `deployments/Dockerfile.migrate` and executed as an isolated pre-deployment batch job in Nomad (`deployments/nomad/adroit-migrate.nomad.hcl`) before rolling out microservice updates.

9. **Multi-Source Workloads & Archive Storage Architecture (`pkg/storage`, `pkg/builder`):**
   - **3 Ingestion Paradigms:** GitOps (`source_type = "git"`), CLI / Archive uploads (`source_type = "archive"`), and Pre-built container images (`source_type = "image"`).
   - **Pluggable Object Storage (`pkg/storage`):** `StorageProvider` abstraction supporting `LocalStorage`, `InMemStorage`, and `S3Storage` (AWS S3, MinIO, Cloudflare R2).
   - **Safe Archive Normalization:** `NormalizeToTarGz` inspects `.zip`, `.tar.gz`, or `.tar`, validates Zip Slip directory traversal defenses, calculates SHA-256 integrity checksums, and produces deterministic `.tar.gz` bundles consumable by BuildKit via `--opt context=<url>`.
   - **Direct Container Image Bypass:** Workloads configured with `source_type = "image"` bypass the `build-service` entirely, directly translating `image_url` into Nomad job specifications via the Orchestration Service.
   - **Database Migration 012:** `012_add_service_source_type_and_archives.{up,down}.sql` adds `source_type`, `archive_url`, `image_url`, `archive_size_bytes`, and `archive_checksum`, and relaxes the `repo_url` NOT NULL constraint so non-Git workloads can be created seamlessly.

10. **Zero-Plaintext Secrets Architecture (HashiCorp Vault KV v2):**
    - **Defense-in-Depth Isolation:** Variables flagged with `is_secret = true` are stored exclusively in HashiCorp Vault KV v2 at `adroit/{orgID}/services/{serviceID}` and `adroit/{orgID}/projects/{projectID}`.
    - **Database Sentinel:** The PostgreSQL table `environment_variables` stores a non-sensitive sentinel: `value = "[vault:secret]"`. Plaintext secrets never appear in database backups, query logs, or replication streams.
    - **Variable Scoping (`scope`):** `runtime` (default for production secrets), `build` (compilation tokens like `NPM_TOKEN`), and `all`. BuildKit strictly queries `scope IN ('build', 'all')` so production credentials (`DATABASE_PASSWORD`, payment API keys) are **never passed into Docker build args or baked into container image layers**.
    - **Immutable Deployment Snapshots:** Every rollout captures an immutable snapshot of environment variables and active secrets in Vault under `adroit/{orgID}/deployments/{deploymentID}` and PostgreSQL table `deployment_env_snapshots`. Rollbacks restore the exact historical release state.
    - **Nomad In-Memory Mounting:** Secrets are **never** passed into Nomad job specifications via `env {}` stanzas (which would leak plaintext in Nomad Raft consensus logs, `nomad job inspect`, and the UI). Instead, Nomad native `template` stanzas render secrets into an in-memory `tmpfs` RAM filesystem (`secrets/app.env`, `env = true`).
    - **Audited Secret Reveals:** All standard GET APIs mask secret values as `••••••••`. Unmasked access requires dedicated, audit-logged endpoints: `POST /api/v1/projects/:id/env/:key/reveal` and `POST /api/v1/services/:id/env/:key/reveal`.
    - **Local Dev Resiliency:** Features an in-memory `SimulatedSecretStore` fallback when HashiCorp Vault is not reachable during local development or unit testing.

11. **Zero-Configuration Builds with Nixpacks & Cloud Native Buildpacks (`pkg/builder`):**
    - **Automatic Zero-Dockerfile Compilation:** When a service omits `dockerfile_path` or has no Dockerfile in its repository/archive, Adroit automatically invokes the Nixpacks engine (`ghcr.io/railwayapp/nixpacks:latest`).
    - **Ecosystem Detection:** Automatically analyzes code manifests (`package.json`, `requirements.txt`, `go.mod`, `Gemfile`, etc.) to detect Node.js, Python, Go, Ruby, Rust, PHP, etc., and configures hermetic build plans.
    - **Distributed Nomad Batch Execution:** `pkg/builder/nomad_batch_builder.go` dynamically compiles a Nomad batch job (`task "buildkit"` using `ghcr.io/railwayapp/nixpacks:latest`), injecting source context (`RepoURL` or `ArchiveURL`), custom build commands, and target image tags.
    - **Local Development Emulation:** `pkg/builder/local_dev_builder.go` emulates detection, planning, building, and exporting phases with real-time SSE log streaming.

12. **HashiCorp Nomad Workload Scheduling & Sandboxing (`pkg/orchestrator`):**
    - **Dynamic HCL2 Generation:** Compiles service definitions into authentic Nomad job specs (`adroit-svc-<service_id>`) with resource stanzas (`cpu`, `memory`), port allocations, and health checks.
    - **Zero-Trust Service Mesh & Ingress:** Automatically injects Consul Connect sidecars (`proxy {}` stanza) for mutual TLS service-to-service communication and Traefik ingress tags (`traefik.enable=true`, `traefik.http.routers...`).
    - **Zero-Downtime Rollout Lifecycles:** Supports Rolling updates with `min_healthy_time`, Canary rollouts (`canary = 1` with automated or manual promotion via `/deployments/{id}/promote` and auto-reversion on failure), and Blue-Green routing.
    - **Sandboxed Virtualization Tiers:** Configures standard OCI Docker containers (`RuntimeTier: general`) or gVisor system-call virtualization (`RuntimeTier: isolated` via `runsc`).
    - **Direct Image Scheduling:** For `source_type: "image"` workloads, Nomad jobs are compiled and scheduled directly from `image_url` without a prior build step.
    - **Pluggable Cluster Drivers:** `NomadHTTPClient` for live Nomad cluster communication and `SimulatedDriver` for zero-dependency local dev and automated CI testing.

13. **Elastic Workload Auto-Scaling & Metrics Pipeline (`internal/runtime-service`, `pkg/orchestrator`):**
    - **Dual Auto-Scaling Paradigms:** Supports both Nomad-native daemon scaling (`nomad-autoscaler` evaluating `scaling { ... }` stanzas) and platform-managed autoscaling via the `runtime-service` control plane.
    - **Reconciliation Engine:** Periodic evaluation loop calculates desired replica counts using horizontal scaling math: $\lceil \text{Current} \times \frac{\text{MetricValue}}{\text{TargetValue}} \rceil$ bounded by $[\text{min\_replicas}, \text{max\_replicas}]$.
    - **Flapping Suppression:** Features deadband tolerance and separate scale-up cooldowns (default 30s) and scale-down cooldowns (default 180s) to prevent resource churn.
    - **Observability Pipeline:** Integrated Prometheus server scrapes Nomad cluster metrics (`/v1/metrics?format=prometheus`) and Caddy edge metrics. Telemetry is saved into `service_metrics_samples` for dashboard charts.
    - **Auditing & Event Dispatch:** Records every scale event in `service_scaling_events` and broadcasts `adroit.service.scaled` via NATS JetStream.

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
| | Nixpacks Zero-Config Engine | Completed | `pkg/builder`, `internal/build-service`: Zero-configuration polyglot application builds via `ghcr.io/railwayapp/nixpacks:latest`, automatic ecosystem detection (Node.js, Python, Go, Ruby, etc.) when `dockerfile_path` is absent, multi-stage build planning, Nomad transient batch job execution, and local dev step emulation. |
| | Orchestration & Runtime | Completed | `internal/orchestration-service`, `cmd/orchestration-service`, and `pkg/orchestrator`: Pluggable Nomad orchestration engine (Live HTTP API client & in-memory simulator), HCL2 job generator (`.nomad.hcl`), zero-downtime rollout strategies (Rolling, Canary, Blue-Green), allocation tracking, health check supervision, deployment lifecycle API (`POST/GET /services/{id}/deployments`, `POST /services/{id}/deploy`, `POST /services/{id}/scale`, `GET /deployments/{id}`, `GET /deployments/{id}/spec`, `GET /deployments/{id}/allocations`, `GET /deployments/{id}/events`, `POST /deployments/{id}/promote`, `POST /deployments/{id}/rollback`, `POST /deployments/{id}/cancel`), dynamic replica scaling up & down verified end-to-end on live Nomad (`scratch_e2e_test.py`), NATS JetStream integration (`adroit.deploy.requested` subscriber, emitting `adroit.deploy.started`, `adroit.deploy.healthy`, `adroit.deploy.failed`, `adroit.deploy.stopped`), migration 007 (`allocations`, `deployment_events`, `strategy`, `canary_promoted`, `job_spec`, `services.active_deployment_id`), and API Gateway / dev runner supervisor integration on port 8087. |
| | Enterprise Secrets (HashiCorp Vault) | Completed | `pkg/vault`, `internal/project-service`, `internal/orchestration-service`: Zero-plaintext PostgreSQL storage (`[vault:secret]`), HashiCorp Vault KV v2 backend, lifecycle variable scoping (`runtime`, `build`, `all`), target environments, audited secret reveal APIs, immutable release snapshots in Vault at deploy time, and in-memory `tmpfs` Nomad template mounting. |
| | Ingress & Caddy Routing | Completed | `internal/dns-service`, `cmd/dns-service`, and `pkg/ingress`: Dynamic Caddy Admin API reverse proxy driver with in-memory simulator, custom domain lifecycle (`POST/GET /services/{id}/domains`, `GET/POST /domains/{id}/verify`, `POST /domains/{id}/sync`, `DELETE /domains/{id}`, `GET /ingress/routes`, `POST /services/{id}/sync-routes`), CNAME and TXT challenge verification engine with localhost/test dev bypass, NATS JetStream integration (`adroit.deploy.healthy` subscriber, emitting `adroit.domain.created`, `adroit.domain.verified`, `adroit.domain.deleted`, `adroit.route.updated`), migration 008 (`custom_domains` and `ingress_routes`), and API Gateway / dev runner supervisor integration on port 8088. |
| | Application Log Streaming | Completed | `internal/log-service`, `cmd/log-service`: Real-time container log aggregation, Server-Sent Events (SSE) and WebSocket duplex streaming with `?tail=` backfill and keep-alive heartbeats, pluggable Nomad agent tailer & simulated driver, high-performance in-memory Broadcaster pub-sub hub, historical search & filter queries (keyword search, stream, date bounds, limit/offset), NATS JetStream lifecycle subscriber (`adroit.deploy.started`, `adroit.deploy.healthy`, `adroit.deploy.failed`, `adroit.deploy.stopped`), migration 009 (`app_logs`), and API Gateway / dev runner supervisor integration on port 8089. |
| | Distributed Rate Limiting | Completed | `pkg/ratelimit`, `pkg/middleware`: Multi-node distributed rate limiting backed by Redis Cluster with atomic Lua scripts, Redis Cluster `{...}` hash tags for slot routing, automated in-memory fallback on Redis partition, customer subscription tier quotas (Free, Pro, Enterprise, Internal), token bucket and sliding window algorithms, and API Gateway integration. |
| | Multi-Source Workloads & Archive Storage | Completed | `pkg/storage`, `pkg/builder`, `internal/project-service`, `internal/build-service`, `internal/orchestration-service`: Multi-source workload support (Git, Archive/ZIP, Pre-built Container Image), pluggable object storage provider (Local filesystem, in-memory, AWS S3 / Cloudflare R2 / MinIO), safe archive normalization engine (`NormalizeToTarGz`, Zip Slip defense, streaming SHA-256 calculation), BuildKit HTTP tarball context ingestion (`--opt context=<url>`), direct container image deployment bypass to Nomad, migration 012 (`012_add_service_source_type_and_archives.up.sql`), API Gateway routes (`POST /api/v1/services/:id/upload`, `GET /api/v1/services/:id/source/download`), OpenAPI 3.0.3 spec, and Postman Collection v2.1.0 coverage. |
| | Elastic Auto-Scaling & Metrics Pipeline | Completed | `internal/runtime-service`, `cmd/runtime-service`, `pkg/orchestrator`: Dedicated elastic autoscaling daemon and telemetry pipeline on port 8090, live metrics collection (Prometheus scraping Nomad `/v1/metrics?format=prometheus` and Caddy edge metrics), horizontal scaling math evaluation with deadband and stabilization windows (scale-up cooldown 30s, scale-down cooldown 180s), Nomad job HCL `scaling { ... }` stanza generation in `pkg/orchestrator/job_spec.go`, standalone Nomad Autoscaler daemon (`adroit-nomad-autoscaler`) on port 8095, Prometheus server on port 9090, migration 013 (`service_scaling_events`, `service_metrics_samples`, `auto_scaling_enabled`, `target_cpu_percent`, `target_memory_percent`, `target_rps`, `current_replicas`), NATS JetStream `adroit.service.scaled` event publishing, and API Gateway route mounting (`/autoscale`, `/metrics`, `/scaling-events`, `/trigger-scale`). |
| | Developer CLI (`adroit`) | Completed | `cmd/adroit`, `internal/cli/`: Production-grade CLI combining Cobra command routing and Charm (`huh`, `lipgloss`) interactive terminal UX. Features multi-profile context management (`~/.adroit/config.yaml`), local project binding (`adroit.yaml`), interactive polyglot project initialization (`adroit init`), zero-friction directory packaging & deployment (`adroit deploy`), real-time SSE container log streaming (`adroit logs -f`), HashiCorp Vault secrets management (`adroit env set --secret`, `adroit env reveal`), dynamic replica scaling (`adroit scale`), one-command release rollbacks (`adroit rollback`), custom domains (`adroit domain`), and machine-readable output (`--json`) for CI/CD pipelines. |
| | Managed Database Service | Up Next | Automated provisioning of dedicated PostgreSQL & Redis instances on Nomad (`managed-database-service`). |


