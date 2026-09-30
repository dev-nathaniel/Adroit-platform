package orchestrator

import (
	"context"
	"errors"
	"time"
)

var (
	ErrJobNotFound        = errors.New("nomad job not found")
	ErrAllocationNotFound = errors.New("nomad allocation not found")
	ErrCanaryNotReady     = errors.New("canary allocation is not healthy or ready for promotion")
	ErrDriverUnavailable  = errors.New("nomad orchestrator driver unavailable")
	ErrInvalidJobSpec     = errors.New("invalid nomad job specification")
)

// TaskState tracks execution status of an individual task inside an allocation
type TaskState struct {
	State      string    `json:"state"` // "running", "dead", "pending"
	Failed     bool      `json:"failed"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Restarts   int       `json:"restarts"`
	Events     []string  `json:"events,omitempty"`
}

// AllocationInfo represents a scheduled workload unit placed on a Nomad client node
type AllocationInfo struct {
	ID            string               `json:"id"`
	JobID         string               `json:"job_id"`
	DeploymentID  string               `json:"deployment_id"`
	NodeID        string               `json:"node_id"`
	NodeName      string               `json:"node_name"`
	ClientStatus  string               `json:"client_status"` // "running", "pending", "failed", "complete"
	DesiredStatus string               `json:"desired_status"` // "run", "stop"
	Healthy       bool                 `json:"healthy"`
	IsCanary      bool                 `json:"is_canary"`
	TaskStates    map[string]TaskState `json:"task_states,omitempty"`
	ModifyIndex   uint64               `json:"modify_index"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

// JobStatus describes the overall state of a Nomad job
type JobStatus struct {
	JobID             string    `json:"job_id"`
	Status            string    `json:"status"` // "running", "pending", "dead"
	StatusDescription string    `json:"status_description,omitempty"`
	Type              string    `json:"type"` // "service", "batch", "system"
	Version           uint64    `json:"version"`
	ModifyIndex       uint64    `json:"modify_index"`
	ActiveDeployment  string    `json:"active_deployment,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// JobSubmissionResult contains metadata returned after submitting or registering a job
type JobSubmissionResult struct {
	JobID            string   `json:"job_id"`
	EvalID           string   `json:"eval_id"`
	EvalCreateIndex  uint64   `json:"eval_create_index"`
	JobModifyIndex   uint64   `json:"job_modify_index"`
	KnownLeader      bool     `json:"known_leader"`
	LastIndex        uint64   `json:"last_index"`
	Warnings         []string `json:"warnings,omitempty"`
}

// HealthResult aggregates health check statuses across allocations of a job
type HealthResult struct {
	Healthy      bool   `json:"healthy"`
	TotalCount   int    `json:"total_count"`
	PassingCount int    `json:"passing_count"`
	FailingCount int    `json:"failing_count"`
	CanaryReady  bool   `json:"canary_ready"`
	Message      string `json:"message"`
}

// Driver defines the pluggable contract for interacting with the Nomad scheduling cluster
type Driver interface {
	// SubmitJob registers or updates a job on Nomad from an HCL2 job specification string
	SubmitJob(ctx context.Context, jobSpecHCL string) (*JobSubmissionResult, error)

	// GetJob inspects the state of a registered job
	GetJob(ctx context.Context, jobID string) (*JobStatus, error)

	// GetAllocations returns all active and historical allocations associated with a job
	GetAllocations(ctx context.Context, jobID string) ([]AllocationInfo, error)

	// PromoteCanary approves and promotes canary allocations during blue-green / canary rollouts
	PromoteCanary(ctx context.Context, jobID string) error

	// ScaleJob updates the instance count for a service's task group
	ScaleJob(ctx context.Context, jobID string, count int) error

	// StopJob deregisters or stops a Nomad job
	StopJob(ctx context.Context, jobID string, purge bool) error

	// CheckHealth inspects allocations and Consul health check results for a job
	CheckHealth(ctx context.Context, jobID string) (*HealthResult, error)
}
