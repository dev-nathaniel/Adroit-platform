# Auth Service (`cmd/auth-service`)

The **Auth Service** is the identity provider and token issuance authority for the Adroit Platform. It manages user accounts, password hashing, JWT signing, credential validation, and automatic default workspace provisioning.

---

## 1. Responsibilities

* **User Registration:** Creates accounts with bcrypt password hashing (cost factor 12) and provisions a personal default workspace.
* **Authentication & Login:** Validates credentials and signs HS256 JWT tokens.
* **GitHub OAuth Integration:** Supports browser redirects, OAuth callbacks, CSRF protection, and direct code exchange for SPAs/CLI. Automatically provisions workspaces for new GitHub accounts and links existing accounts seamlessly.
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
| `GITHUB_CLIENT_ID` | string | `""` | GitHub OAuth application Client ID. |
| `GITHUB_CLIENT_SECRET` | string | `""` | GitHub OAuth application Client Secret. |
| `GITHUB_REDIRECT_URI` | string | `http://localhost:8080/api/v1/auth/github/callback` | OAuth redirect callback URI registered in GitHub. |
| `GITHUB_FRONTEND_URL` | string | `http://localhost:3000/auth/callback` | Frontend destination to redirect browser with JWT token. |
| `DB_HOST` | string | `localhost` | PostgreSQL host. |
| `DB_PORT` | int | `5432` | PostgreSQL port. |
| `DB_USER` | string | `adroit` | PostgreSQL user. |
| `DB_PASSWORD` | string | `adroit_secret` | PostgreSQL password. |
| `DB_NAME` | string | `adroit_platform` | PostgreSQL database name. |

---

## 3. GitHub OAuth Setup Guide (Step-by-Step)

To enable GitHub login in development or production, you must register an OAuth application on GitHub to obtain a **Client ID** and **Client Secret**.

### Step 1: Create a GitHub OAuth App
1. Log into your GitHub account and navigate to:
   **[GitHub Developer Settings > OAuth Apps](https://github.com/settings/developers)**
2. Click the **"New OAuth App"** button (or **"Register a new application"**).
3. Fill in the application registration form with the following values:
   * **Application name:** `Adroit Platform (Local Dev)` (or your preferred name)
   * **Homepage URL:** `http://localhost:8080` (or `http://localhost:3000` if using a frontend app)
   * **Application description:** `Authentication provider for Adroit Platform` (optional)
   * **Authorization callback URL:**
     ```
     http://localhost:8080/api/v1/auth/github/callback
     ```
     > **Note:** The **Authorization callback URL** must match `GITHUB_REDIRECT_URI` exactly. Because all local requests pass through the API Gateway on port `8080`, use port `8080` rather than the direct Auth Service port `8081`.

4. Click **"Register application"**.

---

### Step 2: Obtain Client ID and Generate Client Secret
1. On the app summary page, copy the **Client ID** (e.g. `Iv1.xxxxxxxxxxxx` or `Ov23...`).
2. Under **Client secrets**, click **"Generate a new client secret"**.
3. Confirm your GitHub password or 2FA prompt.
4. Immediately copy the **Client Secret** (it will only be displayed once).

---

### Step 3: Configure Environment Variables
Set the credentials in your local `.env` file or export them in your shell:

```bash
# GitHub OAuth Configuration
GITHUB_CLIENT_ID=your_github_client_id_here
GITHUB_CLIENT_SECRET=your_github_client_secret_here
GITHUB_REDIRECT_URI=http://localhost:8080/api/v1/auth/github/callback
GITHUB_FRONTEND_URL=http://localhost:3000/auth/callback
```

* **`GITHUB_REDIRECT_URI`**: The callback URL where GitHub sends the authorization code.
* **`GITHUB_FRONTEND_URL`**: Optional frontend redirect target. When configured, browser login redirects to `${GITHUB_FRONTEND_URL}?token=<jwt>`. If empty, the callback returns the raw JSON payload.

---

### Step 4: Scopes & Permissions Requested
The platform requests the following scopes:
* **`read:user`**: Reads public user profile data (name, username, avatar URL).
* **`user:email`**: Reads verified email addresses. This is critical because users with private email settings on GitHub return `email: null` from `/user`. The service automatically queries `/user/emails` to resolve their primary verified address.

---

### Step 5: Test the Flow
1. **Initiate Login via Browser:**
   Open:
   ```
   http://localhost:8080/api/v1/auth/github
   ```
   You will be redirected to GitHub to authorize the app. Once authorized, GitHub redirects to `/api/v1/auth/github/callback` with a temporary code.
2. **Fetch URL via JSON (for SPAs/Frontends):**
   ```bash
   curl http://localhost:8080/api/v1/auth/github/url
   ```
3. **Direct Exchange (for CLI / Mobile / SPA):**
   ```bash
   curl -X POST http://localhost:8080/api/v1/auth/github \
     -H "Content-Type: application/json" \
     -d '{"code": "<GITHUB_TEMPORARY_CODE>"}'
   ```

---

## 4. How to Run Locally

```bash
go run ./cmd/auth-service/main.go
```
The server will start listening on `http://0.0.0.0:8081`.

---

## 5. How to Test

### Run Unit & Integration Tests
```bash
go test -v ./internal/auth-service
```

### Run with Coverage
```bash
go test -cover ./internal/auth-service
```

---

## 6. Endpoints & Route Reference

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `GET` | `/healthz` | Health check probe | No |
| `POST` | `/api/v1/auth/register` | Register a new user and create default organization | No |
| `POST` | `/api/v1/auth/login` | Authenticate user and issue JWT token | No |
| `GET` | `/api/v1/auth/github` | Redirects browser to GitHub OAuth authorization | No |
| `GET` | `/api/v1/auth/github/url` | Returns GitHub authorization URL as JSON (for SPAs/CLI) | No |
| `GET` | `/api/v1/auth/github/callback` | GitHub OAuth callback ingress | No |
| `POST` | `/api/v1/auth/github` | Direct authorization code exchange for JWT session | No |
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
