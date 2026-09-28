# Auth Service (`cmd/auth-service`)

The **Auth Service** is the identity provider and token issuance authority for the Adroit Platform. It manages user accounts, password hashing, JWT signing, credential validation, and automatic default workspace provisioning.

---

## 1. Responsibilities

* **User Registration:** Creates accounts with bcrypt password hashing (cost factor 12) and provisions a personal default workspace.
* **Authentication & Login:** Validates credentials and signs HS256 JWT tokens.
* **Session Profile (`/me`):** Decodes and returns the caller profile based on the JWT Bearer token.
* **Security & Throttling:** Protected with token-bucket rate limiting (20 req/sec, burst 40) to mitigate brute force attacks.
* **Storage Resiliency:** Connects to PostgreSQL, with automatic fallback to in-memory storage during standalone testing or when the DB is offline.

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | int | `8081` | HTTP server port. |
| `ADROIT_ENV` | string | `development` | Environment (`development`, `staging`, `production`). |
| `JWT_SECRET` | string | *(dev key)* | HMAC secret key used to sign and verify JWT tokens. |
| `JWT_EXPIRATION` | duration | `24h` | Token validity duration. |
| `DB_HOST` | string | `localhost` | PostgreSQL host. |
| `DB_PORT` | int | `5432` | PostgreSQL port. |
| `DB_USER` | string | `adroit` | PostgreSQL user. |
| `DB_PASSWORD` | string | `adroit_secret` | PostgreSQL password. |
| `DB_NAME` | string | `adroit_platform` | PostgreSQL database name. |

---

## 3. How to Run Locally

```bash
go run ./cmd/auth-service/main.go
```
The server will start listening on `http://0.0.0.0:8081`.

---

## 4. How to Test

### Run Unit & Integration Tests
```bash
go test -v ./internal/auth-service
```

### Run with Coverage
```bash
go test -cover ./internal/auth-service
```

---

## 5. Endpoints & Route Reference

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `GET` | `/healthz` | Health check probe | No |
| `POST` | `/api/v1/auth/register` | Register a new user and create default organization | No |
| `POST` | `/api/v1/auth/login` | Authenticate user and issue JWT token | No |
| `GET` | `/api/v1/auth/me` | Fetch authenticated user profile | Yes (`Bearer <token>`) |

### Example cURL Commands

#### 1. Register User
```bash
curl -X POST http://localhost:8081/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "dev@adroit.run", "name": "John Doe", "password": "StrongPassword123!"}'
```

#### 2. Login User
```bash
curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "dev@adroit.run", "password": "StrongPassword123!"}'
```

#### 3. Get Current User (`/me`)
```bash
curl http://localhost:8081/api/v1/auth/me \
  -H "Authorization: Bearer <TOKEN>"
```
