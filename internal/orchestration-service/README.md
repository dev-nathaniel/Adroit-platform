# Adroit Orchestration & Runtime Service (`orchestration-service`)

The **Orchestration & Runtime Service** manages the workload scheduling, rollout lifecycle, and instance scaling of applications on **HashiCorp Nomad**. It bridges artifact creation (from `build-service`) with running production infrastructure, translating declarative service configurations into authentic Nomad HCL2 job specifications (`.nomad.hcl`), tracking allocations across cluster nodes, coordinating zero-downtime rollouts (rolling, blue-green, and canary), and supervising health checks.

---

## 1. System Architecture & Lifecycle

```
  Build Service                    NATS JetStream                    Orchestration Service
┌────────────────┐             ┌─────────────────────┐              ┌──────────────────────┐
│ Emits          │────────────▶│adroit.deploy.requested│────────────▶│ HandleDeployRequested│
│ Image Tag &    │             └─────────────────────┘              │                      │
│ Resources      │                                                  │ 1. Render Nomad HCL  │
└────────────────┘                                                  │ 2. Submit to Driver  │
                                                                    │ 3. Supervise Rollout │
                                                                    └──────────┬───────────┘
                                                                               │
                                                                               ▼
                                         Nomad Cluster / Simulated Engine ◀────┘
                                         ┌───────────────────────────────────────┐
                                         │ adroit-svc-<service_id>               │
                                         │  ├─ Task Group: app                   │
                                         │  │   ├─ Task: server (Docker/gVisor)  │
                                         │  │   └─ Sidecar: Consul Connect       │
                                         │  └─ Update: Canary / Rolling          │
                                         └──────────────────┬────────────────────┘
                                                            │
                                  ┌─────────────────────────┴────────────────────────┐
                                  ▼                                                  ▼
                        Allocations Healthy                                 Health Check Timeout
                                  │                                                  │
                                  ▼                                                  ▼
                    ┌───────────────────────────┐                      ┌───────────────────────────┐
                    │ Status: active            │                      │ Status: failed            │
                    │ Route: https://<slug>.run │                      │ Auto-revert to previous   │
                    │ Subject: deploy.healthy   │                      │ Subject: deploy.failed    │
                    └───────────────────────────┘                      └───────────────────────────┘
```

---

## 2. Key Capabilities

1. **Nomad Job Specification Generator (`pkg/orchestrator`)**:
   - Compiles full HCL2 service jobs (`adroit-svc-<id>`) with resource stanzas (`cpu`, `memory`), network port mappings, and environment variables.
   - Generates Consul service discovery definitions with dynamic ingress tags (`traefik.enable=true`, `urlprefix-/<service>`, Host routing `app.adroit.run`) and Consul Connect sidecar proxies.
   - Configures runtime sandboxing per `RuntimeTier`:
     - `general`: Standard OCI Docker containers.
     - `isolated`: Hardware/syscall virtualization via gVisor (`runsc`).

2. **Rollout Strategies**:
   - **Rolling (`rolling`)**: Sequential zero-downtime updates with health check verification (`min_healthy_time = "10s"`).
   - **Canary (`canary`)**: Provisions an isolated canary instance (`canary = 1`). Verifies health, allows automated or manual promotion via API (`POST /deployments/{id}/promote`), and auto-reverts on failure.
   - **Blue-Green (`blue-green`)**: Provisions a full parallel set of instances, confirms 100% readiness across sidecars, and switches routing.

3. **Pluggable Cluster Drivers**:
   - **Live Nomad Client (`NomadHTTPClient`)**: Connects to production HashiCorp Nomad HTTP API (`/v1/jobs`, `/v1/job/:id/allocations`, `/v1/deployment/promote/:id`, `/v1/job/:id/scale`).
   - **Simulated Driver (`SimulatedDriver`)**: Built-in, high-fidelity in-memory Nomad state engine for local development and CI testing. Simulates allocations, canary health, and scaling with zero external dependencies.

4. **Event-Driven Handoff (NATS JetStream)**:
   - Subscribes to:
     - `adroit.deploy.requested`
   - Emits:
     - `adroit.deploy.started`
     - `adroit.deploy.healthy`
     - `adroit.deploy.failed`
     - `adroit.deploy.stopped`

5. **Runtime & Scaling Management**:
   - Dynamically scales running instances via `POST /services/{id}/scale` and propagates updates to Nomad task group counts.

6. **Immutable Secret & Configuration Releases (HashiCorp Vault)**:
   - At deployment time, snapshots runtime environment variables into `deployment_env_snapshots` and writes an immutable snapshot of active secrets to Vault at `adroit/{orgID}/deployments/{deploymentID}`.
   - Renders Nomad jobs using native `template` stanzas (`destination = "secrets/app.env"`, `env = true`), eliminating plaintext secrets from job specs, Raft state, and CLI inspections.

---

## 3. REST API Reference

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/healthz` | Public service health check |
| `POST` | `/api/v1/services/{id}/deploy` | Trigger a new deployment (manual or build-linked) |
| `GET` | `/api/v1/services/{id}/deployments` | List paginated deployment history for a service |
| `POST` | `/api/v1/services/{id}/scale` | Scale replica instance count up/down |
| `GET` | `/api/v1/deployments/{id}` | Detailed deployment metadata, active URL, and allocations |
| `GET` | `/api/v1/deployments/{id}/spec` | Fetch rendered Nomad HCL job specification |
| `GET` | `/api/v1/deployments/{id}/allocations` | List Nomad allocation states for deployment |
| `GET` | `/api/v1/deployments/{id}/events` | Rollout audit log and milestone timeline |
| `POST` | `/api/v1/deployments/{id}/promote` | Manually promote a canary deployment |
| `POST` | `/api/v1/deployments/{id}/rollback` | Rollback service to a previous deployment |
| `POST` | `/api/v1/deployments/{id}/cancel` | Abort an active or deploying rollout |

---

## 4. Configuration

| Environment Variable | Default | Description |
| :--- | :--- | :--- |
| `PORT` | `8087` | HTTP listen port |
| `BASE_DOMAIN` | `adroit.run` | Platform base domain for generated service URLs |
| `NOMAD_ADDR` | *(empty)* | Address of live Nomad cluster (e.g. `http://127.0.0.1:4646`) |
| `NOMAD_TOKEN` | *(empty)* | Nomad ACL secret token |
| `NOMAD_DATACENTER` | `dc1` | Target Nomad cluster datacenter |
| `ORCHESTRATOR_DRIVER_TYPE` | `simulated` | Driver type: `simulated` (local dev) or `nomad` (cluster) |
| `NATS_URL` | `nats://localhost:4222` | NATS JetStream server address |
| `JWT_SECRET` | *(dev key)* | Secret key for verifying auth bearer tokens |
| `VAULT_ADDR` | `http://127.0.0.1:8200` | HashiCorp Vault server address |
| `VAULT_TOKEN` | `adroit-dev-root-token` | HashiCorp Vault authentication token |

