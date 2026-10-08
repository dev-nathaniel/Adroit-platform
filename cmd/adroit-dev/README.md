# Adroit Microservices Process Supervisor (`cmd/adroit-dev`)

The **Adroit Process Supervisor** is a cross-platform (Windows, macOS, Linux) local development orchestrator. Instead of re-implementing service logic, it builds and runs the **actual microservice binaries** (`cmd/auth-service`, `cmd/user-service`, `cmd/api-gateway`) as independent child processes and streams their tagged logs into a single unified terminal.

---

## 1. How It Works

```
                        [ adroit-dev supervisor ]
                                   │
         ┌─────────────────────────┼─────────────────────────┐
         │                         │                         │
         ▼                         ▼                         ▼
 [ bin/auth-service ]      [ bin/user-service ]      [ bin/api-gateway ]
       (:8081)                   (:8082)                   (:8080)
```

0. **Pre-flight Database Migrations:** Checks PostgreSQL availability and automatically applies any pending migrations (`golang-migrate`) using embedded SQL scripts before compiling services.
1. **Auto-Compiles Actual Code:** Builds `./cmd/auth-service`, `./cmd/user-service`, and `./cmd/api-gateway` to the local `bin/` directory.
2. **Spawns Child Processes:** Launches each microservice binary on its configured port.
3. **Pipes and Prefixes Logs:** Colors and prefixes every log line:
   * `[Auth Service]` (Green)
   * `[User Service]` (Yellow)
   * `[API Gateway]` (Cyan)
4. **Coordinated Graceful Shutdown:** Pressing `Ctrl+C` kills all child processes cleanly without leaving zombie or orphan processes running on your ports.

---

## 2. Supervised Microservices

| Service | Source Package | Port | Role |
| :--- | :--- | :--- | :--- |
| **Auth Service** | `./cmd/auth-service` | `8081` | Real standalone auth service process |
| **User Service** | `./cmd/user-service` | `8082` | Real standalone user & workspace service process |
| **Notification Svc** | `./cmd/notification-service` | `8083` | Email, webhooks, and transactional alert notifications |
| **Project Service** | `./cmd/project-service` | `8084` | Projects, environments, service definitions, secrets |
| **Git Svc** | `./cmd/git-integration-service` | `8085` | GitHub/GitLab webhook processing & repo integration |
| **Build Service** | `./cmd/build-service` | `8086` | BuildKit compilation triggers and log streaming |
| **Orchestration Svc** | `./cmd/orchestration-service` | `8087` | Nomad workload deployment, rollouts, allocations |
| **DNS & Ingress** | `./cmd/dns-service` | `8088` | Custom domains, ACME challenges, Caddy dynamic routes |
| **Log Service** | `./cmd/log-service` | `8089` | Real-time container log aggregation & WebSocket/SSE streaming |
| **Runtime Svc** | `./cmd/runtime-service` | `8090` | Elastic auto-scaling controller, live metrics & telemetry |
| **API Gateway** | `./cmd/api-gateway` | `8080` | Central ingress reverse proxy, JWT auth, rate limiting |

---

## 3. How to Run Locally

### Quick Run
```powershell
go run ./cmd/adroit-dev
```

### Or Build and Launch (Cross-Platform)
```powershell
go build -o bin/adroit-dev.exe ./cmd/adroit-dev
./bin/adroit-dev.exe
```

*(On macOS/Linux: `go build -o bin/adroit-dev ./cmd/adroit-dev && ./bin/adroit-dev`)*

#### Example Output:
```text
==========================================================================
       ADROIT PLATFORM — MICROSERVICES PROCESS SUPERVISOR               
==========================================================================
   * Auth Service     http://localhost:8081  (./cmd/auth-service)
   * User Service     http://localhost:8082  (./cmd/user-service)
   * API Gateway      http://localhost:8080  (./cmd/api-gateway)
==========================================================================
   -> PostgreSQL detected. Checking database migrations ... DONE (v1)
   Compiling services...
   -> Building Auth Service (./cmd/auth-service) ... DONE
   -> Building User Service (./cmd/user-service) ... DONE
   -> Building API Gateway (./cmd/api-gateway) ... DONE

   Starting all services...
   Press Ctrl+C to stop all services simultaneously.
--------------------------------------------------------------------------
[Auth Service] {"time":"...","level":"INFO","msg":"Starting Adroit Auth Service","port":8081}
[User Service] {"time":"...","level":"INFO","msg":"Starting Adroit User & Organization Service","port":8082}
[API Gateway] {"time":"...","level":"INFO","msg":"Starting Adroit API Gateway with Chi Router","port":8080}
[API Gateway] {"time":"...","level":"INFO","msg":"Mounted proxy for auth-service","target":"http://localhost:8081"}
[API Gateway] {"time":"...","level":"INFO","msg":"Mounted proxy for user-service","target":"http://localhost:8082"}
```

> **Prerequisite:** Start the local infrastructure first via `docker compose -f deployments/docker-compose.dev.yml up -d`. On startup, `adroit-dev` verifies PostgreSQL availability, runs all pending migrations from `deployments/migrations`, and only then launches the services. If PostgreSQL is offline, `adroit-dev` fails fast with an informative error rather than risking silent data loss.

---

## 4. Testing Through API Gateway

Once the supervisor is running, send all requests directly to **port 8080**:

### 1. Register a User
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "alex@adroit.run", "name": "Alex Platform", "password": "SecurePassword123!"}'
```

### 2. Login
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "alex@adroit.run", "password": "SecurePassword123!"}'
```

### 3. Check Profile
```bash
curl http://localhost:8080/api/v1/users/profile \
  -H "Authorization: Bearer <TOKEN>"
```

### 4. Create Workspace
```bash
curl -X POST http://localhost:8080/api/v1/users/organizations \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"name": "Production Workloads"}'
```
