# Adroit Build Service (`build-service`)

The **Build Service** manages the artifact creation lifecycle within the Adroit platform. It turns source code into deployable, immutable OCI container images via pluggable build engines (Local/Dev BuildKit simulator and Nomad batch cluster runner), streams execution logs in real time, and coordinates event-driven handoffs between Git push webhooks and the Orchestration runtime.

---

## 1. Core Architecture

```
                                Git Push / Webhook
                                        │
                                        ▼
                           ┌─────────────────────────┐
                           │ git-integration-service │
                           └────────────┬────────────┘
                                        │ (adroit.build.requested)
                                        ▼
   ┌────────────────────────────────────────────────────────────────────────┐
   │                       Adroit API Gateway (:8080)                       │
   │                /api/v1/builds  |  /api/v1/services/{id}/builds         │
   └───────────────────────────────────┬────────────────────────────────────┘
                                       │
                                       ▼
   ┌────────────────────────────────────────────────────────────────────────┐
   │                     Build Service (Port 8086)                          │
   │                                                                        │
   │   1. Build Lifecycle Management (queued -> building -> succeeded)      │
   │   2. Pluggable Execution Engine (`pkg/builder`)                        │
   │      - LocalDevBuilder: BuildKit step emulation & image generation     │
   │      - NomadBatchBuilder: Dynamic Nomad batch job HCL generation       │
   │   3. Live Log Streaming (`build_logs` table)                           │
   │   4. Cancellation & Concurrency Control                                │
   │   5. Event-Driven Handoff (`adroit.deploy.requested`)                  │
   └───────────────┬────────────────────────────────────────┬───────────────┘
                   │                                        │
                   ▼                                        ▼
         PostgreSQL Database                     NATS JetStream Backbone
       (builds, build_logs,                    - adroit.build.requested
          deployments)                         - adroit.build.started
                                               - adroit.build.completed
                                               - adroit.build.failed
                                               - adroit.deploy.requested
                                                            │
                                                            ▼
                                                  Orchestration Service
                                                    (Nomad Scheduler)
```

---

## 2. API Endpoints

All endpoints require JWT Bearer token authentication and verify organization membership.

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `POST` | `/api/v1/services/{id}/builds` | Trigger manual build (with optional commit, branch, or build_args override) |
| `GET` | `/api/v1/services/{id}/builds` | List build history for a service (`?limit=`, `?offset=`, `?status=`) |
| `POST` | `/api/v1/builds` | Trigger build with `service_id` in JSON body |
| `GET` | `/api/v1/builds/{id}` | Detailed build status, image tag, digest, duration, and error message |
| `POST` | `/api/v1/builds/{id}/cancel` | Abort an in-flight or queued build |
| `GET` | `/api/v1/builds/{id}/logs` | Fetch chronologically ordered execution logs for a build |

---

## 3. Build Engines (`pkg/builder`)

The platform supports pluggable builder engines via the `builder.Builder` interface:

1. **Local / Dev Runner (`LocalDevBuilder`)**:
   - Emulates rootless BuildKit multi-stage builds.
   - Outputs canonical OCI tags: `registry.adroit.run/<org>/<service>:<commit>`.
   - Generates deterministic SHA256 image digests (`sha256:<digest>`).
   - Supports real-time log streaming and atomic cancellation.

2. **Nomad Batch Runner (`NomadBatchBuilder`)**:
   - Generates isolated Nomad batch job specifications running `moby/buildkit` rootless daemon.
   - Allocates dedicated CPU, memory, and ephemeral disk to isolate multi-tenant compile tasks.
   - Pushes built artifacts directly to Harbor internal registry.

---

## 4. Event Lifecycle & Flow

1. **`adroit.build.requested`**:
   - Emitted by `git-integration-service` on Git push or via `POST /api/v1/services/{id}/builds`.
   - Payload: `BuildID`, `ServiceID`, `RepoURL`, `Branch`, `CommitHash`, `DockerfilePath`.

2. **`adroit.build.started`**:
   - Emitted when worker acquires execution slot and begins compilation.
   - Status transitions to `building`.

3. **`adroit.build.completed`**:
   - Emitted when image is generated and validated.
   - Status transitions to `succeeded`.
   - Automatically provisions a pending record in `deployments` table.

4. **`adroit.deploy.requested`**:
   - Emitted immediately upon successful build completion.
   - Contains runtime requirements (`Port`, `Replicas`, `Resources`, `HealthCheck`, `EnvVars`, `RuntimeTier`) for Nomad scheduling.

5. **`adroit.build.failed` / `adroit.build.cancelled`**:
   - Emitted on compilation failure or user abortion.
   - Records error message and finished timestamp.
