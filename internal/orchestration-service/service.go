package orchestrationservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/orchestrator"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	ErrDeploymentNotCanary = errors.New("deployment is not a canary rollout")
	ErrCannotCancelDone    = errors.New("cannot cancel a deployment that is already finished")
)

// ServiceConfig defines operational parameters for the Orchestration Service
type ServiceConfig struct {
	Datacenter         string
	BaseDomain         string
	RolloutTimeout     time.Duration
	HealthPollInterval time.Duration
	AutoPromoteCanary  bool
}

// Service manages the end-to-end workload scheduling lifecycle on Nomad
type Service struct {
	repo     Repository
	driver   orchestrator.Driver
	eventBus events.EventBus
	logger   *slog.Logger
	cfg      ServiceConfig

	mu             sync.Mutex
	activeRollouts map[string]context.CancelFunc
}

// Option configures functional options on the Service
type Option func(*Service)

// WithEventBus sets the event bus
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

// NewService creates an initialized Orchestration & Runtime Service
func NewService(repo Repository, driver orchestrator.Driver, cfg ServiceConfig, opts ...Option) *Service {
	if cfg.Datacenter == "" {
		cfg.Datacenter = "dc1"
	}
	if cfg.BaseDomain == "" {
		cfg.BaseDomain = "adroit.run"
	}
	if cfg.RolloutTimeout <= 0 {
		cfg.RolloutTimeout = 5 * time.Minute
	}
	if cfg.HealthPollInterval <= 0 {
		cfg.HealthPollInterval = 1 * time.Second
	}

	s := &Service{
		repo:           repo,
		driver:         driver,
		logger:         slog.Default(),
		cfg:            cfg,
		activeRollouts: make(map[string]context.CancelFunc),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// StartEventListener binds to NATS JetStream subject adroit.deploy.requested
func (s *Service) StartEventListener(ctx context.Context) error {
	if s.eventBus == nil {
		s.logger.Warn("EventBus not configured; skipping NATS deploy subscription")
		return nil
	}

	err := s.eventBus.Subscribe(events.SubjectDeployRequested, func(subCtx context.Context, subject string, raw []byte) error {
		var evt events.Event[events.DeployRequestedPayload]
		if err := json.Unmarshal(raw, &evt); err != nil {
			s.logger.Error("Failed to unmarshal DeployRequested event", slog.String("error", err.Error()))
			return err
		}

		s.logger.Info("Received adroit.deploy.requested event",
			slog.String("deployment_id", evt.Data.DeploymentID),
			slog.String("service_id", evt.Data.ServiceID),
			slog.String("image_tag", evt.Data.ImageTag),
		)

		go func() {
			if err := s.HandleDeployRequested(context.Background(), evt.Data); err != nil {
				s.logger.Error("Failed to handle deploy request",
					slog.String("deployment_id", evt.Data.DeploymentID),
					slog.String("error", err.Error()),
				)
			}
		}()

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", events.SubjectDeployRequested, err)
	}

	s.logger.Info("Subscribed to NATS JetStream deployment events", slog.String("subject", events.SubjectDeployRequested))
	return nil
}

// HandleDeployRequested executes the Nomad job translation, submission, and rollout monitoring
func (s *Service) HandleDeployRequested(ctx context.Context, payload events.DeployRequestedPayload) error {
	startTime := time.Now().UTC()

	// 1. Fetch Service metadata
	svc, err := s.repo.GetService(ctx, payload.ServiceID)
	if err != nil {
		s.logger.Error("Service not found for deployment", slog.String("service_id", payload.ServiceID), slog.String("error", err.Error()))
		return err
	}

	// 2. Fetch or create deployment record
	deployment, err := s.repo.GetDeployment(ctx, payload.DeploymentID)
	if err != nil {
		if errors.Is(err, ErrDeploymentNotFound) {
			strategy := payload.Strategy
			if strategy == "" {
				strategy = types.RolloutStrategyRolling
			}
			replicas := payload.Replicas
			if replicas <= 0 {
				replicas = svc.MinReplicas
			}
			deployment = &types.Deployment{
				ID:        payload.DeploymentID,
				ServiceID: payload.ServiceID,
				BuildID:   payload.BuildID,
				Status:    types.DeploymentStatusPending,
				Replicas:  replicas,
				Strategy:  strategy,
			}
			if err := s.repo.CreateDeployment(ctx, deployment); err != nil {
				return fmt.Errorf("failed to initialize deployment record: %w", err)
			}
		} else {
			return err
		}
	}

	// 3. Resolve Environment Variables (hierarchical project + service merge)
	envVars := payload.EnvVars
	if len(envVars) == 0 {
		resolved, err := s.repo.GetEnvironmentVariables(ctx, svc.ID, svc.ProjectID)
		if err == nil && len(resolved) > 0 {
			envVars = resolved
		}
	}

	strategy := deployment.Strategy
	if strategy == "" {
		strategy = types.RolloutStrategyRolling
	}

	replicas := deployment.Replicas
	if replicas <= 0 {
		replicas = 1
	}

	// 4. Translate configuration into Nomad HCL2 Job Specification
	jobCfg := orchestrator.JobConfig{
		DeploymentID: deployment.ID,
		ServiceID:    svc.ID,
		ServiceName:  svc.Name,
		ServiceSlug:  svc.Slug,
		ImageTag:     payload.ImageTag,
		Port:         payload.Port,
		Replicas:     replicas,
		Resources:    payload.Resources,
		HealthCheck:  payload.HealthCheck,
		EnvVars:      envVars,
		RuntimeTier:  payload.RuntimeTier,
		Strategy:     strategy,
		Datacenter:   s.cfg.Datacenter,
	}

	specHCL, err := orchestrator.GenerateJobSpec(jobCfg)
	if err != nil {
		s.logger.Error("Failed to generate Nomad job specification", slog.String("error", err.Error()))
		_ = s.repo.UpdateDeploymentStatus(ctx, deployment.ID, types.DeploymentStatusFailed, "", "", err.Error())
		s.emitDeployFailed(ctx, deployment.ID, svc.ID, "", err.Error(), time.Since(startTime).Milliseconds())
		return err
	}

	// Save spec and record milestone event
	_ = s.repo.UpdateDeploymentJobSpec(ctx, deployment.ID, specHCL)
	_ = s.repo.CreateDeploymentEvent(ctx, &types.DeploymentEvent{
		DeploymentID: deployment.ID,
		EventType:    "job_spec_generated",
		Message:      fmt.Sprintf("Generated Nomad job specification (%s rollout strategy)", strategy),
		Details: map[string]any{
			"strategy": strategy,
			"replicas": replicas,
		},
	})

	// 5. Update Status to Deploying
	_ = s.repo.UpdateDeploymentStatus(ctx, deployment.ID, types.DeploymentStatusDeploying, "", "", "")

	// 6. Submit Job to Nomad
	res, err := s.driver.SubmitJob(ctx, specHCL)
	if err != nil {
		s.logger.Error("Nomad driver rejected job submission", slog.String("error", err.Error()))
		_ = s.repo.UpdateDeploymentStatus(ctx, deployment.ID, types.DeploymentStatusFailed, "", "", err.Error())
		_ = s.repo.CreateDeploymentEvent(ctx, &types.DeploymentEvent{
			DeploymentID: deployment.ID,
			EventType:    "nomad_submission_failed",
			Message:      fmt.Sprintf("Nomad submission error: %v", err),
		})
		s.emitDeployFailed(ctx, deployment.ID, svc.ID, "", err.Error(), time.Since(startTime).Milliseconds())
		return err
	}

	nomadJobID := res.JobID
	_ = s.repo.UpdateDeploymentStatus(ctx, deployment.ID, types.DeploymentStatusDeploying, nomadJobID, "", "")

	// Record submission milestone
	_ = s.repo.CreateDeploymentEvent(ctx, &types.DeploymentEvent{
		DeploymentID: deployment.ID,
		EventType:    "job_submitted",
		Message:      fmt.Sprintf("Submitted job to Nomad cluster (eval: %s)", res.EvalID),
		Details: map[string]any{
			"nomad_job_id": nomadJobID,
			"eval_id":      res.EvalID,
		},
	})

	// 7. Emit adroit.deploy.started
	s.emitDeployStarted(ctx, deployment.ID, svc.ID, nomadJobID, strategy)

	// 8. Sync initial allocations
	if allocs, err := s.driver.GetAllocations(ctx, nomadJobID); err == nil {
		for _, a := range allocs {
			_ = s.repo.UpsertAllocation(ctx, &types.Allocation{
				DeploymentID:  deployment.ID,
				NomadAllocID:  a.ID,
				NodeName:      a.NodeName,
				ClientStatus:  a.ClientStatus,
				DesiredStatus: a.DesiredStatus,
				IsHealthy:     a.Healthy,
			})
		}
	}

	// 9. Launch rollout supervisor in background
	rolloutCtx, cancel := context.WithTimeout(context.Background(), s.cfg.RolloutTimeout)
	s.mu.Lock()
	s.activeRollouts[deployment.ID] = cancel
	s.mu.Unlock()

	go s.superviseRollout(rolloutCtx, deployment.ID, svc, nomadJobID, strategy, replicas, startTime)

	return nil
}

// superviseRollout monitors allocations, promotes canaries, and marks deployments active or failed
func (s *Service) superviseRollout(ctx context.Context, deployID string, svc *types.Service, nomadJobID string, strategy types.RolloutStrategy, targetReplicas int, startTime time.Time) {
	defer func() {
		s.mu.Lock()
		delete(s.activeRollouts, deployID)
		s.mu.Unlock()
	}()

	ticker := time.NewTicker(s.cfg.HealthPollInterval)
	defer ticker.Stop()

	canaryPromoted := false

	for {
		select {
		case <-ctx.Done():
			// Timeout or cancellation
			errMsg := "rollout health check deadline exceeded"
			if errors.Is(ctx.Err(), context.Canceled) {
				errMsg = "rollout was cancelled by operator"
			}
			s.logger.Warn("Rollout aborted", slog.String("deployment_id", deployID), slog.String("reason", errMsg))
			_ = s.repo.UpdateDeploymentStatus(context.Background(), deployID, types.DeploymentStatusFailed, nomadJobID, "", errMsg)
			_ = s.repo.CreateDeploymentEvent(context.Background(), &types.DeploymentEvent{
				DeploymentID: deployID,
				EventType:    "rollout_failed",
				Message:      errMsg,
			})
			s.emitDeployFailed(context.Background(), deployID, svc.ID, nomadJobID, errMsg, time.Since(startTime).Milliseconds())
			return

		case <-ticker.C:
			// 1. Sync allocations
			allocs, err := s.driver.GetAllocations(ctx, nomadJobID)
			if err == nil {
				for _, a := range allocs {
					_ = s.repo.UpsertAllocation(context.Background(), &types.Allocation{
						DeploymentID:  deployID,
						NomadAllocID:  a.ID,
						NodeName:      a.NodeName,
						ClientStatus:  a.ClientStatus,
						DesiredStatus: a.DesiredStatus,
						IsHealthy:     a.Healthy,
					})
				}
			}

			// 2. Check cluster health status
			health, err := s.driver.CheckHealth(ctx, nomadJobID)
			if err != nil {
				continue
			}

			// 3. Canary Strategy Handling
			if strategy == types.RolloutStrategyCanary && !canaryPromoted {
				if health.CanaryReady {
					_ = s.repo.CreateDeploymentEvent(context.Background(), &types.DeploymentEvent{
						DeploymentID: deployID,
						EventType:    "canary_healthy",
						Message:      "Canary allocation passed health checks",
					})

					if s.cfg.AutoPromoteCanary {
						s.logger.Info("Auto-promoting canary allocation", slog.String("deployment_id", deployID))
						if err := s.driver.PromoteCanary(ctx, nomadJobID); err == nil {
							canaryPromoted = true
							_ = s.repo.UpdateDeploymentCanaryPromoted(context.Background(), deployID, true)
							_ = s.repo.CreateDeploymentEvent(context.Background(), &types.DeploymentEvent{
								DeploymentID: deployID,
								EventType:    "canary_promoted",
								Message:      "Canary promoted; provisioning remaining service replicas",
							})
						}
					}
				}
				continue
			}

			// 4. Full Deployment Health Verification
			if health.Healthy && health.PassingCount >= targetReplicas {
				activeURL := s.constructActiveURL(svc)

				s.logger.Info("Deployment verified healthy on Nomad",
					slog.String("deployment_id", deployID),
					slog.String("active_url", activeURL),
					slog.Duration("duration", time.Since(startTime)),
				)

				// Update deployment record to active
				_ = s.repo.UpdateDeploymentStatus(context.Background(), deployID, types.DeploymentStatusActive, nomadJobID, activeURL, "")
				// Mark older active deployments replaced
				_ = s.repo.MarkPreviousDeploymentsReplaced(context.Background(), svc.ID, deployID)
				// Update service active deployment pointer
				_ = s.repo.UpdateServiceActiveDeployment(context.Background(), svc.ID, deployID)

				// Record milestone event
				_ = s.repo.CreateDeploymentEvent(context.Background(), &types.DeploymentEvent{
					DeploymentID: deployID,
					EventType:    "deployment_healthy",
					Message:      fmt.Sprintf("All %d replicas healthy. Traffic routed to %s", targetReplicas, activeURL),
					Details: map[string]any{
						"active_url": activeURL,
						"replicas":   targetReplicas,
					},
				})

				// Emit adroit.deploy.healthy
				s.emitDeployHealthy(context.Background(), deployID, svc.ID, nomadJobID, activeURL)
				return
			}
		}
	}
}

// TriggerDeploy initiates a deployment from a request
func (s *Service) TriggerDeploy(ctx context.Context, req TriggerDeployRequest) (*types.Deployment, error) {
	if req.ServiceID == "" {
		return nil, fmt.Errorf("service_id is required")
	}

	svc, err := s.repo.GetService(ctx, req.ServiceID)
	if err != nil {
		return nil, err
	}

	// Resolve build
	var build *types.Build
	if req.BuildID != "" {
		b, err := s.repo.GetBuild(ctx, req.BuildID)
		if err != nil {
			return nil, err
		}
		build = b
	} else {
		b, err := s.repo.GetLatestSucceededBuildForService(ctx, req.ServiceID)
		if err != nil {
			return nil, err
		}
		build = b
	}

	strategy := req.Strategy
	if strategy == "" {
		strategy = types.RolloutStrategyRolling
	}

	replicas := req.Replicas
	if replicas <= 0 {
		replicas = svc.MinReplicas
	}
	if replicas <= 0 {
		replicas = 1
	}

	deployID := uuid.New().String()
	deployment := &types.Deployment{
		ID:        deployID,
		ServiceID: svc.ID,
		BuildID:   build.ID,
		Status:    types.DeploymentStatusPending,
		Replicas:  replicas,
		Strategy:  strategy,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.repo.CreateDeployment(ctx, deployment); err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	// Dispatch deployment via event or directly
	payload := events.DeployRequestedPayload{
		DeploymentID: deployment.ID,
		ServiceID:    svc.ID,
		BuildID:      build.ID,
		ImageTag:     build.ImageTag,
		Port:         svc.Port,
		Replicas:     replicas,
		Resources:    svc.Resources,
		HealthCheck:  svc.HealthCheck,
		RuntimeTier:  svc.RuntimeTier,
		Strategy:     strategy,
	}

	if s.eventBus != nil {
		deployEvt := events.New(events.SubjectDeployRequested, "orchestration-service", payload)
		_ = s.eventBus.Publish(ctx, events.SubjectDeployRequested, deployEvt)
	} else {
		go func() {
			_ = s.HandleDeployRequested(context.Background(), payload)
		}()
	}

	return deployment, nil
}

// PromoteCanary promotes a pending canary deployment
func (s *Service) PromoteCanary(ctx context.Context, deployID string) error {
	deployment, err := s.repo.GetDeployment(ctx, deployID)
	if err != nil {
		return err
	}

	if deployment.Strategy != types.RolloutStrategyCanary {
		return ErrDeploymentNotCanary
	}

	if err := s.driver.PromoteCanary(ctx, deployment.NomadJobID); err != nil {
		return err
	}

	_ = s.repo.UpdateDeploymentCanaryPromoted(ctx, deployID, true)
	_ = s.repo.CreateDeploymentEvent(ctx, &types.DeploymentEvent{
		DeploymentID: deployID,
		EventType:    "canary_promoted",
		Message:      "Canary manually promoted by operator",
	})

	return nil
}

// Rollback restores a previous successful deployment
func (s *Service) Rollback(ctx context.Context, targetDeploymentID string) (*types.Deployment, error) {
	targetDeploy, err := s.repo.GetDeployment(ctx, targetDeploymentID)
	if err != nil {
		return nil, err
	}

	return s.TriggerDeploy(ctx, TriggerDeployRequest{
		ServiceID: targetDeploy.ServiceID,
		BuildID:   targetDeploy.BuildID,
		Replicas:  targetDeploy.Replicas,
		Strategy:  types.RolloutStrategyRolling,
	})
}

// CancelDeployment cancels an active or deploying rollout
func (s *Service) CancelDeployment(ctx context.Context, deployID string) error {
	deployment, err := s.repo.GetDeployment(ctx, deployID)
	if err != nil {
		return err
	}

	if deployment.Status == types.DeploymentStatusActive || deployment.Status == types.DeploymentStatusFailed {
		return ErrCannotCancelDone
	}

	s.mu.Lock()
	if cancel, exists := s.activeRollouts[deployID]; exists {
		cancel()
		delete(s.activeRollouts, deployID)
	}
	s.mu.Unlock()

	if deployment.NomadJobID != "" {
		_ = s.driver.StopJob(ctx, deployment.NomadJobID, false)
	}

	_ = s.repo.UpdateDeploymentStatus(ctx, deployID, types.DeploymentStatusStopped, deployment.NomadJobID, "", "deployment stopped by user")
	_ = s.repo.CreateDeploymentEvent(ctx, &types.DeploymentEvent{
		DeploymentID: deployID,
		EventType:    "deployment_stopped",
		Message:      "Deployment cancelled by user",
	})

	if s.eventBus != nil {
		stopEvt := events.New(events.SubjectDeployStopped, "orchestration-service", events.DeployStoppedPayload{
			DeploymentID: deployID,
			ServiceID:    deployment.ServiceID,
			NomadJobID:   deployment.NomadJobID,
			Reason:       "User requested cancellation",
		})
		_ = s.eventBus.Publish(ctx, events.SubjectDeployStopped, stopEvt)
	}

	return nil
}

// ScaleService scales instance replica count on Nomad and updates database
func (s *Service) ScaleService(ctx context.Context, serviceID string, replicas int) error {
	if replicas <= 0 {
		return fmt.Errorf("replica count must be at least 1")
	}

	svc, err := s.repo.GetService(ctx, serviceID)
	if err != nil {
		return err
	}

	// Update DB
	if err := s.repo.UpdateServiceReplicas(ctx, serviceID, replicas); err != nil {
		return err
	}

	// Scale active Nomad job if one exists
	jobID := orchestrator.GenerateJobID(serviceID)
	if err := s.driver.ScaleJob(ctx, jobID, replicas); err != nil {
		s.logger.Warn("Failed to scale active Nomad job (may not be running)", slog.String("job_id", jobID), slog.String("error", err.Error()))
	}

	s.logger.Info("Scaled service replicas successfully", slog.String("service_id", serviceID), slog.Int("replicas", replicas), slog.String("name", svc.Name))
	return nil
}

// GetDeployment returns deployment with service, allocations, and timeline events
func (s *Service) GetDeployment(ctx context.Context, deployID string) (*DeploymentDetail, error) {
	d, err := s.repo.GetDeployment(ctx, deployID)
	if err != nil {
		return nil, err
	}

	svc, _ := s.repo.GetService(ctx, d.ServiceID)
	allocs, _ := s.repo.ListAllocations(ctx, deployID)
	events, _ := s.repo.ListDeploymentEvents(ctx, deployID)

	return &DeploymentDetail{
		Deployment:  *d,
		Service:     svc,
		Allocations: allocs,
		Events:      events,
	}, nil
}

// ListDeployments returns paginated deployments for a service
func (s *Service) ListDeployments(ctx context.Context, serviceID string, limit, offset int) ([]types.Deployment, int, error) {
	return s.repo.ListDeploymentsByService(ctx, serviceID, limit, offset)
}

// GetDeploymentSpec returns the rendered Nomad HCL job specification
func (s *Service) GetDeploymentSpec(ctx context.Context, deployID string) (string, error) {
	d, err := s.repo.GetDeployment(ctx, deployID)
	if err != nil {
		return "", err
	}
	if d.JobSpec == "" {
		return "", fmt.Errorf("no job specification found for deployment %s", deployID)
	}
	return d.JobSpec, nil
}

// GetDeploymentAllocations returns allocations for a deployment
func (s *Service) GetDeploymentAllocations(ctx context.Context, deployID string) ([]types.Allocation, error) {
	return s.repo.ListAllocations(ctx, deployID)
}

// GetDeploymentEvents returns timeline events for a deployment
func (s *Service) GetDeploymentEvents(ctx context.Context, deployID string) ([]types.DeploymentEvent, error) {
	return s.repo.ListDeploymentEvents(ctx, deployID)
}

func (s *Service) constructActiveURL(svc *types.Service) string {
	slug := svc.Slug
	if slug == "" {
		slug = svc.ID[:8]
	}
	slug = strings.ToLower(slug)
	return fmt.Sprintf("https://%s.%s", slug, s.cfg.BaseDomain)
}

func (s *Service) emitDeployStarted(ctx context.Context, deployID, svcID, nomadJobID string, strategy types.RolloutStrategy) {
	if s.eventBus == nil {
		return
	}
	evt := events.New(events.SubjectDeployStarted, "orchestration-service", events.DeployStartedPayload{
		DeploymentID: deployID,
		ServiceID:    svcID,
		NomadJobID:   nomadJobID,
		Strategy:     strategy,
		StartedAt:    time.Now().UTC(),
	})
	_ = s.eventBus.Publish(ctx, events.SubjectDeployStarted, evt)
}

func (s *Service) emitDeployHealthy(ctx context.Context, deployID, svcID, nomadJobID, activeURL string) {
	if s.eventBus == nil {
		return
	}
	evt := events.New(events.SubjectDeployHealthy, "orchestration-service", events.DeployHealthyPayload{
		DeploymentID: deployID,
		ServiceID:    svcID,
		NomadJobID:   nomadJobID,
		ActiveURL:    activeURL,
	})
	_ = s.eventBus.Publish(ctx, events.SubjectDeployHealthy, evt)
}

func (s *Service) emitDeployFailed(ctx context.Context, deployID, svcID, nomadJobID, errorMsg string, durationMs int64) {
	if s.eventBus == nil {
		return
	}
	evt := events.New(events.SubjectDeployFailed, "orchestration-service", events.DeployFailedPayload{
		DeploymentID: deployID,
		ServiceID:    svcID,
		NomadJobID:   nomadJobID,
		Error:        errorMsg,
		DurationMs:   durationMs,
	})
	_ = s.eventBus.Publish(ctx, events.SubjectDeployFailed, evt)
}
