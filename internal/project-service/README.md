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
| `GET` | `/api/v1/projects/{id}/env` | Bearer JWT | List project-level environment variables |
| `POST` | `/api/v1/projects/{id}/env` | Bearer JWT | Upsert project-level environment variables |
| `DELETE` | `/api/v1/projects/{id}/env/{key}` | Bearer JWT | Delete a project-level environment variable |

### 5.2 Service Endpoints (Standalone & In-Project)

| Method | Path | Auth | Description |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/services` | Bearer JWT | Create service (`project_id` optional; standalone if omitted) |
| `GET` | `/api/v1/services` | Bearer JWT | List services (query: `organization_id`, `project_id`, `standalone=true`) |
| `GET` | `/api/v1/services/{id}` | Bearer JWT | Get service details |
| `PATCH` | `/api/v1/services/{id}` | Bearer JWT | Update service configuration (specs, commands, resources) |
| `POST` | `/api/v1/services/{id}/move` | Bearer JWT | Move service to target project or detach to standalone (`{"project_id": null}`) |
| `DELETE` | `/api/v1/services/{id}` | Bearer JWT | Delete service |
| `GET` | `/api/v1/services/{id}/env` | Bearer JWT | List service env vars (or `?resolved=true` for project+service merged view) |
| `POST` | `/api/v1/services/{id}/env` | Bearer JWT | Upsert service-level environment variables |
| `DELETE` | `/api/v1/services/{id}/env/{key}` | Bearer JWT | Delete a service-level environment variable |

---

## 6. NATS Events Emitted

* `adroit.project.created`: Emitted when a project is created.
* `adroit.project.updated`: Emitted when a project is modified.
* `adroit.project.deleted`: Emitted when a project is deleted.
* `adroit.service.created`: Emitted when a new service (standalone or in-project) is created.
* `adroit.service.updated`: Emitted when a service is updated.
* `adroit.service.deleted`: Emitted when a service is deleted.
* `adroit.service.moved`: Emitted when a service is moved to another project or detached to standalone.
