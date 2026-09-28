# Database Package (`pkg/database`)

The `database` package manages pooled PostgreSQL connections using `pgx/v5/stdlib` and runs SQL migrations.

---

## 1. Features

* **High Performance Pooling:** Configurable `MaxOpenConns`, `MaxIdleConns`, and `ConnMaxLifetime`.
* **Standard DSN Builder:** Formats connection URIs with SSL modes.
* **Migration Runner:** Executes `.sql` migration files to bootstrap database schemas.
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

## 3. How to Test

```bash
go test -v ./pkg/database
```
