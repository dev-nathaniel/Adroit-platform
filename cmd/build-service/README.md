# Adroit Build Service Entrypoint (`cmd/build-service`)

> **Binary:** `bin/build-service` (or `build-service.exe` on Windows)  
> **Default Port:** `8086`  
> **Source Package:** `github.com/dev-nathaniel/Adroit-platform/cmd/build-service`

---

## 1. Overview

`cmd/build-service` is the standalone microservice daemon for the **Build Service**. It coordinates container image builds (BuildKit / local emulation / Nomad batch jobs) from Git repositories or remote HTTP tarballs (`archive_url`), manages real-time build log streaming (SSE), and emits `adroit.deploy.requested` events upon build completion. Workloads deploying pre-built container images (`source_type: image`) bypass this daemon and deploy directly.

For full architecture details, see [internal/build-service/README.md](../../internal/build-service/README.md).

---

## 2. Environment Variables & Configuration

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | Integer | `8086` | HTTP server listening port |
| `ENV` | String | `development` | Runtime environment (`development`, `staging`, `production`) |
| `LOG_LEVEL` | String | `info` | Minimum log severity level (`debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` | String | `json` | Structured output format (`json` or `text`) |
| `JWT_SECRET` | String | *(dev key)* | Shared HMAC-SHA256 signing secret for token verification |
| `DB_HOST` | String | `localhost` | PostgreSQL host |
| `DB_PORT` | Integer | `5432` | PostgreSQL port |
| `DB_USER` | String | `adroit` | PostgreSQL user |
| `DB_PASSWORD` | String | `adroit_secret` | PostgreSQL password |
| `DB_NAME` | String | `adroit_db` | PostgreSQL database name |
| `NATS_URL` | String | `nats://localhost:4222` | NATS JetStream server address |
| `REGISTRY_HOST` | String | `registry.adroit.run` | Container registry host for tagging images |
| `BUILD_RUNNER_TYPE` | String | `local` | Builder engine: `local` (simulated BuildKit) or `nomad` (isolated batch task) |
| `NOMAD_ADDR` | String | *(empty)* | Nomad cluster address for batch builder runner |
| `NOMAD_DATACENTER` | String | `dc1` | Nomad datacenter for batch builder |

---

## 3. How to Run

### Standalone (Direct)
```bash
go run ./cmd/build-service
```

### Supervised via Multi-Service Dev Runner
```bash
go run ./cmd/adroit-dev
```

### Build Binary
```bash
go build -o bin/build-service ./cmd/build-service
./bin/build-service
```

---

## 4. Health Check Probe

```bash
curl http://localhost:8086/healthz
```
