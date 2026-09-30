# Git Integration Service Binary (`cmd/git-integration-service`)

Executable entry point for the Adroit Git Integration Service.

## Port Assignment
* Default Port: `8085`

## Environment Variables
* `PORT` - HTTP listening port (default: `8085`)
* `JWT_SECRET` - Shared secret for validating JWT bearer tokens
* `GITHUB_WEBHOOK_SECRET` - Optional platform-wide GitHub webhook secret
* `GITLAB_WEBHOOK_SECRET` - Optional platform-wide GitLab webhook secret
* `PLATFORM_URL` - External URL of the platform (e.g., `http://localhost:8080`)
* `DATABASE_URL` / `DB_*` - PostgreSQL database connection configuration
* `NATS_URL` - NATS JetStream server connection string (default: `nats://localhost:4222`)

## Running Locally
```bash
go run ./cmd/git-integration-service
```
