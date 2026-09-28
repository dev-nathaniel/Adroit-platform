# Adroit Platform Architecture Specification

> **Platform Type:** Multi-Tenant Cloud Platform-as-a-Service (PaaS)  
> **Inspiration:** Render, Fly.io, Vercel, Netlify  
> **Language & Core Stack:** Go (Golang), HashiCorp Nomad & Consul, Vault, Caddy, PostgreSQL (Patroni), Redis, NATS, BuildKit, Harbor, Firecracker / gVisor / Kata, Ceph

---

## 1. High-Level Architecture Overview

Adroit Platform is an enterprise-ready, developer-centric PaaS designed for automated building, isolated container/microVM deployment, managed databases, dynamic routing, and observability.

```
                              [ Clients ]
               Dashboard | Mobile App | Website | Visitors
                                   │
                                   ▼
                         [ External Load Balancer ]
                                   │
         ┌─────────────────────────┴─────────────────────────┐
         │                                                   │
  (Management Traffic)                               (Application Traffic)
         │                                                   │
         ▼                                                   ▼
┌──────────────────┐                               ┌──────────────────┐
│   Edge Node      │                               │   Edge Node      │
│  - Caddy         │                               │  - Caddy         │
│  - Consul Client │                               │  - Consul Client │
│  - Nomad Client  │                               │  - Nomad Client  │
└────────┬─────────┘                               └────────┬─────────┘
         │                                                   │
         ▼                                                   ▼
┌──────────────────┐                               ┌──────────────────┐
│  PaaS Core Node  │                               │   Runtime Node   │
│  - API Gateway   │                               │  - Consul Envoy  │
│  - Microservices │                               │  - User App Side │
└────────┬─────────┘                               │  - User App      │
         │                                         │  - Firecracker/  │
         │ (Events via NATS / RPC via Consul Mesh) │    gVisor/Kata   │
         ▼                                         └──────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│                       Control Plane Cluster                         │
│       Nomad Server  |  Consul Server  |  Vault Server  |  Raft      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. Infrastructure Node Topology

### 2.1 Ingress & Edge Nodes
* **Components:** Caddy (Edge Web Server & Reverse Proxy), Consul Client, Nomad Client.
* **Responsibilities:**
  * TLS termination with automatic ACME (Let's Encrypt / ZeroSSL) for custom domains and platform subdomains (`*.adroit.run`).
  * Edge routing: routing API/Dashboard traffic to PaaS Core, and end-user web traffic directly to isolated application runtimes via Consul service discovery.
  * DDoS mitigation, rate limiting, and HTTP/3 support.

### 2.2 Control Plane (3–5 Nodes with Integrated Raft)
* **Consul Server Cluster:** Service registry, service mesh (Consul Connect mTLS), decentralized KV configuration, and distributed locks.
* **Nomad Server Cluster:** High-performance workload scheduler for user apps, microservices, and transient build batch jobs.
* **Vault Server:** Secure management of database credentials, customer secrets, GitHub OAuth tokens, and mTLS certificates.
* **Consensus:** Integrated Raft consensus backend spanning the control plane nodes.

### 2.3 PaaS Core Services Node
Hosts the API Gateway and the core platform microservices:

| Service | Responsibility |
| :--- | :--- |
| **API Gateway** | Unified authentication, request routing, rate limiting, and client protocol translation. |
| **User Service** | User accounts, organizations, workspace memberships, teams, and RBAC permissions. |
| **Auth Service** | Identity provider, JWT issuance, OAuth2 / GitHub login, API token generation. |
| **Project Service** | Projects, environments (production, preview, staging), services configuration, and environment variables. |
| **Build Service** | Build trigger lifecycle, BuildKit job scheduling, Dockerfile & buildpack generation, build log streaming. |
| **Git Integration Service** | GitHub/GitLab webhook processing, repo cloning tokens, branch tracking, and commit commit hash monitoring. |
| **Runtime Service** | Application lifecycle management, scaling instances (min/max replicas), health-check status tracking. |
| **Orchestration Service** | Translates deployment states into Nomad Job specifications (`.nomad.hcl`), submits allocations, handles rollouts (blue-green / canary). |
| **Managed Database Service** | Provisioning and lifecycle of managed databases (Postgres, Redis, MySQL), connection string generation, and backups. |
| **DNS Service** | Custom domain verification (CNAME/TXT), DNS record propagation checks, and Caddy ingress dynamic route synchronization. |
| **Billing Service** | Usage metering (vCPU-hours, RAM-hours, bandwidth, build minutes), Stripe integration, plan tiers, and quota enforcement. |
| **Docker Registry Service** | Management of internal registry credentials, repository mappings, and image access control. |
| **Logging Service** | Centralized aggregation of platform service logs and real-time streaming of user app container logs (via WebSockets / SSE). |
| **Notification Service** | Webhooks, Slack/Discord alerts, email notifications (build failures, deployment successes, quota alerts). |

### 2.4 Build Nodes
* **Components:** Consul Client, Nomad Client, **BuildKit** daemon, transient Build Batch Jobs.
* **Execution:** Nomad executes isolated batch jobs running `buildctl` or rootless BuildKit to securely clone source code, execute multi-stage builds, cache layers, and push final images to the Registry Node.

### 2.5 Registry Nodes
* **Components:** Harbor container registry, Trivy/Clair vulnerability scanner, Cosign image signing, Consul & Nomad clients.
* **Responsibilities:** Securely store built container images, scan for CVEs prior to deployment, and verify image signatures before runtime execution.

### 2.6 Runtime Nodes
* **Components:** Consul Client, Nomad Client, Consul Connect Envoy sidecars, Application Health Check sidecars.
* **Isolation Runtimes:**
  * **Firecracker microVMs** (high security, multi-tenant kernel isolation).
  * **Kata Containers / gVisor** (syscall sandboxing).
* **Node Pools / Tiers:**
  * `general`: Standard CPU/memory workloads.
  * `isolated`: Dedicated hardware isolation for enterprise tenants.
  * `high-memory`: RAM-intensive applications (caching, data processing).
  * `gpu`: AI / ML inference and acceleration workloads.

### 2.7 Database Node (Stateful Tier)
* **Components:** PostgreSQL, Patroni (High Availability clustering with distributed consensus), Consul Client, Nomad Client.
* **Responsibilities:** Stores relational state for platform metadata, accounts, builds, deployments, and billing records.

### 2.8 Messaging & Event Node
* **Components:** **NATS** with **JetStream** persistence, Consul Client, Nomad Client.
* **Responsibilities:** Event-driven pub/sub backbone between microservices (e.g., `git.push` $\rightarrow$ `build.triggered` $\rightarrow$ `build.completed` $\rightarrow$ `deploy.requested` $\rightarrow$ `deploy.healthy`).

### 2.9 Cache & Session Node
* **Components:** **Redis Cluster**, Consul Client, Nomad Client.
* **Responsibilities:** Ephemeral session storage, API Gateway rate-limit tracking, fast KV lookups, and real-time pub/sub caching.

### 2.10 Storage Node (Ceph)
* **Components:** **Ceph Distributed Storage** (RadosGW, RBD, CephFS), Consul Client, Nomad Client.
* **Responsibilities:** Distributed object and block storage for application persistent volumes, build cache storage, automated backups of PostgreSQL, Redis, and Harbor registry layers.

---

## 3. Communication Patterns

1. **Synchronous Ingress:** Clients $\rightarrow$ External LB $\rightarrow$ Caddy $\rightarrow$ API Gateway $\rightarrow$ Internal Services via REST / JSON.
2. **Asynchronous Orchestration:** Services publish domain events to **NATS JetStream** (`adroit.events.*`). Subscribed services process events with at-least-once delivery guarantees.
3. **Internal Service Discovery & Mesh:** Services discover peers and establish mutual TLS via **Consul Connect Envoy** sidecars.
4. **Secret Retrieval:** Services authenticate to **Vault** using Nomad workload identities to retrieve dynamic secrets and database credentials.
