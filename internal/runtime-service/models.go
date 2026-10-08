package runtimeservice

import (
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// UpdateAutoscalePolicyRequest allows partial or full update of scaling parameters
type UpdateAutoscalePolicyRequest struct {
	AutoScalingEnabled       *bool `json:"auto_scaling_enabled,omitempty"`
	MinReplicas              *int  `json:"min_replicas,omitempty"`
	MaxReplicas              *int  `json:"max_replicas,omitempty"`
	TargetCPUPercent         *int  `json:"target_cpu_percent,omitempty"`
	TargetMemoryPercent      *int  `json:"target_memory_percent,omitempty"`
	TargetRPS                *int  `json:"target_rps,omitempty"`
	ScaleUpCooldownSeconds   *int  `json:"scale_up_cooldown_seconds,omitempty"`
	ScaleDownCooldownSeconds *int  `json:"scale_down_cooldown_seconds,omitempty"`
}

// TriggerScaleRequest allows manual or simulated override of target replica count
type TriggerScaleRequest struct {
	Replicas int    `json:"replicas"`
	Reason   string `json:"reason,omitempty"`
}

// LiveMetricsResponse encapsulates point-in-time metrics for a service
type LiveMetricsResponse struct {
	Metrics types.LiveServiceMetrics `json:"metrics"`
}

// MetricsHistoryResponse encapsulates historical sampled metrics for chart rendering
type MetricsHistoryResponse struct {
	ServiceID string                     `json:"service_id"`
	Samples   []types.LiveServiceMetrics `json:"samples"`
	Count     int                        `json:"count"`
}

// ScalingEventsResponse returns paginated scaling history
type ScalingEventsResponse struct {
	ServiceID string               `json:"service_id"`
	Events    []types.ScalingEvent `json:"events"`
	Count     int                  `json:"count"`
}

// ScalingDecision captures evaluation outcome for an autoscaling cycle
type ScalingDecision struct {
	ShouldScale      bool
	PreviousReplicas int
	DesiredReplicas  int
	Reason           string
	TriggerMetric    string
	MetricValue      float64
	TargetValue      float64
	CooldownSkipped  bool
}
