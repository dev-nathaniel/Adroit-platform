package orchestrationservice

import (
	"context"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/orchestrator"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupTestEnvironment() (*Service, *mockRepository, *orchestrator.SimulatedDriver, events.EventBus) {
	repo := newMockRepository()
	driver := orchestrator.NewSimulatedDriver(10 * time.Millisecond)
	bus := events.NewInMemBus()

	svc := NewService(
		repo,
		driver,
		ServiceConfig{
			Datacenter:         "dc1",
			BaseDomain:         "adroit.run",
			RolloutTimeout:     5 * time.Second,
			HealthPollInterval: 15 * time.Millisecond,
			AutoPromoteCanary:  true,
		},
		WithEventBus(bus),
	)

	return svc, repo, driver, bus
}

func TestHandleDeployRequested_Rolling(t *testing.T) {
	svc, repo, _, bus := setupTestEnvironment()
	defer bus.Close()

	ctx := context.Background()

	// Seed service and build
	serviceID := "svc-111"
	repo.services[serviceID] = types.Service{
		ID:          serviceID,
		Name:        "API Service",
		Slug:        "api-svc",
		Port:        8080,
		MinReplicas: 2,
		MaxReplicas: 4,
	}

	buildID := "build-111"
	repo.builds[buildID] = types.Build{
		ID:        buildID,
		ServiceID: serviceID,
		Status:    types.BuildStatusSucceeded,
		ImageTag:  "registry.adroit.run/acme/api:v1.0.0",
	}

	deployID := "dep-111"

	var healthyReceived bool
	_ = bus.Subscribe(events.SubjectDeployHealthy, func(subCtx context.Context, subj string, data []byte) error {
		healthyReceived = true
		return nil
	})

	payload := events.DeployRequestedPayload{
		DeploymentID: deployID,
		ServiceID:    serviceID,
		BuildID:      buildID,
		ImageTag:     "registry.adroit.run/acme/api:v1.0.0",
		Port:         8080,
		Replicas:     2,
		Strategy:     types.RolloutStrategyRolling,
	}

	err := svc.HandleDeployRequested(ctx, payload)
	if err != nil {
		t.Fatalf("HandleDeployRequested failed: %v", err)
	}

	// Wait for background supervisor to complete rollout
	time.Sleep(100 * time.Millisecond)

	// Verify deployment state
	dep, err := repo.GetDeployment(ctx, deployID)
	if err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	if dep.Status != types.DeploymentStatusActive {
		t.Errorf("expected deployment status %s, got %s (err: %s)", types.DeploymentStatusActive, dep.Status, dep.ErrorMessage)
	}

	if dep.ActiveURL != "https://api-svc.adroit.run" {
		t.Errorf("unexpected active URL: %s", dep.ActiveURL)
	}

	// Verify service active deployment pointer
	serviceRecord, _ := repo.GetService(ctx, serviceID)
	if serviceRecord.ActiveDeploymentID == nil || *serviceRecord.ActiveDeploymentID != deployID {
		t.Errorf("expected active deployment on service to be updated to %s", deployID)
	}

	// Verify events recorded
	eventsList, err := repo.ListDeploymentEvents(ctx, deployID)
	if err != nil || len(eventsList) == 0 {
		t.Errorf("expected deployment timeline events, found none")
	}

	if !healthyReceived {
		t.Errorf("expected adroit.deploy.healthy event to be dispatched across bus")
	}
}

func TestHandleDeployRequested_Canary(t *testing.T) {
	svc, repo, _, bus := setupTestEnvironment()
	defer bus.Close()

	ctx := context.Background()

	serviceID := "svc-canary-test"
	repo.services[serviceID] = types.Service{
		ID:          serviceID,
		Name:        "Canary Service",
		Slug:        "canary-svc",
		Port:        3000,
		MinReplicas: 3,
	}

	deployID := "dep-canary-test"
	payload := events.DeployRequestedPayload{
		DeploymentID: deployID,
		ServiceID:    serviceID,
		BuildID:      "b-1",
		ImageTag:     "registry.adroit.run/acme/canary:v2.0",
		Port:         3000,
		Replicas:     3,
		Strategy:     types.RolloutStrategyCanary,
	}

	err := svc.HandleDeployRequested(ctx, payload)
	if err != nil {
		t.Fatalf("HandleDeployRequested failed: %v", err)
	}

	// Allow supervisor to verify canary, auto-promote, and achieve full health
	time.Sleep(150 * time.Millisecond)

	dep, err := repo.GetDeployment(ctx, deployID)
	if err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	if dep.Status != types.DeploymentStatusActive {
		t.Errorf("expected canary deployment to become active, got: %s", dep.Status)
	}

	if !dep.CanaryPromoted {
		t.Errorf("expected canary to be marked as promoted")
	}
}

func TestTriggerDeploy(t *testing.T) {
	svc, repo, _, _ := setupTestEnvironment()
	ctx := context.Background()

	serviceID := "svc-trigger"
	repo.services[serviceID] = types.Service{
		ID:          serviceID,
		Name:        "Trigger Svc",
		Slug:        "trigger-svc",
		MinReplicas: 2,
	}
	repo.builds["build-latest"] = types.Build{
		ID:        "build-latest",
		ServiceID: serviceID,
		Status:    types.BuildStatusSucceeded,
		ImageTag:  "registry.adroit.run/org/trigger:latest",
	}

	d, err := svc.TriggerDeploy(ctx, TriggerDeployRequest{
		ServiceID: serviceID,
		Replicas:  2,
		Strategy:  types.RolloutStrategyRolling,
	})
	if err != nil {
		t.Fatalf("TriggerDeploy failed: %v", err)
	}
	if d.BuildID != "build-latest" {
		t.Errorf("expected auto-resolved build-latest, got %s", d.BuildID)
	}
}

func TestTriggerDeploy_ImageSourceDirect(t *testing.T) {
	svc, repo, _, _ := setupTestEnvironment()
	ctx := context.Background()

	serviceID := "svc-image-direct"
	repo.services[serviceID] = types.Service{
		ID:          serviceID,
		Name:        "Postgres Direct",
		Slug:        "postgres-direct",
		SourceType:  types.SourceTypeImage,
		ImageURL:    "postgres:16-alpine",
		MinReplicas: 1,
	}

	// No builds exist initially in repo.builds for this service!
	d, err := svc.TriggerDeploy(ctx, TriggerDeployRequest{
		ServiceID: serviceID,
		Replicas:  1,
		Strategy:  types.RolloutStrategyRolling,
	})
	if err != nil {
		t.Fatalf("TriggerDeploy failed for image service without prior build: %v", err)
	}

	if d.BuildID == "" {
		t.Fatalf("expected non-empty synthetic build ID")
	}

	// Verify build was created in repository
	createdBuild, err := repo.GetBuild(ctx, d.BuildID)
	if err != nil {
		t.Fatalf("expected synthetic build to exist in repository: %v", err)
	}
	if createdBuild.SourceType != types.SourceTypeImage {
		t.Errorf("expected build source_type to be image, got %s", createdBuild.SourceType)
	}
	if createdBuild.ImageTag != "postgres:16-alpine" {
		t.Errorf("expected build image_tag 'postgres:16-alpine', got '%s'", createdBuild.ImageTag)
	}
	if createdBuild.Status != types.BuildStatusSucceeded {
		t.Errorf("expected build status succeeded, got %s", createdBuild.Status)
	}
}

func TestScaleService(t *testing.T) {
	svc, repo, driver, _ := setupTestEnvironment()
	ctx := context.Background()

	serviceID := "svc-scale"
	repo.services[serviceID] = types.Service{
		ID:          serviceID,
		Name:        "Scale Svc",
		Slug:        "scale-svc",
		MinReplicas: 1,
	}

	// First submit job to driver
	cfg := orchestrator.JobConfig{
		ServiceID:   serviceID,
		ServiceName: "Scale Svc",
		ServiceSlug: "scale-svc",
		ImageTag:    "registry.adroit.run/org/scale:v1",
		Replicas:    1,
	}
	spec, _ := orchestrator.GenerateJobSpec(cfg)
	_, _ = driver.SubmitJob(ctx, spec)

	// Scale service
	if err := svc.ScaleService(ctx, serviceID, 3); err != nil {
		t.Fatalf("ScaleService failed: %v", err)
	}

	svcModel, _ := repo.GetService(ctx, serviceID)
	if svcModel.MinReplicas != 3 {
		t.Errorf("expected MinReplicas = 3, got %d", svcModel.MinReplicas)
	}
}

func TestCancelDeployment(t *testing.T) {
	svc, repo, _, _ := setupTestEnvironment()
	ctx := context.Background()

	deployID := "dep-cancel"
	_ = repo.CreateDeployment(ctx, &types.Deployment{
		ID:         deployID,
		ServiceID:  "svc-c",
		Status:     types.DeploymentStatusDeploying,
		NomadJobID: "adroit-svc-svc-c",
	})

	if err := svc.CancelDeployment(ctx, deployID); err != nil {
		t.Fatalf("CancelDeployment failed: %v", err)
	}

	d, _ := repo.GetDeployment(ctx, deployID)
	if d.Status != types.DeploymentStatusStopped {
		t.Errorf("expected status stopped, got %s", d.Status)
	}
}
