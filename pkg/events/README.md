# Events & Message Bus (`pkg/events`)

> **Package:** Universal domain events, CloudEvents-style envelope definitions, and NATS JetStream / In-Memory pub-sub event bus abstractions for the Adroit Platform.

---

## 1. Overview

`pkg/events` provides the standardized event-driven messaging infrastructure used across all Adroit microservices. It features:

- **Universal Event Envelope (`Event[T]`):** CloudEvents-like typed envelope containing unique ID, subject, source, timestamp, and typed payload data.
- **Unified EventBus Interface:** Combines `Publisher` and `Subscriber` capabilities.
- **NATS JetStream Bus (`NATSBus`):** Production-ready connection to NATS JetStream, automatically creating/verifying the `ADROIT_EVENTS` stream with subject wildcard `adroit.>`.
- **In-Memory Event Bus (`InMemBus`):** Thread-safe, non-blocking asynchronous pub-sub bus with wildcard support (`adroit.>` and exact matching), used for zero-dependency unit tests and offline development fallback.
- **Noop Publisher (`NoopPublisher`):** Safe, nil-safe dummy publisher.

---

## 2. Subject Hierarchy

| Subject | Description | Emitted By |
| :--- | :--- | :--- |
| `adroit.org.member_invited` | New member invited to an organization | `user-service` |
| `adroit.org.member_joined` | Member accepted invite & joined org | `user-service` |
| `adroit.user.registered` | New developer account registered | `auth-service` |
| `adroit.notification.email_send` | Request transactional email delivery | Any service |
| `adroit.notification.webhook_send` | Request outbound webhook delivery | Any service |
| `adroit.build.requested` | Build trigger lifecycle initiated | `build-service` |
| `adroit.build.completed` | Build artifact produced | `build-service` |
| `adroit.deploy.requested` | Deployment requested on Nomad | `orchestration-service` |
| `adroit.deploy.healthy` | Health checks confirmed readiness | `runtime-service` |
| `adroit.database.provisioned` | Managed database provisioned | `managed-database-service` |

---

## 3. Usage Example

```go
// Publishing an event
bus := events.NewInMemBus() // or events.NewNATSBus(...)
payload := events.OrgMemberInvitedPayload{
    OrganizationName: "Acme Corp",
    Email:            "dev@acme.com",
    InviteURL:        "https://adroit.run/invite?token=...",
}
_ = bus.Publish(ctx, events.SubjectOrgMemberInvited, payload)

// Subscribing to an event
_ = bus.Subscribe(events.SubjectOrgMemberInvited, func(ctx context.Context, subject string, raw []byte) error {
    var p events.OrgMemberInvitedPayload
    _ = json.Unmarshal(raw, &p)
    fmt.Printf("Invited: %s to %s\n", p.Email, p.OrganizationName)
    return nil
})
```

---

## 4. Testing

```bash
go test -v ./pkg/events/...
```
