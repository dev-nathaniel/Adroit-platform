# Adroit Platform API Documentation & Tooling

This directory contains standardized API documentation and testing collections for the **Adroit Multi-Tenant Cloud Platform-as-a-Service (PaaS)**.

---

## 1. Files Included

| File | Type | Description |
| :--- | :--- | :--- |
| [`openapi.yaml`](./openapi.yaml) | OpenAPI 3.0.3 | Full specification covering all API Gateway, Auth Service, and User/Workspace endpoints, request/response models, and error envelopes. |
| [`adroit-platform.postman_collection.json`](./adroit-platform.postman_collection.json) | Postman Collection v2.1.0 | Ready-to-import Postman collection with dynamic token extraction, environment variable chaining, and cURL requests. |

---

## 2. Using the Postman Collection

1. Open **Postman** (or Bruno / Insomnia / Thunder Client).
2. Click **Import** in the top-left corner.
3. Select or drag-and-drop `docs/adroit-platform.postman_collection.json`.
4. **Token Automation is Pre-configured:**
   - Execute **`2. Authentication -> Register User`** or **`Login User`**.
   - The embedded test script automatically extracts the issued JWT token into the collection variable `{{token}}`.
   - All protected requests in the collection (`/me`, `/profile`, `/organizations`, `/invitations`) automatically inherit the token via Bearer authorization.
   - Creating an organization or invitation also automatically populates `{{orgId}}`, `{{inviteToken}}`, and `{{inviteId}}`.

---

## 3. Viewing the OpenAPI Specification

### Option A: VS Code / IDE Extensions
Install the **OpenAPI (Swagger) Editor** or **Swagger Viewer** extension in VS Code:
- Open `docs/openapi.yaml`
- Press `Shift + Alt + E` (or right-click -> *OpenAPI: Preview*) to render the interactive Swagger UI.

### Option B: Docker Swagger UI
Run a local interactive Swagger UI container pointing to `openapi.yaml`:
```bash
docker run -p 8088:8080 -e SWAGGER_JSON=/spec/openapi.yaml -v ${PWD}/docs:/spec swaggerapi/swagger-ui
```
Then visit `http://localhost:8088` in your browser.

### Option C: Online Swagger Editor
Copy and paste the contents of `docs/openapi.yaml` into [editor.swagger.io](https://editor.swagger.io).

---

## 4. Endpoints Overview

All routes are dispatched through the API Gateway at `http://localhost:8080`:

* **System:** `GET /healthz`, `GET /`, `GET /api/v1/status`
* **Auth:** `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `GET /api/v1/auth/me`
* **Profiles:** `GET /api/v1/users/profile`, `PATCH /api/v1/users/profile`
* **Organizations:** `GET /api/v1/users/organizations`, `POST /api/v1/users/organizations`, `GET /api/v1/users/organizations/:orgID`
* **Members:** `GET /api/v1/users/organizations/:orgID/members`, `POST /api/v1/users/organizations/:orgID/members`
* **Invitations:**
  - `POST /api/v1/users/organizations/:orgID/invitations` (Create invitation)
  - `GET /api/v1/users/organizations/:orgID/invitations` (List invitations)
  - `DELETE /api/v1/users/organizations/:orgID/invitations/:inviteID` (Revoke invitation)
  - `GET /api/v1/users/invitations/:token` (Public preview invitation)
  - `POST /api/v1/users/invitations/:token/accept` (Accept invitation & join workspace)
