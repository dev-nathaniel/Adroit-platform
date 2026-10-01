# Adroit Platform API Documentation & Tooling

This directory contains standardized API documentation, architecture guides, and testing collections for the **Adroit Multi-Tenant Cloud Platform-as-a-Service (PaaS)**.

---

## 1. Files Included

| File | Type | Description |
| :--- | :--- | :--- |
| [`openapi.yaml`](./openapi.yaml) | OpenAPI 3.0.3 | Full specification covering all API Gateway routes, microservice endpoints, request/response models, and error envelopes. |
| [`adroit-platform.postman_collection.json`](./adroit-platform.postman_collection.json) | Postman Collection v2.1.0 | Ready-to-import Postman collection with dynamic token extraction, environment variable chaining, and automated authorization. |

---

## 2. Using the Postman Collection

1. Open **Postman** (or Bruno / Insomnia / Thunder Client).
2. Click **Import** in the top-left corner.
3. Select or drag-and-drop `docs/adroit-platform.postman_collection.json`.
4. **Token Automation is Pre-configured:**
   - Execute **`2. Authentication -> Register User`** or **`Login User`**.
   - The embedded test script automatically extracts the issued JWT token into the collection variable `{{token}}`.
   - All protected requests in the collection (`/me`, `/profile`, `/organizations`, `/projects`, `/services`, `/deployments`) automatically inherit the token via Bearer authorization.
   - Creating an organization or invitation automatically populates `{{orgId}}`, `{{inviteToken}}`, and `{{inviteId}}`.

---

## 3. Viewing the OpenAPI Specification

### Option A: VS Code / IDE Extensions
Install the **OpenAPI (Swagger) Editor** or **Swagger Viewer** extension in VS Code:
- Open `docs/openapi.yaml`
- Press `Shift + Alt + E` (or right-click -> *OpenAPI: Preview*) to render the interactive Swagger UI.

### Option B: Docker Swagger UI
Run a local interactive Swagger UI container pointing to `openapi.yaml`:
```bash
docker run -p 8088:8080 -e SWAGGER_JSON=/spec/openapi.yaml -v ${PWD}/docs:/spec swaggerapi/swagger-ui
```
Then visit `http://localhost:8088` in your browser.

### Option C: Online Swagger Editor
Copy and paste the contents of `docs/openapi.yaml` into [editor.swagger.io](https://editor.swagger.io).

---

## 4. Endpoints Overview

All routes are dispatched through the API Gateway at `http://localhost:8080`:

* **System & Gateway:** `GET /healthz`, `GET /`, `GET /api/v1/status`
* **Authentication:** `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `GET /api/v1/auth/me`, `GET /api/v1/auth/github/login`, `GET /api/v1/auth/github/callback`
* **Profiles:** `GET /api/v1/users/profile`, `PATCH /api/v1/users/profile`
* **Organizations:** `GET /api/v1/users/organizations`, `POST /api/v1/users/organizations`, `GET /api/v1/users/organizations/:orgID`
* **Members:** `GET /api/v1/users/organizations/:orgID/members`, `POST /api/v1/users/organizations/:orgID/members`
* **Invitations:**
  - `POST /api/v1/users/organizations/:orgID/invitations` (Create invitation)
  - `GET /api/v1/users/organizations/:orgID/invitations` (List invitations)
  - `DELETE /api/v1/users/organizations/:orgID/invitations/:inviteID` (Revoke invitation)
  - `GET /api/v1/users/invitations/:token` (Public preview invitation)
  - `POST /api/v1/users/invitations/:token/accept` (Accept invitation & join workspace)
* **Projects & Workspaces:**
  - `POST /api/v1/projects` (Create project)
  - `GET /api/v1/projects` (List projects, optional `?organization_id=`)
  - `GET /api/v1/projects/:id` (Project details & service count)
  - `PATCH /api/v1/projects/:id` (Update project)
  - `DELETE /api/v1/projects/:id` (Delete project)
  - `GET /api/v1/projects/:id/services` (List services in project)
  - `POST /api/v1/projects/:id/services` (Create service in project)
* **Services (Workload Lifecycle):**
  - `POST /api/v1/services` (Create service, standalone or project-scoped)
  - `GET /api/v1/services` (List services, filter by `organization_id`, `project_id`, or `standalone=true`)
  - `GET /api/v1/services/:id` (Service details & specs)
  - `PATCH /api/v1/services/:id` (Update service configuration)
  - `POST /api/v1/services/:id/move` (Move service between projects or detach to standalone)
  - `DELETE /api/v1/services/:id` (Delete service)
* **Environment Variables & Secrets Management:**
  - `GET /api/v1/projects/:id/env` (List project env vars; secret values masked as `••••••••`)
  - `POST /api/v1/projects/:id/env` (Upsert project env vars / secrets; encrypted into Vault KV v2)
  - `DELETE /api/v1/projects/:id/env/:key` (Delete project env var / secret)
  - `POST /api/v1/projects/:id/env/:key/reveal` (Audit-logged Vault secret unmasking)
  - `GET /api/v1/services/:id/env` (List service env vars; secret values masked, `?resolved=true` for hierarchical merged view)
  - `POST /api/v1/services/:id/env` (Upsert service env vars / secrets; encrypted into Vault KV v2)
  - `DELETE /api/v1/services/:id/env/:key` (Delete service env var / secret)
  - `POST /api/v1/services/:id/env/:key/reveal` (Audit-logged Vault secret unmasking)
* **Git Integration & Webhooks:**
  - `GET /api/v1/git/providers` (List supported providers)
  - `GET /api/v1/git/repositories` (Discover organization repos)
  - `POST /api/v1/webhooks/github` (Ingest push/ping events, verify HMAC-SHA256 signature)
* **Build Service:**
  - `POST /api/v1/builds` (Trigger build)
  - `GET /api/v1/builds/:id` (Get build status & metadata)
  - `GET /api/v1/services/:id/builds` (List builds for service)
  - `GET /api/v1/builds/:id/logs` (Get build logs)
  - `GET /api/v1/builds/:id/logs/stream` (Live SSE build log stream)
  - `POST /api/v1/builds/:id/cancel` (Cancel active build)
* **Deployments & Orchestration:**
  - `POST /api/v1/services/:id/deploy` (Trigger rollout)
  - `GET /api/v1/services/:id/deployments` (Deployment history)
  - `GET /api/v1/deployments/:id` (Deployment status & active URL)
  - `GET /api/v1/deployments/:id/spec` (View generated Nomad HCL job specification)
  - `GET /api/v1/deployments/:id/allocations` (List cluster allocation states)
  - `GET /api/v1/deployments/:id/events` (Audit log timeline)
  - `POST /api/v1/deployments/:id/promote` (Promote canary rollout)
  - `POST /api/v1/deployments/:id/rollback` (Roll back to previous release)
  - `POST /api/v1/deployments/:id/cancel` (Abort active rollout)
  - `POST /api/v1/services/:id/scale` (Dynamically scale replicas)
* **Edge Ingress & Custom Domains:**
  - `GET /api/v1/domains` (List domains)
  - `POST /api/v1/domains` (Register custom domain)
  - `GET /api/v1/domains/:id` (Domain status & DNS challenge instructions)
  - `POST /api/v1/domains/:id/verify` (Trigger CNAME/TXT DNS verification)
  - `DELETE /api/v1/domains/:id` (Remove domain)
* **Log Streaming Service:**
  - `GET /api/v1/services/:id/logs` (Historical log query & search)
  - `GET /api/v1/services/:id/logs/stream` (Live SSE stream)
  - `GET /api/v1/services/:id/logs/ws` (Live WebSocket stream)
  - `GET /api/v1/deployments/:id/logs` (Deployment logs)
  - `GET /api/v1/deployments/:id/logs/stream` (Deployment SSE stream)
  - `POST /api/v1/services/:id/logs` (Batch log ingestion)
  - `POST /api/v1/logs/ingest` (Global batch log ingestion)
  - `DELETE /api/v1/services/:id/logs` (Purge historical logs)

---

## 5. Enterprise Secrets Architecture with HashiCorp Vault

Adroit enforces zero-plaintext secrets storage, defense-in-depth isolation, and immutable configuration releases across the entire workload lifecycle.

### 5.1 Architecture Overview

```
                                  +-------------------------------------+
                                  |         User / CI/CD Client         |
                                  +-------------------------------------+
                                                     |
                                            POST /services/:id/env
                                                     |
                                                     v
                                  +-------------------------------------+
                                  |           Project Service           |
                                  +-------------------------------------+
                                     /                               \
                   Secret Values    /                                 \ Non-Secret / Metadata
                                   v                                   v
             +-------------------------------+             +-------------------------------+
             |       HashiCorp Vault         |             |      PostgreSQL Database      |
             |       (KV v2 Engine)          |             |    (Environment Variables)    |
             | adroit/{org}/services/{svc}   |             |  value = "[vault:secret]"     |
             | adroit/{org}/projects/{prj}   |             |  is_secret = true             |
             +-------------------------------+             +-------------------------------+
                                   ^                                   ^
                                   |                                   |
                         Snapshot Secrets                     Read Scope Metadata
                                   |                                   |
                                  +-------------------------------------+
                                  |        Orchestration Service        |
                                  +-------------------------------------+
                                     |
               Generates Immutable Deployment Snapshot:
               - DB: deployment_env_snapshots (Release #, config hash)
               - Vault: adroit/{org}/deployments/{deployment_id}
                                     |
                                     v
                        Renders Nomad Job Specification:
                        template {
                          destination = "secrets/app.env"
                          env         = true
                        }
                                     |
                                     v
                        Container mounts secrets in-memory via tmpfs.
                        Zero secrets in Nomad job HCL or Raft consensus logs!
```

### 5.2 Key Security Controls

1. **HashiCorp Vault KV v2 Storage**:
   - Variables flagged with `is_secret: true` are transmitted directly to HashiCorp Vault over mutual TLS / HTTP REST (`adroit/{orgID}/services/{serviceID}` or `adroit/{orgID}/projects/{projectID}`).
   - The PostgreSQL primary database stores a non-sensitive sentinel: `value = "[vault:secret]"` alongside metadata (`vault_path`, `scope`, `target_env`).
   - Plaintext secrets are never stored in database backups, query logs, or replication streams.

2. **API Masking & Audited Reveal Workflow**:
   - All standard `GET` endpoints (`GET /projects/:id/env`, `GET /services/:id/env`) mask secret values as `••••••••`.
   - Access to unmasked secrets requires calling dedicated, audit-logged endpoints:
     - `POST /api/v1/projects/:id/env/:key/reveal`
     - `POST /api/v1/services/:id/env/:key/reveal`
   - Only authorized organization members with verified RBAC permissions can reveal secrets.

3. **Lifecycle Scoping (`scope`)**:
   - **`runtime`**: Available only to running production containers. Completely excluded from BuildKit execution.
   - **`build`**: Available only during container image compilation (e.g. `NPM_TOKEN`, compiler flags). Never passed into container runtime.
   - **`all`**: Available during both build compilation and container runtime.
   - **Prevention of Docker Layer Leakage**: The `build-service` strictly queries variables where `scope IN ('build', 'all')`. Sensitive production credentials (such as `DATABASE_PASSWORD` or production Stripe keys) are scoped as `runtime` and are **never passed into Docker build args or baked into container image layers**.

4. **Target Environments (`target_env`)**:
   - Supports environment segregation: `all`, `production`, `staging`, `preview`, `development`.
   - Workloads executing in preview or staging pull only variables matching their specific environment or `all`.

5. **Immutable Deployment Configuration Snapshots**:
   - Every deployment execution captures an immutable snapshot of all merged project and service environment variables.
   - Stored in PostgreSQL table `deployment_env_snapshots` (`config_release_num`, non-secret key-values, and secret key names).
   - The exact secret values active at deploy time are snapshotted in Vault under `adroit/{orgID}/deployments/{deploymentID}`.
   - When a user initiates a rollback (`POST /deployments/:id/rollback`), the deployment runs with the exact historical snapshot rather than modern mutable configuration.

6. **Nomad Orchestration via In-Memory Templates**:
   - Secrets are **never** interpolated into the Nomad job specification `env {}` block. Doing so would leak them in plaintext across Nomad Raft state logs, `nomad job inspect`, and Nomad web UI.
   - Instead, the orchestrator generates a native Nomad `template` stanza:
     ```hcl
     template {
       data        = "{{ range secret \"adroit/org-123/deployments/dep-456\" }}{{ .Key }}={{ .Value }}\n{{ end }}"
       destination = "secrets/app.env"
       env         = true
       change_mode = "restart"
     }
     ```
   - Secrets are loaded dynamically into the container via an in-memory `tmpfs` RAM filesystem (`secrets/app.env`) managed by Nomad and Consul/Vault tokens.

7. **Local Development & CI Resiliency**:
   - Both `project-service` and `orchestration-service` feature a thread-safe `SimulatedSecretStore` driver fallback.
   - If HashiCorp Vault is not running or network probes fail during development, the services smoothly fall back to the in-memory simulated driver, ensuring developers and automated CI test suites can execute without mandatory Vault clusters.
