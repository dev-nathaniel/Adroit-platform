# Migration Runner CLI (`cmd/migrate`)

The `migrate` CLI is the standalone, production-ready database migration engine for the Adroit Platform. It packages embedded SQL migrations into a single, self-contained binary built upon `golang-migrate`.

---

## 1. Features

* **Embedded Migrations:** All `.sql` files from `deployments/migrations` are embedded directly into the binary at compile time via `io/fs`. No runtime external path dependencies.
* **Advisory Locking:** Acquires PostgreSQL advisory locks during execution to ensure only one migration runner can modify schema at any moment.
* **Dirty State Detection & Recovery:** Detects half-executed migrations and offers recovery via the `force` command.
* **Rollback Support:** Supports rolling back individual migration steps or entire schemas (`migrate down [steps|all]`).
* **CI/CD & Orchestrator Ready:** Emits non-zero exit codes on failure for automatic deployment halting in Nomad or CI/CD pipelines.

---

## 2. CLI Commands

| Command | Arguments | Description | Example |
| :--- | :--- | :--- | :--- |
| `up` | None | Applies all pending migrations. | `go run ./cmd/migrate up` |
| `down` | `[steps]` or `all` | Reverts migration step(s). Defaults to 1 step. | `go run ./cmd/migrate down 1` |
| `version` | None | Displays the active schema version and dirty flag. | `go run ./cmd/migrate version` |
| `force` | `<version>` | Overrides schema version and clears dirty state after a failure. | `go run ./cmd/migrate force 1` |
| `help` | None | Shows help and usage instructions. | `go run ./cmd/migrate help` |

---

## 3. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `DB_HOST` | string | `localhost` | PostgreSQL host. |
| `DB_PORT` | int | `5432` | PostgreSQL port. |
| `DB_USER` | string | `adroit` | Database user (requires DDL permissions). |
| `DB_PASSWORD` | string | `adroit_secret` | Database password. |
| `DB_NAME` | string | `adroit_platform` | Database name. |
| `DB_SSLMODE` | string | `disable` | SSL mode (`disable`, `require`, `verify-full`). |
| `DATABASE_URL` | string | `""` | Optional direct DSN connection URI. Overrides individual DB variables. |

Alternatively, pass `--dsn` directly:
```bash
go run ./cmd/migrate --dsn "postgres://user:pass@host:5432/dbname?sslmode=disable" up
```

---

## 4. How to Run Locally

### Option A: Standalone CLI
Use the CLI when you want fine-grained control to apply, inspect, or rollback migrations:
```bash
# Apply pending migrations
go run ./cmd/migrate up

# Check status & clean/dirty state
go run ./cmd/migrate version

# Rollback one migration
go run ./cmd/migrate down 1

# Revert all migrations
go run ./cmd/migrate down all

# Recover from a dirty migration
go run ./cmd/migrate force 1
```

### Option B: Automatic Local Dev via `cmd/adroit-dev`
If you are developing locally with the full platform stack, you don't even need to run `cmd/migrate` manually. 
Starting the dev runner automatically checks PostgreSQL and applies pending migrations before starting the microservices:
```bash
go run ./cmd/adroit-dev
```

---

## 5. Production Packaging & Deployment

In production environments (where Go is not installed on worker nodes), migrations are containerized and executed as an isolated pre-deployment batch job before services roll out.

### Step 1: Build & Push Container (`deployments/Dockerfile.migrate`)
Compile the self-contained migration container image:
```bash
# Build the image
docker build -t adroit/migrate:latest -f deployments/Dockerfile.migrate .

# Push to your container registry
docker push adroit/migrate:latest
```

### Step 2: Run via HashiCorp Nomad (`deployments/nomad/adroit-migrate.nomad.hcl`)
Run the one-off batch job on your production cluster:
```bash
# Submit the batch migration job
nomad job run deployments/nomad/adroit-migrate.nomad.hcl

# Verify success (status: complete, exit code: 0)
nomad job status adroit-migrate
```
*Note: Because this job is defined with `type = "batch"`, Nomad runs it once to completion. If it fails, Nomad stops and alerts, preventing faulty deployments before microservices are replaced.*

### Step 3: Or Run Standalone in CI/CD (GitHub Actions / GitLab CI)
```bash
docker run --rm \
  -e DB_HOST="postgres.production.internal" \
  -e DB_PORT="5432" \
  -e DB_USER="adroit_migrator" \
  -e DB_PASSWORD="$PROD_DB_PASSWORD" \
  -e DB_NAME="adroit_platform" \
  -e DB_SSLMODE="require" \
  adroit/migrate:latest up
```

