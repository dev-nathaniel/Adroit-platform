datacenter = "dc1"
data_dir   = "/nomad/data"
bind_addr  = "0.0.0.0"

server {
  enabled          = true
  bootstrap_expect = 1
}

client {
  enabled           = true
  network_interface = "lo"
  artifact {
    disable_filesystem_isolation = true
  }
}

plugin "docker" {
  config {
    endpoint         = "unix:///var/run/docker.sock"
    allow_privileged = true
  }
}

consul {
  address = "adroit-consul:8500"
}

vault {
  enabled = true
  address = "http://adroit-vault:8200"
  token   = "root"
}

telemetry {
  publish_allocation_metrics = true
  publish_node_metrics       = true
  prometheus_metrics         = true
}

