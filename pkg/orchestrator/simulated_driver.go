package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

var (
	reJobID        = regexp.MustCompile(`job\s+"([^"]+)"`)
	reDeployID     = regexp.MustCompile(`"adroit\.deploy_id"\s*=\s*"([^"]+)"`)
	reServiceID    = regexp.MustCompile(`"adroit\.service_id"\s*=\s*"([^"]+)"`)
	reStrategy     = regexp.MustCompile(`"adroit\.strategy"\s*=\s*"([^"]+)"`)
	reCount        = regexp.MustCompile(`count\s*=\s*(\d+)`)
)

type simulatedJob struct {
	jobID          string
	serviceID      string
	deploymentID   string
	strategy       types.RolloutStrategy
	version        uint64
	replicas       int
	status         string
	hclSpec        string
	allocations    []*AllocationInfo
	canaryPromoted bool
	createdAt      time.Time
	updatedAt      time.Time
}

// SimulatedDriver provides an in-memory, zero-dependency Nomad driver for local development and testing
type SimulatedDriver struct {
	mu              sync.RWMutex
	jobs            map[string]*simulatedJob
	healthDelay     time.Duration
	simulateFailure bool
	nodeNames       []string
}

// NewSimulatedDriver creates an initialized simulated Nomad scheduler
func NewSimulatedDriver(healthDelay time.Duration) *SimulatedDriver {
	if healthDelay <= 0 {
		healthDelay = 50 * time.Millisecond
	}
	return &SimulatedDriver{
		jobs:        make(map[string]*simulatedJob),
		healthDelay: healthDelay,
		nodeNames:   []string{"adroit-worker-01", "adroit-worker-02", "adroit-worker-03"},
	}
}

// SetSimulateFailure toggles failure simulation for testing error handling and auto-revert
func (d *SimulatedDriver) SetSimulateFailure(fail bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.simulateFailure = fail
}

// SubmitJob registers or updates a job, placing allocations
func (d *SimulatedDriver) SubmitJob(ctx context.Context, jobSpecHCL string) (*JobSubmissionResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.simulateFailure {
		return nil, fmt.Errorf("simulated Nomad scheduler rejection: node capacity exceeded")
	}

	jobIDMatch := reJobID.FindStringSubmatch(jobSpecHCL)
	if len(jobIDMatch) < 2 {
		return nil, fmt.Errorf("%w: missing job identifier", ErrInvalidJobSpec)
	}
	jobID := jobIDMatch[1]

	var deployID, serviceID string
	if m := reDeployID.FindStringSubmatch(jobSpecHCL); len(m) >= 2 {
		deployID = m[1]
	}
	if m := reServiceID.FindStringSubmatch(jobSpecHCL); len(m) >= 2 {
		serviceID = m[1]
	}

	strategy := types.RolloutStrategyRolling
	if m := reStrategy.FindStringSubmatch(jobSpecHCL); len(m) >= 2 {
		strategy = types.RolloutStrategy(m[1])
	}

	replicas := 1
	if m := reCount.FindStringSubmatch(jobSpecHCL); len(m) >= 2 {
		if c, err := strconv.Atoi(m[1]); err == nil && c > 0 {
			replicas = c
		}
	}

	now := time.Now().UTC()
	job, exists := d.jobs[jobID]
	if !exists {
		job = &simulatedJob{
			jobID:        jobID,
			serviceID:    serviceID,
			deploymentID: deployID,
			strategy:     strategy,
			version:      0,
			replicas:     replicas,
			status:       "running",
			hclSpec:      jobSpecHCL,
			createdAt:    now,
			updatedAt:    now,
		}
		d.jobs[jobID] = job
	} else {
		job.version++
		job.deploymentID = deployID
		job.strategy = strategy
		job.replicas = replicas
		job.hclSpec = jobSpecHCL
		job.status = "running"
		job.canaryPromoted = false
		job.updatedAt = now
	}

	// Create simulated allocations
	var newAllocs []*AllocationInfo
	if strategy == types.RolloutStrategyCanary {
		// Spawn 1 canary allocation first
		canaryAlloc := d.createAllocation(jobID, deployID, true, 0)
		newAllocs = append(newAllocs, canaryAlloc)
	} else {
		// Spawn full replicas
		for i := 0; i < replicas; i++ {
			alloc := d.createAllocation(jobID, deployID, false, i)
			newAllocs = append(newAllocs, alloc)
		}
	}

	job.allocations = newAllocs

	evalID := "eval-" + randomHex(8)
	return &JobSubmissionResult{
		JobID:           jobID,
		EvalID:          evalID,
		EvalCreateIndex: job.version + 1,
		JobModifyIndex:  job.version + 1,
		KnownLeader:     true,
		LastIndex:       job.version + 1,
	}, nil
}

// GetJob returns job state
func (d *SimulatedDriver) GetJob(ctx context.Context, jobID string) (*JobStatus, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	job, exists := d.jobs[jobID]
	if !exists {
		return nil, ErrJobNotFound
	}

	return &JobStatus{
		JobID:            job.jobID,
		Status:           job.status,
		Type:             "service",
		Version:          job.version,
		ModifyIndex:      job.version,
		ActiveDeployment: job.deploymentID,
		CreatedAt:        job.createdAt,
		UpdatedAt:        job.updatedAt,
	}, nil
}

// GetAllocations returns allocations for a job
func (d *SimulatedDriver) GetAllocations(ctx context.Context, jobID string) ([]AllocationInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	job, exists := d.jobs[jobID]
	if !exists {
		return nil, ErrJobNotFound
	}

	result := make([]AllocationInfo, len(job.allocations))
	for i, a := range job.allocations {
		result[i] = *a
	}
	return result, nil
}

// PromoteCanary promotes canary allocation and provisions remaining instances
func (d *SimulatedDriver) PromoteCanary(ctx context.Context, jobID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	job, exists := d.jobs[jobID]
	if !exists {
		return ErrJobNotFound
	}

	// Verify canary exists and is healthy
	var hasHealthyCanary bool
	for _, a := range job.allocations {
		if a.IsCanary && a.Healthy {
			hasHealthyCanary = true
			break
		}
	}

	if !hasHealthyCanary {
		return ErrCanaryNotReady
	}

	job.canaryPromoted = true
	job.updatedAt = time.Now().UTC()

	// Provision remaining replicas
	currentCount := len(job.allocations)
	for i := currentCount; i < job.replicas; i++ {
		alloc := d.createAllocation(job.jobID, job.deploymentID, false, i)
		job.allocations = append(job.allocations, alloc)
	}

	return nil
}

// ScaleJob modifies the replica count of an active service
func (d *SimulatedDriver) ScaleJob(ctx context.Context, jobID string, count int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	job, exists := d.jobs[jobID]
	if !exists {
		return ErrJobNotFound
	}

	if count < 0 {
		return fmt.Errorf("replica count cannot be negative")
	}

	job.replicas = count
	job.updatedAt = time.Now().UTC()

	currentActive := 0
	for _, a := range job.allocations {
		if a.ClientStatus == "running" && a.DesiredStatus == "run" {
			currentActive++
		}
	}

	if count > currentActive {
		// Scale up: add new running allocations
		diff := count - currentActive
		for i := 0; i < diff; i++ {
			alloc := d.createAllocation(job.jobID, job.deploymentID, false, currentActive+i)
			job.allocations = append(job.allocations, alloc)
		}
	} else if count < currentActive {
		// Scale down: mark excess as stopped
		diff := currentActive - count
		stopped := 0
		for i := len(job.allocations) - 1; i >= 0 && stopped < diff; i-- {
			if job.allocations[i].ClientStatus == "running" {
				job.allocations[i].ClientStatus = "complete"
				job.allocations[i].DesiredStatus = "stop"
				job.allocations[i].UpdatedAt = time.Now().UTC()
				stopped++
			}
		}
	}

	return nil
}

// StopJob stops a job and its allocations
func (d *SimulatedDriver) StopJob(ctx context.Context, jobID string, purge bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	job, exists := d.jobs[jobID]
	if !exists {
		return ErrJobNotFound
	}

	now := time.Now().UTC()
	job.status = "dead"
	job.updatedAt = now

	for _, a := range job.allocations {
		a.ClientStatus = "complete"
		a.DesiredStatus = "stop"
		a.UpdatedAt = now
	}

	if purge {
		delete(d.jobs, jobID)
	}

	return nil
}

// CheckHealth inspects health check states, marking allocations healthy after healthDelay
func (d *SimulatedDriver) CheckHealth(ctx context.Context, jobID string) (*HealthResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	job, exists := d.jobs[jobID]
	if !exists {
		return nil, ErrJobNotFound
	}

	now := time.Now().UTC()
	total := len(job.allocations)
	passing := 0
	failing := 0
	canaryReady := false

	for _, a := range job.allocations {
		if a.ClientStatus != "running" {
			failing++
			continue
		}

		// Mark healthy once health delay has elapsed
		if !a.Healthy && now.Sub(a.CreatedAt) >= d.healthDelay {
			a.Healthy = true
			a.UpdatedAt = now
		}

		if a.Healthy {
			passing++
			if a.IsCanary {
				canaryReady = true
			}
		} else {
			failing++
		}
	}

	healthy := (passing == total && total > 0)
	msg := fmt.Sprintf("%d/%d allocations healthy", passing, total)
	if job.strategy == types.RolloutStrategyCanary && !job.canaryPromoted {
		if canaryReady {
			msg = "Canary allocation healthy, awaiting promotion"
		} else {
			msg = "Canary allocation warming up"
		}
	}

	return &HealthResult{
		Healthy:      healthy,
		TotalCount:   total,
		PassingCount: passing,
		FailingCount: failing,
		CanaryReady:  canaryReady,
		Message:      msg,
	}, nil
}

func (d *SimulatedDriver) createAllocation(jobID, deployID string, isCanary bool, index int) *AllocationInfo {
	now := time.Now().UTC()
	allocID := "alloc-" + randomHex(8)
	nodeIdx := index % len(d.nodeNames)
	nodeName := d.nodeNames[nodeIdx]

	return &AllocationInfo{
		ID:            allocID,
		JobID:         jobID,
		DeploymentID:  deployID,
		NodeID:        fmt.Sprintf("node-%d", nodeIdx+1),
		NodeName:      nodeName,
		ClientStatus:  "running",
		DesiredStatus: "run",
		Healthy:       false, // initially unverified
		IsCanary:      isCanary,
		TaskStates: map[string]TaskState{
			"server": {
				State:     "running",
				Failed:    false,
				StartedAt: now,
				Restarts:  0,
			},
		},
		ModifyIndex: 1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
