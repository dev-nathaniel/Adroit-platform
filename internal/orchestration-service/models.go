package orchestrationservice

import (
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// TriggerDeployRequest payload for manually initiating a service deployment
type TriggerDeployRequest struct {
	ServiceID string                `json:"service_id"`
	BuildID   string                `json:"build_id,omitempty"`
	Replicas  int                   `json:"replicas,omitempty"`
	Strategy  types.RolloutStrategy `json:"strategy,omitempty"`
}

// ScaleServiceRequest payload for scaling service task group replicas
type ScaleServiceRequest struct {
	Replicas int `json:"replicas"`
}

// PromoteDeploymentRequest payload for manual canary promotion
type PromoteDeploymentRequest struct {
	DeploymentID string `json:"deployment_id"`
}

// RollbackRequest payload for reverting a service to an earlier deployment
type RollbackRequest struct {
	TargetDeploymentID string `json:"target_deployment_id,omitempty"`
}

// DeploymentDetail encapsulates full status, service metadata, allocations, and rollout audit history
type DeploymentDetail struct {
	Deployment  types.Deployment        `json:"deployment"`
	Service     *types.Service          `json:"service,omitempty"`
	Allocations []types.Allocation      `json:"allocations"`
	Events      []types.DeploymentEvent `json:"events,omitempty"`
}
