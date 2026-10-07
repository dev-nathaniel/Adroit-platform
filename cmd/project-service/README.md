# Adroit Project & Service Catalog CLI Entrypoint (`cmd/project-service`)

> **Binary:** `bin/project-service` (or `project-service.exe` on Windows)  
> **Default Port:** `8084`  
> **Source Package:** `github.com/dev-nathaniel/Adroit-platform/cmd/project-service`

---

## Overview

`cmd/project-service` is the standalone microservice daemon for the **Project & Service Catalog**. It connects to PostgreSQL, NATS JetStream, HashiCorp Vault, and object storage (Local filesystem, AWS S3, Cloudflare R2, or MinIO). It binds to port `8084`, exposes health probes at `/healthz`, serves the `/api/v1/projects` and `/api/v1/services` APIs, handles source archive uploads (`.zip` / `.tar.gz`), and powers workloads across Git, Archive, and Pre-built Container Image source types.

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
PORT=8084 DB_HOST=localhost NATS_URL=nats://localhost:4222 STORAGE_BACKEND=local STORAGE_LOCAL_DIR=./data/storage go run ./cmd/project-service
```

