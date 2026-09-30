# Notification Service CLI (`cmd/notification-service`)

> **Entrypoint:** Microservice executable for transactional emails, webhooks, and event-driven alerting across the Adroit Cloud Platform.

---

## 1. Overview

The **Notification Service** acts as the centralized communications hub for the Adroit Platform. It receives events emitted across the **NATS JetStream** message backbone (e.g. member invitations, build failures, deployment readiness, quota warnings) and dispatches formatted communications via:

1. **Email Providers:**
   - **Console / Mock Provider (Default in Dev):** Formats emails with rich ASCII tables and terminal logging for instant local feedback without needing an external mail server.
   - **SMTP Provider (Production / Staging):** Standard Go `net/smtp` integration supporting TLS, STARTTLS, and direct SMTPS for Mailpit, SendGrid, Amazon SES, Postmark, and Mailgun.
2. **Webhook Dispatcher:**
   - Outbound HTTP POST requests with exponential backoff retries and cryptographic `X-Adroit-Signature` (HMAC-SHA256) validation.
3. **HTTP Diagnostic & Direct Dispatch API:**
   - Exposes REST endpoints on Chi router for health checks, template previews, and direct programmatic dispatch.

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | `int` | `8083` | HTTP listening port for health checks and API |
| `ADROIT_ENV` | `string` | `development` | Deployment environment (`development`, `staging`, `production`) |
| `ADROIT_LOG_LEVEL` | `string` | `debug` | Structured log level (`debug`, `info`, `warn`, `error`) |
| `ADROIT_LOG_FORMAT`| `string` | `json` | Structured log format (`json` or `text`) |
| `NATS_URL` | `string` | `nats://localhost:4222` | NATS JetStream server connection URL |
| `NOTIFICATION_PROVIDER` | `string` | `console` | Outbound email provider (`console` or `smtp`) |
| `SMTP_HOST` | `string` | `localhost` | Outbound SMTP host (e.g. `smtp.sendgrid.net` or `localhost`) |
| `SMTP_PORT` | `int` | `1025` | SMTP port (`1025` for Mailpit dev, `587` for STARTTLS, `465` for TLS) |
| `SMTP_USER` | `string` | `""` | SMTP authentication username |
| `SMTP_PASSWORD` | `string` | `""` | SMTP authentication password / API key |
| `SMTP_FROM` | `string` | `Adroit Platform <no-reply@adroit.run>` | Default RFC 5322 From address |
| `SMTP_SKIP_VERIFY` | `bool` | `false` | Insecurely skip TLS certificate verification |
| `APP_URL` | `string` | `http://localhost:3000` | Platform frontend base URL for invite and dashboard links |

---

## 3. How to Run Locally

### Standalone Service
```bash
# Run with Console email preview (no SMTP server needed)
go run ./cmd/notification-service

# Run with custom port or SMTP Mailpit
PORT=8083 NOTIFICATION_PROVIDER=smtp SMTP_HOST=localhost SMTP_PORT=1025 go run ./cmd/notification-service
```

### Via All-in-One Dev Supervisor
`cmd/adroit-dev` automatically compiles and supervises `notification-service` on port `8083`:
```bash
go run ./cmd/adroit-dev
```

---

## 4. Running Tests

```bash
# Execute unit tests with verbose output
go test -v ./internal/notification-service/...

# Execute with race detection
go test -race ./internal/notification-service/...

# Check code coverage
go test -cover ./internal/notification-service/...
```

---

## 5. API Route Table

All routes are mounted directly or through the API Gateway at `/api/v1/notifications/*`:

| Method | Path | Auth | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/healthz` | Public | Service health & active email/webhook provider names |
| `POST` | `/api/v1/notifications/email` | Internal / Gateway | Direct programmatic email dispatch |
| `POST` | `/api/v1/notifications/webhook` | Internal / Gateway | Direct webhook dispatch with optional HMAC secret |
| `POST` | `/api/v1/notifications/test-invite` | Public / Dev | Triggers sample org invitation email rendering |

---

## 6. Architecture & Event Subscriptions

The service automatically subscribes to the following **NATS JetStream** subjects:

- **`adroit.org.member_invited`**: Dispatches branded organization invitation email with CTA button, token link, and 7-day expiration details.
- **`adroit.notification.email_send`**: Dispatches general transactional emails using pre-compiled templates or raw HTML/text.
- **`adroit.notification.webhook_send`**: Delivers signed HTTP POST webhooks with exponential retry.
