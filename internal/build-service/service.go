package buildservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/builder"
	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// ServiceConfig holds operational configuration for the build service
type ServiceConfig struct {
	RegistryHost         string
	DefaultBuildTimeout  time.Duration
	MaxConcurrentBuilds  int
	DefaultStepDuration  time.Duration
}

// Option configures Service dependencies
type Option func(*Service)

// WithEventBus sets the event bus for publishing and subscribing
func WithEventBus(bus events.EventBus) Option {
	return func(s *Service) {
		s.eventBus = bus
	}
}

// WithLogger sets the structured logger
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) {
		s.logger = log
	}
}

// Service manages build triggers, execution lifecycles, and event dispatching
type Service struct {
	repo         Repository
	builder      builder.Builder
	eventBus     events.EventBus
	logger       *slog.Logger
	cfg          ServiceConfig

	mu           sync.RWMutex
	cancelFuncs  map[string]context.CancelFunc
	inFlight     map[string]bool
	buildSem     chan struct{}
}

// NewService creates a new build service engine
func NewService(repo Repository, b builder.Builder, cfg ServiceConfig, opts ...Option) *Service {
	if cfg.RegistryHost == "" {
		cfg.RegistryHost = "registry.adroit.run"
	}
	if cfg.DefaultBuildTimeout <= 0 {
		cfg.DefaultBuildTimeout = 15 * time.Minute
	}
	if cfg.MaxConcurrentBuilds <= 0 {
		cfg.MaxConcurrentBuilds = 5
	}

	s := &Service{
		repo:        repo,
		builder:     b,
		cfg:         cfg,
		logger:      slog.Default(),
		cancelFuncs: make(map[string]context.CancelFunc),
		inFlight:    make(map[string]bool),
		buildSem:    make(chan struct{}, cfg.MaxConcurrentBuilds),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// TriggerBuild triggers a new build manually for a service
func (s *Service) TriggerBuild(ctx context.Context, userID, serviceID string, req TriggerBuildRequest) (*types.Build, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	// Verify org access
	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = svc.Branch
	}
	if branch == "" {
		branch = "main"
	}

	commitHash := strings.TrimSpace(req.CommitHash)
	if commitHash == "" {
		// Generate realistic commit hash if not specified
		b := make([]byte, 20)
		_, _ = rand.Read(b)
		commitHash = hex.EncodeToString(b)
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		message = fmt.Sprintf("Manual build triggered for branch %s", branch)
	}

	build := &types.Build{
		ID:          uuid.New().String(),
		ServiceID:   svc.ID,
		CommitHash:  commitHash,
		Branch:      branch,
		Message:     message,
		Author:      "user:" + userID,
		Status:      types.BuildStatusQueued,
		TriggeredBy: "manual:" + userID,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.repo.CreateBuild(ctx, build); err != nil {
		return nil, fmt.Errorf("failed to create build record: %w", err)
	}

	s.logger.Info("Created build record",
		slog.String("build_id", build.ID),
		slog.String("service_id", svc.ID),
		slog.String("commit", commitHash),
	)

	// Emit adroit.build.requested event
	projID := ""
	if svc.ProjectID != nil {
		projID = *svc.ProjectID
	}

	dockerfile := req.DockerfilePath
	if dockerfile == "" {
		dockerfile = svc.DockerfilePath
	}

	evt := events.New(events.SubjectBuildRequested, "build-service", events.BuildRequestedPayload{
		BuildID:        build.ID,
		ServiceID:      svc.ID,
		ProjectID:      projID,
		RepoURL:        svc.RepoURL,
		Branch:         branch,
		CommitHash:     commitHash,
		DockerfilePath: dockerfile,
		BuildArgs:      req.BuildArgs,
	})

	if s.eventBus != nil {
		if err := s.eventBus.Publish(ctx, events.SubjectBuildRequested, evt); err != nil {
			s.logger.Warn("Failed to publish SubjectBuildRequested event", slog.Any("error", err))
		}
	}

	// Launch async execution
	go s.ExecuteBuild(context.Background(), build.ID, req.BuildArgs, dockerfile)

	return build, nil
}

// ListBuilds lists build history for a service
func (s *Service) ListBuilds(ctx context.Context, userID, serviceID string, limit, offset int, status string) (*BuildListResponse, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	builds, total, err := s.repo.ListBuildsByService(ctx, serviceID, limit, offset, status)
	if err != nil {
		return nil, err
	}

	return &BuildListResponse{
		Builds: builds,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// GetBuild retrieves a single build by ID
func (s *Service) GetBuild(ctx context.Context, userID, buildID string) (*types.Build, error) {
	build, err := s.repo.GetBuildByID(ctx, buildID)
	if err != nil {
		return nil, err
	}

	svc, err := s.repo.GetServiceByID(ctx, build.ServiceID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	return build, nil
}

// CancelBuild aborts a running or queued build
func (s *Service) CancelBuild(ctx context.Context, userID, buildID string) (*types.Build, error) {
	build, err := s.repo.GetBuildByID(ctx, buildID)
	if err != nil {
		return nil, err
	}

	svc, err := s.repo.GetServiceByID(ctx, build.ServiceID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	if build.Status == types.BuildStatusSucceeded || build.Status == types.BuildStatusFailed || build.Status == types.BuildStatusCancelled {
		return nil, fmt.Errorf("%w: build is currently %s", ErrBuildFinished, build.Status)
	}

	// 1. Signal cancellation context
	s.mu.RLock()
	cancel, hasCancel := s.cancelFuncs[buildID]
	s.mu.RUnlock()

	if hasCancel {
		cancel()
	} else if s.builder != nil {
		_ = s.builder.Cancel(ctx, buildID)
	}

	// 2. Mark cancelled in database
	now := time.Now().UTC()
	durationMs := int64(now.Sub(build.CreatedAt).Milliseconds())

	if err := s.repo.UpdateBuildStatus(ctx, buildID, types.BuildStatusCancelled, "", "", "Build cancelled by user", durationMs, &now); err != nil {
		return nil, err
	}

	// 3. Append cancellation log
	_ = s.repo.InsertBuildLog(ctx, &types.BuildLogEntry{
		BuildID:   buildID,
		Line:      "[!] Build aborted: cancelled by user request",
		Stream:    "stderr",
		Timestamp: now,
	})

	// 4. Emit cancelled event
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.SubjectBuildCancelled, events.New(
			events.SubjectBuildCancelled,
			"build-service",
			events.BuildCancelledPayload{
				BuildID:   buildID,
				ServiceID: build.ServiceID,
			},
		))
	}

	build.Status = types.BuildStatusCancelled
	build.FinishedAt = &now
	build.DurationMs = durationMs
	build.ErrorMessage = "Build cancelled by user"

	s.logger.Info("Build cancelled successfully", slog.String("build_id", buildID))
	return build, nil
}

// GetBuildLogs retrieves all logged execution steps for a build
func (s *Service) GetBuildLogs(ctx context.Context, userID, buildID string, limit, offset int) (*BuildLogsResponse, error) {
	build, err := s.repo.GetBuildByID(ctx, buildID)
	if err != nil {
		return nil, err
	}

	svc, err := s.repo.GetServiceByID(ctx, build.ServiceID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	entries, err := s.repo.GetBuildLogs(ctx, buildID, limit, offset)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(e.Line)
		sb.WriteString("\n")
	}

	return &BuildLogsResponse{
		BuildID:    buildID,
		Logs:       entries,
		FullLog:    sb.String(),
		TotalLines: len(entries),
	}, nil
}

// ProcessBuildRequestedEvent handles incoming events from NATS JetStream (e.g. from git-integration-service)
func (s *Service) ProcessBuildRequestedEvent(ctx context.Context, payload events.BuildRequestedPayload) error {
	if payload.BuildID == "" {
		return fmt.Errorf("%w: payload missing build_id", ErrInvalidRequest)
	}

	s.logger.Info("Received BuildRequested event from event bus",
		slog.String("build_id", payload.BuildID),
		slog.String("service_id", payload.ServiceID),
		slog.String("commit", payload.CommitHash),
	)

	// Launch async execution worker
	go s.ExecuteBuild(context.Background(), payload.BuildID, payload.BuildArgs, payload.DockerfilePath)
	return nil
}

// StartEventListener subscribes to adroit.build.requested on NATS JetStream
func (s *Service) StartEventListener(ctx context.Context) error {
	if s.eventBus == nil {
		return nil
	}

	err := s.eventBus.Subscribe(events.SubjectBuildRequested, func(subCtx context.Context, subject string, raw []byte) error {
		// Attempt envelope unmarshal
		var env events.Event[events.BuildRequestedPayload]
		if err := json.Unmarshal(raw, &env); err == nil && env.Data.BuildID != "" {
			return s.ProcessBuildRequestedEvent(subCtx, env.Data)
		}

		// Direct payload fallback
		var direct events.BuildRequestedPayload
		if err := json.Unmarshal(raw, &direct); err == nil && direct.BuildID != "" {
			return s.ProcessBuildRequestedEvent(subCtx, direct)
		}

		s.logger.Warn("Failed to decode BuildRequested event payload", slog.String("raw", string(raw)))
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", events.SubjectBuildRequested, err)
	}

	s.logger.Info("Subscribed to NATS JetStream subject", slog.String("subject", events.SubjectBuildRequested))
	return nil
}

// ExecuteBuild is the worker routine that executes a build with concurrency control and event emission
func (s *Service) ExecuteBuild(parentCtx context.Context, buildID string, buildArgs map[string]string, dockerfileOverride string) {
	// Deduplication / Concurrency guard
	s.mu.Lock()
	if s.inFlight[buildID] {
		s.mu.Unlock()
		return
	}
	s.inFlight[buildID] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.inFlight, buildID)
		delete(s.cancelFuncs, buildID)
		s.mu.Unlock()
	}()

	// Acquire worker slot
	select {
	case s.buildSem <- struct{}{}:
		defer func() { <-s.buildSem }()
	case <-parentCtx.Done():
		return
	}

	buildCtx, cancel := context.WithTimeout(parentCtx, s.cfg.DefaultBuildTimeout)
	defer cancel()

	s.mu.Lock()
	s.cancelFuncs[buildID] = cancel
	s.mu.Unlock()

	// 1. Fetch build & service
	build, err := s.repo.GetBuildByID(buildCtx, buildID)
	if err != nil {
		s.logger.Error("Worker failed to fetch build record", slog.String("build_id", buildID), slog.String("error", err.Error()))
		return
	}

	// If build was already cancelled or succeeded, exit
	if build.Status == types.BuildStatusCancelled || build.Status == types.BuildStatusSucceeded {
		return
	}

	svc, err := s.repo.GetServiceByID(buildCtx, build.ServiceID)
	if err != nil {
		s.logger.Error("Worker failed to fetch service record", slog.String("service_id", build.ServiceID), slog.String("error", err.Error()))
		now := time.Now().UTC()
		_ = s.repo.UpdateBuildStatus(buildCtx, buildID, types.BuildStatusFailed, "", "", "Service not found", 0, &now)
		return
	}

	orgSlug, _ := s.repo.GetOrganizationSlug(buildCtx, svc.OrganizationID)
	configuredBuildEnv, _ := s.repo.GetBuildEnvironmentVariables(buildCtx, svc.ID, svc.ProjectID)

	mergedBuildArgs := make(map[string]string)
	for k, v := range configuredBuildEnv {
		mergedBuildArgs[k] = v
	}
	for k, v := range buildArgs {
		mergedBuildArgs[k] = v // Explicit request arguments override service env vars
	}

	dockerfile := dockerfileOverride
	if dockerfile == "" {
		dockerfile = svc.DockerfilePath
	}

	// 2. Mark build as building
	now := time.Now().UTC()
	if err := s.repo.UpdateBuildStatus(buildCtx, buildID, types.BuildStatusBuilding, "", "", "", 0, nil); err != nil {
		s.logger.Error("Failed to transition build to building status", slog.String("build_id", buildID), slog.String("error", err.Error()))
		return
	}

	// Emit adroit.build.started
	if s.eventBus != nil {
		_ = s.eventBus.Publish(buildCtx, events.SubjectBuildStarted, events.New(
			events.SubjectBuildStarted,
			"build-service",
			events.BuildStartedPayload{
				BuildID:    buildID,
				ServiceID:  svc.ID,
				CommitHash: build.CommitHash,
				Branch:     build.Branch,
				StartedAt:  now,
			},
		))
	}

	s.logger.Info("Build execution started",
		slog.String("build_id", buildID),
		slog.String("service", svc.Slug),
		slog.String("commit", build.CommitHash),
	)

	// 3. Stream logs collector
	startTime := time.Now().UTC()
	logWriter := builder.FuncLogWriter(func(stream, line string) {
		entry := &types.BuildLogEntry{
			BuildID:   buildID,
			Line:      line,
			Stream:    stream,
			Timestamp: time.Now().UTC(),
		}
		_ = s.repo.InsertBuildLog(context.Background(), entry)
	})

	// 4. Invoke Builder engine
	buildCfg := builder.BuildConfig{
		BuildID:        buildID,
		ServiceID:      svc.ID,
		ServiceName:    svc.Name,
		ServiceSlug:    svc.Slug,
		OrgSlug:        orgSlug,
		RepoURL:        svc.RepoURL,
		Branch:         build.Branch,
		CommitHash:     build.CommitHash,
		DockerfilePath: dockerfile,
		BuildCommand:   svc.BuildCommand,
		RootDirectory:  svc.RootDirectory,
		BuildArgs:      mergedBuildArgs,
		RegistryHost:   s.cfg.RegistryHost,
	}

	res, buildErr := s.builder.Build(buildCtx, buildCfg, logWriter)
	finishedAt := time.Now().UTC()
	durationMs := int64(finishedAt.Sub(startTime).Milliseconds())

	// 5. Handle build cancellation
	if errors.Is(buildErr, builder.ErrBuildCancelled) || buildCtx.Err() == context.Canceled {
		_ = s.repo.UpdateBuildStatus(context.Background(), buildID, types.BuildStatusCancelled, "", "", "Build cancelled during execution", durationMs, &finishedAt)
		s.logger.Warn("Build was cancelled", slog.String("build_id", buildID))
		return
	}

	// 6. Handle build failure
	if buildErr != nil || (res != nil && !res.Success) {
		errMsg := "Build failed"
		if buildErr != nil {
			errMsg = buildErr.Error()
		} else if res != nil && res.Error != nil {
			errMsg = res.Error.Error()
		}

		_ = s.repo.UpdateBuildStatus(context.Background(), buildID, types.BuildStatusFailed, "", "", errMsg, durationMs, &finishedAt)

		if s.eventBus != nil {
			_ = s.eventBus.Publish(context.Background(), events.SubjectBuildFailed, events.New(
				events.SubjectBuildFailed,
				"build-service",
				events.BuildFailedPayload{
					BuildID:    buildID,
					ServiceID:  svc.ID,
					Error:      errMsg,
					DurationMs: durationMs,
				},
			))
		}

		s.logger.Error("Build execution failed",
			slog.String("build_id", buildID),
			slog.String("error", errMsg),
			slog.Int64("duration_ms", durationMs),
		)
		return
	}

	// 7. Handle build success
	imageTag := res.ImageTag
	imageDigest := res.ImageDigest

	if err := s.repo.UpdateBuildStatus(context.Background(), buildID, types.BuildStatusSucceeded, imageTag, imageDigest, "", durationMs, &finishedAt); err != nil {
		s.logger.Error("Failed to update build success record", slog.String("build_id", buildID), slog.String("error", err.Error()))
	}

	// Emit adroit.build.completed
	if s.eventBus != nil {
		_ = s.eventBus.Publish(context.Background(), events.SubjectBuildCompleted, events.New(
			events.SubjectBuildCompleted,
			"build-service",
			events.BuildCompletedPayload{
				BuildID:     buildID,
				ServiceID:   svc.ID,
				ImageTag:    imageTag,
				ImageDigest: imageDigest,
				DurationMs:  durationMs,
			},
		))
	}

	// 8. Create pending deployment & emit adroit.deploy.requested to hand off to Orchestration Service
	deployment := &types.Deployment{
		ID:        uuid.New().String(),
		ServiceID: svc.ID,
		BuildID:   buildID,
		Status:    types.DeploymentStatusPending,
		Replicas:  svc.MinReplicas,
		CreatedAt: finishedAt,
		UpdatedAt: finishedAt,
	}

	if err := s.repo.CreateDeployment(context.Background(), deployment); err != nil {
		s.logger.Error("Failed to create pending deployment for Orchestration handoff", slog.String("build_id", buildID), slog.String("error", err.Error()))
	} else {
		s.logger.Info("Created pending deployment for Orchestration handoff",
			slog.String("deployment_id", deployment.ID),
			slog.String("service_id", svc.ID),
			slog.String("build_id", buildID),
		)

		if s.eventBus != nil {
			deployEvt := events.New(events.SubjectDeployRequested, "build-service", events.DeployRequestedPayload{
				DeploymentID: deployment.ID,
				ServiceID:    svc.ID,
				BuildID:      buildID,
				ImageTag:     imageTag,
				Port:         svc.Port,
				Replicas:     svc.MinReplicas,
				Resources:    svc.Resources,
				HealthCheck:  svc.HealthCheck,
				EnvVars:      nil, // Resolved securely by orchestration service with Vault
				RuntimeTier:  svc.RuntimeTier,
			})

			if err := s.eventBus.Publish(context.Background(), events.SubjectDeployRequested, deployEvt); err != nil {
				s.logger.Warn("Failed to emit SubjectDeployRequested event", slog.Any("error", err))
			} else {
				s.logger.Info("Dispatched SubjectDeployRequested event to Orchestration Service",
					slog.String("deployment_id", deployment.ID),
					slog.String("image_tag", imageTag),
				)
			}
		}
	}

	s.logger.Info("Build execution succeeded",
		slog.String("build_id", buildID),
		slog.String("image_tag", imageTag),
		slog.String("digest", imageDigest),
		slog.Int64("duration_ms", durationMs),
	)
}
