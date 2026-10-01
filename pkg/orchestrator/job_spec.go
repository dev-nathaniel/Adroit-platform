package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// JobConfig defines all parameters required to generate a Nomad job specification
type JobConfig struct {
	DeploymentID string
	ServiceID    string
	ServiceName  string
	ServiceSlug  string
	OrgSlug      string
	ProjectSlug  string
	ImageTag     string
	Port         int
	Replicas     int
	Resources    types.ResourceLimits
	HealthCheck     types.HealthCheckConfig
	EnvVars         map[string]string
	VaultSecretPath string
	RuntimeTier     types.RuntimeTier
	Strategy        types.RolloutStrategy
	Datacenter      string
}

// GenerateJobSpec produces an authentic, production-grade Nomad HCL2 job specification (.nomad.hcl)
func GenerateJobSpec(cfg JobConfig) (string, error) {
	if cfg.ServiceID == "" {
		return "", fmt.Errorf("service_id is required")
	}
	if cfg.ImageTag == "" {
		return "", fmt.Errorf("image_tag is required")
	}

	dc := cfg.Datacenter
	if dc == "" {
		dc = "dc1"
	}

	replicas := cfg.Replicas
	if replicas <= 0 {
		replicas = 1
	}

	port := cfg.Port
	if port <= 0 {
		port = 8080
	}

	strategy := cfg.Strategy
	if strategy == "" {
		strategy = types.RolloutStrategyRolling
	}

	// Calculate CPU MHz: 1.0 CPUCore = 1000 MHz (Nomad resource unit)
	cpuMHz := int(cfg.Resources.CPUCores * 1000)
	if cpuMHz < 128 {
		cpuMHz = 256
	}

	memoryMB := cfg.Resources.MemoryMB
	if memoryMB < 64 {
		memoryMB = 512
	}

	// Health check values with sensible defaults
	hcPath := cfg.HealthCheck.Path
	if hcPath == "" {
		hcPath = "/healthz"
	}
	if !strings.HasPrefix(hcPath, "/") {
		hcPath = "/" + hcPath
	}

	hcInterval := cfg.HealthCheck.IntervalSeconds
	if hcInterval <= 0 {
		hcInterval = 10
	}

	hcTimeout := cfg.HealthCheck.TimeoutSeconds
	if hcTimeout <= 0 {
		hcTimeout = 3
	}

	hcInitialDelay := cfg.HealthCheck.InitialDelaySeconds
	if hcInitialDelay <= 0 {
		hcInitialDelay = 5
	}

	jobID := GenerateJobID(cfg.ServiceID)
	serviceName := sanitizeConsulName(cfg.ServiceSlug)
	if serviceName == "" {
		serviceName = fmt.Sprintf("svc-%s", cfg.ServiceID[:8])
	}

	hostRouting := fmt.Sprintf("%s.adroit.run", cfg.ServiceSlug)
	if cfg.OrgSlug != "" {
		hostRouting = fmt.Sprintf("%s-%s.adroit.run", cfg.ServiceSlug, cfg.OrgSlug)
	}

	// Select task driver based on runtime tier
	driver := "docker"
	var driverConfigExtra string
	switch cfg.RuntimeTier {
	case types.RuntimeTierIsolated:
		// MicroVM or gVisor sandboxed execution
		driver = "docker"
		driverConfigExtra = `
        runtime = "runsc" # gVisor syscall isolation`
	case types.RuntimeTierHighMemory:
		driver = "docker"
	case types.RuntimeTierGPU:
		driver = "docker"
	default:
		driver = "docker"
	}

	// Render Update Stanza according to Strategy
	var updateStanza string
	switch strategy {
	case types.RolloutStrategyCanary:
		updateStanza = fmt.Sprintf(`  update {
    max_parallel      = 1
    min_healthy_time  = "10s"
    healthy_deadline  = "5m"
    progress_deadline = "10m"
    auto_revert       = true
    auto_promote      = false
    canary            = 1
  }`)
	case types.RolloutStrategyBlueGreen:
		updateStanza = fmt.Sprintf(`  update {
    max_parallel      = %d
    min_healthy_time  = "10s"
    healthy_deadline  = "5m"
    progress_deadline = "10m"
    auto_revert       = true
    auto_promote      = false
    canary            = %d
  }`, replicas, replicas)
	default: // Rolling
		updateStanza = fmt.Sprintf(`  update {
    max_parallel      = 1
    min_healthy_time  = "10s"
    healthy_deadline  = "3m"
    progress_deadline = "5m"
    auto_revert       = true
    auto_promote      = false
    canary            = 0
  }`)
	}

	// Deterministic sorted Environment Variables block
	var envBlock strings.Builder
	if len(cfg.EnvVars) > 0 {
		keys := make([]string, 0, len(cfg.EnvVars))
		for k := range cfg.EnvVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			envBlock.WriteString(fmt.Sprintf("        %s = %q\n", k, cfg.EnvVars[k]))
		}
	}

	// Optional Nomad template stanza for HashiCorp Vault secrets integration
	var templateStanza string
	if cfg.VaultSecretPath != "" {
		templateStanza = fmt.Sprintf(`
      template {
        data = <<EOH
{{ with secret %q }}
{{ range $k, $v := .Data.data }}
{{ $k }}="{{ $v }}"
{{ end }}
{{ end }}
EOH
        destination = "secrets/app.env"
        env         = true
      }
`, cfg.VaultSecretPath)
	}

	// Build full HCL spec
	hcl := fmt.Sprintf(`# ==============================================================================
# Adroit Platform — Production Workload Job Specification (Nomad)
# Service: %s (%s)
# Deployment ID: %s | Strategy: %s
# ==============================================================================

job %q {
  datacenters = [%q]
  type        = "service"

  meta {
    "adroit.platform"    = "adroit"
    "adroit.service_id"  = %q
    "adroit.deploy_id"   = %q
    "adroit.strategy"    = %q
    "adroit.tier"        = %q
  }

%s

  group "app" {
    count = %d

    restart {
      attempts = 3
      interval = "2m"
      delay    = "15s"
      mode     = "fail"
    }

    reschedule {
      delay          = "30s"
      delay_function = "exponential"
      max_delay      = "1m"
      unlimited      = true
    }

    network {
      port "http" {
        to = %d
      }
    }

    service {
      name = %q
      port = "http"
      tags = [
        "adroit",
        "service-id=%s",
        "deploy-id=%s",
        "traefik.enable=true",
        "traefik.http.routers.%s.rule=Host(%q)",
        "urlprefix-/%s",
      ]

      check {
        name     = "health-check"
        type     = "http"
        path     = %q
        interval = "%ds"
        timeout  = "%ds"
        initial_status = "passing"
        check_restart {
          limit           = 3
          grace           = "%ds"
          ignore_warnings = false
        }
      }

      connect {
        sidecar_service {}
      }
    }

    task "server" {
      driver = %q

      config {
        image = %q
        ports = ["http"]%s
      }

      env {
        PORT            = "%d"
        ADROIT_DEPLOY_ID = %q
        ADROIT_SERVICE_ID = %q
%s      }
%s
      resources {
        cpu    = %d
        memory = %d
      }
    }
  }
}
`,
		cfg.ServiceName, cfg.ServiceID,
		cfg.DeploymentID, strategy,
		jobID, dc,
		cfg.ServiceID, cfg.DeploymentID, strategy, cfg.RuntimeTier,
		updateStanza,
		replicas,
		port,
		serviceName,
		cfg.ServiceID,
		cfg.DeploymentID,
		serviceName, hostRouting,
		serviceName,
		hcPath, hcInterval, hcTimeout, hcInitialDelay,
		driver,
		cfg.ImageTag, driverConfigExtra,
		port, cfg.DeploymentID, cfg.ServiceID,
		envBlock.String(),
		templateStanza,
		cpuMHz, memoryMB,
	)

	return hcl, nil
}

// GenerateJobID generates the deterministic Nomad job ID for a service
func GenerateJobID(serviceID string) string {
	return fmt.Sprintf("adroit-svc-%s", serviceID)
}

func sanitizeConsulName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var sb strings.Builder
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			sb.WriteRune(ch)
		} else if ch == '_' || ch == ' ' {
			sb.WriteRune('-')
		}
	}
	return sb.String()
}
