# ==============================================================================
# Adroit Platform — Nomad Autoscaler Daemon Configuration
# ==============================================================================

nomad {
  address = "http://adroit-nomad:4646"
}

http {
  bind_address = "0.0.0.0"
}

telemetry {
  prometheus_metrics = true
}

policy {
  default_cooldown            = "30s"
  default_evaluation_interval = "15s"
}
