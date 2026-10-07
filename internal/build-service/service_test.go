package buildservice

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/builder"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

type failingBuilder struct {
	errMsg string
}

func (f *failingBuilder) Build(ctx context.Context, cfg builder.BuildConfig, logWriter builder.LogWriter) (*builder.BuildResult, error) {
	if logWriter != nil {
		logWriter.WriteLine("stderr", "fatal error: compilation failed")
	}
	return nil, errors.New(f.errMsg)
}

func (f *failingBuilder) Cancel(ctx context.Context, buildID string) error {
	return nil
}

func setupTestService(t *testing.T) (*Service, *MockRepository, *events.InMemBus, string, string, string) {
	repo := NewMockRepository()
	bus := events.NewInMemBus()

	userID := "usr_" + uuid.New().String()[:8]
	orgID := "org_" + uuid.New().String()[:8]
	serviceID := "svc_" + uuid.New().String()[:8]

	repo.orgMembers[orgID] = map[string]string{userID: "owner"}
	repo.orgSlugs[orgID] = "acme"

	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
		Name:           "API Gateway",
		Slug:           "api-gateway",
		Type:           types.ServiceTypeWeb,
		RuntimeTier:    types.RuntimeTierGeneral,
		RepoURL:        "https://github.com/acme/api.git",
		Branch:         "main",
		Port:           8080,
		MinReplicas:    2,
		MaxReplicas:    5,
		Resources: types.ResourceLimits{
			CPUCores: 1.0,
			MemoryMB: 1024,
		},
		HealthCheck: types.HealthCheckConfig{
			Path: "/healthz",
			Port: 8080,
		},
	}

	b := builder.NewLocalDevBuilder(0) // fast 0-duration for tests
	svc := NewService(repo, b, ServiceConfig{
		RegistryHost:        "registry.adroit.run",
		DefaultBuildTimeout: 10 * time.Second,
		MaxConcurrentBuilds: 5,
	}, WithEventBus(bus))

	return svc, repo, bus, userID, orgID, serviceID
}

func TestTriggerBuild_Success(t *testing.T) {
	svc, repo, bus, userID, _, serviceID := setupTestService(t)

	var (
		mu           sync.Mutex
		receivedEvts []string
	)

	recordEvt := func(subject string) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvts = append(receivedEvts, subject)
	}

	_ = bus.Subscribe(events.SubjectBuildRequested, func(ctx context.Context, subj string, raw []byte) error {
		recordEvt(subj)
		return nil
	})
	_ = bus.Subscribe(events.SubjectBuildStarted, func(ctx context.Context, subj string, raw []byte) error {
		recordEvt(subj)
		return nil
	})
	_ = bus.Subscribe(events.SubjectBuildCompleted, func(ctx context.Context, subj string, raw []byte) error {
		recordEvt(subj)
		return nil
	})
	_ = bus.Subscribe(events.SubjectDeployRequested, func(ctx context.Context, subj string, raw []byte) error {
		recordEvt(subj)
		return nil
	})

	ctx := context.Background()
	req := TriggerBuildRequest{
		Branch:     "develop",
		CommitHash: "c0ffee123456",
		Message:    "Triggered unit test build",
		BuildArgs: map[string]string{
			"ENV": "test",
		},
	}

	build, err := svc.TriggerBuild(ctx, userID, serviceID, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if build.Status != types.BuildStatusQueued {
		t.Errorf("expected initial status queued, got %s", build.Status)
	}
	if build.Branch != "develop" {
		t.Errorf("expected branch develop, got %s", build.Branch)
	}

	// Wait up to 500ms for async execution to complete
	deadline := time.Now().Add(500 * time.Millisecond)
	var finalBuild *types.Build
	for time.Now().Before(deadline) {
		b, err := repo.GetBuildByID(ctx, build.ID)
		if err == nil && b.Status == types.BuildStatusSucceeded {
			finalBuild = b
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if finalBuild == nil {
		b, _ := repo.GetBuildByID(ctx, build.ID)
		t.Fatalf("build did not reach succeeded in time: %+v", b)
	}

	if !strings.HasPrefix(finalBuild.ImageTag, "registry.adroit.run/acme/api-gateway:c0ffee123456") {
		t.Errorf("unexpected image tag: %s", finalBuild.ImageTag)
	}
	if !strings.HasPrefix(finalBuild.ImageDigest, "sha256:") {
		t.Errorf("unexpected image digest: %s", finalBuild.ImageDigest)
	}

	// Verify deployment created
	if len(repo.deployments) == 0 {
		t.Errorf("expected deployment record created for orchestration handoff")
	}

	// Verify events were emitted
	mu.Lock()
	evtCount := len(receivedEvts)
	mu.Unlock()

	if evtCount < 3 {
		t.Errorf("expected at least 3 events emitted, got %d: %v", evtCount, receivedEvts)
	}
}

func TestTriggerBuild_Forbidden(t *testing.T) {
	svc, _, _, _, _, serviceID := setupTestService(t)

	_, err := svc.TriggerBuild(context.Background(), "attacker_user", serviceID, TriggerBuildRequest{})
	if err == nil {
		t.Fatalf("expected ErrForbidden, got nil")
	}
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestTriggerBuild_ServiceNotFound(t *testing.T) {
	svc, _, _, userID, _, _ := setupTestService(t)

	_, err := svc.TriggerBuild(context.Background(), userID, "non-existent-service", TriggerBuildRequest{})
	if err == nil {
		t.Fatalf("expected ErrNotFound, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListBuilds(t *testing.T) {
	svc, repo, _, userID, _, serviceID := setupTestService(t)
	ctx := context.Background()

	_ = repo.CreateBuild(ctx, &types.Build{
		ID:         "b1",
		ServiceID:  serviceID,
		Status:     types.BuildStatusSucceeded,
		CreatedAt:  time.Now().Add(-2 * time.Minute),
		CommitHash: "111",
		Branch:     "main",
	})
	_ = repo.CreateBuild(ctx, &types.Build{
		ID:         "b2",
		ServiceID:  serviceID,
		Status:     types.BuildStatusFailed,
		CreatedAt:  time.Now().Add(-1 * time.Minute),
		CommitHash: "222",
		Branch:     "main",
	})
	_ = repo.CreateBuild(ctx, &types.Build{
		ID:         "b3",
		ServiceID:  serviceID,
		Status:     types.BuildStatusQueued,
		CreatedAt:  time.Now(),
		CommitHash: "333",
		Branch:     "dev",
	})

	// List all
	resp, err := svc.ListBuilds(ctx, userID, serviceID, 10, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("expected 3 total, got %d", resp.Total)
	}

	// Filter by status
	respFiltered, err := svc.ListBuilds(ctx, userID, serviceID, 10, 0, "succeeded")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respFiltered.Total != 1 {
		t.Errorf("expected 1 succeeded, got %d", respFiltered.Total)
	}
}

func TestGetBuild(t *testing.T) {
	svc, repo, _, userID, _, serviceID := setupTestService(t)
	ctx := context.Background()

	_ = repo.CreateBuild(ctx, &types.Build{
		ID:          "build-lookup",
		ServiceID:   serviceID,
		CommitHash:  "aabbcc112233",
		Branch:      "main",
		Status:      types.BuildStatusSucceeded,
		ImageTag:    "registry.adroit.run/acme/api:aabbcc",
		TriggeredBy: "manual:tester",
	})

	got, err := svc.GetBuild(ctx, userID, "build-lookup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "build-lookup" || got.ImageTag != "registry.adroit.run/acme/api:aabbcc" {
		t.Errorf("unexpected build retrieved: %+v", got)
	}
}

func TestCancelBuild(t *testing.T) {
	svc, repo, bus, userID, _, serviceID := setupTestService(t)
	ctx := context.Background()

	var (
		cancelMu             sync.Mutex
		cancelledEvtReceived bool
	)
	_ = bus.Subscribe(events.SubjectBuildCancelled, func(ctx context.Context, subj string, raw []byte) error {
		cancelMu.Lock()
		cancelledEvtReceived = true
		cancelMu.Unlock()
		return nil
	})

	_ = repo.CreateBuild(ctx, &types.Build{
		ID:        "build-to-cancel",
		ServiceID: serviceID,
		Status:    types.BuildStatusBuilding,
	})

	cancelled, err := svc.CancelBuild(ctx, userID, "build-to-cancel")
	if err != nil {
		t.Fatalf("failed to cancel build: %v", err)
	}

	if cancelled.Status != types.BuildStatusCancelled {
		t.Errorf("expected status cancelled, got %s", cancelled.Status)
	}
	if cancelled.FinishedAt == nil {
		t.Errorf("expected FinishedAt to be populated")
	}

	// Cancelling already cancelled should fail with ErrBuildFinished
	_, err = svc.CancelBuild(ctx, userID, "build-to-cancel")
	if err == nil || !errors.Is(err, ErrBuildFinished) {
		t.Errorf("expected ErrBuildFinished, got %v", err)
	}

	// Wait up to 200ms for InMemBus goroutine
	deadline := time.Now().Add(200 * time.Millisecond)
	gotEvt := false
	for time.Now().Before(deadline) {
		cancelMu.Lock()
		if cancelledEvtReceived {
			gotEvt = true
			cancelMu.Unlock()
			break
		}
		cancelMu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}

	if !gotEvt {
		t.Errorf("expected SubjectBuildCancelled event to be emitted")
	}
}

func TestGetBuildLogs(t *testing.T) {
	svc, repo, _, userID, _, serviceID := setupTestService(t)
	ctx := context.Background()

	_ = repo.CreateBuild(ctx, &types.Build{
		ID:        "build-logs-test",
		ServiceID: serviceID,
		Status:    types.BuildStatusSucceeded,
	})

	_ = repo.InsertBuildLog(ctx, &types.BuildLogEntry{
		BuildID: "build-logs-test",
		Line:    "Step 1: Cloning",
		Stream:  "stdout",
	})
	_ = repo.InsertBuildLog(ctx, &types.BuildLogEntry{
		BuildID: "build-logs-test",
		Line:    "Step 2: Building",
		Stream:  "stdout",
	})

	logs, err := svc.GetBuildLogs(ctx, userID, "build-logs-test", 100, 0)
	if err != nil {
		t.Fatalf("failed to get build logs: %v", err)
	}

	if logs.TotalLines != 2 {
		t.Errorf("expected 2 lines, got %d", logs.TotalLines)
	}
	if !strings.Contains(logs.FullLog, "Step 1: Cloning") {
		t.Errorf("full log missing line 1: %s", logs.FullLog)
	}
}

func TestBuildExecution_Failure(t *testing.T) {
	repo := NewMockRepository()
	bus := events.NewInMemBus()

	userID := "usr_fail"
	orgID := "org_fail"
	serviceID := "svc_fail"

	repo.orgMembers[orgID] = map[string]string{userID: "owner"}
	repo.orgSlugs[orgID] = "failorg"

	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
		Name:           "Fail Service",
		Slug:           "fail-svc",
		Branch:         "main",
	}

	failBld := &failingBuilder{errMsg: "gcc syntax error"}
	svc := NewService(repo, failBld, ServiceConfig{
		DefaultBuildTimeout: 5 * time.Second,
	}, WithEventBus(bus))

	var failedEvtReceived bool
	_ = bus.Subscribe(events.SubjectBuildFailed, func(ctx context.Context, subj string, raw []byte) error {
		failedEvtReceived = true
		return nil
	})

	build, err := svc.TriggerBuild(context.Background(), userID, serviceID, TriggerBuildRequest{})
	if err != nil {
		t.Fatalf("unexpected error triggering build: %v", err)
	}

	// Wait for worker to finish
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		b, err := repo.GetBuildByID(context.Background(), build.ID)
		if err == nil && b.Status == types.BuildStatusFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	finalBuild, err := repo.GetBuildByID(context.Background(), build.ID)
	if err != nil {
		t.Fatalf("failed to get build: %v", err)
	}

	if finalBuild.Status != types.BuildStatusFailed {
		t.Errorf("expected status failed, got %s", finalBuild.Status)
	}
	if !strings.Contains(finalBuild.ErrorMessage, "gcc syntax error") {
		t.Errorf("expected error message to contain 'gcc syntax error', got %s", finalBuild.ErrorMessage)
	}
	if !failedEvtReceived {
		t.Errorf("expected SubjectBuildFailed event to be emitted")
	}
}

func TestBuildExecution_ImageSource(t *testing.T) {
	repo := NewMockRepository()
	bus := events.NewInMemBus()

	userID := "usr_img"
	orgID := "org_img"
	serviceID := "svc_img"

	repo.orgMembers[orgID] = map[string]string{userID: "owner"}
	repo.orgSlugs[orgID] = "acme"

	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
		Name:           "Database",
		Slug:           "database",
		SourceType:     types.SourceTypeImage,
		ImageURL:       "postgres:16-alpine",
		Port:           5432,
		MinReplicas:    1,
		MaxReplicas:    1,
		Resources: types.ResourceLimits{
			CPUCores: 0.5,
			MemoryMB: 512,
		},
	}

	// failingBuilder ensures that if the image branch actually tried to invoke BuildKit, it would fail
	failBld := &failingBuilder{errMsg: "BuildKit should not be called for pre-built image!"}
	svc := NewService(repo, failBld, ServiceConfig{
		DefaultBuildTimeout: 5 * time.Second,
	}, WithEventBus(bus))

	var deployRequestedReceived bool
	var deployedImageTag string
	_ = bus.Subscribe(events.SubjectDeployRequested, func(ctx context.Context, subj string, raw []byte) error {
		var evt events.Event[events.DeployRequestedPayload]
		_ = json.Unmarshal(raw, &evt)
		deployRequestedReceived = true
		deployedImageTag = evt.Data.ImageTag
		return nil
	})

	build, err := svc.TriggerBuild(context.Background(), userID, serviceID, TriggerBuildRequest{})
	if err != nil {
		t.Fatalf("unexpected error triggering build: %v", err)
	}

	if build.SourceType != types.SourceTypeImage {
		t.Errorf("expected build source_type to be image, got %s", build.SourceType)
	}

	// Wait for worker to finish
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		b, err := repo.GetBuildByID(context.Background(), build.ID)
		if err == nil && b.Status == types.BuildStatusSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	finalBuild, err := repo.GetBuildByID(context.Background(), build.ID)
	if err != nil {
		t.Fatalf("failed to get build: %v", err)
	}

	if finalBuild.Status != types.BuildStatusSucceeded {
		t.Fatalf("expected build to succeed for image source, got %s (err: %s)", finalBuild.Status, finalBuild.ErrorMessage)
	}
	if finalBuild.ImageTag != "postgres:16-alpine" {
		t.Errorf("expected ImageTag 'postgres:16-alpine', got '%s'", finalBuild.ImageTag)
	}

	if !deployRequestedReceived {
		t.Errorf("expected SubjectDeployRequested event to be emitted")
	}
	if deployedImageTag != "postgres:16-alpine" {
		t.Errorf("expected deployed image tag 'postgres:16-alpine', got '%s'", deployedImageTag)
	}
}

func TestBuildExecution_ArchiveSource(t *testing.T) {
	repo := NewMockRepository()
	bus := events.NewInMemBus()

	userID := "usr_arch"
	orgID := "org_arch"
	serviceID := "svc_arch"

	repo.orgMembers[orgID] = map[string]string{userID: "owner"}
	repo.orgSlugs[orgID] = "acme"

	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
		Name:           "Archive Web",
		Slug:           "archive-web",
		SourceType:     types.SourceTypeArchive,
		ArchiveURL:     "https://storage.adroit.run/source/svc-123.tar.gz",
		Port:           3000,
		MinReplicas:    1,
		MaxReplicas:    2,
	}

	devBld := builder.NewLocalDevBuilder(0)
	svc := NewService(repo, devBld, ServiceConfig{
		DefaultBuildTimeout: 5 * time.Second,
	}, WithEventBus(bus))

	build, err := svc.TriggerBuild(context.Background(), userID, serviceID, TriggerBuildRequest{})
	if err != nil {
		t.Fatalf("unexpected error triggering build: %v", err)
	}

	if build.SourceType != types.SourceTypeArchive {
		t.Errorf("expected build source_type archive, got %s", build.SourceType)
	}
	if build.ArchiveURL != "https://storage.adroit.run/source/svc-123.tar.gz" {
		t.Errorf("expected build archive_url to match service, got %s", build.ArchiveURL)
	}

	// Wait for worker to finish
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		b, err := repo.GetBuildByID(context.Background(), build.ID)
		if err == nil && b.Status == types.BuildStatusSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	finalBuild, err := repo.GetBuildByID(context.Background(), build.ID)
	if err != nil {
		t.Fatalf("failed to get build: %v", err)
	}

	if finalBuild.Status != types.BuildStatusSucceeded {
		t.Fatalf("expected build to succeed for archive source, got %s (err: %s)", finalBuild.Status, finalBuild.ErrorMessage)
	}
}

