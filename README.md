# Adroit Platform

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Architecture](https://img.shields.io/badge/Architecture-Event--Driven_Microservices-orange)](#architecture)
[![Nomad](https://img.shields.io/badge/Orchestration-HashiCorp_Nomad-00CA8E?logo=nomad)](https://www.nomadproject.io/)
[![BuildKit](https://img.shields.io/badge/Builder-Moby_BuildKit-2496ED?logo=docker)](https://github.com/moby/buildkit)
[![Vault](https://img.shields.io/badge/Secrets-HashiCorp_Vault-000000?logo=vault)](https://www.vaultproject.io/)

**Adroit** is an enterprise-grade, multi-tenant Cloud Platform-as-a-Service (PaaS) built in Go. It enables developers and teams to deploy web services, background workers, cron jobs, and pre-built container workloads with zero infrastructure management overhead.

---

## Architecture Overview

```
                                      [ Clients / CLI / Web UI ]
                                                   │
                                                   ▼
                                      ┌─────────────────────────┐
                                      │   API Gateway (:8080)   │
                                      │  Rate Limiting, Routing │
                                      └────────────┬────────────┘
                                                   │
         ┌───────────────────┬─────────────────────┼─────────────────────┬───────────────────┐
         ▼                   ▼                     ▼                     ▼                   ▼
   [Auth Service]     [User & Org Svc]     [Project Catalog]     [Build Service]    [Orchestration Svc]
       (:8081)             (:8082)              (:8084)               (:8086)             (:8087)
         │                   │                     │                     │                   │
         └───────────────────┴─────────────────────┼─────────────────────┴───────────────────┘
                                                   │
                                      ┌────────────┴────────────┐
                                      │ NATS JetStream Backbone │
                                      └────────────┬────────────┘
                                                   │
                        ┌──────────────────────────┼──────────────────────────┐
                        ▼                          ▼                          ▼
               [Git Integration]           [Ingress & DNS]            [Log Streaming]
                    (:8085)                    (:8088)                    (:8089)
```

---

## Workload Deployment Modes

Adroit supports flexible deployment workflows designed for modern development teams:

```
                                  Deploy Workload
                                         │
        ┌────────────────────────────────┼────────────────────────────────┐
        ▼                                ▼                                ▼
  1. GitOps Mode                   2. Archive / CLI                 3. Container Image
  source_type = "git"              source_type = "archive"          source_type = "image"
        │                                │                                │
  Git Push / Webhook               Zip / Tarball Upload             Direct Image Tag
  (GitHub/GitLab)                  (CLI `adroit deploy`)            (e.g. `postgres:16`)
        │                                │                                │
        ▼                                ▼                                │
  BuildKit Context:                BuildKit Context:                      │
  --opt context=<url>#<branch>     --opt context=<archive_url>            │
        │                          + Nomad artifact stanza                │
        └────────────────────────────────┬────────────────────────────────┘
                                         ▼
                             [ Nomad Batch Builder ] ──────── (Image Bypass)
                                         │                                │
                                         ▼                                ▼
                             [ Orchestration Service ] ◀──────────────────┘
                                (Nomad Job Scheduling)
```

### 1. GitOps Mode (`source_type = "git"`)
- Connected to GitHub or GitLab repositories.
- Automatically triggers container compilation on Git push webhooks or manual API triggers.
- **Dual Build Engines (`pkg/builder`):**
  - **Dockerfile Pipeline:** Multi-stage compilation via rootless Moby BuildKit (`moby/buildkit:v0.12.5-rootless`).
  - **Zero-Config Nixpacks Engine:** When no Dockerfile is provided, Adroit automatically invokes **Nixpacks** (`ghcr.io/railwayapp/nixpacks:latest`) to auto-detect application runtimes (Node.js, Python, Go, Ruby, Rust, etc.) and generate production containers with zero configuration.

### 2. Archive & CLI Uploads (`source_type = "archive"`)
- Deploy directly from folders and local repositories using `adroit deploy` or via `POST /api/v1/services/{id}/upload`.
- Accepts `.zip`, `.tar.gz`, `.tar`, and `.tgz` archives up to 100MB.
- **Automatic Normalization (`pkg/storage`):** Unpacks archive streams in-memory, enforces Zip Slip path traversal defenses, calculates streaming SHA-256 digests, and normalizes into native `.tar.gz` bundles consumable by BuildKit via HTTP without disk thrashing.
- Supports both Dockerfile and zero-config Nixpacks compilation.
- Pluggable storage providers: Local Filesystem, In-Memory, and AWS S3 / Cloudflare R2 / MinIO with AWS SigV4 authentication.

### 3. Pre-built Container Images (`source_type = "image"`)
- Run pre-built OCI container images (e.g. `postgres:16-alpine`, `redis:7-alpine`, `ghcr.io/org/app:v1.0.0`) directly with **zero build overhead**.
- Bypasses BuildKit, Nixpacks, and batch compiler queues.
- Deploys instantly to HashiCorp Nomad via `POST /api/v1/services/{id}/deploy`.

---

## Enterprise Core Infrastructure

### Zero-Plaintext Secrets Management (HashiCorp Vault)
- **KV v2 Backend:** Variables marked with `is_secret = true` are transmitted directly to HashiCorp Vault at `adroit/{orgID}/services/{serviceID}`.
- **Database Sentinel:** The PostgreSQL table `environment_variables` stores a non-sensitive sentinel: `value = "[vault:secret]"`. Plaintext secrets never touch database logs or backups.
- **Variable Scoping (`runtime`, `build`, `all`):** BuildKit only receives `build` or `all` scoped variables. Production secrets (e.g. `DATABASE_PASSWORD`, payment tokens) are strictly excluded from Docker layers.
- **Immutable Deployment Releases:** Every rollout snapshots secrets into Vault under `adroit/{orgID}/deployments/{deploymentID}`, ensuring rollbacks run historical releases reliably.
- **In-Memory tmpfs Nomad Mounting:** Secrets are rendered in-memory via Nomad `template` stanzas (`secrets/app.env`). Zero plaintext secrets exist in Nomad job HCL or Raft consensus logs.

### HashiCorp Nomad Orchestration & Runtime Sandboxing
- **Dynamic HCL2 Generation:** Translates declarative service specifications into Nomad job files (`adroit-svc-<service_id>`) with resource stanzas (`cpu`, `memory`) and port mappings.
- **Consul Connect Service Mesh:** Injects `proxy {}` sidecars for automatic zero-trust mutual TLS between microservices.
- **Dynamic Traefik Ingress:** Automatically attaches Traefik edge tags for automatic HTTPS path and host routing.
- **Zero-Downtime Rollout Strategies:** Supports Rolling updates, Canary rollouts with automated/manual promotion (`POST /deployments/{id}/promote`), and Blue-Green routing.
- **Runtime Sandboxing:** General OCI Docker containers or hardware/syscall virtualization via gVisor (`runsc`).

---

## Microservices Topology

| Service | Port | Description |
| :--- | :--- | :--- |
| **API Gateway** | `8080` | Central ingress, distributed Redis rate limiting, JWT token validation, route reverse-proxying |
| **Auth Service** | `8081` | User registration, authentication, JWT token issuance, bcrypt hashing |
| **User & Org Service** | `8082` | User profiles, multi-tenant organizations, memberships, role-based access control, invitations |
| **Notification Service** | `8083` | Transactional email delivery, invitation templates, HMAC-signed webhooks |
| **Project & Catalog Service** | `8084` | Workload definitions, projects, source archives, Vault secrets management |
| **Git Integration Service** | `8085` | GitHub/GitLab webhook processing, signature verification, push event routing |
| **Build Service** | `8086` | BuildKit compilation, Nomad batch job execution, log capture, image registry pushing |
| **Orchestration Service** | `8087` | HashiCorp Nomad scheduling, rolling/canary/blue-green rollouts, health checking, dynamic scaling |
| **Ingress & DNS Service** | `8088` | Caddy reverse proxy configuration, custom domain DNS challenge validation (CNAME/TXT) |
| **Log Streaming Service** | `8089` | Real-time container log aggregation, Server-Sent Events (SSE), and WebSocket streaming |

---

## Getting Started

### Prerequisites
- **Go 1.22+**
- **PostgreSQL 14+**
- **NATS Server 2.10+** (with JetStream enabled)
- **HashiCorp Nomad 1.6+** (optional; simulated in-memory engine included for development)
- **HashiCorp Vault 1.14+** (optional; simulated secret store included for development)

### Running Locally with Process Supervisor

The repository includes an all-in-one development process supervisor (`cmd/adroit-dev`) that boots dependencies and services concurrently:

```bash
# 1. Clone repository
git clone https://github.com/dev-nathaniel/Adroit-platform.git
cd Adroit-platform

# 2. Run database migrations
go run ./cmd/migrate up

# 3. Start development stack
go run ./cmd/adroit-dev
```

### Running Tests

```bash
# Run all unit and integration tests across the entire workspace
go test ./...

# Run tests with race condition detector
go test -race ./pkg/... ./internal/...
```

---

## API Documentation & Postman Collection

- **OpenAPI 3.0 Specification:** [`docs/openapi.yaml`](docs/openapi.yaml)
- **Postman Collection (v2.1.0):** [`docs/adroit-platform.postman_collection.json`](docs/adroit-platform.postman_collection.json)
  - Features automated JWT token extraction, variable chaining (`{{token}}`, `{{serviceId}}`, `{{orgId}}`), and coverage for all endpoints including archive uploads and container deployments.
- **API Reference Guide:** [`docs/README.md`](docs/README.md)

---

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.
