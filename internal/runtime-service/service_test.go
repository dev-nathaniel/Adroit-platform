package runtimeservice

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/orchestrator"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// mockRepository implements Repository for unit tests
type mockRepository struct {
	services       map[string]*types.Service
	policies       map[string]*types.ServiceAutoScalingPolicy
	samples        map[string][]types.LiveServiceMetrics
	events         map[string][]types.ScalingEvent
	updateReplicas map[string]int
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		services:       make(map[string]*types.Service),
		policies:       make(map[string]*types.ServiceAutoScalingPolicy),
		samples:        make(map[string][]types.LiveServiceMetrics),
		events:         make(map[string][]types.ScalingEvent),
		updateReplicas: make(map[string]int),
	}
}

func (m *mockRepository) GetService(ctx context.Context, serviceID string) (*types.Service, error) {
	if s, ok := m.services[serviceID]; ok {
		return s, nil
	}
	return nil, orchestrator.ErrJobNotFound
}

func (m *mockRepository) GetAutoscalePolicy(ctx context.Context, serviceID string) (*types.ServiceAutoScalingPolicy, error) {
	if p, ok := m.policies[serviceID]; ok {
		return p, nil
	}
	return nil, orchestrator.ErrJobNotFound
}

func (m *mockRepository) UpdateAutoscalePolicy(ctx context.Context, serviceID string, req UpdateAutoscalePolicyRequest) (*types.ServiceAutoScalingPolicy, error) {
	p, ok := m.policies[serviceID]
	if !ok {
		p = &types.ServiceAutoScalingPolicy{ServiceID: serviceID}
		m.policies[serviceID] = p
	}
	if req.AutoScalingEnabled != nil {
		p.AutoScalingEnabled = *req.AutoScalingEnabled
	}
	if req.MinReplicas != nil {
		p.MinReplicas = *req.MinReplicas
	}
	if req.MaxReplicas != nil {
		p.MaxReplicas = *req.MaxReplicas
	}
	if req.TargetCPUPercent != nil {
		p.TargetCPUPercent = *req.TargetCPUPercent
	}
	if req.TargetMemoryPercent != nil {
		p.TargetMemoryPercent = *req.TargetMemoryPercent
	}
	if req.TargetRPS != nil {
		p.TargetRPS = *req.TargetRPS
	}
	return p, nil
}

func (m *mockRepository) ListActiveAutoscalingServices(ctx context.Context) ([]*types.Service, error) {
	var list []*types.Service
	for _, s := range m.services {
		if s.AutoScalingEnabled {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockRepository) RecordMetricsSample(ctx context.Context, sample *types.LiveServiceMetrics) error {
	m.samples[sample.ServiceID] = append(m.samples[sample.ServiceID], *sample)
	return nil
}

func (m *mockRepository) GetLatestMetrics(ctx context.Context, serviceID string) (*types.LiveServiceMetrics, error) {
	samples := m.samples[serviceID]
	if len(samples) == 0 {
		return nil, nil
	}
	return &samples[len(samples)-1], nil
}

func (m *mockRepository) GetMetricsHistory(ctx context.Context, serviceID string, limit int) ([]types.LiveServiceMetrics, error) {
	return m.samples[serviceID], nil
}

func (m *mockRepository) RecordScalingEvent(ctx context.Context, event *types.ScalingEvent) error {
	event.ID = "evt-test-1"
	m.events[event.ServiceID] = append(m.events[event.ServiceID], *event)
	return nil
}

func (m *mockRepository) ListScalingEvents(ctx context.Context, serviceID string, limit int) ([]types.ScalingEvent, error) {
	return m.events[serviceID], nil
}

func (m *mockRepository) UpdateCurrentReplicas(ctx context.Context, serviceID string, replicas int, scaledAt time.Time) error {
	m.updateReplicas[serviceID] = replicas
	if s, ok := m.services[serviceID]; ok {
		s.CurrentReplicas = replicas
		s.LastScaledAt = &scaledAt
	}
	if p, ok := m.policies[serviceID]; ok {
		p.CurrentReplicas = replicas
		p.LastScaledAt = &scaledAt
	}
	return nil
}

// Tests
func TestPolicyEvaluator(t *testing.T) {
	eval := NewPolicyEvaluator()
	now := time.Now().UTC()

	t.Run("Disabled Autoscaling", func(t *testing.T) {
		policy := &types.ServiceAutoScalingPolicy{
			AutoScalingEnabled: false,
			CurrentReplicas:    2,
		}
		metrics := &types.LiveServiceMetrics{
			CPUPercent: 95.0,
		}
		decision := eval.Evaluate(policy, metrics, now)
		if decision.ShouldScale {
			t.Errorf("expected ShouldScale=false when autoscaling is disabled")
		}
	})

	t.Run("Scale Up on CPU Spike", func(t *testing.T) {
		policy := &types.ServiceAutoScalingPolicy{
			AutoScalingEnabled:       true,
			MinReplicas:              1,
			MaxReplicas:              5,
			CurrentReplicas:          1,
			TargetCPUPercent:         70,
			ScaleUpCooldownSeconds:   30,
			ScaleDownCooldownSeconds: 180,
		}
		metrics := &types.LiveServiceMetrics{
			CPUPercent: 90.0, // 90 / 70 = 1.28 -> ceil(1 * 1.28) = 2
		}
		decision := eval.Evaluate(policy, metrics, now)
		if !decision.ShouldScale {
			t.Errorf("expected ShouldScale=true on CPU spike")
		}
		if decision.DesiredReplicas != 2 {
			t.Errorf("expected DesiredReplicas=2, got %d", decision.DesiredReplicas)
		}
		if decision.TriggerMetric != "cpu" {
			t.Errorf("expected trigger metric 'cpu', got %s", decision.TriggerMetric)
		}
	})

	t.Run("Capped at Max Replicas", func(t *testing.T) {
		policy := &types.ServiceAutoScalingPolicy{
			AutoScalingEnabled:       true,
			MinReplicas:              1,
			MaxReplicas:              3,
			CurrentReplicas:          1,
			TargetCPUPercent:         20,
			ScaleUpCooldownSeconds:   30,
			ScaleDownCooldownSeconds: 180,
		}
		metrics := &types.LiveServiceMetrics{
			CPUPercent: 95.0, // 95 / 20 = 4.75 -> ceil(1 * 4.75) = 5 -> capped at 3
		}
		decision := eval.Evaluate(policy, metrics, now)
		if !decision.ShouldScale {
			t.Errorf("expected ShouldScale=true")
		}
		if decision.DesiredReplicas != 3 {
			t.Errorf("expected DesiredReplicas=3, got %d", decision.DesiredReplicas)
		}
	})

	t.Run("Scale Down on Low Utilization", func(t *testing.T) {
		pastTime := now.Add(-5 * time.Minute)
		policy := &types.ServiceAutoScalingPolicy{
			AutoScalingEnabled:       true,
			MinReplicas:              1,
			MaxReplicas:              5,
			CurrentReplicas:          3,
			TargetCPUPercent:         80,
			ScaleUpCooldownSeconds:   30,
			ScaleDownCooldownSeconds: 180,
			LastScaledAt:             &pastTime,
		}
		metrics := &types.LiveServiceMetrics{
			CPUPercent: 15.0, // 15 / 80 = 0.18 -> ceil(3 * 0.18) = 1
		}
		decision := eval.Evaluate(policy, metrics, now)
		if !decision.ShouldScale {
			t.Errorf("expected ShouldScale=true on scale down")
		}
		if decision.DesiredReplicas != 1 {
			t.Errorf("expected DesiredReplicas=1, got %d", decision.DesiredReplicas)
		}
	})

	t.Run("Suppressed by Scale Down Cooldown Window", func(t *testing.T) {
		recentTime := now.Add(-30 * time.Second) // 30s ago, cooldown is 180s
		policy := &types.ServiceAutoScalingPolicy{
			AutoScalingEnabled:       true,
			MinReplicas:              1,
			MaxReplicas:              5,
			CurrentReplicas:          3,
			TargetCPUPercent:         80,
			ScaleUpCooldownSeconds:   30,
			ScaleDownCooldownSeconds: 180,
			LastScaledAt:             &recentTime,
		}
		metrics := &types.LiveServiceMetrics{
			CPUPercent: 10.0,
		}
		decision := eval.Evaluate(policy, metrics, now)
		if decision.ShouldScale {
			t.Errorf("expected ShouldScale=false due to cooldown suppression")
		}
		if !decision.CooldownSkipped {
			t.Errorf("expected CooldownSkipped=true")
		}
	})
}

func TestController_ReconcileService(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	driver := orchestrator.NewSimulatedDriver(10 * time.Millisecond)
	eventBus := events.NewInMemBus()
	defer eventBus.Close()

	svcID := "svc-autoscale-1"
	repo.services[svcID] = &types.Service{
		ID:                 svcID,
		Name:               "Test Workload",
		Slug:               "test-workload",
		CurrentReplicas:    1,
		MinReplicas:        1,
		MaxReplicas:        4,
		AutoScalingEnabled: true,
		Resources: types.ResourceLimits{
			CPUCores: 1.0,
			MemoryMB: 512,
		},
	}
	repo.policies[svcID] = &types.ServiceAutoScalingPolicy{
		ServiceID:                svcID,
		AutoScalingEnabled:       true,
		MinReplicas:              1,
		MaxReplicas:              4,
		CurrentReplicas:          1,
		TargetCPUPercent:         75,
		ScaleUpCooldownSeconds:   10,
		ScaleDownCooldownSeconds: 60,
	}

	collector := NewClusterMetricsCollector("", driver, logger)
	evaluator := NewPolicyEvaluator()
	ctrl := NewAutoScaleController(repo, collector, evaluator, driver, eventBus, logger, 10*time.Second)

	// Register job in driver
	jobCfg := orchestrator.JobConfig{
		DeploymentID: "dep-test-1",
		ServiceID:    svcID,
		ServiceName:  "Test Workload",
		ServiceSlug:  "test-workload",
		ImageTag:     "registry.adroit.run/test:v1",
		Port:         8080,
		Replicas:     1,
	}
	spec, _ := orchestrator.GenerateJobSpec(jobCfg)
	_, _ = driver.SubmitJob(ctx, spec)

	// Inject 95% CPU load
	collector.InjectSimulatedMetrics(svcID, 95.0, 30.0, 50.0)

	decision, err := ctrl.ReconcileService(ctx, svcID)
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	if !decision.ShouldScale {
		t.Errorf("expected ShouldScale=true")
	}
	if decision.DesiredReplicas != 2 {
		t.Errorf("expected DesiredReplicas=2, got %d", decision.DesiredReplicas)
	}

	// Verify database was updated
	if repo.updateReplicas[svcID] != 2 {
		t.Errorf("expected repo replica update to 2, got %d", repo.updateReplicas[svcID])
	}

	// Verify scaling audit event was recorded
	events := repo.events[svcID]
	if len(events) == 0 {
		t.Errorf("expected scaling event to be recorded in repository")
	} else if events[0].Status != "applied" {
		t.Errorf("expected event status 'applied', got %s", events[0].Status)
	}
}
