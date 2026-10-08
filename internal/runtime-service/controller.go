package runtimeservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/orchestrator"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// AutoScaleController manages continuous metrics evaluation and workload reconciliation
type AutoScaleController struct {
	repo      Repository
	collector MetricsCollector
	evaluator *PolicyEvaluator
	driver    orchestrator.Driver
	eventBus  events.EventBus
	logger    *slog.Logger

	interval time.Duration
	running  bool
	mu       sync.Mutex
	stopCh   chan struct{}
}

// NewAutoScaleController constructs an initialized AutoScaleController
func NewAutoScaleController(
	repo Repository,
	collector MetricsCollector,
	evaluator *PolicyEvaluator,
	driver orchestrator.Driver,
	eventBus events.EventBus,
	logger *slog.Logger,
	interval time.Duration,
) *AutoScaleController {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &AutoScaleController{
		repo:      repo,
		collector: collector,
		evaluator: evaluator,
		driver:    driver,
		eventBus:  eventBus,
		logger:    logger,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// Start launches the background periodic reconciliation loop
func (c *AutoScaleController) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	c.logger.Info("Starting AutoScaleController reconciliation loop", slog.Duration("interval", c.interval))

	ticker := time.NewTicker(c.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				c.logger.Info("Stopping AutoScaleController (context cancelled)")
				return
			case <-c.stopCh:
				c.logger.Info("Stopping AutoScaleController (stop signal)")
				return
			case <-ticker.C:
				c.reconcileAll(ctx)
			}
		}
	}()
}

// Stop terminates the reconciliation worker
func (c *AutoScaleController) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		c.running = false
		close(c.stopCh)
	}
}

// reconcileAll iterates over all active services configured with autoscaling
func (c *AutoScaleController) reconcileAll(ctx context.Context) {
	services, err := c.repo.ListActiveAutoscalingServices(ctx)
	if err != nil {
		c.logger.Error("Failed to list active autoscaling services", slog.String("error", err.Error()))
		return
	}

	for _, svc := range services {
		if _, err := c.ReconcileService(ctx, svc.ID); err != nil {
			c.logger.Warn("Reconciliation failed for service", slog.String("service_id", svc.ID), slog.String("error", err.Error()))
		}
	}
}

// ReconcileService performs a single metrics evaluation cycle and executes scaling actions if warranted
func (c *AutoScaleController) ReconcileService(ctx context.Context, serviceID string) (*ScalingDecision, error) {
	svc, err := c.repo.GetService(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service: %w", err)
	}

	policy, err := c.repo.GetAutoscalePolicy(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch policy: %w", err)
	}

	// 1. Collect live workload metrics
	metrics, err := c.collector.CollectServiceMetrics(ctx, svc)
	if err != nil {
		c.logger.Warn("Failed to sample metrics for service", slog.String("service_id", serviceID), slog.String("error", err.Error()))
	} else if metrics != nil {
		// Save metrics sample in database for time-series charting
		_ = c.repo.RecordMetricsSample(ctx, metrics)
	}

	if metrics == nil {
		return nil, fmt.Errorf("no telemetry metrics available for service %s", serviceID)
	}

	// 2. Evaluate policy against metrics
	decision := c.evaluator.Evaluate(policy, metrics, time.Now().UTC())

	// 3. Handle stabilization window cooldown skip (record audit entry)
	if decision.CooldownSkipped {
		c.logger.Debug("Scaling suppressed by cooldown window",
			slog.String("service_id", serviceID),
			slog.String("reason", decision.Reason),
		)
		return &decision, nil
	}

	// 4. Apply scale adjustment if warranted
	if decision.ShouldScale {
		jobID := orchestrator.GenerateJobID(serviceID)
		c.logger.Info("Auto-scaling workload",
			slog.String("service_id", serviceID),
			slog.String("job_id", jobID),
			slog.Int("from", decision.PreviousReplicas),
			slog.Int("to", decision.DesiredReplicas),
			slog.String("reason", decision.Reason),
		)

		// A. Execute scale on Nomad cluster
		if c.driver != nil {
			if err := c.driver.ScaleJob(ctx, jobID, decision.DesiredReplicas); err != nil {
				if errors.Is(err, orchestrator.ErrJobNotFound) {
					c.logger.Debug("Nomad job not found for service, skipping scale", slog.String("job_id", jobID))
					return &decision, nil
				}
				c.logger.Error("Failed to scale Nomad job",
					slog.String("job_id", jobID),
					slog.String("error", err.Error()),
				)
				_ = c.repo.RecordScalingEvent(ctx, &types.ScalingEvent{
					ServiceID:        serviceID,
					PreviousReplicas: decision.PreviousReplicas,
					DesiredReplicas:  decision.DesiredReplicas,
					Reason:           fmt.Sprintf("Nomad scale execution error: %v", err),
					TriggerMetric:    decision.TriggerMetric,
					MetricValue:      decision.MetricValue,
					TargetValue:      decision.TargetValue,
					Status:           "failed",
					CreatedAt:        time.Now().UTC(),
				})
				return &decision, err
			}
		}

		// B. Update database state
		now := time.Now().UTC()
		_ = c.repo.UpdateCurrentReplicas(ctx, serviceID, decision.DesiredReplicas, now)

		// C. Record scaling event log
		_ = c.repo.RecordScalingEvent(ctx, &types.ScalingEvent{
			ServiceID:        serviceID,
			PreviousReplicas: decision.PreviousReplicas,
			DesiredReplicas:  decision.DesiredReplicas,
			Reason:           decision.Reason,
			TriggerMetric:    decision.TriggerMetric,
			MetricValue:      decision.MetricValue,
			TargetValue:      decision.TargetValue,
			Status:           "applied",
			CreatedAt:        now,
		})

		// D. Emit NATS event
		if c.eventBus != nil {
			payload := events.ServiceScaledPayload{
				ServiceID:        serviceID,
				PreviousReplicas: decision.PreviousReplicas,
				DesiredReplicas:  decision.DesiredReplicas,
				Reason:           decision.Reason,
				TriggerMetric:    decision.TriggerMetric,
				MetricValue:      decision.MetricValue,
				TargetValue:      decision.TargetValue,
				ScaledAt:         now,
			}
			_ = c.eventBus.Publish(ctx, events.SubjectServiceScaled, payload)
		}
	}

	return &decision, nil
}
