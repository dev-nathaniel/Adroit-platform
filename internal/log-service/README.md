# Adroit Application Log Streaming Service (`log-service`)

The **Application Log Streaming Service** provides centralized ingestion, storage, historical search, and real-time streaming (via Server-Sent Events and WebSockets) of container runtime logs from HashiCorp Nomad allocations across all microservices and user applications.

---

## 1. System Architecture & Observability Flow

```
            [ Developer Dashboard / CLI Client ]
                             │
         ┌───────────────────┴───────────────────┐
         │                                       │
         ▼ (SSE: /stream)                        ▼ (WS: /ws)
┌────────────────────────────────────────────────────────┐
│             API Gateway (:8080)                        │
└────────────────────────────┬───────────────────────────┘
                             │
                             ▼
┌────────────────────────────────────────────────────────┐
│             Log Streaming Service (:8089)              │
│   ┌──────────────────┐          ┌──────────────────┐   │
│   │   Broadcaster    │◄─────────┤   Log Ingest     │   │
│   │ (In-Memory Hub)  │          │ (Agents/Sidecars)│   │
│   └────────┬─────────┘          └────────┬─────────┘   │
└────────────┼─────────────────────────────┼─────────────┘
             │                             │
    ┌────────┴────────┐           ┌────────┴────────┐
    ▼                 ▼           ▼                 ▼
[ SSE Clients ]  [ WS Clients ] [ PostgreSQL ] [ Nomad Agent ]
(:keep-alive)    (ping/pong)    (app_logs table) (:4646 /fs/logs)
```

---

## 2. Key Capabilities

1. **Dual Real-Time Streaming Protocols**:
   - **Server-Sent Events (SSE)** (`GET /api/v1/services/{id}/logs/stream`):
     - Unidirectional, ultra-lightweight HTTP streaming with standard `text/event-stream` framing.
     - Automatic reconnect and keep-alive heartbeats (`: keep-alive\n\n`) every 15s to bypass reverse proxy timeouts.
     - Immediate historical backlog playback (`?tail=50`) before transitioning smoothly into live logs.
   - **WebSockets** (`GET /api/v1/services/{id}/logs/ws`):
     - Bidirectional real-time socket connections with ping/pong control frames.
     - Direct browser and CLI watcher integration.

2. **Pluggable Allocation Tailing Engine**:
   - **NomadLogTailer**: Connects to the HashiCorp Nomad agent HTTP API (`/v1/client/fs/logs/{alloc_id}?task=server&type=stdout&follow=true`) to stream raw allocation container output.
   - **SimulatedLogTailer**: In-memory simulation generator for hermetic testing and local development when running without a Nomad cluster.

3. **High-Performance In-Memory Broadcaster**:
   - Thread-safe pub-sub hub (`Broadcaster`) delivering zero-latency fan-out to all active subscribers.
   - Non-blocking drops for slow consumers to guarantee ingestion throughput is never stalled.

4. **Historical Search & Filter API**:
   - Filter logs by stream (`stdout`, `stderr`, `system`, or `all`).
   - Case-insensitive substring searching (`?search=error`).
   - Scoped queries by `deployment_id` and `allocation_id`.
   - Chronological ordering and limit/offset pagination.

5. **Lifecycle Event Tracking (NATS JetStream)**:
   - Subscribes to `adroit.deploy.started`, `adroit.deploy.healthy`, `adroit.deploy.failed`, and `adroit.deploy.stopped`.
   - Automatically injects platform system logs into the service log stream so developers can observe startup readiness transitions.

---

## 3. Configuration & Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `PORT` | `8089` | HTTP and streaming port. |
| `DATABASE_URL` | `postgres://adroit:adroit_dev_pwd@localhost:5432/adroit_dev?sslmode=disable` | PostgreSQL connection string. |
| `NOMAD_ADDR` | `http://localhost:4646` | HashiCorp Nomad cluster address. |
| `NOMAD_TOKEN` | `""` | Nomad ACL secret token (optional). |
| `LOG_TAILER_DRIVER` | `auto` | Tailer mode: `nomad`, `simulated`, or `auto` (auto-detects Nomad leader). |
| `NATS_URL` | `nats://localhost:4222` | NATS JetStream server address. |
| `JWT_SECRET` | *(Standard Dev Key)* | HMAC-SHA256 JWT validation secret. |

---

## 4. Route Table

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :---: |
| `GET` | `/healthz` | Service health status probe | No |
| `POST` | `/api/v1/services/{id}/logs` | Ingest batch log records for a service | No (Agent / Internal) |
| `POST` | `/api/v1/logs/ingest` | Global batch log ingestion endpoint | No (Agent / Internal) |
| `GET` | `/api/v1/services/{id}/logs` | Query historical logs with search & filters | **Yes** (Bearer or `?token=`) |
| `GET` | `/api/v1/deployments/{id}/logs`| Query historical logs for a deployment | **Yes** (Bearer or `?token=`) |
| `GET` | `/api/v1/services/{id}/logs/stream` | Stream real-time logs via Server-Sent Events (SSE) | **Yes** (Bearer or `?token=`) |
| `GET` | `/api/v1/deployments/{id}/logs/stream` | Stream deployment logs via Server-Sent Events (SSE) | **Yes** (Bearer or `?token=`) |
| `GET` | `/api/v1/services/{id}/logs/ws` | Stream real-time logs via WebSocket | **Yes** (Bearer or `?token=`) |
| `DELETE`| `/api/v1/services/{id}/logs` | Purge historical logs before specified cutoff timestamp | **Yes** (Bearer or `?token=`) |

---

## 5. Running Tests

```bash
# Run unit tests
go test -v ./internal/log-service/...
```
