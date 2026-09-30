package logservice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// Service encapsulates core log collection, storage, and live streaming workflows
type Service struct {
	repo        Repository
	broadcaster *Broadcaster
	tailer      LogTailer
	eventBus    events.EventBus
	log         *slog.Logger
}

// NewService creates a new Log Service instance
func NewService(
	repo Repository,
	broadcaster *Broadcaster,
	tailer LogTailer,
	eventBus events.EventBus,
	log *slog.Logger,
) *Service {
	if broadcaster == nil {
		broadcaster = NewBroadcaster()
	}
	if log == nil {
		log = slog.Default()
	}

	return &Service{
		repo:        repo,
		broadcaster: broadcaster,
		tailer:      tailer,
		eventBus:    eventBus,
		log:         log,
	}
}

// IngestLogs processes, stores, and broadcasts incoming application log entries
func (s *Service) IngestLogs(ctx context.Context, serviceID string, req IngestLogsRequest) error {
	if serviceID == "" && req.ServiceID != "" {
		serviceID = req.ServiceID
	}
	if serviceID == "" {
		return fmt.Errorf("service_id is required")
	}

	// 1. Verify service exists
	_, err := s.repo.GetService(ctx, serviceID)
	if err != nil {
		return fmt.Errorf("invalid service for log ingestion: %w", err)
	}

	if len(req.Logs) == 0 {
		return nil
	}

	// 2. Prepare entries
	now := time.Now().UTC()
	entries := make([]types.AppLogEntry, 0, len(req.Logs))

	for _, item := range req.Logs {
		ts := now
		if item.Timestamp != nil && !item.Timestamp.IsZero() {
			ts = item.Timestamp.UTC()
		}

		stream := item.Stream
		if stream == "" {
			stream = "stdout"
		}

		depID := item.DeploymentID
		if depID == nil && req.DeploymentID != "" {
			depID = &req.DeploymentID
		}

		allocID := item.AllocationID
		if allocID == nil && req.AllocationID != "" {
			allocID = &req.AllocationID
		}

		entry := types.AppLogEntry{
			ID:           "log_" + uuid.New().String()[:12],
			ServiceID:    serviceID,
			DeploymentID: depID,
			AllocationID: allocID,
			Stream:       stream,
			Message:      item.Message,
			Timestamp:    ts,
		}
		entries = append(entries, entry)
	}

	// 3. Batch persist in database
	if err := s.repo.InsertLogsBatch(ctx, entries); err != nil {
		s.log.Error("Failed to persist batch log entries", slog.String("service_id", serviceID), slog.Any("error", err))
		return fmt.Errorf("failed to persist logs: %w", err)
	}

	// 4. Live broadcast to all active subscribers
	for _, entry := range entries {
		s.broadcaster.Broadcast(entry)
	}

	return nil
}

// IngestSystemLog records an automated platform event line for a service
func (s *Service) IngestSystemLog(ctx context.Context, serviceID string, deploymentID *string, message string) {
	entry := types.AppLogEntry{
		ID:           "log_" + uuid.New().String()[:12],
		ServiceID:    serviceID,
		DeploymentID: deploymentID,
		Stream:       "system",
		Message:      message,
		Timestamp:    time.Now().UTC(),
	}

	_ = s.repo.InsertLog(ctx, entry)
	s.broadcaster.Broadcast(entry)
}

// GetLogs returns paginated historical logs with authorization check
func (s *Service) GetLogs(ctx context.Context, userID string, filter types.AppLogsQueryFilter) (*types.AppLogsResponse, error) {
	// 1. Authorize user against service organization
	svc, err := s.repo.GetService(ctx, filter.ServiceID)
	if err != nil {
		return nil, err
	}

	if userID != "" {
		hasAccess, err := s.repo.UserHasOrgAccess(ctx, userID, svc.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("failed to verify organization access: %w", err)
		}
		if !hasAccess {
			return nil, ErrUnauthorized
		}
	}

	// 2. Fetch logs
	logs, total, err := s.repo.GetLogs(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &types.AppLogsResponse{
		Logs:       logs,
		TotalLines: total,
		ServiceID:  filter.ServiceID,
	}, nil
}

// GetDeploymentLogs returns paginated logs scoped to a deployment
func (s *Service) GetDeploymentLogs(ctx context.Context, userID, deploymentID string, filter types.AppLogsQueryFilter) (*types.AppLogsResponse, error) {
	logs, total, err := s.repo.GetDeploymentLogs(ctx, deploymentID, filter)
	if err != nil {
		return nil, err
	}

	serviceID := filter.ServiceID
	if len(logs) > 0 && serviceID == "" {
		serviceID = logs[0].ServiceID
	}

	return &types.AppLogsResponse{
		Logs:       logs,
		TotalLines: total,
		ServiceID:  serviceID,
	}, nil
}

// Subscribe opens a real-time pub-sub stream channel for a service
func (s *Service) Subscribe(serviceID string) (<-chan types.AppLogEntry, func()) {
	return s.broadcaster.Subscribe(serviceID)
}

// StartTailer launches a background worker that tails logs from a Nomad allocation into the service
func (s *Service) StartTailer(ctx context.Context, serviceID, deploymentID, allocID, task, stream string) {
	if s.tailer == nil {
		return
	}

	go func() {
		outCh := make(chan types.AppLogEntry, 64)
		tailCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		go func() {
			for {
				select {
				case <-tailCtx.Done():
					return
				case entry, ok := <-outCh:
					if !ok {
						return
					}
					_ = s.repo.InsertLog(tailCtx, entry)
					s.broadcaster.Broadcast(entry)
				}
			}
		}()

		if err := s.tailer.TailAllocation(tailCtx, serviceID, deploymentID, allocID, task, stream, outCh); err != nil {
			s.log.Debug("Tailer stopped", slog.String("alloc_id", allocID), slog.Any("error", err))
		}
		close(outCh)
	}()
}

// StartEventSubscribers registers NATS JetStream listeners for deployment lifecycle events
func (s *Service) StartEventSubscribers(ctx context.Context) error {
	if s.eventBus == nil {
		return nil
	}

	// 1. Subscribe to deployment started events
	err := s.eventBus.Subscribe(events.SubjectDeployStarted, func(ctx context.Context, subject string, raw []byte) error {
		var evt events.Event[events.DeployStartedPayload]
		if err := json.Unmarshal(raw, &evt); err == nil && evt.Data.ServiceID != "" {
			msg := fmt.Sprintf("[adroit-system] Deployment %s started on Nomad cluster", evt.Data.DeploymentID)
			s.IngestSystemLog(ctx, evt.Data.ServiceID, &evt.Data.DeploymentID, msg)
		}
		return nil
	})
	if err != nil {
		s.log.Warn("Failed to subscribe to deploy started events", slog.Any("error", err))
	}

	// 2. Subscribe to deployment healthy events
	err = s.eventBus.Subscribe(events.SubjectDeployHealthy, func(ctx context.Context, subject string, raw []byte) error {
		var evt events.Event[events.DeployHealthyPayload]
		if err := json.Unmarshal(raw, &evt); err == nil && evt.Data.ServiceID != "" {
			msg := fmt.Sprintf("[adroit-system] Deployment %s is healthy and passing all readiness checks", evt.Data.DeploymentID)
			s.IngestSystemLog(ctx, evt.Data.ServiceID, &evt.Data.DeploymentID, msg)
		}
		return nil
	})
	if err != nil {
		s.log.Warn("Failed to subscribe to deploy healthy events", slog.Any("error", err))
	}

	// 3. Subscribe to deployment failed events
	err = s.eventBus.Subscribe(events.SubjectDeployFailed, func(ctx context.Context, subject string, raw []byte) error {
		var evt events.Event[events.DeployFailedPayload]
		if err := json.Unmarshal(raw, &evt); err == nil && evt.Data.ServiceID != "" {
			msg := fmt.Sprintf("[adroit-system] Deployment %s failed: %s", evt.Data.DeploymentID, evt.Data.Error)
			s.IngestSystemLog(ctx, evt.Data.ServiceID, &evt.Data.DeploymentID, msg)
		}
		return nil
	})
	if err != nil {
		s.log.Warn("Failed to subscribe to deploy failed events", slog.Any("error", err))
	}

	// 4. Subscribe to deployment stopped events
	err = s.eventBus.Subscribe(events.SubjectDeployStopped, func(ctx context.Context, subject string, raw []byte) error {
		var evt events.Event[events.DeployStoppedPayload]
		if err := json.Unmarshal(raw, &evt); err == nil && evt.Data.ServiceID != "" {
			msg := fmt.Sprintf("[adroit-system] Deployment %s stopped and decommissioned", evt.Data.DeploymentID)
			s.IngestSystemLog(ctx, evt.Data.ServiceID, &evt.Data.DeploymentID, msg)
		}
		return nil
	})
	if err != nil {
		s.log.Warn("Failed to subscribe to deploy stopped events", slog.Any("error", err))
	}

	return nil
}

// PurgeLogs deletes historical logs before the specified cutoff date
func (s *Service) PurgeLogs(ctx context.Context, userID, serviceID string, before time.Time) (int64, error) {
	svc, err := s.repo.GetService(ctx, serviceID)
	if err != nil {
		return 0, err
	}

	if userID != "" {
		hasAccess, err := s.repo.UserHasOrgAccess(ctx, userID, svc.OrganizationID)
		if err != nil {
			return 0, err
		}
		if !hasAccess {
			return 0, ErrUnauthorized
		}
	}

	return s.repo.PurgeLogs(ctx, serviceID, before)
}
