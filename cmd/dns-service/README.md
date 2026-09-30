# Ingress & DNS Service CLI (`cmd/dns-service`)

Binary entrypoint for the **Adroit Ingress & DNS Service**.

## Running Locally

```bash
# Start standalone
go run ./cmd/dns-service

# Or supervised via adroit-dev runner
go run ./cmd/adroit-dev
```

## Health Check Probe

```bash
curl http://localhost:8088/healthz
```
