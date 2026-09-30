# Application Log Streaming Service CLI (`cmd/log-service`)

Binary entrypoint for the **Adroit Application Log Streaming Service**.

## Responsibilities
- Centralized ingestion of container and application stdout/stderr logs.
- Real-time pub-sub fan-out over Server-Sent Events (SSE) and WebSockets.
- Pluggable allocation tailer connecting to HashiCorp Nomad client log APIs.
- Historical log queries with keyword search, stream filtering, and pagination.

## Running Locally

```bash
# Start standalone (default port 8089)
go run ./cmd/log-service

# Or supervised via adroit-dev runner
go run ./cmd/adroit-dev
```

## Health Check Probe

```bash
curl http://localhost:8089/healthz
```
