# Adroit Orchestration & Runtime Service Entrypoint (`cmd/orchestration-service`)

> **Binary:** `bin/orchestration-service` (or `orchestration-service.exe` on Windows)  
> **Default Port:** `8087`  
> **Source Package:** `github.com/dev-nathaniel/Adroit-platform/cmd/orchestration-service`

---

## 1. Overview

`cmd/orchestration-service` is the standalone microservice daemon for the **Orchestration & Runtime Service**. It manages the scheduling, rollout lifecycle (rolling, blue-green, canary), dynamic scaling, and health monitoring of containerized workloads on **HashiCorp Nomad**.

### Key Responsibilities:
* **Nomad Job Generation**: Compiles declarative service specifications into Nomad HCL2 job files with Consul Connect sidecars and Traefik ingress tags.
* **Vault Secret Snapshots**: Snapshots configuration and secret values to HashiCorp Vault at `adroit/{orgID}/deployments/{deploymentID}` and mounts them via in-memory `template` stanzas (`secrets/app.env`).
* **Dual Driver Support**: Pluggable driver layer supporting either live Nomad clusters (`NomadHTTPClient`) or a zero-dependency in-memory simulator (`SimulatedDriver`).
* **NATS JetStream Consumer**: Listens for asynchronous build completion events (`adroit.deploy.requested`) and emits rollout milestones (`adroit.deploy.started`, `adroit.deploy.healthy`, `adroit.deploy.failed`).
* **REST & Health Endpoints**: Serves deployment inspection, canary promotion, manual rollbacks, and replica scaling APIs on port `8087`.

For full architecture details, state diagrams, and API tables, see [internal/orchestration-service/README.md](../../internal/orchestration-service/README.md).

---

## 2. Environment Variables & Configuration

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | Integer | `8087` | HTTP server listening port |
| `ENV` | String | `development` | Runtime environment (`development`, `staging`, `production`) |
| `LOG_LEVEL` | String | `info` | Minimum log severity level (`debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` | String | `json` | Structured output format (`json` or `text`) |
| `JWT_SECRET` | String | *(dev key)* | Shared HMAC-SHA256 signing secret for token verification |
| `DB_HOST` | String | `localhost` | PostgreSQL host |
| `DB_PORT` | Integer | `5432` | PostgreSQL port |
| `DB_USER` | String | `adroit` | PostgreSQL user |
| `DB_PASSWORD` | String | `adroit_secret` | PostgreSQL password |
| `DB_NAME` | String | `adroit_db` | PostgreSQL database name |
| `NATS_URL` | String | `nats://localhost:4222` | NATS JetStream server address (falls back to InMemBus if unreachable) |
| `BASE_DOMAIN` | String | `adroit.run` | Platform base domain for generated service URLs |
| `ORCHESTRATOR_DRIVER_TYPE` | String | `simulated` | Driver mode: `simulated` (local dev) or `nomad` (live cluster) |
| `NOMAD_ADDR` | String | *(empty)* | HTTP address of Nomad cluster (e.g. `http://127.0.0.1:4646`) |
| `NOMAD_TOKEN` | String | *(empty)* | Nomad ACL secret token |
| `NOMAD_DATACENTER` | String | `dc1` | Target Nomad cluster datacenter |
| `VAULT_ADDR` | String | `http://127.0.0.1:8200` | HashiCorp Vault server address |
| `VAULT_TOKEN` | String | `root` | HashiCorp Vault authentication token |
| `VAULT_MOUNT` | String | `secret` | Vault KV v2 engine mount path |

---

## 3. How to Run

### Standalone (Direct)
```bash
# Run with default simulated driver
go run ./cmd/orchestration-service

# Run targeting a live Nomad cluster and Vault instance
NOMAD_ADDR=http://127.0.0.1:4646 ORCHESTRATOR_DRIVER_TYPE=nomad VAULT_ADDR=http://127.0.0.1:8200 go run ./cmd/orchestration-service
```

### Supervised via Multi-Service Dev Runner
```bash
go run ./cmd/adroit-dev
```

### Build Binary
```bash
# Compile standalone executable
go build -o bin/orchestration-service ./cmd/orchestration-service

# Run compiled binary
./bin/orchestration-service
```

---

## 4. Health Check Probe

Verify that the microservice is operational:

```bash
curl http://localhost:8087/healthz
```

Expected Response:
```json
{
  "service": "orchestration-service",
  "status": "healthy"
}
```

---

## 5. Related Documentation

* **Architecture & API Specs**: [internal/orchestration-service/README.md](../../internal/orchestration-service/README.md)
* **Job Generator & Driver**: [pkg/orchestrator/README.md](../../pkg/orchestrator/README.md)
* **OpenAPI Specification**: [docs/openapi.yaml](../../docs/openapi.yaml)
* **Platform Overview**: [docs/README.md](../../docs/README.md)
