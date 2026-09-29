# ==============================================================================
# Adroit Platform — Production Database Migration Batch Job (Nomad)
#
# Runs migrations as an isolated one-off batch task before service rolling updates.
# Exit code 0 indicates all migrations succeeded; any non-zero exit aborts deploy.
# ==============================================================================

job "adroit-migrate" {
  datacenters = ["dc1"]
  type        = "batch"

  group "migration" {
    count = 1

    # Batch jobs do not restart automatically on failure
    restart {
      attempts = 0
      mode     = "fail"
    }

    reschedule {
      attempts = 0
    }

    task "run-migrations" {
      driver = "docker"

      config {
        image   = "adroit/migrate:latest"
        command = "up"
      }

      env {
        DB_HOST    = "postgres.service.consul"
        DB_PORT    = "5432"
        DB_NAME    = "adroit_platform"
        DB_USER    = "adroit_migrator"
        DB_SSLMODE = "require"
      }

      # Dynamic credentials from Vault (if integrated) or injected secrets
      template {
        data = <<EOH
{{- with secret "secret/data/adroit/database" -}}
DB_PASSWORD="{{ .Data.data.password }}"
{{- end -}}
EOH
        destination = "secrets/db.env"
        env         = true
      }

      resources {
        cpu    = 256
        memory = 256
      }
    }
  }
}
