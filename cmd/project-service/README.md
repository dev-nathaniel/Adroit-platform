# Adroit Project & Service Catalog CLI Entrypoint (`cmd/project-service`)

> **Binary:** `bin/project-service` (or `project-service.exe` on Windows)  
> **Default Port:** `8084`  
> **Source Package:** `github.com/dev-nathaniel/Adroit-platform/cmd/project-service`

---

## Overview

`cmd/project-service` is the standalone microservice daemon for the **Project & Service Catalog**. It connects to PostgreSQL and NATS JetStream, binds to port `8084`, exposes health probes at `/healthz`, and serves the `/api/v1/projects` and `/api/v1/services` APIs.

For full architecture details, route tables, and configuration options, see [internal/project-service/README.md](../../internal/project-service/README.md).

---

## Build and Run

### Build Binary
```bash
go build -o bin/project-service ./cmd/project-service
```

### Run Binary
```bash
./bin/project-service
```

### Run with Custom Configuration
```bash
PORT=8084 DB_HOST=localhost NATS_URL=nats://localhost:4222 go run ./cmd/project-service
```
