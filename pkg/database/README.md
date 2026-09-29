# Database Package (`pkg/database`)

The `database` package manages pooled PostgreSQL connections using `pgx/v5/stdlib` and runs SQL migrations.

---

## 1. Features

* **High Performance Pooling:** Configurable `MaxOpenConns`, `MaxIdleConns`, and `ConnMaxLifetime`.
* **Standard DSN Builder:** Formats connection URIs with SSL modes.
* **Migration Engine (`MigrateUp`, `MigrateDown`, `MigrateVersion`, `MigrateForce`):** Powered by `golang-migrate` with embedded filesystem (`io/fs`) drivers and PostgreSQL advisory locking.
* **Health Check:** `PingContext` with timeout ensures database availability.

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `DB_HOST` | string | `localhost` | PostgreSQL host. |
| `DB_PORT` | int | `5432` | PostgreSQL port. |
| `DB_USER` | string | `adroit` | Database username. |
| `DB_PASSWORD` | string | `adroit_secret` | Database password. |
| `DB_NAME` | string | `adroit_platform` | Database name. |
| `DB_SSLMODE` | string | `disable` | SSL mode (`disable`, `require`, `verify-full`). |
| `DB_MAX_OPEN_CONNS` | int | `25` | Maximum active open connections. |
| `DB_MAX_IDLE_CONNS` | int | `10` | Maximum idle connections in pool. |
| `DB_CONN_MAX_LIFETIME` | duration | `5m` | Maximum connection reuse duration. |

---

## 3. Database Migrations

The platform uses `golang-migrate` with embedded `.sql` scripts located in `deployments/migrations`.

### Programmatic API:
```go
import (
    "github.com/dev-nathaniel/Adroit-platform/deployments/migrations"
    "github.com/dev-nathaniel/Adroit-platform/pkg/database"
)

// Apply all pending migrations
err := database.MigrateUp(dsn, migrations.FS)

// Roll back 1 step (or pass 0 to roll back all)
err := database.MigrateDown(dsn, migrations.FS, 1)

// Query active version
version, dirty, err := database.MigrateVersion(dsn, migrations.FS)

// Clear dirty state / force version
err := database.MigrateForce(dsn, migrations.FS, 1)
```

### CLI Commands:
For standalone execution, use the dedicated CLI runner in [`cmd/migrate`](../../cmd/migrate/README.md):
```bash
# Run all pending migrations
go run ./cmd/migrate up

# Check current version and clean/dirty state
go run ./cmd/migrate version

# Roll back 1 migration step
go run ./cmd/migrate down 1

# Reset dirty state or force a version
go run ./cmd/migrate force 1
```

---

## 4. How to Test

```bash
go test -v ./pkg/database
```

