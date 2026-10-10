# Adroit CLI (`adroit`)

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![CLI Framework](https://img.shields.io/badge/CLI-Cobra_%2B_Charm-00CA8E)](https://github.com/spf13/cobra)

The **Adroit Developer CLI** (`adroit`) is the official client-side command-line tool for the Adroit Cloud Platform-as-a-Service (PaaS). It provides an intuitive, high-velocity developer experience for deploying web applications, tailing live container logs, managing HashiCorp Vault secrets, scaling instances, and configuring custom domains.

---

## Installation & Building

```bash
# Build the binary
go build -o bin/adroit.exe ./cmd/adroit

# Move to your PATH or run directly
./bin/adroit.exe version
```

---

## Core Command Taxonomy

### 1. Authentication & Context
```bash
# Interactive login (email & password prompt)
adroit auth login

# Register a new account and organization workspace
adroit auth register

# Verify current identity and target API gateway
adroit auth whoami

# Clear stored session credentials
adroit auth logout
```

### 2. Project Initialization
```bash
# Auto-detect application runtime (Node.js, Go, Python, Dockerfile, etc.)
# and create an adroit.yaml configuration file
adroit init
```

### 3. Deployments & Workloads
```bash
# Package working directory into an in-memory tarball, upload to Adroit,
# trigger BuildKit / Nixpacks compilation, and follow Nomad rollout
adroit deploy .

# Deploy with custom strategy and replica count
adroit deploy . --strategy canary --replicas 3

# View live workloads and allocations
adroit ps

# Inspect detailed service metadata
adroit service info [service-id]
```

### 4. Real-Time Observability
```bash
# Stream live stdout/stderr container logs via Server-Sent Events (SSE)
adroit logs [service-id] -f

# Backfill historical logs
adroit logs [service-id] --tail 100 --stream stderr
```

### 5. Secrets & Environment Variables (HashiCorp Vault)
```bash
# List variables (Vault secrets masked as ••••••••)
adroit env list [service-id]

# Set a standard runtime variable
adroit env set [service-id] PORT=8080 NODE_ENV=production

# Store a sensitive token securely in HashiCorp Vault KV v2
adroit env set [service-id] API_KEY=secret123 --secret

# Unmask and reveal a Vault secret value
adroit env reveal [service-id] API_KEY
```

### 6. Dynamic Scaling & Rollbacks
```bash
# Scale service instances up or down
adroit scale [service-id] --replicas 5

# Instantly revert to a previous deployment release
adroit rollback [deployment-id]
```

### 7. Custom Domains & Ingress
```bash
# Add a custom domain
adroit domain add [service-id] api.example.com

# Trigger DNS CNAME/TXT challenge verification
adroit domain verify [domain-id]
```

---

## Global Flags

| Flag | Description | Default |
| :--- | :--- | :--- |
| `--api-url` | Override API Gateway target URL | `http://localhost:8080` |
| `--token` | Explicit JWT / Personal Access Token | Stored in `~/.adroit/config.yaml` |
| `--org` | Target Organization / Workspace ID | Active context org |
| `--json` | Output machine-readable JSON (ideal for CI/CD and scripts) | `false` |
| `-q, --quiet` | Suppress banners and decorative output | `false` |
| `-v, --verbose` | Enable debug request/response logging | `false` |
| `--no-color` | Disable ANSI terminal escape codes | `false` |

---

## Configuration Files

- **Global User Config:** `~/.adroit/config.yaml` (profile contexts, API tokens, default workspaces).
- **Project Config:** `adroit.yaml` (local service binding, build ignore lists, rollout strategies).
