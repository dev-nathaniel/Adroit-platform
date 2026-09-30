# Notification Service Internal Package (`internal/notification-service`)

> **Package:** Core business logic, template rendering engines, email/webhook providers, and NATS JetStream event consumers for Adroit Platform communications.

---

## 1. Package Structure

```
internal/notification-service/
├── models.go           # EmailMessage, WebhookMessage, SendEmailRequest, SendWebhookRequest
├── templates.go        # HTML/Text template manager (org_invitation, welcome, alert)
├── providers.go        # EmailProvider (Console, SMTP) & HTTPWebhookSender (HMAC & backoff)
├── service.go          # Core Service coordinating templates, email sending, and webhooks
├── consumer.go         # NATS JetStream & InMem subscriber processing domain events
├── handler.go          # Chi HTTP controller endpoints (/healthz, /email, /webhook, /test-invite)
├── service_test.go     # Comprehensive unit tests for templates, emails, webhooks, and events
└── README.md           # This architecture documentation
```

---

## 2. Core Components

### 2.1 `TemplateManager`
Pre-compiles HTML and plain-text templates with responsive, dark-mode CSS styling matching Adroit's UI design system.

- **`org_invitation`**: Features gradient headers, inviter details, organization name, role badge, primary "Accept Invitation" CTA button linking to `/invite?token=...`, expiration reminder, and plaintext fallback.
- **`welcome`**: Onboarding email for newly registered developers.
- **`alert`**: High-priority notifications for failed builds, crashing services, or quota warnings.

### 2.2 Outbound Providers
- **`ConsoleEmailProvider`**: Captures messages in-memory and renders styled ASCII boxes in the terminal for rapid local development.
- **`SMTPEmailProvider`**: Generates proper MIME `multipart/alternative` payloads with UTF-8 encoding and plain/HTML parts. Supports standard TLS (port 465) and STARTTLS (port 587).
- **`HTTPWebhookSender`**: Sends outbound HTTP POST payloads with exponential retry (up to 3 attempts with 200ms initial backoff). Automatically computes `X-Adroit-Signature: sha256=<hex>` using HMAC-SHA256 when a secret is configured.

### 2.3 `Consumer`
Subscribes to NATS JetStream events:
- Handles `adroit.org.member_invited`: parses `OrgMemberInvitedPayload`, renders the invite template, resolves absolute URLs, and dispatches the email.
- Handles `adroit.notification.email_send` & `adroit.notification.webhook_send`.

---

## 3. Event Payloads

### `OrgMemberInvitedPayload`
```json
{
  "invitation_id": "inv_01J8Y...",
  "organization_id": "org_01J8X...",
  "organization_name": "Acme Cloud",
  "email": "developer@example.com",
  "role": "admin",
  "invited_by_id": "usr_01J8W...",
  "invited_by_name": "Jane Doe",
  "invited_by_email": "jane@acme.com",
  "invite_url": "http://localhost:3000/invite?token=...",
  "expires_at": "2026-10-07T00:00:00Z"
}
```

---

## 4. Running Unit Tests

```bash
go test -v ./internal/notification-service/...
```
All unit tests run fully in-memory with zero external network dependencies.
