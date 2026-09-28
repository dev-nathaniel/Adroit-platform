# User & Organization Service (`cmd/user-service`)

The **User Service** manages platform user profiles, multi-tenant organizations (workspaces), team memberships, and Role-Based Access Control (RBAC).

---

## 1. Responsibilities

* **User Profiles:** Read and update personal account details (display name, avatar URL).
* **Multi-Tenant Workspaces (Organizations):** Create and list organizations with role mappings.
* **Team Memberships & RBAC:** Invite and assign roles (`owner`, `admin`, `member`) to teammates within an organization.
* **Authorization Checks:** Enforces that only owners and admins can invite new teammates, and ensures private workspace access.

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | int | `8082` | HTTP port. |
| `ADROIT_ENV` | string | `development` | Environment (`development`, `staging`, `production`). |
| `JWT_SECRET` | string | *(dev key)* | Shared secret key for JWT verification. |
| `DB_HOST` | string | `localhost` | PostgreSQL host. |
| `DB_PORT` | int | `5432` | PostgreSQL port. |
| `DB_USER` | string | `adroit` | PostgreSQL user. |
| `DB_PASSWORD` | string | `adroit_secret` | PostgreSQL password. |
| `DB_NAME` | string | `adroit_platform` | PostgreSQL database name. |

---

## 3. How to Run Locally

```bash
go run ./cmd/user-service/main.go
```
The server will start listening on `http://0.0.0.0:8082`.

---

## 4. How to Test

### Run Unit & Integration Tests
```bash
go test -v ./internal/user-service
```

### Run with Coverage
```bash
go test -cover ./internal/user-service
```

---

## 5. Endpoints & Route Reference

All routes require a valid JWT bearer token: `Authorization: Bearer <TOKEN>` (except `/healthz`).

| Method | Endpoint | Description | Roles Required |
| :--- | :--- | :--- | :--- |
| `GET` | `/healthz` | Health check probe | Public |
| `GET` | `/api/v1/users/profile` | Fetch caller profile | Authenticated |
| `PATCH` | `/api/v1/users/profile` | Update profile (name, avatar) | Authenticated |
| `GET` | `/api/v1/users/organizations` | List organizations user belongs to | Authenticated |
| `POST` | `/api/v1/users/organizations` | Create a new organization workspace | Authenticated |
| `GET` | `/api/v1/users/organizations/{orgID}` | Get workspace details | Org Member |
| `POST` | `/api/v1/users/organizations/{orgID}/members` | Add teammate to organization | Org Owner or Admin |
| `GET` | `/api/v1/users/organizations/{orgID}/members` | List members of organization | Org Member |

### Example cURL Commands

#### 1. Update Profile
```bash
curl -X PATCH http://localhost:8082/api/v1/users/profile \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"name": "Senior Cloud Engineer"}'
```

#### 2. Create Workspace
```bash
curl -X POST http://localhost:8082/api/v1/users/organizations \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"name": "DevOps Engineering"}'
```

#### 3. Invite Team Member
```bash
curl -X POST http://localhost:8082/api/v1/users/organizations/<ORG_ID>/members \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"email": "colleague@adroit.run", "role": "admin"}'
```
