# Adroit Git Integration Service (`git-integration-service`)

The **Git Integration Service** provides first-class support for **GitHub** and **GitLab** version control systems within the Adroit platform. It powers continuous deployment by ingesting and cryptographically verifying VCS webhooks, matching push events to registered microservices, scheduling container builds in the database, and dispatching event-driven build triggers across the NATS JetStream event bus.

---

## 1. Core Architecture

```
  ┌─────────────────────────────────────────────────────────────┐
  │         External Git Providers (GitHub & GitLab)            │
  └──────────────┬───────────────────────────────┬──────────────┘
                 │                               │
       GitHub Webhook Push               GitLab Webhook Push
   (X-Hub-Signature-256 HMAC)             (X-Gitlab-Token)
                 │                               │
                 ▼                               ▼
  ┌─────────────────────────────────────────────────────────────┐
  │                 Adroit API Gateway (:8080)                  │
  │     /api/v1/webhooks/github    |    /api/v1/webhooks/gitlab │
  └──────────────────────────────┬──────────────────────────────┘
                                 │
                                 ▼
  ┌─────────────────────────────────────────────────────────────┐
  │           Git Integration Service (Port 8085)               │
  │                                                             │
  │  1. Cryptographic Signature & Secret Token Verification     │
  │  2. Canonical URL Normalization (HTTPS, SSH, git://)        │
  │  3. Multi-Tenant Service & Branch Matching                 │
  │  4. Transactional Build Record Creation (builds table)      │
  │  5. Audit Log Persistence (git_webhook_events table)        │
  │  6. Remote Provider REST API Integration (Repos & Branches) │
  └──────────────┬───────────────────────────────┬──────────────┘
                 │                               │
                 ▼                               ▼
       PostgreSQL Database               NATS JetStream Backbone
    (builds, git_providers,              Subject: adroit.build.requested
      git_webhook_events)                Subject: adroit.git.push_received
                                                 │
                                                 ▼
                                           Build Service
                                        (BuildKit Runner)
```

---

## 2. API Endpoints

### 2.1 Public Webhook Ingress (Cryptographically Authenticated)

| Method | Endpoint | Description | Verification Header |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/webhooks/github` | Ingests GitHub push & ping webhooks | `X-Hub-Signature-256` (HMAC-SHA256) |
| `POST` | `/api/v1/webhooks/gitlab` | Ingests GitLab push & system webhooks | `X-Gitlab-Token` (Secret token) |

### 2.2 Provider Account Connections (Requires JWT)

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/git/providers` | List connected Git providers for current user |
| `POST` | `/api/v1/git/providers/connect` | Link a GitHub or GitLab personal access token or OAuth token |
| `DELETE` | `/api/v1/git/providers/{provider}` | Unlink a GitHub or GitLab provider connection |

### 2.3 Repository & Branch Discovery (Requires JWT)

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/git/{provider}/repos` | Discover user/organization repositories (`?page=1&per_page=30`) |
| `GET` | `/api/v1/git/{provider}/repos/{owner}/{repo}/branches` | List branches for a selected repository |

### 2.4 Service Webhook Configuration (Requires JWT)

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `POST` | `/api/v1/git/services/{id}/webhook` | Generate secret & auto-register webhook on GitHub/GitLab |
| `GET` | `/api/v1/git/services/{id}/webhook` | Fetch webhook instructions and payload URL for manual setup |
| `GET` | `/api/v1/git/webhooks/events` | Audit trail of incoming webhook deliveries and triggered builds |

---

## 3. Cryptographic Signature Verification

### 3.1 GitHub Webhooks
GitHub computes an HMAC-SHA256 signature using the configured secret and transmits it in the `X-Hub-Signature-256` header formatted as:
```
X-Hub-Signature-256: sha256=<hex_digest>
```
The service computes the HMAC using `crypto/hmac` and validates the signature in constant time using `crypto/subtle.ConstantTimeCompare`.

### 3.2 GitLab Webhooks
GitLab transmits the webhook secret token directly in the `X-Gitlab-Token` header. The service performs constant-time string comparison to prevent timing attacks.

---

## 4. URL Normalization Engine

Different developers and Git clients configure repository URLs differently. The normalizer handles:
* HTTPS: `https://github.com/org/repo.git` $\rightarrow$ `github.com/org/repo`
* HTTPS (no suffix): `https://github.com/org/repo` $\rightarrow$ `github.com/org/repo`
* SSH SCP: `git@github.com:org/repo.git` $\rightarrow$ `github.com/org/repo`
* SSH Protocol: `ssh://git@gitlab.com/group/subgroup/project.git` $\rightarrow$ `gitlab.com/group/subgroup/project`
* Case insensitive host matching and `.git` suffix stripping.

---

## 5. Event-Driven Coordination (NATS JetStream)

When a push webhook matches a registered service:
1. A new record is inserted into the `builds` table with `status = "queued"` and `triggered_by = "webhook:<provider>"`.
2. Emits `adroit.build.requested` (`SubjectBuildRequested`) to notify the **Build Service**:
   ```json
   {
     "build_id": "bld_9c4e...",
     "service_id": "svc_1234...",
     "project_id": "prj_5678...",
     "repo_url": "https://github.com/adroit-platform/core-api",
     "branch": "main",
     "commit_hash": "7c3a4f6e1b2d8c90",
     "dockerfile_path": "Dockerfile"
   }
   ```
3. Emits `adroit.git.push_received` (`SubjectGitPushReceived`) with commit, author, matched services, and triggered build IDs for telemetry and notification dispatch.
