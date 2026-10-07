# Adroit Project & Service Catalog (`project-service`)

> **Package:** `internal/project-service`  
> **Entrypoint:** `cmd/project-service` (Port: `8084`)  
> **Dependencies:** PostgreSQL, NATS JetStream (or InMemBus fallback), Chi Router, JWT Identity

---

## 1. Purpose & Responsibilities

The **Project & Service Catalog** manages projects, workloads (services), and environment variables across multi-tenant organizations on the Adroit PaaS.

Key capabilities:
* **Projects Management:** Group services, workloads, and shared environment configurations under named projects within an organization.
* **Standalone Services:** Services can be created directly inside an organization without being assigned to a project (`project_id = null`).
* **Multi-Source Workload Support:** Supports GitOps (`source_type: "git"`), Archive/CLI folder uploads (`source_type: "archive"`), and Pre-built container images (`source_type: "image"`).
* **Archive Upload & Normalization:** Ingests `.zip` and `.tar.gz` archives, normalizes them into standard `.tar.gz`, calculates SHA-256 digests, and stores them in pluggable storage backends (Local, S3, Cloudflare R2, MinIO).
* **Dynamic Workload Reassignment:** Services can be smoothly moved into a project, transferred between projects, or detached back into standalone services via `POST /api/v1/services/{id}/move`.
* **Multi-Tenant Cross-Org Security:** Prevents moving services across different organizations.
* **Environment Variables Management:** Manage secrets and environment variables at both the service and project levels, with hierarchical resolution (service variables override project-level variables).
* **Event-Driven Lifecycle:** Publishes events across NATS JetStream (`adroit.project.*`, `adroit.service.*`) to coordinate with Build, Orchestration, and Logging services.

---

## 2. Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | Integer | `8084` | HTTP server listening port |
| `ENV` | String | `development` | Runtime environment (`development`, `staging`, `production`) |
| `LOG_LEVEL` | String | `info` | Minimum log severity level (`debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` | String | `json` | Structured output format (`json` or `text`) |
| `JWT_SECRET` | String | `super-secret-...` | Shared HMAC-SHA256 signing secret for token verification |
| `DB_HOST` | String | `localhost` | PostgreSQL host |
| `DB_PORT` | Integer | `5432` | PostgreSQL port |
| `DB_USER` | String | `adroit` | PostgreSQL user |
| `DB_PASSWORD`| String | `adroit_secret` | PostgreSQL password |
| `DB_NAME` | String | `adroit_db` | PostgreSQL database name |
| `NATS_URL` | String | `nats://localhost:4222` | NATS JetStream server address |
| `VAULT_ADDR` | String | `http://127.0.0.1:8200` | HashiCorp Vault server address |
| `VAULT_TOKEN` | String | `adroit-dev-root-token` | HashiCorp Vault authentication token |
| `STORAGE_BACKEND` | String | `local` | Source archive storage provider: `local`, `s3`, or `memory` |
| `STORAGE_LOCAL_DIR` | String | `data/uploads/sources` | Storage directory for local filesystem backend |
| `STORAGE_BASE_URL` | String | `/api/v1/services/source/download` | Base download URL prefix |
| `STORAGE_S3_ENDPOINT` | String | *(empty)* | Custom S3 endpoint (e.g. MinIO or Cloudflare R2) |
| `STORAGE_S3_REGION` | String | `us-east-1` | S3 region or `auto` for Cloudflare R2 |
| `STORAGE_S3_BUCKET` | String | `adroit-sources` | Target S3 bucket name |
| `STORAGE_S3_ACCESS_KEY`| String | *(empty)* | AWS / R2 / MinIO access key |
| `STORAGE_S3_SECRET_KEY`| String | *(empty)* | AWS / R2 / MinIO secret key |
| `STORAGE_S3_USE_PATH_STYLE` | Boolean | `false` | Enable path-style S3 routing (required for MinIO) |

---

## 3. How to Run Locally

### Direct execution
```bash
go run ./cmd/project-service
```

### Via All-in-One Process Supervisor
```bash
go run ./cmd/adroit-dev
```

---

## 4. How to Run Unit Tests

```bash
# Run tests
go test -v ./internal/project-service/...

# Run with race detection
go test -race ./internal/project-service/...

# Run with coverage
go test -cover ./internal/project-service/...
```

---

## 5. API Route Table

All routes (except `/healthz`) require a valid Bearer JWT token in the `Authorization` header (`Bearer <token>`).

### 5.1 Project Endpoints

| Method | Path | Auth | Description |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/projects` | Bearer JWT | Create a new project in an organization |
| `GET` | `/api/v1/projects` | Bearer JWT | List projects (optional `?organization_id=<id>`) |
| `GET` | `/api/v1/projects/{id}` | Bearer JWT | Get project details and service count |
| `PATCH` | `/api/v1/projects/{id}` | Bearer JWT | Update project name, slug, or description |
| `DELETE` | `/api/v1/projects/{id}` | Bearer JWT | Delete project (attached services become standalone) |
| `GET` | `/api/v1/projects/{id}/services` | Bearer JWT | List all services inside a project |
| `POST` | `/api/v1/projects/{id}/services` | Bearer JWT | Create a service directly in a project |
| `GET` | `/api/v1/projects/{id}/env` | Bearer JWT | List project-level environment variables (secrets masked as `••••••••`) |
| `POST` | `/api/v1/projects/{id}/env` | Bearer JWT | Upsert project-level environment variables (secrets written to Vault) |
| `DELETE` | `/api/v1/projects/{id}/env/{key}` | Bearer JWT | Delete a project-level environment variable |
| `POST` | `/api/v1/projects/{id}/env/{key}/reveal` | Bearer JWT | Reveal unmasked project secret from HashiCorp Vault (audit-logged) |

### 5.2 Service Endpoints (Standalone & In-Project)

| Method | Path | Auth | Description |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/services` | Bearer JWT | Create service (`source_type`: `git`, `archive`, `image`) |
| `GET` | `/api/v1/services` | Bearer JWT | List services (query: `organization_id`, `project_id`, `standalone=true`) |
| `GET` | `/api/v1/services/{id}` | Bearer JWT | Get service details |
| `PATCH` | `/api/v1/services/{id}` | Bearer JWT | Update service configuration (specs, commands, resources, image_url) |
| `POST` | `/api/v1/services/{id}/upload` | Bearer JWT | Upload source archive (`.zip` / `.tar.gz`), optional `?trigger_build=true` |
| `GET` | `/api/v1/services/{id}/source/download` | Public / Gateway | Stream normalized `.tar.gz` source code archive for BuildKit or client |
| `POST` | `/api/v1/services/{id}/move` | Bearer JWT | Move service to target project or detach to standalone (`{"project_id": null}`) |
| `DELETE` | `/api/v1/services/{id}` | Bearer JWT | Delete service |
| `GET` | `/api/v1/services/{id}/env` | Bearer JWT | List service env vars (or `?resolved=true` for project+service merged view; secrets masked) |
| `POST` | `/api/v1/services/{id}/env` | Bearer JWT | Upsert service-level environment variables (secrets written to Vault) |
| `DELETE` | `/api/v1/services/{id}/env/{key}` | Bearer JWT | Delete a service-level environment variable |
| `POST` | `/api/v1/services/{id}/env/{key}/reveal` | Bearer JWT | Reveal unmasked service secret from HashiCorp Vault (audit-logged) |

---

## 6. NATS Events Emitted

* `adroit.project.created`: Emitted when a project is created.
* `adroit.project.updated`: Emitted when a project is modified.
* `adroit.project.deleted`: Emitted when a project is deleted.
* `adroit.service.created`: Emitted when a new service (standalone or in-project) is created.
* `adroit.service.updated`: Emitted when a service is updated.
* `adroit.service.deleted`: Emitted when a service is deleted.
* `adroit.service.moved`: Emitted when a service is moved to another project or detached to standalone.
* `adroit.env.updated`: Emitted whenever project or service environment variables/secrets change.

---

## 7. HashiCorp Vault Secrets Storage & Scoping

* **Vault KV v2 Paths:** Service secrets are stored at `adroit/{orgID}/services/{serviceID}`. Project secrets are stored at `adroit/{orgID}/projects/{projectID}`.
* **Database Sentinel:** The PostgreSQL table `environment_variables` stores `value = "[vault:secret]"` for all secrets.
* **Variable Scoping (`scope`):** Supports `runtime` (default for secrets), `build`, and `all`. BuildKit only accesses variables where `scope IN ('build', 'all')`, ensuring runtime secrets are never baked into container image layers.
* **Target Environments (`target_env`):** Supports `all`, `production`, `staging`, `preview`, `development`.
* **Offline Resiliency:** When Vault is unreachable during local development, the service falls back gracefully to `SimulatedSecretStore`.

---

## 8. Service Source Types & Validation Rules

Adroit supports 3 source modes configured by `source_type`:

| `source_type` | Required Fields | Optional Fields | Description |
| :--- | :--- | :--- | :--- |
| **`git`** (default) | `repo_url` | `branch`, `build_command`, `dockerfile_path` | Connected to a remote Git repository (GitHub/GitLab). Triggered via webhooks or manual builds. |
| **`archive`** | *(none at creation)* | `build_command`, `dockerfile_path` | Deployed via CLI folder uploads or CI artifacts. User uploads source archive via `POST /services/{id}/upload`. Build can be auto-triggered on upload via `?trigger_build=true`. |
| **`image`** | `image_url` | `port`, `env_vars`, `resources` | Directly references an existing Docker / OCI container image (e.g. `redis:7-alpine`, `postgres:16`, `ghcr.io/org/app:v1.0`). **Bypasses Build Service** entirely and deploys directly to Nomad via Orchestration Service. |


