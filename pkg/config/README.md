# Config Package (`pkg/config`)

The `config` package provides 12-factor application configuration loading from environment variables with fallback defaults and type conversion.

---

## 1. Functions & API Reference

* `config.GetEnv(key, defaultValue string) string`  
  Retrieves a string environment variable, falling back to `defaultValue` if unset or empty.
* `config.GetEnvInt(key string, defaultValue int) int`  
  Retrieves an integer environment variable, parsing and falling back if invalid or unset.
* `config.GetEnvBool(key string, defaultValue bool) bool`  
  Retrieves a boolean environment variable (`true`, `false`, `1`, `0`).
* `config.GetEnvDuration(key string, defaultValue time.Duration) time.Duration`  
  Retrieves a duration environment variable (e.g. `10s`, `15m`, `1h`).
* `config.LoadBaseConfig(defaultPort int) BaseConfig`  
  Loads standard platform configs (`ADROIT_ENV`, `PORT`, `ADROIT_LOG_LEVEL`, `ADROIT_LOG_FORMAT`).

---

## 2. How to Test

```bash
go test -v ./pkg/config
```
