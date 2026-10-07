package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func TestGenerateJobSpec(t *testing.T) {
	t.Run("Rolling Update Job Spec", func(t *testing.T) {
		cfg := JobConfig{
			DeploymentID: "dep-12345",
			ServiceID:    "svc-99999",
			ServiceName:  "Frontend Web",
			ServiceSlug:  "frontend-web",
			OrgSlug:      "acme-corp",
			ImageTag:     "registry.adroit.run/acme-corp/frontend-web:v1.0.0",
			Port:         3000,
			Replicas:     2,
			Resources: types.ResourceLimits{
				CPUCores: 1.5,
				MemoryMB: 1024,
			},
			HealthCheck: types.HealthCheckConfig{
				Path:                "/healthz",
				IntervalSeconds:     10,
				TimeoutSeconds:      2,
				InitialDelaySeconds: 5,
			},
			EnvVars: map[string]string{
				"NODE_ENV": "production",
				"API_HOST": "https://api.acme-corp.adroit.run",
			},
			RuntimeTier: types.RuntimeTierGeneral,
			Strategy:    types.RolloutStrategyRolling,
			Datacenter:  "dc1",
		}

		spec, err := GenerateJobSpec(cfg)
		if err != nil {
			t.Fatalf("unexpected error generating job spec: %v", err)
		}

		// Assert key elements in HCL output
		if !strings.Contains(spec, `job "adroit-svc-svc-99999"`) {
			t.Errorf("expected job name in spec, got:\n%s", spec)
		}
		if !strings.Contains(spec, `canary            = 0`) {
			t.Errorf("expected rolling update with canary=0 in spec")
		}
		if !strings.Contains(spec, `count = 2`) {
			t.Errorf("expected count = 2 in spec")
		}
		if !strings.Contains(spec, `cpu    = 1500`) {
			t.Errorf("expected cpu = 1500 in spec")
		}
		if !strings.Contains(spec, `memory = 1024`) {
			t.Errorf("expected memory = 1024 in spec")
		}
		if !strings.Contains(spec, `image = "registry.adroit.run/acme-corp/frontend-web:v1.0.0"`) {
			t.Errorf("expected image tag in spec")
		}
		if !strings.Contains(spec, `API_HOST = "https://api.acme-corp.adroit.run"`) {
			t.Errorf("expected env var in spec")
		}
		if !strings.Contains(spec, `traefik.http.routers.frontend-web.rule=Host(\"frontend-web-acme-corp.adroit.run\")`) {
			t.Errorf("expected host routing tag in spec")
		}
	})

	t.Run("Canary Update Job Spec", func(t *testing.T) {
		cfg := JobConfig{
			DeploymentID: "dep-canary-1",
			ServiceID:    "svc-backend",
			ServiceName:  "Backend API",
			ServiceSlug:  "backend",
			OrgSlug:      "acme",
			ImageTag:     "registry.adroit.run/acme/backend:v2.0.0",
			Port:         8080,
			Replicas:     3,
			Strategy:     types.RolloutStrategyCanary,
		}

		spec, err := GenerateJobSpec(cfg)
		if err != nil {
			t.Fatalf("unexpected error generating canary spec: %v", err)
		}

		if !strings.Contains(spec, `canary            = 1`) {
			t.Errorf("expected canary = 1 in update block")
		}
		if !strings.Contains(spec, `auto_promote      = false`) {
			t.Errorf("expected auto_promote = false in update block")
		}
	})

	t.Run("Isolated Runtime Tier (gVisor)", func(t *testing.T) {
		cfg := JobConfig{
			DeploymentID: "dep-iso-1",
			ServiceID:    "svc-secure",
			ServiceName:  "Secure Microservice",
			ServiceSlug:  "secure",
			ImageTag:     "registry.adroit.run/acme/secure:v1.0",
			RuntimeTier:  types.RuntimeTierIsolated,
		}

		spec, err := GenerateJobSpec(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(spec, `runtime = "runsc"`) {
			t.Errorf("expected runsc gVisor runtime in isolated tier")
		}
	})

	t.Run("Vault Secret Integration via Template Stanza", func(t *testing.T) {
		cfg := JobConfig{
			DeploymentID:    "dep-vault-1",
			ServiceID:       "svc-vault-app",
			ServiceName:     "Vault Integrated App",
			ServiceSlug:     "vault-app",
			ImageTag:        "registry.adroit.run/acme/app:v2.0",
			VaultSecretPath: "secret/data/adroit/org-123/deployments/dep-vault-1",
			EnvVars: map[string]string{
				"PUBLIC_ENV": "active",
			},
		}

		spec, err := GenerateJobSpec(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(spec, `template {`) {
			t.Errorf("expected template stanza for vault integration")
		}
		if !strings.Contains(spec, `with secret "secret/data/adroit/org-123/deployments/dep-vault-1"`) {
			t.Errorf("expected vault secret query in template")
		}
		if !strings.Contains(spec, `destination = "secrets/app.env"`) {
			t.Errorf("expected secrets/app.env destination in template")
		}
		if !strings.Contains(spec, `PUBLIC_ENV = "active"`) {
			t.Errorf("expected non-secret env var in env block")
		}
	})
}

func TestSimulatedDriver(t *testing.T) {
	ctx := context.Background()
	driver := NewSimulatedDriver(10 * time.Millisecond)

	t.Run("Rolling Deployment Lifecycle", func(t *testing.T) {
		cfg := JobConfig{
			DeploymentID: "dep-roll-1",
			ServiceID:    "svc-roll",
			ServiceName:  "Roll App",
			ServiceSlug:  "roll-app",
			ImageTag:     "registry.adroit.run/org/roll:v1",
			Port:         8080,
			Replicas:     2,
			Strategy:     types.RolloutStrategyRolling,
		}

		spec, err := GenerateJobSpec(cfg)
		if err != nil {
			t.Fatalf("failed to generate spec: %v", err)
		}

		res, err := driver.SubmitJob(ctx, spec)
		if err != nil {
			t.Fatalf("failed to submit job: %v", err)
		}
		if res.JobID != "adroit-svc-svc-roll" {
			t.Errorf("unexpected job ID: %s", res.JobID)
		}

		allocs, err := driver.GetAllocations(ctx, res.JobID)
		if err != nil {
			t.Fatalf("failed to get allocations: %v", err)
		}
		if len(allocs) != 2 {
			t.Fatalf("expected 2 allocations, got %d", len(allocs))
		}

		// Wait for health check delay to pass
		time.Sleep(25 * time.Millisecond)

		health, err := driver.CheckHealth(ctx, res.JobID)
		if err != nil {
			t.Fatalf("failed to check health: %v", err)
		}
		if !health.Healthy {
			t.Errorf("expected job to be healthy after delay, got: %+v", health)
		}
		if health.PassingCount != 2 {
			t.Errorf("expected 2 passing allocations, got %d", health.PassingCount)
		}
	})

	t.Run("Canary Deployment Lifecycle", func(t *testing.T) {
		cfg := JobConfig{
			DeploymentID: "dep-can-1",
			ServiceID:    "svc-canary",
			ServiceName:  "Canary App",
			ServiceSlug:  "canary-app",
			ImageTag:     "registry.adroit.run/org/canary:v1",
			Port:         8080,
			Replicas:     3,
			Strategy:     types.RolloutStrategyCanary,
		}

		spec, err := GenerateJobSpec(cfg)
		if err != nil {
			t.Fatalf("failed to generate spec: %v", err)
		}

		res, err := driver.SubmitJob(ctx, spec)
		if err != nil {
			t.Fatalf("failed to submit canary job: %v", err)
		}

		allocs, err := driver.GetAllocations(ctx, res.JobID)
		if err != nil {
			t.Fatalf("failed to get allocations: %v", err)
		}
		// Only 1 canary allocation should be created initially
		if len(allocs) != 1 {
			t.Fatalf("expected 1 canary allocation initially, got %d", len(allocs))
		}
		if !allocs[0].IsCanary {
			t.Errorf("expected allocation to be marked as canary")
		}

		// Promotion before healthy should fail
		err = driver.PromoteCanary(ctx, res.JobID)
		if err == nil {
			t.Errorf("expected promote to fail before canary is healthy")
		}

		// Wait for canary to become healthy
		time.Sleep(25 * time.Millisecond)
		health, err := driver.CheckHealth(ctx, res.JobID)
		if err != nil {
			t.Fatalf("failed to check health: %v", err)
		}
		if !health.CanaryReady {
			t.Errorf("expected CanaryReady to be true")
		}

		// Promote canary
		if err := driver.PromoteCanary(ctx, res.JobID); err != nil {
			t.Fatalf("failed to promote canary: %v", err)
		}

		// Verify remaining allocations provisioned
		allocsAfter, err := driver.GetAllocations(ctx, res.JobID)
		if err != nil {
			t.Fatalf("failed to get allocations after promotion: %v", err)
		}
		if len(allocsAfter) != 3 {
			t.Errorf("expected 3 allocations after promotion, got %d", len(allocsAfter))
		}
	})

	t.Run("Scaling Job Replicas", func(t *testing.T) {
		jobID := "adroit-svc-svc-scale"
		cfg := JobConfig{
			DeploymentID: "dep-scale-1",
			ServiceID:    "svc-scale",
			ServiceName:  "Scale App",
			ServiceSlug:  "scale-app",
			ImageTag:     "registry.adroit.run/org/scale:v1",
			Port:         8080,
			Replicas:     2,
			Strategy:     types.RolloutStrategyRolling,
		}

		spec, _ := GenerateJobSpec(cfg)
		_, err := driver.SubmitJob(ctx, spec)
		if err != nil {
			t.Fatalf("failed to submit scale job: %v", err)
		}

		// Scale up to 4
		if err := driver.ScaleJob(ctx, jobID, 4); err != nil {
			t.Fatalf("failed to scale up: %v", err)
		}
		allocs, _ := driver.GetAllocations(ctx, jobID)
		activeCount := 0
		for _, a := range allocs {
			if a.ClientStatus == "running" && a.DesiredStatus == "run" {
				activeCount++
			}
		}
		if activeCount != 4 {
			t.Errorf("expected 4 active allocations after scaling up, got %d", activeCount)
		}

		// Scale down to 1
		if err := driver.ScaleJob(ctx, jobID, 1); err != nil {
			t.Fatalf("failed to scale down: %v", err)
		}
		allocsDown, _ := driver.GetAllocations(ctx, jobID)
		activeDown := 0
		for _, a := range allocsDown {
			if a.ClientStatus == "running" && a.DesiredStatus == "run" {
				activeDown++
			}
		}
		if activeDown != 1 {
			t.Errorf("expected 1 active allocation after scaling down, got %d", activeDown)
		}
	})

	t.Run("Simulated Failure Injection", func(t *testing.T) {
		driver.SetSimulateFailure(true)
		defer driver.SetSimulateFailure(false)

		cfg := JobConfig{
			DeploymentID: "dep-fail-1",
			ServiceID:    "svc-fail",
			ServiceName:  "Fail App",
			ServiceSlug:  "fail-app",
			ImageTag:     "registry.adroit.run/org/fail:v1",
		}
		spec, _ := GenerateJobSpec(cfg)
		_, err := driver.SubmitJob(ctx, spec)
		if err == nil {
			t.Errorf("expected failure when simulateFailure is true")
		}
	})
}

func TestLiveNomadSubmission(t *testing.T) {
	client := NewNomadHTTPClient(NomadClientConfig{
		Address: "http://localhost:4646",
	})
	cfg := JobConfig{
		DeploymentID: "dep-live-test",
		ServiceID:    "svc-test123",
		ServiceName:  "Test App",
		ServiceSlug:  "test-app",
		ImageTag:     "alpine:latest",
		Port:         8080,
		Replicas:     1,
		Datacenter:   "dc1",
	}
	spec, err := GenerateJobSpec(cfg)
	if err != nil {
		t.Fatalf("failed to generate job spec: %v", err)
	}

	res, err := client.SubmitJob(context.Background(), spec)
	if err != nil {
		t.Fatalf("live Nomad submission failed: %v", err)
	}
	t.Logf("Job successfully parsed and submitted to live Nomad! EvalID: %s, JobID: %s", res.EvalID, res.JobID)
	_ = client.StopJob(context.Background(), res.JobID, true)
}
